# Plan coordination meta-plan: backlog consolidation (autonomous run)

## TL;DR

**Why:** the backlog under `plans/backlog/{p2..p4,parked}/` and the top-level plans need one place that records what is still open and in what order to run it. Until now the remaining work was gated on a human at every step.
**End result:** every unit below is shipped (or recorded as blocked with the reason), the shop empty-template variant exists, kickoff runs end to end, and only the human-only items remain. This file is deleted once its units are done and the human-only items are handed off.

## ▶ Next executable step (resume here)

**No autonomous units remain (2026-11-06).** D4 passed; the `/fix-bug` handoffs shipped (red phase `5df20a23`, `optivem/actions` `708d562`, .NET case check already in `e965d52c`). What's left waits on the user, in the order the user chose: items 2 and 4 (deferred decisions), then the gh-optivem release last.

1. **Shop scaffolding gap (found by D4, 2026-11-06): fixed** on gh-optivem `main` (`b684625e`, `3a73ae55`; rehearsed green on shop `main`). The release now waits on shop cutting a `meta-v*` with the new layout — see `plans/20261103-1536-gh-optivem-scaffolds-single-pipeline.md`. Cloud setup for scaffold users: `plans/20261106-1305-scaffold-cloud-setup-guide.md`.
2. **ERP-gap option (user, ⏳ deferred to the end of the run 2026-11-06 — user is thinking it over):** confirm Option A + slimmed B in `plans/backlog/p2-atdd-quality/20260613-1950-view-product-list-erp-list-gap.md` (the F2 rerun on 2026-11-05 still reproduces the gap).
3. **User, when convenient:** gh-optivem release (unblocks E2; user wants it as the **last** thing), SonarCloud Won't Fix (S3776 test files, canary S8545), one attended session for C2b, D2, D3a, and #65's board column (was In Progress; prior column unknown).
4. **D3b/C2c issue edits (user, ⏳ deferred to the end of the run 2026-11-06):** may the agent tag #69's criteria with existing ids (D3b) and restore #68's absence-shaped criterion (C2c)?

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
- [ ] **E2.** ⏳ Blocked on a gh-optivem release: baseline 53% (164/307 killed), threshold 50 chosen and recorded in shop `plans/backlog/p1-ci-pipeline/20261103-1335-readd-inner-loop-pilot-config.md` (shop `7bdcd272`), but released v1.6.105 rejects the `inner-loop` keys, so the config write waits for the next release (releases need the user).
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


- **gh-optivem release** to unblock E2 (then write `mutation-threshold: 50` into shop `gh-optivem-multitier-java.yaml`).

