# Reservoir Security Review

Date: 2026-05-22
Scope: whole-system review of `main` (commit `a613bba`)
Reviewer: security audit pass via Claude Code

> To run a security review like this on any existing PR, run `/security-review` from Claude Code. To run this for the entire repo, run that slash command and then clarify that you want to run it on the entire codebase.

## Summary

Five concrete, exploitable issues identified. Four are in this report; the fifth — unauthenticated JWT minting via `POST /auth/google` and `POST /auth/clever` (added in PR #7) — was identified separately and is excluded here only to avoid duplication. All five should be treated as a single workstream.

| # | Finding | Severity | Confidence |
|---|---|---|---|
| 0 | `POST /auth/google` & `/auth/clever` accept attacker-supplied `uid`/`email`, ignore `accessToken` (PR #7) | HIGH | 10 |
| 1 | `POST /auth/icloud` mints JWTs for any Apple UID with zero verification | HIGH | 9 |
| 2 | Logout doesn't revoke refresh tokens — 30-day window post-logout | HIGH | 9 |
| 3 | Magic-link secret logged in plaintext on every `/auth/token` request | HIGH | 9 |
| 4 | Per-IP login rate limiter bypassed via `X-Forwarded-For` → password brute force | HIGH | 9 |

---

## Finding 0 — `POST /auth/google` and `POST /auth/clever` accept attacker-supplied `uid`/`email`, ignore `accessToken`

**Files:** `cmd/server/main.go` (public route group), `internal/oauth/handler.go:23-99` (`GoogleTokenAuth`, `CleverTokenAuth`), `internal/oauth/service.go:166-247` (`AuthenticateWithGoogleToken`, `AuthenticateWithCleverToken`)
**Severity:** HIGH · **Category:** Authentication bypass · **Confidence:** 10 · **Introduced in:** PR #7 / commit `01e744c`

These endpoints were added so the Rails LMS could call into Reservoir after completing Google/Clever SSO via OmniAuth on its side. The handlers bind four fields:

```go
var req struct {
    UID   string `json:"uid"   binding:"required"`
    Email string `json:"email" binding:"required"`
    Name  string `json:"name"`
    Token string `json:"token" binding:"required"`
}
```

The `Token` field is required by binding and then **never used** — it is not sent to `https://oauth2.googleapis.com/tokeninfo`, not used to call Google's userinfo endpoint, not verified against Google's JWKS. The service function constructs an `OAuthUserInfo` directly from the request body and passes it to `findOrCreateGoogleUser` (or the Clever equivalent), which looks up — or **auto-provisions** — a user record and then issues a JWT.

The endpoints are mounted on the public router with no shared secret, no mTLS, no IP allowlist, no signed assertion from the LMS. Any caller able to reach the ALB can mint a Reservoir JWT for any email:

```
POST /auth/google
{"uid":"whatever","email":"victim@school.edu","name":"x","token":"x"}
```

Because `findOrCreateGoogleUser` auto-creates accounts on miss (matching the existing real OAuth flow's behavior), this is also an account-creation primitive against arbitrary email addresses. The Clever endpoint is structurally identical.

The handler docstrings make the assumed trust model explicit: *"Called by LMS after OmniAuth has already completed the Google OAuth flow."* That trust assumption is not enforced anywhere — the route is on the same public group as `/auth/login`.

**Fix.** Either (a) put a shared HMAC secret on the LMS↔Reservoir channel and require a signed assertion from the LMS, or (b) actually verify the supplied `accessToken` against Google's `tokeninfo` / Clever's userinfo endpoint and ignore the body-supplied `uid`/`email` in favor of the values the provider returns. Option (a) is the typical pattern for "trusted upstream service" calls; option (b) means there's no trust assumption to enforce. Either fixes the bug; the current code does neither.

---

## Finding 1 — `POST /auth/icloud` mints JWTs for any Apple UID with zero verification

**Files:** `cmd/server/main.go:126`, `internal/oauth/handler.go:220-254`, `internal/oauth/service.go:376-419`
**Severity:** HIGH · **Category:** Authentication bypass · **Confidence:** 9

The endpoint is mounted on the public router. The handler binds a single field:

```go
var req struct {
    UID string `json:"uid" binding:"required"`
}
```

`AuthenticateWithiCloud(uid)` calls `findOrCreateiCloudUser(uid)` and immediately calls `tokenService.Generate(...)`. There is no Apple ID token, no JWKS lookup, no signature check, no nonce. The code carries the comment *"No server-side token verification is performed."* PR #5 (`f31a452 "icloud auth from client only"`) removed 252 lines of the prior legitimate Apple flow (private-key loading, client-secret generation, ID-token parsing) and replaced it with this. Apple `sub` values are not secret — they are stored in the LMS, in client devices, and routinely leak via analytics. Any attacker who learns or guesses a victim's `icloud_uid` gets a fully valid JWT for that student/parent account:

```
POST /auth/icloud  {"uid":"<victim-apple-sub>"}
```

**Fix.** Require the full Apple ID token from the client; fetch Apple's JWKS; verify RS256 signature; validate `iss=https://appleid.apple.com`, `aud=<service ID>`, `exp`, and a server-generated nonce bound to the session. Only then trust `sub`.

---

## Finding 2 — Logout doesn't revoke refresh tokens; stolen refresh tokens remain valid for 30 days

**Files:** `internal/token/jwt.go:30-81` (Generate), `internal/auth/handler.go:77-103` (Logout handler), `internal/auth/service.go:230-245` (Logout), `internal/auth/service.go:253-313` (RefreshToken), `internal/config/config.go:59` (TTL = 720h)
**Severity:** HIGH · **Category:** Session management · **Confidence:** 9

`Generate()` issues access and refresh tokens with distinct JTIs. `Logout` receives only the access token (from `Authorization`) and blacklists *its* JTI. `RefreshToken` validates signature + expiry + JTI blacklist — there is no per-user revocation flag, no token-version counter, no server-side refresh-token whitelist.

Impact: an attacker who exfiltrates a refresh token (XSS on the LMS, log leak, lost device, malicious extension) keeps minting access tokens via `POST /auth/refresh` for up to 30 days **after the user clicks Log Out everywhere**. Secondary issue: if the access token has already expired when the user clicks Log Out, `tokenService.Validate` fails and `Logout` silently no-ops — no JTI is blacklisted at all.

**Fix.** Either (a) require the refresh token to be submitted to `/auth/logout` and blacklist its JTI too, or (b) add a `token_version` integer on `users`, embed it as a claim, bump on logout-all, and check on every refresh.

---

## Finding 3 — Magic-link login secret is logged in plaintext on every `/auth/token` request

**Files:** `internal/middleware/logger.go:15,27-35`, `cmd/server/main.go:112`, `internal/auth/handler.go:52-73`, `internal/auth/service.go:141-157`
**Severity:** HIGH · **Category:** Sensitive data exposure · **Confidence:** 9

The magic-link endpoint is `GET /auth/token?token=SECRET`. The Logger middleware unconditionally calls `zap.String("query", c.Request.URL.RawQuery)` for every request, including 200s, with no key-name redaction. The secret therefore lands in stdout, every log aggregator, every LB access log, browser history, and any `Referer` header the user sends on the next navigation.

`login_tokens` rows with `permanent=true` are explicitly exempted from the 5-minute expiry check (`internal/auth/service.go:153-157`) **and are not deleted after use** — so a permanent token captured from logs is a permanent credential. Non-permanent tokens give a 5-minute replay window to anyone tailing logs.

**Fix.** (1) Accept the token via `POST` body or `Authorization` header instead of query string. (2) Redact the `token` query key in the logger middleware. (3) Eliminate `permanent=true` login tokens, or mark them single-use.

---

## Finding 4 — Per-IP rate limiter bypassed via `X-Forwarded-For`, enabling password brute force

**Files:** `cmd/server/main.go:93` (no `SetTrustedProxies`), `internal/ratelimit/limiter.go:30-37`, `internal/auth/handler.go:31`, `internal/auth/service.go:62-95`
**Severity:** HIGH · **Category:** Authentication weakness · **Confidence:** 9

Lockout keys are `ratelimit:login:<ip>:<email>` and `ratelimit:lockout:<ip>:<email>`, where `<ip>` comes from `c.ClientIP()`. The router never calls `SetTrustedProxies(...)`, so Gin honors a client-supplied `X-Forwarded-For`. There is no email-only backstop — `RecordLoginAttempt` writes to a Postgres audit table but is not consulted by the limiter.

An attacker brute-forcing a single victim email sends each guess with a fresh `X-Forwarded-For: 1.2.3.<n>`; each request creates a fresh `(ip,email)` key with count=1 and the 5-attempt lockout never fires. Teacher emails are publicly listed on most school staff pages, so user enumeration is not needed. Bcrypt cost 10 still permits dozens of guesses/sec/worker — enough to defeat weak passwords common on educational accounts.

**Fix.** (1) `router.SetTrustedProxies([known LB IPs])` (or `nil` if direct-exposed) so `c.ClientIP()` ignores spoofed `X-Forwarded-For`. (2) Add an email-only key (`ratelimit:login:email:<email>`) with a wider window. (3) Make `RecordFailedAttempt` extend rather than reset the lockout window when called during an active lockout.

---

## Reviewed and found acceptable

These areas were examined and **did not produce concrete findings** at the confidence bar for this review. They are not a clean bill of health — just "no exploitable bug visible right now."

- **SQL access** (`internal/user/repository.go`, `internal/auth/*`, `internal/oauth/*`) — all queries use `$N` parameterization through `sqlx`/`database/sql`. No string-interpolated `WHERE`, `ORDER BY`, or `LIMIT` clauses. Username-generation logic (PR #1) uses an advisory-lock pattern that is race-tested.
- **JWT algorithm pinning** (`internal/token/jwt.go:87-88`) — the parser explicitly checks `token.Method.(*jwt.SigningMethodHMAC)`, rejecting `alg:none` and HS-vs-RS confusion.
- **OAuth `state` parameter** (Google, Clever flows in `internal/oauth/`) — generated with `crypto/rand`, stored in Redis with a 10-minute TTL, deleted on first use. No replay or CSRF window observed.
- **Refresh-token rotation on `/auth/refresh`** — the old refresh JTI is blacklisted on successful rotation (the gap is logout, covered in Finding 2, not rotation itself).
- **Password hashing** (`internal/auth/password.go`) — bcrypt with cost 10. Login response is uniform on unknown-email vs bad-password (`INVALID_CREDENTIALS`), so direct user-enumeration via response code does not work. (Timing-based enumeration not measured; not raised as a finding.)
- **Migrations** (`migrations/`) — no seeded admin/backdoor rows, no overly broad `GRANT` statements.
- **CORS reflection + `Allow-Credentials: true`** with default `CORS_ALLOWED_ORIGINS="*"` (`internal/middleware/cors.go`, `internal/config/config.go:78`) — verified anti-pattern, but auth is exclusively `Authorization: Bearer` with no cookies, so browsers cannot induce credential-bearing cross-origin requests. Becomes a HIGH finding the moment any cookie-based session is introduced; recommend hardening preemptively (don't set `Allow-Credentials` when ACAO is `*`, and remove the `"*"` default).
- **Token blacklist** (`internal/token/` + Redis) — keyed on JTI; correctly checked on both access and refresh validation paths.
- **Configuration** (`internal/config/config.go`, `.env.example`) — JWT secret has no insecure default value; the service refuses to boot without it.

## Considered but excluded by scope

- **DOS / resource exhaustion** — out of scope per review rules.
- **Outdated third-party dependency versions** — managed separately.
- **Lack of audit logging / hardening best-practices** — only concrete vulnerabilities are reported.
- **The 31-hour 2026-05-19 incident** — that was an availability issue (silent DB-write failures masked by green dashboards), not a security vulnerability. Addressed by PR #12.

---

## Methodology

The review was performed in three phases following the `/security-review` workflow.

### Phase 1 — Repository context

- Identified the service: a public-facing Go + Gin auth gateway issuing JWTs for the Boddle LMS, backed by Postgres + Redis.
- Mapped the route surface from `cmd/server/main.go` and the package layout under `internal/`.
- Read `internal/middleware/*` to understand the auth, CORS, logging, and metrics chain.

### Phase 2 — Targeted analysis

A single audit subtask was dispatched with the following directive (paraphrased):

> Audit the whole codebase on `main`. Find concrete, exploitable security issues. For each finding output: file:line, severity (HIGH/MEDIUM only), category, exploit scenario, confidence 1–10 (only include 8+), fix. Focus on:
>
> 1. Routing — which endpoints are public vs auth-required, anything sensitive missing the auth middleware
> 2. JWT handling — signing-key source, algorithm pinning, audience/issuer, expiry, refresh rotation, blacklist correctness
> 3. OAuth state/CSRF — `state` validation, replay protection, PKCE, open redirect via `redirect_url`
> 4. iCloud auth — verify signature or `ParseUnverified`?
> 5. SQL — string-interpolated queries, dynamic ORDER BY/LIMIT
> 6. Password handling — bcrypt cost, timing, user enumeration, password reset
> 7. CORS / security headers — wildcard + credentials, CSRF on state-changing endpoints
> 8. Token blacklist / logout — refresh-token reuse after logout
> 9. Rate limiter — `X-Forwarded-For` spoofing, email-only fallback
> 10. Logging — passwords, JWTs, raw tokens in logs
> 11. Config — default/hardcoded JWT secret
> 12. Migrations — backdoor rows
>
> Apply standard false-positive exclusions: no DOS, no "lack of hardening," no theoretical races, no log-spoofing, no SSRF unless host/protocol controllable, no outdated-dep findings, no test-only files, no documentation files.

### Phase 3 — Parallel false-positive verification

Each finding from Phase 2 was independently re-verified by a separate read-only subagent given a skeptical brief ("try to disprove this") and asked to quote exact code lines. Findings were retained only at confidence ≥ 8. One candidate finding (CORS reflection + credentials) verified as a true positive but was demoted because the architecture is Bearer-only and cookie-less — moved to "Reviewed and found acceptable" with a forward-looking note.

## Status of open PRs against these findings

At review time, two PRs are open on the repository. **Neither addresses any finding in this report.**

- **PR #12 — `post-incident hardening: write probe, async last_logged_on, scaling defaults`** (open). Addresses the 2026-05-19 availability incident: startup write probe in `internal/database/postgres.go`, async last-login batcher in `internal/user/last_login_writer.go`, new metrics in `internal/middleware/metrics.go`, doubled CFN task sizing. None of the five security findings are touched — the PR does not modify `internal/oauth/`, the logger middleware, the refresh/logout path, or proxy/trust configuration. The closest tangential overlap is the new `reservoir_auth_db_write_errors_total` counter, which would make brute-force attempts under Finding 4 more visible (each failed login still hits `RecordLoginAttempt`), but it is detection, not prevention.
- **PR #8 — `thundering herd initial commit`** (open since 2026-04-24, empty PR body, single commit `55afcc6`). Scaling/load-handling work, not security. No overlap with any finding.

All five findings (including the known Google/Clever issue) remain unaddressed in any open PR and need their own fixes.

## Recommended priority

The prioritization below was produced by Claude Code based on the audit above, not by a human security engineer — it should be sanity-checked before being treated as authoritative.

- **This week:** Findings 0, 1, 3, 4 — each independently permits account takeover with no prior compromise.
- **This sprint:** Finding 2 — extends the blast radius of any future token leak to 30 days.
- **Preemptive hardening:** the CORS configuration noted under "acceptable" — fix before any cookie-based session is introduced.

---

## Alternative recommendations

Reservoir is doing the work of a textbook **identity provider** — JWT issuance with refresh rotation, email/password login, OAuth brokering for Google/Clever/Apple, magic-link tokens, blacklist/revocation, per-IP rate limiting, polymorphic user types. Every finding in this report is a solved problem in mature IdP software. The current code is a bespoke reimplementation of something that already exists in well-audited open-source form.

Three serious candidates, ranked for Boddle's context (K-12 LMS with Clever, small Go team, existing Rails LMS calling in via OmniAuth):

### 1. Keycloak — recommended

Java/Quarkus, Apache 2.0, run by Red Hat.

- **Identity brokering** for Google and Apple is built-in. Clever is plain OAuth2 → configure it as a generic OIDC/OAuth2 broker; no code.
- **Apple Sign-In** is brokered with full JWKS verification and nonce handling — Finding 1 disappears.
- **Refresh-token rotation + revocation** (back-channel logout, `/revoke` endpoint, per-session blacklist, "logout everywhere" by user) — Finding 2 disappears.
- **Magic links** via the built-in "Action Token" flow, POSTed/redirected not query-stringed, single-use, server-managed expiry — Finding 3 disappears.
- **Brute-force detection** with permanent + temporary lockout, email-keyed not just IP-keyed — Finding 4 disappears.
- LMS integration: keep Rails OmniAuth pointed at Keycloak as the OIDC provider; Rails stops caring about Google/Clever/Apple directly. The "LMS already verified it" anti-pattern that PR #7 created goes away because there's a real OIDC token in the middle — Finding 0 disappears.
- Admin UI, audit log, RBAC, user federation. Massive community, plenty of K-12 deployment precedent.

Trade-off: it's a JVM service. If that's a hard constraint, see #2.

### 2. Ory Kratos + Ory Hydra — best Go-native fit

Apache 2.0, Go, headless/API-first.

- **Kratos** = identity (registration, login, password reset, MFA, social sign-in, recovery, verification — the "magic link" flow done right).
- **Hydra** = OAuth2/OIDC issuer (token issuance, rotation, revocation, JWKS).
- Clean separation of concerns, matches the existing package layout in `internal/`. No JVM.
- Trade-off: two services, no built-in UI (you supply the login pages — fine if the web portal already owns that). Steeper than Keycloak for a small team.

### 3. Zitadel — modern middle ground

Apache 2.0, written in Go, single-binary or hosted. Multi-tenant by design.

- Built-in OIDC, social logins (Google + Apple verified properly), event-sourced audit log, decent admin UI.
- Newer than Keycloak; smaller community but growing. No native Clever, but Clever is OIDC-shaped so a generic provider config works.
- Good if you want "Keycloak-style completeness but in Go" without running Java.

### Not recommended

- **SuperTokens** — fine for a SaaS app, but its self-host story for "be the IdP for a separate LMS + external OAuth brokering" is weaker than the three above.
- **Authelia** — designed as a forward-auth gateway for protecting internal apps, not as a public-facing IdP.
- **Auth.js / Lucia / Passport** — libraries, not systems. Adopting one of these still leaves Boddle maintaining the IdP, just with better primitives. That's the same trap, slightly nicer walls.

### Concrete migration sketch

For Boddle's situation — small Go team, public-facing K-12 product, existing Rails LMS doing OmniAuth, real CVEs already in the homegrown code, Clever as a required broker — the recommended path is to deploy **Keycloak** and make Reservoir a thin shim (or delete it entirely):

1. Stand up Keycloak with realms for teachers / students / parents (or one realm + roles, depending on tenancy needs).
2. Configure Google, Apple, Clever as identity providers in Keycloak's admin UI.
3. Point the Rails LMS at Keycloak as its OIDC provider (replaces OmniAuth-direct-to-Google).
4. Point the Unity/web portal at Keycloak for token issuance.
5. Reservoir becomes either (a) gone, or (b) a tiny adapter mapping Keycloak's `sub` to the existing `users.id` / `MetaID` shape during the migration window — then deleted.

The "we already ship something custom" sunk cost is real but smaller than it looks: Reservoir is ~5 weeks old, has five known HIGH-severity findings, and the team is already paying the operational cost (PR #10 sizing, PR #12 hardening). Migrating to Keycloak buys the security fixes for free *and* eliminates the maintenance.

If a JVM service is genuinely off the table, **Ory Kratos + Hydra** reaches the same outcome in Go with more wiring.

This alternative-recommendations section was produced by Claude Code based on the audit above; the team should validate the operational and compliance trade-offs (COPPA, FERPA, hosting, on-call) before committing to a migration.
