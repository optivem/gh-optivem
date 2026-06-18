# Gate: a clock-driving scenario must carry `@isolated`

> **DRAFT — awaiting go/no-go.** Spun out of the **B-gate** section of
> `plans/20260618-0934-acceptance-test-writer-isolation-determinism.md` (Items A/B/C of
> that plan are done; this is the one piece deliberately deferred because it couldn't be
> built without inventing a mechanism). Do **not** execute until the decision below is made.

## TL;DR

**Why:** The isolation-determinism work made the `@isolated` tag the single source of truth — but nothing *enforces* that a clock-mutating scenario actually carries it. If an author writes a new test that drives the clock and forgets the tag, that test silently runs in parallel and becomes flaky again — re-opening the exact bug (#76) the parent plan closed. The guard is currently "remember the tag," enforced only by review.
**End result (if approved):** An automated check fails the build when a test drives the clock-control DSL but lacks its isolation marker. Forgetting the tag becomes impossible, not just discouraged.

## ▶ Decision needed (resume here)

This plan is **not yet approved to execute.** Before any work, decide **two** things:

1. **Build it at all?** The parent plan's determinism win stands without this gate; today's clock-mutating tests are all correctly isolated. This gate only guards against a *future* author's mistake. Options: **build it (recommended once a real near-miss occurs)** / **leave as a review convention**.
2. **If building — where does the check live?** See "Options for where the gate lives" below. Recommendation: **Option A (shop-side static check)**.

Once both are answered, this draft can be promoted to `plans/` (flat) and executed.

## Problem

The parent plan established: author puts a bare `@isolated` Gherkin/scenario tag → the writer mechanically mirrors it into `@Isolated("reason")` / `[Isolated("reason")]`. Deterministic, good.

But the *correctness invariant* — "a scenario that mutates the wall clock **must** be isolated, or parallel runs clash on the clock and flake" — is only upheld if the author remembers to tag. There is no automated enforcement. The parent plan's design §4 wanted a gate "keyed on the clock-control DSL mechanism," but it could not be built in-place because:

- **The clock-control DSL lives in the `shop` repo**, not gh-optivem (e.g. `.given().clock().withTime("…")` Java / `.Given().Clock().WithTime("…")` .NET / the TS clock step). gh-optivem has zero clock-step identifiers.
- **gh-optivem has no AC-content-validation primitive.** Its process-flow gateways branch on *ticket kind/subtype*, never on a scenario's step text — so there is nothing to extend that "reads the steps and rejects."
- **`shop` has no `.feature` files** — acceptance criteria are encoded directly in DSL test classes (Java/.NET) and Playwright specs (TS). So "the scenario" in shop *is* the test class; any check operates on test source, not Gherkin.

## What "drives the clock" means (the identifier the check keys on)

The authoritative signal is a call into the clock-control DSL entrypoint:

- **Java:** `.clock().withTime(` (via the `scenario` DSL) — and any further clock-mutating step off `.clock()`.
- **.NET:** `.Clock().WithTime(` .
- **TypeScript:** the clock step used in the Playwright specs (confirm exact name during execution).

The isolation marker the check requires when the above is present:

- **Java:** class-level `@Isolated(` (the optivem annotation; carries `@Tag("isolated")`).
- **.NET:** class-level `[Collection("Isolated")]` **and** method/class `[Isolated(` (the `[Collection]` is what serialises; see parent §2).
- **TypeScript:** the `test.describe('@isolated', …)` serial wrapper.

(These mirror exactly what the migrated shop tests already carry — the check is "make the existing shape mandatory whenever the clock is driven.")

## Options for where the gate lives

### Option A — shop-side static check (lint / contract test) — RECOMMENDED
A test (or lint rule) in `shop` that scans the acceptance-test sources; for every test that calls the clock-control DSL, assert it also carries the language's isolation marker, else fail.
- **Pros:** lives next to the clock DSL and the actual test files it checks; pure static analysis (no runtime); enforces on the *real committed artifact*; one check per language, self-contained in shop's existing test build.
- **Cons:** fires at shop-test-time (after the writer generated the test) rather than at AC-authoring time — acceptable, because in the ATDD flow the generated tests *are* the shop artifact, and shop CI is where they first compile/run.

### Option B — gh-optivem writer/refiner-time gate
Check the AC at generation time: if its steps drive the clock, require `@isolated` before the writer runs, else reject.
- **Pros:** catches it earliest, before a test is even generated.
- **Cons:** requires inventing a new AC-content-validation primitive in gh-optivem **and** teaching gh-optivem the shop-side clock-step vocabulary — duplicating shop knowledge into the generator. Higher cost, more coupling; rejected unless Option A proves insufficient.

### Option C — no gate (status quo)
Keep "remember the tag" as a review/lint-by-eyeball convention.
- **Pros:** zero work.
- **Cons:** the invariant remains forgettable; a single missed tag silently reintroduces flakiness.

## Items (only if Option A is chosen — do NOT start until the decision gate above is resolved)

### Shop: clock-driving-implies-isolated check
- [ ] Confirm the exact clock-control DSL entrypoint identifier per language (Java/.NET/TS) against the current shop DSL.
- [ ] Add a static check (one per language, in the existing system-test build) that fails when a test source calls the clock DSL but lacks the isolation marker. Keep it simple: source-text/AST scan over `system-test/**/acceptance/**`.
- [ ] Add a deliberately-untagged fixture (or a unit test of the check) proving it rejects a clock-driving test without the marker, and passes one with it.
- [ ] Wire it into shop CI so the build fails on violation.

### gh-optivem (only if the team also wants generation-time prevention — likely a separate decision)
- [ ] Reconsider Option B only if Option A's after-the-fact failure proves too late in practice.

## Out of scope
- Re-deriving the `@isolated` → `@Isolated("reason")` mirror (done in the parent plan).
- The writer's test-count invention (fabricated extra parameterized test / `@DataSource` rows) — a *separate* deferred concern noted in the parent plan's §5, not this gate.
