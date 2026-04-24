# Plan: Scaffold output improvements + step-sequence cleanup

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-04-24T06:36:15Z`

## Motivation

During a 2026-04-23 scaffold run of `course-tester-atdd-typescript-20260423-192402`, the user surfaced a cluster of related issues with the `gh optivem init` console output and step ordering:

- Output is information-poor: created repo URL, clone path, secret/var names, SonarCloud URLs, and the workflow run URL being watched were all missing.
- SonarCloud "OK" lines don't distinguish *created* vs *already existed*.
- Environment names carry an arch+lang prefix (`monolith-typescript-acceptance`) that is redundant inside a single scaffolded repo.
- No verbosity control, no log-file output, no convention-aware color handling (ANSI codes emitted unconditionally).
- Local validation (compile, leftover-name scan) runs **after** push, so broken code lands in remote history.
- Hardcoded `"Step N:"` strings inside step functions drift out of sync with the orchestrator's loop index — visible in the run as "Step 8" appearing twice.

## Already done (do not redo)

These five edits compiled clean during the conversation; they're listed so the next pass can verify, not re-implement:

- [github_setup.go:13-21](../internal/steps/github_setup.go#L13-L21) — `logCreated` prints repo URL; `logCloned` takes `localPath` and prints `-> <path>`.
- [github_setup.go:106-115](../internal/steps/github_setup.go#L106-L115) — `setSecret` / `setVariable` helpers print `secret: NAME` / `variable: NAME` (no values).
- [sonarcloud.go:98-159](../internal/shell/sonarcloud.go#L98-L159) — `CreateOrg` and `CreateProject` distinguish `(created)` vs `(already existed)`; `CreateProject` prints project URL.
- [github.go:387](../internal/shell/github.go#L387) — `RunWatchWorkflow` prints `Watching workflow run: https://github.com/<repo>/actions/runs/<id>` before `gh run watch`.

## Items

### Group 1 — Step sequence cleanup (depends on nothing else)

### Group 2 — Logging refactor (verbosity + log-file + color convention)

Recommended single coordinated change in [internal/log/log.go](../internal/log/log.go):

### Group 3 — Environment naming (local to gh-optivem)

`shop` **keeps** prefixed env names (`monolith-typescript-acceptance` etc.) since it hosts templates for all arch+lang combos side-by-side. Scaffolded repos — which only ever have one arch+lang combo — should emit **unprefixed** env names (`acceptance`, `qa`, `production`).

Two coordinated changes inside `gh-optivem`:

