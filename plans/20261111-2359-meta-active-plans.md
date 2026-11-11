# 2026-11-11 23:59 — Meta-plan: run all active top-level plans

## TL;DR

**Why:** 8 top-level plans were open; most waited on user decisions.
**End result:** all decisions on the original plans were taken on 2026-11-10, and `1130` is done. What is left, in order: the two plans written after this meta-plan (`1115`, `1125`), then the shop-dependent items, then the user-only items under **Verification**.

## ▶ Next executable step (resume here)

`1125` decisions taken 2026-11-11 (narrow; review-plan UC IDs + story criterion IDs, whole-suite fallback; system and test refactors). Next: `/create-plan` the implementation plan from `plans/20261110-1125-consider-narrow-test-runs-after-refactor.md`. Tag precondition already checked 2026-11-11: shop tests carry `@Tag("UC-001.N")` criterion tags.

## Order of remaining work

Shop-independent work first; shop-dependent items last (user, 2026-11-10).

1. `1125` narrow test runs after a refactoring: `plans/20261110-1125-consider-narrow-test-runs-after-refactor.md`. Decisions taken 2026-11-11 (answer: narrow). Remaining: `/create-plan` the implementation plan. That implementation edits `internal/atdd/process/process-flow.yaml`.
2. Shop-dependent, last: E2 shop config write (unblocked: v1.6.108 released 2026-11-10) and the Docker rehearsals, both tracked in `plans/20261104-1120-meta-backlog-consolidation.md`.
3. Delete this meta-plan once 1 is done (2 lives in the backlog meta-plan).

`1115` gh-optivem side done 2026-11-10 (commit 505b0861); its plan keeps only the deferred shop follow-up (drop `docker rmi -f`), blocked on the next release.

`1130` done 2026-11-10: `gh optivem characterize` entry point + Step 5 docs shipped (commits 884dfb51..4bb94684, then 63795d48). Plan deleted.

`1300` done 2026-11-10: v1.6.108 released (run 42806020020); `gh optivem spec validate` on shop `main` prints `OK: 12 use case(s) valid, 25 criteria gated` (UC-001 now has 16 criteria, UC-001.1–.16, because shop added 2 after the plan was written; UC-004 has 9). Plan deleted. Its shop-side copy is deleted from a shop session.

## Decisions taken (2026-11-10)

- `1302` UX/UI review: **out of scope** for gh-optivem (human manual QA). Plan deleted.
- `1300` release: last step of the run, one push to `main` carrying everything, then validate.
- `1250`: keep running the whole suite; narrowing deferred to `plans/20261110-1125-consider-narrow-test-runs-after-refactor.md`.
- `1301` legacy baseline: separate bulk process, system started once, layer by layer, gates on the core use case only, mandatory system-code safety gate. Implementation plan `20261110-1130-legacy-baseline-bulk-impl` (done, deleted). Per-ticket `cover-system-behavior` unchanged.
- `1258` Step 7 done; `1258` and the `1219` umbrella deleted.

## Verification (user, not executed by the run)

- `1305` Step 5: live GCP run with a short repo name, following `docs/cloud-setup.md`; settle its unverified bits (GHCR `/orgs/` vs `/users/` link, public-package default, multirepo script location).
- `1121`: waits on the walking-skeleton decision (meta-backlog Phase B); not part of this run.
- `1258` operator check: read `gh optivem spec generate` output on shop and judge if it reads well to a PO.
- Operator rehearsals from the deleted `1219` umbrella, once shop/Docker is free: Align loop (rule stated only in general terms → refiner proposes a numeric example; answer "no" once → next round uses the feedback); Use Case Criteria body section on a GitHub issue (re-run still parses); gift-wrap rehearsal (Red reviewer flags a planted dropped `.then().shouldSucceed()`; commits match the talk); Code & Architecture Review on shop (`refactor` → `review-code-and-architecture`, `docs/guidelines/<area>.md` gains a rule, a `GUIDELINES` commit lands); WIP-gate removal (final commit has no gate, pipeline runs the tests).
