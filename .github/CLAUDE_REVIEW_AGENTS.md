# Claude PR review agents

Two GitHub Actions workflows run [Claude Code](https://claude.ai/code) against every relevant PR. Both are **advisory** — neither blocks merge.

| Workflow | Triggers on | Purpose |
|---|---|---|
| [`claude-code-review.yml`](workflows/claude-code-review.yml) | every non-docs PR | Code quality: correctness, clarity, missing tests, consistency with the rest of the repo. |
| [`claude-security-review.yml`](workflows/claude-security-review.yml) | PRs touching auth/oauth/middleware/token/ratelimit/config/database/user, server entrypoint, migrations, CloudFormation, `go.mod`/`go.sum`, **`.github/`, `scripts/`, any `*.sh`/`*.bash`, `Dockerfile`, `Makefile`, `Jenkinsfile`** | Exploitable HIGH/MEDIUM findings only. Mirrors the methodology used in [`docs/pre-release-hardening/reservoir-security-review.md`](../docs/pre-release-hardening/reservoir-security-review.md). Reviews include workflow / supply-chain / shell-injection categories alongside the auth-focused ones. |

## Setup

One-time, by a repository admin:

1. Add an Anthropic API key as a repository secret named `ANTHROPIC_API_KEY` (Settings → Secrets and variables → Actions).
2. Confirm the [Claude Code GitHub App](https://github.com/apps/claude) is installed on this repo. The fastest way is to run `claude` locally and invoke `/install-github-app` — it walks through the install and secret setup.
3. (Optional) Add billing alerts on the Anthropic account; see [Operating cost](#operating-cost) below for expected spend.

After that, the workflows run automatically on every matching PR — no per-PR configuration.

## Operating cost

Each agent invocation runs Claude Sonnet 4.6 with the PR diff and a small amount of surrounding-file context. For a typical Reservoir PR (<500 lines changed) expect roughly $0.05–$0.50 per run per agent, depending on how much context Claude pulls. Cost telemetry is printed to the workflow run log.

Concurrency is set to `cancel-in-progress: true` — when you push new commits to a PR, the in-flight review is cancelled and only the latest revision is reviewed. This keeps cost bounded even on PRs with many rapid pushes.

## Skipping or re-running

- **Skip both agents on a PR**: open the PR as a draft. Both workflows have `if: github.event.pull_request.draft == false`. Mark it ready for review when you want feedback.
- **Skip the code-quality agent for a docs-only PR**: handled automatically — `paths-ignore` excludes `**.md`, `docs/**`, and `LICENSE`. (The security agent uses a positive `paths:` filter so it won't trigger on docs at all.)
- **Re-run after changing the prompt**: edit the workflow file, push, and open a new PR (or push to an existing one). The workflow on the PR's branch is what runs, so prompt iteration is per-PR.
- **Manually re-run a single review**: Actions tab → pick the failed run → Re-run jobs.

## Reading the output

Both agents post findings as **inline review comments** at `file:line` rather than as a long summary blob. Look for:

- A tracking comment from `claude[bot]` titled "Claude Code is reviewing this pull request…" — appears at start, updates to "Completed" when done.
- Inline comments at specific lines, severity-tagged where applicable (security agent always tags HIGH/MEDIUM; code-quality agent uses prose).
- A single short summary comment, or "no findings" if Claude found nothing material.

If you disagree with a finding, reply on the inline thread as you would with any human reviewer. There is no suppression file yet — if a false positive keeps recurring across PRs, edit the prompt in the workflow file to exclude that pattern explicitly.

## When to update the prompts

- **Code-quality agent**: if Claude is consistently flagging things the team doesn't care about (e.g. a style choice the codebase has deliberately settled on), add an explicit "do not flag X" instruction in the prompt's "Things to NOT comment on" section.
- **Security agent**: if Claude is consistently missing a class of finding a human review *did* catch, add that class to Phase 2 of the prompt. The PIR / security review docs in [`docs/pre-release-hardening/`](../docs/pre-release-hardening/) are the source of truth for what "good" looks like.

## Cross-repo rollout

These workflows live in the Reservoir repo. A parallel rollout to the LMS repo is tracked separately ([86ba53xxp](https://app.clickup.com/t/86ba53xxp), [86ba53y6m](https://app.clickup.com/t/86ba53y6m)). When that ships, the security agent's path filter will look different (Rails layout, not Go) but the methodology and output format should stay aligned so reviewers see the same surface across repos.
