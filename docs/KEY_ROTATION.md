# Signing Key Rotation Guide

Identra signs JWTs with RS256 and exposes verification keys through
`identra.v1.KeyService/ListSigningKeys`.

## Key lifecycle

- **ACTIVE**: signs new tokens and is returned to clients.
- **PASSIVE**: no longer signs new tokens but remains available for validating existing tokens.
- **RETIRED**: is removed after every token it signed has expired.

Only one key may be ACTIVE. Multiple PASSIVE keys may coexist during a rotation.

## Rotation procedure

1. Create a new PASSIVE key with `KeyManager.AddKeyPassive`.
2. Allow clients to discover it through `KeyService.ListSigningKeys`.
3. Promote it with `KeyManager.PromoteKey`; the previous ACTIVE key becomes PASSIVE.
4. Keep the old key available for at least the maximum access-token lifetime.
5. Retire the old key with `KeyManager.RetireKey`.

Clients should cache the complete signing-key list, select a key by the JWT `kid`, and refresh the list
when an unknown `kid` is encountered. A `SigningKey` identifies its algorithm and carries a typed RSA
public key containing modulus bytes and exponent.

## Operational checks

- Before promotion, confirm both the current ACTIVE key and new PASSIVE key are returned.
- After promotion, confirm new tokens contain the new key ID.
- Before retirement, confirm the oldest token signed by the previous key has expired.
- Alert on unknown key IDs, verification failures, or multiple ACTIVE keys.

The current in-memory key manager API supports lifecycle transitions, while persistent key-ring storage
and automated scheduled rotation remain future work.
