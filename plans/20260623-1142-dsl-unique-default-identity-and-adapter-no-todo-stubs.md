# 2026-06-23 11:42:31 UTC — DSL unique default identities + adapter no-TODO-stub gate (#65 postmortem)

## TL;DR

**Why:** The #65 (View product list) rehearsal halted at the contract-real GREEN verify because the contract test seeded two products that both inherited the single shared `DEFAULT_SKU`, and the real json-server ERP simulator rejected the duplicate id. A second, latent defect — `getProductList` left as a `TODO` driver stub — would have halted the very next run. Both are agent-layer authoring defects; the BPMN and `gh optivem` command behaved correctly.
**End result:** Two ATDD runtime agent prompts are tightened so that (A) the DSL auto-assigns a **distinct** default identity per seeded entity instance for **every** domain entity (not just products), and (B) the external-system driver-adapter implementer never leaves a `TODO`/throw stub on a port method the contract suite exercises. A future #65-style multi-instance GIVEN reaches green contract-real on the first verify, for any domain.

## Outcomes

What we get out of this — the goals and deliverables:

- `dsl-implementer.md` instructs that when the DSL auto-seeds a default identity for an entity in a `given()` step, **multiple instances of the same entity in one scenario each get a distinct default identity** (so the real, id-enforcing external simulator never sees a duplicate id). Stated **domain-agnostically** — covers products (SKU), orders (order-number), coupons (coupon-code), and any future entity — not product-specific.
- The rule is framed to stay **faithful to the shop reference testkit DSL** (the implementer mirrors the reference; it does not invent a bespoke scheme).
- `external-system-driver-adapter-implementer.md` requires implementing **every** driver port method the contract tests under verification exercise, with an explicit **done-check**: grep the concrete driver(s) for remaining `TODO`/throw stubs on port methods the contract suite calls before declaring complete.
- No BPMN, command, contract-test-writer, or acceptance-test-writer changes (explicitly out of scope).

## ▶ Next executable step (resume here)

Step 1 — read `internal/atdd/assets/runtime/agents/atdd/dsl-implementer.md` end-to-end, locate where it describes Given-step / default-value / identity-seeding behavior, and draft the domain-agnostic "distinct default identity per instance" rule as a targeted insertion (mirror the prompt's existing voice and the cross-domain framing already used elsewhere in the file). No code changes — edit the prompt only. This unblocks Step 2 (the adapter done-check), which is independent and can follow in the same session.

## Steps

- [ ] Step 1: **dsl-implementer — distinct default identity per instance (Fix A).** In `internal/atdd/assets/runtime/agents/atdd/dsl-implementer.md`, add an instruction: when the DSL auto-assigns a *default* identity to an entity seeded in a `given()` step, multiple instances of the **same** entity within one scenario must each receive a **distinct** default identity, rather than all inheriting one shared constant. Keep it at the **requirement level** — state the property (distinct-per-instance, domain-agnostic, faithful to the shop reference DSL) and do **not** prescribe a specific algorithm (the reference DSL dictates the concrete scheme, e.g. counter vs. derive-from-name). Frame it **domain-agnostically** (products/SKU, orders/order-number, coupons/coupon-code, future entities). Cite the concrete failure mode (real id-enforcing simulator rejects duplicate id; a stub would silently tolerate it, hiding the bug until contract-real).
- [ ] Step 2: **driver-adapter implementer — no leftover TODO stubs (Fix B).** In `internal/atdd/assets/runtime/agents/atdd/external-system-driver-adapter-implementer.md`, add the requirement to implement **every** driver port method the contract tests under verification exercise, plus a **done-check** step: grep the concrete driver(s) for remaining `TODO: External System Driver` / `throw` stubs **on port methods the contract suite calls** (verify-path scope only — do not force implementing out-of-scope methods), and treat any remaining stub on that path as incomplete.
- [ ] Step 3: **Consistency pass.** Re-read both edited prompts in full to confirm the new rules don't restate preamble/scope chunks, don't contradict existing instructions, and match each file's voice; confirm no unresolved `${placeholder}` was introduced.
- [ ] Step 4: **Build/render check.** Run the prompt-render test that exercises agent × architecture × channel matrices (e.g. `go test` for the clauderun render package, scoped per the Windows-safe test guidance) to confirm the edited prompts still render with no unfilled placeholders.

## Verification (operator, not agent steps)

- Re-run the #65 view-product-list rehearsal with an operator present; confirm contract-real reaches green on the first `VERIFY_TESTS_PASS_CONTRACT_REAL_AFTER_SIMULATOR` without entering the unexpected-failing-tests fixer / human gate.
- Confirm the multi-instance GIVEN works for a non-product domain (an order/coupon multi-seed) in a later rehearsal.

## Decisions (resolved)

- **Fix A stays requirement-level** — the prompt states the property (distinct-per-instance, domain-agnostic, faithful to the shop reference DSL) and does not mandate a specific algorithm; the reference DSL dictates the concrete scheme.
- **Fix B done-check is verify-path-scoped** — grep only port methods the contract suite calls, not all port methods, to avoid forcing implementation of out-of-scope methods.
