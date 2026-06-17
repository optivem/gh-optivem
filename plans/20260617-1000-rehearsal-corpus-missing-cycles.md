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

All open questions are resolved (see **Resolved decisions**) — scenarios confirmed, label encoding verified from source, Java-only rehearsal. The plan is ready for `/execute-plan`. The first executable unit is **Step 1a**: author the `system-refactor` shop issue (native Issue Type **Task** + label `subtype:system-refactor`, Checklist body, `OrderPricing`-extraction scenario), mirroring ticket **#61**.

## Steps

Each ticket follows the same four moves: author the shop issue → add to `DEFAULT_TICKETS` → add to `CONTRIBUTING.md` → rehearse and confirm the intended CYCLE was walked. Task-kind tickets use a **Checklist** body (not Gherkin AC) per the BPMN `AC XOR Checklist` parse rule.

### Step 0 — Prerequisites (shared)

- [ ] Classify each of the four issues using the **verified encoding** (resolved 2026-06-17 from `internal/atdd/runtime/tracker/github/github.go` + the `GATE_TASK_SUBTYPE` branches in `process-flow.yaml`):
  - **`ticket-kind == task`** is the issue's **native GitHub Issue Type** — set the issue's *Type* to **"Task"** in the GitHub UI/API. `Tracker.Classify` reads `repository.issue.issueType.name` via `gh api graphql` and lowercases it; it is **not** a label. An issue with no native type is rejected (`confident=false`) — the operator must set it before the orchestrator proceeds.
  - **`task-subtype`** is a **label** of the form **`subtype:<value>`** (`Tracker.Subtypes` strips the `subtype:` prefix). Add exactly **one** such label per issue — the intake gate treats 0 as "operator must declare" and 2+ as "must reconcile". The four values (exact strings, matching the `GATE_TASK_SUBTYPE` `when:` clauses):
    - `subtype:system-refactor` → `refactor-system-structure`
    - `subtype:test-refactor` → `refactor-test-structure`
    - `subtype:legacy-coverage` → `cover-system-behavior`
    - `subtype:external-system-redesign` → `redesign-external-system-structure`
  - Mirror the existing `task / system-redesign` ticket **#61** for label/type placement when authoring.

### Step 1 — `task / system-refactor` → `refactor-system-structure`

- [ ] 1a. Author shop issue. **Recommended scenario:** extract the cart-line discount + shipping-fee math into a single internal pricing component (e.g. an `OrderPricing` calculator), preserving all behavior. Checklist body; no DSL/driver/port change. Labels per Step 0.
- [ ] 1b. Add the issue number to `DEFAULT_TICKETS` in `scripts/atdd-rehearsal-loop.sh` under a new `--- structural refactor (no behavior change) ---` group, with a comment block matching the existing style.
- [ ] 1c. Add a matching subsection to `CONTRIBUTING.md` ("For structural refactor — system internals").
- [ ] 1d. Rehearse: `bash scripts/atdd-rehearsal.sh <n> --config gh-optivem-monolith-java.yaml --auto --headless`. Confirm from the trace/`.log` it dispatched `refactor-system-structure` and ran the full regression GREEN (no AT-red phase).

### Step 2 — `task / test-refactor` → `refactor-test-structure`

- [ ] 2a. Author shop issue. **Recommended scenario:** extract repeated cart/order-building setup in the acceptance DSL into a shared builder/helper, with no production change. Checklist body. Labels per Step 0.
- [ ] 2b. Add to `DEFAULT_TICKETS` (same `--- structural refactor ---` group).
- [ ] 2c. Add the `CONTRIBUTING.md` subsection ("For structural refactor — test structure").
- [ ] 2d. Rehearse under `gh-optivem-monolith-java.yaml`; confirm it walked `refactor-test-structure` → `refactor-and-verify-tests` with tests staying GREEN.

### Step 3 — `task / legacy-coverage` → `cover-system-behavior`

