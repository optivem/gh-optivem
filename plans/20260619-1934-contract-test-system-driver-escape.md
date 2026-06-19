# 2026-06-19 19:34:00 UTC — Prevent contract tests from escaping into the system-under-test

## TL;DR

**Why:** Rehearsal #65 burned ~16 min on an unwinnable fix loop because the `contract-test-writer` authored an ERP contract test that drove the *my-shop System Driver* (`.when().viewProductList()`) instead of the *external (ERP) driver port*. The call hit an unimplemented `TODO: System Driver` stub that **no contract-phase agent is scoped to implement**, so the test could never go green.
**End result:** Contract tests are authored given→then against the external-system driver port only — never via `.when()` system-under-test action chains — and if one ever escapes again, the pipeline fails it fast with a clear adjudication message instead of spending a full fix loop.

## Outcomes

What we get out of this — the goals and deliverables:

- The `contract-test-writer` agent never authors a contract test that uses a `.when().<systemAction>()` chain (or otherwise routes through the my-shop System Driver). Contract tests are given→then against the external-system driver port only. **(Confirmed by user: `.when()` is never legitimate in a contract test.)**
- A contract test that *does* escape into the system-under-test is caught **fast** — halted on first failure with a message that names the violation ("contract test routed through the my-shop System Driver; contract tests must exercise only the external-system driver port") — instead of consuming the 2-attempt fix loop (~16 min).
- The adjudication message a human sees on halt points at the real cause (test shape), not a generic "tests still red".
- **Scope:** this plan ships **Layer 1 + Layer 2**. A separate no-progress-guard bug surfaced while diagnosing this run (see Follow-up) is spun out into its own investigation, not bundled here.

## ▶ Next executable step (resume here)

**Scope is settled: ship Layer 1 + Layer 2.** The first executable unit is **Layer 1**: edit `internal/atdd/assets/runtime/agents/atdd/contract-test-writer.md` to add an explicit prohibition on `.when()` / system-under-test driver chains in contract tests, with the given→then ERP-port shape as the required pattern (model on the sibling `shouldBeAbleToGetProduct`). This is a prompt-text change only — no code, no test runs. It unblocks the source-level prevention; Layer 2 (the fail-fast classifier) is defense-in-depth and can follow independently. The no-progress-guard bug (Follow-up) is out of scope for this plan.

## Steps

### Layer 1 — AGENT (primary, source fix) — `contract-test-writer.md`

- [ ] Step 1: In `internal/atdd/assets/runtime/agents/atdd/contract-test-writer.md`, add an explicit rule: an external-system contract test MUST exercise only the external-system driver port (given→then), and MUST NOT use `.when().<systemAction>()` chains or otherwise route through the my-shop System Driver. State the sibling `shouldBeAbleToGetProduct` (given→then via `erpDriver.getProduct()`) as the canonical shape to mirror.
- [ ] Step 2: Add the "need a new external read?" guidance: if the contract needs a capability the ERP port lacks (e.g. "returns a product list"), the test should drive a new ERP-port read (e.g. `getProductList` mirroring `getProduct`) — never reuse the acceptance `when().viewProductList()` chain. Confirm this stays within the agent's existing scope (`ct-test`, `dsl-port`, `dsl-core`) and note the hand-off to the driver-adapter-implementer for the `adapter/external` wiring.
- [ ] Step 3: Confirm the new wording doesn't contradict the existing "model on the sibling contract test" instruction (it reinforces it) and doesn't remove intentional behavior.

### Layer 2 — COMMAND (fail-fast enforcement) — `gh optivem test run` classifier

- [ ] Step 4: Locate the `gh optivem test run` outcome classifier (the `*_commands.go` that maps a suite run to pass/fail/infra). Identify where a contract-suite failure is classified.
- [ ] Step 5: Add a rule: when a CONTRACT suite test fails with the `TODO: System Driver` marker in its output, classify it as a **contract-authoring violation** — halt immediately with the adjudication message, instead of returning an ordinary fixable red that feeds the fix loop. The marker is a **single cross-language substring** — `TODO: System Driver` is byte-identical across TypeScript, Java, and .NET (only the wrapping exception type differs), so the classifier needs one match, not per-language variants (verified against `system-driver-adapter-implementer.md:8` + the `language-equivalents` tables).
- [ ] Step 6: Decide the exact halt surface (new error-end mapping vs. a distinct verdict the BPMN already routes) so the message reaches the human cleanly. Capture as a sub-decision.

## Follow-up (separate plan — out of scope here)

- **No-progress-guard bug.** In run #65 `check-fix-progress` reported `fix-loop-progressing=true` even though the fixer made **no edits** and the failure signature was **byte-identical** across both passes — so `FIX_LOOP_NO_PROGRESS_EXHAUSTED` never fired and the count-cap `FIX_LOOP_EXHAUSTED` caught it on attempt 2 instead. The no-progress guard exists precisely to halt on attempt 1 when a fix pass changes nothing, so this looks like a standalone defect in `check-fix-progress` (`internal/atdd/process/`) with **broad benefit** — it would tighten *every* never-green fix loop, not just this contract case. Worth its own `/create-plan` + likely a `bpmn-logic-audit` pass. (This is the "Layer 3b" we split out; the scope-aware-halt idea, "3a", was dropped as redundant with Layer 2.)

## Decisions log

- **Layer scope:** ship **Layer 1 + Layer 2**. Layer 1 is the root-cause fix; Layer 2 is a cheap fail-fast that contains the blast radius regardless of which agent misbehaves. (Resolved 2026-06-19.)
- **Layer 3:** **3a (scope-aware halt) dropped** — redundant with Layer 2. **3b (no-progress-guard bug) spun out** to its own plan (see Follow-up). (Resolved 2026-06-19.)
- **`.when()` in contract tests:** confirmed by user — **never legitimate**. Contract tests are given→then against the external port only. Layer 1 is therefore a flat prohibition. (Resolved 2026-06-19.)
- **Marker cross-language:** `TODO: System Driver` is byte-identical across all three stacks → Layer 2 is a single substring match. (Verified 2026-06-19.)
