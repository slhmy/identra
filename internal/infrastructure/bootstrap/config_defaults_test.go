package bootstrap

import (
	"os"
	"testing"
	"time"
)

func TestInitConfigAppliesDefaults(t *testing.T) {
	unsetConfigEnv(t,
		"GRPC_PORT",
		"HTTP_PORT",
		"GRPC_ENDPOINT",
		"CORS_ALLOWED_ORIGINS",
		"CORS_ALLOW_CREDENTIALS",
		"LOG_LEVEL",
		"LOG_FORMAT",
		"AUTH_OAUTH_STATE_EXPIRATION",
		"AUTH_ACCESS_TOKEN_EXPIRATION",
		"AUTH_REFRESH_TOKEN_EXPIRATION",
		"AUTH_TOKEN_ISSUER",
		"REDIS_URLS",
		"PERSISTENCE_TYPE",
		"PERSISTENCE_GORM_DRIVER",
		"PERSISTENCE_GORM_DBNAME",
	)

	initConfig(t.TempDir())

	if got := config.GetUint("grpc_port"); got != 50051 {
		t.Fatalf("expected default grpc_port 50051, got %d", got)
	}
	if got := config.GetUint("http_port"); got != 8080 {
		t.Fatalf("expected default http_port 8080, got %d", got)
	}
	if got := config.GetString("grpc_endpoint"); got != "localhost:50051" {
		t.Fatalf("expected default grpc_endpoint localhost:50051, got %q", got)
	}
	if got := config.GetStringSlice("cors.allowed_origins"); len(got) != 3 || got[0] != "http://localhost:3000" {
		t.Fatalf("expected default local cors origins, got %#v", got)
	}
	if got := config.GetBool("cors.allow_credentials"); !got {
		t.Fatal("expected cors credentials to be allowed by default")
	}
	if got := config.GetString(configKeyLogLevel); got != "info" {
		t.Fatalf("expected default log level info, got %q", got)
	}
	if got := config.GetString(configKeyLogFormat); got != logFormatTint {
		t.Fatalf("expected default log format %q, got %q", logFormatTint, got)
	}
	if got := config.GetDuration("auth.oauth_state_expiration"); got != 10*time.Minute {
		t.Fatalf("expected default oauth state expiration 10m, got %s", got)
	}
	if got := config.GetDuration("auth.access_token_expiration"); got != 15*time.Minute {
		t.Fatalf("expected default access token expiration 15m, got %s", got)
	}
	if got := config.GetDuration("auth.refresh_token_expiration"); got != 7*24*time.Hour {
		t.Fatalf("expected default refresh token expiration 168h, got %s", got)
	}
	if got := config.GetString("auth.token_issuer"); got != "identra" {
		t.Fatalf("expected default token issuer identra, got %q", got)
	}
	if got := config.GetStringSlice("redis.urls"); len(got) != 1 || got[0] != "localhost:6379" {
		t.Fatalf("expected default redis urls [localhost:6379], got %#v", got)
	}
	if got := config.GetString("persistence.type"); got != "gorm" {
		t.Fatalf("expected default persistence type gorm, got %q", got)
	}
	if got := config.GetString("persistence.gorm.driver"); got != "sqlite" {
		t.Fatalf("expected default gorm driver sqlite, got %q", got)
	}
	if got := config.GetString("persistence.gorm.dbname"); got != "data/users.db" {
		t.Fatalf("expected default gorm dbname data/users.db, got %q", got)
	}
}

func unsetConfigEnv(t *testing.T, names ...string) {
	t.Helper()

	previous := make(map[string]string, len(names))
	present := make(map[string]bool, len(names))
	for _, name := range names {
		value, ok := os.LookupEnv(name)
		previous[name] = value
		present[name] = ok
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("failed to unset %s: %v", name, err)
		}
	}

	t.Cleanup(func() {
		for _, name := range names {
			if present[name] {
				_ = os.Setenv(name, previous[name])
			} else {
				_ = os.Unsetenv(name)
			}
		}
	})
}
