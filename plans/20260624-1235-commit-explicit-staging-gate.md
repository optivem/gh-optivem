# 2026-06-24 12:35 CEST — `gh optivem commit`: gate the silent `add -A` sweep

## TL;DR

**Why:** `gh optivem commit` falls through to `git add -A` (`cross_repo_commands.go:349`) whenever `--paths` is not given. In `--yes` (scripted / agent) mode this silently stages **every tracked modification** in the working tree, including unrelated parallel-agent WIP. This bit run `c3c88400` in `shop`: a contract-suite-rename commit swept in 6 unrelated files (Prettier reformats, a `sqlite3` dep, a nock-import fix, and a `/refine-plan` marker) authored by other agents, landing them under a misleading message and pushing them to `main`.

The tool **already** treats this as a foot-gun for *untracked* files: under `--yes` it refuses untracked entries unless `--include-untracked` is passed ("the stray-file foot-gun is opt-in for scripted callers", `cross_repo_commands.go:330`). That guard simply doesn't extend to *tracked* modifications — which is exactly what a blanket `add -A` sweeps.

**End result:** The blanket `add -A` sweep becomes an explicit opt-in (`--all`) for non-interactive callers, symmetric to the existing `--include-untracked` design. A bare `gh optivem commit --yes "msg"` with no `--paths` and no `--all` **fails closed** with a message pointing the caller at `--paths` (surgical) or `--all` (deliberate sweep). Interactive use is unchanged — the human already sees the file list before confirming. Agent-facing docs and skills are updated to pass `--paths` (surgical) or `--all` (intentional sweep) explicitly.

## Outcomes

- **New `--all` flag.** Opt-in to the whole-tree `git add -A` sweep. No effect interactively (the human confirms against a printed file list either way).
- **`--yes` + no `--paths` + no `--all` → hard error**, with a message naming both escapes:
  - surgical: `--paths "<space-separated paths>"` (requires `--repo`)
  - deliberate sweep: `--all`
- **Interactive mode unchanged.** The y/N prompt against the printed `git status --short` list is itself the review gate, so no new flag is required there.
- **`--include-untracked` unchanged** and still composes (an `--all` sweep that also pulls in untracked files still needs `--include-untracked`).
- **Callers updated:** intentional sweeps (the `/commit` skill) add `--all`; agent-authored surgical commits switch to `--paths`.
- `go build ./...` and the commit-command unit tests pass; new tests pin the gate.

## End state of the tool — flag matrix

| Invocation | Before | After |
|---|---|---|
| interactive, no `--paths` | prompt → `add -A` | **unchanged** (prompt → `add -A`) |
| interactive, `--paths "…"` | prompt → `add -- …` | **unchanged** |
| `--yes`, `--paths "…"` | `add -- …` | **unchanged** |
| `--yes`, no `--paths`, no `--all` | silent `add -A` ⚠️ | **hard error** → use `--paths` or `--all` |
| `--yes --all`, no `--paths` | n/a | `add -A` (deliberate sweep) |
| `--yes`, untracked present, no `--include-untracked` | already refused | **unchanged** (still refused) |

Net: every non-interactive blanket stage is now a deliberate choice. Neither tracked nor untracked files can be swept silently.

## ▶ Status: EXECUTED 2026-06-24 (pending commit approval)

All steps complete. `go build ./...`, the commit-command suite, and the process package suite pass; the gate was demonstrated end-to-end with the built binary (bare `--yes` → two-escape error; `--yes --all` → commits). Flag name resolved to `--all` per recommendation. Files touched: `cross_repo_commands.go`, `cross_repo_commands_test.go`, `internal/atdd/process/process-flow.yaml`, `internal/atdd/process/clauderun/clauderun.go`, `internal/claude/assets/commands/commit.md`, `README.md`. `docs/process-diagram*.md` deliberately NOT touched — they regenerate from the YAML via the regenerate-diagram CI workflow on push.

## Steps

