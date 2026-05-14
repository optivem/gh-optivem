# Plan: audit gh-optivem for external-call retry coverage

## Context

Today's smoke-test failure (https://github.com/optivem/gh-optivem/actions/runs/25877369208) traced to `gh project field-list` hitting a transient GitHub GraphQL error. Investigation showed the call bypassed the existing retry wrapper (`internal/shell/ghretry.go:105 — RunWithRetry`) entirely, because `internal/steps/project.go` used `shell.RunCapture` directly and no retrying sibling existed.

The companion plan `20260514-1830-harden-init-graphql-transients.md` fixes that one site. **This plan is broader**: every other place in the Go code that talks to an external service (GitHub API, SonarCloud API, network) could have the same gap. An audit sweep is needed to find and classify them, then either route them through retry or document why retry is intentionally not applied.

Scope is **only retry coverage**. The orthogonal "silent stderr" audit was completed earlier today as `audits/20260514-silent-external-call-failures.md` (H1–H5 fixed) — that work is not re-opened here.

## Scope

In:

- `internal/**/*.go` and `main.go`
- Top-level `*_commands.go` files (`workspace_commands.go`, `cleanup_commands.go`, `implement_commands.go`, etc.)
- `internal/shell/sonarcloud.go` (direct `net/http` calls — important; not currently retried)

Out:

- Bash side (`.github/scripts/gh-retry.sh`, `.github/workflows/`) — covered by the sibling shop-workflow audit plan and by the `gh-retry.sh` edit in the GraphQL-transients plan.
- Local-only commands (`git add`, `git commit`, file ops, `docker compose`) — retry on a local operation is rarely correct and usually masks bugs; these are out of scope unless they cross a network boundary (e.g. `git push`, `git clone`).

## Method

Use the `code-auditor` agent — it is already scoped to `internal/**` and `main.go` and exists for exactly this kind of recurring-pitfall sweep (see `.claude/agents/code-auditor.md`). Brief it on the new pitfall pattern explicitly: it has been used previously for silent stderr coverage; retry coverage is a new dimension.

Audit method per call site:

1. **Identify call sites.** Grep for every invocation of:
   - `shell.Run`, `shell.MustRun`, `shell.RunCapture`, `shell.RunStdin`, `shell.RunPassthrough` (and their `Must*` variants) — known: 29 occurrences across 9 files per today's grep.
   - Direct `exec.Command(...)` outside the `shell` package — known: 18 files, mostly runtime-side bindings.
   - `net/http` calls — known: 6 files (`internal/shell/sonarcloud.go` is the load-bearing one).
2. **Classify each site** by the kind of work the command does:
   - **GH API** (`gh api`, `gh project`, `gh repo`, `gh release`, `gh issue`, `gh pr`, `gh secret`, `gh variable`, `gh workflow`, `gh run`, `gh label`, …) → **must retry on transient**.
   - **Git remote** (`git push`, `git fetch`, `git clone`, `git ls-remote`) → **must retry on transient** (network).
   - **SonarCloud HTTP** → **must retry on transient**.
   - **Local git** (`git add`, `git commit`, `git status`, `git rev-parse`, `git log`, `git diff`) → no retry (local).
   - **Local tools** (`docker`, `mvn`, `gradle`, `npm`, `npx`, `dotnet`, etc.) → no retry (local lifecycle).
   - **Probes designed to fail** (`gh auth status`, `gh api rate_limit`, owner/repo existence checks) → no retry (would hide a hard 4xx).
3. **Classify retry state per site**:
   - **R-OK**: Already uses one of `MustRunWithRetry`, `RunWithRetry`, `MustRunStdinWithRetry`, `MustRunPostCreate`, or (after the GraphQL plan) `RunCaptureWithRetry`.
   - **R-MISSING**: External call without retry → finding to fix.
   - **R-DOC-OK**: Local or probe call; no retry needed. Comment in code should document why if not obvious.
4. **For each R-MISSING site**, recommend the smallest correct wrapper swap (`Run` → `RunWithRetry`, `RunCapture` → `RunCaptureWithRetry` once added, `RunStdin` → `MustRunStdinWithRetry`).

The retry classifier itself (`ghRetryTransient` regex at `internal/shell/ghretry.go:24`) is also in scope: every new wording the audit surfaces should be added in lockstep to both the Go regex and the bash twin at `.github/scripts/gh-retry.sh:29`.

## Critical files

- `internal/shell/github.go` — `Run`, `MustRun`, `RunCapture`, `RunStdin`, `RunPassthrough` definitions.
- `internal/shell/ghretry.go` — retry wrappers and classifier.
- `internal/shell/sonarcloud.go` — direct `net/http` calls; **highest-priority candidate** since SonarCloud calls are not currently wrapped in any retry at all.
- `internal/shell/github.go` — `gh repo create` follow-ups already use `MustRunPostCreate` (permissive classifier); use as a working template.
- `internal/steps/*.go` — primary consumers of the shell helpers; most R-MISSING sites will be here.
- `internal/atdd/runtime/**` — runtime-side bindings; per the prior audit, these mostly use `cmd.Output()` directly. They are out of the smoke-test path but in scope for completeness.
- `audits/20260514-silent-external-call-failures.md` — companion document; the new report should adopt the same structure (TL;DR, prioritized table, healthy patterns).

## Deliverable

1. A new audit report at `audits/<date>-external-call-retry-coverage.md`, structured as:
   - **TL;DR** — name the seed gap (if any new family emerges) and the blast radius.
   - **Findings table** — High / Medium / Low, columns: site, command kind, current wrapper, recommended wrapper.
   - **Healthy patterns** — list every R-OK site that should serve as a template.
   - **Recommended order of fixes** — high-blast-radius first.
   - **Counts** — keep under the 20-finding cap from the agent contract.

2. A follow-up plan file `plans/<date>-fix-external-call-retry-gaps.md` listing each R-MISSING site as a discrete edit, in the order from the audit's "Recommended order of fixes". Each entry: file:line, current line, replacement line, rationale.

## Verification

Audit-phase verification:
- `go build ./...` and `go test ./...` must remain green throughout (audit is read-only).
- The audit's R-OK and R-MISSING lists must together cover every external-call site grep finds — no "unclassified" residue.
- Spot-check 3 R-DOC-OK sites by reading the surrounding code/comments to confirm the classification.

Fix-phase verification (per the follow-up plan):
- `go test ./internal/shell/...` for each new wrapper added.
- `go test ./internal/steps/...` to confirm seam swaps are invisible to existing tests (the project.go seam pattern is the template).
- A targeted failure-injection in one fix site (e.g. local edit that forces `gh api` to a non-existent endpoint and asserts the retry logs surface) — single manual smoke is enough; do not add a network-dependent unit test.

## Out of scope (explicit)

- Adding retry to local git / docker / build-tool calls.
- Changing the backoff schedule or `ghRetryAttempts` constant.
- Reworking the runtime-side bindings to centralise on `shell.*` — that's a separate architectural concern; this audit only flags whether their existing `cmd.Run()`/`cmd.Output()` calls need retry, not whether they should be refactored to share infrastructure.
