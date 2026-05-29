# Fix the WIP-gate snippet: additive annotation, sourced from shared/

**Created:** 2026-05-29 16:11 (local, UTC+2)
**Cross-references:**
- `plans/20260528-1528-replace-disable-enable-with-env-var-gating.md` — the env-var-gating change
  (commit `18ebedc`) that introduced this regression.
- `plans/20260529-1612-archive-references-subsystem.md` — independent follow-up; can land in either
  order. This plan is the urgent production fix and should land first.

## Why this plan exists

A `gift-wrap-an-order` rehearsal (rehearsal-71) failed at the post-implementation acceptance verify
with `java.lang.IllegalStateException: Unknown channel: null`.

Root cause: the `acceptance-test-writer` emitted AT methods declared with a plain `@Test` instead of
the scaffold's mandatory `@TestTemplate` + `@Channel({...})`. The `@Channel` annotation is what feeds
the `channel` value into `BaseConfigurableTest.createMyShopDriverForChannel`; without it the channel
is `null`. The agent did this because the WIP-gate snippet inlined into its prompt
(`${gate-marker-example}`, produced by `renderGateMarkerExample` in
`internal/atdd/runtime/clauderun/clauderun.go`) shows the gate paired with a self-contained
`@Test void shouldXxx() {...}` method, and the agent copied the whole shape.

The gate annotation itself (`@EnabledIfEnvironmentVariable`) is correct and the gate *ran* — the tests
were not skipped. The defect is purely that the snippet doubles as a declaration template and
prescribes the wrong declaration. This regressed at `18ebedc` because gate-application moved into the
writer agent; previously a separate `test-disabler` only prepended `@Disabled` to an already-correct
declaration, so it could never clobber `@TestTemplate`/`@Channel`.

## Decision (resolved with the user 2026-05-29)

Move the snippet content into per-language `shared/` assets (visibility — currently it is a Go string
literal, and a duplicate copy lives in the to-be-archived `references/language-equivalents/`) **and**
reshape it so the gate is an **additive** annotation layered onto the existing channel-parameterized
declaration, never a standalone test method.

## Items

1. **Create `internal/assets/runtime/shared/wip-gate-java.md`.** Shows the gate annotation added
   *above the existing `@TestTemplate` + `@Channel(...)` lines*, which are kept verbatim. Draft:

   > Add the gate annotation directly above each Acceptance Test method, keeping its existing
   > `@TestTemplate` and `@Channel(...)` annotations exactly as written:
   > ```java
   > @EnabledIfEnvironmentVariable(named = "GH_OPTIVEM_RUN_WIP_TESTS", matches = "1", disabledReason = "Work-in-progress test; set GH_OPTIVEM_RUN_WIP_TESTS=1 to run")
   > @TestTemplate
   > @Channel({ChannelType.UI, ChannelType.API})
   > void shouldXxx() { ... }
   > ```
   > Add `import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;` next to the other
   > JUnit imports if not already present. Do not replace `@TestTemplate`/`@Channel` with `@Test`.

2. **Create `internal/assets/runtime/shared/wip-gate-csharp.md` and `-typescript.md`.** Same principle:
   the gate is additive and must not clobber the scaffold's channel-parameterized declaration.
   **Before finalizing each:** confirm that language's shop-scaffold acceptance-test declaration idiom
   (the current Go literal's C# "use `[SkippableFact]` in place of `[Fact]`" and TS `test.skip(...)`
   body-statement forms may already clobber a channel attribute/wrapper) and reshape to additive.
   See Open Questions.

3. **Rework `renderGateMarkerExample(lang)` in `internal/atdd/runtime/clauderun/clauderun.go`** to read
   `runtime/shared/wip-gate-<lang>.md` via `assets.FS.ReadFile` instead of the inline `fmt.Sprintf`
   literals. Preserve the contract: return `""` for empty/unrecognised lang (and now also a missing
   asset), so the caller's "register placeholder only when non-empty → `findUnfilledPlaceholders` fails
   fast" behaviour is unchanged. Drop the now-unused `reason` const.

4. **Update `internal/assets/runtime/agents/atdd/acceptance-test-writer.md` Step 1** so the wording
   makes clear the gate is added *alongside* the normal declaration, not as a replacement method shape.

## Verification

- `go build ./...`; scoped `go test ./internal/atdd/runtime/clauderun/...` (Windows: never unbounded
  `go test ./...` — use `-p 2` or `scripts/test.sh`).
- Re-run the `gift-wrap-an-order` rehearsal (or just the `acceptance-test-writer` dispatch) and confirm
  the emitted Java AT methods retain `@TestTemplate`/`@Channel` and the two tests reach green.

## Open questions (resolve during execution, not deferred)

- **C#/TS scaffold declaration idiom (Item 2).** Need the shop-scaffold C# and TypeScript acceptance
  tests to confirm how channel parameterization is declared, so the reshaped snippet is additive rather
  than declaration-replacing (same bug class as Java). If the scaffold templates aren't reachable from
  this repo, inspect a generated multi-language project or the template source before finalizing those
  two assets. Do not ship a C#/TS snippet that prescribes replacing the channel-bearing
  attribute/wrapper.
