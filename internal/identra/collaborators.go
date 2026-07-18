package identra

import (
	"context"
	"time"
)

type EmailCodeStore interface {
	Set(ctx context.Context, email, code string) error
	Consume(ctx context.Context, email, code string) (bool, error)
}

type OAuthState struct {
	Provider    string
	RedirectURL string
}

type OAuthStateStore interface {
	Add(ctx context.Context, state, provider, redirectURL string) error
	Consume(ctx context.Context, state string) (OAuthState, bool, error)
}

type EmailMessage struct {
	ToEmails []string
	Subject  string
	Body     string
	IsHTML   bool
}

type EmailSender interface {
	SendEmail(message EmailMessage) error
}

type RateLimiter interface {
	IsAllowed(ctx context.Context, key string) (bool, error)
	Record(ctx context.Context, key string) error
	Reset(ctx context.Context, key string) error
}

type RefreshTokenRevocationStore interface {
	Revoke(ctx context.Context, tokenID string, expiresAt time.Time) error
	IsRevoked(ctx context.Context, tokenID string) (bool, error)
}

type AuditEvent struct {
	ID           string
	OccurredAt   time.Time
	ActorType    string
	ActorID      string
	Action       string
	ResourceType string
	ResourceID   string
	Metadata     map[string]string
}

type AuditStore interface {
	Record(ctx context.Context, event AuditEvent) error
	List(ctx context.Context, offset, limit int) ([]AuditEvent, error)
}
