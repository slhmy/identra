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
	scopePattern          = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:*/-]{0,127}$`)
)

// Account is the persisted public portion of a service account.
type Account struct {
	ID        string    `json:"client_id"`
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	CreatedAt time.Time `json:"created_at"`
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
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return BootstrapResult{}, errors.New("service account name is required")
	}
	if len(name) > 128 {
		return BootstrapResult{}, errors.New("service account name must not exceed 128 characters")
	}

	scopes, err := normalizeScopes(req.Scopes)
	if err != nil {
		return BootstrapResult{}, err
	}
	clientID, err := randomValue(clientIDPrefix, 18)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("generate client ID: %w", err)
	}
	credentialID, err := randomValue(credentialIDPrefix, 18)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("generate credential ID: %w", err)
	}
	clientSecret, err := randomValue(clientSecretPrefix, 32)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("generate client secret: %w", err)
	}
	digest := sha256.Sum256([]byte(clientSecret))
	now := time.Now().UTC()

	account, created, err := store.Bootstrap(ctx, BootstrapRecord{
		Account: Account{
			ID:        clientID,
			Name:      name,
			Scopes:    scopes,
			CreatedAt: now,
		},
		CredentialID: credentialID,
		SecretHash:   base64.RawURLEncoding.EncodeToString(digest[:]),
	}, req.IfNotExists, req.Force)
	if err != nil {
		return BootstrapResult{}, err
	}
	result := BootstrapResult{Account: account, Created: created}
	if created {
		result.ClientSecret = clientSecret
	}
	return result, nil
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
