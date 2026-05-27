# Observability

Reservoir emits three streams of telemetry: **New Relic APM** for HTTP and database traces, **Prometheus metrics** at `/metrics`, and **structured logs** via `zap` to stdout.

This document covers configuration, what each stream captures, and operational guidance. The motivation for adding APM is documented in [PIR 2026-05-19](https://app.clickup.com/9014075154/v/dc/8cmfqrj-53074/8cmfqrj-87414) — Reservoir's CPU saturated to 99.4% during a production outage and the failing `UPDATE last_logged_on` query was invisible until logs were combed by hand.

---

## New Relic APM

### What gets captured

- **HTTP transactions** — one per request, named after the matched Gin route (e.g. `POST /auth/login`). Captures status code, latency, throughput, error rate, distributed-trace metadata.
- **Database segments** — every query executed within a request context becomes a datastore segment under the surrounding HTTP transaction. Captures query duration, error class (including the `pq: cannot execute UPDATE in a read-only transaction` error that drove the 2026-05-19 incident), and connection-pool wait time.

### What is not captured (yet)

- **Redis operations** — no `nrredis` integration. Add `github.com/newrelic/go-agent/v3/integrations/nrredis-v9` and wrap the `redis.Client` if this becomes important.
- **Custom segments** (bcrypt verification, JWT generation, token blacklist checks) — no manual instrumentation. Add via [`newrelic.FromContext(ctx).StartSegment("name")`](https://docs.newrelic.com/docs/apm/agents/go-agent/instrumentation/instrument-go-segments/) where useful.
- **Health probe** — `Health` runs queries with `context.Background()` rather than a request context. This is intentional so health checks don't pollute the transaction count.

### Configuration

| Env var | Required | Default | Notes |
|---|---|---|---|
| `NEW_RELIC_LICENSE_KEY` | No | empty | Empty = agent disabled (dev/local). Set in SSM at `/boddle/${EnvironmentName}/reservoir/NEW_RELIC_LICENSE_KEY` per environment. |
| `NEW_RELIC_APP_NAME` | No | `reservoir` | Override per-env (e.g. `reservoir-prod1`, `reservoir-staging`) to distinguish accounts in the NR UI. |

When `NEW_RELIC_LICENSE_KEY` is empty:

- The agent initializes with `ConfigEnabled(false)`.
- `nrgin.Middleware` is still installed but emits no transactions.
- The `nrpostgres` driver is still registered and used, but each query is a transparent passthrough to `lib/pq` — no segments emitted.
- No connection to New Relic is attempted; no errors are logged.

This means **dev and CI environments boot identically to prod from a code perspective** — the only difference is the env var.

### How it integrates

```
Request → nrgin.Middleware ──┐
                             │  (creates txn, attaches to c.Request.Context())
                             ↓
                          Handler ──→ Service ──→ Repository
                                                       │
                                                       ↓
                                          sqlx.DB (nrpostgres driver)
                                                       │
                                                       ↓
                                          ExecContext(c.Request.Context(), ...)
                                                       │
                                                       ↓
                                          Datastore segment attached to txn
```

Two things make this work without per-call code:

1. `nrgin.Middleware` puts the New Relic transaction onto the request context.
2. `nrpostgres` (registered as the `"nrpostgres"` driver via the blank import in [`internal/database/postgres.go`](../internal/database/postgres.go)) inspects the context passed to every `*Context` method on the DB. If a transaction is present, the query becomes a segment.

Handlers must therefore use `c.Request.Context()` when calling repository methods — they already do.

### Per-environment rollout

The agent is **off by default**. To enable for an environment:

1. Add `NEW_RELIC_LICENSE_KEY` to SSM at `/boddle/${EnvironmentName}/reservoir/NEW_RELIC_LICENSE_KEY`.
2. (Optional) Set `NEW_RELIC_APP_NAME` to a per-env value, e.g. `reservoir-prod1`.
3. Restart the ECS service. On boot, look for:

   ```
   {"level":"info","msg":"Connected to New Relic","app":"reservoir-prod1"}
   ```

   If you see this instead, the license is set but New Relic is unreachable — the service still boots:

   ```
   {"level":"warn","msg":"New Relic agent did not connect within deadline; continuing"}
   ```

4. In the NR UI, the app should appear under **APM & Services** within 1–2 minutes of the first request.

### Troubleshooting

- **No data appearing in NR**: Confirm `NEW_RELIC_LICENSE_KEY` is set in the running task (`echo $NEW_RELIC_LICENSE_KEY` in an exec), confirm the boot log shows `Connected to New Relic`, confirm the app name in NR matches `NEW_RELIC_APP_NAME`.
- **Queries not showing as segments under HTTP transactions**: A handler is using `context.Background()` or `context.TODO()` instead of `c.Request.Context()`. Grep for these in handler files.
- **App appears but no DB segments**: Confirm the driver name in [`internal/database/postgres.go`](../internal/database/postgres.go) is `"nrpostgres"`, not `"postgres"`. The blank import of `_ "github.com/newrelic/go-agent/v3/integrations/nrpq"` must also be present — its `init()` registers the driver.

### Versions

Pinned to:

- `github.com/newrelic/go-agent/v3 v3.40.1` — last version with `go 1.22` directive (newer versions require `go 1.25`, which would force a Dockerfile bump from `golang:1.22-alpine`).
- `github.com/newrelic/go-agent/v3/integrations/nrgin v1.3.1`
- `github.com/newrelic/go-agent/v3/integrations/nrpq v1.1.1`

If the Dockerfile is later bumped to a newer Go version, all three can be moved to `@latest`.

---

## Prometheus metrics

Reservoir exposes `/metrics` for Prometheus scraping. Metrics live next to the code that emits them. Notable counters:

| Metric | Source | Purpose |
|---|---|---|
| HTTP request count / duration / status | [`internal/middleware/metrics.go`](../internal/middleware/metrics.go) | Standard request-level telemetry, also covered by NR HTTP transactions. |

Additional counters are introduced on the `csj/post-incident-hardening` branch (`reservoir_auth_db_write_errors_total`, `reservoir_last_login_*`) and will land with that work.

Prometheus and New Relic are complementary, not redundant: Prometheus is the canonical surface for alerting rules and Grafana dashboards; New Relic is the canonical surface for transaction-level drilldown and DB query introspection. Don't move alerting to New Relic — keep alert routing in Prometheus/Grafana so on-call paging stays in one system.

---

## Structured logs

Logging is via [`zap`](https://github.com/uber-go/zap). The logger is configured in [`cmd/server/main.go`](../cmd/server/main.go):

- Development: `zap.NewDevelopment()` — human-readable, colorized.
- Production: `zap.NewProduction()` — JSON, suitable for log aggregators.

All `fmt.Printf` error reporting was removed in the post-incident hardening work — the `fmt.Printf` stdout-mutex contention path was identified as a contributor to the 2026-05-19 CPU saturation. Every code path now uses `zap` with structured fields.

Sensitive value redaction is a known gap (see the security review at [`docs/pre-release-hardening/reservoir-security-review.md`](./pre-release-hardening/reservoir-security-review.md), Finding 3 — `/auth/token?token=SECRET` is currently logged in plaintext).
