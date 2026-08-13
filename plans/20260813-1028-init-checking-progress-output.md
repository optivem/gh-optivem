# 2026-08-13 10:28:00 UTC — Print visible "checking..." progress lines for gh optivem init's Phase 2 GitHub-prerequisite checks

## TL;DR

**Why:** `gh optivem init`'s Phase 2 (`internal/config/config.go` `ParseAndValidate`, ~lines 1193-1212) runs `CheckOwnerExists`, `CheckProjectExists`, and `confirmReposExist` entirely silently — nothing is printed unless a check fails. This is inconsistent with the immediately-preceding `VerifyEnvironment` call, which prints one `log.Successf`/`log.Errorf` line per tool/compiler check (`  gh CLI: installed`, `  java: installed`, etc.) under a "Verifying environment..." header (`internal/config/token_auth.go` `verifyEnvironmentWithClient` + `runChecks`). A user watching `init` run sees a burst of environment-check lines, then a long silent gap before either Phase 3 starts or a FatalExit appears — no feedback on which owner/project/repo check is in flight.
**End result:** Phase 2's owner-exists, project-exists, and repo-availability checks each print a visible progress line (pass or fail) in the same style as the environment-verify section, so the terminal output shows continuous incremental feedback through the whole pre-flight sequence instead of a silent gap between "Verifying environment..." and Phase 3.

## Outcomes

- Running `gh optivem init` shows a line for the owner-exists check, the project-exists check (when `--project-url` is supplied), and each target-repo availability check, each resolving to a visible OK or FAIL — no more silent gap between environment verification and shop-ref resolution.
- The existing fail-fast behavior of `confirmReposExist` (added in `plans/20260813-1007-fix-confirm-repos-exist-dead-prompt.md` — `log.FatalExit` naming the pre-existing repo, no prompt) is preserved; the new progress line supplements it, not replaces it.
- Output format is consistent with the existing `check`/`runChecks` pattern in `token_auth.go`, so a future reader recognizes the same convention rather than a one-off format.
- `--project-url` empty (the common case — Path A auto-creates a board) produces no vacuous "project exists: OK" row.

## Design decisions

- **Section placement:** a new `log.Info("Checking GitHub prerequisites...")` block, printed right after `VerifyEnvironment` and before Phase 3 (shop-ref resolution). Kept separate from "Verifying environment..." — that section validates local toolchain/creds (same every run), this one validates this run's specific `--owner`/`--project-url`/`--repo` inputs against live GitHub state.
- **Mechanism:** `CheckOwnerExists` and `CheckProjectExists` are wrapped as `check{name, fn, okWord}` entries and run through the existing `runChecks` (`internal/config/token_auth.go`) — both already return `error`, a clean fit. `confirmReposExist` prints its own rows directly (`log.Successf`/`log.Errorf`, matching runChecks' visual style) rather than being forced into the `check` shape — it takes a slice of repos and needs its own aggregate FatalExit message, which doesn't map onto a single `check.fn() error`.
- **Repo row shape:** one row per candidate repo (main + backend/frontend/system, whichever are non-empty) — e.g. up to 4 rows for a multitier multirepo run. Each existing repo shows as a FAIL row; the existing aggregate `log.FatalExit` message (naming every pre-existing repo) still fires afterward unchanged — the row supplements it, doesn't replace it.
- **Row label format:** field name + value + result, e.g. `  Owner: valentinajemuovic — exists`, `  Project: <url> — exists`, `  Repo: valentinajemuovic/book-shop — available`. Deliberately deviates from `token_auth.go`'s generic tool-name style (`java: installed`) since these check per-invocation values, not fixed tool names — showing the value is the point.
- **Project-exists row:** skipped entirely when `--project-url` is empty (the common case — Path A auto-creates a board on first run) — no vacuous "OK" row for a check that didn't run.

## ▶ Next executable step (resume here)

Step 5 is deferred — no more agent-executable work remains. When the user is ready, they run `gh optivem init` themselves (or hand back a throwaway repo target) and visually confirm the new progress lines appear in the right place, in the right order, with the right wording — including the existing-repo FATAL case still working as before. Once confirmed, delete this plan file.

## Steps

- [ ] Step 5: Manually verify: run `gh optivem init` and visually confirm the new progress lines appear in the right place, in the right order, with the right wording — including the existing-repo FATAL case still working as before. — ⏳ Deferred: needs a real run against live GitHub; user to trigger when convenient.