- **Item 22 (#20) Step 8:** collapse `scripts/readme-steps.sh` after a green clean-room VM run. Plan: `plans/backlog/p2-atdd-quality/20260812-1705-environment-setup-command.md`.
- **Item 24 (Sonar S3776):** mark remaining test-file findings Won't Fix in SonarCloud (needs admin rights), then delete `plans/backlog/p4-maintenance/20260625-0944-sonar-deferred.md`. Also mark `githubactions:S8545` on `.github/workflows/gh-actionlint-canary.yml:41` (`actionlint@latest`) Accepted/Won't Fix: unpinned is the canary's purpose (found by D6, 2026-11-05).
- **Items 20 (#13) and 21 (#16):** live in `shop/plans/20261103-1049-gh-optivem-driven-shop-changes.md` (#13 Option A, Java only; #16 `/users` directory, monolith-java only, refine deferred); gh-optivem plans: `plans/backlog/p2-atdd-quality/20260613-1950-view-product-list-erp-list-gap.md`, `plans/backlog/p3-tooling-cost/20260617-1001-rehearsal-corpus-testinstance-and-refine.md`. Not part of this run.

- **C2c (Item 15 / #21 Step 6):** restore #68's dropped criterion (or create a new absence-shaped issue), then rehearse that an absence-shaped ticket goes green by copying the shop reference. See `plans/backlog/p2-atdd-quality/20260620-0806-no-order-persisted-fanout-prevention.md`.
- **D3a attended check:** attended `implement 69`: approve `merge-use-cases`, confirm the merge diff and tests tagged with merged `UC-NNN.M` ids.
- **D3b:** tag a corpus story's scenarios with existing ids (e.g. #69 → `@UC-001.8`) or create a tagged issue, then confirm the merge is skipped.
- **Board tidy-up from this run:** #69 restored to In QA and #76 found already in In QA (2026-11-06); #65 is In Progress (prior column unknown; user to say).
- **D2 attended check:** in an attended run, let the fixer loop exhaust after an out-of-scope edit and confirm the halt is report-only and lists "fixer edited: N file(s)" (#15, commit 416a761f).
- **C2b attended check:** in an attended `implement` run that hits a compile failure, approve the `fix-*` (human-category) dispatch and confirm the fixer runs with the writer's scope.

## Not in scope (triggers, not units)

`plans/backlog/p4-maintenance/20260511-1702-shop-frontend-react-to-typescript-rename.md` (blocked until shop renames its frontend); everything under `parked/`. Possible follow-up (unplanned): cross-check every registered external system appears in the approved use-case diagram.

## Follow-ups found by the run (not units yet)

- **Spec coverage for .NET/TypeScript (updated 2026-11-06):** scaffolds now ship `spec.yaml` with a per-language `results` glob (Java Gradle XML, .NET `SystemTests/TestResults/**/*.trx`, TS `playwright-report/*.json`). Still missing for `spec record`/`spec check` to work there: (1) the spec parser (`ReadJUnitFiles`) reads JUnit XML only (TRX → 0 tests, Playwright JSON → parse error); (2) shop's .NET/TS system tests carry no `UC-x.y` criterion tags (Java gets them via build.gradle injecting `@Tag` as JUnit properties). Needs a shop plan (tag .NET/TS tests, or emit JUnit XML) plus a gh-optivem parser change; until then .NET/TS story runs fail at RECORD_SPEC_FINGERPRINTS. Also unchecked: Playwright's HTML reporter may clear `playwright-report/` between suites.

## Rehearsal setup (Linux devcontainer, verified 2026-11-05)

Tokens load from `~/.config/gh-optivem/.env` (CRLF file: never `source` it into a shell; let gh-optivem load it). Install the extension from the repo (`go build -o gh-optivem . && gh extension install .`). apt: `openjdk-21-jdk-headless` + Chromium libs (Ubuntu 24.04 `t64` names: `libatk1.0-0t64 libatk-bridge2.0-0t64 libcups2t64 libasound2t64 libatspi2.0-0t64 libxkbcommon0 libxcomposite1 libxdamage1 libxfixes3 libxrandr2 libgbm1 libnss3 libpango-1.0-0 libcairo2`). Per run, inside a scratch shop clone at `main`: fresh branch, `rm -rf system-test/java/src/test/java/com/mycompany/myshop/systemtest/legacy`, `export GH_OPTIVEM_CONFIG=$PWD/gh-optivem-monolith-java.yaml`, `gh optivem system clean`, `gh optivem --auto implement N ... --headless --log-file ../x.log`. `--shop-ref` is a scaffold flag, not an `implement` flag. `scripts/atdd-rehearsal.sh` has no lock: "no containers / no other `implement` running" is the lock.

## Blocked (filled in by the run)

- **Sonar preflight 401 (resolved 2026-11-05, not a product bug):** D1 attempts 1–2 failed preflight with `HTTP 401` because the worker had `source`d the CRLF `~/.config/gh-optivem/.env` into its shell, so `SONAR_TOKEN` carried a trailing `\r` (gh-optivem's own .env loader trims it). Don't `source` that file; let gh-optivem load it. Side change kept: Sonar preflight/token check now send Bearer (consistency with `internal/devworkflow/sonar`), commit 2a6a9ba0; its message wrongly blames SonarCloud.

- **C2c gate not met:** no absence-shaped ticket is in the rehearsal corpus (#68's criterion was dropped, #69 has none), so "an absence-shaped ticket goes green by copying the shop reference" has not been observed. Needs either a new absence-shaped issue (creating one is the user's call) or #68's criterion restored first.

