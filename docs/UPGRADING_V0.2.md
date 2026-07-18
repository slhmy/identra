# Upgrading to v0.2.0

v0.2.0 is a one-time breaking protocol upgrade. There is no compatibility service for the old RPC names.

1. Stop Identra and back up both the SQLite database and any configured RSA private key.
2. Regenerate clients from `proto/identra/v1` and migrate calls to the domain services.
3. Configure `AUTH_RSA_PRIVATE_KEY_FILE` on durable storage. If `AUTH_RSA_PRIVATE_KEY` was already used,
   keep it configured or copy the same key into the file.
4. Start one v0.2.0 instance. SQLite migrations run automatically and atomically.
5. Call `SystemService.GetServerInfo`; confirm the expected version, schema version, and capabilities.
6. Bootstrap the first service account offline, exchange its credential, and verify `AuditService` access.
7. Roll out clients and remaining instances.

Databases opened by a newer schema version are rejected by older Identra builds. Rollback therefore
requires restoring the pre-upgrade database backup. If the old deployment used the implicit ephemeral
RSA key, its outstanding JWTs cannot survive the first v0.2.0 restart; plan a fresh login window.

For containers, keep `/app/data` as a durable volume. It contains `users.db` and, by default,
`signing-key.pem`. Never replace one without the other during restore.
