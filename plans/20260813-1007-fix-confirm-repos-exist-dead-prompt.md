# 2026-08-13 10:07:00 UTC — Fix `gh optivem init` warning-and-prompting for an already-existing repo when the run can no longer succeed

🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-08-13T10:11:21Z`

## TL;DR

**Why:** Issue #60's fix (commit `e98a4af0`) made `GitHub.CreateRepo()` (`internal/kernel/shell/github.go:432-433`) unconditionally `log.Fatalf` when the target repo already exists — by explicit design, no `--force`/opt-in. But it left the earlier preflight gate, `confirmReposExist()` (`internal/config/config.go:1117-1166`, called from Phase 2 at `config.go:1241`), unchanged: it still `log.Warnf`s and prompts `Proceed? [y/n]:` (or silently proceeds under `--yes`), implying a "yes" answer can succeed. It cannot — `internal/scaffolding/steps/github_setup.go:27` (and lines 35/38/42 for multirepo/multitier) unconditionally calls `gh.CreateRepo()` for every configured repo, so any repo that trips the prompt is guaranteed to hit the fatal moments later, after burning time on env/project checks, shop-ref resolution, and cloning.
**End result:** `gh optivem init` against an already-existing target repo fails immediately in Phase 2 — before shop-ref resolution or cloning — with a clear message naming the repo(s), and no misleading "Proceed?" prompt. Now-dead code paths left over from the old warn+confirm+continue behavior (`PreExistingRepos` plumbing) are removed.

## Outcomes

- Running `gh optivem init` against an already-existing target repo (or any component repo in multirepo/multitier) aborts immediately in Phase 2 with a clear, actionable error naming the repo(s) — no prompt, no wasted work.
- `--yes` no longer documents skipping an "existing-repo prompt" that no longer exists.
- `Config.PreExistingRepos` and its only consumer (`finalize.go`'s conditional `commitAndPushRepo` argument) are removed, since a pre-existing repo can never reach `finalize()` anymore.
- Tests reflect the new fail-fast behavior (existing repo → immediate fatal, no prompt) instead of the old warn+confirm+continue behavior.

## ▶ Next executable step (resume here)

Step 8: Manually verify: run `gh optivem init` against an already-existing throwaway repo (not `book-shop`) and confirm it fails immediately in Phase 2 with a clear message naming the repo, no `Proceed?` prompt shown. This needs a real throwaway GitHub repo to target — requires the user to say which repo (or confirm creating one) before it can run, since it's a real network operation against GitHub.

## Steps

- [ ] Step 8: Manually verify: run `gh optivem init` against an already-existing throwaway repo (not `book-shop`) and confirm it fails immediately in Phase 2 with a clear message naming the repo, no `Proceed?` prompt shown.

## Open questions

None — this completes the original plan's already-agreed decision (`plans` history: `20260813-0850-init-repeat-run-repo-corruption-guard.md`, "recommended: unsupported for now — smallest safe fix") that `CreateRepo()` should fail outright on an existing repo with no `--force`. This plan just finishes wiring that decision through the earlier preflight gate that was left stale.
