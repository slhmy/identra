CREATE TABLE IF NOT EXISTS service_accounts (
    id TEXT PRIMARY KEY NOT NULL,
    name TEXT NOT NULL UNIQUE,
    scopes TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    disabled_at DATETIME
);

CREATE TABLE IF NOT EXISTS service_account_credentials (
    id TEXT PRIMARY KEY NOT NULL,
    service_account_id TEXT NOT NULL,
    secret_hash TEXT NOT NULL UNIQUE,
    created_at DATETIME NOT NULL,
    expires_at DATETIME,
    revoked_at DATETIME,
    FOREIGN KEY (service_account_id) REFERENCES service_accounts (id)
);

CREATE INDEX IF NOT EXISTS idx_service_account_credentials_account_id
    ON service_account_credentials (service_account_id);

CREATE TABLE IF NOT EXISTS system_state (
    key TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL,
    updated_at DATETIME NOT NULL
);
