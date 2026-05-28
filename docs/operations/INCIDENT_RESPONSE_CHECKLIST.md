# Incident response checklist

Per [PIR 2026-05-19](https://app.clickup.com/9014075154/v/dc/8cmfqrj-53074/8cmfqrj-87414) action item **#13**, "measure users affected" is now a standard step in incident response — the 2026-05-19 PIR explicitly noted the affected-user count was *"not measured at time of writing"*, which left the severity classification and any external comms as estimates rather than data.

This checklist is the running playbook for any P1/P2 incident touching the LMS, Reservoir, or the game.

## During the incident

| Step | Owner | Notes |
|---|---|---|
| 1. Open an incident channel | First responder | Name it `incident-YYYY-MM-DD-<short>`. Pin the running summary. |
| 2. Name an Incident Commander (IC) | First responder, then handed off | The IC coordinates; they don't necessarily fix. One person, named explicitly. |
| 3. Note start time and observed symptoms | IC | UTC. This is the canonical time everything else references. |
| 4. Mitigate before diagnosing | Whoever has the tool | Scale out, roll back, take a feature flag off. Diagnosis can happen with the service running. |
| 5. Cross-check the [`ROLLBACK.md`](./ROLLBACK.md) decision tree | IC | "Should we roll back?" is a decision, not a reflex. |
| 6. Record every meaningful action in-channel with a timestamp | Whoever takes the action | "Juan scaling Reservoir 8 → 16 at 12:42 UTC" — same shape as the 2026-05-19 timeline. |

## At the moment of mitigation

| Step | Owner | Notes |
|---|---|---|
| 7. Note the recovery time | IC | UTC. The window between Step 3 and Step 7 is the incident duration; everything else gets measured against it. |
| 8. **Measure users affected during the window** | IC or a designated engineer | See [User impact measurement](#user-impact-measurement) below. **Do this within 24 hours, not at PIR-writing time.** This is the PIR #13 reminder — 2026-05-19 missed this and the PIR carried an "unknown" affected count as a result. |
| 9. Confirm no residual issues | IC | "Recovered" is more than just metrics returning to baseline — confirm secondary effects (queue backlog, retry storms, downstream services that piled up requests) have also drained. |

## Within 24 hours

| Step | Owner | Notes |
|---|---|---|
| 10. Snapshot the incident channel | IC | Export the full transcript. Slack canvas, plain text, whatever survives the channel being archived. |
| 11. File the affected-user count | IC | Add to a running issue / Linear ticket so the PIR draft has the number ready. |
| 12. Identify the on-call rotation impact | IC | Who paged, when, how many escalations. Useful for the PIR's "what went well" section. |

## Within 7 days

| Step | Owner | Notes |
|---|---|---|
| 13. Draft the PIR | IC or an engineer involved | Template lives in ClickUp; mirror the 2026-05-19 PIR's structure (Summary, Timeline, Confirmed Failure Mode, Cascade Mechanism, Impact, Action Items, What Went Well). |
| 14. Identify action items | PIR author + team | Each item has an owner, a priority, and a tracker ID. No "we should think about" items — every item must be executable. |
| 15. Review the PIR with the whole team | Team lead | Sign-off step. Action items go into the next sprint. |

## User impact measurement

This is the explicit PIR #13 step. **The 2026-05-19 incident missed this and the PIR carried an "unknown" user count as a result.**

For incidents touching auth specifically (Reservoir / LMS auth path), see the dedicated template at [`docs/pre-release-hardening/USER_IMPACT_MEASUREMENT.md`](../pre-release-hardening/USER_IMPACT_MEASUREMENT.md). It includes:

- The exact log-grep / SQL query templates for counting failed logins by auth method during a time window.
- A worked example against the 2026-05-19 12:35–13:30 UTC window so the next operator has a reference point.
- The "what counts as affected" definitional rules — a user who retried successfully within the window is still affected.

For non-auth incidents (game outages, LMS internal degradation, etc.), the same idea applies: produce a defensible number for "how many users tried to do the thing and couldn't" using whichever logs/metrics cover the affected surface.

## What this checklist is NOT

- **It is not a substitute for the IC's judgment.** The order above is the default — skip or reorder when the incident demands it.
- **It is not a status template.** The incident channel summary is the status surface; this is the to-do list behind it.
- **It is not finished.** Add to it whenever a real incident turns up a missing step. The 2026-05-19 incident contributed step 8; future incidents will contribute their own.

## Adjacent runbooks

- [`ROLLBACK.md`](./ROLLBACK.md) — what to do when the answer is "roll back" (PIR #9).
- [`POST_LAUNCH_MONITORING.md`](./POST_LAUNCH_MONITORING.md) — the structured post-deploy watch that often *prevents* incidents (PIR #10).
- [`../pre-release-hardening/USER_IMPACT_MEASUREMENT.md`](../pre-release-hardening/USER_IMPACT_MEASUREMENT.md) — the auth-specific measurement template referenced in step 8.
