# Post-launch monitoring checklist

Addresses [PIR 2026-05-19](https://app.clickup.com/9014075154/v/dc/8cmfqrj-53074/8cmfqrj-87414) action item **#10** — *define a post-launch monitoring checklist and assign an owner for every major deploy*.

The 2026-05-19 LMS deploy went out at 04:46 UTC. The failure manifested ~7 hours later at the start of the school-day login peak (12:00 UTC). There was no named owner watching for problems through that traffic window. This checklist exists so that doesn't happen again.

## When this applies

**Any of these is a "major deploy" for the purposes of this checklist:**

- A change to the auth path in Reservoir or the LMS (handlers, services, middleware, repository writes).
- Enabling Reservoir for additional auth methods in the LMS.
- Schema migrations on the shared Postgres.
- A change that crosses the LMS↔Reservoir boundary (e.g. new fields in the JWT, new auth endpoints called by LMS).
- A change to the rate limiter, token blacklist, or session lifecycle.
- A CFN parameter change to ECS sizing, autoscaling, or DB connection pools.
- Anything where the engineer who wrote it would describe it as "load-bearing."

If you're not sure, default to running the checklist. The cost of running it for a small change is a few hours of someone's attention; the cost of skipping it for a load-bearing change can be a P1.

## Roles per deploy

| Role | Who | What they do |
|---|---|---|
| **Deploy owner** | The engineer who shipped the change | Drives the merge → release → deploy sequence; fills out the pre-deploy section of this checklist. |
| **Monitoring owner** | A different engineer, named at deploy time | Watches the metrics in the post-launch window. Has explicit veto/rollback authority during the window. |
| **Backup monitoring owner** | A third engineer, named at deploy time | Takes over from the monitoring owner during their off-hours if the window crosses a timezone boundary. |

The monitoring owner is **not** the deploy owner — that's the point. The author of a change has motivated reasoning that biases them toward "it's working fine." A fresh pair of eyes that hasn't spent days on the change is what catches the 12:35 UTC CPU spike.

## Window

Watch for at least one full **traffic peak** after the deploy lands. For Reservoir/LMS, that's the next US-Central morning between 08:00 and 12:00 CST (~13:00–17:00 UTC, "school start"). If the deploy lands during the off-hours before that peak, the window is from deploy time through the peak.

Specifically:

- Deploy goes out at 02:00 UTC (Bogotá overnight) → window runs from 02:00 UTC through ~17:00 UTC the same day.
- Deploy goes out at 14:00 UTC (mid-school-day) → window starts immediately and runs through the *next* day's school start, since the first peak is already in flight and the second peak is the canary.
- Deploy goes out Friday afternoon US time → window runs until Monday's school start. (Don't deploy major changes Friday afternoon. This is a default; weekly cadence may need to override.)

## What to watch

The monitoring owner watches these and acknowledges them as healthy at the start, mid-point, and end of the window:

### Reservoir

