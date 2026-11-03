# Plan coordination meta-plan: backlog consolidation (status doc)

## TL;DR

**Why:** the backlog under `plans/backlog/{p1..p4,parked}/` needs one place that records what is still open, what needs an attended session, and which plans collide on files.
**End result:** this file is deleted once every open item below has shipped, been handed to its own plan, or been closed by a human.

## Next step (resume here)

Nothing unattended is left except what is listed below. Everything else needs an attended session (rehearsal, VM run, shop push OK, or SonarCloud admin rights). Several items have local, unpushed commits (Item 15: shop `32e90dcb`, gh-optivem `15b30e6c`; Items 22 and 23); run `/sync` to push gh-optivem (and shop, with OK).

Older child plans are proposals, not instructions: when an item starts, judge its plan on the merits and amend, delete or park it if the premise is obsolete.

## Open items

- [ ] **Item 13 (#12 remainder)** `[rehearsal]` `[shop-push]` — code shipped (#12 `59450e7d`). Remaining in `plans/backlog/p2-atdd-quality/20260608-2109-failure-capture-head-window.md`: step 6 (Gradle `--warning-mode none`, a shop edit in `system-test/java/tests*.yaml` plus shop push, needs OK) and the attended rehearsal check.
- [ ] **Item 15 (#21)** `[rehearsal]` `[shop-push]` — code shipped locally (shop `32e90dcb`, gh-optivem `15b30e6c`, unpushed, compile-checked only). Remaining in `plans/backlog/p2-atdd-quality/20260620-0806-no-order-persisted-fanout-prevention.md`: push shop `main` (needs OK), attended runtime check of `cannotPlaceOrderWithNonExistentCoupon` on monolith + multitier x3 languages, rehearsal check of the `scope-task-name` fixer change, then Step 6 (restore #68's criterion).
- [ ] **Item 22 (#20)** — Steps 1-7 shipped (`95f1bb46`..`39e245d9`, unpushed). Step 8 (collapse `scripts/readme-steps.sh`) needs a green clean-room VM run first. Plan: `plans/backlog/p2-atdd-quality/20260812-1705-environment-setup-command.md`.
- [ ] **Item 23 (#42 Slice 1)** — partly shipped (`90fb81e2`..`fa21df35`, shop `cf2869ad`, unpushed). Step 1b (`--backend-inner-loop` flags and init prompts) deferred until a second tier opts in. Left in `plans/backlog/p2-atdd-quality/20261023-0959-optional-inner-tdd-loop-per-component.md`: the nightly mutation CI workflow in shop (push needs OK) and setting `mutation-threshold` from a full `--all` baseline. Slices 2-4 untouched.
- [ ] **Item 24 (Sonar S3776)** — all production-code findings are done (commits `bb2db1c8`..`bf5db66d`). Only test-file findings remain. A human must mark those Won't Fix in SonarCloud (needs admin rights), then delete `plans/backlog/p4-maintenance/20260625-0944-sonar-deferred.md`.
- **Items 20 (#13) and 21 (#16)** moved to the shop plan `shop/plans/20261103-1049-gh-optivem-driven-shop-changes.md` (decisions: #13 Option A, Java only; #16 `/users` customer directory, monolith-java only, refine deferred). Their gh-optivem plans are `plans/backlog/p2-atdd-quality/20260613-1950-view-product-list-erp-list-gap.md` and `plans/backlog/p3-tooling-cost/20260617-1001-rehearsal-corpus-testinstance-and-refine.md`.

Other backlog plans with no open decision here: `plans/backlog/p3-tooling-cost/20261002-0817-unattended-plan-execution.md` (Option A: orchestrator plus one subagent per item, headless loop as escape hatch), `plans/backlog/p4-maintenance/20260511-1702-shop-frontend-react-to-typescript-rename.md` (blocked until shop renames its frontend). The `parked/` plans stay parked.

## Attended verification checklist

The plans below were deleted because their work shipped; only these rehearsal and operator checks remain. Do them in an attended session and tick off.

- [ ] **channel-touched skipped tests (#76 re-run):** re-run the #76 channel-touched scenario in a rehearsal and confirm skipped tests no longer trigger a no-op dispatch followed by a halt (Item 7: #27 `dd5883ea`, #28 `10c2546c`/`6d553b95`).
- [ ] **scope-drift fix-loop (Item 13, #15 `416a761f`):** run a rehearsal that induces a test-only fixer and confirm the scope-drift guard / fix loop behaves as designed (report-only).
- [ ] **schedule orphan-repo cleanup:** confirm the first scheduled orphan-cleanup run on `main` executes and cleans up as intended.
- [ ] **actionlint latest canary:** trigger the canary workflow manually via `workflow_dispatch`, confirm it runs, and check whether Sonar rule S8545 fires on it.
- [ ] **no-legacy omits legacy artifacts:** the plan was deleted as shipped; spot-check `--no-legacy` output once during any attended scaffold run.

## Conflicts still live

| ID | File(s) | Plans | Resolution |
|---|---|---|---|
| C7 | `internal/atdd/process/process-flow.yaml` | retry-budget, optional-inner-tdd, contract-driven outer loop (parked) | never two in parallel; rebase between them |

The other conflicts recorded earlier (C1-C6, C8-C12) involved plans that have since shipped or been deleted and are retired.

Shared-resource rule: `gh optivem system start` and rehearsals bind fixed docker ports. Only one session at a time may run a shop rehearsal. Items tagged `[rehearsal]` take that lock.

## Shop dependency notes

| Kind | Plans | What has to happen in shop |
|---|---|---|
| shop-first (blocked until shop changes merge) | react-to-typescript-rename; optional-inner-tdd Slice 3; microservices-scaffold-pipeline (parked) | shop renames `frontend-react` to `frontend-typescript`; a shop plan promotes `backend-clean-java` to `kind: system`; a shop `system/microservices/` template must exist |
| edits shop | view-product-list-erp-list-gap, rehearsal-corpus-testinstance-and-refine, no-order-persisted-fanout-prevention, optional-inner-tdd Step 4, failure-capture-head-window step 6 | commit via `--repo shop`; pushes to shop `main` change the images the active shop cloud plan deploys, so get an OK first |
| needs a synced `../shop` clone | spec-driven-positioning-and-branding (parked) | `/sync` before starting |
| verify-only rehearsal on shop | failure-capture-head-window, no-order-persisted-fanout-prevention, contract-driven-outer-loop (parked) | nothing to change; needs a free docker stack |

## Wave order

1. Attended verification checklist and the push of local commits (`/sync`, shop push with OK).
2. Items 13 and 15 (shop-push + rehearsal), one at a time under the rehearsal lock.
3. Items 22 and 23 remainders (VM run; second tier opts in; mutation workflow in shop).
4. Item 24 human step (Won't Fix in SonarCloud, then delete the sonar-deferred plan).
5. Shop-first and parked plans, when their triggers occur.

## Pre-execute checks

1. `grep -l "PICKUP\|in-flight\|claimed by\|Picked up by agent" plans/*.md plans/*/*.md plans/*/*/*.md`. A hit from this check text is expected; any other hit means a live agent, so stop and ask.
2. `git status` in gh-optivem and `../shop`: both clean on the files the work owns.
3. `/sync` shop before any rehearsal.
4. The shop cloud plan (`shop/plans/20261029-1316-deploy-shop-cloud-run.md`) may be active: don't touch `*-cloud.yml`, `terraform/`, Keycloak cloud files, and get an OK before pushing to shop `main`.
5. Re-anchor stale paths in older plans: `internal/atdd/runtime/{actions,gates,clauderun,statemachine}` is now `internal/atdd/process/...`; `internal/steps` is `internal/scaffolding/steps`; `internal/projectconfig` is `internal/kernel/projectconfig`; shop `.claude/agents/atdd/` is `internal/atdd/assets/runtime/agents/atdd/`.
6. On Windows run Go tests with `scripts/test.sh` or `-p 2`.
