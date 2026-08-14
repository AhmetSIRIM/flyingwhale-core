package migrate

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"

	_ "modernc.org/sqlite"
)

// openTestDB opens a fresh SQLite database for a single test and closes it
// when the test ends. The ":memory:" DSN gives every pool connection its own
// private database, so the pool is capped at one connection: the test's
// assertions only hold if every query lands on the same database the
// migration ran against.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close() returned error: %v", err)
		}
	})
	return db
}

// queryStrings runs a single-column query and collects every value, so
// schema assertions can compare table lists with go-cmp.
func queryStrings(t *testing.T, db *sql.DB, query string) []string {
	t.Helper()

	rows, err := db.Query(query)
	if err != nil {
		t.Fatalf("query %q returned error: %v", query, err)
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			t.Fatalf("scan of %q returned error: %v", query, err)
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iteration of %q returned error: %v", query, err)
	}
	return values
}

// brokenMigrationFS carries an already-applied migration at alreadyApplied
// plus a next one whose trailing statement is unparsable. Its first
// statement is valid, so a runner without a transaction leaves half_applied
// behind.
func brokenMigrationFS(alreadyApplied int) fstest.MapFS {
	return fstest.MapFS{
		fmt.Sprintf("migrations/%04d_init.sql", alreadyApplied): &fstest.MapFile{
			Data: []byte("CREATE TABLE first_step (id INTEGER PRIMARY KEY);"),
		},
		fmt.Sprintf("migrations/%04d_broken.sql", alreadyApplied+1): &fstest.MapFile{
			Data: []byte("CREATE TABLE half_applied (id INTEGER PRIMARY KEY);\nCREATE TABLE ;"),
		},
	}
}

// TestApplyCreatesTablesAndTracksUserVersion pins the happy path: two
// migrations run in ascending order, each table they create exists
// afterward, and user_version lands on the highest version applied. A
// second Apply call against the same source applies nothing, pinning the
// skip-already-applied rule.
func TestApplyCreatesTablesAndTracksUserVersion(t *testing.T) {
	ctx := t.Context()
	db := openTestDB(t)
	source := fstest.MapFS{
		"migrations/0001_first.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE first_table (id INTEGER PRIMARY KEY);"),
		},
		"migrations/0002_second.sql": &fstest.MapFile{
			Data: []byte("CREATE TABLE second_table (id INTEGER PRIMARY KEY);"),
		},
	}

	if err := Apply(ctx, db, source); err != nil {
		t.Fatalf("Apply() first call returned error: %v", err)
	}

	tables := queryStrings(t, db, `SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	wantTables := []string{"first_table", "second_table"}
	if diff := cmp.Diff(wantTables, tables); diff != "" {
		t.Errorf("tables after Apply() mismatch (-want +got):\n%s", diff)
	}

	version, err := schemaVersion(ctx, db)
	if err != nil {
		t.Fatalf("schemaVersion() returned error: %v", err)
	}
	if version != 2 {
		t.Errorf("user_version after Apply() = %d, want 2", version)
	}

	if err := Apply(ctx, db, source); err != nil {
		t.Fatalf("Apply() second call returned error: %v", err)
	}
	version, err = schemaVersion(ctx, db)
	if err != nil {
		t.Fatalf("schemaVersion() after second Apply() returned error: %v", err)
	}
	if version != 2 {
		t.Errorf("user_version after second Apply() = %d, want 2 (already-applied migrations must not reapply)", version)
	}
}

// TestApplyFailsClosedOnBrokenMigration pins the deploy-safety rule: a
// failing migration reports the offending file, rolls its partial work
// back, and leaves user_version untouched so the caller refuses to treat a
// half-migrated schema as ready.
func TestApplyFailsClosedOnBrokenMigration(t *testing.T) {
	ctx := t.Context()
	db := openTestDB(t)

	before, err := schemaVersion(ctx, db)
	if err != nil {
		t.Fatalf("schemaVersion() before the broken migration returned error: %v", err)
	}

	brokenName := fmt.Sprintf("%04d_broken.sql", before+1)
	err = Apply(ctx, db, brokenMigrationFS(before))
	if err == nil {
		t.Fatal("Apply() returned nil error, want a failure for the broken migration")
	}
	if !strings.Contains(err.Error(), brokenName) {
		t.Errorf("error %q does not name the failing migration file %q", err, brokenName)
	}

	gotVersion, versionErr := schemaVersion(ctx, db)
	if versionErr != nil {
		t.Fatalf("schemaVersion() returned error: %v", versionErr)
	}
	if diff := cmp.Diff(before, gotVersion); diff != "" {
		t.Errorf("user_version after the failed migration mismatch (-want +got):\n%s", diff)
	}

	leftovers := queryStrings(t, db, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'half_applied'`)
	if len(leftovers) != 0 {
		t.Errorf("tables left behind by the failed migration = %v, want none", leftovers)
	}
}

// TestLoadMigrationsRejectsUnnumberedFiles keeps the NNNN_description.sql
// naming contract enforced rather than conventional.
func TestLoadMigrationsRejectsUnnumberedFiles(t *testing.T) {
	source := fstest.MapFS{
		"migrations/init.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
	}

	if _, err := loadMigrations(source); err == nil {
		t.Fatal("loadMigrations() returned nil error, want a failure for a file without a version prefix")
	}
}
