# 2026-06-22 11:52:00 UTC — Acceptance-test GIVEN precondition consistency (acceptance-test-writer)

## TL;DR

**Why:** Rehearsal #70 authored an acceptance test that seeds an order for SKU `"DELL-XPS"` but never seeds a matching product, so it fails in GIVEN setup (`Product does not exist for SKU: DELL-XPS-…`) — a *false RED* that can never go green via production code. The orchestration can't distinguish a false RED from a real one, so it ran the whole implement chain and dead-ended at the human-gated unexpected-failing-tests-fixer (exit 32).
**End result:** The `acceptance-test-writer` runtime prompt carries a domain-neutral precondition-consistency rule — *when you customize an attribute that cross-references another seeded entity, seed that entity with the matching value; don't rely on auto-defaults once you override a cross-referencing key* — so future tests have internally consistent GIVENs across any scaffolded project, not just shop.

## Outcomes

What we get out of this:

- `internal/atdd/assets/runtime/agents/atdd/acceptance-test-writer.md` states a precondition-consistency authoring principle that a writer agent will follow when translating ACs into tests.
- The principle is **domain-neutral**: products/SKU appear only as one illustrative example; the rule generalizes to orders→products, users→accounts, bookings→rooms, etc.
- A future acceptance-test-writer run no longer produces a GIVEN that references an entity it didn't seed with a matching key — eliminating this class of false-RED test before it enters the implement chain.
- The render/placeholder test matrix still passes (no new `${placeholder}` introduced; this is prose only).

## ▶ Next executable step (resume here)

Edit `internal/atdd/assets/runtime/agents/atdd/acceptance-test-writer.md` Step 1 (near the line 30 "model each new test on the existing sibling test … copy its shape, annotations, and DSL chain" guidance) to add the domain-neutral precondition-consistency principle described in Step 1 below. Prose-only edit to a single runtime asset; gate for review before commit per repo convention. Then run the asset/render verification (Step 2) and confirm no test-matrix breakage.

## Steps

- [ ] Step 1: In `acceptance-test-writer.md`, add a domain-neutral precondition-consistency principle to Step 1, adjacent to the existing "model each new test on the existing sibling test" guidance. Draft wording (refine during execution):
  > **Keep GIVEN preconditions self-consistent.** When you customize an attribute that cross-references another seeded entity (for example an order line referencing a product by its key, a coupon code, or a country), you must also seed that referenced entity with the *same* value. Auto-seeded defaults use the default key, so once you override a cross-referencing attribute the default will not match and the precondition will fail before the behavior under test runs. Mirror the seed order shown in the sibling/reference test.

  Constraints: do **not** hardcode SKU/product/eshop vocabulary as the rule (example only); keep it additive to existing Step 1 prose; introduce no new `${placeholder}`.
- [ ] Step 2: Verify the prompt still renders — run the runtime-prompt render/placeholder check (e.g. `scripts/test.sh` scoped to the render matrix, or the `TestRenderMatrix_NoUnfilledPlaceholders` package) to confirm no unfilled placeholder and no regression. Follow the Windows go-test safety rule (never unbounded `go test ./...`).
- [ ] Step 3: Gate the edited file for user review, then commit on approval (raw git, scoped to gh-optivem).

## Notes / future considerations (not plan work)

- Defense-in-depth at the **command** layer (classify a failure that occurs in a GIVEN frame as a precondition/authoring failure rather than a plain RED) and the **BPMN** layer (gate that RED is for the WHEN/THEN reason, not setup) were considered and **deliberately left out**: both depend on a shop-DSL-specific stack-frame signal (`…dsl.core.scenario.given…`) and do not generalize. They only become worthwhile if the `com.optivem.testing` framework later exposes a structured given-vs-then phase marker.
- A shop-repo DSL change (e.g. `GivenImpl.setupErp` auto-seeding the default product using the order's referenced key instead of `DEFAULT_SKU`) would also prevent this, but lives in the scaffolded app, outside gh-optivem's three layers.
