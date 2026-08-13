// Package migrate applies SQLite schema migrations tracked in a database's
// PRAGMA user_version. Migration files are named NNNN_description.sql and are
// applied in ascending numeric order, starting after the current version.
// Each migration runs inside its own transaction: a failure rolls back that
// migration's statements and leaves user_version untouched, so a partially
// applied migration never appears successful.
package migrate

import (
	"database/sql"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
)

type migration struct {
	version    int
	name       string
	statements string
}

func loadMigrations(source fs.FS) ([]migration, error) {
	paths, err := fs.Glob(source, "migrations/*.sql")
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("list migrations: no migration files found")
	}

	loaded := make([]migration, 0, len(paths))
	for _, filePath := range paths {
		name := path.Base(filePath)
		prefix, _, found := strings.Cut(strings.TrimSuffix(name, ".sql"), "_")
		if !found {
			return nil, fmt.Errorf("migration %s: name must be NNNN_description.sql", name)
		}
		version, err := strconv.Atoi(prefix)
		if err != nil {
			return nil, fmt.Errorf("migration %s: unparsable version prefix: %w", name, err)
		}
		body, err := fs.ReadFile(source, filePath)
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", name, err)
		}
		loaded = append(loaded, migration{version: version, name: name, statements: string(body)})
	}

	sort.Slice(loaded, func(i, j int) bool { return loaded[i].version < loaded[j].version })
	return loaded, nil
}

func schemaVersion(db *sql.DB) (int, error) {
	var version int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read user_version: %w", err)
	}
	return version, nil
}

// Apply loads the NNNN_description.sql migrations in source and runs every
// one whose version is greater than db's current user_version, in ascending
// order. Each migration commits its statements and the resulting
// user_version together, so a failure partway through leaves db exactly as
// it was before that migration started.
//
// source must contain a migrations/ directory holding the .sql files; a
// source with no migrations/ directory, or one where it is empty, is an
// error rather than a no-op. Any .sql file inside migrations/ whose name is
// not prefixed with a numeric NNNN version fails the whole run.
func Apply(db *sql.DB, source fs.FS) error {
	loaded, err := loadMigrations(source)
	if err != nil {
		return err
	}
	current, err := schemaVersion(db)
	if err != nil {
		return err
	}

	for _, m := range loaded {
		if m.version <= current {
			continue
		}
		if err := applyMigration(db, m); err != nil {
			return fmt.Errorf("migration %s failed: %w", m.name, err)
		}
	}
	return nil
}

func applyMigration(db *sql.DB, m migration) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(m.statements); err != nil {
		return err
	}
	// PRAGMA user_version does not accept bound parameters, and the version comes
	// from a validated integer file prefix.
	if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.version)); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}
	return tx.Commit()
}
