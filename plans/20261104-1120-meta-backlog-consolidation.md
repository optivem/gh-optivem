# Plan coordination meta-plan: backlog consolidation (autonomous run)

## TL;DR

**Why:** the backlog under `plans/backlog/{p2..p4,parked}/` and the top-level plans need one place that records what is still open and in what order to run it. Until now the remaining work was gated on a human at every step.
**End result:** every unit below is shipped (or recorded as blocked with the reason), the shop empty-template variant exists, kickoff runs end to end, and only the human-only items remain. This file is deleted once its units are done and the human-only items are handed off.

## ▶ Next executable step (resume here)

**Waiting on Docker (2026-11-09):** the user approved three rehearsals, but they wait until the user allows gh-optivem to use Docker again (the parallel shop session is busy for several hours). Run them one at a time, in a scratch shop clone on a local branch that is never pushed (F2 precedent): (a) #65 ERP list story (Item 2), (b) shop #85 for D3b, (c) #68 for C2c. Record each result here and in its backlog plan.

gh-optivem `v1.6.107` released 2026-11-06 (cloud setup in scaffolds, `2454ada5`). What's left beyond the rehearsals: E2 (a shop config write, for the user's shop chat) and the Human-only items below.

1. **Shop scaffolding gap (found by D4, 2026-11-06): fixed** on gh-optivem `main` (`b684625e`, `3a73ae55`; rehearsed green on shop `main`). Released in `v1.6.106` (port plan done and deleted). Cloud setup for scaffold users: `plans/20261106-1305-scaffold-cloud-setup-guide.md`.
2. **ERP gap (#65), user approved 2026-11-09:** Option A was built in shop `d777ecde` (2026-11-06, Java monolith). The slimmed Option B shipped in gh-optivem `40f4d891` (2026-11-09: `acceptance-test-writer` stages lists via `given().products()`, empty list staged explicitly, assert on staged SKUs not on counts; `dsl-implementer` doesn't call `defaults.ensure` on list reads). Remaining: the #65 rehearsal (waits on Docker, above). Plan: `plans/backlog/p2-atdd-quality/20260613-1950-view-product-list-erp-list-gap.md`.
3. **User, when convenient:** one attended session for C2b, D2, D3a. SonarCloud done 2026-11-09: 36 test-file S3776 findings and the canary S8545 marked Accepted through the API; 19 production S3776 findings stay open. (#65 moved to Ready 2026-11-06.)
4. **D3b/C2c issue edits, user approved 2026-11-09:** C2c: #68's `And no order is persisted` restored (the Layer 1 reference is already green on all 6 combos). D3b: #69 was **not** tagged, because its quantity-100 rule isn't in UC-001 (UC-001.8 covers zero/negative/non-integer/empty), so tagging it would hide a new rule from the merge. Instead shop #85 was created ("Reject order with zero quantity", tagged `@UC-001.8`, copying the criterion). Both rehearsals wait on Docker (above).

## Pre-authorizations (granted by the user on 2026-11-05)

An autonomous run may, without asking:
- **Push to shop `main`** via `gh optivem commit --repo shop`. Never touch `*-cloud.yml`, `terraform/`, or Keycloak cloud files (the shop cloud plan `shop/plans/20261029-1316-deploy-shop-cloud-run.md` may be active). Run shop's own CI/tests before each push.
- **Run docker rehearsals**, one at a time (they bind fixed docker ports; take the lock, release after each).
- **Push gh-optivem commits** through `/commit` or `/sync` (never raw git).

Added 2026-11-05 (execution session):
- **Create and clean up throwaway GitHub repos** for rehearsals, and **dispatch workflow runs** (D5/D6), without asking.
- Rehearsals after shop pushes use **`--shop-ref main`** (the default pin is the latest `meta-v*` release; cutting a release is not authorized).
- Shop-editing units (B1, C1 step 6, E1/E2) clone shop into the session scratch dir per unit; no persistent `../shop` needed.

Not pre-authorized, so still stop-and-ask: force-push, releases, closing GitHub issues, deleting anything outside this plan's own files.

## Run rules

- Orchestrator + one subagent per unit; units are sequential unless marked parallel-safe. Each unit commits only after its gate passes; one commit per unit per repo.
- A unit that fails its gate twice: record the blocker under "Blocked" at the bottom, skip to the next unblocked unit.
- `internal/atdd/process/process-flow.yaml` (conflict C7): never edit from two units in parallel.
- Older child plans are proposals: judge on the merits; amend, park or delete if the premise is obsolete.
- Pre-execute checks (run once at start):
  1. `grep -l "PICKUP\|in-flight\|claimed by\|Picked up by agent" plans/*.md plans/*/*.md plans/*/*/*.md`; any hit other than this plan's own marker means a live agent: stop and ask.
  2. `git status` in gh-optivem and `../shop` clean on files the work owns; `/sync` shop before any rehearsal.
  3. Stale paths in older plans: `internal/atdd/runtime/{actions,gates,clauderun,statemachine}` is now `internal/atdd/process/...`; `internal/steps` is `internal/scaffolding/steps`; `internal/projectconfig` is `internal/kernel/projectconfig`; shop `.claude/agents/atdd/` is `internal/atdd/assets/runtime/agents/atdd/`.
  4. On Windows run Go tests with `scripts/test.sh` or `-p 2`.

## Executable units (in order)

Non-Docker units run first; everything that needs Docker (system start, rehearsals) is grouped at the end (user, 2026-11-05).

### Part 1: no Docker

- Phase A done 2026-11-05: A0 shipped (`--allowedTools` default `Read, Glob, Grep, Edit, Write, Bash(gh optivem *)`, per-agent `tools:` frontmatter override). End-to-end confirmation rides on C1.

#### Phase E: Item 23 remainder (#42 Slice 1; plan `plans/backlog/p2-atdd-quality/20261023-0959-optional-inner-tdd-loop-per-component.md`)
- E1 done 2026-11-05 (shop `aa8aa8de`+`b700f301`, nightly `multitier-backend-java-mutation-nightly.yml`, dispatch run 41993634354 green).
- [ ] **E2.** Unblocked by gh-optivem `v1.6.106` (2026-11-06); hand to the user's shop chat (shop change): write `mutation-threshold: 50` into shop `gh-optivem-multitier-java.yaml`. Baseline 53% (164/307 killed), threshold 50 chosen and recorded in shop `plans/backlog/p1-ci-pipeline/20261103-1335-readd-inner-loop-pilot-config.md` (shop `7bdcd272`), but released v1.6.105 rejects the `inner-loop` keys, so the config write waits for the next release (releases need the user).
- Step 1b (`--backend-inner-loop` flags) stays deferred until a second tier opts in. Slices 2-4 untouched.

#### Phase B: kickoff shop removal — ⏳ Deferred (needs discussion; user, 2026-11-05)
- [ ] **B1–B4** (shop "empty" variant, `removal.go`, removal tests, end-to-end run) — ⏳ Deferred: not ready; the skeleton design needs a future discussion. Findings so far: a `--no-legacy`-style delete list can't produce a buildable repo (the sample runs through the driver/DSL aggregators and all system code), so shop must ship replacement skeleton sources (~100+ files, 9 projects, multi-session). Recommendation on the table (not decided): a **walking skeleton** (one thin hello-world slice UI → API → backend → DB, one acceptance test through DSL/driver on API + UI channels, tagged as skeleton not `UC-001`; keep the test architecture, Postgres, Keycloak, WireMock harness; drop clock/erp/tax and the order/coupon code), shipped in shop as an overlay + delete list so shop CI proves it for all 6 combos. See `plans/20261104-1121-switch-shop-to-own-domain.md`. (B4's end-to-end run needs Docker; run it with Part 2 when B is picked up.)

#### Phase F: unattended story intake (approved by the user 2026-11-05)
- F2 done 2026-11-05: unattended #65 passed `MERGE_USE_CASES` with no yield (`category=test-agent`), then halted at `ESCC_UNDECLARED_HALT` (rc=1) on the same `system-implementer` ERP-list scope exception. Recorded in the ERP-gap plan, with two new reasons; the user confirms Option A + slimmed B. The shop branch `f2-rehearsal-65` stayed local; nothing was pushed.
- F1 done 2026-11-05: `merge-use-cases` moved to `category: test-agent` (process-flow.yaml, transitions_test, README, cli-reference, use-case-merger prompt); `go test -p 2 ./...` green. `docs/process-diagram.md` is left to the CI regeneration bot.
### Part 2: Docker (run last, one at a time under the rehearsal lock)

#### Phase C: Items 13, 15
- C1 done 2026-11-05 (closed `p2-atdd-quality/20260608-2109-failure-capture-head-window.md`, whose last item was this check): #76 red-phase rehearsal rc=0; `verify_failure_output` opens with the `AssertionError` (head window), then `--- (later output) ---`, zero deprecation lines. A0 confirmed end to end (children `Write` + `gh optivem test compile`/`output write` with no denials; one intended denial of a `python3` heredoc edit).
- **C2 (Item 15 / #21).** In `plans/backlog/p2-atdd-quality/20260620-0806-no-order-persisted-fanout-prevention.md`:
  - C2b done 2026-11-05 (partial evidence, as decided): unattended #76 run with an induced compile break after the writer; trace `EXECUTE_COMMAND ... params=...,command=gh optivem system-test compile,scope-task-name=write-acceptance-tests,task-name=compile-tests` → `FIX` → `APPROVE_PRE category=human` → yield rc=2; flow.txt shows FIX/APPROVE_PRE/ASK_HUMAN all carry `scope-task-name=write-acceptance-tests`. The attended fixer-runs-with-writer-scope check stays Human-only. Side note: `setup-tests` passes no `scope-task-name` (already listed out of scope in the child plan).
  - **C2c.** Moved to Human-only (user decision 2026-11-05: don't edit #68 autonomously).
  Done when: all runtime checks pass.

#### Phase D: verification checklist (sequential; all need `implement` runs)
- D1 pass 2026-11-05: #76 re-run rc=0; `GATE_CHANNEL_TOUCHED -> bool=false` → `CHANNEL_SKIPPED channel=ui` ("Channel Untouched — System Cycle Skipped"), no no-op dispatch, no halt, ends `test-outcome=pass`. (#76 is left in In Acceptance on the board.)
- D2 unattended part pass 2026-11-05: out-of-scope file injected during the writer → `VALIDATE_OUTPUTS_AND_SCOPES -> failure-kind=scope-diff, failing-task-name=write-acceptance-tests` → `FIX` → `APPROVE_PRE category=human` → yield rc=2; no separate scope-drift halt. Attended part is Human-only.
- D3 2026-11-05: (c) pass (local spec edit of UC-001.2 → `spec status` "Changed 1", `spec check --mode strict` rc=1, `record --refresh` clears it); (a) partial: #69 reaches `CHECK_STORY_MERGE_NEEDED -> story-merge-needed=true` → `MERGE_USE_CASES` (`category=human`) and yields, so merge diff + `UC-NNN.M` tagging need an attended run (Human-only); (b) blocked: no corpus story carries `@UC-NNN.M` tags (checked #65, #68–72, #75, #84), needs an issue edit (Human-only); unit test `TestCheckStoryMergeNeeded_NoOpWhenScenariosNameExistingCriteria` covers the skip path.
- D3d done 2026-11-05 (`8f4cb799`): `change-system-behavior` now runs GATE_STORY_SPEC_RECORD → RECORD_SPEC_FINGERPRINTS (`gh optivem spec record --create --criteria '${merged-criteria}'`) → COMMIT_SPEC_FINGERPRINTS after the last channel's implement-and-verify, stories only; separate commit because each channel commits its own system change.
- D4 pass 2026-11-06 (Linux devcontainer, default shop ref `meta-v1.0.223`): `init --no-legacy` rc=0; no `systemtest/legacy`, `gh-optivem.legacy.yaml`, `tests.legacy.yaml`, legacy workflow or badge; system + system tests compiled; first CI run (commit-stage 42220144451) green; repo deleted. A first attempt with `--shop-ref main` exposed the scaffolding gap (next-step item 1).
Done when: each check is recorded pass/fail; failures become new plan items or `/fix-bug` handoffs.

## Human-only (excluded from autonomous runs)


- **Item 22 (#20) Step 8:** collapse `scripts/readme-steps.sh` after a green clean-room VM run. Plan: `plans/backlog/p2-atdd-quality/20260812-1705-environment-setup-command.md`.
- **Item 24 (Sonar S3776): test-file part done 2026-11-09** (36 test-file S3776 + canary S8545 marked Accepted through the API). Production part done the same day: the remaining 19 production S3776 functions were split by helper extraction (CC <= 15 by estimate; SonarCloud re-scan after the push confirms). The Sonar plan was deleted.
- **Items 20 (#13) and 21 (#16):** live in `shop/plans/20261103-1049-gh-optivem-driven-shop-changes.md` (#13 Option A, Java only; #16 `/users` directory, monolith-java only, refine deferred); gh-optivem plans: `plans/backlog/p2-atdd-quality/20260613-1950-view-product-list-erp-list-gap.md`, `plans/backlog/p3-tooling-cost/20260617-1001-rehearsal-corpus-testinstance-and-refine.md`. Not part of this run.

- **C2c (Item 15 / #21 Step 6), user approved 2026-11-09:** #68's criterion restored. Remaining: rehearse #68 to confirm the absence-shaped ticket goes green by copying the shop reference (waits on Docker). See `plans/backlog/p2-atdd-quality/20260620-0806-no-order-persisted-fanout-prevention.md`.
- **D3a attended check:** attended `implement 69`: approve `merge-use-cases`, confirm the merge diff and tests tagged with merged `UC-NNN.M` ids.
- **D3b, user approved 2026-11-09:** shop #85 created (scenario tagged `@UC-001.8`). Remaining: run `implement 85` and confirm `MERGE_USE_CASES` is skipped (waits on Docker).
- **Board tidy-up from this run:** #69 restored to In QA and #76 found already in In QA (2026-11-06); #65 moved to Ready (user, 2026-11-06).
- **D2 attended check:** in an attended run, let the fixer loop exhaust after an out-of-scope edit and confirm the halt is report-only and lists "fixer edited: N file(s)" (#15, commit 416a761f).
- **C2b attended check:** in an attended `implement` run that hits a compile failure, approve the `fix-*` (human-category) dispatch and confirm the fixer runs with the writer's scope.

Moved in 2026-11-11 from the deleted `20261111-2359-meta-active-plans` (its Verification section):

- **`1305` Step 5:** live GCP run with a short repo name, following `docs/cloud-setup.md`; settle its unverified bits (GHCR `/orgs/` vs `/users/` link, public-package default, multirepo script location).
- **`1121`:** waits on the walking-skeleton decision (Phase B).
- **`1258` operator check:** read `gh optivem spec generate` output on shop and judge if it reads well to a PO.
- **Rehearsals from the deleted `1219` umbrella** (once shop/Docker is free): Align loop (rule stated only in general terms → refiner proposes a numeric example; answer "no" once → next round uses the feedback); Use Case Criteria body section on a GitHub issue (re-run still parses); gift-wrap rehearsal (Red reviewer flags a planted dropped `.then().shouldSucceed()`; commits match the talk); Code & Architecture Review on shop (`refactor` → `review-code-and-architecture`, `docs/guidelines/<area>.md` gains a rule, a `GUIDELINES` commit lands); WIP-gate removal (final commit has no gate, pipeline runs the tests).
- **Narrowed post-refactor test runs** (shipped 2026-11-11, from the deleted plan `20261111-1023`): rehearse one story with a system refactor and one review-driven refactor on each stack; confirm the local run executes only the use case's tests (`--use-case` in the `system-test run` line; Java via `-DincludeTags`, .NET via `tag~`, TypeScript via `--grep @UC-…`), that the guideline round still runs the whole suite, and that the pipeline runs the full suite after push. The Java `&(…)` and TypeScript lookahead filters through `gradlew.bat` / `npx.cmd` are not yet run against a real stack.

## Not in scope (triggers, not units)

`plans/backlog/p4-maintenance/20260511-1702-shop-frontend-react-to-typescript-rename.md` (blocked until shop renames its frontend); everything under `parked/`. Possible follow-up (unplanned): cross-check every registered external system appears in the approved use-case diagram.

## Follow-ups found by the run (not units yet)

- **Spec coverage for .NET/TypeScript (updated 2026-11-09):** no TRX/Playwright-JSON parser is needed. Shop now tags its .NET (`[Trait("tag", "UC-…")]` + `JunitXml.TestLogger`) and TypeScript (`criteria(…)` annotations + Playwright `junit` reporter) tests and writes JUnit XML, which `internal/spec` already reads; the scaffold `results` globs now point at `SystemTests/TestResults/junit/*.xml` and `junit-results/*.xml`. The remaining gh-optivem work (Gherkin criteria, folder per use case, `status: planned`, no-criteria error) is in `plans/20261107-1300-spec-parse-gherkin-criteria.md`; only its release is left.

## Rehearsal setup (Linux devcontainer, verified 2026-11-05)

Tokens load from `~/.config/gh-optivem/.env` (CRLF file: never `source` it into a shell; let gh-optivem load it). Install the extension from the repo (`go build -o gh-optivem . && gh extension install .`). apt: `openjdk-21-jdk-headless` + Chromium libs (Ubuntu 24.04 `t64` names: `libatk1.0-0t64 libatk-bridge2.0-0t64 libcups2t64 libasound2t64 libatspi2.0-0t64 libxkbcommon0 libxcomposite1 libxdamage1 libxfixes3 libxrandr2 libgbm1 libnss3 libpango-1.0-0 libcairo2`). Per run, inside a scratch shop clone at `main`: fresh branch, `rm -rf system-test/java/src/test/java/com/mycompany/myshop/systemtest/legacy`, `export GH_OPTIVEM_CONFIG=$PWD/gh-optivem-monolith-java.yaml`, `gh optivem system clean`, `gh optivem --auto implement N ... --headless --log-file ../x.log`. `--shop-ref` is a scaffold flag, not an `implement` flag. `scripts/atdd-rehearsal.sh` has no lock: "no containers / no other `implement` running" is the lock.

## Blocked (filled in by the run)

- **Sonar preflight 401 (resolved 2026-11-05, not a product bug):** D1 attempts 1–2 failed preflight with `HTTP 401` because the worker had `source`d the CRLF `~/.config/gh-optivem/.env` into its shell, so `SONAR_TOKEN` carried a trailing `\r` (gh-optivem's own .env loader trims it). Don't `source` that file; let gh-optivem load it. Side change kept: Sonar preflight/token check now send Bearer (consistency with `internal/devworkflow/sonar`), commit 2a6a9ba0; its message wrongly blames SonarCloud.

- **C2c gate not met:** no absence-shaped ticket is in the rehearsal corpus (#68's criterion was dropped, #69 has none), so "an absence-shaped ticket goes green by copying the shop reference" has not been observed. Needs either a new absence-shaped issue (creating one is the user's call) or #68's criterion restored first.

## Shop follow-ups moved in (2026-11-06)

From shop `plans/move-to-gh-optivem/20261106-gh-optivem-followups-from-p4-p5.md` (deleted there): item 1 → ERP plan (above); item 2 → `plans/backlog/p3-tooling-cost/20260617-1001-rehearsal-corpus-testinstance-and-refine.md` (5a/5b done, 5c–5f open; the `crm` story issue needs the user's go); item 3 = C2c (Human-only, deferred by the user); item 4 (.NET `--test` case check) already fixed in `e965d52c`; item 5 (custom domains) → `plans/20261106-1305-scaffold-cloud-setup-guide.md` Step 4b. Rehearsal prerequisite noted by shop: `gh` token needs `project` scope (granted 2026-11-06) and gh-optivem's `.env`.
