# Reservoir Capacity Planning

Sizing recommendations for the Fargate task and autoscaling parameters in [.cloudformation/reservoir.cfhighlander.rb](../.cloudformation/reservoir.cfhighlander.rb).

## Baseline (DEV)

```ruby
ComponentParam 'DesiredCount', 1
ComponentParam 'MinCount', 1
ComponentParam 'MaxCount', 1
ComponentParam 'EnableScaling', 'false'
ComponentParam 'Cpu', '2048'     # 2 vCPU
ComponentParam 'Memory', '4096'  # 4 GB
```

## Bottleneck

Reservoir is **CPU-bound on bcrypt password verification**, not I/O. Cost factor is pinned to 10 to match Rails' `has_secure_password` ([internal/auth/password.go:10](../internal/auth/password.go#L10)) — this is a cross-system contract, not a tuning knob.

Per-request cost:
- **POST /auth/login** — ~100ms bcrypt + 2 indexed Postgres queries + 1–2 Redis ops
- **GET /auth/me** — ~10ms JWT parse + 1 Postgres query (two joins)
- **POST /auth/refresh** — ~1ms JWT only, no DB

Roughly: at cost factor 10, one vCPU handles ~10 logins/sec sustained. A 4 vCPU task handles ~40 logins/sec at saturation, ~25 at the 60% CPU autoscaling target ([.cloudformation/app.config.yaml:87-92](../.cloudformation/app.config.yaml#L87)).

Token-validation and refresh traffic are ~10× cheaper than login, so the mix matters. Assumptions below use a 50/30/20 login/validate/refresh split.

## Recommendation: 5,000 req/min (~83 req/sec)

```ruby
ComponentParam 'DesiredCount', 2
ComponentParam 'MinCount', 2
ComponentParam 'MaxCount', 4
ComponentParam 'EnableScaling', 'true'
ComponentParam 'Cpu', '4096'     # 4 vCPU
ComponentParam 'Memory', '8192'  # 8 GB (Fargate minimum at 4 vCPU)
```

**Why:**
- 2 tasks × 4 vCPU = 8 vCPU. ~42 logins/sec needs ~4 vCPU of bcrypt budget → ~80% headroom steady-state.
- A single-task loss (deploy, AZ blip) still leaves capacity to serve the SLO.
- MaxCount 4 absorbs 2× spikes (~10k rpm) without re-deploying.

**Cheaper alternative if HA is negotiable** (non-prod only):

```ruby
ComponentParam 'DesiredCount', 1
ComponentParam 'MinCount', 1
ComponentParam 'MaxCount', 3
ComponentParam 'EnableScaling', 'true'
ComponentParam 'Cpu', '4096'
ComponentParam 'Memory', '8192'
```

One 4 vCPU task handles 83 req/sec steady-state; autoscaling covers spikes. Note: `MinimumHealthyPercent: 100` ([.cloudformation/reservoir.cfhighlander.rb:28](../.cloudformation/reservoir.cfhighlander.rb#L28)) requires room for an additional task during rollout — Max=3 satisfies this; Min=Max=1 would stall deploys.

## Recommendation: 10,000 req/min (~167 req/sec)

```ruby
ComponentParam 'DesiredCount', 3
ComponentParam 'MinCount', 3
ComponentParam 'MaxCount', 8
ComponentParam 'EnableScaling', 'true'
ComponentParam 'Cpu', '4096'
ComponentParam 'Memory', '8192'
```

**Why:**
- ~83 logins/sec needs ~8 vCPU of bcrypt budget. Three 4-vCPU tasks (12 vCPU) gives headroom and survives a single-task loss.
- MaxCount 8 covers bursts up to ~25k rpm via the 60% CPU scaling target.
- Horizontal beats vertical here — Fargate's standard tier caps at 4 vCPU/task, and cost steps up sharply above that.

## Supporting changes to verify before cutover

1. **RDS connection budget.** Connection pool is hardcoded to 50 per task in [internal/database/postgres.go:28](../internal/database/postgres.go#L28).
   - 5k rpm config: 100 baseline → 200 peak connections
   - 10k rpm config: 150 baseline → 400 peak connections

   Verify `max_connections` on the target RDS instance. `db.t3.medium` is ~200; the 10k config likely needs `db.r6g.large` or larger.

2. **Bump pool to `SetMaxOpenConns(75)`** only if login latency shows DB-wait time in metrics, and only after confirming RDS headroom.

3. **Internal ALB deregistration delay.** Verify it's short enough that scale-in doesn't drop in-flight requests. (Internal ALB was added in commit `d3a0efd`.)

4. **Load-test before cutover.** [tests/load-test.js](../tests/load-test.js) already targets p95 < 500ms on `/auth/login`:
   - 5k rpm: ~100 VUs / 10 min sustained
   - 10k rpm: ~200 VUs / 10 min sustained

## What to avoid

- **Don't raise per-task Cpu above 4096** on the standard Fargate tier — horizontal scaling is more cost-efficient and gives you HA for free.
- **Don't lower bcrypt cost factor** — it's a contract with Rails, not a performance knob.
- **Don't set Min=Max=1** in any environment that takes deploys — `MinimumHealthyPercent: 100` will stall the rollout.