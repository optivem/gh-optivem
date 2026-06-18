# Make acceptance-test isolation deterministic, not writer-inferred

> Spun out of `plans/20260617-1651-per-channel-verify-covers-isolated-suite.md`
> (now deleted). 1651's framework fix — making the per-channel verify cover the
> isolated partition — shipped via `plans/20260618-0653-symmetric-acceptance-partition-naming.md`
> (group-promotion of `acceptance-<ch>`). This plan carries forward the *separate,
> fuzzier* concern 1651 explicitly carved out and never resolved.

## TL;DR

**Why:** Whether an acceptance test lands in the isolated partition currently depends on the acceptance-test-writer *inferring* isolation, not on a deterministic signal carried by the ticket/AC. Across two runs of the identical bug ticket #76 the writer produced materially different tests — and only one of them isolated the clock-mutating scenario. A clock-mutating `@TimeDependent` test that runs in parallel is flaky by construction, so "did the writer remember to isolate it" is a correctness coin-flip.
**End result:** Isolation is decided deterministically upstream of the writer (the ticket/refiner carries the tag), and the writer mechanically mirrors it — so the same ticket always produces the same isolation outcome, and clock-mutating scenarios are always parallel-safe.

## ▶ Next executable step (resume here)

This is a **design plan, not a mechanical edit** — the core is an unresolved policy decision (see Open questions). Next move is `/refine-plan` on this file to resolve the open questions, then expand into concrete items. No pickup marker; do not start editing prompts/process docs until the policy is decided.

## Problem

Two observations from rehearsal #76 (bug: "Order cancellation blackout on Dec 31 ends at 22:30 instead of 23:00"), same ticket run twice:

1. **acceptance-test-writer non-determinism.** One run emitted a non-isolated, single-method test (`cannotCancelAnOrderAt2245OnDecember31st`). The other emitted an `@Isolated` class **plus an invented, unreported second parameterized test** (`cannotCancelAnOrderOn31stDecBetween2200And2230`, with 5 fabricated `@DataSource` rows the single-scenario AC never asked for). The writer prompt states isolation is "mechanical mirroring of the tag the refiner/author already set — *not* a judgement", yet the AC carries no `@isolated` tag — so the writer has nothing to mirror and is improvising.

2. **Corpus-authoring gap.** A clock-mutating `@TimeDependent` scenario *must* be isolated to be parallel-safe (parallel clock mutation is flaky). Today nothing guarantees the `@isolated` tag is present on such scenarios; it's left to the writer to infer.

These are a teaching-repo concern (the generated tests are what students read), so determinism and honesty matter beyond just green/red.

## Open questions (resolve via /refine-plan before items)

1. **Should `@TimeDependent ⇒ @isolated` become an explicit, enforced rule?** If a scenario is tagged `@TimeDependent` (clock-mutating), should the refiner/ticket pipeline *automatically* carry `@isolated`, rather than leaving the writer to infer it? (Recommendation: yes — it's a correctness invariant, not a style choice.)
   - Where is it enforced — at refinement (tag added to the AC), at ticket parsing, or as a validation gate that rejects a `@TimeDependent` AC lacking `@isolated`?
2. **How to constrain the writer's non-determinism / invention?** The writer fabricated an extra parameterized test and 5 `@DataSource` rows the AC never specified. Is the fix prompt-level (tighten "mirror, don't invent"), structural (the writer must emit exactly the scenarios the AC enumerates), or a downstream check that flags tests with no corresponding AC scenario?
3. **Scope of the isolation signal.** Does `@isolated` belong only on `@TimeDependent` scenarios, or are there other must-isolate categories (shared mutable external state, ordering-sensitive)? Pin the closed set before encoding a rule.

## Out of scope

- The per-channel verify / partition-naming mechanics — already shipped (0653). This plan does not touch `testselect`, the unroll, or `tests.yaml`.
- Any change to *what* the isolated partition executes — only *how the decision to isolate a test is made and carried*.
