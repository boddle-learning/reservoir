# Reservoir rollback runbook

Addresses [PIR 2026-05-19](https://app.clickup.com/9014075154/v/dc/8cmfqrj-53074/8cmfqrj-87414) action item **#9** — *document and test a rollback path; ensure at least two people have access to execute it without depending on one person*.

This document covers rolling back **Reservoir** (this repo). For rolling back the **LMS**, see the LMS repo's rollback runbook — during the 2026-05-19 incident the actual recovery was an LMS rollback (PR `554262a` reverted), not a Reservoir rollback, because that's what removed Reservoir from the auth path entirely.

## Who can execute this

Both of the following must be true at all times. If only one person on either row is available, that's a single-person-dependency situation; on-call should escalate to staff up.

| Role | Capability needed | Verify quarterly |
|---|---|---|
| **Primary executor** | AWS CLI access with the `reservoir-deploy` role assumed; ECR pull; CloudFormation update on the env stack | `aws sts get-caller-identity` in the target env returns the deploy role |
| **Backup executor** | Same as Primary, on a different machine, by a different person | Same check, different shell |

The quarterly verification is a deliberate step — access decays silently (rotated SSO tokens, expired role trust, people changing teams). If the backup hasn't run the verification command in 90 days, treat them as unavailable until they do.

## Decision tree — what to roll back

Use this when an incident points at the Reservoir auth path. Read top-down; the first match wins.

1. **Reservoir is returning 5xx on `/health` or auth endpoints** → roll back **Reservoir** to the previous green release. Section *Rolling back Reservoir* below.
2. **Reservoir is healthy but CPU is saturated and auth latency is spiking** → first try **scaling out** (raise `MaxCount`); rollback is only correct if you have evidence of a regression in the most recent Reservoir release. PIR 2026-05-19 was *not* solved by rolling back Reservoir — the actual recovery was an LMS rollback.
3. **LMS is degraded and the most recent change is a deploy that wired/changed Reservoir in the auth path** → roll back the **LMS** (separate runbook). Reservoir keeps running; the LMS just stops calling it.
4. **Neither service had a recent deploy but something is broken** → not a rollback situation. Open an incident channel, gather signals, then decide.

When in doubt, **scale out before you roll back.** Scaling is reversible; a bad rollback can amplify the incident.

## Rolling back Reservoir

This rolls back the ECS service to the previous container image. The CloudFormation stack itself doesn't change — only the `appTaskVersion` parameter that points it at the previous tag in ECR.

### Step 0 — Decide the rollback target

The rollback target is the **last green release tag** before the suspect deploy. Find it:

```bash
# Show recent Reservoir releases (tags + their SHAs)
gh release list --repo boddle-learning/reservoir --limit 10

# Show what's currently running in the target env
aws ssm get-parameter \
  --name /boddle/${ENVIRONMENT_NAME}/reservoir/IMAGE_TAG \
  --region us-east-1 \
  --query 'Parameter.Value' --output text
```

The target is the version one *before* what's running, unless you have specific knowledge that an earlier release is the right one.

**Write down the rollback target before executing.** This is a checkpoint — if you can't articulate "from $X to $Y," you don't have a rollback plan yet.

### Step 1 — Verify the image exists in ECR

```bash
aws ecr describe-images \
  --repository-name boddle-learning/reservoir \
  --image-ids imageTag=$ROLLBACK_TARGET \
  --region us-east-1
```

If this fails, the image was garbage-collected. Stop here — you'll need to rebuild from the source tag (`make build-app && make build-container VERSION=$ROLLBACK_TARGET`) before continuing.

### Step 2 — Communicate before executing

Post to the incident channel:

```
[ROLLBACK] Reservoir ${ENVIRONMENT_NAME}
  Current:  ${CURRENT_TAG}
  Target:   ${ROLLBACK_TARGET}
  Executor: @me
  Backup:   @backup-person
```

Both primary and backup acknowledge in-channel before the executor runs the rollback. This is the "two-executor" check from the PIR — not two people typing, but two people having confirmed the plan.

### Step 3 — Execute

```bash
cd boddle-environment-${ENVIRONMENT_NAME}/reservoir
# Edit the version parameter (or however the env repo pins it):
#   reservoir_version: ${ROLLBACK_TARGET}
git add . && git commit -m "rollback reservoir to ${ROLLBACK_TARGET}: ${INCIDENT_REF}"
git push
# The env-repo deploy pipeline picks this up automatically.
```

Alternatively, for an emergency where the env-repo pipeline is the bottleneck, update the CloudFormation stack directly:

```bash
make cf-publish VERSION=$ROLLBACK_TARGET ENVIRONMENT_NAME=$ENVIRONMENT_NAME
```

Note that bypassing the env repo means the next env-repo deploy will overwrite the rollback. Always follow up with an env-repo PR within the same hour.

### Step 4 — Verify recovery

Watch for these signals — all should improve within ~3 minutes:

| Signal | Where | Healthy state |
|---|---|---|
| ECS task count at desired | AWS console → ECS → reservoir service | `running` matches `desired` |
| `/health` returns 200 | ALB target group health check | All tasks `healthy` |
| Reservoir CPU | New Relic (when configured) or CloudWatch metric `CPUUtilization` | Below 50% sustained |
| `reservoir_auth_db_write_errors_total` | `/metrics` or NR | Flat (not increasing) |
| Auth latency p95 | LMS-side logs or NR | At or below pre-incident baseline |

If any signal stays bad after 5 minutes: **the rollback didn't fix it.** Escalate; don't roll back again to an even-earlier version without staff support.

### Step 5 — Close out

- Post-rollback summary to the incident channel (target reached, time-to-recovery, residual issues).
- File a post-incident issue in this repo titled `rollback follow-up: <date>`, with: what triggered the rollback, what specific change caused the regression, what the prevention should be.
- The rolled-back code stays in `main` until the regression is understood — don't revert in-repo immediately. The release pipeline knowing it's running an older tag is enough for now.

## Practice cadence

Once per quarter, both executors:

1. Pick a non-prod environment.
2. Pick a fictitious "rollback target" (one release behind what's there).
3. Run Steps 1–4 end to end.
4. Verify recovery signals.
5. Roll forward to what was there before.
6. Update this runbook with anything that surprised you.

A practice that turns up no surprises is itself a finding — write down what you confirmed so the next person knows it's been checked.

## Adjacent runbooks

- [`POST_LAUNCH_MONITORING.md`](./POST_LAUNCH_MONITORING.md) — what to watch immediately after a deploy (PIR #10). A bad deploy caught at the post-launch window often avoids needing this rollback runbook at all.
- [`PRE_DEPLOY_SMOKE_TEST.md`](./PRE_DEPLOY_SMOKE_TEST.md) — write-path smoke test (PIR #11). Catches a misconfigured `DB_HOST` before traffic shifts.
- LMS rollback runbook (in the LMS repo) — for LMS-side rollbacks during a Reservoir-related incident.
