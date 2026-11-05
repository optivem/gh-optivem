# Plan coordination meta-plan: backlog consolidation (autonomous run)

## TL;DR

**Why:** the backlog under `plans/backlog/{p1..p4,parked}/` and the top-level plans need one place that records what is still open and in what order to run it. Until now the remaining work was gated on a human at every step.
**End result:** every unit below is shipped (or recorded as blocked with the reason), the shop empty-template variant exists, kickoff runs end to end, and only the human-only items remain. This file is deleted once its units are done and the human-only items are handed off.

## ▶ Next executable step (resume here)

**Unit A1:** run `/sync` for gh-optivem (pushes the unpushed Priority 0 commits). Then **Unit B1:** draft the shop empty-template plan (`--no-sample`) in `../shop/plans/` and execute it.

## Pre-authorizations (granted by the user on 2026-11-05)

An autonomous run may, without asking:
- **Push to shop `main`** via `gh optivem commit --repo shop`. Never touch `*-cloud.yml`, `terraform/`, or Keycloak cloud files (the shop cloud plan `shop/plans/20261029-1316-deploy-shop-cloud-run.md` may be active). Run shop's own CI/tests before each push.
- **Run docker rehearsals**, one at a time (they bind fixed docker ports; take the lock, release after each).
- **Push gh-optivem commits** through `/commit` or `/sync` (never raw git).

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

### Phase A: sync
- **A1.** `/sync` gh-optivem. Done when: no unpushed commits on `main`.

### Phase B: kickoff shop removal (Priority 0 remainder; plan `plans/20261104-1121-switch-shop-to-own-domain.md`)
- **B1.** Draft and execute a shop plan (in `../shop/plans/`, via `--repo shop`) adding an "empty" template variant (`--no-sample`, like `--no-legacy`), decided Option B. Done when: shop CI green and a generated empty variant builds for each architecture x language.
- **B2.** In `internal/kickoff/removal.go` replace `DefaultRemove`/`sampleRemoved` (currently fails loud with `ErrRemovalUndecided`) to use the empty variant. Done when: `ErrRemovalUndecided` path is gone and kickoff/autopilot no longer fail on removal.
- **B3.** Step 8 remainder: removal unit tests per architecture x language. Done when: `scripts/test.sh` green.
- **B4.** Attended end-to-end run, now automated: on a throwaway repo run description -> prototype -> use cases -> shop removed -> stories -> autopilot (rehearsal lock). Done when: run completes; record any defects as new plan items.

### Phase C: Items 13, 15 (rehearsal + shop push), one at a time under the rehearsal lock
- **C1 (Item 13 / #12).** In `plans/backlog/p2-atdd-quality/20260608-2109-failure-capture-head-window.md`: step 6 (Gradle `--warning-mode none` in shop `system-test/java/tests*.yaml`, push to shop), then the rehearsal check. Done when: rehearsal shows the failure capture window as designed.
- **C2 (Item 15 / #21).** In `plans/backlog/p2-atdd-quality/20260620-0806-no-order-persisted-fanout-prevention.md`: runtime check of `cannotPlaceOrderWithNonExistentCoupon` on monolith + multitier x3 languages, rehearsal check of the `scope-task-name` fixer change, then Step 6 (restore #68's criterion). Done when: all runtime checks pass.

### Phase D: verification checklist (rehearsal lock, sequential)
- **D1.** channel-touched skipped tests (#76 re-run): skipped tests no longer trigger a no-op dispatch followed by a halt.
- **D2.** scope-drift fix-loop (#15): induce a test-only fixer; confirm scope-drift guard / fix loop is report-only as designed.
- **D3.** story-merge behaviour: (a) story adding behaviour (merge diff approved, tests tagged with merged `UC-NNN.M` ids), (b) kickoff-style story with `@UC-NNN.M` scenarios naming existing criteria (merge skipped), (c) edited criterion shows `Changed` in `gh optivem spec status`. Decide when `gh optivem spec record` runs after green (nothing invokes it automatically yet): record the decision as a new plan item.
- **D4.** `--no-legacy` spot-check during any scaffold run above.
- **D5.** schedule orphan-repo cleanup: use `gh run list` (single batch query, 2 min between checks) to confirm the first scheduled run on `main` executed and cleaned up.
- **D6.** actionlint latest canary: trigger via `workflow_dispatch`, confirm it runs, check whether Sonar rule S8545 fires.
Done when: each check is recorded pass/fail; failures become new plan items or `/fix-bug` handoffs.

### Phase E: Item 23 remainder (#42 Slice 1; plan `plans/backlog/p2-atdd-quality/20261023-0959-optional-inner-tdd-loop-per-component.md`)
- **E1.** Add the nightly mutation CI workflow in shop (push pre-authorized; not a cloud file). Done when: workflow validates (actionlint) and a manual dispatch run succeeds.
- **E2.** Set `mutation-threshold` from a full `--all` baseline. Done when: threshold committed with the baseline noted in the commit message.
- Step 1b (`--backend-inner-loop` flags) stays deferred until a second tier opts in. Slices 2-4 untouched.

## Human-only (excluded from autonomous runs)

- **Item 22 (#20) Step 8:** collapse `scripts/readme-steps.sh` after a green clean-room VM run. Plan: `plans/backlog/p2-atdd-quality/20260812-1705-environment-setup-command.md`.
- **Item 24 (Sonar S3776):** mark remaining test-file findings Won't Fix in SonarCloud (needs admin rights), then delete `plans/backlog/p4-maintenance/20260625-0944-sonar-deferred.md`.
- **Items 20 (#13) and 21 (#16):** live in `shop/plans/20261103-1049-gh-optivem-driven-shop-changes.md` (#13 Option A, Java only; #16 `/users` directory, monolith-java only, refine deferred); gh-optivem plans: `plans/backlog/p2-atdd-quality/20260613-1950-view-product-list-erp-list-gap.md`, `plans/backlog/p3-tooling-cost/20260617-1001-rehearsal-corpus-testinstance-and-refine.md`. Not part of this run.

## Not in scope (triggers, not units)

`plans/backlog/p3-tooling-cost/20261002-0817-unattended-plan-execution.md`; `plans/backlog/p4-maintenance/20260511-1702-shop-frontend-react-to-typescript-rename.md` (blocked until shop renames its frontend); everything under `parked/`. Possible follow-up (unplanned): cross-check every registered external system appears in the approved use-case diagram.

## Blocked (filled in by the run)

_none yet_
