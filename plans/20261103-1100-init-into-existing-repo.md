# 2026-11-03 11:00 — `gh optivem init` into an existing repo (preview, warn, confirm)
> 🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-11-03T15:40:19Z`

## TL;DR

**Why:** `init` is greenfield-only. Since `9dd37403` (2026-08-13) `confirmReposExist` fails fast and `CreateRepo` fatals on an existing repo. Before that, a warn + `Proceed?` prompt existed but was dead code: the prompt said "may overwrite", yet `CreateRepo` fataled right after a "yes". It never previewed what would be overwritten. Teams adopting the pipeline on a repo they already have are blocked.
**End result:** `gh optivem init --overwrite` (explicit opt-in) scaffolds into an existing repo. It first prints a per-file and per-resource preview of what would be created, overwritten, or left alone, warns, and asks for confirmation. `--yes` skips the prompt but still prints the preview. Without `--overwrite`, the current fail-fast stays.

## Next step (resume here)

Decisions are settled (see Decisions below). Steps 2-3 are done (`--overwrite` flag, `confirmReposExist` returns the pre-existing set, `Config.PreExistingRepos`, `CreateRepos` skips them). Step 4 is done (`steps.BuildPreview(scaffoldDir, repoDir) ([]PreviewEntry, error)` in `preview.go`; renames report as NEW+KEPT, case-only matches carry `ExistingPath`). Start at Step 5.

## Design

**Opt-in is explicit.** A repo name typo must not silently scaffold into someone else's repo, and a genuine name collision must still fail loud (see `88300117`: only a *lost create attempt* is adopted). So the default stays fail-fast, with the error now pointing at `--overwrite`. `--yes` alone never implies it.

**Flag naming.** `--overwrite` (industry-conventional, cf. `copier`, `cookiecutter`) opts in to scaffolding into an existing repo, with the preview + confirm as the safety net. Alongside it add `--dry-run`: print the preview and exit without writing anything (the preview is built before any write, so this is cheap). `--yes` skips the prompt only. Note that `--overwrite` also covers creating NEW files and reusing remote resources, not just replacing files, so the `--help` text must say so.

**Preview (dry-run) comes before any write.** Run the template apply into the scratch dir (it already happens locally: clone shop, apply, replace), then diff against a clone of the existing repo's default branch. Classify every path:

| Class | Meaning | Action |
|---|---|---|
| NEW | in scaffold, not in repo | add |
| IDENTICAL | same bytes | no-op |
| OVERWRITE | in both, content differs | replace (listed, with `+/-` line counts; full diff via `--verbose`) |
| KEPT | in repo, not in scaffold | untouched |

The same classification applies to remote resources, each probed read-only: environments, variables/secrets, subtype labels, project board, SonarCloud project(s), branch protection if init sets any. Classes: create / reuse / differs (value changes shown, secret values never printed).

**Confirmation.** Show the summary counts, then the OVERWRITE list prominently (these are the destructive ones). Prompt through `internal/promptio` with no default. Non-TTY without `--yes` fails with an actionable message. With `--yes`, print the preview and proceed. Use the `approval` tier consistent with the other destructive prompts.

**Write path (decided):** push directly to the default branch after confirmation, same trunk-push flow as fresh repos. No branch/PR. Because git history is the only undo here, the OVERWRITE list in the confirmation must be prominent and `--dry-run` is the way to review first.

**Preconditions (fail loud, name the cause):**
- repo reachable, token has push access (403 and 404 are different errors, never folded into "not found")
- default branch exists and is non-empty, else treat as a normal fresh scaffold
- the local clone is clean

## Steps

- [x] Step 1: Resolve open questions (done, see Decisions).
- [ ] Step 5: Remote probes for environments, variables/secrets, labels, project board, Sonar. Each is read-only and each fails hard on an indeterminate response (auth/5xx), never "doesn't exist", per the check-* rule. Existing steps (`SetupEnvironments`, `SeedSubtypeLabels`, `SetupVariablesAndSecrets`, `EnsureProjectBoard`) must be idempotent on re-run, so audit and fix any that error on "already exists".
- [ ] Step 6: Render and confirm (and implement `--dry-run`: render, then exit 0 before any write). Print the summary + OVERWRITE list, prompt via `promptio`, honour `--yes`, and fail on non-TTY without `--yes`. Insert as a new phase "Preview" after "Apply template" and before "Push scaffold" in `buildSteps` (main.go). The local scaffold is already built by then, so the preview is free of remote writes.
- [ ] Step 7: Finalize for pre-existing repos: push to the default branch (no new write path). Allow a clean tree for pre-existing repos (re-running on an already-adopted repo exits 0 with "nothing to change", no commit), and keep clean-tree-is-an-error for fresh repos.
- [ ] Step 8: Verify stages run as for fresh repos, against the default branch after the push, honouring `--verify-level`. Local verify first.
- [ ] Step 9: Tests: `internal/config` (flag gating, fail-fast without flag), `steps` preview classes, finalize clean-tree rules, non-TTY behaviour. Update `--help` text (help-text-updater) and README.
- [ ] Step 10: Attended check against a throwaway repo that has (a) unrelated files, (b) a conflicting `README.md` and workflow, (c) pre-existing environments. Needs the user to name or approve a throwaway repo.

## Decisions

- **Write path:** push to the default branch after confirmation (no PR). Dropped the earlier branch+PR recommendation at the user's choice.
- **Re-run on an already-adopted repo:** idempotent no-op, exit 0, "nothing to change".
- **Line endings:** normalise CRLF/LF before comparing.
- **Verification:** local verify as today, remote stages per `--verify-level`, on the default branch (there is no PR branch).
- **Mixed existing/missing repos:** allowed, one combined preview; existing repos go through preview/confirm, missing ones are created.
- **Merging:** none in v1; overwrites are listed explicitly and the user can decline. Revisit if the OVERWRITE list is routinely noisy.
