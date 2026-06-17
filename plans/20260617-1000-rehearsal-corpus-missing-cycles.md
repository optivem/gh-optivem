# 2026-06-17 10:00:00 UTC — Rehearsal corpus: cover the four untested implement-ticket CYCLEs

## TL;DR

**Why:** The `atdd-rehearsal` corpus (61, 65, 68–72, 76) exercises only two of the six CYCLE branches of the `implement-ticket` gateway — `change-system-behavior` (story/bug) and `redesign-system-structure` (#61). Four task-subtype branches have **zero** rehearsal coverage, so a regression in any of them ships undetected.
**End result:** Four new `task`-kind tickets in `optivem/shop`, one per uncovered CYCLE, each wired into `DEFAULT_TICKETS` and `CONTRIBUTING.md` and proven to walk its intended CYCLE end-to-end under `gh-optivem-monolith-java.yaml`.

## Outcomes

What we get out of this — the goals and deliverables:

- Rehearsal coverage for every CYCLE branch of the `implement-ticket` two-level gateway except the still-unbuilt refine path (which is its own plan).
- A `task / system-refactor` ticket that walks `refactor-system-structure` (full-regression GREEN, no DSL/driver/port change).
- A `task / test-refactor` ticket that walks `refactor-test-structure` (`refactor-and-verify-tests`, test-only change).
- A `task / legacy-coverage` ticket that walks `cover-system-behavior` (writes a **passing** AT for coupon validity-window enforcement during PlaceOrder — existing-but-uncovered behavior in `CouponService.getDiscount` — the green-from-start cover path, distinct from #76's red-then-green bug path).
- A `task / external-system-redesign` ticket that walks `redesign-external-system-structure` (reshapes the ERP external boundary; #61 only covers the system-side sibling).
- `DEFAULT_TICKETS` in `scripts/atdd-rehearsal-loop.sh` and the corpus prose in `CONTRIBUTING.md` extended in sync, each with the documentation comment block the existing entries carry.

## ▶ Next executable step (resume here)

**All authoring + file edits are done** (2026-06-17): the four shop issues are live — **#78** `subtype:system-refactor`, **#79** `subtype:test-refactor`, **#80** `subtype:legacy-coverage`, **#81** `subtype:external-system-redesign` (all native Issue Type **Task**, verified) — and both `DEFAULT_TICKETS` (loop script) and `CONTRIBUTING.md` are extended in sync, with the corpus-count references updated to `61 65 68 69 70 71 72 76 78 79 80 81`.

The only remaining work is the **four rehearsals + final corpus sweep** (Steps R1–R5 below). These are **slow end-to-end ATDD orchestrator runs** (fresh worktree → binary build → docker clean → implement) and are **operator-run** — they are not triggered automatically. Run them one at a time, confirm each walked its intended CYCLE from the `.log`/trace, then delete the step. **Start with R1 (#78).**

## Steps — remaining (operator-run rehearsals)

Authoring (issues #78–#81, Type=Task + `subtype:*` labels), `DEFAULT_TICKETS`, and `CONTRIBUTING.md` (incl. corpus-count references) are **all complete** — see git history on `gh-optivem`. What remains is verifying each ticket walks its intended CYCLE end-to-end. Each rehearsal is a slow ATDD orchestrator run; run from `../shop`, one at a time, and delete the step once its trace/`.log` confirms the expected CYCLE.

- [ ] **R1 — #78 `system-refactor`.** `bash scripts/atdd-rehearsal.sh 78 --config gh-optivem-monolith-java.yaml --auto --headless`. Confirm it dispatched `refactor-system-structure` and ran the full regression GREEN (no AT-red phase, no DSL/driver/port change).
- [ ] **R2 — #79 `test-refactor`.** `bash scripts/atdd-rehearsal.sh 79 --config gh-optivem-monolith-java.yaml --auto --headless`. Confirm it walked `refactor-test-structure` → `refactor-and-verify-tests` with tests staying GREEN (no production change).
- [ ] **R3 — #80 `legacy-coverage`.** `bash scripts/atdd-rehearsal.sh 80 --config gh-optivem-monolith-java.yaml --auto --headless`. Confirm it walked `cover-system-behavior` → `write-and-verify-acceptance-tests-pass` (`verify-mode: green-when-complete`): the AT was GREEN from the first verify, no system-implementation phase.
- [ ] **R4 — #81 `external-system-redesign`.** `bash scripts/atdd-rehearsal.sh 81 --config gh-optivem-monolith-java.yaml --auto --headless` (ERP is declared in this config). Confirm it walked `redesign-external-system-structure` → `update-external-system-driver-adapters` with full-regression GREEN.
- [ ] **R5 — Final corpus sweep.** `bash scripts/atdd-rehearsal-loop.sh` to confirm the four new tickets pass alongside the existing eight (12 total). The corpus-count references in `CONTRIBUTING.md` + loop-script header are already updated; this step only re-verifies the whole corpus green.

## Resolved decisions

*(Resolved 2026-06-17 via `/refine-plan`.)*

- **Domain scenario per ticket** — **accept all four recommended scenarios**, with one substitution. The `system-refactor` (`OrderPricing` extraction), `test-refactor` (acceptance-DSL builder extraction), and `external-system-redesign` (reshape ERP `GetProductResponse`) scenarios stand as written. The `legacy-coverage` target was **changed**: "cancel a non-blackout order" is already covered by `CancelOrderPositiveTest`, so it is *not* uncovered behavior. The new target is **coupon validity-window enforcement during PlaceOrder** (expired coupon → `couponCode: "...has expired"`), implemented in `CouponService.getDiscount` (`validTo` check) but uncovered by any AT — a genuine green-from-start `cover-system-behavior` case. See Step 3a.
- **Label vocabulary** — resolved from source (`tracker/github/github.go` + `process-flow.yaml` `GATE_TASK_SUBTYPE`). `ticket-kind == task` is the issue's **native GitHub Issue Type = "Task"** (not a label); `task-subtype` is a single **`subtype:<value>`** label per issue, with values `system-refactor` / `test-refactor` / `legacy-coverage` / `external-system-redesign`. Full encoding in Step 0.
- **Rehearsal stacks** — **author + verify under Java only** (`gh-optivem-monolith-java.yaml`) for all four, matching the documented corpus default. These tickets exercise orchestration gateway branches (which CYCLE the BPMN walks), not language-specific code, so one stack proves the branch. *Note:* the loop already accepts `--config <yaml>` (default `gh-optivem-monolith-java.yaml`), so the operator can run the whole corpus — including these four — under `gh-optivem-monolith-typescript.yaml` / `gh-optivem-monolith-dotnet.yaml` on demand without any plan change; the four tickets just join `DEFAULT_TICKETS` once. **Caveat carried into execution:** cross-stack parity of the four scenarios is *not* verified by this plan (e.g. the `given().coupon().withValidFrom/withValidTo` DSL and `CouponService` validity rules are confirmed in the Java shop only) — a `--config typescript`/`dotnet` corpus run may surface a stack gap that is out of scope here.
