# 2026-06-25 09:56:16 UTC — Recover commit-stage verify from fresh-repo first-push trigger drops

## TL;DR

**Why:** `gh optivem init` / the manual test fails at "Verify commit stage" when GitHub drops a fresh repo's first-push trigger *without* emitting a `startup_failure`. The runtime gate at `internal/kernel/shell/github.go:556` reads that as a genuinely-broken `paths:` filter and fails loud — a false negative, since the filter is correct.
**End result:** A genuinely-broken commit-stage `paths:` filter is caught deterministically *before* push (static check, fails loud at scaffold time). At runtime, a missing push-triggered run is always recovered via `workflow_dispatch` (bounded by `maxReDispatches`), so the GitHub first-push flake no longer fails the scaffold.

## Outcomes

What we get out of this — the goals and deliverables:

- The manual test / `gh optivem init` no longer fails at "Verify commit stage" when GitHub silently drops the first-push trigger on a fresh repo (the no-`startup_failure` variant of the flake).
- Protection against a real broken `paths:` filter is preserved — moved to a deterministic, local, pre-push static check that fails loud at scaffold time with a precise message (filter patterns + scaffolded files).
- The runtime watcher (`RunWatchPushWorkflow`) becomes a single recovery path: if the push-triggered run doesn't appear, re-dispatch via `workflow_dispatch` regardless of whether a `startup_failure` is present.
- Unit tests reflect the new behavior: the old "fail loud when no startup_failure" test becomes a "recover on missing run" test; a new test covers the static paths-filter check.

## ▶ Next executable step (resume here)

Step 1: Add the static pre-push paths-filter check in `internal/scaffolding/steps/verify.go` (alongside `VerifyScaffoldWorkflows`). Parse each scaffolded commit-stage workflow's `on.push.paths` with `gopkg.in/yaml.v3` and assert at least one tracked/scaffolded file in the repo dir matches a positive pattern; fail loud (`log.Fatalf`) listing the filter patterns and the scaffolded file set when nothing matches. Needs `**`-aware glob matching (`path.Match` is insufficient — add a small matcher or a doublestar lib; executor's discretion). This unblocks Step 2 (loosening the runtime gate is only safe once the filter is statically guaranteed).

## Steps

- [ ] Step 1: Static pre-push paths-filter validation in `internal/scaffolding/steps/verify.go`. For every scaffolded commit-stage workflow (`*-commit-stage.yml` / `commit-stage.yml` across mono/multi layouts), parse `on.push.paths` (yaml.v3) and assert ≥1 tracked file matches a positive pattern (negations like `!backend/VERSION` only subtract). Fail loud at scaffold time if none match. Pick a `**`-aware matcher. Run this in the existing pre-push lint phase (`phaseApplyTemplate`), not the runtime verify phase.
- [ ] Step 2: Loosen the runtime gate in `internal/kernel/shell/github.go` `RunWatchPushWorkflow` (~546-570). When the push-triggered run doesn't appear within the appear-window, re-dispatch via `workflow_dispatch` (bounded by `maxReDispatches`) unconditionally — drop the `hasRecentStartupFailure()` gate that currently fails loud at github.go:556-562. Keep `hasRecentStartupFailure()` only if it adds useful log context (e.g. distinguishing "GitHub emitted startup_failure" from "GitHub silently dropped the trigger" in the warn message). Remove the now-dead `github.go:561` "push trigger did not fire" error path (or repoint it).
- [ ] Step 3: Update tests in `internal/kernel/shell/github_test.go`. Rewrite `TestRunWatchPushWorkflow_FailsLoudWhenNoStartupFailure` (line ~178) into a "recover-on-missing-run" test: no run + no startup_failure now re-dispatches (asserts `dispatched == maxReDispatches` and the error mentions "re-dispatch attempts", matching the existing startup_failure case). `TestRunWatchPushWorkflow_ReDispatchesOnStartupFailure` (line ~208) still holds. Add a unit test for the Step 1 static check (a matching filter passes; a filter pointing at a non-existent dir fails loud).
- [ ] Step 4: Verification — `go build ./...` and `go test ./internal/kernel/shell/... ./internal/scaffolding/...`.

## Verification

- `go build ./...` passes.
- `go test ./internal/kernel/shell/... ./internal/scaffolding/...` passes.
- Re-run the manual test (fresh-repo scaffold) and confirm the commit stage recovers via `workflow_dispatch` when GitHub drops the first-push trigger, instead of fataling at "Verify commit stage". *(Operator step — needs a live fresh repo + GitHub's intermittent behavior.)*

## Notes

- Single Go implementation (the `gh-optivem` tool). No parallel .NET/Java/TS implementations to mirror.
- Root cause is intermittent GitHub infra behavior on a fresh repo's first (branch-creating) push: null `before` SHA → no diff base → path-filtered `push` workflows are unreliably created. The `startup_failure` variant was already handled; this plan adds the no-run variant.
- Design rationale (per the repo's fail-loud-at-the-right-layer convention): the "is the paths filter broken?" question is answered *definitively* and *locally* by the static check (Step 1); the runtime watcher (Step 2) then only has to answer "did GitHub fire it, and recover if not" — no more guessing from the `startup_failure` proxy signal.
