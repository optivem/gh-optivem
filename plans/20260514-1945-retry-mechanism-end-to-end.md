# Plan: end-to-end retry mechanism — consolidate, harden, audit, close gaps

Single source of truth for retry-related work in this workspace.

## Status (2026-05-15)

| Phase | What | Status |
|---|---|---|
| 1 | Canonical bash engine (`actions/shared/{retry-core,gh-retry,docker-retry,sonar-retry}.sh` + smoke tests + `sync-shared.sh`) | ✅ DONE |
| 2 | Sonar-504 fix in shop dotnet/java commit-stage workflows (using new shared engine) | ✅ DONE |
| 3 | Go engine extraction (`internal/shell/retrycore.go` + `ghretry.go` GraphQL transient + `finalize.go` missing-dir skip) | ✅ DONE — commit `f2f9211` |
| 4 | Audit gh-optivem Go for retry coverage | ✅ DONE — `audits/20260514-external-call-retry-coverage.md` |
| 5 | Audit shop workflows + add §4 rubric to `workflow-auditor.md` | ✅ DONE — `audits/20260514-shop-workflow-retry-coverage.md` |
| 6 | Execute fix lists from Phases 4 & 5 | 🟡 IN PROGRESS — see sub-plans below |

### Phase 6 split

- **Go side** — [`20260514-fix-gh-optivem-retry-gaps.md`](20260514-fix-gh-optivem-retry-gaps.md). **0 / 10 items done.** Not yet started; no pickup marker.
- **Shop side** — [`20260514-fix-shop-workflow-retry-gaps.md`](20260514-fix-shop-workflow-retry-gaps.md). **In progress** — picked up by `Valentina_Desk` at `2026-05-15T05:33:25Z`. Items 1–8 dropped per commit `c4ede16`; items 9–20 remain.

### Open side-question (not work)

- [`20260514-2200-retry-helpers-canonical-home.md`](20260514-2200-retry-helpers-canonical-home.md) — proposal asking whether to move the canonical bash helpers from `optivem/actions/` to `optivem/gh-optivem/`. Status: **decision pending**, recommendation is Option A (status quo, no work). Requires human review before any execution.

---

## Current architecture (implemented)

The system as it exists today, post Phases 1-5. Kept as a reference for ongoing maintenance.

### `optivem/actions/shared/` — canonical bash source

| File | Purpose |
|---|---|
| `retry-core.sh` | Generic engine: `retry_with_policy <transient_re> <hard_fail_re> <prefix> -- <cmd...>`. Owns mktemp dance, attempt loop, 5s → 15s → 45s backoff, hard-fail pass-through, `::notice::`/`::warning::` annotations |
| `gh-retry.sh` / `docker-retry.sh` / `sonar-retry.sh` / `git-retry.sh` | Tool-specific regex wrappers, ~15 LOC each, delegating to `retry_with_policy` |
| `_test-*.sh` | Smoke tests for each |

### `optivem/actions/scripts/sync-shared.sh`

Idempotent vendoring script. Reads canonical helpers and writes them into:
- `../shop/.github/workflows/scripts/`
- `../gh-optivem/.github/scripts/`

Each vendored file gets a generated-from-X-at-SHA banner. Run manually after editing the engine or any wrapper, then commit per repo via `/commit`.

### `optivem/gh-optivem/internal/shell/` — Go port

| File | Role |
|---|---|
| `retrycore.go` | Generic `RetryWithPolicy(transient, hardFail *regexp.Regexp, prefix string, fn func() error) error`. Mirrors the bash factoring |
| `ghretry.go` | gh-specific regex constants + ~30 lines calling `RetryWithPolicy`. `RunCaptureWithRetry` added; `ghRetryTransient` extended to cover GraphQL "Something went wrong while executing your query" |
| `retrycore_test.go` / `ghretry_test.go` | Unit tests |

### Workspace files

| Location | What |
|---|---|
| `optivem/shop/.github/workflows/scripts/` | Vendored helpers (banner-tagged) |
| `optivem/gh-optivem/.github/scripts/` | Vendored helpers (banner-tagged) |
| `optivem/gh-optivem/.claude/agents/workflow-auditor.md` | §4 rubric — R-OK / R-DOC-OK / N-A / N-B / N-C categories |
| `optivem/gh-optivem/audits/20260514-external-call-retry-coverage.md` | Phase 4 audit |
| `optivem/gh-optivem/audits/20260514-shop-workflow-retry-coverage.md` | Phase 5 audit |

## Maintenance going forward

- **New transient pattern observed in CI logs** → one edit to canonical `retry-core.sh` (or a `<tool>-retry.sh` if tool-specific) + run `sync-shared.sh` + commit per affected repo via `/commit`. No mirror edits. For Go callers, mirror the regex change in `internal/shell/ghretry.go` (or the relevant Go wrapper).
- **New external tool needs retry** → drop in a ~15-line `<tool>-retry.sh` wrapping `retry_with_policy` with the tool's regex, re-run `sync-shared.sh`. For Go callers, add a thin function passing the tool's regex to `RetryWithPolicy`.
- **Sync drift detection** → vendored files carry a generated-from-SHA banner that flags mismatch on visual inspection; a CI lint enforcing this is logged as out-of-scope follow-up below.

## Intentionally NOT touched

- `audits/20260514-silent-external-call-failures.md` — orthogonal, complete (H1–H5 fixed). That work was about *what* errors contain (lossy stdio teeing); this plan is about *whether* the call is retried.

## Remaining validation (Phase 6)

- `go test ./internal/shell/...` for each new Go wrapper added.
- `go test ./internal/steps/...` to confirm seam swaps are invisible.
- For each composite changed in shop: run the smallest workflow that uses it via `workflow_dispatch` against a throwaway branch and confirm green.
- For each N-A leaf-workflow fix: re-dispatch on a test repo.
- One targeted failure-injection per fix family (point a `gh api` call at a non-existent endpoint, confirm retry logs surface, revert).

