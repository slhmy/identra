package config

import (
	"strings"
	"testing"
	"time"

	"github.com/slhmy/identra/internal/cache/redis"
	"github.com/slhmy/identra/internal/mail/smtp"
	"github.com/slhmy/identra/internal/store/gorm"
	"github.com/slhmy/identra/internal/store/mongo"
)

func TestGRPCConfigValidateAcceptsLocalDefaults(t *testing.T) {
	cfg := GRPCConfig{
		GRPCPort: 50051,
		Redis: redis.Config{
			Urls: []string{"localhost:6379"},
		},
		Persistence: PersistenceConfig{
			Type: "gorm",
			GORM: &gorm.Config{
				Driver: "sqlite",
				DbName: "data/users.db",
			},
		},
		Auth: AuthConfig{
			Token: TokenConfig{
				Issuer:                 "identra",
				AccessTokenExpiration:  15 * time.Minute,
				RefreshTokenExpiration: 7 * 24 * time.Hour,
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

func TestPersistenceConfigValidateRejectsInvalidMongo(t *testing.T) {
	cfg := PersistenceConfig{
		Type:  "mongo",
		Mongo: &mongo.Config{URI: "mongodb://localhost:27017"},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected missing mongo database to fail")
	}
	if !strings.Contains(err.Error(), "mongo database") {
		t.Fatalf("expected mongo database error, got %q", err)
	}
}

func TestGatewayConfigValidateRejectsWildcardCORSWithCredentials(t *testing.T) {
	cfg := GatewayConfig{
		HTTPPort:     8080,
		GRPCEndpoint: "localhost:50051",
		CORS: CORSConfig{
			AllowedOrigins:   []string{"*"},
			AllowCredentials: true,
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected wildcard CORS with credentials to fail")
	}
	if !strings.Contains(err.Error(), "cors config") {
		t.Fatalf("expected error to identify CORS config area, got %q", err)
	}
}

func validGRPCConfig() GRPCConfig {
	return GRPCConfig{
		GRPCPort: 50051,
		Redis: redis.Config{
			Urls: []string{"localhost:6379"},
		},
		Persistence: PersistenceConfig{
			Type: "gorm",
			GORM: &gorm.Config{
				Driver: "sqlite",
				DbName: "data/users.db",
			},
		},
		Auth: AuthConfig{
			Token: TokenConfig{
				Issuer:                 "identra",
				AccessTokenExpiration:  15 * time.Minute,
				RefreshTokenExpiration: 7 * 24 * time.Hour,
			},
		},
	}
}
