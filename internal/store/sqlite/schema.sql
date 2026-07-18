CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    deleted_at DATETIME,
    email TEXT NOT NULL UNIQUE,
    hashed_password TEXT,
    hash TEXT,
    last_login_at DATETIME
);

CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users (deleted_at);

CREATE TABLE IF NOT EXISTS external_identities (
    id TEXT PRIMARY KEY NOT NULL,
    user_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    deleted_at DATETIME,
    CONSTRAINT idx_external_identities_provider_provider_user_id
        UNIQUE (provider, provider_user_id),
    FOREIGN KEY (user_id) REFERENCES users (id)
);

CREATE INDEX IF NOT EXISTS idx_external_identities_user_id
    ON external_identities (user_id);
CREATE INDEX IF NOT EXISTS idx_external_identities_deleted_at
    ON external_identities (deleted_at);

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

CREATE TABLE IF NOT EXISTS audit_events (
    id TEXT PRIMARY KEY NOT NULL,
    occurred_at DATETIME NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL,
    metadata TEXT NOT NULL DEFAULT '{}'
);

CREATE INDEX IF NOT EXISTS idx_audit_events_occurred_at
    ON audit_events (occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_events_actor
    ON audit_events (actor_type, actor_id);
