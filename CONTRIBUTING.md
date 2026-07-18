# Contributing

## Local development

Start Redis, Mailpit, and the gRPC server:

```sh
make dev
```

For a faster host-side loop:

```sh
make dev-infra
make run-grpc
```

The server listens on `localhost:50051`, enables reflection and gRPC health checking, and captures
development email in Mailpit at `http://localhost:8025`.

## Verification

Run the same core checks used by CI:

```sh
make verify
```

Focused targets:

```sh
make test
make test-integration
make vet
make lint
make arch-check
make generate-check
```

The integration target expects Redis at `localhost:6379` by default. Override it with
`IDENTRA_REDIS_URL=host:port`.

## Protobuf generation

Install Buf and sqlc, then run:

```sh
make generate
```

Buf generates only Go protobuf and gRPC code. Generated files under `gen/go` are committed. Keep RPC
messages colocated with their domain service and put shared token, provider, user, and signing-key types
in `types.proto`.

## Architecture boundary

`internal/identra` contains service behavior and collaborator interfaces. It must not import outer
application, bootstrap, cache, configuration, mail, OAuth, or persistence implementations. Update the
architecture test intentionally if that boundary changes.
