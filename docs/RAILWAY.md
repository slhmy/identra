# Deploying on Railway

Railway pre-deploy commands run without the service Volume, so they cannot initialize Identra's SQLite
database. Identra supports an opt-in, idempotent startup bootstrap for this deployment model.

## Service layout

1. Deploy Identra from its Dockerfile with the default `identra serve` command.
2. Add a Railway Volume mounted at `/app/data`.
3. Add a Redis service in the same Railway environment and use its private address for `REDIS_URLS`.
4. Set the target port or `GRPC_PORT` to `50051`.

The Volume retains both `/app/data/users.db` and `/app/data/signing-key.pem`. Railway does not allow
replicas for services with Volumes, which also matches the current single-writer SQLite architecture.

## First-start variables

Generate the ID and secret outside Railway, store the secret as a Railway secret variable, and configure:

```text
PERSISTENCE_SQLITE_PATH=/app/data/users.db
AUTH_RSA_PRIVATE_KEY_FILE=/app/data/signing-key.pem
GRPC_PORT=50051

BOOTSTRAP_SERVICE_ACCOUNT_ENABLED=true
BOOTSTRAP_SERVICE_ACCOUNT_NAME=platform-admin
BOOTSTRAP_SERVICE_ACCOUNT_CLIENT_ID=isa_railway_admin
BOOTSTRAP_SERVICE_ACCOUNT_CLIENT_SECRET=<at-least-32-random-characters>
BOOTSTRAP_SERVICE_ACCOUNT_SCOPES=identra.admin
```

On startup Identra runs schema migrations, then atomically creates the configured account if it does not
exist. The plaintext secret is never logged or stored. Later deploys leave an existing account untouched;
a changed client ID for the same name fails startup instead of silently creating a different administrator.

After exchanging the bootstrap credential, rotate it through `ServiceAccountService` and disable startup
bootstrap if automatic disaster recovery is not required. If it remains enabled, restoring an empty Volume
will recreate the account with the configured Railway secret.

## Networking

For services in the same project, use `identra.railway.internal:50051` over Railway private networking.
For public access, Railway documents HTTP/2 support at its HTTPS edge, but does not explicitly guarantee
gRPC behavior for every routing setup. Verify reflection and streaming against the chosen public domain.
The raw TCP Proxy does not add Identra application TLS; do not transmit Bearer or bootstrap credentials
over an untrusted plaintext connection.

Schedule Railway Volume backups because the database and signing key must be restored together.
