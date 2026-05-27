# Reservoir CPU profiling runbook

Addresses [PIR 2026-05-19](https://app.clickup.com/9014075154/v/dc/8cmfqrj-53074/8cmfqrj-87414) action item **#8** — *profile Reservoir CPU under realistic auth load to attribute saturation contributors*.

## Why this matters

On 2026-05-19 Reservoir CPU saturated to 99.4% under school-day login load. The PIR identified three likely contributors:

1. Failing `UPDATE last_logged_on` against a read-only endpoint on every request
2. bcrypt password verification (cost 12, ~250–350ms CPU per email/password login)
3. `fmt.Printf` stdout-mutex contention under high goroutine concurrency

Contributors 1 and 3 are now eliminated in code (async batched writes, `zap` structured logging). Contributor 2 is structural to bcrypt. Without a profile we don't know the actual relative weight. This runbook is how you produce one.

## When to run

- **Before re-enabling Reservoir in the LMS auth path.** Required pre-flight; cited in PIR action item #8.
- **After major changes to the auth hot path** (new password hashing scheme, new OAuth provider, structural change to the request lifecycle).
- **After any incident where CPU is suspected** — captured live if possible, immediately post-incident if not.

## Prerequisites

- A dev or staging environment with traffic-shaped DB seeding (enough users to make `FindByEmail` realistic; otherwise the index lookup is artificially cheap).
- A load-test harness that can replay the school-day mix: email/password, magic-link, Google, Clever, iCloud. The LMS load-test tasks in the current sprint cover most of these.
- Go toolchain locally for `go tool pprof`.

## How to expose pprof in Reservoir

Reservoir does **not** expose pprof endpoints by default — they leak runtime internals and shouldn't ship to prod without an access control story. For a profiling session:

1. On a profiling branch (do not merge to `main`), add to [`cmd/server/main.go`](../../cmd/server/main.go):

   ```go
   import _ "net/http/pprof"
   ```

   and bind the pprof handlers to a **separate, internal-only port** — never the public router:

   ```go
   // Profiling endpoints on a localhost-only port. Container exposes this
   // only inside the task's network namespace, never to the ALB.
   go func() {
       if err := http.ListenAndServe("127.0.0.1:6060", nil); err != nil {
           logger.Warn("pprof listener exited", zap.Error(err))
       }
   }()
   ```

2. Build and deploy to the profiling environment.

3. Verify the endpoints are reachable from inside the task:

   ```bash
   aws ecs execute-command --cluster ... --task ... \
     --command "curl -sf http://127.0.0.1:6060/debug/pprof/" --interactive
   ```

   They should NOT be reachable from outside the VPC.

4. **After the profiling session, revert the change.** Do not merge it.

## How to capture the profile

With the pprof endpoint reachable (either via `ecs execute-command` port-forward or a temporary security-group rule from a bastion):

```bash
# CPU profile — 30 seconds while load is at peak
go tool pprof -seconds=30 http://127.0.0.1:6060/debug/pprof/profile

# Heap profile — point-in-time snapshot
go tool pprof http://127.0.0.1:6060/debug/pprof/heap

# Goroutine dump — useful if you suspect blocked goroutines (e.g. mutex contention)
go tool pprof http://127.0.0.1:6060/debug/pprof/goroutine

# Mutex contention profile (must be enabled via runtime.SetMutexProfileFraction)
go tool pprof http://127.0.0.1:6060/debug/pprof/mutex
```

Apply representative load (see Prerequisites) while the profile is being captured.

## What to look for

In `go tool pprof`'s interactive shell or with `pprof -http=:8080` for the flame-graph UI:

| Symbol family in top-N self time | Likely contributor | Note |
|---|---|---|
| `golang.org/x/crypto/bcrypt.*` | Bcrypt password verification | Structural to bcrypt. Reducing cost from 12 → 10 ≈ 4× faster but weakens security; not recommended. |
| `database/sql.*`, `lib/pq.*`, `nrpq.*` | DB query overhead | If high, check: connection-pool saturation? Read replica routing missing? N+1 from auth lookups? |
| `runtime.mutex*`, `sync.(*Mutex).Lock` | Mutex contention | A return of the pre-incident `fmt.Printf` pattern. Should be near-zero post-2026-05-19. |
| `go-redis/redis.*` | Token blacklist + rate limiter | Confirm rate-limiter window/lockout settings aren't producing pathological Redis traffic. |
| `gin.*`, `net/http.*` | HTTP framework overhead | Usually low; if dominant, that's a clue the auth work itself is cheap and CPU is elsewhere. |
| `runtime.gcBgMarkWorker` | GC | If >5%, look at allocation hotspots in the heap profile. |

Cross-reference with the per-auth-RPS efficiency math in the [PIR doc](../pre-release-hardening/reservoir-security-review.md) (CPU Analysis section): Reservoir at 0.203 vCPU/auth RPS was efficient relative to LMS but should be significantly lower for a Go service. After this profiling exercise, that gap should be either explained or closed.

## What to record

Archive in `docs/pre-release-hardening/profiles/YYYY-MM-DD-<context>/`:

- The raw `.pprof` files (CPU, heap, goroutine).
- A short writeup (`README.md`): load applied (RPS, mix), task count, instance type, time of day, top-10 functions by self time, recommendation.
- Update the [PIR action items doc](../pre-release-hardening/PIR_2026_05_19_ACTION_ITEMS.md) entry for #8 with a link to the archive.

## What "done" looks like

- pprof profiles captured under load representative of the school-day peak.
- Per-path CPU attribution documented (e.g. "bcrypt is 60% of CPU, DB is 15%, …").
- Either: (a) confirms the PIR's narrative and we move on, or (b) surfaces an unexpected hotspot that becomes its own follow-up.

## Open questions for the operator

- Is the dev environment's user table seeded with enough rows to make `FindByEmail` realistic? If users.id has <10k rows, the index lookup is unrealistically fast.
- Does the load test exercise the *mix* of auth paths in school-day traffic, or only email/password? Per the PIR, email/password is the bcrypt-heavy path; OAuth paths skip bcrypt.
- Do we have New Relic distributed-trace coverage on the load-test environment to corroborate the pprof findings? See [`docs/OBSERVABILITY.md`](../OBSERVABILITY.md) (lands with [PR #13](https://github.com/boddle-learning/reservoir/pull/13)).
