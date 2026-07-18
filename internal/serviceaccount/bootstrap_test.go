package serviceaccount

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

type captureStore struct {
	record  BootstrapRecord
	account Account
	created bool
	err     error
}

func (s *captureStore) Bootstrap(_ context.Context, record BootstrapRecord, _, _ bool) (Account, bool, error) {
	s.record = record
	if s.account.ID == "" {
		s.account = record.Account
	}
	return s.account, s.created, s.err
}

func TestBootstrapGeneratesOneTimeCredential(t *testing.T) {
	store := &captureStore{created: true}
	result, err := Bootstrap(context.Background(), store, BootstrapRequest{
		Name:   " platform-admin ",
		Scopes: []string{"identra.users.read", "identra.admin", "identra.admin"},
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if !result.Created || !strings.HasPrefix(result.ID, clientIDPrefix) || !strings.HasPrefix(result.ClientSecret, clientSecretPrefix) {
		t.Fatalf("unexpected result: %+v", result)
	}
	if got := strings.Join(result.Scopes, ","); got != "identra.admin,identra.users.read" {
		t.Fatalf("scopes = %q", got)
	}
	if strings.Contains(store.record.SecretHash, result.ClientSecret) {
		t.Fatal("persisted hash contains plaintext secret")
	}
	digest := sha256.Sum256([]byte(result.ClientSecret))
	if want := base64.RawURLEncoding.EncodeToString(digest[:]); store.record.SecretHash != want {
		t.Fatalf("secret hash = %q, want %q", store.record.SecretHash, want)
	}
}

func TestBootstrapDoesNotReturnSecretForExistingAccount(t *testing.T) {
	store := &captureStore{account: Account{ID: "isa_existing", Name: "admin", Scopes: []string{"identra.admin"}}}
	result, err := Bootstrap(context.Background(), store, BootstrapRequest{
		Name:        "admin",
		Scopes:      []string{"identra.admin"},
		IfNotExists: true,
	})
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if result.Created || result.ClientSecret != "" || result.ID != "isa_existing" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestBootstrapUsesConfiguredCredential(t *testing.T) {
	store := &captureStore{created: true}
	secret := "configured-secret-with-at-least-thirty-two-characters"
	result, err := Bootstrap(context.Background(), store, BootstrapRequest{
		Name: "railway-admin", Scopes: []string{"identra.admin"},
		ClientID: "isa_railway_admin", ClientSecret: secret,
	})
	if err != nil {
		t.Fatalf("bootstrap configured credential: %v", err)
	}
	if result.ID != "isa_railway_admin" || result.ClientSecret != secret {
		t.Fatalf("unexpected configured credential result: %+v", result)
	}
	if store.record.SecretHash == secret {
		t.Fatal("configured secret was stored as plaintext")
	}
}

func TestBootstrapValidatesInput(t *testing.T) {
	tests := []BootstrapRequest{
		{Scopes: []string{"identra.admin"}},
		{Name: "admin"},
		{Name: "admin", Scopes: []string{"invalid scope"}},
		{Name: "admin", Scopes: []string{"identra.admin"}, ClientID: "isa_admin"},
		{Name: "admin", Scopes: []string{"identra.admin"}, ClientID: "isa_admin", ClientSecret: "short"},
	}
	for _, req := range tests {
		if _, err := Bootstrap(context.Background(), &captureStore{}, req); err == nil {
			t.Fatalf("expected validation error for %+v", req)
		}
	}
}
