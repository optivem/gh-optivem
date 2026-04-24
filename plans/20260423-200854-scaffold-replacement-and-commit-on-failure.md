# Plan: Fix scaffold bugs (replacement gaps + skipped release stages) + enable debug commit on FATAL

## Motivation

On 2026-04-23 a course-tester run invoked:

```bash
gh optivem init --owner valentinajemuovic --system-name "Blue Horizon" \
  --repo course-tester-atdd-typescript-20260423-192402 \
  --repo-strategy monorepo --arch monolith --lang typescript
```

Step 8 "Replace system name" FATAL'd with:

```
WARN Leftover template name "shop" found in 3 file(s) after replacement:
WARN   .github\workflows\commit-stage.yml
WARN   system-test\docker-compose.pipeline.monolith.real.yml
WARN   system-test\docker-compose.pipeline.monolith.stub.yml
FATAL: System name replacement incomplete: "shop" still present in scaffolded repo.
```

CLI auto-filed optivem/gh-optivem#29.

Five distinct problems surfaced:

1. **Replacement gap** — `shop-system` / `shop` remain in 3 specific files after the system-name pass. Verified by inspecting the preserved scaffold dir at `%TEMP%\scaffold-3732598035\repo\`; e.g. [commit-stage.yml:115-116](../../gh-optivem/internal/steps/replacements.go) shows `optivem_shop-system` and `shop-system` intact.
2. **Env-name replacement gap** — user flagged that environment names (`monolith-<lang>-<stage>`) also aren't getting rewritten in the scaffolded repo. Needs reproduction.
3. **"Acceptance stage passed" but "No RC version found"** — after the `shop` bug was worked around, a full `--verify-level release` run reported the acceptance stage as passing (`OK Acceptance stage passed!`) but immediately emitted `WARN No RC version found — acceptance stage may have skipped promotion (e.g. no artifacts yet)`. Either the stage truly passed (and should have produced an RC), or it didn't (and shouldn't have been reported as "passed"). The two lines are self-contradictory and make it impossible to tell whether the release pipeline is actually working.
4. **QA + production stages silently skipped on `--verify-level release`** — as a direct consequence of #3, Steps "QA stage", "QA signoff", and "production stage" all printed `WARN Skipping ... — no RC version available` and then `OK Step N done`. The operator asked for release-level verification and got 0 of 3 release-gate stages actually run — but the overall run ended with `OK All steps passed!`. Silent skipping here means `--verify-level release` provides no guarantee it exercised the release pipeline.
5. **No debug state on the remote** — when any pre-push step FATALs (Steps 4-11), the remote GitHub repo stays at its initial state (README + LICENSE only) because "Commit and push" is Step 12. The local `scaffold-*` temp dir is preserved (because [log.Fatal](../internal/log/log.go#L58-L61) uses `panic` not `os.Exit`), but remote is empty — hard to share, hard to diff, hard to triage without shell access to the operator's `%TEMP%`. The related plan [20260423-200000-scaffold-output-and-step-order.md](20260423-200000-scaffold-output-and-step-order.md) reorders Commit and push to Step 14 — which makes this worse, not better.

Also: **why CI didn't catch #1** — [gh-commit-stage.yml:33-34](../.github/workflows/gh-commit-stage.yml#L33-L34) runs only `go test ./...` (no unit coverage for the no-leftover scan); [gh-acceptance-stage.yml:19](../.github/workflows/gh-acceptance-stage.yml#L19) default matrix is `condensed` = 12 off-diagonal polyglot combos, which explicitly excludes single-language combos (TS-TS, JV-JV, NET-NET) on the assumption `optivem/shop` covers them.

## Items

### Issue 1 — Fix `shop` → system-name replacement gap (optivem/gh-optivem#29)

- [ ] **Identify the missing cases** in [internal/steps/replacements.go](../internal/steps/replacements.go). The pass misses `shop-system` as a compound token inside Sonar args and docker-compose contexts:
  - `.github/workflows/commit-stage.yml` — `-Dsonar.projectKey=optivem_shop-system` and `-Dsonar.projectName=shop-system`
  - `system-test/docker-compose.pipeline.monolith.real.yml`
  - `system-test/docker-compose.pipeline.monolith.stub.yml`
- [ ] Extend the replacement logic (likely the Pass 5 suffix-dedupe referenced at [cleanup.go:19-21](../internal/steps/cleanup.go#L19-L21)) so `shop-<suffix>` forms like `shop-system` are rewritten to `<kebabName>-system`.
- [ ] Verify with a fresh `gh optivem init --lang typescript --system-name "Blue Horizon"` end-to-end; rebuild `gh-optivem.exe`.

### Issue 2 — Environment name replacement gap [NEEDS REPRO]

User flagged that **environment names aren't updated** in the scaffolded repo either. Symptom and exact location TBD.

- [ ] **Reproduce:** after Issue 1 is fixed, do a fresh TS scaffold and grep the resulting repo for leftover env references matching `monolith-(java|dotnet|typescript)-(acceptance|qa|production)` or `multitier-...-(acceptance|qa|production)`. Confirm which files / which patterns remain unchanged.
- [ ] **Decide scope:**
  - If the finding matches the design discussed in Group 3 of [20260423-200000-scaffold-output-and-step-order.md](20260423-200000-scaffold-output-and-step-order.md) (drop the arch+lang prefix entirely), merge this item into that plan instead of duplicating.
  - If it's a separate replacement bug (e.g. wrong lang prefix for the chosen scenario), treat like Issue 1 — fix the replacement pass and add a fixture to the Issue 6 test.

### Issue 3 — "Acceptance stage passed" contradicts "No RC version found"

Observed during a `--verify-level release` run (full output in the appendix of [20260423-200000-scaffold-output-and-step-order.md](20260423-200000-scaffold-output-and-step-order.md)):

```
OK Acceptance stage passed!
WARN No RC version found — acceptance stage may have skipped promotion (e.g. no artifacts yet). Downstream stages will be skipped.
OK Step 16 done (5m 15s)
```

These three lines are self-contradictory. Either the acceptance stage actually ran and passed end-to-end (including promotion to an RC) and the warning is a false alarm — OR it bailed early without producing an RC, in which case `OK Acceptance stage passed!` is wrong.

- [ ] **Trace the `VerifyAcceptanceStage` implementation** (referenced from [main.go:150](../main.go#L150)) to find where the pass/fail decision is made and where the RC-version lookup happens. Determine whether "passed" means "workflow conclusion was success" (which can be true while the promotion-gate job was skipped inside the workflow) or something stronger.
- [ ] **Decide the correct semantics:**
  - If the acceptance workflow is allowed to skip promhttps://github.com/optivem/hub/discussionsotion (e.g. because there are no new artifacts), "passed" should be renamed to `OK Acceptance stage completed (promotion skipped)` and Step 16 should not end on an OK/WARN pair that implies both.
  - If promotion is *required* at `--verify-level release`, a missing RC should fail the step, not warn.
- [ ] **Update the log line in verify.go** so "passed" is only printed when both the workflow succeeded *and* an RC was produced (at release level).
- [ ] **Log `gh run view <run-id>` output after each stage passes.** [RunWatchWorkflow](../internal/shell/github.go#L397) already resolves the run's `databaseId`; return it alongside the error so callers can capture it. After `reportParallelResult` prints "passed", call `gh run view <run-id> --repo <repo>` and log its output (job/step status table + web URL). This turns the one-line "passed" into a visible breakdown of which jobs actually ran vs were skipped — which would have immediately explained the "RC exists but not detected" case on run 24884129262. Apply to every `Verify*` step that watches a workflow (commit, acceptance latest, acceptance legacy, QA, QA signoff, release, production).

### Issue 4 — QA + production stages silently skipped on `--verify-level release`

Direct downstream consequence of Issue 3:

```
> Step 14: Triggering and verifying QA stage...
WARN Skipping QA stage — no RC version available
OK Step 19 done (0s)
> Step 15: Triggering and verifying QA signoff...
WARN Skipping QA signoff — no RC version available
OK Step 20 done (0s)
> Step 16: Triggering and verifying production stage...
WARN Skipping production stage — no RC version available
OK Step 21 done (0s)
```

The user asked for `--verify-level release` — meaning "verify all the way to production deploy." They got **zero of three release-gate stages actually executed**, but the overall run terminated with `OK All steps passed! Completed in 15m 30s`. The `--verify-level release` contract is broken: operators have no way to tell from the exit status whether release-gate verification actually ran.

- [ ] **Decide policy for `release` verify-level when no RC exists:**
  - **Option A (recommended):** fail fast. If `--verify-level release` and no RC is produced by the acceptance stage, exit non-zero with a clear message: "verify-level=release requires an RC; acceptance did not produce one." Operators can downgrade to `--verify-level acceptance` if they know no RC will be produced.
  - **Option B:** print a prominent non-warning summary at the end — e.g. change the final summary from `OK All steps passed!` to `OK Steps passed (N/M release-gate stages skipped)` with a count.
- [ ] **Ensure the exit code reflects reality.** Today, skipping a release-gate step and succeeding the step both yield exit 0. At minimum, introduce a `SKIP` step state distinct from `OK` so the orchestrator's final summary can count skipped release-gate stages and refuse to print "All steps passed" if any release-gate step was skipped at `--verify-level release`.
- [ ] **Investigate whether the RC-not-produced situation is itself a bug.** In a brand-new scaffolded repo with verify-level=release, the expected happy path is: acceptance stage produces an RC → QA uses it → prod uses it. If the RC isn't produced for a freshly scaffolded repo, something in the default shop template / workflow gating is wrong for first-run scenarios. Escalate to a separate ticket if root cause is outside gh-optivem (e.g. in `optivem/shop` or `optivem/actions`).

### Issue 5 — Add `--commit-on-failure` flag for debug ergonomics

Today when Step 4-11 panics, the remote repo has only the initial README + LICENSE. Only the operator's local `%TEMP%\scaffold-*` dir holds the partial state — not shareable, not linkable from a bug report.

- [ ] **Add a `--commit-on-failure` boolean flag** (default `false`) to `init` in [config.go](../internal/config/config.go).
- [ ] In the top-level step runner at [main.go:173-192](../main.go#L173-L192), on `recover()`: if `cfg.CommitOnFailure && cfg.RepoDir != ""` and the working tree has changes, commit with a message like `debug: partial scaffold (failed at "<step.name>")` and push to a `debug/<timestamp>` branch (so the main branch remains clean).
- [ ] **Include the debug branch URL** in the auto-filed bug report from `createBugReport` (currently linked in [docs/how-it-works.md:21](../docs/how-it-works.md#L21)).
- [ ] Document the flag in `README.md` under troubleshooting.

### Issue 6 — Add unit test to prevent replacement regressions

Moves the "no leftover `shop`" check from `-tags=system` (acceptance-stage only) into default `go test ./...` (commit-stage).

- [ ] **Add `internal/steps/replacements_test.go`** that:
  1. Seeds a temp dir with a small fixture covering the known failure surface: the 3 files from Issue 1, plus a representative Sonar config line, a test-config YAML, and a Dockerfile. Fixtures should be checked into `testdata/`.
  2. Calls the full replacement chain with a chosen system name (e.g. `"Blue Horizon"`).
  3. Walks the temp dir post-replacement and asserts no `shop`, `Shop`, or `SHOP` substring remains (with a small allowlist for any legitimately preserved tokens, if any exist).
- [ ] Table-drive over system names that exercise different casing outcomes: single word, two words, hyphenated, numeric suffix.
- [ ] If Issue 2 lands here (rather than merging into the sibling env-prefix-rename plan), extend the same test to assert no leftover `monolith-<lang>-` / `multitier-<lang>-` env-name fragments.

## Out of scope

- The step-reorder / env-prefix simplification / output verbosity cleanup in [20260423-200000-scaffold-output-and-step-order.md](20260423-200000-scaffold-output-and-step-order.md). Complementary but tracked separately. Note: **that plan's Group 1 reorder pushes "Commit and push" later (to Step 14)**, which worsens the debug-state problem this plan's Issue 5 fixes — so Issue 5 should land **before or with** that reorder.
- Changing `gh-acceptance-stage` default matrix from `condensed` to `full`. Considered; dropped because Issue 6's unit test is cheaper coverage for the same regression class.

## Order of execution

1. **Issue 1 + Issue 6** in the same PR. Fix the `shop` bug and add the test that would have caught it.
2. **Issue 2** next — reproduce env-name symptom on a clean scaffold (only meaningful once Issue 1 is in), then fix or merge into the sibling env-prefix-rename plan.
3. **Issue 3 + Issue 4** together — they share root cause (the RC-production gate) and the fix for one clarifies the other. Start by tracing `VerifyAcceptanceStage` to pin down the semantics.
4. **Issue 5** last — debug ergonomics, independent of the bug fixes.
