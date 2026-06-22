# 2026-06-22 20:02:30 UTC — Prevent ESCC mis-route + close the DSL-authoring gap (rehearsal #68)

## TL;DR

**Why:** Rehearsal #68 ("Apply automatic quantity discount on cart lines") halted at `ESCC_UNDECLARED_HALT` advising the operator to declare an external system — but the feature is pure internal business logic with no external system. The halt was a false positive: the system-implementer's production code was correct, the acceptance suite stayed red only because the test DSL couldn't express the authored scenarios, and the categorization mis-classified that internal-DSL scope-exception as external-contract work.

**End result:** Two layers change so the class can't recur. (1) **command:** a scope-exception that names only internal DSL files (`dsl-core`/`dsl-port`) no longer routes to `ESCC_UNDECLARED_HALT` — it falls through to the honest generic `STOP_SCOPE_VIOLATION`. (2) **agent:** the DSL-authoring prompts encode two generalizable rules (accumulating builders; consistent alias resolution) so the DSL can express every authored scenario in the first place. BPMN self-healing recovery is explicitly **out of scope** (deferred).

## Outcomes

What we get out of this:

- A vanilla internal-DSL scope-exception on a non-ESCC ticket **never** surfaces the misleading "add a `## External System Contract Criteria` section" halt — it gets the correct, honest `STOP_SCOPE_VIOLATION` instead.
- The existing #65-class behaviour is preserved: a genuine external-contract/stub scope-exception (which still names a `ct-test` or external-system-driver file) continues to route to `ESCC_UNDECLARED_HALT`.
- A regression test pins the fix: a `dsl-core`-only scope-exception on a non-ESCC ticket asserts `scope-exception-needs-escc == false`.
- The DSL-authoring agents carry two written rules that prevent the underlying defect class:
  - **Accumulating builders** — a fluent builder callable multiple times for distinct entities (e.g. `withLine(sku, quantity)`) appends rather than overwrites, and the When/Then stages model a collection (a real multi-line order), not a single entity.
  - **Consistent alias resolution** — every assertion/lookup helper that takes a param-or-alias resolves it via `useCaseContext.getParamValue`, matching the established sibling helper (`hasAppliedCoupon`).
- The `contractStubScopeLayers` doc comment reflects the narrowed contract (no stale rationale).

## ▶ Next executable step (resume here)

**Step 1 (command layer).** In `internal/atdd/process/actions/scope_exception.go`, narrow the ESCC-determining layer set from `{ct-test, dsl-port, dsl-core, external-system-driver-adapter}` to **`{ct-test, external-system-driver-adapter}`** — remove `dsl-core` AND `dsl-port`. `scope-exception-needs-escc` then becomes true only when ≥1 scope-exception file sits under one of those two genuinely-external layers AND `ticket-has-escc == false`. Update the `contractStubScopeLayers` doc comment (`scope_exception.go:11-32`) to match. Stop at: code compiles, no test run yet (tests come in Step 2). Unblocks the regression test and the rest of the command-layer work.

> Rationale (resolved): a layer only qualifies as an external-work fingerprint if no vanilla agent writes there. `dsl-core` and `dsl-port` are both written by the vanilla `acceptance-test-writer` (`process-flow.yaml:2060`), so they're false-positive prone — drop both. `ct-test` is written only by the external-contract agents (`contract-test-writer`, `stub-fidelity-test-writer`), so it stays.

## Steps

### Command layer — `internal/atdd/process/actions/scope_exception.go`

- [ ] Step 1: Narrow the ESCC-determining layer set to **`{ct-test, external-system-driver-adapter}`** — remove both `dsl-core` and `dsl-port` (both are written by the vanilla `acceptance-test-writer`, so neither is a reliable external-work signal); keep `ct-test` and `external-system-driver-adapter` (written only by external-contract agents). Refresh the `contractStubScopeLayers` doc comment (`:11-32`) to state the narrowed contract — self-contained, no cross-language/cross-project references.
- [ ] Step 2: Add/adjust the scope-exception tests (`internal/atdd/process/actions/` + the `bindings_test.go` in `actions/` and `gates/`) to cover the regression: a `dsl-core`-only scope-exception on a non-ESCC ticket → `scope-exception-needs-escc == false` (routes to `STOP_SCOPE_VIOLATION`, not ESCC). Keep/confirm an existing case proving a genuine external-layer file still routes to `ESCC_UNDECLARED_HALT`.
- [ ] Step 3: Run `go test ./internal/atdd/process/...` (and `go build ./...`) to confirm green. Verify no other caller of `contractStubScopeLayers` / `contractStubSystemNames` depends on the old membership.

### Agent layer — runtime prompts

- [ ] Step 4: In `internal/atdd/assets/runtime/agents/atdd/dsl-implementer.md`, add the two DSL-authoring rules: (a) accumulating builders append per call and the stages model a collection (real multi-line order placement, not a single sku/quantity); (b) every assertion/lookup helper resolves a param-or-alias via `useCaseContext.getParamValue`, matching the `hasAppliedCoupon` reference — never match the raw argument.
- [ ] Step 5: In `internal/atdd/assets/runtime/agents/atdd/acceptance-test-writer.md`, require the **prototype shape** to support these rules: a repeatable builder must be designed so it *can* hold multiple distinct entities (a collection-shaped prototype), so the dsl-implementer can fill in accumulation without redesigning the surface.
- [ ] Step 6: Capture the honest scope caveat in the prompts (or the plan's record): the alias-resolution rule is a reliable win; the multi-line capability is larger (it spans the when/then DSL plus the order-placement plumbing — driver port + system driver adapter), so the prompt rule reduces but may not fully eliminate the multi-line gap. The command-layer fix is the safety net.

### Wrap-up

- [ ] Step 7: Rebuild the `gh-optivem` binary so the prompt edits take effect — the runtime `*.md` prompts are embedded via `//go:embed runtime` (`internal/atdd/assets/embed.go:21`), so they are compiled in and not live until rebuilt. Run `go build ./...` and confirm the full test suite is green.

## Open questions

_All resolved during planning (2026-06-22):_

- **`dsl-port` membership** → **drop it** alongside `dsl-core`. The vanilla `acceptance-test-writer` writes `dsl-port` (`process-flow.yaml:2060`), so it has the same false-positive risk. Folded into Step 1.
- **`ct-test` as an ESCC-determining layer** → **keep it.** `ct-test` is written only by the external-contract agents (`contract-test-writer`, `stub-fidelity-test-writer`); no vanilla agent writes it, so it's a safe external signal. Folded into Step 1.
- **Embedded assets** → **rebuild required.** Prompts are `//go:embed runtime` (`internal/atdd/assets/embed.go:21`), compiled into the binary; a `go build` is needed for edits to go live. Folded into Step 7.
- **Agent-rule effectiveness** → accepted caveat: the multi-line rule may not fully close the gap via prompt alone. Acceptable because the command-layer fix makes any residual failure honest rather than misleading. (No action; recorded for expectations in Step 6.)
