# Make acceptance-test isolation deterministic, not writer-inferred

> Spun out of `plans/20260617-1651-per-channel-verify-covers-isolated-suite.md`
> (now deleted). 1651's framework fix — making the per-channel verify cover the
> isolated partition — shipped via `plans/20260618-0653-symmetric-acceptance-partition-naming.md`
> (group-promotion of `acceptance-<ch>`). This plan carries forward the *separate,
> fuzzier* concern 1651 explicitly carved out and never resolved.

## TL;DR

**Why:** Whether an acceptance test lands in the isolated partition currently depends on the acceptance-test-writer *inferring* isolation, not on a deterministic signal carried by the ticket/AC. Across two runs of the identical bug ticket #76 the writer produced materially different tests — and only one of them isolated the clock-mutating scenario. A clock-mutating test that runs in parallel is flaky by construction, so "did the writer remember to isolate it" is a correctness coin-flip.
**End result:** Isolation is decided deterministically upstream of the writer — a bare `@isolated` tag on the Gherkin AC — and the writer mechanically mirrors it into an `@Isolated(reason = …)` code annotation. `@TimeDependent` is retired across all languages; the clock-mutation correctness invariant is enforced off the clock-control DSL, not off any annotation. So the same ticket always produces the same isolation outcome, and clock-mutating scenarios are always parallel-safe.

## ▶ Next executable step (resume here)

Design is **resolved** (see Resolved design). This is now a coordinated three-repo change with a library release in the middle — run `/execute-plan` (ideally after `/clear`) and walk the Items **in dependency order**: optivem-testing library release first, then gh-optivem writer/gate, then optivem/shop consumer. No pickup marker yet; check for concurrent agents before starting.

## Problem

Two observations from rehearsal #76 (bug: "Order cancellation blackout on Dec 31 ends at 22:30 instead of 23:00"), same ticket run twice:

1. **acceptance-test-writer non-determinism.** One run emitted a non-isolated, single-method test (`cannotCancelAnOrderAt2245OnDecember31st`). The other emitted an `@Isolated` class **plus an invented, unreported second parameterized test** (`cannotCancelAnOrderOn31stDecBetween2200And2230`, with 5 fabricated `@DataSource` rows the single-scenario AC never asked for). The writer prompt states isolation is "mechanical mirroring of the tag the refiner/author already set — *not* a judgement", yet the AC carries no `@isolated` tag — so the writer has nothing to mirror and is improvising.

2. **Corpus-authoring gap.** A clock-mutating scenario *must* be isolated to be parallel-safe (parallel clock mutation is flaky). Today nothing guarantees an isolation signal is present on such scenarios; it's left to the writer to infer.

These are a teaching-repo concern (the generated tests are what students read), so determinism and honesty matter beyond just green/red.

## Resolved design

1. **Isolation signal is a bare `@isolated` Gherkin tag on the AC.** The ticket author sets it; the writer mechanically mirrors it. This is the deterministic upstream signal that was missing. Gherkin tags are whitespace-delimited bare identifiers, so the tag itself **cannot** carry a free-text reason — `@isolated` only.

2. **Code annotation carries the reason: `@Isolated(reason = "…")`, reason OPTIONAL.** Free-text string, because the full set of isolation reasons can't be enumerated up front. The writer **lifts the reason verbatim** from an adjacent Gherkin comment / scenario-description line when present, and emits bare `@Isolated()` when absent. The writer never invents a reason. Consequence to accept: with an optional reason, "every isolated test states its why" is a *convention enforced at review/lint*, not a compile guarantee.

   ```gherkin
   @isolated
   # isolated: mutates the cancellation-blackout clock; parallel runs would be flaky
   Scenario: Cannot cancel an order at 22:45 on December 31st
   ```
   → `@Isolated(reason = "mutates the cancellation-blackout clock; parallel runs would be flaky")`

3. **`@TimeDependent` is retired in all languages (Java, .NET, TypeScript).** It's subsumed: the reason now lives in the `@Isolated` string, and the isolation decision lives in the bare tag. Removing it is a breaking change → **major version bump** of the optivem-testing library. **Pre-removal check (blocker):** confirm `@TimeDependent` is a pure marker, not the mechanism that *activates* the controlled/fake clock. If it activates the clock, repurpose rather than remove. The plan assumes clock control is driven by explicit DSL steps (per #76's "reusing the existing clock-control DSL steps"); verify before deleting.

4. **The clock correctness invariant is enforced by a gate keyed on the clock-control DSL mechanism** — not on the reason prose and not on `@TimeDependent`. A scenario that drives the clock must carry `@isolated`, else reject. This keeps the one known correctness invariant un-forgettable without a closed reason enum.

5. **Writer invention (the fabricated extra test + 5 `@DataSource` rows) is a SEPARATE concern, not solved here.** This plan makes the isolation *decision* deterministic; it does not constrain *how many* tests the writer emits. That needs an "emit exactly the scenarios the AC enumerates" constraint — left open, to be spun into its own plan.

## Items

Walk in dependency order — each repo depends on the one above it being released/landed.

### A. optivem-testing library (all languages) → new major release
- [ ] Add an optional `reason` parameter to `@Isolated` in each language binding (Java, .NET, TypeScript).
- [ ] Confirm `@TimeDependent` is a marker (not clock activation), then remove it in each language binding.
- [ ] Major version bump + release per the library's release process.

### B. gh-optivem (writer + gate + docs)
- [ ] acceptance-test-writer prompt: mirror the bare `@isolated` AC tag into `@Isolated(reason = …)`, lifting the reason verbatim from an adjacent AC comment/description when present, bare `@Isolated()` otherwise; never invent. Remove all expectation/emission of `@TimeDependent`.
- [ ] Add the clock-control-DSL gate: a scenario that drives the clock must carry `@isolated`.
- [ ] Sweep process docs / runtime prompts for `@TimeDependent` references and retire them.
- [ ] Bump the scaffolded optivem-testing dependency to the new release.

### C. optivem/shop (consumer)
- [ ] Bump the optivem-testing dependency to the new release.
- [ ] Retag #76's AC: replace `@TimeDependent` with bare `@isolated` (+ optional reason comment). Re-check #80 (coupon `validTo`) — likely date-seeded, not clock-mutating, so probably no isolation needed; confirm.
- [ ] Update any committed tests using `@TimeDependent` → `@Isolated(reason = …)`.

## Verification

- Rehearse #76 twice (`atdd-rehearsal.sh 76 …`) → identical isolation outcome both runs (the original non-determinism repro is gone).
- Confirm the clock-control-DSL gate rejects a clock-driving AC that lacks `@isolated`.
- Shop builds green against the new library release.

## Out of scope

- The per-channel verify / partition-naming mechanics — already shipped (0653). This plan does not touch `testselect`, the unroll, or `tests.yaml`.
- Any change to *what* the isolated partition executes — only *how the decision to isolate a test is made and carried*.
- Constraining the writer's test-count invention (fabricated extra parameterized test / `@DataSource` rows) — real, but a separate concern; spin into its own plan.