| Signal | Where | Healthy state | Why we care |
|---|---|---|---|
| `/health` returning 200 with `db_writer: ok` (and `db_reader: ok` if configured) | ALB target group | All tasks healthy | Confirms the write probe ([PIR #6](../pre-release-hardening/PIR_2026_05_19_ACTION_ITEMS.md)) is happy and the reader pool is reachable. |
| `reservoir_auth_db_write_errors_total` | `/metrics` or New Relic | **Flat (rate near zero)** | If this counter is climbing, a write is failing on every request. This is the canary that would have surfaced 2026-05-19 in seconds. |
| `reservoir_last_login_dropped_total` | `/metrics` or NR | Flat or rare | Steady drops mean the async queue is overflowing — auth is faster than the DB can keep up. Investigate before rolling back. |
| `reservoir_last_login_queue_depth` | `/metrics` or NR | Near zero | Same as above, point-in-time. |
| CPU utilization per task | NR or CloudWatch | Below 50% at peak | The 2026-05-19 alert fired at 99.4% — but the warning at 70% would have been the earlier signal. |
| HTTP error rate per route | NR (when configured) | <1% | A spike on `/auth/login` or `/auth/refresh` is the headline symptom of an auth regression. |
| p95 / p99 latency per route | NR or LB metrics | At or below pre-deploy baseline | A 15× latency increase at constant error rate is what filled the LMS Puma pool in 2026-05-19 — symptomatic of slow downstream calls. |

### LMS (cross-watched even when the deploy is Reservoir-side)

| Signal | Where | Healthy state |
|---|---|---|
| Puma queue depth / thread utilization | LMS dashboards | Not climbing |
| Healthy host count on `lms-internal` / `lms-external` ELBs | CloudWatch | At desired |
| Auth-call latency from LMS to Reservoir | LMS-side logs / NR | At or below pre-deploy baseline |
| Failed-login rate | LMS logs | At or below pre-deploy baseline |

### Game / downstream

| Signal | Where | Healthy state |
|---|---|---|
| Concurrent users (CCU) | Game metrics dashboard | Tracking the day's normal curve |
| Game-API auth-related errors | Game logs | Flat |

## Cadence within the window

| Time after deploy | Owner action |
|---|---|
| **t = 0** (deploy completes) | Post in incident channel: "Reservoir/LMS deploy of `${SHA}` is live. Monitoring owner: @me. Window: ${WINDOW}." |
| **t + 5 min** | Confirm `/health` healthy on all tasks; confirm the deploy actually rolled out (image tag matches). |
| **t + 15 min** | Walk the Reservoir signal list above; no spikes, no anomalies. Post "no anomalies, t+15m" in-channel. |
| **t + 1 hour** | Re-walk both Reservoir + LMS signal lists. Post "no anomalies, t+1h." |
| **At start of peak** (e.g. 12:00 UTC for school morning) | Re-walk the full list. **This is the critical checkpoint** — 2026-05-19 manifested here, not at deploy time. |
| **During peak** | Glance at the dashboards every 15 minutes for the first hour of peak. |
| **At end of window** | Post "Window closed. Status: healthy. Handing back to normal on-call." If not healthy, this is when you decide between continued monitoring vs. rollback (see [`ROLLBACK.md`](./ROLLBACK.md)). |

## Escalation triggers

The monitoring owner has **explicit veto/rollback authority during the window.** They do not need to wait for the deploy owner to agree. If any of these fires, the response is documented:

| Trigger | Response |
|---|---|
| Reservoir CPU > 70% sustained for 5 min | Scale out per [`ROLLBACK.md` decision tree](./ROLLBACK.md). Do not roll back yet. |
| Reservoir CPU > 90% sustained for 2 min | Scale out **and** alert the deploy owner. Prepare rollback. |
| `reservoir_auth_db_write_errors_total` rate > 1/sec | Immediately page the deploy owner. This is the 2026-05-19 signature. |
| LMS healthy hosts < threshold | Immediately page the deploy owner and start the LMS rollback (LMS repo runbook). |
| User-visible failed-login rate > 2× baseline | Page; start incident channel; consider rollback. |

"Page" means: ring the deploy owner, post `@here` in the team channel, do not wait for a reply before scaling/rolling.

## Adoption

This checklist becomes real when the first deploy uses it. Recommended order of adoption:

1. The next non-trivial deploy after merging this PR uses the checklist explicitly: deploy owner posts the t=0 message; monitoring owner names themselves; cadence is followed.
2. After three deploys, the deploy owner adds "monitoring owner assigned" to the merge checklist on every PR that meets the "major deploy" criteria above.
3. After six months, audit: did any deploy in the window get a real escalation? If not, are we under-applying the checklist, or has the underlying risk shifted?

## Adjacent runbooks

- [`PRE_DEPLOY_SMOKE_TEST.md`](./PRE_DEPLOY_SMOKE_TEST.md) — run **before** flipping traffic (PIR #11). Catches configuration regressions before they hit users.
- [`ROLLBACK.md`](./ROLLBACK.md) — what to do if the window turns up a real problem (PIR #9).
- [`INCIDENT_RESPONSE_CHECKLIST.md`](./INCIDENT_RESPONSE_CHECKLIST.md) — measure-user-impact reminder per PIR #13.
