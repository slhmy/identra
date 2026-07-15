// Package sqlite provides the SQLite persistence implementation used by Identra.
package sqlite

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schemaSQL string

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

	dsn := cfg.Path + "?_foreign_keys=on&_busy_timeout=5000&_journal_mode=WAL"
	db, err := sql.Open("sqlite3", dsn)
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
	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("initialize sqlite schema: %w", err)
	}

	return db, nil
}
