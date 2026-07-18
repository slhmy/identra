# Configuration reference

Identra reads `config.toml`, environment variables, and built-in defaults. Environment names are the
upper-case key with dots replaced by underscores; `auth.rsa_private_key_file` becomes
`AUTH_RSA_PRIVATE_KEY_FILE`.

| Key | Default | Purpose |
| --- | --- | --- |
| `grpc_port` | `50051` | Plaintext gRPC listener port. |
| `persistence.type` | `sqlite` | Persistence implementation. |
| `persistence.sqlite.path` | `data/users.db` | SQLite database path. |
| `auth.rsa_private_key_file` | `data/signing-key.pem` | Generated/read RS256 private key. |
| `auth.rsa_private_key` | empty | Inline PEM override; takes precedence over the key file. |
| `auth.token_issuer` | `identra` | JWT issuer. |
| `auth.access_token_expiration` | `15m` | User access-token lifetime. |
| `auth.refresh_token_expiration` | `168h` | Refresh-token lifetime. |
| `auth.service_token_expiration` | `15m` | Service Token lifetime. |
| `auth.oauth_state_expiration` | `10m` | OAuth state lifetime. |
| `auth.oauth.fetch_email_if_missing` | `false` | Fetch a GitHub primary email when needed. |
| `auth.github.client_id/client_secret` | empty | GitHub OAuth credentials. |
| `redis.urls` | `localhost:6379` | Redis nodes. |
| `redis.password` | empty | Redis password. |
| `smtp_mailer.host/port` | empty | SMTP endpoint; empty host disables delivery. |
| `smtp_mailer.username/password` | empty | Optional SMTP credentials. |
| `smtp_mailer.from_email/from_name` | empty | Sender identity. |
| `smtp_mailer.start_tls` | `true` | Upgrade SMTP with STARTTLS. |
| `smtp_mailer.auth_enabled` | `true` | Enable SMTP authentication. |
| `bootstrap.service_account.enabled` | `false` | Idempotently create the first service account during startup. |
| `bootstrap.service_account.name` | `platform-admin` | Startup bootstrap account name. |
| `bootstrap.service_account.client_id` | empty | Stable operator-supplied client ID. |
| `bootstrap.service_account.client_secret` | empty | Operator-supplied secret of at least 32 characters. |
| `bootstrap.service_account.scopes` | `identra.admin` | Startup bootstrap scopes. |

Signing keys and SQLite data must share the same durable backup boundary. The generated key has
owner-only permissions on Unix. Never place PEM content in logs or source control.

## Docker Secrets

Mount a Docker Secret as the configured key file. Keep database storage on a volume:

```yaml
services:
  identra:
    image: ghcr.io/slhmy/identra:0.2
    environment:
      AUTH_RSA_PRIVATE_KEY_FILE: /run/secrets/identra_signing_key
      PERSISTENCE_SQLITE_PATH: /app/data/users.db
      REDIS_URLS: redis:6379
    secrets: [identra_signing_key]
    volumes: [identra-data:/app/data]
secrets:
  identra_signing_key:
    file: ./secrets/signing-key.pem
```

To let Identra generate the key, use writable `/app/data/signing-key.pem` instead of a read-only Secret.

## Kubernetes Secrets

Mount the signing key, inject provider credentials, and use a PersistentVolumeClaim for SQLite. Only
one Identra replica should write a SQLite volume.

```yaml
env:
  - name: AUTH_GITHUB_CLIENT_ID
    valueFrom: {secretKeyRef: {name: identra, key: github-client-id}}
  - name: AUTH_GITHUB_CLIENT_SECRET
    valueFrom: {secretKeyRef: {name: identra, key: github-client-secret}}
  - name: AUTH_RSA_PRIVATE_KEY_FILE
    value: /var/run/identra/signing-key.pem
volumeMounts:
  - {name: signing-key, mountPath: /var/run/identra, readOnly: true}
  - {name: data, mountPath: /app/data}
volumes:
  - name: signing-key
    secret: {secretName: identra-signing-key, defaultMode: 0400}
```

## TLS boundary

Identra deliberately serves plaintext gRPC. Terminate HTTP/2 TLS at a gRPC-capable ingress, cloud load
balancer, Envoy, or service mesh. Restrict the backend listener to a trusted network and preserve the
`authorization` metadata. Clients use the CLI's `--tls` flag when connecting to that external endpoint.

Startup bootstrap is intended for platforms where an offline command cannot access the runtime volume.
The configured secret is hashed before persistence and is never logged. Once an account with the same
name and client ID exists, startup does not recreate or overwrite its credentials.
