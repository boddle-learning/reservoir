# Configuring the DB reader replica per environment

Addresses [PIR 2026-05-19](https://app.clickup.com/9014075154/v/dc/8cmfqrj-53074/8cmfqrj-87414) action item **#14** — *ensure usage of read replica in dev*.

Reservoir's writer/reader pool split is implemented in [`internal/database/postgres.go`](../../internal/database/postgres.go) (lands with [PR #12](https://github.com/boddle-learning/reservoir/pull/12)). When `DB_READER_HOST` is unset, the writer handle is reused for reads — same observable behavior as production when the replica is misconfigured. Dev should exercise the reader path so regressions surface there, not in prod. This runbook is how each environment gets configured.

## What changes in code

Nothing. The split is already done. This runbook is purely about per-environment **SSM configuration** that activates the reader path.

## The env-var contract

| Env var | Required? | What it does |
|---|---|---|
| `DB_HOST` | Yes | The writer endpoint. The startup [`VerifyWritable`](../../internal/database/postgres.go) probe fails the task if this resolves to a reader. |
| `DB_READER_HOST` | No | The reader-replica endpoint. When set, `internal/user/repository.go` routes all SELECTs through the reader pool. When unset, reads fall back to the writer (degraded mode — fine functionally, but doesn't exercise the split). |
| `DB_MAX_OPEN_CONNS` | No (default 25) | Writer pool size per task. |
| `DB_READER_MAX_OPEN_CONNS` | No (default 11) | Reader pool size per task. |

For the full sizing math see [`docs/CAPACITY_PLANNING.md`](../CAPACITY_PLANNING.md) and the comments in [`internal/config/config.go`](../../internal/config/config.go).

## Setting `DB_READER_HOST` in dev

1. **Identify the reader endpoint.** For a single RDS cluster, this is the `*.cluster-ro-*.rds.amazonaws.com` hostname (the `-ro-` is the giveaway). For an Aurora Serverless v2 deployment, the reader endpoint is a separate CloudFormation export named like `${EnvironmentName}-rds-ReaderEndpoint`. Confirm with:

   ```bash
   aws rds describe-db-cluster-endpoints \
     --db-cluster-identifier "$ENVIRONMENT_NAME-reservoir-cluster" \
     --region us-east-1 \
     --query "DBClusterEndpoints[].{Type:EndpointType,Endpoint:Endpoint}"
   ```

2. **Set the SSM parameter:**

   ```bash
   aws ssm put-parameter \
     --name "/boddle/dev/reservoir/DB_READER_HOST" \
     --type String \
     --value "$READER_ENDPOINT" \
     --region us-east-1 \
     --overwrite
   ```

3. **(Optional) override the reader pool size** for environments where the default `11` doesn't fit the replica's `max_connections`:

   ```bash
   aws ssm put-parameter \
     --name "/boddle/dev/reservoir/DB_READER_MAX_OPEN_CONNS" \
     --type String \
     --value "5" \
     --region us-east-1 \
     --overwrite
   ```

   The default (11) is sized for serverless v2 minimum ACUs. Smaller dev environments can go lower.

4. **Redeploy or restart the dev Reservoir task** so it picks up the new env. On boot, look for:

   ```
   {"level":"info","msg":"Connected to PostgreSQL writer"}
   {"level":"info","msg":"Connected to PostgreSQL reader replica","host":"<reader>"}
   {"level":"info","msg":"Database write probe passed"}
   ```

   All three lines must appear. If only the writer line appears, `DB_READER_HOST` didn't make it into the running env — confirm SSM was read at boot.

## Verification

`/health` should now report both `db_writer` and `db_reader`:

```bash
curl -s https://reservoir.dev.env.boddlelearning.com/health | jq
```

Expected response:

```json
{
  "status": "healthy",
  "db_writer": "ok",
  "db_reader": "ok"
}
```

If `db_reader` is missing entirely, the env var isn't set (or didn't load). If `db_reader: "error"` appears, the reader endpoint is configured but the connection is broken — check security groups, credentials (the reader shares writer credentials), and `DB_PORT`/`DB_SSL_MODE`.

## What to expect in normal dev traffic

After enabling the reader:

- **Auth-path SELECTs** (`FindByEmail`, `FindByID`, `FindWithMeta`, OAuth UID lookups) flow through the reader pool. Confirm via New Relic datastore segments (when configured per [`docs/OBSERVABILITY.md`](../OBSERVABILITY.md)) — segments should show queries going to the reader host.
- **Auth-path writes** (`UpdateLastLoggedOn` via the async `LastLoginWriter`, `RecordLoginAttempt`, `DeleteLoginToken`, the OAuth UID updates) flow through the writer pool. Same NR check, writer host.
- **The login-token lookup** in `internal/user/repository.go` deliberately uses the writer (the comment in code calls this out) to avoid replica lag — a token created and immediately consumed could miss on a lagging reader.

If any auth SELECT shows up as going to the writer pool in NR (other than the login-token lookup), that's a bug — file it.

## Per-environment rollout order

| Env | Owner | When |
|---|---|---|
| dev | LMS Devs | Immediately on this PR landing. The PIR specifically calls this out as the gap. |
| staging | LMS Devs | After dev has been on the reader for at least one week with no issues. |
| prod1 | LMS lead | After staging has run on the reader for two weeks and the load-test task ([86ba541q4](https://app.clickup.com/t/86ba541q4)) has confirmed behavior under reindex lag. |

The conservative rollout is deliberate. The 2026-05-19 incident was the inverse of this — a reader-pointed *writer* shipped to prod. Configuring this slowly and verifying at each step is the cheapest way to avoid the symmetric mistake (a writer-pointed reader, which would silently bypass the replica with no observable change).

## What "done" looks like

- `DB_READER_HOST` set in dev SSM and the dev `/health` endpoint reports `db_reader: ok`.
- New Relic confirms auth-path SELECTs are hitting the reader host (when NR is configured for dev — see PR #13).
- This runbook's "Per-environment rollout order" table is checked off through dev, with staging and prod1 dates planned.
- The [load-test task](https://app.clickup.com/t/86ba541q4) for Postgres-upgrade-window behavior includes the reader endpoint under test.
