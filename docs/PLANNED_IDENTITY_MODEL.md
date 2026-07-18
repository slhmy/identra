# Identra Identity Model

This document defines Identra’s identity model and the rules for **login**, **binding**, and **automatic account merging**.

It is written to be **unambiguous**: engineers should be able to implement or test behavior directly from the rules below.

---

## Goals

1. Support **one user** binding to **multiple OAuth providers** (e.g., GitHub + Google).
2. Support **passwordless** login via **email code** (and planned **SMS code**).
3. Support **password** registration/login as an explicit flow (not “login creates account”).
4. Allow **account merging** during OAuth login **only when a provider supplies a verified email**.
5. Allow **one account to have multiple emails** (and multiple phones), with a single primary email/phone.

---

## Implementation Roadmap

Use this as the PR-sized execution plan for moving from the current user table toward the full model.

### Current baseline

- Users have one primary `email` field and optional password hash.
- OAuth identities are split into `external_identities` with a unique `(provider, provider_user_id)` binding.
- Password registration is explicit via `AuthService.RegisterWithPassword`; password login does not create or set accounts.
- Refresh tokens can be revoked and are rotated on successful refresh.

### Phase 1: Email identity table

- Add `email_identities` with `user_id`, normalized `email`, `verified`, `primary`, `added_by`, and timestamps.
- Backfill existing `users.email` into `email_identities` as `primary=true`.
- Keep `users.email` temporarily as a denormalized primary email for compatibility.
- Update registration, email-code login, OAuth login, and login-info responses to read/write through the email identity table.

### Phase 2: Verified-email OAuth merge

- Require provider email provenance: provider, email, verified flag, and whether the provider marks it primary.
- Merge OAuth login into an existing user only when the matched email identity is verified and provider email is verified.
- Return a conflict instead of merging when provider email is missing, unverified, or already linked inconsistently.
- Add tests for same-provider login, new-provider bind, verified-email merge, and unverified-email no-merge.

### Phase 3: Account management APIs

- Add APIs to list login methods, add/remove secondary emails, set primary email, and unlink OAuth identities.
- Require at least one remaining login method before removing an email/password/OAuth identity.
- Add audit events for link, unlink, primary-change, password-change, and token revocation.

### Phase 4: Phone/SMS identities

- Add `phone_identities` with E.164 normalization, verification state, primary flag, and added-by metadata.
- Add SMS-code send/login flows using the same rate-limit and verification-code patterns as email code.
- Extend login-info responses with phone identity summaries.

---

## Core Concepts

### User
A **User** is the internal identity principal. It is uniquely identified by `user_id` (e.g., UUID).

A user can exist even if it has **no email** and **no phone** (e.g., OAuth provider did not supply a verified email).

### External Identity (OAuth identity)
An **External Identity** represents an account from an OAuth provider.

- Key: `(provider, provider_user_id)`
- Example: `(github, "123456")`, `(google, "abcdef")`
- Each external identity is linked to exactly one `user_id`.

> External identities are the authoritative keys for “sign in with GitHub/Google”.

### Email Identity
An **Email Identity** represents an email address owned by a user.

- Key: `email` (normalized)
- Properties:
  - `verified` (boolean)
  - `primary` (boolean)
  - `added_by` (e.g., `oauth:github`, `email_code`, `manual`)
  - optional metadata (timestamps, etc.)

A user may have **multiple** emails, but at most **one** is primary.

### Phone Identity (planned)
A **Phone Identity** represents a phone number owned by a user.

- Key: `phone_e164` (normalized to E.164)
- Properties:
  - `verified` (boolean)
  - `primary` (boolean)
  - `added_by` (e.g., `sms_code`, `manual`)

A user may have **multiple** phones, but at most **one** is primary.

---

## Uniqueness Constraints (Hard Rules)

These are invariants that must always hold.

1. **External identity is globally unique**
   - `(provider, provider_user_id)` MUST be linked to **exactly one** user.
   - It MUST NOT be linked to multiple users.

2. **Email is globally unique**
   - A normalized email MUST belong to **at most one** user.
   - (I.e., the same email cannot be attached to two different users.)

3. **Phone is globally unique (planned)**
   - A normalized phone number (E.164) MUST belong to **at most one** user.

