# 2026-06-16 11:23 UTC — Disambiguate duplicate commit-layer labels (channel / external-system)

## TL;DR

**Why:** Within one ticket's ATDD run the commit log contains byte-identical messages — e.g. two `#72 … - DSL`, two `… - SYSTEM DRIVER ADAPTERS`, two `… - SYSTEM`. The message template (`internal/atdd/process/process-flow.yaml:2363`) is `#${ticket-id} ${issue-title} - ${layer}`, and `${layer}` is a fixed string per layer with **no discriminator** for the channel (`api`/`ui`) or external system (`erp`) that the commit actually belongs to. So genuinely distinct work collapses to identical log lines.
**End result:** Each commit message carries the discriminator it already has in scope, so the log reads `… - SYSTEM (api)` / `… - SYSTEM (ui)`, `… - SYSTEM DRIVER ADAPTERS (api)` / `… (ui)`, and `… - DSL` (system) / `… - DSL (external: erp)`. Channel-agnostic layers (`ACCEPTANCE TESTS`, `TESTS`) stay bare. Non-channel / non-external runs are completely unchanged.

## Outcomes

What we get out of this:

- A `${layer-suffix}` param is threaded into the three layer sub-processes (`implement-and-verify-system`, `implement-and-verify-system-driver-adapters`, `implement-and-verify-dsl`) and appended to the hard-coded layer label at their commit nodes.
- The two **channel** unrolls (`UnrollSystemChannels`, `UnrollSystemDriverAdapterChannels` in `internal/engine/statemachine/channels.go`) inject a per-clone suffix (` (api)`, ` (ui)`); the static anchors default the suffix to `""`, so a no-`channels:` full run keeps today's bare label.
- The **external-system** DSL caller injects ` (external: ${external-system-name})`; the system-side DSL caller injects `""`.
- Strict `ExpandParams` (`run.go:382-409`) stays satisfied: **every** caller of the three sub-processes binds `layer-suffix`, so no unresolved-placeholder error can fire.
- Channel-agnostic commits (`ACCEPTANCE TESTS` at line 1024, `TESTS` at line 1471) are untouched and stay bare.

## End-result example (issue #72)

Before → After:

```
… - ACCEPTANCE TESTS              … - ACCEPTANCE TESTS
… - DSL                           … - DSL
… - DSL                           … - DSL (external: erp)
… - SYSTEM DRIVER ADAPTERS        … - SYSTEM DRIVER ADAPTERS (api)
… - SYSTEM DRIVER ADAPTERS        … - SYSTEM DRIVER ADAPTERS (ui)
… - SYSTEM                        … - SYSTEM (api)
… - SYSTEM                        … - SYSTEM (ui)
```

## Design

The discriminator is supplied as a single new param, `layer-suffix` (a pre-formatted string including its leading space, e.g. `" (api)"` or `""`), threaded from each caller down to the commit node. This is preferred over passing `channel` / `external-system-name` raw and building the label in YAML, because: (a) YAML cannot conditionally omit the parens on a no-channel run, and (b) the `""` default cleanly preserves today's behaviour where no discriminator exists. Lower-case channel token (` (api)`) matches the `acceptance-<ch>` suite naming; the unroll's display-name `ToUpper` is cosmetic and stays as-is.

**Consumer sites (append `${layer-suffix}` to the hard-coded label):**
- `implement-and-verify-system` → `COMMIT_SYSTEM`: `layer: "SYSTEM"` → `layer: "SYSTEM${layer-suffix}"` (line 1417). `layer-suffix` resolves from `implement-and-verify-system`'s own scope.
- `implement-and-verify-system-driver-adapters` → `IMPLEMENT_TEST_LAYER`: `layer: "SYSTEM DRIVER ADAPTERS"` → `layer: "SYSTEM DRIVER ADAPTERS${layer-suffix}"` (line 1082). Expanded here, then forwarded into `implement-test-layer`'s `COMMIT_LAYER` (already `layer: ${layer}`).
- `implement-and-verify-dsl` → `IMPLEMENT_TEST_LAYER`: `layer: "DSL"` → `layer: "DSL${layer-suffix}"` (line 1059).

