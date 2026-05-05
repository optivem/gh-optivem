# Tracer-bullet test selection after a driver-adapter change

## Problem

The current `verify_run_tests_after_driver` action (`internal/atdd/runtime/actions/bindings.go:602`) calls `testselect.Select`, which returns the **affected set** — every test that traverses any changed adapter method. That set is regression-safe and minimal in the formal sense, but in practice it is too large to use as the iteration-time gate.

A worked example: after an agent edits a single Page Object helper `NewOrderPage.inputSku`, the affected set is **103 tests** (50 acceptance-api + 50 acceptance-ui + 3 contract-stub). Reasons:

- `inputSku` is a Page Object helper. The selector bridges through `MyShopUiDriver.placeOrder` (the only adapter caller, and itself port-backed). So far, fan-out is 1.
- Every DSL helper that wraps `placeOrder` is pulled in.
- Every higher-level DSL helper that calls those wrappers is pulled in (transitive closure).
- Almost every acceptance test calls one of those DSL helpers, since "place an order" is the dominant arrange step.
- Tests carry no `@Channel` annotations, so `tagSuites` returns empty and `fallbackSuitesForLayer("shop")` enrolls every test in **both** `acceptance-api` and `acceptance-ui`. The 50 distinct tests become 100.

For the inner WRITE-phase loop (after each adapter edit, before commit), 103 tests is too slow to be the default. Students skip it; the feedback loop is long enough that the agent's output goes stale before a failure is observed — exactly the symptom the original plan named.

## Key insight

The WRITE-phase question is **"did I break the layering I just edited?"** — not **"is the world still correct?"**. Those are different questions and deserve different gates.

- Inside a WRITE phase, after every adapter touch, the cheap question is: does *one* test that traverses the full Test → DSL → Port → Adapter chain for the changed method still pass? If yes, the structural edit is at least non-broken. If no, the agent broke something obvious and should iterate now.
- Before commit (or at PR time), the expensive question is: does the full affected set still pass, including every negative-path test? That's where regression safety lives.

The `Select` (affected-set) function already answers question 2. We need a second function `SelectTracer` that answers question 1, and we wire both into the prompt so the user picks based on context.

## Why this is not a regression of the original plan

The original plan (`plans/20260504-130000-minimal-test-set-after-driver-adapter-change.md`) explicitly rejected "one per method" framing on the grounds that the affected set is simultaneously regression-safe AND covers traversal. That argument is correct *for the affected-set use case* (commit-time gate). The tracer-bullet is not a replacement — it is an additional, cheaper signal for the iteration-time gate. Both exist; the user picks.

## Approach

### New entry point

Add `SelectTracer(repoRoot, baseRef string) (TracerResult, error)` to `internal/atdd/runtime/testselect/`. New file `tracer.go` next to `testselect.go`.

```go
type TracerSelection struct {
    Suite       string // single suite — never both
    Test        string // class-qualified test name
    DSLMethod   string // which DSL method bridged the test to the adapter
    PortMethod  string // resolved port method (post-bridge)
    AdapterFile string // changed file
    AdapterMethod string // changed method
    Stage       string // "when" | "given" | "then" — which DSL form was picked
}

type TracerResult struct {
    Selections  []TracerSelection // one per changed adapter method per channel
    Unmapped    []ChangedMethod   // no tracer found — caller should fall back
    Changed     []ChangedMethod
    Diagnostics []string
}
```

### Pick rule (deterministic, no annotation required)

For each changed adapter method:

1. **Bridge to port** — reuse `resolveAdapterToPortBackedMethods`. No change.
2. **Channel from adapter path** — derive the channel from the changed file's path:
   - `/myShop/ui/` (or `/MyShop/Ui/` etc., case-insensitive) → `acceptance-ui`
   - `/myShop/api/` → `acceptance-api`
   - under `external/` → `contract-stub`
   - otherwise → `unmapped` (fall back to `Select` mode for this method)
3. **Pick DSL helper** — for each port-backed bridged method, gather DSL methods that call it (reuse `callersOf`). Rank by file-path stage:
   - Stage `when` (path contains `/when/` or class name `When*`) preferred.
   - Stage `given` next.
   - Stage `then` last (a `then` helper that places an order to set up an assertion is technically a caller but a poor tracer).
   - Within a stage, pick alphabetically first by `(file, method)`.
4. **Pick test** — among tests that call the picked DSL method (reuse `callersOfTest`), pick alphabetically first by `(class, method)`. Filter out `@time-dependent` and any test in a `@isolated`-only group, since those are deselected by suite filters.
5. **Suite tag** — the suite is the channel from step 2. No `@Channel` lookup, no fallback fan-out.

