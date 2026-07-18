# Identra

Identra is a gRPC authentication and user-management service. It supports GitHub OAuth, email codes,
password authentication, JWT session rotation, account linking, and typed signing-key discovery.

## gRPC API

The public API is defined in `proto/identra/v1` and split by responsibility:

- `identra.v1.AuthService`: registration and login flows
- `identra.v1.SessionService`: refresh and revoke sessions
- `identra.v1.UserService`: current-user data and OAuth account linking
- `identra.v1.KeyService`: public signing keys for JWT verification

Generated Go clients are committed under `gen/go/identra/v1`. Server reflection and the standard gRPC
health service are enabled.

Authenticated `UserService` calls require this gRPC metadata:

```text
authorization: Bearer <access-token>
```

Tokens are RS256 JWTs. `TokenPair` contains short-lived access and long-lived refresh tokens; each token
has a string `value` and a protobuf `Timestamp` expiration. `KeyService.ListSigningKeys` returns the
active and passive RSA verification keys as typed modulus/exponent values.

## Quick start

Start Redis, Mailpit, and the gRPC service:

```sh
make dev
```

The gRPC server listens on `localhost:50051`; Mailpit is available at
`http://localhost:8025` for inspecting locally captured email.

For host-side development:

```sh
make dev-infra
make run-grpc
```

Use reflection to inspect the API:

```sh
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext localhost:50051 describe identra.v1.AuthService
```

Register with a password:

```sh
grpcurl -plaintext \
  -d '{"email":"user@example.com","password":"correct-horse-battery-staple"}' \
  localhost:50051 identra.v1.AuthService/RegisterWithPassword
```

Refresh a session:

```sh
grpcurl -plaintext \
  -d '{"refresh_token":"<refresh-token>"}' \
  localhost:50051 identra.v1.SessionService/RefreshSession
```

Fetch the current user:

```sh
grpcurl -plaintext \
  -H 'authorization: Bearer <access-token>' \
  -d '{}' \
  localhost:50051 identra.v1.UserService/GetCurrentUser
```

List public signing keys:

```sh
grpcurl -plaintext -d '{}' localhost:50051 identra.v1.KeyService/ListSigningKeys
```

## Authentication flows

### Email code

1. Call `AuthService.RequestEmailLoginCode` with an email address.
2. Read the six-digit code from email.
3. Call `AuthService.LoginWithEmailCode` to receive a `TokenPair`.

### Password

- `AuthService.RegisterWithPassword` creates an account and session.
- `AuthService.LoginWithPassword` creates a session for an existing password account.

### GitHub OAuth

1. Call `AuthService.ListOAuthProviders` and ensure GitHub is enabled.
2. Call `AuthService.StartOAuthLogin` with `AUTH_PROVIDER_GITHUB` and the callback URL.
3. Complete authorization using the returned URL.
4. Exchange the callback code and state through `AuthService.LoginWithOAuth`.

To link GitHub to the current user, start the OAuth flow and call
`UserService.LinkOAuthAccount` with Bearer metadata, code, and state.

### Session lifecycle

- `SessionService.RefreshSession` rotates the refresh token and revokes the token it consumed.
- `SessionService.RevokeSession` revokes a refresh token for logout.

## Configuration

Defaults are registered in `internal/bootstrap/config_defaults.go`. Common settings include:

- `grpc_port`
- `auth.rsa_private_key`
- `auth.oauth_state_expiration`
- `auth.access_token_expiration`, `auth.refresh_token_expiration`, `auth.token_issuer`
- `auth.github.client_id`, `auth.github.client_secret`
- `redis.urls`, `redis.password`
- `persistence.type`, `persistence.sqlite.path`
- `smtp_mailer.*`

SQLite is the current persistence implementation. Redis stores email codes, OAuth state, rate limits,
and refresh-token revocations. If no RSA private key is configured, Identra generates one at startup.

## Development

```sh
make generate       # regenerate protobuf/gRPC and sqlc code
make test           # run unit tests
make test-integration
make verify         # vet, tests, lint, architecture, generated-file check
```

Install Buf and sqlc before generation. The committed protobuf outputs must remain in sync with the
definitions under `proto/identra/v1`.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the development workflow and [Agent.md](Agent.md) for a
consumer-oriented integration guide.
