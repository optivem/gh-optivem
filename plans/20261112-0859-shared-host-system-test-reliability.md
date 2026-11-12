# 2026-11-12 08:59 UTC — System tests and commits from a shared checkout / docker host

> Comes from shop's `plans/20261110-1207-system-test-reliability-followups.md`, ideas #36, #37, #38, #40, #42 and #45. Shop never edits gh-optivem; the shop-side follow-ups below wait for a gh-optivem release.

## TL;DR

**Why:** When two sessions share one checkout and one docker host (the normal setup for parallel agent work in shop), gh-optivem lets them get in each other's way, and then reports the result wrongly. On 2026-11-11 one session's `system start` replaced another session's running stack ("system stub is not running"). Two `system-test setup` runs at once corrupted `system-test/java/build`. `commit --paths` committed but could not push because someone else had unstaged changes. `--test` / `--suite` runs that passed were reported as "requested test(s) never executed", and `--suite X --test Y` ran other acceptance suites too. A TypeScript sample suite ran 0 tests after `npx` downloaded a different Playwright version.
**End result:**
- `system start` refuses, naming the owner and how to override, when a stack started from a different checkout/config is still running. `system-test setup` takes a lock, so two setups in one checkout can't run at once.
- `commit --paths` pushes even when the tree is dirty outside `--paths`: a plain push when there is nothing to pull, otherwise `pull --rebase --autostash`.
- `--suite` restricts the run to the named suites. `--test` accepts class-qualified names on .NET. The "never executed" check matches names the way the runner did (including `|` alternation). `--help` documents the filter semantics for each language.
- Playwright suites run the project-local binary. A missing local install fails loud ("run `gh optivem system-test setup`") instead of letting `npx` fetch whatever version is latest.

## ▶ Next executable step (resume here)

Resolve the two open questions below (each has a recommendation), then do Step 1. Steps 1 to 4 touch different files and can run in any order. Steps 1 and 3 both touch `internal/build/runner/`, so don't run them in parallel.

## Resolved decisions

1. **#36 + #45, one session per docker host, enforced:** `gh optivem system start` refuses (or warns loudly) when a stack for a different config is already running and was started by another process. Document "one session per docker host (and per checkout) for system tests". `system-test setup` takes a lock file so two setups can't run at once in one checkout. Decided 2026-11-12 by the user, from shop REL #36 (with #45 folded in).
2. **#37, `commit --paths` pushes from a dirty shared checkout:** when the tree is dirty only outside `--paths`, pull with `--autostash`, or fall back to a plain fast-forward push when there is nothing to pull. Decided 2026-11-12 by the user, from shop REL #37.
3. **#38 + #42, filter semantics:** document the filter semantics per language in `--help`. Make the "never executed" check use the same matching the runner used. Make `--suite` restrict the run, and support class-qualified names on .NET. Decided 2026-11-12 by the user, from shop REL #38 (with #42 folded in).
4. **#40, Playwright is resolved locally:** the runner calls the project-local Playwright binary instead of relying on `npx` resolution, so an unpinned download can't change the version and drop tests. (Shop's wording was "investigate if it recurs"; the fix is the part the user approved for this plan.) Decided 2026-11-12 by the user, from shop REL #40.

## Open questions

1. **#42: what stops `--suite X --test Y` from running other suites?** The extra suites come from the membership safety net (`classifyMissingNames` → `NamesExecutedIn` in `internal/build/runner/tests.go`). It runs the complement acceptance suites whenever a requested name looks missing. `test_commands.go` sets `MembershipProbeSuites` on every CLI `system-test run`, and so does `aggregate_commands.go`. The ATDD verify step relies on it to tell a cross-channel skip from a typo.
   - (a) Fix only the matching (Step 3a/3b). The probe then fires only on a real miss, but when it does it still runs suites outside `--suite`.
   - (b) Make the probe opt-in: a hidden `--membership-probe` flag that the ATDD verify command builder (`internal/atdd/process/actions/command.go`) passes. A manual `--suite` run never leaves its suites; a real miss fails loud as "never executed".
   - **Recommended: (b).** An explicit `--suite` should mean exactly those suites, and the ATDD verify step keeps its cross-channel skip because it asks for the probe explicitly.
2. **#40: where does "project-local Playwright" live?** Shop's `system-test/typescript/tests.yaml` (which scaffolded repos copy) writes `npx playwright ...` in every suite command, and gh-optivem runs those commands as given (`runShell`).
   - (a) The runner checks first: before a suite whose command invokes `playwright`, it requires `<testsCwd>/node_modules/.bin/playwright` (`.cmd` on Windows) and fails loud if it's missing. Shop's tests.yaml switches to `npx --no-install playwright` as a follow-up.
   - (b) The runner rewrites `npx playwright` to the local binary path.
   - **Recommended: (a).** It fails loud and names the fix. A silent command rewrite would hide what actually ran. `--no-install` is a second guard in the config.

## Steps

