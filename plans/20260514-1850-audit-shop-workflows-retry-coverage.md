# Plan: audit shop workflows for external-I/O retry coverage

## Context

Today's smoke-test failure (https://github.com/optivem/gh-optivem/actions/runs/25877369208) was rooted in an un-retried `gh` call on the **Go-binary side** (gh-optivem itself). The companion plan `20260514-1830-harden-init-graphql-transients.md` fixes that one site and `20260514-1845-audit-gh-optivem-retry-coverage.md` sweeps the Go binary for similar gaps.

But every project scaffolded by `gh optivem init` runs the **shop workflows** (`../shop/.github/workflows/*.yml`) every day in CI. Any step in those workflows that hits an external service without retry will fail just as flakily as the gh-optivem CLI did — and a single flaky workflow step amplifies across every consumer repo (88 workflow files in `shop/.github/workflows/` today). This plan audits the shop's workflows for external-I/O retry coverage and produces a remediation list.

The bash wrapper `.github/scripts/gh-retry.sh` already exists in this repo; sibling consumer repos and the shop workflows ship their own gh-retry equivalents through `optivem/actions/shared/gh-retry.sh` (referenced in the script's docstring). The audit verifies they actually **use** the wrapper at every external-I/O step.

## Scope

In:

- `../shop/.github/workflows/*.yml` — 88 files: commit-stage, acceptance-stage, qa-stage, prod-stage, prerelease, release, cleanup, drift, bump-patch, plus dotnet/java/typescript variants.
- Reusable workflows under `../shop/.github/workflows/_*.yml` (e.g. `_prerelease-pipeline.yml`, `_meta-prerelease-pipeline.yml`).
- Composite actions invoked by those workflows (`../actions/**/action.yml` and any `./.github/actions/**` in shop), since the actual `gh` / `curl` calls live in composites, not in the workflow files themselves.

Out:

- gh-optivem's own `./.github/workflows/` — covered by the gh-optivem audit plan.
- Sibling consumer repos other than `shop` (optivem-testing etc.) — handle in a follow-up if needed; shop is the load-bearing template.
- Local steps (`docker build`, `mvn`, `gradle`, `npm`, `dotnet`) that do not cross a network boundary on every step — these can have transient failures (registry outages) but their retry story is separate and usually handled by the build tool itself.

## Method

Use the `workflow-auditor` agent — it already scopes `../shop/.github/workflows/` and uses a rubric-based structure (`.claude/agents/workflow-auditor.md`). Extend its rubric with a new section before invoking:

> **§N — External-I/O retry coverage.**
> Any step that invokes a network-bound external service (GitHub API via `gh`, Docker Hub, GHCR, SonarCloud, language registries — npm, maven central, nuget) **must** wrap the call in a retry mechanism with bounded exponential backoff.
>
> Acceptable mechanisms, in order of preference:
> 1. **`gh_retry` wrapper** from `optivem/actions/shared/gh-retry.sh` for `gh` calls (matches the bash twin already in this repo at `.github/scripts/gh-retry.sh`).
> 2. **Native action with retry built in** — e.g. `docker/build-push-action@v6` has internal retry; explicit retry-loop YAML is unnecessary alongside it.
> 3. **Per-step `nick-fields/retry@v3`** (or similar) for non-`gh` commands.
> 4. **Custom shell retry loop** in the step's `run:` block — acceptable, but should match the 4-attempt / 5s→15s→45s schedule used in `gh-retry.sh` so behaviour is consistent across the workspace.
>
> **Findings categories:**
> - **N-A — `gh` call without retry.** Step calls `gh api`, `gh release`, `gh issue`, `gh pr`, `gh run`, etc. directly. Recommendation: source `gh-retry.sh` and switch to `gh_retry ...`.
> - **N-B — Other network call without retry.** `curl`, `wget`, `docker pull`, `docker push`, `npm install`, `mvn deploy`, `dotnet restore`, SonarCloud scan upload — any of these without retry. Recommendation: wrap in `nick-fields/retry@v3` or a matching shell loop. Note: many of these are *implicit* (e.g. `actions/setup-node` does its own registry fetch); flag only the ones the workflow YAML invokes directly.
> - **N-C — Retry present but misconfigured.** Retry exists but uses an aggressive schedule (e.g. 60s × 10), masks 4xx errors as transient, or wraps a hard-fail probe (`gh release view` to detect absence). Recommendation: align with `gh-retry.sh`'s 4×{5,15,45} schedule and hard-fail pass-through.
>
> **Anti-patterns to also flag:**
> - `continue-on-error: true` used as a retry substitute — silently masks failures rather than recovering from them.
> - `if: failure()` retry blocks that re-run the whole step, including non-idempotent work (e.g. `gh release create` would create duplicates).
> - Long-running commands wrapped in retry without a timeout — a stuck network call retries 4× with no progress.

Method per workflow / composite:

1. Grep for every external command in the scope:
   - `gh ` calls in `run:` blocks (`grep -nE '^\s+gh\b' workflows/*.yml`).
   - `curl`, `wget` calls.
   - `docker push`, `docker pull`, `docker login`.
   - `uses:` references to known network-dependent actions (`actions/setup-*`, `docker/login-action`, `sonarsource/sonarcloud-github-action`, `softprops/action-gh-release`).
2. For each match, classify by §N-A / §N-B / §N-C or "R-OK" (retry present and correctly configured).
3. Cross-reference with composites: many shop workflows delegate to `../../actions/<name>/action.yml`. Audit the composite's `run:` blocks, not just the workflow that calls it.
4. Cap findings at 20 per the agent contract; if the cap is hit, note the overflow file count.

## Critical files

- `../shop/.github/workflows/*-acceptance-stage.yml` — most likely to hit GitHub API transients during dispatch + status polling.
- `../shop/.github/workflows/*-prerelease-pipeline.yml`, `*-release-stage.yml` — package-registry uploads (Docker Hub, GHCR, npm, maven central) are common N-B candidates.
- `../shop/.github/workflows/cleanup.yml`, `drift.yml`, `compose-drift.yml` — scheduled workflows; failures here run unobserved and the API surface is small but uniform (`gh api` calls).
- `../shop/.github/workflows/_meta-prerelease-pipeline.yml`, `_prerelease-pipeline.yml` — reusable workflows; one fix here cascades.
- `../actions/**/action.yml` — the actual `gh`/`curl` call sites; many workflow steps are thin shims.
- `optivem/actions/shared/gh-retry.sh` — confirm it's the deployed wrapper across all composites (the in-tree `.github/scripts/gh-retry.sh` should be byte-identical or a deliberate fork).

## Deliverable

1. A new audit report at `audits/<date>-shop-workflow-retry-coverage.md`. Structure (mirroring the existing `audits/20260514-silent-external-call-failures.md` shape):
   - **TL;DR** — the dominant gap (likely "Composite X uses raw `gh` without sourcing `gh-retry.sh`") plus the worst-affected workflow.
   - **Findings table** — N-A / N-B / N-C, columns: workflow:line, command, current state, recommended wrapper.
   - **Healthy patterns** — at least one workflow / composite already doing it right, as a template.
   - **Recommended order of fixes** — composite-level fixes first (they cascade), then leaf-workflow one-offs.
   - **Counts** under the 20-cap.

2. A follow-up plan file `plans/<date>-fix-shop-workflow-retry-gaps.md` listing concrete edits per `gh-optivem repo guidelines` style: file, line, before, after, rationale. Each edit must reference the shop repo's PR conventions (separate PR per logical change; the workflow-auditor agent should not batch unrelated fixes).

3. **§N rubric addition** committed back to `.claude/agents/workflow-auditor.md` so future audits inherit the rule. This is the one in-tree code change the audit phase produces; everything else is read-only.

## Verification

Audit-phase:
- The audit report must classify every external-I/O step found by the greps above — no "unclassified" residue.
- Cross-check against `gh-retry.sh`: every R-OK site should either source `gh-retry.sh` (or its `optivem/actions/shared/` twin) or use a Marketplace action with documented internal retry.
- Spot-check 3 N-A findings by manually reading the workflow + composite to confirm no upstream wrapper exists.

Fix-phase (per the follow-up plan, run in the **shop repo**, not here):
- For each composite changed, run the smallest workflow that uses it (`*-commit-stage.yml` is typically the cheapest) via `workflow_dispatch` against a throwaway branch and confirm green.
- For each N-A leaf-workflow fix, re-dispatch on a test repo (use the same test-app pattern as `gh-acceptance-stage` smoke jobs).
- Verify the retry actually triggers by injecting one failure: temporarily change a `gh api` call to target a non-existent endpoint, confirm the wrapper retries 4×, then revert. Do not commit the injection.

## Out of scope (explicit)

- Adding retry to local-only steps (compile, unit test, lint).
- Retry inside language toolchains (`mvn`, `gradle`, `npm`) — these have their own retry contracts.
- The retry schedule itself — keep `gh-retry.sh`'s 4×{5,15,45} unless a finding shows it's empirically wrong.
- Workflows in repos other than `shop` — handle in follow-up audits if smoke data justifies it.

## Coordination with the gh-optivem audit

The two audits share a regex: the Go-side `ghRetryTransient` (`internal/shell/ghretry.go:24`) and the bash-side `_GH_RETRY_RETRYABLE` (`.github/scripts/gh-retry.sh:29`) must list the same alternatives. The GraphQL-transients plan adds `Something went wrong while executing your query` to both; this audit should treat that as already-done and flag any **further** divergence between the two regexes as a finding.
