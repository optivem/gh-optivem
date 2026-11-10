# 2026-11-09 19:09 UTC — Meta-plan: run all active top-level plans

## TL;DR

**Why:** 8 top-level plans were open; most waited on user decisions.
**End result:** all decisions were taken on 2026-11-10 and the agent work is done. What is left is the release check of `1300` and the user-only items under **Verification**.

## ▶ Next executable step (resume here)

Release check for `plans/20261107-1300-spec-parse-gherkin-criteria.md` (its Step 5): the push to `main` made on 2026-11-10 triggers gh-optivem's acceptance pipeline, which cuts the release. As of 2026-11-10 the latest release was still v1.6.107 (2026-11-06) and shop's `main` was failing (another agent is fixing it). Wait for BOTH a new release and a green shop. The user has approved cloning shop into a temp dir (read-only, deleted afterwards) at that point; then run `gh optivem spec validate` and confirm 14 UC-001 criteria, then delete `1300` (shop-side copy is deleted from a shop session), then delete this meta-plan.

## Decisions taken (2026-11-10)

- `1302` UX/UI review: **out of scope** for gh-optivem (human manual QA). Plan deleted.
- `1300` release: last step of the run, one push to `main` carrying everything, then validate.
- `1250`: keep running the whole suite; narrowing deferred to `plans/20261110-1125-consider-narrow-test-runs-after-refactor.md`.
- `1301` legacy baseline: separate bulk process, system started once, layer by layer, gates on the core use case only, mandatory system-code safety gate. Implementation plan: `plans/20261110-1130-legacy-baseline-bulk-impl.md`. Per-ticket `cover-system-behavior` unchanged.
- `1258` Step 7 done; `1258` and the `1219` umbrella deleted.

## Verification (user, not executed by the run)

- `1305` Step 5: live GCP run with a short repo name, following `docs/cloud-setup.md`; settle its unverified bits (GHCR `/orgs/` vs `/users/` link, public-package default, multirepo script location).
- `1121`: waits on the walking-skeleton decision (meta-backlog Phase B); not part of this run.
- `1258` operator check: read `gh optivem spec generate` output on shop and judge if it reads well to a PO.
- Operator rehearsals from the deleted `1219` umbrella, once shop/Docker is free: Align loop (rule stated only in general terms → refiner proposes a numeric example; answer "no" once → next round uses the feedback); Use Case Criteria body section on a GitHub issue (re-run still parses); gift-wrap rehearsal (Red reviewer flags a planted dropped `.then().shouldSucceed()`; commits match the talk); Code & Architecture Review on shop (`refactor` → `review-code-and-architecture`, `docs/guidelines/<area>.md` gains a rule, a `GUIDELINES` commit lands); WIP-gate removal (final commit has no gate, pipeline runs the tests).
