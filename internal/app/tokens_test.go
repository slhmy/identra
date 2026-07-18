package app

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/slhmy/identra/internal/config"
	"github.com/slhmy/identra/internal/security"
)

func TestBuildTokenKeysPersistsGeneratedKeyAcrossRestart(t *testing.T) {
	keyFile := filepath.Join(t.TempDir(), "keys", "signing-key.pem")
	authConfig := config.AuthConfig{
		RSAPrivateKeyFile: keyFile,
		Token: config.TokenConfig{
			Issuer: "identra-test", AccessTokenExpiration: time.Minute,
			RefreshTokenExpiration: time.Hour, ServiceTokenExpiration: time.Minute,
		},
	}
	first, err := buildTokenKeys(authConfig)
	if err != nil {
		t.Fatalf("first startup: %v", err)
	}
	token, err := security.NewServiceToken("service-1", []string{"identra.admin"}, first.tokenConfig)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	second, err := buildTokenKeys(authConfig)
	if err != nil {
		t.Fatalf("second startup: %v", err)
	}
	if first.manager.GetKeyID() != second.manager.GetKeyID() {
		t.Fatalf("key ID changed across restart: %q != %q", first.manager.GetKeyID(), second.manager.GetKeyID())
	}
	if _, err := security.ValidateServiceToken(token.GetValue(), second.tokenConfig.PublicKey); err != nil {
		t.Fatalf("validate pre-restart token after restart: %v", err)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(keyFile)
		if err != nil {
			t.Fatalf("stat key file: %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("key permissions = %o, want 600", info.Mode().Perm())
		}
	}
}
