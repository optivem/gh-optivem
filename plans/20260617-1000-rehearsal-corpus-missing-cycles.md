# 2026-06-17 10:00:00 UTC — Rehearsal corpus: cover the four untested implement-ticket CYCLEs

## TL;DR

**Why:** The `atdd-rehearsal` corpus (61, 65, 68–72, 76) exercises only two of the six CYCLE branches of the `implement-ticket` gateway — `change-system-behavior` (story/bug) and `redesign-system-structure` (#61). Four task-subtype branches have **zero** rehearsal coverage, so a regression in any of them ships undetected.
**End result:** Four new `task`-kind tickets in `optivem/shop`, one per uncovered CYCLE, each wired into `DEFAULT_TICKETS` and `CONTRIBUTING.md` and proven to walk its intended CYCLE end-to-end under `gh-optivem-monolith-java.yaml`.

## Outcomes

What we get out of this — the goals and deliverables:

- Rehearsal coverage for every CYCLE branch of the `implement-ticket` two-level gateway except the still-unbuilt refine path (which is its own plan).
- A `task / system-refactor` ticket that walks `refactor-system-structure` (full-regression GREEN, no DSL/driver/port change).
- A `task / test-refactor` ticket that walks `refactor-test-structure` (`refactor-and-verify-tests`, test-only change).
- A `task / legacy-coverage` ticket that walks `cover-system-behavior` (writes **passing** ATs against existing behavior — the green-from-start cover path, distinct from #76's red-then-green bug path).
- A `task / external-system-redesign` ticket that walks `redesign-external-system-structure` (reshapes the ERP external boundary; #61 only covers the system-side sibling).
- `DEFAULT_TICKETS` in `scripts/atdd-rehearsal-loop.sh` and the corpus prose in `CONTRIBUTING.md` extended in sync, each with the documentation comment block the existing entries carry.

## ▶ Next executable step (resume here)

Resolve the **Open questions** first — they are design decisions, not mechanical edits, so this resumes under `/refine-plan`, not `/execute-plan`. Specifically: (1) confirm the concrete shop-domain scenario for each of the four tickets (recommendations are in the Steps below), and (2) confirm the exact `ticket-kind` / `task-subtype` label strings the tracker classifier (`Tracker.Classify` / `Tracker.Subtypes`) expects, by inspecting an existing task ticket or the classifier source. Once both are settled, the first executable unit is **Step 1a** (author the `system-refactor` shop issue).

## Steps

Each ticket follows the same four moves: author the shop issue → add to `DEFAULT_TICKETS` → add to `CONTRIBUTING.md` → rehearse and confirm the intended CYCLE was walked. Task-kind tickets use a **Checklist** body (not Gherkin AC) per the BPMN `AC XOR Checklist` parse rule.

### Step 0 — Prerequisites (shared)

- [ ] Determine the label vocabulary the tracker maps to `ticket-kind == task` and to each `task-subtype` (`system-refactor`, `test-refactor`, `legacy-coverage`, `external-system-redesign`). Inspect `Tracker.Classify` / `Tracker.Subtypes` and an existing task ticket; record the exact label strings so all four issues are classified correctly.

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

- [ ] 3a. Pick existing-but-uncovered behavior. **Recommended scenario:** add acceptance coverage for cancelling a *non-blackout* order (cancel already exists from the #76 domain) — i.e. behavior the system already has with no AT today. Verify the behavior is implemented but uncovered before authoring.
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

## Open questions

- **Domain scenario per ticket** — the Steps carry a recommended scenario each; confirm or substitute before authoring. The `legacy-coverage` one is load-bearing: it must target behavior that genuinely exists with no current AT.
- **Label vocabulary** (Step 0) — exact `ticket-kind`/`task-subtype` label strings are unverified; resolve from the classifier before authoring issues.
- **Rehearsal stacks** — recommend rehearsing all four under `gh-optivem-monolith-java.yaml` only (matches the documented corpus). Decide whether any should also be run under another stack (e.g. typescript/dotnet) for cross-stack confidence.
