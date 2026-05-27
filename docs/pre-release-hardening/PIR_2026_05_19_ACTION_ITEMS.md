# PIR 2026-05-19 — Action Item Tracking

Code-side tracking for action items from the [Revised PIR May 2026 19 — Reservoir Auth CPU Saturation & LMS Cascade Outage](https://app.clickup.com/9014075154/v/dc/8cmfqrj-53074/8cmfqrj-87414).

The ClickUp PIR is the source of truth for *what should be done*. This document is the source of truth for *what is done in code* on the `csj/post-incident-hardening` branch (most items) and the `csj/newrelic-apm` branch (item 7 APM). The two are kept in sync manually.

**Re-deploy gate:** items 1–3 (all P0) must be done before Reservoir is re-enabled in the LMS auth path.

Last reviewed: 2026-05-27 against branches `csj/post-incident-hardening` and `csj/newrelic-apm`.

---

## P0 — Re-deploy blockers

### 1. Verify Reservoir DB connection points at the writer — **Done**

Reservoir now opens two pools — a writer pool (`DB_HOST`) and an optional reader pool (`DB_READER_HOST`) — and runs a write probe at startup that fails fast if `DB_HOST` resolves to a reader replica or a read-only role.

- Writer/reader pools: [`internal/database/postgres.go`](../../internal/database/postgres.go) — `NewPostgresDB`, `NewPostgresReaderDB`
- Config split: [`internal/config/config.go`](../../internal/config/config.go) — `DB_HOST`, `DB_READER_HOST`, `HasReader()`
- Repository routing: [`internal/user/repository.go`](../../internal/user/repository.go) — SELECTs use `r.reader`, writes use `r.db`; login-token lookup uses the writer to avoid replica lag

Operational confirmation: SSM at `/boddle/${EnvironmentName}/reservoir` must continue to set `DB_HOST` to the writer endpoint. See item 6 for the runtime check.

### 2. Move `last_logged_on` write off the synchronous auth hot path — **Done**

`LastLoginWriter` defers `last_logged_on` updates into a bounded queue flushed by a single background goroutine. `Enqueue` is non-blocking; overflow drops the ID and increments a counter rather than blocking auth.

- Implementation: [`internal/user/last_login_writer.go`](../../internal/user/last_login_writer.go)
  - Queue capacity 10,000; batch size 500; flush interval 5s; flush timeout 5s
  - Graceful shutdown drains pending IDs with a caller-supplied deadline
- Wired in: [`internal/auth/service.go`](../../internal/auth/service.go) (email/password, login-token), [`internal/oauth/service.go`](../../internal/oauth/service.go) (Google, Clever, iCloud — 5 paths)
- Lifecycle: started in [`cmd/server/main.go`](../../cmd/server/main.go), flushed before exit with a fresh 3s deadline after HTTP server shutdown
- Tests: [`internal/user/last_login_writer_test.go`](../../internal/user/last_login_writer_test.go)

Metrics:

- `reservoir_last_login_enqueued_total`
- `reservoir_last_login_dropped_total`
- `reservoir_last_login_flushed_total`
- `reservoir_last_login_batch_errors_total`
- `reservoir_last_login_queue_depth`

### 3. Fast-fail / circuit-breaker for write errors — **Done (per PIR)**

Already marked complete in the PIR. The async queue architecturally prevents write failures from blocking auth or causing cascade. The observable signal is `reservoir_auth_db_write_errors_total`, incremented from `RecordAuthDBWriteError` in [`internal/user/last_login_writer.go`](../../internal/user/last_login_writer.go) and from the login-token delete path in [`internal/auth/service.go`](../../internal/auth/service.go).

A full circuit breaker is no longer a re-deploy blocker.

---

## P1

### 4. Reduce LMS Reservoir timeout — **Done (per PIR, LMS-side)**

Already marked complete in the PIR. Lives in the LMS repo, not Reservoir.

### 5. Right-size DB connection pool per-task — **Done**

Pool sizes are env-configurable with defaults sized against the RDS writer's `max_connections` and the reader's serverless v2 minimum ACUs.

- [`internal/config/config.go`](../../internal/config/config.go):
  - `DB_MAX_OPEN_CONNS` default `25` — `floor(r7g.8xlarge_max_connections × 0.8 / max_tasks)`
  - `DB_READER_MAX_OPEN_CONNS` default `11` — `floor(serverless_v2_min_acus_max_connections × 0.8 / max_tasks)`
- Per-environment overrides via SSM
- Pool wiring: [`internal/database/postgres.go`](../../internal/database/postgres.go) — `SetMaxOpenConns`, `SetMaxIdleConns = MaxOpenConns / 2`, `SetConnMaxLifetime = 5m`, `SetConnMaxIdleTime = 10m`

### 6. Write-capable health probe — **Done**

`VerifyWritable` runs a zero-row `UPDATE users SET last_logged_on = last_logged_on WHERE id = -1` inside a transaction that is rolled back. The predicate is impossible (positive serial), but Postgres still evaluates write permission and surfaces `cannot execute UPDATE in a read-only transaction` before the task joins the ALB pool.

- Implementation: [`internal/database/postgres.go`](../../internal/database/postgres.go) — `VerifyWritable`
- Startup invocation: [`cmd/server/main.go`](../../cmd/server/main.go) — runs with a 5s context, `logger.Fatal` on failure before HTTP server starts
- Runtime check: `/health` reports `db_writer` and (when configured) `db_reader` independently, always HTTP 200 so ALB doesn't kill tasks on a transient blip — see [`internal/auth/handler.go`](../../internal/auth/handler.go)

### 7. APM + structured error logging — **Done in code (operational rollout pending)**

Two pieces, both addressed:

- **Structured logging** (on `csj/post-incident-hardening`): `fmt.Printf` has been removed from every auth path, replaced with `zap` across `internal/auth`, `internal/oauth`, and the `LastLoginWriter`. The stdout-mutex contention path identified in the PIR is gone.
- **New Relic APM** (separate branch `csj/newrelic-apm`, [PR #13](https://github.com/boddle-learning/reservoir/pull/13)): `nrgin` middleware wraps the Gin router; the `nrpostgres` driver wraps the sqlx pool so each query becomes a datastore segment under the surrounding HTTP transaction. Disabled when `NEW_RELIC_LICENSE_KEY` is empty so dev/CI boot identically to prod. See [`docs/OBSERVABILITY.md`](../OBSERVABILITY.md) (lands with PR #13) for the full operational story.

**Remaining operational step:** set `NEW_RELIC_LICENSE_KEY` in SSM at `/boddle/${EnvironmentName}/reservoir/NEW_RELIC_LICENSE_KEY` per environment. This is config, not code, and is out of scope for this branch. Until done, the agent runs disabled — no APM data shows up in New Relic, but the service still boots normally.

### 8. Profile Reservoir CPU under realistic auth load — **Runbook ready (profile run pending)**

Runbook at [`docs/operations/CPU_PROFILING.md`](../operations/CPU_PROFILING.md) covers when to run, how to expose pprof on a non-public port, how to capture CPU/heap/goroutine profiles, what to look for in the top-N (with a contributor-attribution table mapping symbols to PIR-identified causes), and where to archive results.

**Still outstanding:** actually run the profiling exercise against a representative auth load and archive results under `docs/pre-release-hardening/profiles/`. The runbook calls out the dev-environment prerequisites (user table seeding, full auth-path mix in the load test).

### 9. Document and test rollback path — **Runbook ready (dry-run pending)**

Runbook at [`docs/operations/ROLLBACK.md`](../operations/ROLLBACK.md) covers: who can execute (with a quarterly access-verification check baked in), a decision tree for whether to roll back Reservoir vs the LMS vs scale out first, step-by-step Reservoir rollback (with the explicit "communicate before executing" two-executor checkpoint the PIR called for), recovery signals to watch, and a quarterly practice cadence.

**Still outstanding:** identify the primary and backup executors by name and run the first quarterly practice. The runbook is structured so a stranger to the system could follow it under pressure, but it needs to actually be run end-to-end once before the next prod deploy.

### 10. Post-launch monitoring checklist — **Checklist ready (adoption pending)**

Checklist at [`docs/operations/POST_LAUNCH_MONITORING.md`](../operations/POST_LAUNCH_MONITORING.md) defines what counts as a major deploy, the three named roles (deploy owner / monitoring owner / backup), the window definition (must cover the next traffic peak — the 2026-05-19 failure didn't surface until ~7h after deploy at the school-morning peak), what to watch with concrete metric names, a time-after-deploy cadence with explicit checkpoints, and escalation triggers with pre-authorized rollback authority for the monitoring owner.

**Still outstanding:** adopt on the next non-trivial deploy and add "monitoring owner assigned" to the PR merge checklist after three trial deploys.

### 11. Write-path smoke test against prod Reservoir before enabling — **Script + runbook ready (first run pending)**

Script at [`scripts/smoke-write-path.sh`](../../scripts/smoke-write-path.sh) and runbook at [`docs/operations/PRE_DEPLOY_SMOKE_TEST.md`](../operations/PRE_DEPLOY_SMOKE_TEST.md). The script: reads pre-test metrics + `last_logged_on`, hits `POST /auth/login`, polls for `last_logged_on` to advance within 15s (Reservoir's async writer flush interval), and asserts `reservoir_auth_db_write_errors_total` didn't increase. Pass = exit 0; fail = exit 1 with a specific symptom. Complementary to PIR #6's in-process `VerifyWritable` probe — startup probe catches reader-pointed `DB_HOST` at boot; this smoke catches anything that probe might miss plus the LMS↔Reservoir integration end-to-end.

**Still outstanding:** create the dedicated smoke-test user account in each environment, provision the SSM secrets (`/boddle/${ENVIRONMENT}/reservoir-smoke/PASSWORD` + `LMS_TOKEN`), confirm an LMS endpoint exposes the current user's `last_logged_on` (or add one), and run the smoke once against the next pre-deploy.

### 12. Audit production autoscaling configuration — **Done**

CloudFormation defaults updated so a new environment cannot ship without autoscaling.

- [`.cloudformation/reservoir.cfhighlander.rb`](../../.cloudformation/reservoir.cfhighlander.rb):
  - `DesiredCount`: 1 → 2
  - `MinCount`: 1 → 2
  - `MaxCount`: 1 → 8 (matches prod1 override; incident relief required ~16)
  - `EnableScaling`: 'false' → 'true'
  - `Cpu`: 4096 (4 vCPU), `Memory`: 8192 (Fargate minimum at 4 vCPU)

The inline comment cites the 2026-05-19 outage so the rationale survives future edits. Prod overrides should still be confirmed for any environment expecting school-day peak load.

---

## P2

### 13. Measure user impact for this incident — **Open**

Failed-login rate from LMS logs during 12:35–13:30 UTC has not been measured. Also needs "measure users affected" added to the standard incident response checklist.

### 14. Read replica usage in dev — **Open**

Dev environment configuration not yet updated.

### 15. New Relic accounts for all LMS devs — **Done (per PIR)**

Already marked complete in the PIR.

### 16. Environment-repo access for all LMS devs — **Done (per PIR)**

Already marked complete in the PIR.

---

## Summary

| Status | Count | Items |
|---|---|---|
| Done in code | 10 | 1, 2, 3, 4, 5, 6, 7, 12, 15, 16 |
| Open | 6 | 8, 9, 10, 11, 13, 14 |

Re-deploy gates 1–3 are all done. The remaining work is operational hardening (profiling, runbooks, monitoring, the per-env New Relic license key) rather than code that blocks re-enabling Reservoir in the LMS auth path.

**"Done in code" vs "Done in production":** the items above are done on branch — [PR #12](https://github.com/boddle-learning/reservoir/pull/12) for items 1, 2, 3, 5, 6, 12 and [PR #13](https://github.com/boddle-learning/reservoir/pull/13) for the APM half of item 7. They become done in production when those PRs merge and the relevant SSM values land in each environment.
