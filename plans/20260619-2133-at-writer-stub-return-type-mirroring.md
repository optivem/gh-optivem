# 2026-06-19 21:33:00 UTC — acceptance-test-writer must mirror port return types in DSL-core stubs

## TL;DR

**Why:** Rehearsal #65 ("View product list") halted because `acceptance-test-writer` stubbed a DSL-core method as `viewProductList(): never { throw … }`. `never` has no members, so the acceptance test's fluent `.viewProductList().then()` chain failed to type-check and `gh optivem test compile` exited 2. The agent prompt tells the writer to add a throwing stub "so compilation works" but never says the stub must mirror the **port method's signature** (same return type) — leaving the agent free to invent a `never` return that compiles as a body but breaks every caller.

**End result:** `acceptance-test-writer.md` Step 2 requires every DSL-core stub to mirror its port method's exact signature (params + return type), explicitly forbidding `never`/placeholder return types, with a worked correct-vs-wrong example. A future run cannot have the writer emit a stub whose return type breaks the test's fluent chain, so compile cannot fail on agent-authored stubs in production or rehearsal.

## Outcomes

What we get out of this — the goals and deliverables:

- `acceptance-test-writer.md` Step 2 states the **signature-mirroring rule**: a DSL-core stub must declare the same parameter list and the same return type as the corresponding port interface method (`${dsl-port}`); the body throws `TODO: DSL`, but the declared return type is the real port type so every caller's fluent chain still compiles.
- The prompt **explicitly forbids** `never` (and any placeholder return type) on a stub, naming the failure mode: a `never`/placeholder return compiles as a body but breaks chained callers (TS2339 "Property X does not exist on type 'never'").
- The rule is **language-general** — the same "throwing method keeps its real declared return type" guidance covers the Java and .NET DSL stubs, not just TypeScript.
- A **worked example** in the prompt (or a shared chunk) shows the correct throwing stub that keeps its real return type next to the wrong `never` form, so the rule is concrete, not prose-only.
- No behavior is changed in the command or BPMN layers — both did their job; this is an agent-prompt-only prevention.

## ▶ Next executable step (resume here)

**Design/scope decision still open — do not edit yet.** Before any edit, settle the two open questions below with the user (tomorrow's chat): (1) is the layer scope agent-only, and (2) does the worked example live inline in `acceptance-test-writer.md` or in a shared chunk. Once both are settled, run `/execute-plan` on this file: edit Step 2 of `internal/atdd/assets/runtime/agents/atdd/acceptance-test-writer.md` to add the signature-mirroring rule + `never` prohibition (Step 1 below), then add the worked example in the location chosen (Step 2 below). No code/system changes — runtime prompt asset only.

## Steps

- [ ] Step 1 (primary): In `internal/atdd/assets/runtime/agents/atdd/acceptance-test-writer.md`, Step 2, add the signature-mirroring requirement — the DSL-core stub must mirror the port method's exact signature (same params + same return type as declared in `${dsl-port}`); body throws `TODO: DSL`; declared return type must be the real port return type so callers' fluent chains compile. Explicitly forbid `never`/placeholder return types and name the TS2339 failure mode. Phrase language-generally so it covers Java/.NET stubs too.
- [ ] Step 2 (reinforcement): Add a short worked DSL-stub example — correct throwing stub keeping its real return type vs. the wrong `never` form — either inline in `acceptance-test-writer.md` Step 2 or in a shared chunk under `internal/atdd/assets/runtime/shared/` (location TBD — see Open questions).
- [ ] Step 3 (verify): Re-read the edited prompt to confirm the rule is unambiguous and the example renders; optionally re-run the #65 rehearsal scenario to confirm the writer now emits a compiling stub. (No new test infra — reuse the existing rehearsal loop.)

## Open questions

- **Layer scope (provisional — confirm tomorrow).** Diagnosis says this is an agent-only defect: the command (`gh optivem test compile`) correctly classified the non-compiling suite as `command-failed`, and the BPMN FIX→fixer self-heal path works in a real run (the rehearsal deliberately auto-rejects it to surface the bad artifact). Recommended: keep the plan **agent-only**. Confirm we're not also touching command/BPMN.
- **Example location.** Inline in `acceptance-test-writer.md` Step 2 (keeps the rule and its example together) vs. a shared chunk under `internal/atdd/assets/runtime/shared/` (reusable if other agents need the same stub guidance). Lean inline unless a shared chunk already hosts comparable stub guidance.
- **Worked-example piece optional?** Decide whether Step 2 (the example) ships with Step 1 or is deferred — Step 1 alone may be sufficient prevention.

## Diagnosis (reference — from /atdd-postmortem of run 20260619-204710)

- **Ticket:** #65 View product list (worktree `rehearsal-20260619-224651-65-view-product-list`).
- **Halt path (cosmetic):** `… → COMPILE_TESTS → EXECUTE_COMMAND → FIX → FIX_REJECTED_END` ("Fix Declined — Run Halted"). The FIX rejection is the rehearsal harness auto-rejecting the remediation gate (`ASK_HUMAN → rejected` in 0s) — intentional rehearsal behavior to surface bad artifacts, **not** the root cause.
- **Real failure:** `gh optivem test compile` exited 2 (`failure-kind=command-failed`):
  - `view-product-list-positive-test.spec.ts(15,14): error TS2339: Property 'then' does not exist on type 'never'.`
  - `view-product-list-positive-test.spec.ts(27,14): error TS2339: Property 'then' does not exist on type 'never'.`
- **Root cause (pinned):** `system-test/typescript/src/testkit/dsl/core/scenario/when/when-stage.ts:37` → `viewProductList(): never { throw new Error('TODO: DSL'); }`. The test chains `.viewProductList().then()…` (spec lines 15, 27); `never` has no members, so `.then()` won't type-check and the suite won't compile.
- **Convention violated:** Other DSL-core stubs throw at runtime but **preserve their real return type**, so the chain compiles and red comes from the runtime throw (matching `verify-pending-on=dsl`). Example: `core/scenario/given/given-product.ts:13` → `withName(_name: string): this { throw new Error('TODO: DSL'); }`. The port already declares the right type: `port/when/when-stage.ts:14` → `viewProductList(): WhenViewProductList;`. Correct stub: `viewProductList(): WhenViewProductList { throw new Error('TODO: DSL'); }`.
