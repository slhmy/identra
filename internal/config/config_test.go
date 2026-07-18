package config

import (
	"strings"
	"testing"
	"time"

	"github.com/slhmy/identra/internal/cache/redis"
	"github.com/slhmy/identra/internal/mail/smtp"
	"github.com/slhmy/identra/internal/store/sqlite"
)

func TestGRPCConfigValidateAcceptsLocalDefaults(t *testing.T) {
	cfg := GRPCConfig{
		GRPCPort: 50051,
		Redis: redis.Config{
			Urls: []string{"localhost:6379"},
		},
		Persistence: PersistenceConfig{
			Type:   "sqlite",
			SQLite: sqlite.Config{Path: "data/users.db"},
		},
		Auth: AuthConfig{
			RSAPrivateKeyFile: "data/signing-key.pem",
			Token: TokenConfig{
				Issuer:                 "identra",
				AccessTokenExpiration:  15 * time.Minute,
				RefreshTokenExpiration: 7 * 24 * time.Hour,
				ServiceTokenExpiration: 15 * time.Minute,
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected local default config to be valid: %v", err)
	}
}

func TestGRPCConfigValidateReportsConfigArea(t *testing.T) {
	cfg := validGRPCConfig()
	cfg.Redis.Urls = nil

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected missing redis URL to fail")
	}
	if !strings.Contains(err.Error(), "redis config") {
		t.Fatalf("expected error to identify redis config area, got %q", err)
	}
}

func TestGRPCConfigValidateRejectsInvalidSMTP(t *testing.T) {
	cfg := validGRPCConfig()
	cfg.SmtpMailer = smtp.Config{
		Host:        "smtp.example.com",
		Port:        587,
		FromEmail:   "noreply@example.com",
		AuthEnabled: true,
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected incomplete SMTP config to fail")
	}
	if !strings.Contains(err.Error(), "smtp mailer config") {
		t.Fatalf("expected error to identify SMTP config area, got %q", err)
	}
}

func TestGRPCConfigValidateAcceptsLocalSMTPWithoutTLSOrAuth(t *testing.T) {
	cfg := validGRPCConfig()
	cfg.SmtpMailer = smtp.Config{
		Host:      "localhost",
		Port:      1025,
		FromEmail: "noreply@identra.local",
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected local SMTP config to be valid: %v", err)
	}
}

func TestPersistenceConfigValidateRejectsMongo(t *testing.T) {
	cfg := PersistenceConfig{
		Type:   "mongo",
		SQLite: sqlite.Config{Path: "data/users.db"},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected unsupported mongo persistence to fail")
	}
	if !strings.Contains(err.Error(), "unsupported persistence type") {
		t.Fatalf("expected unsupported persistence error, got %q", err)
	}
}

func validGRPCConfig() GRPCConfig {
	return GRPCConfig{
		GRPCPort: 50051,
		Redis: redis.Config{
			Urls: []string{"localhost:6379"},
		},
		Persistence: PersistenceConfig{
			Type:   "sqlite",
			SQLite: sqlite.Config{Path: "data/users.db"},
		},
		Auth: AuthConfig{
			RSAPrivateKeyFile: "data/signing-key.pem",
			Token: TokenConfig{
				Issuer:                 "identra",
				AccessTokenExpiration:  15 * time.Minute,
				RefreshTokenExpiration: 7 * 24 * time.Hour,
				ServiceTokenExpiration: 15 * time.Minute,
			},
		},
	}
}
