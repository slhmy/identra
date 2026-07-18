// Package serviceaccount contains service-account credential lifecycle logic.
package serviceaccount

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	clientIDPrefix     = "isa_"
	credentialIDPrefix = "iac_"
	clientSecretPrefix = "is_"
)

var (
	ErrAlreadyExists      = errors.New("service account already exists")
	ErrBootstrapCompleted = errors.New("service-account bootstrap has already completed")
	ErrNotFound           = errors.New("service account not found")
	ErrInvalidCredential  = errors.New("invalid service-account credential")
	scopePattern          = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:*/-]{0,127}$`)
)

// Account is the persisted public portion of a service account.
type Account struct {
	ID         string     `json:"client_id"`
	Name       string     `json:"name"`
	Scopes     []string   `json:"scopes"`
	CreatedAt  time.Time  `json:"created_at"`
	DisabledAt *time.Time `json:"disabled_at,omitempty"`
}

// BootstrapRecord contains all values needed for an atomic bootstrap write.
// SecretHash is safe to persist; the plaintext secret is never passed to storage.
type BootstrapRecord struct {
	Account      Account
	CredentialID string
	SecretHash   string
}

// BootstrapStore atomically creates the first privileged service account.
type BootstrapStore interface {
	Bootstrap(context.Context, BootstrapRecord, bool, bool) (Account, bool, error)
}

// Store persists and authenticates service accounts after initial bootstrap.
type Store interface {
	BootstrapStore
	Create(context.Context, BootstrapRecord) error
	Authenticate(context.Context, string, string, time.Time) (Account, error)
	GetByID(context.Context, string) (Account, error)
	List(context.Context) ([]Account, error)
	Disable(context.Context, string, time.Time) (Account, error)
	RotateCredential(context.Context, string, string, string, time.Time) error
}

// BootstrapRequest controls first-service-account creation.
type BootstrapRequest struct {
	Name        string
	Scopes      []string
	IfNotExists bool
	Force       bool
}

// BootstrapResult contains the one-time credential returned to the operator.
type BootstrapResult struct {
	Account
	ClientSecret string `json:"client_secret,omitempty"`
	Created      bool   `json:"created"`
}

// Bootstrap creates a service account and a single credential atomically.
func Bootstrap(ctx context.Context, store BootstrapStore, req BootstrapRequest) (BootstrapResult, error) {
	record, clientSecret, err := newRecord(req.Name, req.Scopes)
	if err != nil {
		return BootstrapResult{}, err
	}
	account, created, err := store.Bootstrap(ctx, record, req.IfNotExists, req.Force)
	if err != nil {
		return BootstrapResult{}, err
	}
	result := BootstrapResult{Account: account, Created: created}
	if created {
		result.ClientSecret = clientSecret
	}
	return result, nil
}

// Create adds a service account and returns its one-time credential.
func Create(ctx context.Context, store Store, name string, scopes []string) (BootstrapResult, error) {
	record, clientSecret, err := newRecord(name, scopes)
	if err != nil {
		return BootstrapResult{}, err
	}
	if err := store.Create(ctx, record); err != nil {
		return BootstrapResult{}, err
	}
	return BootstrapResult{Account: record.Account, ClientSecret: clientSecret, Created: true}, nil
}

// Authenticate verifies a client ID and secret against an active credential.
func Authenticate(ctx context.Context, store Store, clientID, clientSecret string) (Account, error) {
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)
	if clientID == "" || clientSecret == "" {
		return Account{}, ErrInvalidCredential
	}
	return store.Authenticate(ctx, clientID, secretHash(clientSecret), time.Now().UTC())
}

// RotateCredential revokes existing credentials and creates a new one.
func RotateCredential(ctx context.Context, store Store, clientID string) (string, error) {
	credentialID, err := randomValue(credentialIDPrefix, 18)
	if err != nil {
		return "", fmt.Errorf("generate credential ID: %w", err)
	}
	secret, err := randomValue(clientSecretPrefix, 32)
	if err != nil {
		return "", fmt.Errorf("generate client secret: %w", err)
	}
	if err := store.RotateCredential(ctx, strings.TrimSpace(clientID), credentialID, secretHash(secret), time.Now().UTC()); err != nil {
		return "", err
	}
	return secret, nil
}

func newRecord(name string, scopes []string) (BootstrapRecord, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return BootstrapRecord{}, "", errors.New("service account name is required")
	}
	if len(name) > 128 {
		return BootstrapRecord{}, "", errors.New("service account name must not exceed 128 characters")
	}
	normalizedScopes, err := normalizeScopes(scopes)
	if err != nil {
		return BootstrapRecord{}, "", err
	}
	clientID, err := randomValue(clientIDPrefix, 18)
	if err != nil {
		return BootstrapRecord{}, "", fmt.Errorf("generate client ID: %w", err)
	}
	credentialID, err := randomValue(credentialIDPrefix, 18)
	if err != nil {
		return BootstrapRecord{}, "", fmt.Errorf("generate credential ID: %w", err)
	}
	clientSecret, err := randomValue(clientSecretPrefix, 32)
	if err != nil {
		return BootstrapRecord{}, "", fmt.Errorf("generate client secret: %w", err)
	}
	now := time.Now().UTC()
	return BootstrapRecord{
		Account:      Account{ID: clientID, Name: name, Scopes: normalizedScopes, CreatedAt: now},
		CredentialID: credentialID,
		SecretHash:   secretHash(clientSecret),
	}, clientSecret, nil
}

func secretHash(secret string) string {
	digest := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func normalizeScopes(values []string) ([]string, error) {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		scope := strings.TrimSpace(value)
		if !scopePattern.MatchString(scope) {
			return nil, fmt.Errorf("invalid scope %q", value)
		}
		unique[scope] = struct{}{}
	}
	if len(unique) == 0 {
		return nil, errors.New("at least one scope is required")
	}
	scopes := make([]string, 0, len(unique))
	for scope := range unique {
		scopes = append(scopes, scope)
	}
	sort.Strings(scopes)
	return scopes, nil
}

func randomValue(prefix string, size int) (string, error) {
	value := make([]byte, size)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value), nil
}