- [ ] **`SetupEnvironments`** in [github_setup.go:58-72](../internal/steps/github_setup.go#L58-L72) — create bare env names. Simplify:
  ```go
  for _, stage := range []string{"acceptance", "qa", "production"} {
      gh.CreateEnvironment(stage)
  }
  log.Success("Created environments: acceptance, qa, production")
  ```

- [ ] **Workflow content rewrite.** After template files are copied into the scaffolded repo, rewrite every `environment: <arch>-<lang>-(acceptance|qa|production)` reference to `environment: $1`. Integrate into the existing `FixupWorkflowContent` pipeline in [apply_template.go](../internal/steps/apply_template.go). Applies to all 4 variants (monolith/multitier × monorepo/multirepo). For multitier, rewrite against `<arch>-<backend-lang>-...` (since env names use backendLang).

- [ ] **Verify with a fresh scaffold + commit-stage run** that a freshly scaffolded repo's workflows reference the unprefixed env names and CI succeeds end-to-end.

**Deferred (follow-up, out of scope for this plan):** 23 course docs under `courses/01-pipeline/` and `courses/02-atdd/` reference the old prefixed names. Easy regex sweep; will be done after the code lands and is verified.

## Order of execution

1. **Group 1 first.** Pure refactor, no API/UX implications, makes subsequent debugging cleaner.
2. **Group 2 second.** Independent of Group 3, useful in its own right, makes Group 3's verification easier (log file = paper trail of the cross-repo change).
3. **Group 3 last.** Needs both repos changed in lockstep; do a coordinated commit per repo and verify with a scaffold-and-watch end-to-end run.

## Out of scope

- Restructuring the verify-tier flag (`--verify-level`). Current shape is fine.
- Reworking the bug-report creation path in `createBugReport`. Out of scope for this output-cleanup pass.
- Changing the SonarCloud branch-rename behavior. Tangential.

## Appendix: Actual run output — 2026-04-23 (TS monolith-monorepo, `--verify-level release`)

Captured from a full 15m 30s successful end-to-end run (after the `shop` replacement gap from [20260423-200854-scaffold-replacement-and-commit-on-failure.md](20260423-200854-scaffold-replacement-and-commit-on-failure.md) Issue 1 was worked around). Kept here as concrete evidence for the motivation bullets above and as a baseline to diff future output against.

### Observations

Numbers reference the step index printed by each line (`> Step N:` from inside the step function vs `OK Step N done` from the orchestrator's loop counter).

1. **Hardcoded `"Step N:"` labels are out of sync with the loop counter.** The hardcoded numbers appear to have been written against an older step list. Every hardcoded number is wrong from Step 9 onward:
   - `> Step 8: Replacing system name...` → `OK Step 8 done` ✓ (correct)
   - `> Step 8: Generating README...` → `OK Step 9 done` (hardcoded label duplicates Step 8; real index is 9)
   - `> Writing project config...` → `OK Step 10 done` (no hardcoded number at all)
   - `> Step 9: Creating SonarCloud projects...` → `OK Step 11 done` (hardcoded says 9, real is 11)
   - `> Step 10: Committing and pushing...` → `OK Step 12 done`
   - `> Validating no leftover system names...` → `OK Step 13 done` (no hardcoded number)
   - `> Verifying local compilation...` → `OK Step 14 done` (no hardcoded number)
   - `> Step 11: Verifying commit stage workflow...` → `OK Step 15 done`
   - `> Step 12: Triggering and verifying acceptance stage...` → `OK Step 16 done`
   - `> Step 13: Triggering and verifying acceptance stage legacy...` → `OK Step 17 done`
   - `> Step 17: Running local system tests...` → `OK Step 18 done` (hardcoded jumped from 13 → 17)
   - `> Step 14: Triggering and verifying QA stage...` → `OK Step 19 done`
   - `> Step 15: Triggering and verifying QA signoff...` → `OK Step 20 done`
   - `> Step 16: Triggering and verifying production stage...` → `OK Step 21 done`
   - `> Generating project registration info...` → `OK Step 22 done` (no hardcoded number)

   **Fix is already in Group 1 Item 2** of this plan — strip all hardcoded `"Step N:"` prefixes from step functions.

2. **Env names carry the arch+lang prefix** (Group 3 of this plan):
   `OK Created environments: monolith-typescript-acceptance, monolith-typescript-qa, monolith-typescript-production`

3. **Two `OK Applied template files` lines in a row** on Step 5 — the second is a redundant summary after the first reported the specific `(monolith monorepo)` variant. Minor cosmetic.

4. **Step ordering**: Step 2 creates environments and Step 3 sets secrets/variables, but neither can be verified until Step 4 clones. More importantly, SonarCloud project creation (Step 11) and push (Step 12) happen after local validation (Steps 13-14 as ordered today), which violates the "validate-before-side-effect" principle — **Group 1 Item 1 of this plan already proposes reordering**; this output confirms why.

5. **`valentinajemuovic_course-tester-atdd-typescript-20260423-192402-system`** is the Sonar project key. The `-system` suffix is an internal convention ([cleanup.go:22-28](../internal/steps/cleanup.go#L22-L28)) that's not obvious from the output. Consider printing the human-friendly name alongside: `SonarCloud project: … (suffix: -system for monolith-monorepo)`.

6. **Skipped downstream stages on `release` verify-level** when no RC exists:
   ```
   WARN No RC version found — acceptance stage may have skipped promotion...
   WARN Skipping QA stage — no RC version available
   WARN Skipping QA signoff — no RC version available
   WARN Skipping production stage — no RC version available
   ```
   These WARN-then-OK sequences are confusing — the step prints `WARN Skipping...` then immediately `OK Step N done`. Consider distinguishing "skipped" from "passed" in the orchestrator's summary line (e.g. `SKIP Step 19 skipped (0s)` instead of `OK`).

7. **Good signals worth preserving** (for regression watch after the Group 2 log rewrite):
   - Repo/Actions/SonarCloud URLs printed in the "Project Registration Info" block ✓
   - Per-pass replacement counts on Step 6/7/8 (e.g. `Pass 1: replaced optivem/shop -> … (4 files)`) — very debuggable ✓
   - Timestamp/duration per step (`Step N done (Xs)`) ✓
   - Final summary with total duration ✓

### Raw output

```
valen_4rjvn9e@Valentina_Desk MINGW64 /c/GitHub/optivem/academy/courses (main)
$   gh optivem init --owner valentinajemuovic --system-name "Blue Horizon" \
    --repo course-tester-atdd-typescript-20260423-192402 \
    --repo-strategy monorepo --arch monolith --lang typescript
OK Cloned shop to C:\Users\VALEN_~1\AppData\Local\Temp\shop-2367812557 (pinned to e0f79e96bb68c8d5bc0f5f5b64fe5c424bd09b50)

==========================================
  Pipeline Project Setup
==========================================

> Owner:       valentinajemuovic
> Repo:        course-tester-atdd-typescript-20260423-192402
> System:      Blue Horizon
> Arch:        monolith
> Language:    typescript
> Test lang:   typescript
> Dry run:     false
> Test mode:   false
> Workdir:     C:\Users\VALEN_~1\AppData\Local\Temp\scaffold-1389983804

> Step 1: Creating repository valentinajemuovic/course-tester-atdd-typescript-20260423-192402...
WARN Repository valentinajemuovic/course-tester-atdd-typescript-20260423-192402 already exists -- skipping creation
OK Created repository: valentinajemuovic/course-tester-atdd-typescript-20260423-192402
OK Step 1 done (1s)
> Step 2: Creating environments...
OK Created environments: monolith-typescript-acceptance, monolith-typescript-qa, monolith-typescript-production
OK Step 2 done (1s)
> Step 3: Setting secrets and variables...
OK Set secrets and variables
OK Step 3 done (4s)
> Step 4: Cloning repo(s)...
OK Cloned valentinajemuovic/course-tester-atdd-typescript-20260423-192402
OK Step 4 done (2s)
> Step 5: Applying template files...
OK Applied template files (monolith monorepo)
OK Applied template files
OK Step 5 done (4s)
> Step 6: Replacing repository references...
OK Pass 1: replaced optivem/shop -> valentinajemuovic/course-tester-atdd-typescript-20260423-192402 (4 files)
OK Pass 2: replaced optivem_shop -> valentinajemuovic_course-tester-atdd-typescript-20260423-192402 (1 files)
OK Pass 3: replaced sonar org pattern (1 files)
OK Pass 4: replaced sonar projectName pattern (1 files)
OK Safety check passed: optivem/actions references intact in C:\Users\VALEN_~1\AppData\Local\Temp\scaffold-1389983804\repo
OK Docker-compose image URLs lowercased
OK Infra: replaced docker-compose project names shop- -> course-tester-atdd-typescript-20260423-192402- (4 files)
OK Infra: replaced infrastructure names (docker-compose, DB config, scripts)
OK Repository reference replacement complete
OK Step 6 done (1s)
> Step 7: Replacing namespaces...
OK TypeScript: replaced @optivem/shop-system-test -> @valentinajemuovic/course-tester-atdd-typescript-20260423-192402-system-test (0 files)
OK TypeScript: updated package.json metadata
OK Namespace replacement complete
OK Step 7 done (0s)
> Step 8: Replacing system name...
OK System name: PascalCase Shop -> BlueHorizon (60 files)
OK System name: Java camel shop -> blueHorizon (0 files)
OK System name: Java build shop -> bluehorizon (0 files)
OK System name: .NET shop -> blueHorizon (0 files)
OK System name: test config keys shop -> blueHorizon (4 files)
OK System name: TS kebab prefix shop- -> blue-horizon- (20 files)
OK System name: TS camel shop -> blueHorizon (82 files)
OK System name: HTML kebab shop -> blue-horizon (0 files)
OK System name: renamed 6 PascalCase files
OK System name: renamed 14 kebab files
OK System name: renamed 0 PascalCase directories
OK System name: renamed 3 TS camelCase directories
OK System name: renamed 0 lowercase directories
OK System name replacement complete
OK Step 8 done (1s)
> Step 8: Generating README...
OK Generated README.md
OK Step 9 done (0s)
> Writing project config...
OK Wrote .optivem/config.json
OK Step 10 done (0s)
> Step 9: Creating SonarCloud projects...
OK SonarCloud org: valentinajemuovic
OK SonarCloud project: valentinajemuovic_course-tester-atdd-typescript-20260423-192402-system
OK Step 11 done (2s)
> Step 10: Committing and pushing...
OK Pushed template to valentinajemuovic/course-tester-atdd-typescript-20260423-192402
OK Step 12 done (3s)
> Validating no leftover system names...
OK Step 13 done (0s)
> Verifying local compilation...
OK Compiled system source (typescript)
OK Compiled system tests (typescript)
OK Step 14 done (28s)
> Step 11: Verifying commit stage workflow...
OK Commit stage passed!
OK Step 15 done (4m 14s)
> Step 12: Triggering and verifying acceptance stage...
OK Acceptance stage passed!
WARN No RC version found — acceptance stage may have skipped promotion (e.g. no artifacts yet). Downstream stages will be skipped.
OK Step 16 done (5m 15s)
> Step 13: Triggering and verifying acceptance stage legacy...
OK Acceptance stage legacy passed!
OK Step 17 done (5m 13s)
> Step 17: Running local system tests...
OK Step 18 done (0s)
> Step 14: Triggering and verifying QA stage...
WARN Skipping QA stage — no RC version available
OK Step 19 done (0s)
> Step 15: Triggering and verifying QA signoff...
WARN Skipping QA signoff — no RC version available
OK Step 20 done (0s)
> Step 16: Triggering and verifying production stage...
WARN Skipping production stage — no RC version available
OK Step 21 done (0s)
> Generating project registration info...

------------------------------------------
  Project Registration Info
------------------------------------------

  Owner:         valentinajemuovic
  System Name:   Blue Horizon
  Architecture:  monolith
  Language:      typescript
  Repo Strategy: monorepo

  Repository:    https://github.com/valentinajemuovic/course-tester-atdd-typescript-20260423-192402
  Actions:       https://github.com/valentinajemuovic/course-tester-atdd-typescript-20260423-192402/actions
  SonarCloud:    https://sonarcloud.io/project/overview?id=valentinajemuovic_course-tester-atdd-typescript-20260423-192402-system

------------------------------------------

OK Project registration info printed above — copy-paste into your registration form.
OK Step 22 done (0s)

==========================================
OK All steps passed! Completed in 15m 30s

  System:     Blue Horizon
  Repository: https://github.com/valentinajemuovic/course-tester-atdd-typescript-20260423-192402
  Actions:    https://github.com/valentinajemuovic/course-tester-atdd-typescript-20260423-192402/actions
```
