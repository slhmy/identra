## Agent Integration Guide (Identra)

This repository provides **Identra**, an authentication + user management service. For “agents” (CLI tools, bots, backend services, web/mobile apps), Identra’s role is to **issue and refresh JWTs** and to provide **login methods** (GitHub OAuth, email code, email+password).

### Architecture at a glance

- **`identra-grpc`**: gRPC server implementing `identra.v1.IdentraService` (business logic, persistence, token signing).
- **`identra-gateway`**: HTTP server using grpc-gateway to expose REST/JSON endpoints (and optionally serve a frontend SPA).
- **JWT + JWKS**: Tokens are **RS256-signed** and the public key is exposed as **JWKS** for verification by other services.

## HTTP API (what an agent calls)

The service interface is defined in `proto/identra/v1/identra_service.proto` and exported as OpenAPI at `gen/openapi/identra.swagger.json`.

The gateway mounts the API under an **`/api/`** prefix, so routes below are typically reachable as:

- `GET /api/.well-known/jwks.json`
- `POST /api/password/login`
- etc.

### Endpoints

- **JWKS**
  - `GET /.well-known/jwks.json`: fetch public keys for verifying JWTs.

- **OAuth (GitHub)**
  - `GET /oauth/url?provider=github&redirect_url=...`: returns `{ url, state }` for starting OAuth.
  - `POST /oauth/login`: exchange `{ code, state }` for a `TokenPair`.
  - `POST /oauth/bind`: bind GitHub identity to an existing user with `Authorization: Bearer ...` and `{ code, state }`.

- **Email verification code**
  - `POST /email/code`: send a login code to `{ email, use_html }`.
  - `POST /email/login`: exchange `{ email, code }` for a `TokenPair`.

- **Email + password**
  - `POST /password/register`: create a password account with `{ email, password }`.
  - `POST /password/login`: exchange `{ email, password }` for a `TokenPair`.

- **Tokens**
  - `POST /token/refresh`: exchange `{ refresh_token }` for a new `TokenPair`.

- **Session introspection**
  - `POST /me/login-info`: returns linked login methods for the bearer token (email, GitHub link status, etc.).

## Tokens & verification (what an agent should know)

### Token model

`TokenPair` contains:

- **access_token**: short-lived JWT used to authenticate API calls (default 15 minutes).
- **refresh_token**: long-lived JWT used only to refresh tokens (default 7 days).
- **token_type**: `"Bearer"`.

### Signing and JWKS

- Tokens are signed using **RS256** and include a `kid` header.
- Retrieve keys from `GET /.well-known/jwks.json` and select the matching `kid`.

### Claims

Identra uses standard registered claims plus a few custom ones:

- **Registered**: `iss`, `sub` (user id), `exp`, `iat`, `nbf`, `jti`
- **Custom**
  - `uid`: user id (duplicates `sub`)
  - `typ`: `"access"` or `"refresh"`

### How access tokens are used

Identra’s authenticated endpoints (`/oauth/bind`, `/me/login-info`) accept the access token via the standard HTTP header:

- `Authorization: Bearer <access_token>`

The older JSON body `access_token` field is still accepted for compatibility.

## Typical agent flows

### Email-code login

1. `POST /email/code` with `{ "email": "...", "use_html": true }`
2. User receives a 6-digit code (stored in Redis with TTL).
3. `POST /email/login` with `{ "email": "...", "code": "123456" }` → `TokenPair`

### Password registration and login

1. `POST /password/register` with `{ "email": "...", "password": "..." }` to create the account and receive a `TokenPair`.
2. `POST /password/login` with `{ "email": "...", "password": "..." }` for later sessions.
3. Login returns `NotFound` for unknown users and `FailedPrecondition` when an OAuth/email-code account has no password set.

### GitHub OAuth login

1. `GET /oauth/url?provider=github&redirect_url=<your callback URL>` → `{ url, state }`
2. User completes GitHub consent; your callback receives `code` (and you already have `state`).
3. `POST /oauth/login` with `{ "code": "...", "state": "..." }` → `TokenPair`

### Bind GitHub to an existing user

1. Start OAuth the same way (`/oauth/url`).
2. `POST /oauth/bind` with `Authorization: Bearer <current access token>` and `{ "code": "...", "state": "..." }`
3. Returns a refreshed `TokenPair` after linking.

### Refresh tokens

1. `POST /token/refresh` with `{ "refresh_token": "<refresh token>" }` → new `TokenPair`

## Local development (service-side)

### Protobuf / OpenAPI generation

See `CONTRIBUTING.md`:

- `buf dep update`
- `buf generate --clean`

### Runtime dependencies

- **Redis** is required for email-code login (verification codes are stored in Redis).
- **SMTP** is optional (email sending is disabled when `smtp_mailer.host` is empty).
- **Persistence** defaults to **SQLite via GORM** (`data/users.db`) but MongoDB is supported.

### Configuration knobs (selected)

Config keys are defined in `internal/infrastructure/configs/keys.go`. Built-in local defaults are registered in `internal/infrastructure/bootstrap/config_defaults.go`; the root `config.toml` is for overrides:

- **Ports**
  - `grpc_port`
  - `http_port`
  - `grpc_endpoint` (gateway upstream target)
- **CORS**
  - `cors.allowed_origins`
  - `cors.allow_credentials`
- **Auth**
  - `auth.rsa_private_key` (optional; if empty Identra generates a key pair at startup)
  - `auth.oauth_state_expiration`
  - `auth.access_token_expiration`, `auth.refresh_token_expiration`
  - `auth.token_issuer`
  - `auth.github.client_id`, `auth.github.client_secret`
- **Redis**
  - `redis.urls`, `redis.password`
- **Persistence**
  - `persistence.type` (`gorm` or `mongo`)
  - `persistence.gorm.*`, `persistence.mongo.*`
- **SMTP**
  - `smtp_mailer.*`

### OAuth state storage

OAuth `state` is stored in Redis via `internal/infrastructure/cache/redis_oauth_state_store.go`, so multi-instance deployments share OAuth state as long as all replicas use the same Redis backend.
