# 2026-11-09 19:09 UTC — Meta-plan: run all active top-level plans

## TL;DR

**Why:** 8 top-level plans were open; most waited on user decisions.
**End result:** all decisions were taken on 2026-11-10. What is left, in order: `1130` implementation (3 parts), the `1300` release check, then the shop-dependent items, then the user-only items under **Verification**.

## ▶ Next executable step (resume here)

`1130` Step 3, part 1 (Go code): `/execute-plan plans/20261110-1130-legacy-baseline-bulk-impl.md`. Covers `system start --restart --external-only` (`internal/build/runner/system.go` + `system_commands.go`) and the Go actions `record-start-commits`, `select-next-use-case`, `check-system-unchanged`, `check-no-assertion-removed` (registered in `register.go`), with unit tests. No shop, no Docker.

## Order of remaining work

Shop-independent work first; shop-dependent items last (user, 2026-11-10).

1. `1130` Step 3, part 1: Go code (see above).
2. `1130` Step 3, part 2: `process-flow.yaml` (new/changed processes per the plan's Step 3 work list) + the 3 new agents + Step 4 process transition tests. Never in parallel with another plan that edits `process-flow.yaml`.
3. `1130` Step 3, part 3: `gh optivem characterize` entry point + Step 5 docs.
4. `1300` release check (its Step 5). Release run dispatched 2026-11-10 (user-approved): https://github.com/optivem/gh-optivem/actions/runs/42806020020. It tests against shop's latest `meta-v*` release (`meta-v1.0.224`), not shop `main`, so shop's CI state does not block it. A push to `main` does NOT cut a release (`gh-acceptance-stage.yml` is `workflow_dispatch` only). Once a release newer than v1.6.107 exists: clone shop into a temp dir (user-approved, read-only, deleted afterwards), run `gh optivem spec validate`, confirm 14 UC-001 criteria, delete `1300` (the shop-side copy is deleted from a shop session). If the run failed, root-cause it before re-dispatching.
5. Shop-dependent, last: E2 shop config write (needs the new release) and the Docker rehearsals, both tracked in `plans/20261104-1120-meta-backlog-consolidation.md`.
6. Delete this meta-plan once 1–4 are done (5 lives in the backlog meta-plan).

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
