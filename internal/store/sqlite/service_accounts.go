package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/slhmy/identra/internal/identra"
	"github.com/slhmy/identra/internal/serviceaccount"
)

const bootstrapCompletedKey = "service_account_bootstrap_completed"

// ServiceAccountStore persists bootstrap credentials in SQLite.
type ServiceAccountStore struct {
	db *sql.DB
}

var _ serviceaccount.BootstrapStore = (*ServiceAccountStore)(nil)
var _ serviceaccount.Store = (*ServiceAccountStore)(nil)

func NewServiceAccountStore(db *sql.DB) *ServiceAccountStore {
	return &ServiceAccountStore{db: db}
}

func (s *ServiceAccountStore) Create(ctx context.Context, record serviceaccount.BootstrapRecord) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin service-account creation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertServiceAccount(ctx, tx, record); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return mapServiceAccountError(err)
	}
	return nil
}

func (s *ServiceAccountStore) Authenticate(ctx context.Context, clientID, secretHash string, now time.Time) (serviceaccount.Account, error) {
	var account serviceaccount.Account
	var scopesJSON string
	err := s.db.QueryRowContext(ctx, `
SELECT sa.id, sa.name, sa.scopes, sa.created_at
FROM service_accounts sa
JOIN service_account_credentials credential ON credential.service_account_id = sa.id
WHERE sa.id = ?
  AND sa.disabled_at IS NULL
  AND credential.secret_hash = ?
  AND credential.revoked_at IS NULL
  AND (credential.expires_at IS NULL OR credential.expires_at > ?)
LIMIT 1`, clientID, secretHash, now).Scan(&account.ID, &account.Name, &scopesJSON, &account.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return serviceaccount.Account{}, serviceaccount.ErrInvalidCredential
	}
	if err != nil {
		return serviceaccount.Account{}, fmt.Errorf("authenticate service account: %w", err)
	}
	if err := json.Unmarshal([]byte(scopesJSON), &account.Scopes); err != nil {
		return serviceaccount.Account{}, fmt.Errorf("decode service-account scopes: %w", err)
	}
	return account, nil
}

func (s *ServiceAccountStore) GetByID(ctx context.Context, clientID string) (serviceaccount.Account, error) {
	var account serviceaccount.Account
	var scopesJSON string
	var disabledAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
SELECT id, name, scopes, created_at, disabled_at
FROM service_accounts WHERE id = ?`, clientID).Scan(
		&account.ID, &account.Name, &scopesJSON, &account.CreatedAt, &disabledAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return serviceaccount.Account{}, serviceaccount.ErrNotFound
	}
	if err != nil {
		return serviceaccount.Account{}, fmt.Errorf("get service account: %w", err)
	}
	if err := json.Unmarshal([]byte(scopesJSON), &account.Scopes); err != nil {
		return serviceaccount.Account{}, fmt.Errorf("decode service-account scopes: %w", err)
	}
	if disabledAt.Valid {
		value := disabledAt.Time
		account.DisabledAt = &value
	}
	return account, nil
}

func (s *ServiceAccountStore) List(ctx context.Context) ([]serviceaccount.Account, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, scopes, created_at, disabled_at
FROM service_accounts ORDER BY created_at ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list service accounts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var accounts []serviceaccount.Account
	for rows.Next() {
		var account serviceaccount.Account
		var scopesJSON string
		var disabledAt sql.NullTime
		if err := rows.Scan(&account.ID, &account.Name, &scopesJSON, &account.CreatedAt, &disabledAt); err != nil {
			return nil, fmt.Errorf("scan service account: %w", err)
		}
		if err := json.Unmarshal([]byte(scopesJSON), &account.Scopes); err != nil {
			return nil, fmt.Errorf("decode service-account scopes: %w", err)
		}
		if disabledAt.Valid {
			value := disabledAt.Time
			account.DisabledAt = &value
		}
		accounts = append(accounts, account)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate service accounts: %w", err)
	}
	return accounts, nil
}

func (s *ServiceAccountStore) Disable(ctx context.Context, clientID string, now time.Time) (serviceaccount.Account, error) {
	result, err := s.db.ExecContext(ctx, `
UPDATE service_accounts SET disabled_at = ?, updated_at = ?
WHERE id = ? AND disabled_at IS NULL`, now, now, clientID)
	if err != nil {
		return serviceaccount.Account{}, fmt.Errorf("disable service account: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return serviceaccount.Account{}, fmt.Errorf("read disabled row count: %w", err)
	}
	if rows == 0 {
		return serviceaccount.Account{}, serviceaccount.ErrNotFound
	}
	return s.GetByID(ctx, clientID)
}

func (s *ServiceAccountStore) RotateCredential(ctx context.Context, clientID, credentialID, secretHash string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return fmt.Errorf("begin credential rotation: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var exists int
	err = tx.QueryRowContext(ctx, `SELECT 1 FROM service_accounts WHERE id = ? AND disabled_at IS NULL`, clientID).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return serviceaccount.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("find service account for rotation: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
UPDATE service_account_credentials SET revoked_at = ?
WHERE service_account_id = ? AND revoked_at IS NULL`, now, clientID); err != nil {
		return fmt.Errorf("revoke service-account credentials: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO service_account_credentials (id, service_account_id, secret_hash, created_at)
VALUES (?, ?, ?, ?)`, credentialID, clientID, secretHash, now); err != nil {
		return mapServiceAccountError(err)
	}
	return tx.Commit()
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

	if err := insertServiceAccount(ctx, tx, record); err != nil {
		return serviceaccount.Account{}, false, err
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO system_state (key, value, updated_at) VALUES (?, ?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, bootstrapCompletedKey, record.Account.ID, record.Account.CreatedAt)
	if err != nil {
		return serviceaccount.Account{}, false, fmt.Errorf("write bootstrap state: %w", err)
	}
	metadata, err := json.Marshal(map[string]string{"name": record.Account.Name})
	if err != nil {
		return serviceaccount.Account{}, false, fmt.Errorf("encode bootstrap audit metadata: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO audit_events
  (id, occurred_at, actor_type, actor_id, action, resource_type, resource_id, metadata)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, uuid.NewString(), record.Account.CreatedAt, identra.AuditActorSystem, "bootstrap", "service_account.bootstrap", "service_account", record.Account.ID, string(metadata))
	if err != nil {
		return serviceaccount.Account{}, false, fmt.Errorf("record bootstrap audit event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return serviceaccount.Account{}, false, mapServiceAccountError(err)
	}
	return record.Account, true, nil
}

func insertServiceAccount(ctx context.Context, tx *sql.Tx, record serviceaccount.BootstrapRecord) error {
	scopesJSON, err := json.Marshal(record.Account.Scopes)
	if err != nil {
		return fmt.Errorf("encode service-account scopes: %w", err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO service_accounts (id, name, scopes, created_at, updated_at)
VALUES (?, ?, ?, ?, ?)`, record.Account.ID, record.Account.Name, string(scopesJSON), record.Account.CreatedAt, record.Account.CreatedAt)
	if err != nil {
		return mapServiceAccountError(err)
	}
	_, err = tx.ExecContext(ctx, `
INSERT INTO service_account_credentials (id, service_account_id, secret_hash, created_at)
VALUES (?, ?, ?, ?)`, record.CredentialID, record.Account.ID, record.SecretHash, record.Account.CreatedAt)
	return mapServiceAccountError(err)
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
	if err == nil {
		return nil
	}
	if isUniqueConstraintError(err) {
		return serviceaccount.ErrAlreadyExists
	}
	return err
}
