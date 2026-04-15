# Identra Identity Model

This document describes the identity model and authentication rules used by Identra. It is intended to be precise enough to derive test cases from and to serve as the authoritative reference for all identity-related behaviour.

---

## Table of Contents

1. [Definitions](#1-definitions)
2. [Uniqueness Constraints](#2-uniqueness-constraints)
3. [Normalization Rules](#3-normalization-rules)
4. [Verification Semantics](#4-verification-semantics)
5. [Authentication Flows](#5-authentication-flows)
   - [5.1 OAuth Login](#51-oauth-login)
   - [5.2 OAuth Bind](#52-oauth-bind)
   - [5.3 Email-Code Login](#53-email-code-login)
   - [5.4 Password Registration](#54-password-registration)
   - [5.5 Password Login](#55-password-login)
   - [5.6 Phone / SMS Login (Planned)](#56-phone--sms-login-planned)
6. [Conflict Matrix and Error Codes](#6-conflict-matrix-and-error-codes)
7. [Rate Limiting and Brute-Force Protection](#7-rate-limiting-and-brute-force-protection)
8. [Security Notes](#8-security-notes)

---

## 1. Definitions

### User

A **User** is the central identity record. Each user has:

| Field            | Type          | Description                                                       |
|------------------|---------------|-------------------------------------------------------------------|
| `id`             | UUID string   | Immutable primary key, auto-generated on creation.               |
| `email`          | string        | The user's canonical email address. May be empty for OAuth-only accounts created without an email. Globally unique when non-empty. |
| `hashed_password`| string (nullable) | bcrypt hash of the user's password. `null` means password login is not set up for this account. |
| `github_id`      | string (nullable) | Provider user ID for the GitHub OAuth identity. Globally unique when non-empty. |
| `last_login_at`  | timestamp (nullable) | Updated on every successful authentication.                  |
| `created_at`     | timestamp     | Set on creation.                                                  |
| `updated_at`     | timestamp     | Updated on every write.                                           |

A user record may exist with **no email** (OAuth-only user whose provider did not supply one) or with an email but **no password** (email-code-only or OAuth-only account).

### External Identity (OAuth)

An **External Identity** represents a link between a User and an OAuth provider account. It is identified by the pair `(provider, provider_user_id)`.

Current implementation stores the GitHub provider identity as a single `github_id` field on the User record. Conceptually:

```
ExternalIdentity {
  provider:          "github"        // name of the OAuth provider
  provider_user_id:  string          // opaque ID issued by the provider
}
```

Each External Identity belongs to exactly one User. A provider identity may only be bound to one user at a time (globally unique).

### Email Identity

An **Email Identity** is the combination of a verified email address and a shared secret (password or one-time code) that allows a user to authenticate. The email address stored on the User record acts as the email identity key.

### Phone Identity (Planned)

A **Phone Identity** is analogous to an Email Identity but uses a phone number in E.164 format and SMS one-time codes. This is not yet implemented. See [Section 5.6](#56-phone--sms-login-planned) for the planned semantics.

---

## 2. Uniqueness Constraints

| Attribute           | Scope  | Constraint                                                                      |
|---------------------|--------|---------------------------------------------------------------------------------|
| `user.id`           | Global | Unique; auto-generated UUID. Never reused.                                      |
| `user.email`        | Global | Unique when non-empty. Two users cannot share the same (normalized) email.      |
| `user.github_id`    | Global | Unique when non-empty. One GitHub account maps to at most one User.             |
| Phone number        | Global | (Planned) Unique when non-empty, stored in E.164 format.                        |

Uniqueness is enforced at the database layer (unique index). The application layer checks for existing records before insert and maps duplicate-key errors to `ALREADY_EXISTS` gRPC status.

---

## 3. Normalization Rules

Normalization is applied before any lookup or storage operation.

| Attribute     | Rule                                         | Example                            |
|---------------|----------------------------------------------|------------------------------------|
| Email         | Leading/trailing whitespace stripped (`TrimSpace`). Case is preserved as supplied. | `"  Alice@Example.com "` → `"Alice@Example.com"` |
| Provider name | Lowercased and whitespace-stripped.          | `"  GitHub "` → `"github"`         |
| Phone number  | (Planned) Normalized to E.164 format.        | `"(555) 867-5309"` → `"+15558675309"` |

> **Note on email case:** The current implementation stores and matches email addresses exactly as normalized (trimmed, original case). Two addresses that differ only in case (e.g., `alice@example.com` vs `Alice@example.com`) are treated as **different** identities. Callers should canonicalize email addresses to lowercase before submitting them if case-insensitive uniqueness is required.

---

## 4. Verification Semantics

"Verified" means that ownership of the identity credential has been demonstrated.

| Method                   | What is verified                                         | How                                                                 |
|--------------------------|----------------------------------------------------------|---------------------------------------------------------------------|
| OAuth provider email     | The email address returned by the OAuth provider is accepted as verified **by the provider**. Identra trusts it without additional checks. | Provider asserts the email in the user-info response.               |
| Email-code (`/email/login`) | The user controls the inbox for the submitted email address. | A 6-digit one-time code (valid 10 minutes) is sent and must be consumed to complete login. |
| Password (`/password/login`) | The user knows the password stored for that email address. | bcrypt hash comparison.                                             |
| SMS-code (planned)       | The user controls the phone number.                      | A one-time code sent via SMS must be consumed.                      |

A provider email used in the OAuth auto-merge flow (Section 5.1) is treated as verified by the provider; no additional Identra-level email verification is performed.

---

## 5. Authentication Flows

### 5.1 OAuth Login

**Endpoint:** `POST /oauth/login` (gRPC: `LoginByOAuth`)

**Prerequisites:** A valid OAuth state token previously issued by `GET /oauth/url`.

#### Flow

```
Client                          Identra
  |                               |
  |-- GET /oauth/url ------------>|  (provider, redirect_url) → state token stored
  |<-- authorization URL, state --|
  |                               |
  |-- redirect to provider ------>|  (user authorizes)
  |<-- callback with code --------|
  |                               |
  |-- POST /oauth/login ----------|  (code, state)
  |                               |-- validate & consume state
  |                               |-- exchange code for access token
  |                               |-- fetch user info from provider
  |                               |-- [optional] fetch emails if missing
  |                               |-- ensureOAuthUser (see decision table)
  |<-- token pair, user info -----|
```

#### `ensureOAuthUser` Decision Table

This function resolves or creates the User record for an incoming OAuth authentication.

| Provider ID found? | Email provided? | Email matches existing user? | Action                                                                 | Result                        |
|--------------------|-----------------|------------------------------|------------------------------------------------------------------------|-------------------------------|
| Yes                | (any)           | (any)                        | Update user's email to provider email if it has changed (non-empty email only). | Existing user returned.       |
| No                 | Yes             | Yes                          | Link provider ID to the existing email-matched user (auto-merge).     | Existing user returned.       |
| No                 | Yes             | No                           | Create new user with `email` + provider ID.                           | New user created and returned.|
| No                 | No              | N/A                          | Create new user with provider ID only; email is empty.                | New user created and returned.|

**Auto-merge rule:** When a provider presents an email address that already belongs to a local user, Identra automatically links the provider identity to that local user *without asking the user to confirm*. This relies entirely on the provider having verified the email. See [Section 8](#8-security-notes) for the security rationale and limitations.

**Email update rule:** If the provider sends a different email than what is stored on the user record, Identra updates the stored email to the provider's current value. This only applies when the incoming email is non-empty.

#### Email-Fetch Fallback

If `OAuthFetchEmailIfMissing` is enabled and the initial user-info response has no email, Identra makes a second call to the provider's email API (if supported). The fetched email, if non-empty, is used in the `ensureOAuthUser` logic above.

---

### 5.2 OAuth Bind

**Endpoint:** `POST /oauth/bind` (gRPC: `BindUserByOAuth`)

Binds an existing authenticated user to an OAuth provider account. Unlike login, this requires a valid Identra access token.

#### Flow

```
Client                          Identra
  |                               |
  |-- GET /oauth/url ------------>|  (state stored)
  |-- redirect to provider ------>|
  |<-- callback with code --------|
  |                               |
  |-- POST /oauth/bind ---------->|  (access_token, code, state)
  |                               |-- validate access token → resolve user
  |                               |-- validate & consume state
  |                               |-- exchange code for provider token
  |                               |-- fetch user info from provider
  |                               |-- check for conflicts (see table)
  |                               |-- link provider ID to user
  |                               |-- update email if needed
  |<-- token pair, user info -----|
```

#### Bind Conflict Decision Table

| Current user has provider ID? | Incoming provider ID already linked to another user? | Action                                                                              | gRPC Status               |
|-------------------------------|------------------------------------------------------|-------------------------------------------------------------------------------------|---------------------------|
| No                            | No                                                   | Set provider ID on current user. Update stored email from provider if non-empty.   | OK                        |
| Same as incoming              | No (same user)                                       | Idempotent: no-op on the link; refresh token pair and update email if needed.      | OK                        |
| Different provider ID         | (any)                                                | User is already linked to a different provider account. Reject.                    | `FAILED_PRECONDITION`     |
| No                            | Yes (different user)                                 | The provider account is already owned by another user. Reject.                     | `ALREADY_EXISTS`          |

**Manual bind does not require email matching.** The user must only hold a valid access token. No email comparison is performed.

---

### 5.3 Email-Code Login

**Endpoints:**
- `POST /email/code` — send verification code (gRPC: `SendLoginEmailCode`)
- `POST /email/login` — verify code and log in (gRPC: `LoginByEmailCode`)

This is the *passwordless* login flow. It creates new accounts automatically (register-on-first-login semantics).

#### Send Code Flow

```
POST /email/code  { "email": "alice@example.com" }
```

1. Validate that the email field is non-empty.
2. Check send-code rate limit for this email address.
   - If exceeded → `RESOURCE_EXHAUSTED`
3. Generate a cryptographically random 6-digit code (zero-padded).
4. Store `(email → code)` in Redis with a 10-minute TTL (overwrites any previous code).
5. Send the code to the email address via SMTP.
6. Record the send attempt in the rate limiter.

#### Login with Code Flow

```
POST /email/login  { "email": "alice@example.com", "code": "483920" }
```

| Step | Condition                       | Action                               | gRPC Status            |
|------|---------------------------------|--------------------------------------|------------------------|
| 1    | Email or code is empty          | Reject                               | `INVALID_ARGUMENT`     |
| 2    | Login rate limit exceeded       | Reject                               | `RESOURCE_EXHAUSTED`   |
| 3    | Code is invalid or expired      | Record failed attempt; reject        | `UNAUTHENTICATED`      |
| 4    | Code is valid (consumed)        | Proceed                              | —                      |
| 5    | User exists with this email     | Use existing user                    | —                      |
| 6    | No user found with this email   | Create new user with only `email` set | —                     |
| 7    | Reset rate-limit counter        | Clear failed attempts for this email | —                      |
| 8    | Issue token pair                | Return `access_token` + `refresh_token` | OK                  |

**Key guarantee:** A new user is created **only after** the code is successfully verified. An undelivered email never results in a phantom account.

---

### 5.4 Password Registration

**Endpoint:** `POST /password/register` (gRPC: `RegisterByPassword`)

Explicitly creates a new account with an email + password.

#### Flow

```
POST /password/register  { "email": "alice@example.com", "password": "..." }
```

| Step | Condition                          | Action                               | gRPC Status        |
|------|------------------------------------|--------------------------------------|--------------------|
| 1    | Email or password is empty         | Reject                               | `INVALID_ARGUMENT` |
| 2    | User with this email already exists | Reject                              | `ALREADY_EXISTS`   |
| 3    | No existing user                   | Hash password with bcrypt; create user | —                |
| 4    | Duplicate key at create time       | Reject (race condition safeguard)    | `ALREADY_EXISTS`   |
| 5    | Issue token pair                   | Return `access_token` + `refresh_token` | OK             |

Registration returns a token pair immediately (the user is considered logged in after registration).

---

### 5.5 Password Login

**Endpoint:** `POST /password/login` (gRPC: `LoginByPassword`)

Authenticates an existing user using their email and password. Does **not** create accounts; does **not** set a password on an account that has none.

#### Flow

```
POST /password/login  { "email": "alice@example.com", "password": "..." }
```

| Step | Condition                          | Action                               | gRPC Status            |
|------|------------------------------------|--------------------------------------|------------------------|
| 1    | Email or password is empty         | Reject                               | `INVALID_ARGUMENT`     |
| 2    | Login rate limit exceeded          | Reject                               | `RESOURCE_EXHAUSTED`   |
| 3    | No user found with this email      | Reject                               | `NOT_FOUND`            |
| 4    | User has no password set           | Reject                               | `FAILED_PRECONDITION`  |
| 5    | Password is incorrect              | Record failed attempt; reject        | `UNAUTHENTICATED`      |
| 6    | Password is correct                | Reset rate-limit counter             | —                      |
| 7    | Issue token pair                   | Return `access_token` + `refresh_token` | OK                  |

#### Examples

```
# Correct password
POST /password/login { "email": "alice@example.com", "password": "correct" }
→ 200 OK  { "token": { "access_token": "...", "refresh_token": "..." } }

# Wrong password
POST /password/login { "email": "alice@example.com", "password": "wrong" }
→ 401 Unauthenticated

# Account exists but was created via email-code (no password)
POST /password/login { "email": "alice@example.com", "password": "anything" }
→ 412 Failed Precondition  "password login not set up for this account"

# Account does not exist
POST /password/login { "email": "nobody@example.com", "password": "anything" }
→ 404 Not Found  "user not found"
```

---

### 5.6 Phone / SMS Login (Planned)

Phone login is not yet implemented but is planned. The intended semantics mirror the email-code flow:

1. **Send code:** `POST /phone/code` — accept a phone number in E.164 format; enforce send-code rate limit; generate and store a 6-digit code with a 10-minute TTL; deliver via SMS.
2. **Login with code:** `POST /phone/login` — accept phone number + code; enforce login rate limit; consume code; create user if none exists; issue token pair.

Uniqueness and normalization rules for phone numbers:
- Stored in E.164 format (e.g., `+15558675309`).
- Globally unique per user (one phone number → at most one user).

---

## 6. Conflict Matrix and Error Codes

The table below maps common conflict scenarios to the gRPC status code returned and the equivalent HTTP status code produced by the gRPC-gateway.

| Scenario                                                           | gRPC Status          | HTTP Status |
|--------------------------------------------------------------------|----------------------|-------------|
| Required field is empty or missing                                 | `INVALID_ARGUMENT`   | 400         |
| OAuth state token is invalid or has expired                        | `INVALID_ARGUMENT`   | 400         |
| Provider not configured (e.g., missing client ID/secret)           | `FAILED_PRECONDITION`| 412         |
| User not found (password login only)                               | `NOT_FOUND`          | 404         |
| Current user has no password set up                                | `FAILED_PRECONDITION`| 412         |
| Email/provider already registered to a different account           | `ALREADY_EXISTS`     | 409         |
| Current user already bound to a different provider account         | `FAILED_PRECONDITION`| 412         |
| Invalid OAuth authorization code or expired                        | `UNAUTHENTICATED`    | 401         |
| Invalid or expired email code                                      | `UNAUTHENTICATED`    | 401         |
| Invalid access token (for bind or user-info)                       | `UNAUTHENTICATED`    | 401         |
| Wrong password                                                     | `UNAUTHENTICATED`    | 401         |
| Invalid or expired refresh token                                   | `UNAUTHENTICATED`    | 401         |
| Rate limit exceeded (login attempts or code sends)                 | `RESOURCE_EXHAUSTED` | 429         |
| Internal failure (DB error, hash failure, etc.)                    | `INTERNAL`           | 500         |

---

## 7. Rate Limiting and Brute-Force Protection

Identra uses a Redis-backed sliding-window counter for two independent rate limits.

### Login Rate Limiter

Applied to: `POST /password/login`, `POST /email/login`

| Parameter              | Default | Config key             |
|------------------------|---------|------------------------|
| Max failed attempts    | 5       | `LoginMaxAttempts`     |
| Lockout window         | 15 min  | `LoginLockoutDuration` |

The counter key is the **email address** being authenticated. On each **failed** attempt the counter is incremented. On a **successful** login the counter is reset. The limiter is checked before any credential verification; if the limit is already exceeded the request is rejected immediately with `RESOURCE_EXHAUSTED`.

The limiter fails **open**: if Redis is unavailable the check is skipped and login proceeds normally. This prioritizes availability over perfect rate-limit enforcement.

### Send-Code Rate Limiter

Applied to: `POST /email/code`

| Parameter              | Default | Config key              |
|------------------------|---------|-------------------------|
| Max sends per window   | 5       | `SendCodeMaxAttempts`   |
| Window duration        | 1 hour  | `SendCodeWindow`        |

The counter key is the **email address** the code is being sent to. A successful send increments the counter. The limiter also fails open.

---

## 8. Security Notes

### Why Auto-Merge Is Trusted

When an OAuth provider returns a verified email address and that address already belongs to a local user, Identra automatically links the two accounts ([Section 5.1](#51-oauth-login)). This is safe under the assumption that the provider has verified the email (e.g., GitHub requires email verification). If you use a provider that does not verify emails, you should disable or audit auto-merge behaviour.

### Why Manual Bind Does Not Require Email Matching

The bind flow ([Section 5.2](#52-oauth-bind)) does not require the provider's email to match the current user's email. The user must instead present a valid Identra access token, which proves they are already authenticated. The access token acts as the proof of ownership; email congruence is not an additional requirement.

### User Enumeration

The password login endpoint returns `NOT_FOUND` when no account exists for the given email, which reveals whether an account is registered. This is an explicit design trade-off: unambiguous error messages improve usability and are compatible with the use case (identity service for developer tools). If your deployment is sensitive to account enumeration, consider returning a uniform `UNAUTHENTICATED` response regardless of whether the account exists.

### OAuth State Store

The OAuth state token (`state` parameter) is stored in memory by default. In a multi-replica deployment, the OAuth callback may be served by a different replica than the one that generated the URL, causing state validation to fail. To support multi-replica deployments, replace the in-memory state store with a shared Redis-backed implementation.

### Token Lifetimes

| Token        | Default TTL | Config key                       |
|--------------|-------------|----------------------------------|
| Access token | 15 minutes  | `AccessTokenExpirationDuration`  |
| Refresh token| 7 days      | `RefreshTokenExpirationDuration` |

Refresh tokens are signed JWTs and are validated cryptographically. There is currently no server-side refresh-token revocation list; a stolen refresh token remains valid until it expires.

### Password Hashing

Passwords are hashed with bcrypt before storage. The raw password is never persisted.

### Email Code Security

- Codes are generated with `crypto/rand` (cryptographically secure).
- Each code is 6 digits, zero-padded.
- Codes expire after 10 minutes.
- A new code request overwrites any previously stored code for the same email.
- Codes are single-use: consuming a code deletes it from the store.
- The send-code rate limit (default: 5 per hour) limits SMS/email flooding attacks.
- The login rate limit (default: 5 per 15 minutes) limits brute-force guessing of codes.
