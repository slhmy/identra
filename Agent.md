# Agent Integration Guide

Identra exposes a gRPC-only authentication and user-management API on `identra.v1`.

## Services

- `AuthService`: password, email-code, and OAuth authentication
- `SessionService`: refresh-token rotation and revocation
- `UserService`: current-user data and OAuth account linking
- `KeyService`: typed RSA public keys for JWT verification

Use server reflection or the proto files under `proto/identra/v1` as the API reference:

```sh
grpcurl -plaintext localhost:50051 list
grpcurl -plaintext localhost:50051 describe identra.v1.UserService
```

## Tokens

Authentication responses contain `tokens: TokenPair`. Access tokens are short-lived RS256 JWTs; refresh
tokens are longer-lived and are rotated after every successful refresh. Each token contains `value` and
`expires_at` fields.

JWT claims include standard `iss`, `sub`, `exp`, `iat`, `nbf`, and `jti` claims plus `uid` and `typ`.
Retrieve verification keys with `KeyService.ListSigningKeys` and select the key matching the JWT `kid`.

For authenticated methods, send the access token only as metadata:

```text
authorization: Bearer <access-token>
```

## Common flows

### Email-code login

1. `AuthService.RequestEmailLoginCode(email, use_html)`
2. Collect the six-digit email code.
3. `AuthService.LoginWithEmailCode(email, code)`

### Password login

- Create an account with `AuthService.RegisterWithPassword`.
- Sign in later with `AuthService.LoginWithPassword`.

### GitHub OAuth

1. Verify availability through `AuthService.ListOAuthProviders`.
2. Call `AuthService.StartOAuthLogin` with `AUTH_PROVIDER_GITHUB` and a redirect URL.
3. Complete GitHub authorization.
4. Exchange the returned code/state using `AuthService.LoginWithOAuth`.

Linking an OAuth identity uses the same start flow followed by `UserService.LinkOAuthAccount` with
Bearer metadata.

### Session refresh and logout

- `SessionService.RefreshSession(refresh_token)` returns a new pair and revokes the consumed token.
- `SessionService.RevokeSession(refresh_token)` logs out that refresh token.

## Runtime requirements

Redis is required for email codes, OAuth state, rate limiting, and refresh-token revocation. SQLite is
the supported persistence backend. SMTP is optional; email-code delivery is unavailable when SMTP is
disabled. The standard gRPC health service is registered for liveness/readiness integrations.
