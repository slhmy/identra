# Contributing to Identra

Thank you for considering contributing to Identra!
We welcome contributions from the community to help improve and expand the project.

## Development Setup

### Local Stack

Start Redis, the gRPC service, and the HTTP gateway:

```sh
make dev
```

The Compose stack exposes the gRPC service on `localhost:50051`, the HTTP gateway on
`http://localhost:8080`, and Redis on `localhost:6379`. The gRPC container stores local
SQLite data at `/app/data/users.db` on the `identra-data` Docker volume. Mailpit captures
local email on SMTP port `1025`; inspect messages at `http://localhost:8025`.

For host-side Go debugging without rebuilding images after every edit:

```sh
make dev-infra
make run-grpc       # terminal 1
make run-gateway    # terminal 2
```

Stop Redis and Mailpit with `make dev-down`.

### Local Verification

Run the same core checks locally before opening a pull request:

```sh
make verify
```

Useful focused targets:

```sh
make test            # run Go tests
make lint            # run Go and protobuf lint checks
make generate        # regenerate protobuf, gRPC gateway, and OpenAPI outputs
make generate-check  # regenerate code and fail if gen/ changed
make arch-check      # enforce core package import boundaries
make test-integration # run Redis-backed cache contract tests
```

`make test-integration` expects Redis at `localhost:6379` by default. Override it with
`IDENTRA_REDIS_URL=host:port make test-integration` when testing against a different Redis.

### Architecture Boundary

`internal/identra` is the core service package. It may define service behavior and
interfaces, but it must not import outer infrastructure packages such as `internal/app`,
`internal/bootstrap`, `internal/cache`, `internal/config`, `internal/gateway`,
`internal/mail`, `internal/oauth`, or `internal/store`. Update
`internal/arch/identra_boundaries_test.go` intentionally if the boundary changes.

### Generating Protobuf Code

```sh
# Install Buf if you haven't already
brew install bufbuild/buf/buf

make generate
```