4. **Primary constraints**
   - For each user: at most one primary email.
   - For each user: at most one primary phone (planned).

---

## Normalization Rules

Normalization MUST happen **before** any uniqueness check or lookup.

### Email normalization
Given an input email `raw_email`:

- `email = strings.TrimSpace(raw_email)`
- `email = strings.ToLower(email)`

> Do NOT apply provider-specific alias normalization (e.g., Gmail `+tag` stripping) unless explicitly added later.

### Phone normalization (planned)
Given an input phone number:

- Parse and format into **E.164** (e.g., `+14155552671`)
- Store only E.164 in persistence

> If parsing fails, the request is invalid.

---

## Verification Semantics

### Verified email (provider)
A provider email is considered `verified=true` **only if** the provider API explicitly indicates the email is verified.

- If the provider does not supply a verified signal, treat it as `verified=false`.
- **Automatic merge is allowed only with verified provider email** (see OAuth Login Flow).

### Verified email (email code)
When a user successfully logs in via **email-code** using an email, that email becomes `verified=true` for that user (since control over inbox is proven).

### Verified phone (SMS code) (planned)
When a user successfully logs in via **sms-code** using a phone number, that phone becomes `verified=true` for that user.

---

## Login / Bind Flows

### API Surface (current)
At time of writing, Identra exposes a gRPC-only API:

- `AuthService.RegisterWithPassword`, `LoginWithPassword`
- `AuthService.RequestEmailLoginCode`, `LoginWithEmailCode`
- `AuthService.ListOAuthProviders`, `StartOAuthLogin`, `LoginWithOAuth`
- `SessionService.RefreshSession`, `RevokeSession`
- `UserService.GetCurrentUser`, `LinkOAuthAccount`
- `KeyService.ListSigningKeys`

---

## OAuth Login Flow (automatic merge allowed only with verified email)

OAuth login inputs:
- `provider`
- `provider_user_id`
- `provider_emails[]` (optional, may include `verified` and `primary` info)
- (Optional) profile fields (username/avatar)

#### Chosen provider email
If the provider returns multiple emails, Identra MUST select **one** email for merge decisions:

1. Prefer `primary && verified`
2. Else prefer first `verified`
3. Else: no chosen verified email exists

Only the **chosen verified email** may trigger automatic merge.

#### Decision Table: OAuth Login

| Condition | Action |
|---|---|
| External identity `(provider, provider_user_id)` already linked to `userA` | Log in as `userA` |
| External identity not linked AND chosen verified email exists AND email belongs to `userB` | **Merge**: link the external identity to `userB`, then log in as `userB` |
| External identity not linked AND chosen verified email exists AND email belongs to no user | Create `userC`, link external identity to `userC`, attach email to `userC` as verified, log in as `userC` |
| External identity not linked AND no chosen verified email exists | Create `userC`, link external identity to `userC` (no email attached by default), log in as `userC` |

#### Notes
- Merge happens only in OAuth login, not in manual bind.
- If an external identity is linked to a user, it is authoritative even if provider email changed later.

---

## OAuth Bind Flow (manual binding; email mismatch allowed)

Bind flow inputs:
- Current authenticated user: `current_user_id`
- OAuth `code` and `state` -> yields `(provider, provider_user_id, provider_emails...)`

Bind is user-initiated, so **it does not perform automatic merge** based on email matching.

#### Decision Table: OAuth Bind

| Condition | Action |
|---|---|
| Identity already linked to `current_user_id` | Idempotent success |
| Identity already linked to a different user | Fail with `AlreadyExists` |
| Identity not linked to any user | Link identity to `current_user_id` |

#### Optional email attachment during bind
If the provider supplies a **verified** email:
- It MAY be attached to the current user as an email identity **only if** it does not violate global uniqueness.
- It MUST NOT steal ownership of an email that belongs to another user.
- If the current user has no primary email, the first verified email attached MAY become primary.

> It is acceptable to bind a provider even when provider email differs from the user’s existing emails.

---

## Email-code Login Flow (passwordless)

Email-code flow inputs:
- `email` (normalized)
- `code`

Hard rule: user creation may happen **only after** code verification succeeds.