- [ ] Step 1: **#36/#45, stack ownership lock and `system-test setup` lock.**
  - Record the owner at `system start`: a small state file (the home is `internal/userstate/userstate.go`) holding the checkout root, the config path (`GH_OPTIVEM_CONFIG`), the compose project names and the start time. `Down` (`system stop` / `system clean`) clears it.
  - In `runner.Up` (`internal/build/runner/system.go`; the CLI is `system_commands.go` `start`), check before any `down`/`up`. If the recorded owner differs (other checkout or other config) and its containers are still running (`dockerCapture` / `composeRunningServices`), refuse. The error names the owner's config, checkout and start time and says how to override (`--take-over`, a new flag, or stop the other stack). If the containers are gone, the record is stale: reclaim it and say so. The same config from the same checkout counts as the owner, so behaviour is unchanged (the stale-images plan `plans/20261110-1115-system-start-stale-images.md` already made a healthy start rebuild in place; don't touch that logic). `aggregate_commands.go` calls `runner.Up` too, so it gets the check for free. Don't run this step in parallel with other work on `system.go`, as that plan also says.
  - In `runner.RunSetup` (`internal/build/runner/tests.go`; the CLI is `test_commands.go` `newTestSetupCmd`), create `<testsCwd>/.gh-optivem-setup.lock` exclusively (holding PID and start time). While another live holder has it, wait with a timeout and then fail loud naming the holder. Remove a lock whose PID is dead, and say so. Remove the lock on exit.
  - Docs: `docs/cli-reference.md` (`system start`, `system-test setup`) gets "one session per docker host / checkout for system tests", the refusal, and `--take-over`.
  - Tests: owner match / mismatch / stale in `system_test.go`, lock contention / stale PID in `tests_test.go`.
  - Shop follow-up after release: add the same rule to shop `CONTRIBUTING.md`, then delete shop ideas #36 and #45.

- [ ] Step 2: **#37, `commit --paths` pull with autostash or a fast-forward push.**
  - In `cross_repo_commands.go`: `pullAndPushRepo` → `pullRebaseUpstream`, and the push-retry path near the `pull --rebase` retry (around line 1115). For a `--paths` commit whose tree is still dirty afterwards (dirty only outside `--paths` by then):
    1. `git fetch <remote> <branch>`. If upstream has nothing new (`HEAD` already contains `<remote>/<branch>`), push directly with no pull.
    2. Otherwise run `git pull --rebase --autostash <remote> <branch>`. If the autostash pop conflicts, git keeps the stash. Report it loudly (stash ref, "the other session's changes are in the stash") and count the repo as committed-not-pushed. Never drop the stash.
  - A clean tree and the non-`--paths` path keep today's behaviour. Update the `commit` `Long` help text ("pull --rebase before …") to match.
  - Tests: nothing to pull + foreign dirty file → push without pull; upstream ahead + foreign dirty file → autostash pull keeps the foreign change in the working tree.
  - Shop follow-up after release: delete shop idea #37.

- [ ] Step 3: **#38/#42, `--test` / `--suite` filter semantics.**
  - **3a, alternation:** a `--test` value is substituted into the runner's filter as is, so `A|B` runs both tests, but `missingNames` / `nameExecuted` (`internal/build/runner/tests.go`) look for the literal `A|B`. For the presence check, split each requested value on `|` when the config joins with `or` / `fragment-or` (`TestFilterJoin`, `internal/build/runner/config.go`). Each part has to have run.
  - **3b, class-qualified names on .NET:** `bareMethodTRX` (`internal/build/runner/testnames.go`) drops everything up to the last `.`, and the shop .NET filter is `&DisplayName~<test>`, so `ViewProduct` (a class) matches nothing, and a shared method name pulls in another class's tests. Keep the fully qualified name next to the bare method in `namesTRX` (and the JUnit `classname` in `namesJUnit*`). When the token contains a `.`, match it against the qualified name. Shop follow-up: the .NET `testFilter` becomes `&FullyQualifiedName~<test>` (that matches `Class`, `Class.Method` and `Method` alike); check what the Java `--tests '*.<test>'` form already accepts.
  - **3c, `--suite` restricts:** per open question 1 (recommended (b)): `TestOptions.MembershipProbeSuites` is set only when the new hidden `--membership-probe` flag is passed (`test_commands.go`). The ATDD verify command builder (`internal/atdd/process/actions/command.go`) passes it. `aggregate_commands.go` keeps setting it, because it runs every suite anyway. When the probe runs, it prints which suites it runs and why.
  - **3d, `--help`:** the `--test` / `--suite` flag text in `test_commands.go` and `docs/cli-reference.md` say, per language, what a token matches: Java = method (`*.<test>`), .NET = substring of the qualified name (after 3b), TS = Playwright `--grep` regex on the title. They also say that `|` is alternation and that `--suite` never runs suites outside the selection.
  - Tests in `tests_test.go` / `testnames_test.go`: `A|B` with both run → pass; a .NET class token → match; `--suite X --test Y` with Y absent → "never executed", and no other suite runs.
  - Shop follow-up after release: the .NET `testFilter` change above, then delete shop ideas #38 and #42.

- [ ] Step 4: **#40, call the project-local Playwright binary.**
  - Per open question 2 (recommended (a)): in `runOneSuite` (`internal/build/runner/tests.go`), before a suite whose command invokes `playwright`, check for `<testsCwd>/node_modules/.bin/playwright` (`playwright.cmd` on Windows; see `runshell_windows.go` for how `.cmd` targets are resolved). If it's missing, fail loud: "local Playwright not installed in <testsCwd>; run `gh optivem system-test setup`". Don't let `npx` fetch one.
  - The same check in the scaffold verify step (`internal/scaffolding/steps/verify.go`, which mirrors `system-test setup`).
  - Tests: missing binary → that error, and the suite command is not run.
  - Shop follow-up after release: `system-test/typescript/tests.yaml` suite and setup commands use `npx --no-install playwright`. Then delete shop idea #40.