### Optional negative test (anytime)

On a throwaway shop branch, point `sonar.host.url` at `https://example.invalid` in one workflow; confirm the new annotation appears:

```
::notice::[sonar-retry] attempt 1 failed (exit 1): ... -- retrying in 5s
::notice::[sonar-retry] attempt 2 failed (exit 1): ... -- retrying in 15s
::notice::[sonar-retry] attempt 3 failed (exit 1): ... -- retrying in 45s
::warning::[sonar-retry] exhausted 4 attempts (exit 1): ...
```

before the step exits 1. Do not commit the injection.

## Remaining risks

- **Sync drift.** A vendored copy is edited directly in shop or gh-optivem, not via the canonical source. Mitigation: generated-from-X-at-SHA banner at top of each vendored file. Could be reinforced later with a CI lint comparing the vendored file's banner SHA against the canonical (out of scope here; logged in "Deferred / follow-up" below).
- **Cross-repo push ordering on future engine edits.** Push order is actions → gh-optivem → shop; vendored copies must not predate the canonical change they reference.
- **Audit cap overflow.** Each audit is capped at 20 findings per the agent contracts. If either Phase 4/5 audit needed a follow-up pass against the remainder, that pass hasn't been done; the existing reports cover what they cover.

## Deferred / follow-up topics

Intentionally not in this plan, but worth tracking. Each entry includes a trigger that should prompt revisiting it.

- **TS commit-stage workflows.** Three workflows (`monolith-typescript-commit-stage.yml`, `multitier-backend-typescript-commit-stage.yml`, `multitier-frontend-react-commit-stage.yml`) use `SonarSource/sonarqube-scan-action@v7` — a `uses:` step that inline `sonar_retry` can't wrap. **Currently tracked as Items 9–11 in the shop-side Phase 6 plan**, gated on upstream-retry verification. **Trigger to revisit independently:** a TS combo observably 504s in CI. **Options:** (a) replace the action with `npx sonar-scanner` + `sonar_retry` (preferred — consistent with the bash pattern), or (b) wrap with `Wandalen/wretry.action@v3` (would re-introduce the multi-mechanism split this plan eliminates, so avoid).
- **CI lint enforcing sync-banner SHA matches canonical.** Each vendored helper carries a `generated-from-X-at-SHA` banner; today verification is visual. A small lint workflow could compare each vendored copy's banner SHA against the canonical file's current SHA and fail the build on mismatch. **Trigger to revisit:** any observed instance of a vendored copy drifting from canonical, or the first time `sync-shared.sh` is forgotten and the regex update only lands in some repos.
- **`shop/system/.../run-sonar.sh` local helpers.** Local-dev-only; not invoked by CI. **Trigger to revisit:** a developer reports a transient 504 hitting them locally and asks for retry parity. Low priority either way.
- **TypeScript port of `retry-core` for future TS/JS code in shop.** Today shop's TS code shells via npm/npx which has its own retry contract. **Trigger to revisit:** new TS code in any consumer repo starts shelling to `gh`, `docker`, or a registry directly and observes transients.
- **Retry audits for repos other than `shop`** (e.g. `optivem/optivem-testing`, `optivem/courses`, `optivem/hub`). **Trigger to revisit:** smoke-test failures attributable to retry gaps in those repos. Method would parallel Phase 5.
- **5xx-only retry predicate vs. current regex-based classification.** Today both bash and Go classifiers grep stderr for keyword patterns (`HTTP 5\d\d|timeout|EOF|...`). A more precise approach could parse exit codes or HTTP status from structured output. **Trigger to revisit:** a misclassification observed in production (a non-retryable error triggering retries, or vice versa). Premature otherwise.

## Out by policy (not expected to change)

Decisions, not deferrals — these will not be revisited unless the policy itself is reconsidered (in which case, separate plan).

- **No retry on local-only operations** — `git add`, `git commit`, `docker build`, `dotnet build`, `mvn package`, `gradle assemble`, filesystem ops, etc. Retry on local operations is rarely correct and usually masks bugs (compilation errors don't get better the second time you try). The audit phases (4 and 5) classify these as R-DOC-OK, not R-MISSING.
- **Backoff schedule fixed at 4 attempts, 5s → 15s → 45s.** The current schedule has been stable across `gh-retry.sh`, `docker-retry.sh`, and the Go port. Changing it without empirical justification fragments the observation pattern in CI logs ("how many retries are normal?") and invalidates the existing test fixtures.
- **No retry mechanism other than the shared in-house engine.** Adopting `Wandalen/wretry.action@v3`, `nick-fields/retry@v3`, or similar alongside the in-house engine would re-create the multi-mechanism split this plan exists to eliminate. If a future requirement genuinely doesn't fit the engine, the right response is to extend the engine, not bypass it.

## Notes on shape decisions (for reviewers)

- **Why not a composite action wrapping retry?** Loses `$(gh_retry ...)` capture, loses interleaving with non-retried shell logic in one `run:` block, doesn't help the Go port.
- **Why not `Wandalen/wretry.action@v3` org-wide?** Inconsistent with the existing in-house pattern (different log format, different transient/hard-fail semantics, default-retry-on-everything).
- **Why vendor all helpers into all consumer repos, not only what each uses?** Sync script stays trivial (one cp loop, no per-repo allow-list). Storage cost is negligible. The day a new tool needs retry in a repo, the helper is already on disk.
- **Why a sync script, not a checkout/submodule/curl?** Preserves the "self-contained per repo" property the project deliberately chose for the existing bash helpers. No runtime plumbing fee, no extra failure mode.
