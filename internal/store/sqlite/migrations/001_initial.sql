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
