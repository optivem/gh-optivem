# Make acceptance-test isolation deterministic, not writer-inferred

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-06-18T09:18:56Z`

> Spun out of `plans/20260617-1651-per-channel-verify-covers-isolated-suite.md`
> (now deleted). 1651's framework fix — making the per-channel verify cover the
> isolated partition — shipped via `plans/20260618-0653-symmetric-acceptance-partition-naming.md`
> (group-promotion of `acceptance-<ch>`). This plan carries forward the *separate,
> fuzzier* concern 1651 explicitly carved out and never resolved.

## TL;DR

**Why:** Whether an acceptance test lands in the isolated partition currently depends on the acceptance-test-writer *inferring* isolation, not on a deterministic signal carried by the ticket/AC. Across two runs of the identical bug ticket #76 the writer produced materially different tests — and only one of them isolated the clock-mutating scenario. A clock-mutating test that runs in parallel is flaky by construction, so "did the writer remember to isolate it" is a correctness coin-flip.
**End result:** Isolation is decided deterministically upstream of the writer — a bare `@isolated` tag on the Gherkin AC — and the writer mechanically mirrors it into an `@Isolated(reason = …)` code annotation. `@TimeDependent` is retired across all languages; the clock-mutation correctness invariant is enforced off the clock-control DSL, not off any annotation. So the same ticket always produces the same isolation outcome, and clock-mutating scenarios are always parallel-safe.

## ▶ Next executable step (resume here)

**Item A's edits are done** (time markers deleted + docs swept, committed to optivem-testing). The next unit is the **gated publish of `optivem-testing 1.1.9`** (PATCH — VJ exception): from `../optivem-testing`, `echo "1.1.9" > VERSION && bash scripts/bump-version.sh`, then commit + push `main` and `gh workflow run release-stage.yml`. This is outward-facing and irreversible → **requires user approval before running**. Once `1.1.9` is live on Maven Central / NuGet / npm, run B (gh-optivem writer/gate/docs + dep bump to `1.1.9`) and C (optivem/shop dep bump + migrate `@TimeDependent`/`[Time]` usages, incl. #76; re-check #80). B and C can't be verified to build green until the release lands.

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

   **Already in the library — no add needed.** Java `Isolated.java` has `String value() default ""`; .NET `IsolatedAttribute` has an `IsolatedAttribute(string reason)` ctor + `Reason` property. So `@Isolated("reason")` / `[Isolated("reason")]` already compile today. TypeScript has **no** isolation annotation concept at all (its test model is channel-based) — nothing to add there.

3. **The time-marker annotations are retired.** They're subsumed: the reason now lives in the `@Isolated` string, the isolation decision lives in the bare tag. Per-language names differ:
   - Java: `@TimeDependent` (`TimeDependent.java`) **and** its already-`@Deprecated(forRemoval=true)` alias `@Time` (`Time.java`).
   - .NET: `[Time]` (`TimeAttribute.cs`, incl. `TimeTraitDiscoverer`). *(There is no `[TimeDependent]` in .NET.)*
   - TypeScript: none exists — nothing to remove.

   **Pre-removal check — RESOLVED (not a blocker).** All of these are pure marker meta-annotations: Java `@TimeDependent` = `@Tag("time-dependent") @Tag("time") @Isolated("Time-dependent test")`; .NET `[Time]` = a `TraitDiscoverer` emitting `time`+`isolated` categories. None *activates* the clock — clock control is driven by explicit DSL steps (per #76). Safe to delete. Note: `@TimeDependent`'s ISO `value()` is documentation-only ("potential future automation"); migrating drops it, which is fine (the actual instant is set via DSL steps).

   **Version: `1.1.8` → `1.1.9` (PATCH) — deliberate exception, set by VJ 2026-06-18.** Removing the public marker types is technically breaking and would default to a major bump (`2.0.0`); the breaking-change risk was flagged and VJ chose a patch bump anyway. All dependency bumps below pin `1.1.9`.

4. **The clock correctness invariant is enforced by a gate keyed on the clock-control DSL mechanism** — not on the reason prose and not on `@TimeDependent`. A scenario that drives the clock must carry `@isolated`, else reject. This keeps the one known correctness invariant un-forgettable without a closed reason enum.

5. **Writer invention (the fabricated extra test + 5 `@DataSource` rows) is a SEPARATE concern, not solved here.** This plan makes the isolation *decision* deterministic; it does not constrain *how many* tests the writer emits. That needs an "emit exactly the scenarios the AC enumerates" constraint — left open, to be spun into its own plan.

## Items

Walk in dependency order — each repo depends on the one above it being released/landed. All paths are relative to the academy workspace root (`../` from gh-optivem).

### A. `../optivem-testing` → patch release (`1.1.9`)
The time-marker deletes + doc sweep are **done** (committed to optivem-testing). What remains is the gated publish.
- [ ] Release (PATCH — deliberate exception, see §3): `echo "1.1.9" > ../optivem-testing/VERSION && bash ../optivem-testing/scripts/bump-version.sh` → commit + push `main` (triggers RC commit stages) → `gh workflow run release-stage.yml` (promotes RC to Maven Central `com.optivem:optivem-testing`, NuGet `Optivem.Testing`, npm `@optivem/optivem-testing`; tags + GitHub Release). See `../optivem-testing/CONTRIBUTING.md` "Release Checklist". **Outward-facing/irreversible → user-gated.**

### B. gh-optivem (writer + gate + docs) — after A is published
- [ ] acceptance-test-writer prompt (`internal/atdd/assets/runtime/agents/atdd/*.md` — the AT-write prompt): mirror the bare `@isolated` AC tag into `@Isolated(reason = …)`, lifting the reason verbatim from an adjacent AC comment/description when present, bare `@Isolated()` otherwise; never invent. Remove all expectation/emission of `@TimeDependent` / `[Time]`.
- [ ] Add the clock-control-DSL gate: a scenario that drives the clock-control DSL steps must carry `@isolated`, else reject.
- [ ] Any code that reads the `@Isolated` reason (writer mirror logic, lint) must treat absence idiom-proof: the reason defaults to `null` in .NET (xUnit-`Skip` idiom) and `""` in Java (JUnit-`@Disabled` idiom) — use `string.IsNullOrEmpty` / `isBlank()`, never an `== null` or `== ""` check alone.
- [ ] Sweep gh-optivem for the retired names: `grep -rinE "TimeDependent|@Time\b|\[Time\]" internal docs` and retire references (prompts, process docs, fixtures).
- [ ] Bump the scaffolded optivem-testing dependency to `1.1.9` wherever the scaffold pins it (Maven/Gradle, NuGet, npm template files).

### C. optivem/shop (consumer) — after A is published
- [ ] Bump the optivem-testing dependency to `1.1.9` in each language config (`build.gradle` / `.csproj` / `package.json`).
- [ ] Find every usage: `grep -rinE "@TimeDependent|@Time\b|\[Time\]" ../shop`. Migrate each to bare `@isolated` Gherkin tag (on the AC) + `@Isolated("reason")` (in the test). This includes #76's clock-mutating cancellation scenario.
- [ ] Re-check #80 (coupon `validTo` in the past) — likely date-seeded, not clock-mutating, so probably needs **no** isolation; confirm before tagging.

## Verification

- Rehearse #76 twice (`atdd-rehearsal.sh 76 …`) → identical isolation outcome both runs (the original non-determinism repro is gone).
- Confirm the clock-control-DSL gate rejects a clock-driving AC that lacks `@isolated`.
- Shop builds green against the new library release.

## Out of scope

- The per-channel verify / partition-naming mechanics — already shipped (0653). This plan does not touch `testselect`, the unroll, or `tests.yaml`.
- Any change to *what* the isolated partition executes — only *how the decision to isolate a test is made and carried*.
- Constraining the writer's test-count invention (fabricated extra parameterized test / `@DataSource` rows) — real, but a separate concern; spin into its own plan.
