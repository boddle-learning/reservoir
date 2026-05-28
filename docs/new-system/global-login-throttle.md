# Global Login Throttle

Protects the auth gateway and its downstream systems from **thundering-herd** logins — the burst of traffic that follows a dependency outage (e.g. a game server) being resolved, when every idle client reconnects at once.

## Why the per-user rate limiter is not enough

The existing [`ratelimit.Limiter`](../../internal/ratelimit/limiter.go) is a **per-user brute-force guard**. Its key is `ratelimit:login:{ip}:{email}`, so each `{ip, email}` pair has its own independent bucket. If 10,000 unique users simultaneously attempt to log in, every request passes the per-user check cleanly — the limiter never sees aggregate volume.

To cap total system throughput, a second limiter is needed: one with a single shared key across all users and all instances of the service.

## Design: Redis-backed token bucket

Implemented in [`internal/ratelimit/global.go`](../../internal/ratelimit/global.go).

| Property | Value |
|---|---|
| Algorithm | Token bucket |
| Storage | Redis hash at key `ratelimit:global:login` |
| Atomicity | Lua script (refill + consume in one round-trip) |
| Scope | Global across all gateway instances |
| Behavior when empty | Reject with `429 + Retry-After` (no queuing) |

### Why token bucket, not a fixed window

A token bucket allows **short bursts up to the capacity** while enforcing a **steady-state refill rate**. This matches what we want: a modest burst at the leading edge of an outage-recovery wave is fine; a sustained flood is not.

### Why reject instead of queue

A true queue (position feedback, hold connections until a slot opens) would require significantly more infrastructure: a dedicated queue service, long polling or websockets for status updates, and careful attention to client timeouts. For outage recovery, a simple `429 + Retry-After` is sufficient — clients back off, retry, and the surge smooths out on its own.

If future requirements demand user-visible queue position, this design can evolve: the token bucket is in front of the auth flow, not replacing it.

### Why Lua

Naive read-modify-write against Redis hashes is racy. Under load, two concurrent clients can both read `tokens=1`, both decrement, and both proceed — allowing over-consumption. The Lua script runs atomically on the Redis server, which eliminates the race without needing WATCH/MULTI round-trips.

## Configuration

Environment variables (see [`internal/config/config.go`](../../internal/config/config.go)):

| Variable | Default | Meaning |
|---|---|---|
| `RATE_LIMIT_GLOBAL_LOGIN_CAPACITY` | `200` | Bucket size — maximum burst allowed. Values `<= 0` disable the throttle. |
| `RATE_LIMIT_GLOBAL_LOGIN_REFILL_PER_SEC` | `100` | Steady-state refill rate (tokens/second). Values `<= 0` disable the throttle. |

**Defaults are placeholders.** Tune based on what the downstream PostgreSQL connection pool and any services you proxy to can actually sustain. A reasonable starting point is capacity ≈ 2× refill, so the bucket can absorb a brief 2-second burst before steady-state throttling kicks in.

The throttle is enabled only when **both** `RATE_LIMIT_GLOBAL_LOGIN_CAPACITY > 0` and `RATE_LIMIT_GLOBAL_LOGIN_REFILL_PER_SEC > 0`. Setting either to `0` (or any negative value) disables it entirely.

## Integration

Applied as a Gin middleware ([`internal/middleware/loginqueue.go`](../../internal/middleware/loginqueue.go)) on token-minting endpoints in [`cmd/server/main.go`](../../cmd/server/main.go):

| Route | Throttled? | Reasoning |
|---|---|---|
| `POST /auth/login` | ✓ | Primary login path |
| `GET /auth/token` | ✓ | Magic-link login |
| `GET /auth/google/callback` | ✓ | OAuth callback — does the DB/user-linking work |
| `GET /auth/clever/callback` | ✓ | OAuth callback — same |
| `POST /auth/icloud` | ✓ | Client-side OAuth, server mints token |
| `GET /auth/google` | ✗ | Only redirects to provider; no auth work |
| `GET /auth/clever` | ✗ | Only redirects to provider; no auth work |
| `POST /auth/refresh` | ✗ | Already gated by a valid refresh token |
| `POST /auth/logout` | ✗ | Logout must always succeed |

## Client-facing behavior

When the bucket is empty the middleware aborts with:

```http
HTTP/1.1 429 Too Many Requests
Retry-After: 3
Content-Type: application/json

{
  "success": false,
  "error": {
    "code": "LOGIN_THROTTLED",
    "message": "Service is temporarily rate-limiting logins. Please retry shortly.",
    "retry_after": 3
  }
}
```

The `Retry-After` value is computed from the token bucket — it is the number of seconds until enough tokens refill for one request. Clients should honor it (with jitter, to avoid synchronized retry waves).

## Failure mode: fail-open on Redis errors

If the Redis call errors (timeout, connection drop, script failure), the middleware **allows the request through** rather than rejecting it. The rationale: a transient Redis problem should not convert itself into a total login outage. This trades a small window of uncapped traffic during Redis incidents for much higher availability overall.

## Observability

Prometheus metric exposed on `/metrics`:

| Metric | Type | Meaning |
|---|---|---|
| `auth_global_login_throttle_total` | Counter | Logins rejected by the global token bucket |

Pair this with the existing `auth_login_attempts_total` to compute a throttle ratio. A non-zero rate here during normal operations is a signal that capacity should be raised; a spike during an incident is the expected protective behavior.

## File reference

| File | Role |
|---|---|
| [`internal/ratelimit/global.go`](../../internal/ratelimit/global.go) | Token bucket + Lua script |
| [`internal/middleware/loginqueue.go`](../../internal/middleware/loginqueue.go) | Gin middleware, 429 response, fail-open |
| [`internal/middleware/metrics.go`](../../internal/middleware/metrics.go) | `auth_global_login_throttle_total` counter |
| [`internal/config/config.go`](../../internal/config/config.go) | `RateLimitConfig` fields |
| [`cmd/server/main.go`](../../cmd/server/main.go) | Wiring and route application |
