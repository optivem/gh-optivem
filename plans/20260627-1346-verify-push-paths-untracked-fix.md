# 2026-06-27 13:46:00 UTC — Fix "Verify push path filters" failing on unstaged scaffold files

## TL;DR

**Why:** The newly-added "Verify push path filters" scaffold step fatals on every Smoke config because it enumerates candidate files with plain `git ls-files` (the git **index**), but it runs in `phaseApplyTemplate` *before* `CommitAndPush` stages anything — so the freshly scaffolded files are untracked and every positive `on.push.paths` pattern "matches no tracked file."
**End result:** `checkPushPathsFilter` matches patterns against the file set the upcoming commit will actually contain (`git ls-files --cached --others --exclude-standard`), so the check is order-independent, stays positioned before push, and all 4 Smoke configs pass.

## Outcomes

What we get out of this — the goals and deliverables:

- `gh optivem init` scaffolding no longer fatals at "Verify push path filters" for well-formed templates; the check only fires on a genuinely misconfigured `on.push.paths` filter.
- All 4 Smoke matrix configs (`TestValidMonolithConfigurations` / `TestValidMultitierConfigurations` in `internal/config`) pass green in CI run 28290135277's workflow.
- The push-path check is **order-independent** — it evaluates against the exact set of files the upcoming commit will include (tracked + new-untracked-non-ignored, respecting `.gitignore`), whether it runs before or after staging.
- Unit coverage for the real pipeline ordering: a test proving the check passes when matching files exist on disk but are **not yet staged**, so the gap can't silently regress.

## ▶ Next executable step (resume here)

Edit `internal/scaffolding/steps/verify.go` (~line 202): change `shell.Run("git ls-files", true, repoDir)` to `shell.Run("git ls-files --cached --others --exclude-standard", true, repoDir)`, and adjust the surrounding doc comment (verify.go:142–146) so it says the check matches "files the commit will include" rather than "tracked files." Then add the unstaged-files test case in `verify_test.go` (Step 2) and run `go test -p 2 ./internal/scaffolding/steps/...`. Single Go file change + one test; gh-optivem is Go-only, no parallel-language twins.

## Steps

- [ ] Step 1: In `internal/scaffolding/steps/verify.go` (~line 202, inside `checkPushPathsFilter`), change the enumeration command from `git ls-files` to `git ls-files --cached --others --exclude-standard`. This returns tracked + new-untracked-non-ignored files — the exact set `CommitAndPush` will commit — so the check no longer depends on whether files are staged at check time.
- [ ] Step 2: Update the doc comment on `VerifyPushPathsFilter` / `checkPushPathsFilter` (verify.go:142–146 and any inline wording) so it describes matching against "files the upcoming commit will include," not "tracked files" — the old wording implied the index and was the source of the bug.
- [ ] Step 3: In `internal/scaffolding/steps/verify_test.go`, add a test case that mirrors the real pipeline ordering: write `system/main.go` and a `commit-stage.yml` whose filter is `system/**`, **do not** `git add` them, and assert `checkPushPathsFilter` does not panic (the prior plain-`ls-files` code would have fataled here). Keep the existing matching/non-matching tests unchanged.
- [ ] Step 4: Run `go test -p 2 ./internal/scaffolding/steps/...` (Windows rule: never unbounded `go test ./...`) and confirm green.
- [ ] Step 5 (verification, operator): Re-run run 28290135277's workflow (the Smoke matrix) and confirm all 4 configs — monolith monorepo java, monolith multirepo dotnet, multitier monorepo ts, multitier multirepo java — pass.

## Notes

- Root cause pinned to `internal/scaffolding/steps/verify.go:202`; step ordering at `main.go:435–440` ("Verify push path filters", `phaseApplyTemplate`) vs `main.go:442–447` ("Commit and push", `phasePushScaffold`).
- The bug was masked by `verify_test.go`'s existing tests, which `git add .` + commit before calling the function — so they exercised the index-populated path the real pipeline never hits at check time.
- Keep the check in its current position (before push); the intent is to surface a misconfigured filter as a scaffold-time error before the bad workflow lands on GitHub. Do **not** move it after `CommitAndPush`.