### Unmapped handling

If any changed method has no tracer (no port bridge, no DSL caller, no test caller, or ambiguous channel), it goes into `TracerResult.Unmapped`. The action handles unmapped the same way it does today: warn and fall back to full suite for safety.

### Action wiring

`bindings.go:verifyRunTestsAfterDriver` extends the prompt:

```
Choose: [t]racer (default), [r]un all selected, [a]pprove, [x]reject, [f]ull suite:
```

- `t` (or empty) — call `SelectTracer`. Print one line per selection (chain visible). Run those tests via `gh optivem test system --suite <s> --test <t>`. If `Unmapped` is non-empty, warn and fall back to full suite.
- `r` — current behavior (`Select`, run affected set).
- `a`, `x`, `f` — unchanged.

The empty-input default changes from "approve without running" to "run tracer". This matches the new mental model: the cheap default does *something*, not nothing.

### Output shape

Tracer mode prints the chain so the user can see what it picked:

```
inputSku (system-test/typescript/.../NewOrderPage.ts)
  → MyShopUiDriver.placeOrder
  → MyShopDriver.placeOrder (port)
  → WhenPlaceOrder.placeOrder (DSL, when)
  → PlaceOrderPositiveTest.shouldBeAbleToPlaceOrderForValidInput (acceptance-ui)
```

Verbose mode (`OPTIVEM_VERIFY_VERBOSE=1`) additionally lists the DSL helpers that *would* have been picked at each stage but lost the tie-break, so a student can see why this particular test was selected.

## Scope

- New file `internal/atdd/runtime/testselect/tracer.go` — `SelectTracer`, the channel-from-path inference, the stage-ranked DSL pick, and the test tie-break.
- Edit `internal/atdd/runtime/actions/bindings.go` — extend the prompt, add `case "t"` and `case ""` branches, add `printTracerSummary` analogous to `printVerifySummary`.
- New tests in `internal/atdd/runtime/testselect/tracer_test.go` covering:
  - WHEN-preferred pick when both WHEN and THEN call the port.
  - GIVEN/THEN fallback when no WHEN exists.
  - Alphabetical tie-break within a stage.
  - Channel inference from `/ui/`, `/api/`, `external/` segments.
  - Unmapped path (ambiguous channel, no DSL caller).
- New test in `internal/atdd/runtime/actions/bindings_test.go` for the extended prompt:
  - Empty input runs tracer, not approve.
  - `t` runs tracer.
  - `r` still runs full affected set.
  - Tracer with unmapped falls back to full suite with a warning.
- No layout config changes. Path inference uses existing string segments; stage ranking uses path matching that's data, not code.

## Out of scope

- Adding `@Channel` annotations to existing tests. That's a separate improvement that would shrink `Select`'s output by ~50%. Tracer-bullet is independent of it.
- A `@Tracer` annotation for explicit tracer designation. The deterministic pick rule above is sufficient for v1; we can add an opt-in annotation later if students complain about the auto-pick.
- Changing the commit-time / PR-time gate. That continues to use the affected set.

## Open questions

- **Default choice**: this plan makes `[t]` the default (replacing `[a]pprove without running` as the empty-input fallback). Confirm this is acceptable. If students rely on "press enter to skip", we either keep `[a]` as default or print a one-time warning the first time tracer fires.
- **External-layer tracer**: an `external/` adapter change picks `contract-stub` and runs one contract test. The suite tagging works but the WHEN/GIVEN/THEN stage ranking may not — contract tests don't necessarily route through scenario-stage DSL helpers. If the rank gives no result for external/, we fall back to first-alphabetical contract test that calls the port.
- **Bridge ambiguity**: when `inputSku` bridges to multiple port methods (e.g. a Page Object helper used by both `placeOrder` and `cancelOrder`), should the tracer run **one test per bridged port method** (current plan) or pick a single representative across the set? Current plan: one per port method, since that's the minimal traversal proof for the bridge.

## Done when

- `SelectTracer` returns a TracerResult with one selection per (changed adapter method, channel) for the worked `inputSku` example, and that selection's test passes.
- The verify prompt includes `[t]racer` as the default option.
- Tests in `tracer_test.go` and `bindings_test.go` pass.
- Running `bash scripts/atdd-rehearsal.sh 61` against the shop worktree runs **one** test in `acceptance-ui` rather than 103, and that one test traverses the full `inputSku → ... → placeOrder` chain.
