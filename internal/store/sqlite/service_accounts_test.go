package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

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
	authenticated, err := serviceaccount.Authenticate(ctx, store, first.ID, first.ClientSecret)
	if err != nil || authenticated.ID != first.ID {
		t.Fatalf("authenticate first account result=%+v error=%v", authenticated, err)
	}
	if _, err := serviceaccount.Authenticate(ctx, store, first.ID, "wrong-secret"); !errors.Is(err, serviceaccount.ErrInvalidCredential) {
		t.Fatalf("invalid credential error = %v", err)
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

	managed, err := serviceaccount.Create(ctx, store, "worker", []string{"identra.users.read"})
	if err != nil {
		t.Fatalf("managed create: %v", err)
	}
	accounts, err := store.List(ctx)
	if err != nil || len(accounts) != 3 {
		t.Fatalf("list accounts len=%d error=%v", len(accounts), err)
	}
	rotatedSecret, err := serviceaccount.RotateCredential(ctx, store, managed.ID)
	if err != nil {
		t.Fatalf("rotate credential: %v", err)
	}
	if _, err := serviceaccount.Authenticate(ctx, store, managed.ID, managed.ClientSecret); !errors.Is(err, serviceaccount.ErrInvalidCredential) {
		t.Fatalf("old credential error = %v", err)
	}
	if _, err := serviceaccount.Authenticate(ctx, store, managed.ID, rotatedSecret); err != nil {
		t.Fatalf("new credential authentication: %v", err)
	}
	disabled, err := store.Disable(ctx, managed.ID, time.Now().UTC())
	if err != nil || disabled.DisabledAt == nil {
		t.Fatalf("disable result=%+v error=%v", disabled, err)
	}
	if _, err := serviceaccount.Authenticate(ctx, store, managed.ID, rotatedSecret); !errors.Is(err, serviceaccount.ErrInvalidCredential) {
		t.Fatalf("disabled authentication error = %v", err)
	}
}