- [x] **Step 1 — Audit callers.** Grep the repo for `gh optivem commit` invocations and classify each:
  - `internal/claude/assets/commands/commit.md` (`/commit` skill: `gh optivem commit --yes $ARGUMENTS`) — **intentional sweep → add `--all`**.
  - `internal/claude/assets/commands/execute-plan.md`, `create-plan.md`, `fix-sonar-warnings.md` — agent-authored, should be **surgical → `--paths`** (note: `execute-plan.md:167` also still points at the stale `commit.sh`; fixing that is tracked separately — do not expand this plan to fix it, just don't reintroduce a sweep).
  - The ATDD wrapping-CLI commit step (where `command.go` / the runtime build the `gh optivem commit` shell-out) — determine whether it sweeps or is path-scoped; classify accordingly.
  - `README.md:350–353` examples + the flag reference.
  - Any `scripts/**` or workflow `.yml` sweep callers.
  - Record the final per-caller decision (`--all` vs `--paths`) before editing.
- [x] **Step 2 — Add `--all` flag + gate (`cross_repo_commands.go`).**
  - Add `opts.All` bound to a `--all` bool flag ("Stage all tracked changes (`git add -A`). Required with `--yes` when `--paths` is not given.").
  - In `commitOneRepo`, the non-`--paths` branch (currently lines 312–356): when `opts.Yes && !opts.All`, return an error mirroring the `--include-untracked` refusal — name both escapes (`--paths`, `--all`). Keep the existing `--include-untracked` untracked-refusal check ahead of it (so untracked guidance still fires first when relevant).
  - Leave the interactive path (`!opts.Yes`) reaching `add -A` unchanged.
- [x] **Step 3 — Help text (`newCommitCmd`).** Update `Use` (add `[--all]`), `Long` (document `--all` next to `--include-untracked` and state the `--yes`-without-`--paths`-or-`--all` refusal), and `Example` (show one `--all` sweep and keep the `--paths` example).
- [x] **Step 4 — Update intentional-sweep callers to `--all`.** `internal/claude/assets/commands/commit.md` → `gh optivem commit --yes --all $ARGUMENTS`. Any `scripts/**` sweep callers found in Step 1.
- [x] **Step 5 — Update surgical callers to `--paths`.** The agent-facing command docs from Step 1 — phrase commit guidance as "stage only the files you touched via `--paths`," cross-referencing `[[feedback_use_commit_skill]]` and `[[feedback_never_create_patches]]`.
- [x] **Step 6 — README.** Update the examples block (lines 350–353) to show `--all` for the sweep form and add a `--all` row/line to the flag reference.
- [x] **Step 7 — Tests.** Add unit coverage in the commit-command test file for: (a) `--yes` + no `--paths` + no `--all` → error naming both escapes; (b) `--yes --all` → sweep proceeds; (c) `--yes --paths` → unchanged; (d) interactive no-flags → unchanged. Run `go build ./...` and the commit tests.

## Open questions

- **Flag name `--all` vs `--sweep` vs `--add-all`.** `--all` is shortest and reads naturally against `--paths`, but could be misread as "all repos" (scope is already controlled by `--repo`/workspace mode). *Recommendation:* `--all` with a `Usage` string that says "all tracked changes" to disambiguate; revisit only if Step 1 finds a scope-`--all` collision elsewhere in the CLI.
- **Does the ATDD wrapping-CLI commit step already scope its stage?** If the runtime builds its `gh optivem commit` shell-out with `--paths` (or commits only a known artifact set), it needs no change and the gate is transparent to it. Step 1 confirms; if it relies on the bare sweep, it moves to `--all` with a comment explaining why the ATDD commit is legitimately whole-tree.

## Dependencies

None. Independent of the in-flight `20260624-0925` / `20260624-0953` cascade plans (different files — `cross_repo_commands.go` + skill docs vs `process-flow.yaml` + gate bindings). Safe to execute in parallel.

## Verification (operator)

- After Step 7: run `gh optivem commit --yes --repo <somerepo> "test"` against a dirty tree with no `--paths`/`--all` and confirm it errors with the two-escape message instead of committing.
- Confirm `/commit` (now `--yes --all`) still sweeps the workspace as before.

## References

- `[[feedback_use_commit_skill]]` — `/commit` sweeps; surgical agent commits use explicit file lists.
- `[[feedback_never_create_patches]]` — stage whole files, but only the files you name.
- `[[feedback_concurrent_agent_collision]]` — parallel-agent WIP is exactly what the sweep absorbs; re-check HEAD before staging.
- Existing precedent: `--include-untracked` opt-in for untracked files (`cross_repo_commands.go:326–340`) — this plan extends the same philosophy to tracked modifications.
- Incident: `shop` commit `c3c88400` (range `978103e8..c3c88400`) swept 6 unrelated files into a contract-rename commit.