- [ ] 3a. Pick existing-but-uncovered behavior. **Confirmed scenario (verified 2026-06-17):** add acceptance coverage for **coupon validity-window enforcement during PlaceOrder** — placing an order with an *expired* coupon (`validTo` in the past) is rejected with `couponCode: "Coupon code <X> has expired"`. The rule is implemented in `CouponService.getDiscount` (the `validTo` check) but has **no AT** today — `PlaceOrderNegativeTest` covers only coupon does-not-exist and usage-limit-reached, not the validity window. The `given().coupon()` DSL already exposes `withValidFrom`/`withValidTo`, so the scenario is authorable as-is. (The not-yet-valid `validFrom` rule is an equally-uncovered sibling and may be added as a second assertion.) *Rejected the earlier "cancel a non-blackout order" target: `CancelOrderPositiveTest` already covers it, so it is not uncovered behavior.*
- [ ] 3b. Author shop issue (Checklist body, labels per Step 0).
- [ ] 3c. Add to `DEFAULT_TICKETS` under a new `--- legacy coverage (passing ATs for existing behavior) ---` group.
- [ ] 3d. Add the `CONTRIBUTING.md` subsection ("For legacy coverage — write passing ATs").
- [ ] 3e. Rehearse; confirm it walked `cover-system-behavior` → `write-and-verify-acceptance-tests-pass` (`verify-mode: green-when-complete`), i.e. the AT was GREEN from the first verify with no system-implementation phase.

### Step 4 — `task / external-system-redesign` → `redesign-external-system-structure`

- [ ] 4a. Author shop issue. **Recommended scenario:** structurally reshape the ERP `GetProductResponse` (e.g. nest `price` under a `pricing` object, or rename a field) with no behavior change — full regression preserved. Checklist body, labels per Step 0.
- [ ] 4b. Add to `DEFAULT_TICKETS` under a new `--- external-system redesign ---` group.
- [ ] 4c. Add the `CONTRIBUTING.md` subsection ("For structural redesign — external system boundary").
- [ ] 4d. Rehearse under `gh-optivem-monolith-java.yaml` (ERP is declared there). Confirm it walked `redesign-external-system-structure` → `update-external-system-driver-adapters` with full-regression GREEN.

### Step 5 — Final corpus sweep

- [ ] Run the full loop (`bash scripts/atdd-rehearsal-loop.sh`) to confirm the four new tickets pass alongside the existing eight, then update the corpus count/order references in `CONTRIBUTING.md` and the loop-script header comment.

## Resolved decisions

*(Resolved 2026-06-17 via `/refine-plan`.)*

- **Domain scenario per ticket** — **accept all four recommended scenarios**, with one substitution. The `system-refactor` (`OrderPricing` extraction), `test-refactor` (acceptance-DSL builder extraction), and `external-system-redesign` (reshape ERP `GetProductResponse`) scenarios stand as written. The `legacy-coverage` target was **changed**: "cancel a non-blackout order" is already covered by `CancelOrderPositiveTest`, so it is *not* uncovered behavior. The new target is **coupon validity-window enforcement during PlaceOrder** (expired coupon → `couponCode: "...has expired"`), implemented in `CouponService.getDiscount` (`validTo` check) but uncovered by any AT — a genuine green-from-start `cover-system-behavior` case. See Step 3a.
- **Label vocabulary** — resolved from source (`tracker/github/github.go` + `process-flow.yaml` `GATE_TASK_SUBTYPE`). `ticket-kind == task` is the issue's **native GitHub Issue Type = "Task"** (not a label); `task-subtype` is a single **`subtype:<value>`** label per issue, with values `system-refactor` / `test-refactor` / `legacy-coverage` / `external-system-redesign`. Full encoding in Step 0.
- **Rehearsal stacks** — **author + verify under Java only** (`gh-optivem-monolith-java.yaml`) for all four, matching the documented corpus default. These tickets exercise orchestration gateway branches (which CYCLE the BPMN walks), not language-specific code, so one stack proves the branch. *Note:* the loop already accepts `--config <yaml>` (default `gh-optivem-monolith-java.yaml`), so the operator can run the whole corpus — including these four — under `gh-optivem-monolith-typescript.yaml` / `gh-optivem-monolith-dotnet.yaml` on demand without any plan change; the four tickets just join `DEFAULT_TICKETS` once. **Caveat carried into execution:** cross-stack parity of the four scenarios is *not* verified by this plan (e.g. the `given().coupon().withValidFrom/withValidTo` DSL and `CouponService` validity rules are confirmed in the Java shop only) — a `--config typescript`/`dotnet` corpus run may surface a stack gap that is out of scope here.