#### Behavior
1. Verify `code` for `email`:
   - if invalid/expired: fail
2. Lookup user by email identity:
   - if exists: log in as that user
   - if not exists: create new user, attach `email` as `verified=true`, set as primary email, log in

---

## SMS-code Login Flow (planned; analogous to email-code)

Inputs:
- `phone` (normalized E.164)
- `code`

Behavior mirrors email-code:
- Verify code first, then:
  - existing phone identity -> login
  - not found -> create user and attach verified phone, set primary phone

---

## Password Registration & Login

### `AuthService.RegisterWithPassword`
Inputs:
- `email` (normalized)
- `password`

Rules:
- If a user already exists with this email identity: fail with `AlreadyExists`
- Create a user, attach email (`verified=false` unless you also require email verification), store password hash
- Return token pair

> If you later require email verification for password accounts, document the additional steps here.

### `AuthService.LoginWithPassword`
Inputs:
- `email` (normalized)
- `password`

Rules:
- If user not found by email identity: `NotFound`
- If user exists but has no password set: `FailedPrecondition`
- If password does not match: `Unauthenticated`
- On success: return token pair

---

## Multi-email / Multi-phone Rules

### Adding emails
An email may be added to a user when:
- It is verified via email-code login, OR
- It is provided by an OAuth provider and is verified, OR
- It is added manually (if you support that later) but remains `verified=false` until verified.

### Primary selection
- When a user receives its **first verified email**, set it as primary email.
- When a user receives additional verified emails, DO NOT automatically change primary email (unless primary is missing).
- Same concept for phones.

### Removal
If a primary email is removed:
- Either pick another verified email as primary (implementation-defined), OR
- allow “no primary email” state (but document which you choose).

---

## Conflict Matrix & Error Codes

This table describes the expected gRPC status codes.

| Scenario | gRPC code | Notes |
|---|---|---|
| Missing required fields | `InvalidArgument` | e.g., empty email/password |
| OAuth provider not supported | `InvalidArgument` | |
| OAuth not configured (missing client id/secret) | `FailedPrecondition` | |
| OAuth state invalid/expired | `InvalidArgument` | |
| OAuth identity already linked to another user during bind | `AlreadyExists` | prevents account hijack |
| RegisterWithPassword email already exists | `AlreadyExists` | |
| LoginWithPassword user not found | `NotFound` | |
| LoginWithPassword no password set | `FailedPrecondition` | OAuth-only account |
| Wrong password / wrong email code | `Unauthenticated` | |
| Rate limit exceeded (login attempts / send code) | `ResourceExhausted` | brute-force protection |
| Internal persistence failure | `Internal` | |

---

## Security Notes

### Why automatic merge is restricted
Automatic merge can accidentally join two users if the merge key is not trustworthy. Therefore:
- Automatic merge occurs only during OAuth login
- Automatic merge requires a **verified** email from the provider
- Only a single **chosen verified email** is used for merge decisions

### User enumeration considerations
Endpoints like “send email code” can leak whether an email exists if responses differ. Recommended:
- Return a generic success response for send-code (regardless of existence), and rely on rate limiting
- Avoid distinct error messages that reveal user presence

### Rate limiting
Rate limiting should apply to:
- Send verification code attempts per identifier (email/phone)
- Login failure attempts per identifier (email/phone)
- Optionally per IP address (future enhancement)

---

## Examples (Concise)

### Example 1: OAuth login merges into existing email account
- User U1 exists with verified email identity `a@example.com`
- OAuth login returns external identity `(github, 123)` and chosen verified email `a@example.com`
- Result: bind `(github, 123)` -> U1, login as U1

### Example 2: Bind succeeds with different email
- User U1 primary email is `a@example.com`
- User U1 binds Google identity `(google, abc)` even if provider email is `b@example.com`
- Result: identity is linked to U1; `b@example.com` may be attached only if verified and not owned by others

---

## Open Questions (to decide later)
These are intentionally left as future decisions; do not implement inconsistent behavior.

1. Should password registration require email verification before enabling password login?
2. Should we support multiple identities for the same provider on one user (typically **no**)?
3. How to resolve primary email/phone when primary is deleted?

---
