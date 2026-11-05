# Plan coordination meta-plan: backlog consolidation (autonomous run)

> 🤖 **Picked up by agent** — `8f47336fc9eb` at `2026-11-05T07:44:15Z`

## TL;DR

**Why:** the backlog under `plans/backlog/{p1..p4,parked}/` and the top-level plans need one place that records what is still open and in what order to run it. Until now the remaining work was gated on a human at every step.
**End result:** every unit below is shipped (or recorded as blocked with the reason), the shop empty-template variant exists, kickoff runs end to end, and only the human-only items remain. This file is deleted once its units are done and the human-only items are handed off.

## ▶ Next executable step (resume here)

**Prerequisite:** `gh auth login` done and `gh extension install .` run from gh-optivem (dev container starts without either). Then **Unit C1:** in `plans/backlog/p2-atdd-quality/20260608-2109-failure-capture-head-window.md` do step 6 (Gradle `--warning-mode none` in shop `system-test/java/tests*.yaml`, shop cloned into the session scratch dir), push shop, then the rehearsal check with `--shop-ref main`. Phase B is deferred.

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

### Phase B: kickoff shop removal — ⏳ Deferred (needs discussion; user, 2026-11-05)
- [ ] **B1–B4** (shop "empty" variant, `removal.go`, removal tests, end-to-end run) — ⏳ Deferred: not ready; the skeleton design needs a future discussion. Findings so far: a `--no-legacy`-style delete list can't produce a buildable repo (the sample runs through the driver/DSL aggregators and all system code), so shop must ship replacement skeleton sources (~100+ files, 9 projects, multi-session). Recommendation on the table (not decided): a **walking skeleton** (one thin hello-world slice UI → API → backend → DB, one acceptance test through DSL/driver on API + UI channels, tagged as skeleton not `UC-001`; keep the test architecture, Postgres, Keycloak, WireMock harness; drop clock/erp/tax and the order/coupon code), shipped in shop as an overlay + delete list so shop CI proves it for all 6 combos. See `plans/20261104-1121-switch-shop-to-own-domain.md`.

### Phase C: Items 13, 15 (rehearsal + shop push), one at a time under the rehearsal lock
- **C1 (Item 13 / #12).** In `plans/backlog/p2-atdd-quality/20260608-2109-failure-capture-head-window.md`: step 6 (Gradle `--warning-mode none` in shop `system-test/java/tests*.yaml`, push to shop), then the rehearsal check. Done when: rehearsal shows the failure capture window as designed.
- **C2 (Item 15 / #21).** In `plans/backlog/p2-atdd-quality/20260620-0806-no-order-persisted-fanout-prevention.md`: runtime check of `cannotPlaceOrderWithNonExistentCoupon` on monolith + multitier x3 languages, rehearsal check of the `scope-task-name` fixer change, then Step 6 (restore #68's criterion). Done when: all runtime checks pass.

### Phase D: verification checklist (rehearsal lock, sequential)
- **D1.** channel-touched skipped tests (#76 re-run): skipped tests no longer trigger a no-op dispatch followed by a halt.
- **D2.** scope-drift fix-loop (#15): induce a test-only fixer; confirm scope-drift guard / fix loop is report-only as designed.
- **D3.** story-merge behaviour: (a) story adding behaviour (merge diff approved, tests tagged with merged `UC-NNN.M` ids), (b) kickoff-style story with `@UC-NNN.M` scenarios naming existing criteria (merge skipped), (c) edited criterion shows `Changed` in `gh optivem spec status`. Decide when `gh optivem spec record` runs after green (nothing invokes it automatically yet): record the decision as a new plan item.
- **D4.** `--no-legacy` spot-check during any scaffold run above.
Done when: each check is recorded pass/fail; failures become new plan items or `/fix-bug` handoffs.

### Phase E: Item 23 remainder (#42 Slice 1; plan `plans/backlog/p2-atdd-quality/20261023-0959-optional-inner-tdd-loop-per-component.md`)
- **E1.** Add the nightly mutation CI workflow in shop (push pre-authorized; not a cloud file). Done when: workflow validates (actionlint) and a manual dispatch run succeeds.
- **E2.** Set `mutation-threshold` from a full `--all` baseline. Done when: threshold committed with the baseline noted in the commit message.
- Step 1b (`--backend-inner-loop` flags) stays deferred until a second tier opts in. Slices 2-4 untouched.

## Human-only (excluded from autonomous runs)

- **Item 22 (#20) Step 8:** collapse `scripts/readme-steps.sh` after a green clean-room VM run. Plan: `plans/backlog/p2-atdd-quality/20260812-1705-environment-setup-command.md`.
- **Item 24 (Sonar S3776):** mark remaining test-file findings Won't Fix in SonarCloud (needs admin rights), then delete `plans/backlog/p4-maintenance/20260625-0944-sonar-deferred.md`. Also mark `githubactions:S8545` on `.github/workflows/gh-actionlint-canary.yml:41` (`actionlint@latest`) Accepted/Won't Fix: unpinned is the canary's purpose (found by D6, 2026-11-05).
- **Items 20 (#13) and 21 (#16):** live in `shop/plans/20261103-1049-gh-optivem-driven-shop-changes.md` (#13 Option A, Java only; #16 `/users` directory, monolith-java only, refine deferred); gh-optivem plans: `plans/backlog/p2-atdd-quality/20260613-1950-view-product-list-erp-list-gap.md`, `plans/backlog/p3-tooling-cost/20260617-1001-rehearsal-corpus-testinstance-and-refine.md`. Not part of this run.

## Not in scope (triggers, not units)

`plans/backlog/p3-tooling-cost/20261002-0817-unattended-plan-execution.md`; `plans/backlog/p4-maintenance/20260511-1702-shop-frontend-react-to-typescript-rename.md` (blocked until shop renames its frontend); everything under `parked/`. Possible follow-up (unplanned): cross-check every registered external system appears in the approved use-case diagram.

## Blocked (filled in by the run)

- **Headless-agent rehearsals (`gh optivem --auto implement ... --headless`) are refused by this session's auto-mode permission check** (it launches headless claude agents). Not bypassed. Affects C1's trace check, C2's fixer rehearsal check, D1–D4. Needs an attended run or a user-granted permission rule.
- **C1:** step 6 shipped (shop `de90fbe7`, `--warning-mode none` in `system-test/java/tests*.yaml`; child plan updated in gh-optivem `3032161d`). Probe of `formatVerifyFailureOutput` on real Gradle output shows the assertion in the head window and no deprecation chaff. Remaining: the #76 red-phase trace check (headless rehearsal, see above).
