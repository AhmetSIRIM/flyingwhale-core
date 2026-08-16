// Package migrate applies SQLite schema migrations tracked in a database's
// PRAGMA user_version. Migration files are named NNNN_description.sql and are
// applied in ascending numeric order, starting after the current version.
// Each migration runs inside its own transaction: a failure rolls back that
// migration's statements and leaves user_version untouched, so a partially
// applied migration never appears successful.
package migrate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

// ErrNoMigrations reports that source holds no migration files to apply.
var ErrNoMigrations = errors.New("migrate: no migration files found")

type migration struct {
	version    int
	name       string
	statements string
}

func loadMigrations(source fs.FS) ([]migration, error) {
	paths, err := fs.Glob(source, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("migrate: list migrations: %w", err)
	}
	if len(paths) == 0 {
		return nil, ErrNoMigrations
	}

	loaded := make([]migration, 0, len(paths))
	for _, filePath := range paths {
		name := path.Base(filePath)
		prefix, _, found := strings.Cut(strings.TrimSuffix(name, ".sql"), "_")
		if !found {
			return nil, fmt.Errorf("migrate: %s: name must be NNNN_description.sql", name)
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("migrate: %s: unparsable version prefix: %w", name, err)
		}
		body, err := fs.ReadFile(source, filePath)
		if err != nil {
			return nil, fmt.Errorf("migrate: read %s: %w", name, err)
		}
		loaded = append(loaded, migration{version: version, name: name, statements: string(body)})
	}

	sort.Slice(loaded, func(i, j int) bool { return loaded[i].version < loaded[j].version })
	for i := 1; i < len(loaded); i++ {
		if loaded[i].version == loaded[i-1].version {
			return nil, fmt.Errorf("migrate: %s and %s share version %04d", loaded[i-1].name, loaded[i].name, loaded[i].version)
		}
	}
	return loaded, nil
}

func schemaVersion(ctx context.Context, db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("migrate: read user_version: %w", err)
	}
	return version, nil
}

// LatestVersion returns the highest version among the migrations in source,
// so a consumer can assert the schema state it should land on without
// re-parsing the NNNN prefixes itself. It fails for the same reasons Apply
// does: no migration files, a malformed name, an unparsable version prefix,
// a duplicate version, or an unreadable file.
func LatestVersion(source fs.FS) (int, error) {
	loaded, err := loadMigrations(source)
	if err != nil {
		return 0, err
	}
	return loaded[len(loaded)-1].version, nil
}

// Apply loads the NNNN_description.sql migrations in source and runs every
// one whose version is greater than db's current user_version, in ascending
// order. Each migration commits its statements and the resulting
// user_version together, so a failure partway through leaves db exactly as
// it was before that migration started.
//
// source must contain a migrations/ directory holding the .sql files; a
// source with no migrations/ directory, or one where it is empty, is an
// error (ErrNoMigrations) rather than a no-op. Any .sql file inside
// migrations/ whose name is not prefixed with a numeric NNNN version, or
// two files that share a version, fails the whole run.
//
// The context bounds the whole run. A canceled context fails the run
// closed: the transaction of the migration in flight rolls back, so a
// canceled run never leaves a partially applied migration committed.
func Apply(ctx context.Context, db *sql.DB, source fs.FS) error {
	loaded, err := loadMigrations(source)
	if err != nil {
		return err
	}
	current, err := schemaVersion(ctx, db)
	if err != nil {
		return err
	}

	for _, m := range loaded {
		if m.version <= current {
			continue
		}
		if err := applyMigration(ctx, db, m); err != nil {
			return fmt.Errorf("migrate: migration %s failed: %w", m.name, err)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, m migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("migrate: begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, m.statements); err != nil {
		return err
	}
	// PRAGMA user_version does not accept bound parameters, and the version comes
	// from a validated integer file prefix.
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
		return fmt.Errorf("migrate: set user_version: %w", err)
	}
	return tx.Commit()
}
