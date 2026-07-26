# AGENTS

Identra is a single Go (gRPC-only) authentication and user-management service. Standard
commands live in the `Makefile`, `README.md`, and `CONTRIBUTING.md`; consult those first.
`Agent.md` is the consumer-facing API integration guide.

## Cursor Cloud specific instructions

Environment already provisioned by the setup/update script: Go toolchain, Go module deps,
and the CLI tools `buf`, `sqlc`, `protoc-gen-go`, `protoc-gen-go-grpc`, `grpcurl` (on
`$(go env GOPATH)/bin`), plus `golangci-lint` (installed to `./bin` by `make tools`).
Redis and `jq` are installed system-wide.

- Put `$(go env GOPATH)/bin` on `PATH` before running `make lint`, `make generate`, or
  `grpcurl` (e.g. `export PATH="$(go env GOPATH)/bin:$PATH"`). `golangci-lint` is at `./bin`.
- Redis is required at runtime AND for `make test-integration`. Start it once per boot with
  `redis-server --daemonize yes` (verify with `redis-cli ping`). It listens on
  `localhost:6379`.
- Docker is NOT available here, so `make dev` and `make dev-infra` (both `docker compose`)
  do not work. Run the service directly instead: `make run`, or build with
  `go build -o bin/identra ./cmd/identra` and run `./bin/identra serve`.
- Mailpit/SMTP is optional and not running. The email-code login flow (`RequestEmailLoginCode`)
  cannot deliver codes without it; password, OAuth-config, session, key, service-account, and
  system flows all work without SMTP.
- The server needs persistence + key env vars when run outside `make run`. A working set:
  `PERSISTENCE_TYPE=sqlite PERSISTENCE_SQLITE_PATH=<path>/users.db`
  `AUTH_RSA_PRIVATE_KEY_FILE=<path>/signing-key.pem REDIS_URLS=localhost:6379`.
  The signing key is created once at `AUTH_RSA_PRIVATE_KEY_FILE` and reused across restarts.
- The gRPC server binds `localhost:50051`. `scripts/smoke.sh` starts its own server on the
  same port, so stop any already-running `identra serve` first or the smoke test's
  `token service` step fails with a misleading "IDENTRA_SERVICE_TOKEN required" error.
- Create the first privileged service account offline before management CLI flows:
  `./bin/identra bootstrap service-account --name platform-admin --scope identra.admin`
  (uses the same persistence env vars). Bootstrap is blocked after the first account exists.
- Full CI-equivalent gate is `make verify` (vet, test, lint, arch-check, generate-check);
  the Redis integration test and `scripts/smoke.sh` are run separately in CI.
