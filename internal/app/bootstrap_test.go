package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/slhmy/identra/internal/config"
	"github.com/slhmy/identra/internal/serviceaccount"
	"github.com/slhmy/identra/internal/store/sqlite"
)

func TestBootstrapServiceAccountIsIdempotentWithConfiguredCredential(t *testing.T) {
	db, err := sqlite.Open(sqlite.Config{Path: filepath.Join(t.TempDir(), "railway.db")})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewServiceAccountStore(db)
	cfg := config.BootstrapConfig{
		Enabled: true, Name: "railway-admin", ClientID: "isa_railway_admin",
		ClientSecret: "railway-test-secret-with-more-than-32-characters", Scopes: []string{"identra.admin"},
	}
	if err := bootstrapServiceAccount(context.Background(), cfg, store); err != nil {
		t.Fatalf("first bootstrap: %v", err)
	}
	if err := bootstrapServiceAccount(context.Background(), cfg, store); err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
	account, err := serviceaccount.Authenticate(context.Background(), store, cfg.ClientID, cfg.ClientSecret)
	if err != nil || account.ID != cfg.ClientID {
		t.Fatalf("authenticate configured credential: account=%+v error=%v", account, err)
	}
}
