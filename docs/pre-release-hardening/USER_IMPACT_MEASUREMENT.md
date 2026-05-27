# Measuring user impact for an auth incident

Addresses [PIR 2026-05-19](https://app.clickup.com/9014075154/v/dc/8cmfqrj-53074/8cmfqrj-87414) action item **#13** — the 2026-05-19 PIR carried *"users affected: not measured at time of writing"* as a finding. This template makes the measurement repeatable so the next PIR doesn't have to.

## Goal

For an auth incident with a known start time and recovery time, produce a defensible count of:

- **Users who attempted to log in during the window** (the denominator — "how many users were exposed to the problem").
- **Users who failed at least one auth attempt during the window** (the numerator — "how many users hit the bug").
- **Users who failed and never succeeded within the window** (the residual — "how many users gave up or stayed locked out").

These three numbers together let the PIR author write *"~N users attempted to log in during the window; M failed at least once; K never recovered within the incident"* with confidence rather than waving at a CCU graph.

## When to run

Within **24 hours** of incident recovery, per [`docs/operations/INCIDENT_RESPONSE_CHECKLIST.md`](../operations/INCIDENT_RESPONSE_CHECKLIST.md) step 8. Waiting longer risks log rotation, especially for high-volume LB access logs.

## Where the data lives

For Boddle's stack, the canonical sources in priority order:

| Source | Has | Doesn't have |
|---|---|---|
| **LMS Rails logs** (CloudWatch Logs group `lms-prod-app`, retention 30 days) | Every `POST /sessions` / `POST /api/login` attempt with timestamp, IP, user-agent, response status; failure reasons are logged with structured fields | Doesn't know what happened inside Reservoir |
| **Reservoir logs** (CloudWatch Logs group `reservoir-prod`, retention 30 days; structured `zap` output as of the post-incident-hardening branch) | Every `/auth/login`, `/auth/refresh`, `/auth/token`, OAuth callback with timestamp, email (hashed/normalized), outcome | Existed but had `fmt.Printf` noise on 2026-05-19; structured logging is what makes this cheaper to query going forward |
| **`login_attempts` Postgres table** | Every login attempt (success + failure) with email, IP, timestamp, success boolean. This is the durable record. | Doesn't break down by auth method (no `method` column); doesn't include OAuth callbacks where the user never even hits `/login` |
| **New Relic** (when configured per [`docs/OBSERVABILITY.md`](../OBSERVABILITY.md), lands with PR #13) | Per-transaction error rate and latency, aggregated by route | Aggregates only; doesn't tell you *which* users were affected |

For attempt-level counts, use the `login_attempts` table — it's the durable source of truth. For breakdown by auth method, supplement with log queries.

## Query template — `login_attempts` table

Run from a read-only DB role (or via the LMS's read endpoint if direct DB access isn't available).

```sql
-- Replace these for the incident window. Use UTC.
\set window_start '2026-05-19 12:35:00+00'
\set window_end   '2026-05-19 13:30:00+00'

-- Q1. Unique users who attempted to log in during the window.
SELECT count(DISTINCT email) AS users_attempted
FROM login_attempts
WHERE attempted_at >= :'window_start'
  AND attempted_at <  :'window_end';

-- Q2. Unique users who failed at least one attempt during the window.
SELECT count(DISTINCT email) AS users_with_a_failure
FROM login_attempts
WHERE attempted_at >= :'window_start'
  AND attempted_at <  :'window_end'
  AND success = false;

-- Q3. Unique users who failed and never succeeded within the window.
WITH window_attempts AS (
  SELECT email, success
  FROM login_attempts
  WHERE attempted_at >= :'window_start'
    AND attempted_at <  :'window_end'
)
SELECT count(*) AS users_never_recovered
FROM (
  SELECT email
  FROM window_attempts
  GROUP BY email
  HAVING bool_or(success) = false
) t;

-- Q4. Sanity check — total attempts (not unique).
SELECT
  count(*) AS total_attempts,
  count(*) FILTER (WHERE success = true)  AS successful_attempts,
  count(*) FILTER (WHERE success = false) AS failed_attempts
FROM login_attempts
WHERE attempted_at >= :'window_start'
  AND attempted_at <  :'window_end';

-- Q5. Time-bucketed failure rate, useful for the PIR timeline.
SELECT
  date_trunc('minute', attempted_at) AS minute,
  count(*) AS attempts,
  count(*) FILTER (WHERE success = false) AS failures,
  round(100.0 * count(*) FILTER (WHERE success = false) / nullif(count(*), 0), 1) AS failure_pct
FROM login_attempts
WHERE attempted_at >= :'window_start' - interval '30 minutes'
  AND attempted_at <  :'window_end'   + interval '30 minutes'
GROUP BY 1
ORDER BY 1;
```

Q5's ± 30-minute padding is deliberate — the PIR timeline benefits from showing the failure rate *before* the incident (baseline) and *after* recovery (confirmed-clean), not just during the window.

## Query template — CloudWatch Logs Insights (LMS, breakdown by method)

For breaking failures down by auth method, since `login_attempts` doesn't carry that field:

```
fields @timestamp, @message
| filter @timestamp >= '2026-05-19T12:35:00Z'
| filter @timestamp <  '2026-05-19T13:30:00Z'
| filter @message like /auth_attempt/
| parse @message /"method":"(?<method>[^"]+)".*"outcome":"(?<outcome>[^"]+)"/
| stats count(*) as attempts, count_if(outcome = 'failure') as failures by method
```

This depends on the LMS logging an `auth_attempt` structured event with `method` and `outcome` fields. If the LMS doesn't yet, that's a follow-up — add the structured logging on the LMS side before the next deploy.

## Defining "affected"

A user is **affected** if they attempted to log in during the window and at least one attempt failed. This is deliberately liberal:

- A user who retried successfully on the third attempt is still affected — they hit the bug.
- A user who got locked out by the rate limiter as a knock-on of the underlying failure is affected.
- A user who tried to log in and got an error page that they then closed is affected, even if no retry exists in logs.
- A user who *would have* logged in but didn't try (because the LMS UI was unreachable) is **not** counted here. That's a separate "who didn't even get to attempt" measurement, much harder, usually estimated from CCU drop or signup-funnel data.

The PIR should distinguish these — "M users were affected (failed ≥ 1 attempt during the window); CCU dropped by X during the same period, suggesting another ~Y users may have been blocked without attempting."

## Worked example: 2026-05-19

The 2026-05-19 PIR did not measure user impact. Running these queries against the historical `login_attempts` data should produce a defensible number. **TBD — populate this section once the queries have been run.** Tracker: PIR #13 in [`PIR_2026_05_19_ACTION_ITEMS.md`](./PIR_2026_05_19_ACTION_ITEMS.md).

When populated, this section should read like:

> Window: 2026-05-19 12:35–13:30 UTC (55 minutes).
> Users who attempted to log in during the window: N
> Users with at least one failed attempt: M
> Users who never recovered within the window: K
> Peak failure rate: P% at HH:MM UTC
> Breakdown by method: email/password X%, Google Y%, Clever Z%, iCloud W%, magic-link V%

Once populated, link from the PIR ClickUp doc back to this measurement so a future reader sees the data, not just the conclusion.

## Saving the measurement

Once run for a specific incident, archive the results next to the PIR doc:

```
docs/pre-release-hardening/incident-impact/2026-05-19/
  query-results.csv     -- raw output from the SQL above
  summary.md            -- the human-readable summary, structured like the worked example
  cloudwatch-insights.png  -- screenshot of the Logs Insights timeline (optional)
```

Reference the directory from the PIR's Impact section so the next person looking at the PIR sees that the affected-user count was measured, where, and by whom.
