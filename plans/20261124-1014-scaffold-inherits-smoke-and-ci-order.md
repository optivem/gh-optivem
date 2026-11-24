# 2026-11-24 10:14 UTC — Verify scaffolded repos inherit shop's post-deploy smoke job and contract-first CI order

> Comes from shop's meta-plan unit X1: Step 2 of `plans/backlog/after-cloud-deploy/20261016-1058-qa-stage-smoke-tests.md` (B-QAS, was Step 6) and Step 3 of `plans/backlog/after-cloud-deploy/20261016-1117-align-test-order-with-environment-model.md` (B-ORD). Shop's side of both is done and green in CI.

## TL;DR

**Why:** Shop now (1) runs `gh optivem system-test run --suite smoke-real` in a `smoke` job at the end of `_deploy-stage.yml`, which `_qa-stage.yml`'s `record` and `_prod-stage.yml`'s `release` jobs depend on, and (2) runs every contract pass before any acceptance suite in CI (`_acceptance-stage.yml`, `_acceptance-stage-docker-compose.yml`). Student repos should get both with no hand-editing, but nobody has checked a scaffold since.
**End result:** a test scaffold for each architecture and language has the `smoke` job (with correctly rewritten paths, toolchain setup and `GH_OPTIVEM_CONFIG`) gating QA record and release, and runs the latest suites in the CI order below. Any gh-optivem doc or test that lists the CI suite order matches it; local-order listings stay unchanged.

## Target CI order (shop, decided 2026-11-16)

Smoke (`smoke-stub`, `smoke-real`) → all Contract (`contract-stub`, `contract-stub-isolated`, `contract-real`) → Acceptance ×4 → E2E. The **local** order (`tests.yaml`, plain `gh optivem system-test run`) stays Smoke → Acceptance → Contract → E2E. The legacy suite set keeps its module-introduction order.

## What gh-optivem already does (checked 2026-11-24)

- `copySharedPipeline` (`internal/scaffolding/steps/shared_pipeline.go`) copies every shared `_*.yml` the callers reference, transitively, so `_qa-stage.yml` → `_deploy-stage.yml` (with its `smoke` job) and both `_acceptance-stage*.yml` come across verbatim from shop. The CI order therefore lives only in shop's files and is inherited as-is.
- `grep -rn "contract-stub\|acceptance-parallel"` outside `plans/` finds no CI-order listing in `MAPPING.md`, `README.md` or the scaffolding tests; the hits are ATDD suite selection (`internal/atdd/runtime/testselect/suite.go`, `actions/command.go`), which is not CI order. Recheck at execution time.

So the expected work is verification, plus fixes only if a scaffold shows a gap.

## ▶ Next executable step (resume here)

Step 1: check that `templates.FixupWorkflowContent` (run from `apply_template.go`) rewrites the `smoke` job in the copied `_deploy-stage.yml` correctly for each flavor: the `system-test/<lang>` paths, the toolchain setup steps, `GH_OPTIVEM_CONFIG`, and for multirepo the `multirepoSharedWorkflowRewrites` targets.

## Steps

- [ ] Step 1: Read shop's `_deploy-stage.yml` `smoke` job and confirm every shop-specific line in it is covered by the per-flavor `contentReplacements` in `internal/scaffolding/steps/apply_template.go` (monolith, multitier, multirepo). Confirm `steps/verify.go` needs no change. Fix any missing rewrite, with a unit test.
- [ ] Step 2: Recheck `grep -rn "contract-stub\|acceptance-parallel"` (and `smoke-real`) across gh-optivem docs and scaffolding tests for any listing of the CI suite order. Update CI-order listings to the target order; leave local-order listings alone.
- [ ] Step 3: Run a test scaffold for each architecture/language. In each generated repo check: `_deploy-stage.yml` has the `smoke` job; `_qa-stage.yml` `record` and `_prod-stage.yml` `release` have `needs: [check, deploy]`; the acceptance workflows run the latest suites in the target CI order. Where a cloud project is available, run the QA stage once and confirm `deploy / smoke` passes before `record`.
- [ ] Step 4: Tell shop the result so the orchestrator can tick B-QAS Step 2 and B-ORD Step 3 in the shop plans.
