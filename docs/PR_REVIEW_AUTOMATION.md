# PR review automation

Three GitHub Actions workflows run on PRs. The two Claude agents are **advisory** (don't block merge); shellcheck is a **required check** because its failures are deterministic and almost always real bugs.

| Workflow | Triggers on | Blocks merge? | Purpose |
|---|---|---|---|
| [`claude-code-review.yml`](../.github/workflows/claude-code-review.yml) | every PR (opened / synchronize / ready_for_review / reopened) | No | Code quality review via the [`code-review`](https://github.com/anthropics/claude-code) plugin. Was previously a custom prompt focused on correctness / clarity / missing tests; replaced 2026-05-27 with the plugin-based default. |
| [`claude-security-review.yml`](../.github/workflows/claude-security-review.yml) | PRs touching auth/oauth/middleware/token/ratelimit/config/database/user, server entrypoint, migrations, CloudFormation, `go.mod`/`go.sum`, `.github/`, `scripts/`, any `*.sh`/`*.bash`, `Dockerfile`, `Makefile`, `Jenkinsfile` | No | Exploitable HIGH/MEDIUM findings only, confidence ≥ 8. Mirrors the methodology in [`pre-release-hardening/reservoir-security-review.md`](./pre-release-hardening/reservoir-security-review.md). Reviews workflow / supply-chain / shell-injection categories alongside the auth-focused ones. |
| [`shellcheck.yml`](../.github/workflows/shellcheck.yml) | PRs (and pushes to `main`) that change any `*.sh` or `*.bash` file | Yes | Deterministic shell-script linting at `--severity=warning`. Complements the Claude agents — Claude looks for problems only a human could spot, shellcheck catches the bugs a linter can enumerate. |

## Setup

One-time, by a repository admin:

1. Run `claude` locally and invoke `/install-github-app`. The wizard installs the [Claude Code GitHub App](https://github.com/apps/claude) on the repo and provisions a `CLAUDE_CODE_OAUTH_TOKEN` repository secret tied to your Anthropic account.
2. (Optional) Add billing alerts on the Anthropic workspace; see [Operating cost](#operating-cost) for expected spend.

After that, the workflows run automatically on every matching PR — no per-PR configuration.

### Note about the auth secret

The action accepts either `anthropic_api_key` (a direct API key) or `claude_code_oauth_token` (an OAuth token tied to an Anthropic account). This repo uses the OAuth path. The two are not interchangeable per-workflow — all three workflows must reference the same secret, or runs will fail with `Either ANTHROPIC_API_KEY, CLAUDE_CODE_OAUTH_TOKEN, or workload identity federation is required when using direct Anthropic API`.

## Operating cost

Each agent invocation runs against the PR diff plus a small amount of surrounding-file context. For a typical Reservoir PR (<500 lines changed) expect roughly **$0.05–$0.50 per run per agent**, depending on how much context Claude pulls. Cost telemetry is printed to the workflow run log.

Concurrency is set to `cancel-in-progress: true` on the security workflow — when you push new commits to a PR, the in-flight review is cancelled and only the latest revision is reviewed. This keeps cost bounded even on PRs with many rapid pushes. The code-review workflow (post 2026-05-27 rewrite via `/install-github-app`) does not currently set `concurrency:` and will fan out parallel runs on rapid pushes. Worth restoring if cost becomes a concern.

## Skipping or re-running

- **Skip the security agent on a PR**: open the PR as a draft. The security workflow has `if: github.event.pull_request.draft == false`. Mark it ready for review when you want feedback. (The code-review workflow no longer has this skip post-rewrite — it will review drafts.)
- **Skip the code-quality agent for a docs-only PR**: not currently automatic. The pre-2026-05-27 version of the code-review workflow had `paths-ignore: ['**.md', 'docs/**', 'LICENSE']`; the rewrite removed that. The security workflow uses a positive `paths:` filter so it won't trigger on pure docs PRs.
- **Re-run after changing the prompt**: edit the workflow file, push, and open a new PR (or push to an existing one). The workflow on the PR's branch is what runs, so prompt iteration is per-PR.
- **Manually re-run a single review**: Actions tab → pick the failed run → Re-run jobs.

## Reading the output

Both agents post findings as **inline review comments** at `file:line` rather than as a long summary blob. Look for:

- A tracking comment from `claude[bot]` titled "Claude Code is reviewing this pull request…" — appears at start, updates to "Completed" when done.
- Inline comments at specific lines, severity-tagged where applicable (security agent always tags HIGH/MEDIUM; code-quality agent uses prose).
- A single short summary comment, or "no findings" if Claude found nothing material.

If you disagree with a finding, reply on the inline thread as you would with any human reviewer. There is no suppression file yet — if a false positive keeps recurring across PRs, edit the prompt in the workflow file to exclude that pattern explicitly.

## When to update the prompts

- **Code-quality agent**: uses the upstream `code-review` plugin, so the prompt is largely outside this repo. To customize, either (a) override `prompt:` in the workflow with a local string, or (b) replace the plugin with a self-hosted prompt — the pre-2026-05-27 version of this workflow had a customized prompt focused on correctness / clarity / missing tests; it's recoverable from git history if the team wants it back.
- **Security agent**: if Claude is consistently missing a class of finding a human review *did* catch, add that class to Phase 2 of the prompt. The PIR / security review docs in [`pre-release-hardening/`](./pre-release-hardening/) are the source of truth for what "good" looks like.

## Known limitations / drift

- The code-review workflow's permission block is `pull-requests: read`, which is narrower than what GitHub's `${{ github.token }}` needs to post inline comments. The action authenticates as the Claude Code GitHub App (separate auth path) which carries its own permissions, so this works in practice — but the workflow's stated permissions are misleading.
- The code-review workflow has no `concurrency:` guard. Rapid pushes to a PR will fan out parallel API calls.
- Neither Claude workflow pins `anthropics/claude-code-action` by SHA — both reference `@v1`. Worth pinning to a SHA for stronger supply-chain posture; tracked in the security agent's own findings on subsequent PRs.

## Cross-repo rollout

These workflows live in the Reservoir repo. A parallel rollout to the LMS repo is tracked separately ([86ba53xxp](https://app.clickup.com/t/86ba53xxp), [86ba53y6m](https://app.clickup.com/t/86ba53y6m)). When that ships, the security agent's path filter will look different (Rails layout, not Go) but the methodology and output format should stay aligned so reviewers see the same surface across repos.
