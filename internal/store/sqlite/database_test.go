package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenKeepsLegacyGORMSchemaReadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", "file:"+filepath.ToSlash(path))
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}

	const legacySchema = `
CREATE TABLE users (
    id text PRIMARY KEY,
    created_at datetime,
    updated_at datetime,
    deleted_at datetime,
    email text,
    hashed_password text,
    hash text,
    last_login_at datetime
);
CREATE UNIQUE INDEX idx_users_email ON users (email);
CREATE TABLE external_identities (
    id text PRIMARY KEY,
    user_id text,
    provider text,
    provider_user_id text,
    created_at datetime,
    updated_at datetime,
    deleted_at datetime
);
CREATE UNIQUE INDEX idx_external_identities_provider_provider_user_id
    ON external_identities (provider, provider_user_id);
`
	if _, err := legacy.Exec(legacySchema); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	now := time.Now().UTC().Round(time.Second)
	if _, err := legacy.Exec(
		`INSERT INTO users (id, created_at, updated_at, email) VALUES (?, ?, ?, ?)`,
		"legacy-user", now, now, "legacy@example.com",
	); err != nil {
		t.Fatalf("insert legacy user: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	db, err := Open(Config{Path: path})
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	userStore, _ := NewStores(db)
	user, err := userStore.GetByID(context.Background(), "legacy-user")
	if err != nil {
		t.Fatalf("read legacy user: %v", err)
	}
	if user.Email != "legacy@example.com" {
		t.Fatalf("legacy user email = %q", user.Email)
	}
}

func TestConfigValidateRejectsEmptyPath(t *testing.T) {
	if err := (Config{}).Validate(); err == nil {
		t.Fatal("expected empty sqlite path to fail validation")
	}
}
