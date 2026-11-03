# 2026-11-03 11:00 — `gh optivem init` into an existing repo (preview, warn, confirm)

## TL;DR

**Why:** `init` is greenfield-only. Since `9dd37403` (2026-08-13) `confirmReposExist` fails fast and `CreateRepo` fatals on an existing repo. Before that, a warn + `Proceed?` prompt existed but was dead code: the prompt said "may overwrite", yet `CreateRepo` fataled right after a "yes". It never previewed what would be overwritten. Teams adopting the pipeline on a repo they already have are blocked.
**End result:** `gh optivem init --overwrite` (explicit opt-in) scaffolds into an existing repo. It first prints a per-file and per-resource preview of what would be created, overwritten, or left alone, warns, and asks for confirmation. `--yes` skips the prompt but still prints the preview. Without `--overwrite`, the current fail-fast stays.

## Next step (resume here)

Decisions are settled (see Decisions below). Steps 2-3 are done (`--overwrite` flag, `confirmReposExist` returns the pre-existing set, `Config.PreExistingRepos`, `CreateRepos` skips them). Step 4 is done (`steps.BuildPreview(scaffoldDir, repoDir) ([]PreviewEntry, error)` in `preview.go`; renames report as NEW+KEPT, case-only matches carry `ExistingPath`). Step 5 is done (`steps.ProbeRemote(cfg, RemoteTargets) ([]RemoteEntry, error)` in `remote_preview.go`: environments, labels, variables/secrets, project board, Sonar, classes create/reuse/differs; repos not in `PreExistingRepos` are all-create with no calls; every indeterminate response returns an error naming the resource and likely fix; existing secrets report `differs` since values are unreadable; `variableSecretPlan` is shared with `SetupVariablesAndSecrets`; reused project boards now get an additive Status merge instead of a wholesale replace, which would reset card statuses). Step 6 is done: `--dry-run` flag (`Config.DryRun`; requires `--overwrite` and every target repo to exist, since creating a missing repo is a remote write); `steps.CaptureExistingBaselines` (after "Clone repos": push-access check with distinct 404/403/401 errors, clean-clone check, parks existing files in `<dir>.baseline` and empties the working tree so the template renders as a pure scaffold; empty repos get an empty baseline = fresh scaffold); `steps.RunPreview` in `preview_confirm.go` (renders counts, prominent OVERWRITE list with +/-, remote classes; full diffs and NEW list only with `--verbose`; `--yes` prints and proceeds; non-TTY without `--yes` errors; otherwise `approval.Confirm` at `CategoryHuman`, no default; on proceed restores KEPT files from the baseline); `withPreview` in main.go inserts the "Preview" phase before "Push scaffold" and, for pre-existing runs only, moves the remote-write steps (project board, environments, labels, variables/secrets, Sonar projects) behind it as phase "Setup remote resources" and re-runs `WriteOptivemYAML` there so the board URL is baked in. Step 7 is done: `stageCommitAndPush` in `finalize.go` (returns pushed=false with no commit for a clean tree in a `PreExistingRepos` repo, "Nothing to change" message, exit 0; clean tree in a fresh repo still errors; same trunk push to main). Step 8 is done: verify stages needed no ordering/level changes (local verify runs on the merged tree, remote stages stay gated by `--verify-level`), but three gaps closed. `run_scope.go`: for pre-existing repos the push step records the repo's newest run ID before pushing (`recordRunBaseline`, fatal if unreadable) and the verify watchers only accept runs newer than it (`GitHub.MinRunID`/`WithMinRunID`), so stale runs from before the scaffold are never reported as its verdict; a no-op push marks the repo unchanged and the push-triggered commit-stage verification is skipped with an explicit warning (dispatched acceptance/QA/prod/cleanup still run on the default branch); `checkRepoAccess` now fails loud when the default branch is not `main`, since the push and workflows target `main`. Step 9 is done (tests, `--help`, README, cli-reference). Only Step 10 remains: idempotent re-run check and throwaway cleanup (see the step).

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
- [ ] Step 10: Attended check, nearly done. Throwaway repo `valentinajemuovic/init-overwrite-throwaway` (private; unrelated files, conflicting README + `commit-stage.yml`, pre-existing env `preexisting-env`). Dry-run (466 new, 2 OVERWRITE, 14 remote creates) and a real run (`--overwrite --verify-level local`, exit 0) both passed; afterwards the repo had the scaffold commit on `main`, `acceptance`/`qa`/`production` envs alongside `preexisting-env`, and `notes/todo.txt` kept. Remaining: (a) re-run the same init command and confirm the idempotent "Nothing to change" no-op (the attempt was blocked: Windows Defender flagged the Go build output as a virus, rerun once that is cleared); (b) the user deletes the repo and the 2 SonarCloud projects (`valentinajemuovic_init-overwrite-throwaway-system`, `-system-test`); the token lacks `delete_repo`.

## Decisions

- **Write path:** push to the default branch after confirmation (no PR). Dropped the earlier branch+PR recommendation at the user's choice.
- **Re-run on an already-adopted repo:** idempotent no-op, exit 0, "nothing to change".
- **Line endings:** normalise CRLF/LF before comparing.
- **Verification:** local verify as today, remote stages per `--verify-level`, on the default branch (there is no PR branch).
- **Mixed existing/missing repos:** allowed, one combined preview; existing repos go through preview/confirm, missing ones are created.
- **Merging:** none in v1; overwrites are listed explicitly and the user can decline. Revisit if the OVERWRITE list is routinely noisy.
