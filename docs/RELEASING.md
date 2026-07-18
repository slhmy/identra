# Releasing Identra

Releases are produced from `vX.Y.Z` tags. A suffix such as `v0.2.0-rc.1` creates a GitHub prerelease and
does not move the `latest` container tag.

1. Update `CHANGELOG.md`, run `make verify`, and ensure `main` is clean.
2. Create and push `v0.2.0-rc.1`.
3. Download an archive and verify it against `checksums.txt`.
4. Exercise bootstrap, token exchange, restart, and upgrade against production-like data.
5. Fix issues on `main` and repeat with the next RC. Do not retag an existing RC.
6. After acceptance, create and push `v0.2.0` from the accepted commit.

The release workflow gates publishing on tests, vet, architecture checks, Buf lint, generated-file
consistency, Redis contracts, and the restart smoke test. It publishes Windows amd64, Linux amd64/arm64,
and macOS amd64/arm64 archives with SHA-256 checksums. GHCR receives both Linux architectures with exact,
minor, and (for stable releases) `latest` tags.
