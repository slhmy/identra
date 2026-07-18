// Package sqlite provides the SQLite persistence implementation used by Identra.
package sqlite

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

// CurrentSchemaVersion is the newest database schema understood by this build.
const CurrentSchemaVersion = 3

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Config struct {
	Path string
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Path) == "" {
		return errors.New("sqlite database path is required")
	}
	return nil
}

func Open(cfg Config) (*sql.DB, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if dir := filepath.Dir(cfg.Path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create sqlite database directory: %w", err)
		}
	}

	dsn := "file:" + filepath.ToSlash(cfg.Path) + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	// SQLite serializes writes. A single shared connection also ensures every
	// query uses the same connection-level PRAGMA configuration from the DSN.
	db.SetMaxOpenConns(1)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}
	if err := applyMigrations(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	return db, nil
}

func applyMigrations(db *sql.DB) error {
	var currentVersion int
	if err := db.QueryRow("PRAGMA user_version").Scan(&currentVersion); err != nil {
		return fmt.Errorf("read sqlite schema version: %w", err)
	}
	if currentVersion > CurrentSchemaVersion {
		return fmt.Errorf("sqlite schema version %d is newer than supported version %d", currentVersion, CurrentSchemaVersion)
	}

	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read embedded sqlite migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	lastVersion := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		prefix, _, ok := strings.Cut(entry.Name(), "_")
		if !ok {
			return fmt.Errorf("invalid sqlite migration filename %q", entry.Name())
		}
		version, err := strconv.Atoi(prefix)
		if err != nil || version != lastVersion+1 {
			return fmt.Errorf("sqlite migration %q is not the next sequential version after %d", entry.Name(), lastVersion)
		}
		lastVersion = version
		if version <= currentVersion {
			continue
		}

		script, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read sqlite migration %s: %w", entry.Name(), err)
		}
		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin sqlite migration %s: %w", entry.Name(), err)
		}
		if _, err := tx.Exec(string(script)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply sqlite migration %s: %w", entry.Name(), err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", version)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record sqlite migration %s: %w", entry.Name(), err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit sqlite migration %s: %w", entry.Name(), err)
		}
	}

	if lastVersion != CurrentSchemaVersion {
		return fmt.Errorf("embedded sqlite schema version is %d, expected %d", lastVersion, CurrentSchemaVersion)
	}
	return nil
}
