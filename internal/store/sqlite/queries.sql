-- name: CreateUser :exec
INSERT INTO users (
    id,
    created_at,
    updated_at,
    email,
    hashed_password,
    hash,
    last_login_at
) VALUES (?, ?, ?, ?, ?, ?, ?);

-- name: GetUserByID :one
SELECT id, created_at, updated_at, deleted_at, email, hashed_password, hash, last_login_at
FROM users
WHERE id = ? AND deleted_at IS NULL
LIMIT 1;

-- name: GetUserByEmail :one
SELECT id, created_at, updated_at, deleted_at, email, hashed_password, hash, last_login_at
FROM users
WHERE email = ? AND deleted_at IS NULL
LIMIT 1;

-- name: UpdateUser :execrows
UPDATE users
SET email = ?,
    hashed_password = ?,
    hash = ?,
    last_login_at = ?,
    updated_at = ?
WHERE id = ? AND deleted_at IS NULL;

-- name: SoftDeleteUser :execrows
UPDATE users
SET deleted_at = ?, updated_at = ?
WHERE id = ? AND deleted_at IS NULL;

-- name: ListUsers :many
SELECT id, created_at, updated_at, deleted_at, email, hashed_password, hash, last_login_at
FROM users
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT sqlc.arg(page_size) OFFSET sqlc.arg(page_offset);

-- name: CountUsers :one
SELECT COUNT(*)
FROM users
WHERE deleted_at IS NULL;

-- name: CreateExternalIdentity :exec
INSERT INTO external_identities (
    id,
    user_id,
    provider,
    provider_user_id,
    created_at,
    updated_at
) VALUES (?, ?, ?, ?, ?, ?);

-- name: GetExternalIdentityByProviderID :one
SELECT id, user_id, provider, provider_user_id, created_at, updated_at, deleted_at
FROM external_identities
WHERE provider = ? AND provider_user_id = ? AND deleted_at IS NULL
LIMIT 1;

-- name: ListExternalIdentitiesByUserID :many
SELECT id, user_id, provider, provider_user_id, created_at, updated_at, deleted_at
FROM external_identities
WHERE user_id = ? AND deleted_at IS NULL
ORDER BY created_at ASC;
