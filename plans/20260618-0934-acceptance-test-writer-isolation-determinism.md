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

**Items A, B, C are done; only the deferred clock-DSL gate remains.** `1.1.9` is live on all three registries (A). The writer prompt/marker chunks (B) and the shop migration + dep bump (C) are edited in the working trees of gh-optivem and shop respectively — **pending the commit gate** (two commits: gh-optivem, shop). The one open design item is **B-gate** (the clock-control-DSL gate), deferred to its own plan because gh-optivem has no AC-content-validation primitive and the DSL is shop-side — the next executable move there is `/create-plan`, not a mechanical edit. Verification (rehearse #76 twice for identical isolation) can run once the commits land.

## Problem

Two observations from rehearsal #76 (bug: "Order cancellation blackout on Dec 31 ends at 22:30 instead of 23:00"), same ticket run twice:

1. **acceptance-test-writer non-determinism.** One run emitted a non-isolated, single-method test (`cannotCancelAnOrderAt2245OnDecember31st`). The other emitted an `@Isolated` class **plus an invented, unreported second parameterized test** (`cannotCancelAnOrderOn31stDecBetween2200And2230`, with 5 fabricated `@DataSource` rows the single-scenario AC never asked for). The writer prompt states isolation is "mechanical mirroring of the tag the refiner/author already set — *not* a judgement", yet the AC carries no `@isolated` tag — so the writer has nothing to mirror and is improvising.

2. **Corpus-authoring gap.** A clock-mutating scenario *must* be isolated to be parallel-safe (parallel clock mutation is flaky). Today nothing guarantees an isolation signal is present on such scenarios; it's left to the writer to infer.

These are a teaching-repo concern (the generated tests are what students read), so determinism and honesty matter beyond just green/red.

## Resolved design

1. **Isolation signal is a bare `@isolated` Gherkin tag on the AC.** The ticket author sets it; the writer mechanically mirrors it. This is the deterministic upstream signal that was missing. Gherkin tags are whitespace-delimited bare identifiers, so the tag itself **cannot** carry a free-text reason — `@isolated` only.

2. **Code annotation carries the reason POSITIONALLY: `@Isolated("…")` / `[Isolated("…")]`, reason OPTIONAL.** Free-text string, because the full set of isolation reasons can't be enumerated up front. The writer **lifts the reason verbatim** from an adjacent Gherkin comment / scenario-description line when present, and emits the bare form (`@Isolated()` / `[Isolated]`) when absent. The writer never invents a reason. Consequence to accept: with an optional reason, "every isolated test states its why" is a *convention enforced at review/lint*, not a compile guarantee.

   ```gherkin
   @isolated
   # isolated: mutates the cancellation-blackout clock; parallel runs would be flaky
   Scenario: Cannot cancel an order at 22:45 on December 31st
   ```
   → `@Isolated("mutates the cancellation-blackout clock; parallel runs would be flaky")`

   **NB — the reason is positional, NOT a named `reason =` param.** Verified against released `1.1.9`: Java `Isolated.java` declares `String value() default ""` (so `@Isolated(reason = …)` would not compile — use `@Isolated("…")`); .NET `IsolatedAttribute` has an `IsolatedAttribute(string reason)` ctor + nullable `Reason` property (`[Isolated("…")]`). Per-language placement differs (matches the shop reference): **Java** puts `@Isolated("reason")` at **class** level; **.NET** keeps `[Collection("Isolated")]` + `[Trait("Category","isolated")]` at class level (the optivem `[Isolated]` only tags the trait — `[Collection]` is what serialises isolated classes) and puts the reason-bearing `[Isolated("reason")]` at **method** level. TypeScript has **no** isolation annotation (channel/serial-`describe` model) — nothing to add.

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

### A. `../optivem-testing` → patch release (`1.1.9`) — ✅ DONE
Time-marker deletes + doc sweep + version bump committed; `1.1.9` published to **Maven Central, NuGet, and npm**; git tag `v1.1.9` + GitHub Release created (release run `27752979718`, after an `NPM_TOKEN` refresh + `rerun --failed`). Nothing left here.

### B. gh-optivem (writer + docs) — ✅ DONE (gate deferred, see below)
- Writer prompt + per-language marker chunks updated to mirror the bare `@isolated` tag into the positional `@Isolated("…")` / `[Isolated("…")]` shape, lift the reason verbatim from an adjacent AC comment, bare form when absent, never invent. .NET chunk reconciled to the shop reference (`[Collection("Isolated")]`+`[Trait]` at class, `[Isolated("reason")]` at method) + a `clauderun_test.go` assertion pinning `[Collection("Isolated")]`. (`internal/atdd/assets/runtime/shared/isolated-marker-{java,csharp}.md`, `acceptance-test-writer.md`, `clauderun.go`, `clauderun_test.go`.)
- **Sweep / dep-bump were no-ops:** `@TimeDependent`/`[Time]` were already retired in commit `06ec40b` (grep is clean); gh-optivem has **no** optivem-testing version pin to bump — the scaffold copies build files verbatim from the shop repo (`cfg.ShopPath`), so the `1.1.9` pin lives shop-side (Item C, done).

### B-gate. Clock-control-DSL gate — ⏳ DEFERRED to its own plan
The design (§4) assumed a gate keyed on "the clock-control DSL mechanism," but that DSL lives in the **shop** repo, not gh-optivem, and gh-optivem has **no AC-content-validation primitive** to extend (the process-flow gateways key on ticket kind/subtype, never on scenario step text). Building it would mean inventing both a new content-validation gate location and an authoritative list of clock-DSL step identifiers — pure inference, which repo policy forbids. The core determinism goal is already met without it (the `@isolated` tag is the deterministic signal; clock-mutating scenarios are tagged in shop). Spin a fresh plan to design where this safety-net gate lives (likely a shop-side contract test / lint over the DSL) and its step-identifier source.

### C. optivem/shop (consumer) — ✅ DONE
- optivem-testing bumped to `1.1.9` across Java (`build.gradle`), .NET (4 `.csproj`), TypeScript (`package.json`+lockfile).
- Every `@TimeDependent`/`[Time]`/`@time-dependent` usage migrated to `@Isolated("reason")` / `[Isolated("reason")]` (+ TS title-suffix removed); reasons written in plain English (no ISO-timestamp carryover). Covers #76's clock-mutating cancellation pair. (No `.feature` files exist — ACs are encoded in DSL test classes, so the isolation signal sits at the test-class level.)
- **#80** (`cannotPlaceOrderWithExpiredCoupon`) determined **clock-mutating** (drives `.clock().withTime(...)` at runtime, not merely date-seeded) → correctly isolated.

## Verification

- Rehearse #76 twice (`atdd-rehearsal.sh 76 …`) → identical isolation outcome both runs (the original non-determinism repro is gone).
- Confirm the clock-control-DSL gate rejects a clock-driving AC that lacks `@isolated`.
- Shop builds green against the new library release.

## Out of scope

- The per-channel verify / partition-naming mechanics — already shipped (0653). This plan does not touch `testselect`, the unroll, or `tests.yaml`.
- Any change to *what* the isolated partition executes — only *how the decision to isolate a test is made and carried*.
- Constraining the writer's test-count invention (fabricated extra parameterized test / `@DataSource` rows) — real, but a separate concern; spin into its own plan.
