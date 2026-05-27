# Pre-deploy write-path smoke test

Addresses [PIR 2026-05-19](https://app.clickup.com/9014075154/v/dc/8cmfqrj-53074/8cmfqrj-87414) action item **#11** — *a write-path smoke test against prod Reservoir before enabling it in the LMS auth path would have surfaced the read-only error before any users were impacted*.

The PIR identified that a simple end-to-end check — log in once as a known test account and confirm `last_logged_on` actually advanced — would have caught the read-only `DB_HOST` misconfiguration before the school-day peak. This is that check.

## What it does

[`scripts/smoke-write-path.sh`](../../scripts/smoke-write-path.sh) performs an end-to-end write-path verification:

1. Reads the pre-test value of `reservoir_auth_db_write_errors_total` from `/metrics`.
2. Reads the pre-test `last_logged_on` for a known smoke-test account (via the LMS, not directly against the DB — same trust boundary as production traffic).
3. Sends `POST /auth/login` with the smoke-test account credentials.
4. Polls for `last_logged_on` to advance for up to 15 seconds (Reservoir's async writer flushes every 5s).
5. Re-reads `reservoir_auth_db_write_errors_total` and asserts it didn't increase.

Pass = all four checks succeed. Fail = exit code 1 with a specific message; **do not flip traffic to Reservoir until the failure is understood**.

## Relationship to the in-process write probe

These are complementary, not redundant:

| Check | When it runs | What it catches |
|---|---|---|
| [`VerifyWritable`](../../internal/database/postgres.go) (PIR #6) | At task startup, before the task joins the ALB pool | A reader-pointed `DB_HOST` at the task level. Catches misconfiguration at boot. |
| This smoke test (PIR #11) | Manually, before flipping LMS auth traffic to Reservoir | A misconfiguration that startup might miss: a writer pool that *can* write (so `VerifyWritable` passes) but where the auth-path query fails for a different reason (permissions, transaction state, replica lag affecting reads-after-writes). Also catches: the LMS↔Reservoir integration itself broken end-to-end. |

If `VerifyWritable` failed, the task wouldn't even be reachable for this script to hit. So a passing smoke implies both layers are healthy.

## When to run

- **Before flipping LMS auth-path traffic to Reservoir in any environment** (the canonical PIR #11 use case).
- After any change to Reservoir's DB connection config, pool sizing, or the `LastLoginWriter`.
- As a one-off at the start of every Reservoir-related incident response, to confirm whether the auth path is still functional.
- Optionally on a cron in non-prod, as a synthetic. The 15s deadline + a steady-state account is cheap.

## Prerequisites

### A smoke-test user account

A dedicated user account, **not** a real teacher or student, used only for this check.

- Email: e.g. `smoke-test@boddlelearning.com` (the actual email doesn't matter — pick something distinctive).
- Password: stored in SSM as `/boddle/${ENVIRONMENT}/reservoir-smoke/PASSWORD` (or however your env repo holds secrets).
- The account exists in the shared Postgres users table for the target environment. **Each environment has its own smoke-test account** — do not share an account across envs.
- A long-lived read-only LMS token for the same account, stored at `/boddle/${ENVIRONMENT}/reservoir-smoke/LMS_TOKEN`. Used by the script to read `last_logged_on` without needing DB creds.

### An LMS endpoint that returns the user's `last_logged_on`

The script's `LAST_LOGIN_CHECK_METHOD=LMS_API` (default) needs a way to read the user's `last_logged_on` via HTTP. The simplest version is an existing LMS endpoint that returns the current user's `last_logged_on` timestamp; if no such endpoint exists, add one (must require auth, must not allow looking up other users).

Fallback `LAST_LOGIN_CHECK_METHOD=SKIP` is available for cases where the LMS endpoint isn't reachable from the runner. It degrades the smoke — confirms login succeeded but not that the write actually landed. Use only when LMS is unreachable, not as the default.

## Running it

From a machine with:

- `curl`, `bash`, `awk`, `bc` available (no Go toolchain needed).
- Outbound network to the target Reservoir + LMS hosts.
- AWS CLI configured to read the smoke-test secrets from SSM.

```bash
ENVIRONMENT=staging \
RESERVOIR_BASE_URL=https://reservoir.staging.env.boddlelearning.com \
LMS_BASE_URL=https://lms.staging.env.boddlelearning.com \
SMOKE_TEST_EMAIL=smoke-test@boddlelearning.com \
SMOKE_TEST_PASSWORD=$(aws ssm get-parameter \
  --name /boddle/staging/reservoir-smoke/PASSWORD \
  --with-decryption --query Parameter.Value --output text) \
LMS_SMOKE_TOKEN=$(aws ssm get-parameter \
  --name /boddle/staging/reservoir-smoke/LMS_TOKEN \
  --with-decryption --query Parameter.Value --output text) \
./scripts/smoke-write-path.sh
```

Exit code 0 = pass. Non-zero = stop and investigate; do not flip LMS traffic.

## Interpreting failures

| Symptom | Likely cause |
|---|---|
| `Could not fetch /metrics. Is Reservoir reachable?` | DNS, security group, or the service is fully down. Not a Reservoir auth-path issue per se — check `/health` first. |
| `Login returned HTTP 5xx` | Reservoir is up but the login flow itself is broken. Tail Reservoir logs; this is what the New Relic transaction view ([`docs/OBSERVABILITY.md`](../OBSERVABILITY.md)) is for. |
| `Login returned HTTP 401` | Smoke-test account credentials are wrong or the account doesn't exist in this environment. |
| `last_logged_on did NOT advance within 15s` | **This is the 2026-05-19 signature.** The async writer is enqueuing but failing to flush — likely a read-only DB connection. Check `reservoir_auth_db_write_errors_total` (which the script will also flag in the next step), check the writer's connection string, check the reader/writer split config. |
| `reservoir_auth_db_write_errors_total increased` | DB writes are failing. Investigate before flipping traffic. |

## Cron-style synthetic in non-prod

For continuous confidence in non-prod environments, run this script every 15 minutes via a CloudWatch Event → ECS task → log to CloudWatch Logs. If three consecutive runs fail, page the on-call.

In prod, the cost-benefit is different — the smoke creates a real auth attempt every run, which inflates rate-limit counters for the smoke account and adds load. Run it manually before-deploy in prod, not on a cron.

## Adjacent runbooks

- [`POST_LAUNCH_MONITORING.md`](./POST_LAUNCH_MONITORING.md) — what to watch *after* the smoke passes and you've flipped traffic (PIR #10).
- [`ROLLBACK.md`](./ROLLBACK.md) — what to do if the smoke catches a problem *post*-deploy and you need to back out (PIR #9).
