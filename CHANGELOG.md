# Changelog

## v0.2.0 (unreleased)

### Added

- Persistent RSA signing-key files, preserving JWT verification across restarts.
- Sequential SQLite schema migrations with downgrade protection.
- Scoped service accounts, short-lived Service Tokens, secret rotation, and offline bootstrap.
- Service Token exchange rate limiting, management audit events, and server capability discovery.
- Idempotent startup bootstrap for volume-based platforms such as Railway.
- Cross-platform release archives, checksums, multi-architecture images, and restart smoke tests.

### Changed

- The public API is gRPC-only and split into domain services.
- Protected RPC authentication uses Bearer metadata and protocol messages are strongly typed.

### Removed

- HTTP Gateway, OpenAPI generation, CORS configuration, and gateway dependencies.
- Deprecated symmetric legacy user-token helpers.

### Upgrade note

The v0.2.0 protocol is intentionally incompatible with the old `identra.v1` client surface. Regenerate
clients. The first v0.2.0 startup also replaces the old ephemeral signing key with a newly persisted key,
so sessions issued before that startup cannot be retained unless an explicit RSA key was already configured.
