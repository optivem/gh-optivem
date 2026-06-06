# Plan: Decide where the acceptance-test isolation decision is made

> **STATUS: needs a human design decision before any code/prompt change.** This plan frames the
> question and the options; it does **not** prescribe an implementation. Spun out of
> `plans/20260606-1356-run-isolated-acceptance-suites.md` (which fixes the *run* side — making the
> `acceptance` group actually execute isolated suites). This plan is the *authoring* side.

## Why

The scaffold ships two flavours of acceptance test: plain, and `@Isolated`
(`com.optivem.testing.Isolated`). `@Isolated` marks a test that controls **process-global state
shared across concurrently-running tests** and therefore must run serially (`-DincludeTags=isolated`
with `maxParallelForks=1`). In the shop demo the only such state is the **clock**
(`given().clock().withTime(...)`) and **promotion** (`given().promotion()...`); everything else is
per-scenario data (product / country / coupon) that needs no isolation.

Today there is **no guidance anywhere** (`grep -ri isolat` over `.claude/agents/`,
`internal/assets/runtime/`, `docs/atdd/` → nothing) on when a new acceptance test should be
`@Isolated`. `acceptance-test-writer.md` does a "mechanical 1:1 translation … model each new test on
the existing sibling test" — so whether a new AT is isolated depends purely on which sibling the
agent happens to read. The choice is **accidental**.

Failure modes of getting it wrong:
- **Under-isolation** (should be `@Isolated`, written plain) → the test flakes under parallel forks.
  This is the dangerous one.
- **Over-isolation** (plain test tagged `@Isolated`) → runs serially, slower but correct.

## The hard constraint (rules out the obvious fix)

The obvious fix — "tell the agent: isolate clock/promotion tests" — is **wrong**. `clock` and
`promotion` are shop-domain builders. A student/teacher domain may have neither, and may have its
own globals (a feature flag, a global config, a system clock). Hardcoding shop builder names into a
generic agent prompt is exactly the scaffold→agent coupling `[[feedback_no_scaffold_repo_coupling]]`
forbids. Any solution must be **domain-agnostic**.

## Options (for the human to choose)

**A. Writer decides, via a concept-based rule.** Add to `acceptance-test-writer.md`: *"Default to a
plain test. Use `@Isolated` only when the scenario sets process-global state shared across tests that
can't be scoped per-scenario (a singleton like the system clock or a global toggle). Anchor: if an
existing test using the same `given()`-builder is `@Isolated`, mirror it."* Clock/promotion are
examples in the shop, never the rule.
- *Pro:* self-contained; degrades correctly (a domain with no shared-global builders has no
  `@Isolated` siblings → agent always writes plain).
- *Con:* asks the writer to *classify*, which sits right at the "don't classify, just translate 1:1"
  boundary the prompt currently draws (Step 1). Concept-based judgement is the kind of thing agents
  get subtly wrong, and the dangerous failure mode (under-isolation) is the silent one.

**B. The criterion carries it; the writer mirrors.** The isolation property is decided upstream —
the ticket author or the acceptance-criteria refiner marks a criterion as isolated — and the writer
just translates it through, no judgement. Keeps the writer's "translate, don't classify" boundary
clean.
- *Pro:* decision made by whoever understands the domain (human or refiner with domain context);
  writer stays mechanical.
- *Con:* needs a place to *express* isolation in the AC/ticket format (a tag/marker), and the
  refiner (`acceptance-criteria-refiner`) or ticket schema must learn it. More surface.

**C. Leave it to sibling-mirroring (status quo, documented).** Do nothing mechanical; rely on "model
on the existing sibling." Optionally document the convention in a DSL reference doc.
- *Pro:* zero new mechanism; works whenever a representative `@Isolated` sibling already exists.
- *Con:* brand-new global-state behaviour (no sibling yet) is a coin-flip; the gap that started
  this stays open for genuinely new cases.

## Recommendation (non-binding — human call)

Lean **B** for correctness-critical domains (decision lives with domain understanding, writer stays
mechanical, honours `[[feedback_agents_dont_validate_inputs]]` — judgement belongs upstream, not in
the agent body), with **A**'s concept-based wording as the fallback anchor if expressing isolation in
the AC format proves too heavy. **C** alone is insufficient — it leaves the new-behaviour case open,
which is the case that exposed the gap.

## Open questions for the human

1. Should the agent ever decide isolation, or is it strictly an upstream (ticket/refiner) property?
2. If upstream: where is isolation expressed — a Gherkin tag, a ticket field, a marker the refiner
   emits? Does the ticket schema / `acceptance-criteria-refiner` need to change?
3. Is the dangerous failure mode (silent under-isolation → flake) acceptable to mitigate by
   convention, or does it warrant a stronger check (e.g. a lint that flags a plain test touching a
   known-global builder)?

## Not in this plan

- The run-side fold (covered by `plans/20260606-1356-run-isolated-acceptance-suites.md`).
- Any actual prompt/schema edit — pending the decision above.
