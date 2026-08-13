# 2026-08-13 10:07:00 UTC — Fix `gh optivem init` warning-and-prompting for an already-existing repo when the run can no longer succeed

## TL;DR

**Why:** Issue #60's fix (commit `e98a4af0`) made `GitHub.CreateRepo()` (`internal/kernel/shell/github.go:432-433`) unconditionally `log.Fatalf` when the target repo already exists — by explicit design, no `--force`/opt-in. But it left the earlier preflight gate, `confirmReposExist()` (`internal/config/config.go:1117-1166`, called from Phase 2 at `config.go:1241`), unchanged: it still `log.Warnf`s and prompts `Proceed? [y/n]:` (or silently proceeds under `--yes`), implying a "yes" answer can succeed. It cannot — `internal/scaffolding/steps/github_setup.go:27` (and lines 35/38/42 for multirepo/multitier) unconditionally calls `gh.CreateRepo()` for every configured repo, so any repo that trips the prompt is guaranteed to hit the fatal moments later, after burning time on env/project checks, shop-ref resolution, and cloning.
**End result:** `gh optivem init` against an already-existing target repo fails immediately in Phase 2 — before shop-ref resolution or cloning — with a clear message naming the repo(s), and no misleading "Proceed?" prompt. Now-dead code paths left over from the old warn+confirm+continue behavior (`PreExistingRepos` plumbing) are removed.

## Outcomes

- Running `gh optivem init` against an already-existing target repo (or any component repo in multirepo/multitier) aborts immediately in Phase 2 with a clear, actionable error naming the repo(s) — no prompt, no wasted work.
- `--yes` no longer documents skipping an "existing-repo prompt" that no longer exists.
- `Config.PreExistingRepos` and its only consumer (`finalize.go`'s conditional `commitAndPushRepo` argument) are removed, since a pre-existing repo can never reach `finalize()` anymore.
- Tests reflect the new fail-fast behavior (existing repo → immediate fatal, no prompt) instead of the old warn+confirm+continue behavior.

## ▶ Next executable step (resume here)

Step 1: In `internal/config/config.go`, rewrite `confirmReposExist()` (~lines 1117-1166) to `log.FatalExit` immediately once any repo in `fullRepos` is found to already exist — naming the repo(s), mirroring the style of `CheckOwnerExists`/`CheckProjectExists` immediately above its call site at `config.go:1235-1240`. Remove the `approval.Confirm` prompt and the `assumeYes` "proceed after warning" branch entirely (existing repo is always fatal, matching `CreateRepo()`; `--yes` has nothing left to bypass on this check).

## Steps

- [ ] Step 1: Rewrite `confirmReposExist()` in `internal/config/config.go` (~lines 1117-1166) to fail immediately (`log.FatalExit`) naming the pre-existing repo(s) instead of warning + prompting + continuing. Drop the `approval.Confirm`/`assumeYes` branches. Update the function's doc comment (currently describes "asks the user once whether to proceed" and the "legitimate re-scaffold" rationale) to describe the new fail-fast behavior.
- [ ] Step 2: Remove the now-unused `PreExisting`/`preExistingSet` local plumbing in `LoadProjectConfigForInit` (`config.go:1241-1250`) and the `PreExistingRepos` field on `Config` (`config.go:190,195`) and its assignment (`config.go:1364`), since `confirmReposExist` returning means no repo pre-existed.
- [ ] Step 3: Simplify `internal/scaffolding/steps/finalize.go:117-124` — `commitAndPushRepo` calls no longer need the `cfg.PreExistingRepos[...]` argument; check `commitAndPushRepo`'s signature and drop the now-always-false parameter (or the parameter entirely if nothing else sets it).
- [ ] Step 4: Update the `--yes` flag help text at `config.go:623` to remove the "existing-repo prompt" mention.
- [ ] Step 5: Update/add Go tests in `internal/config` covering `confirmReposExist`/`LoadProjectConfigForInit` to assert the new fail-fast behavior (existing repo → immediate fatal, no prompt), replacing any test asserting the old warn+confirm+continue behavior or `PreExistingRepos`.
- [ ] Step 6: Check `internal/kernel/shell/github_test.go` and any other tests referencing `PreExistingRepos` or the old prompt behavior, and update accordingly.
- [ ] Step 7: Run the full Go test suite to confirm no regressions.
- [ ] Step 8: Manually verify: run `gh optivem init` against an already-existing throwaway repo (not `book-shop`) and confirm it fails immediately in Phase 2 with a clear message naming the repo, no `Proceed?` prompt shown.

## Open questions

None — this completes the original plan's already-agreed decision (`plans` history: `20260813-0850-init-repeat-run-repo-corruption-guard.md`, "recommended: unsupported for now — smallest safe fix") that `CreateRepo()` should fail outright on an existing repo with no `--force`. This plan just finishes wiring that decision through the earlier preflight gate that was left stale.
