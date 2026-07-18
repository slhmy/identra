# Identra

Identra is a gRPC authentication and user-management service. It supports GitHub OAuth, email codes,
password authentication, JWT session rotation, account linking, and typed signing-key discovery.

## gRPC API

The public API is defined in `proto/identra/v1` and split by responsibility:

- `identra.v1.AuthService`: registration and login flows
- `identra.v1.SessionService`: refresh and revoke sessions
- `identra.v1.UserService`: current-user data and OAuth account linking
- `identra.v1.KeyService`: public signing keys for JWT verification
- `identra.v1.ServiceAccountService`: service-token exchange and scoped machine-identity management
- `identra.v1.AuditService`: paginated management audit events
- `identra.v1.SystemService`: public build, schema, and capability discovery

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
make run
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

## CLI and first service account

Identra ships as one `identra` executable and one container image. The image
defaults to `identra serve`; operational commands reuse the same configuration
and data volume.

Create the first privileged service account before starting the service:

```sh
docker compose run --rm --no-deps identra \
  bootstrap service-account \
  --name platform-admin \
  --scope identra.admin \
  --output json

docker compose up -d
```

For a host-side binary, run the equivalent command directly:

```sh
identra bootstrap service-account \
  --name platform-admin \
  --scope identra.admin
```

The generated `client_secret` is shown exactly once; only its hash is stored.
Bootstrap is blocked after the first account is created. `--if-not-exists`
makes deployment scripts idempotent without generating another secret, while
`--force` is reserved for an operator with direct database access performing
recovery. Bootstrap is an offline database operation and is not exposed as an
unauthenticated RPC.

Exchange the bootstrap credential for a short-lived Service Token. Secrets are
accepted from an environment variable or file, never from a command-line flag:

```sh
export IDENTRA_CLIENT_ID='<client-id>'
export IDENTRA_CLIENT_SECRET='<client-secret>'
identra token service --endpoint localhost:50051
```

Use the returned `token.value` as `IDENTRA_SERVICE_TOKEN` or store it in a file,
then manage machine identities remotely:

```sh
export IDENTRA_SERVICE_TOKEN='<service-token>'

identra service-account create \
  --name reporting-worker \
  --scope identra.service_accounts.read

identra service-account list
identra service-account rotate --client-id '<client-id>'
identra service-account disable --client-id '<client-id>'
```

The same online CLI is available from the Docker image. With the server already
running, pass secrets through inherited environment variables and address the
Compose service by name:

```sh
docker compose run --rm --no-deps \
  -e IDENTRA_CLIENT_ID -e IDENTRA_CLIENT_SECRET \
  identra token service --endpoint identra:50051
```

The CLI connects to `localhost:50051` without TLS by default, matching the local
server. Use `--endpoint` for another address and `--tls` when TLS terminates at
an upstream proxy. The built-in scopes are `identra.admin`,
`identra.service_accounts.manage`, `identra.service_accounts.read`, and `identra.audit.read`.
Use `identra audit list` to inspect management activity and `identra server-info` to discover the
running version and capabilities.

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
- `auth.rsa_private_key_file` (defaults to `data/signing-key.pem`)
- `auth.oauth_state_expiration`
- `auth.access_token_expiration`, `auth.refresh_token_expiration`, `auth.service_token_expiration`, `auth.token_issuer`
- `auth.github.client_id`, `auth.github.client_secret`
- `redis.urls`, `redis.password`
- `persistence.type`, `persistence.sqlite.path`
- `smtp_mailer.*`

SQLite is the current persistence implementation. Redis stores email codes, OAuth state, rate limits,
and refresh-token revocations. If no inline RSA private key is configured, Identra creates a private
key once at `auth.rsa_private_key_file` and reuses it across restarts. Back up that file together with
the SQLite database.

The gRPC listener is plaintext. Production deployments should expose it through a TLS-capable ingress,
load balancer, or service mesh and keep the direct listener on a trusted network. The CLI's `--tls`
flag is for that external TLS endpoint.

## Development

```sh
make generate       # regenerate protobuf/gRPC and sqlc code
make test           # run unit tests
make test-integration
make verify         # vet, tests, lint, architecture, generated-file check
```

Install Buf and sqlc before generation. The committed protobuf outputs must remain in sync with the
definitions under `proto/identra/v1`.

See [docs/CONFIGURATION.md](docs/CONFIGURATION.md) for deployment configuration,
[docs/UPGRADING_V0.2.md](docs/UPGRADING_V0.2.md) for the upgrade procedure,
[docs/RELEASING.md](docs/RELEASING.md) for release operations, and [Agent.md](Agent.md) for a
consumer-oriented integration guide.

Railway deployments can use the idempotent startup bootstrap described in
[docs/RAILWAY.md](docs/RAILWAY.md); it does not require a pre-deploy command.
