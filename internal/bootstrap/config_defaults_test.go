package bootstrap

import (
	"os"
	"testing"
	"time"
)

func TestInitConfigAppliesDefaults(t *testing.T) {
	unsetConfigEnv(t,
		"GRPC_PORT",
		"LOG_LEVEL",
		"LOG_FORMAT",
		"AUTH_OAUTH_STATE_EXPIRATION",
		"AUTH_ACCESS_TOKEN_EXPIRATION",
		"AUTH_REFRESH_TOKEN_EXPIRATION",
		"AUTH_TOKEN_ISSUER",
		"REDIS_URLS",
		"SMTP_MAILER_START_TLS",
		"SMTP_MAILER_AUTH_ENABLED",
		"PERSISTENCE_TYPE",
		"PERSISTENCE_SQLITE_PATH",
	)

	if err := initConfig(t.TempDir()); err != nil {
		t.Fatalf("failed to init config: %v", err)
	}

	if got := config.GetUint("grpc_port"); got != 50051 {
		t.Fatalf("expected default grpc_port 50051, got %d", got)
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
	if !config.GetBool("smtp_mailer.start_tls") {
		t.Fatal("expected SMTP STARTTLS to be enabled by default")
	}
	if !config.GetBool("smtp_mailer.auth_enabled") {
		t.Fatal("expected SMTP authentication to be enabled by default")
	}
	if got := config.GetString("persistence.type"); got != "sqlite" {
		t.Fatalf("expected default persistence type sqlite, got %q", got)
	}
	if got := config.GetString("persistence.sqlite.path"); got != "data/users.db" {
		t.Fatalf("expected default sqlite path data/users.db, got %q", got)
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