**Caller sites (must bind `layer-suffix` — strict ExpandParams):**
- `implement-and-verify-system` callers:
  - line 443 — `change-system-behavior` anchor (channel-unrolled): static default `layer-suffix: ""`, **overridden per clone** by `UnrollSystemChannels`.
  - lines 524, 567, 595 — cover / redesign paths (not unrolled): bind `layer-suffix: ""`.
- `implement-and-verify-system-driver-adapters` caller:
  - line 923 — `write-and-verify-acceptance-tests` anchor (channel-unrolled): static default `layer-suffix: ""`, **overridden per clone** by `UnrollSystemDriverAdapterChannels`.
- `implement-and-verify-dsl` callers:
  - line 743 — system/AT-side (`shared-contract`): bind `layer-suffix: ""`.
  - line 1158 — external contract cycle: bind `layer-suffix: " (external: ${external-system-name})"` (resolves against the external clone's baked `external-system-name`, channels.go:169).

**channels.go injection** (in each `perItemParams` closure, alongside the existing `channel`/`suite` overrides):
- `UnrollSystemChannels` (line ~68): `params["layer-suffix"] = fmt.Sprintf(" (%s)", ch)`.
- `UnrollSystemDriverAdapterChannels` (line ~118): `params["layer-suffix"] = fmt.Sprintf(" (%s)", ch)`.
- `UnrollExternalSystems` is **not** touched — the external DSL suffix is supplied in YAML at line 1158 (the external clone already binds `external-system-name`).

## ▶ Next executable step (resume here)

Mechanical, no open decisions. Start with **Step 1** (channels.go injections) and **Step 2** (the three consumer labels), which are the core of the change; Step 3 (bind `""` at the remaining callers) is what keeps strict ExpandParams green and must land in the same change or tests fail.

## Steps

- [ ] **Step 1 — channels.go suffix injection.** In `internal/engine/statemachine/channels.go`, add `params["layer-suffix"] = fmt.Sprintf(" (%s)", ch)` to the `perItemParams` closures of `UnrollSystemChannels` and `UnrollSystemDriverAdapterChannels`. Leave `UnrollExternalSystems` unchanged.
- [ ] **Step 2 — consumer labels.** In `process-flow.yaml`, append `${layer-suffix}` to the three hard-coded labels: `COMMIT_SYSTEM` (line 1417), the system-driver-adapter `IMPLEMENT_TEST_LAYER` (line 1082), and the DSL `IMPLEMENT_TEST_LAYER` (line 1059).
- [ ] **Step 3 — caller binds (strict-ExpandParams safety).** Add `layer-suffix:` to every caller of the three sub-processes: static `""` on the two unrolled anchors (lines 443, 923); `""` on the non-unrolled system callers (lines 524, 567, 595); `""` on the system-side DSL caller (line 743); `" (external: ${external-system-name})"` on the external DSL caller (line 1158). Leave `ACCEPTANCE TESTS` (1024) and `TESTS` (1471) bare.
- [ ] **Step 4 — tests.** Update/extend the unroll + message tests: `internal/engine/statemachine/channels_test.go` (assert each clone's `layer-suffix` param), `run_test.go` / `internal/atdd/process/run_default_test.go` (any golden assertions on commit-message / layer strings). Add a case proving a **no-channels, no-external** run still emits bare `- SYSTEM` / `- DSL` (regression guard for the `""` default). Scope `go test` per-package (no unbounded `./...` on Windows).
- [ ] **Step 5 — verify no diagram/doc drift.** Layer labels are runtime **param values**, not node/process names, so the architecture & process diagrams are unaffected (no regeneration). Grep `docs/atdd/**` for any prose that documents the literal commit-message format and update only if a concrete example is shown. Do **not** edit generated diagram files.

## Open questions

1. **Suffix format.** ` (api)` / ` (external: erp)` as drafted — lower-case token, parens, leading space. Acceptable, or prefer e.g. ` [api]`, ` — api`, or upper-case to match the unroll display names? Pure cosmetics; decides the exact string literals.
2. **Should the bare system-side DSL also be annotated** (` (system)`) for symmetry with ` (external: erp)`, or stay bare? Bare keeps the diff minimal and matches "no discriminator = the primary path".
3. **Cover / redesign system paths** (callers 524/567/595) currently bind `""` and so stay un-suffixed. If those paths are ever channel-unrolled later, they'd want the same suffix wiring — out of scope here, noted for continuity.
