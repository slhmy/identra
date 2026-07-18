package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/slhmy/identra/internal/serviceaccount"
)

const bootstrapCompletedKey = "service_account_bootstrap_completed"

// ServiceAccountStore persists bootstrap credentials in SQLite.
type ServiceAccountStore struct {
	db *sql.DB
}

var _ serviceaccount.BootstrapStore = (*ServiceAccountStore)(nil)

func NewServiceAccountStore(db *sql.DB) *ServiceAccountStore {
	return &ServiceAccountStore{db: db}
}

func (s *ServiceAccountStore) Bootstrap(ctx context.Context, record serviceaccount.BootstrapRecord, ifNotExists, force bool) (serviceaccount.Account, bool, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return serviceaccount.Account{}, false, fmt.Errorf("begin service-account bootstrap: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	existing, found, err := findServiceAccountByName(ctx, tx, record.Account.Name)
	if err != nil {
		return serviceaccount.Account{}, false, err
	}
	if found {
		if ifNotExists {
			return existing, false, nil
		}
		return serviceaccount.Account{}, false, serviceaccount.ErrAlreadyExists
	}

	var marker string
	err = tx.QueryRowContext(ctx, `SELECT value FROM system_state WHERE key = ?`, bootstrapCompletedKey).Scan(&marker)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return serviceaccount.Account{}, false, fmt.Errorf("read bootstrap state: %w", err)
	}
	if err == nil && !force {
		return serviceaccount.Account{}, false, serviceaccount.ErrBootstrapCompleted
	}

	scopesJSON, err := json.Marshal(record.Account.Scopes)
	if err != nil {
		return serviceaccount.Account{}, false, fmt.Errorf("encode service-account scopes: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO service_accounts (id, name, scopes, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)`, record.Account.ID, record.Account.Name, string(scopesJSON), record.Account.CreatedAt, record.Account.CreatedAt)
	if err != nil {
		return serviceaccount.Account{}, false, mapServiceAccountError(err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO service_account_credentials (id, service_account_id, secret_hash, created_at)
VALUES (?, ?, ?, ?)`, record.CredentialID, record.Account.ID, record.SecretHash, record.Account.CreatedAt)
	if err != nil {
		return serviceaccount.Account{}, false, mapServiceAccountError(err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO system_state (key, value, updated_at) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, bootstrapCompletedKey, record.Account.ID, record.Account.CreatedAt)
	if err != nil {
		return serviceaccount.Account{}, false, fmt.Errorf("write bootstrap state: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return serviceaccount.Account{}, false, mapServiceAccountError(err)
	}
	return record.Account, true, nil
}

func findServiceAccountByName(ctx context.Context, tx *sql.Tx, name string) (serviceaccount.Account, bool, error) {
	var account serviceaccount.Account
	var scopesJSON string
	err := tx.QueryRowContext(ctx, `
SELECT id, name, scopes, created_at
FROM service_accounts
WHERE name = ?`, name).Scan(&account.ID, &account.Name, &scopesJSON, &account.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return serviceaccount.Account{}, false, nil
	}
	if err != nil {
		return serviceaccount.Account{}, false, fmt.Errorf("find service account: %w", err)
	}
	if err := json.Unmarshal([]byte(scopesJSON), &account.Scopes); err != nil {
		return serviceaccount.Account{}, false, fmt.Errorf("decode service-account scopes: %w", err)
	}
	return account, true, nil
}

func mapServiceAccountError(err error) error {
	if isUniqueConstraintError(err) {
		return serviceaccount.ErrAlreadyExists
	}
	return err
}
