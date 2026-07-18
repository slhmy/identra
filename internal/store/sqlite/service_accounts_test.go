package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/slhmy/identra/internal/serviceaccount"
)

func TestServiceAccountBootstrapLifecycle(t *testing.T) {
	db, err := Open(Config{Path: filepath.Join(t.TempDir(), "bootstrap.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewServiceAccountStore(db)
	ctx := context.Background()

	first, err := serviceaccount.Bootstrap(ctx, store, serviceaccount.BootstrapRequest{
		Name: "platform-admin", Scopes: []string{"identra.admin"},
	})
	if err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}
	if !first.Created || first.ClientSecret == "" {
		t.Fatalf("unexpected first result: %+v", first)
	}

	var storedHash string
	if err := db.QueryRow(`SELECT secret_hash FROM service_account_credentials WHERE service_account_id = ?`, first.ID).Scan(&storedHash); err != nil {
		t.Fatalf("read stored hash: %v", err)
	}
	if storedHash == first.ClientSecret {
		t.Fatal("database contains plaintext client secret")
	}

	existing, err := serviceaccount.Bootstrap(ctx, store, serviceaccount.BootstrapRequest{
		Name: "platform-admin", Scopes: []string{"identra.admin"}, IfNotExists: true,
	})
	if err != nil {
		t.Fatalf("idempotent bootstrap: %v", err)
	}
	if existing.Created || existing.ClientSecret != "" || existing.ID != first.ID {
		t.Fatalf("unexpected existing result: %+v", existing)
	}

	_, err = serviceaccount.Bootstrap(ctx, store, serviceaccount.BootstrapRequest{
		Name: "second-admin", Scopes: []string{"identra.admin"},
	})
	if !errors.Is(err, serviceaccount.ErrBootstrapCompleted) {
		t.Fatalf("second bootstrap error = %v", err)
	}

	second, err := serviceaccount.Bootstrap(ctx, store, serviceaccount.BootstrapRequest{
		Name: "recovery-admin", Scopes: []string{"identra.admin"}, Force: true,
	})
	if err != nil || !second.Created {
		t.Fatalf("forced bootstrap result=%+v error=%v", second, err)
	}
}
