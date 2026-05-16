# ATDD AT-Cycle Gap Analysis

Comparison of `docs/atdd-at-cycle.md` against the internal/assets pages for AT-RED and AT-GREEN, in both `global/` and `runtime/` trees.

**Date:** 2026-05-16

## Sources compared

- `docs/atdd-at-cycle.md` (the compact spec)
- `internal/assets/global/docs/atdd/process/at-red-test.md`
- `internal/assets/global/docs/atdd/process/at-red-dsl.md`
- `internal/assets/global/docs/atdd/process/at-red-system-driver.md`
- `internal/assets/global/docs/atdd/process/at-green-system.md`
- `internal/assets/runtime/prompts/atdd/at-red-test.md`
- `internal/assets/runtime/prompts/atdd/at-red-dsl.md`
- `internal/assets/runtime/prompts/atdd/at-red-system-driver.md`
- `internal/assets/runtime/prompts/atdd/at-green-system-backend.md`
- `internal/assets/runtime/prompts/atdd/at-green-system-frontend.md`

The runtime prompts are thin — they mostly point at the global process docs — so the bulk of the gap is between `atdd-at-cycle.md` and the **global** process pages.

## Things present in internal/assets but missing from atdd-at-cycle.md

### Cross-cutting (across all phases)

- **"Purpose" framing** for each phase — what the phase exists to accomplish.
- **"What it produces"** section per phase — the post-condition (working-tree state + test state).
- **Explicit test-state per phase** — change-driven scenarios disabled with reason `"AT - RED - TEST"` / `"AT - RED - DSL"` / `"AT - RED - SYSTEM DRIVER"` (used as the marker for the next phase to re-enable).
- **Re-enable step at the start of each next phase** ("Enable the tests marked disabled with reason `…`") — atdd-at-cycle.md never mentions enabling/disabling.
- **Legacy-coverage scenarios** as a distinct concept — they stay enabled and passing throughout RED; never `@Disabled`.
- **Concrete before/after code examples** per phase (Java shown, pointer to language-equivalents).
- **Pointer to `language-equivalents/`** for `@Disabled`/skip syntax and `"TODO:"` prototype syntax per language.
- **Pointer to `glossary.md`** for the precise definition of "interface change", "System Driver" vs "External System Driver".
- **Pointer to `architecture/` docs** (test.md, dsl-core.md, driver-port.md).
- **"Anti-patterns" section** in every phase — what NOT to do and why.
- **"Scope" callout** in DSL/Driver phases — what is allowed to be touched in that phase.

### AT - RED - TEST specific

- **Unit of work = the ticket** (all scenarios written as a batch, no per-scenario inner loop).
- **Scenario ordering rule** within the test class:
  1. Legacy Coverage scenarios
  2. New scenarios using only existing DSL
  3. New scenarios needing new DSL
- **One-to-one Gherkin→test mapping** ("no interpretation"; every precondition must appear in the test).
- **"Minimum data" rule** — only inputs/assertions directly relevant; let DSL defaults handle the rest.
- **"WRITE must compile"** rule (RED is proven by runtime failure, not compile failure).
- **Legacy-coverage tests must pass on first run** — failing legacy is a real bug → STOP, do not paper over with `@Disabled`.

### AT - RED - DSL specific

- **Both flags must be set explicitly** — `External System Driver Interface Changed` AND `System Driver Interface Changed` (atdd-at-cycle.md only mentions setting them, doesn't call out that unset = bug).
- **Driver interface changes must be minimal** — only what new DSL actually calls.
- **"Leaving TODO: DSL behind"** anti-pattern — phase not done until all are replaced.

### AT - RED - SYSTEM DRIVER specific

- **File-scope constraint** — only `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/<channel>` (api, ui). All driver code in test tree, not `system/`.
- **Do NOT touch `external/`** — that's the CT sub-process.
- **Do NOT read backend/frontend source code** — model new methods on sibling Driver methods in the same file.

### AT - GREEN - SYSTEM specific

- **Tests, DSL, Drivers are frozen** — if making them pass seems to require touching those layers, an earlier phase was wrong.
- **Backend and frontend split** into explicit steps (atdd-at-cycle.md collapses to one line).
- **Escalation rule**: if you can't make tests pass without touching tests/DSL/Drivers, **ask the user** — don't patch around it.

### From the runtime prompts (operational, not in the global pages either)

- **Compile-fix retry policy** — "if your previous WRITE didn't compile, fix the broken/missing piece minimally; don't change test intent / DSL semantics."
- **Batch edits to the same file** in one Write or one larger Edit (latency/token note).
- **"Do not present or wait for approval inside the agent."**
- **Model/effort assignments** per phase (Sonnet vs Opus, medium vs high):
  - Test = Sonnet / medium
  - DSL = Opus / medium
  - System Driver = Sonnet / medium
  - Backend / Frontend = Sonnet / high

## Things atdd-at-cycle.md has that the internal pages don't really emphasize

- The clean **PRE** step (Gherkin shape check + positive/negative coverage) — the global pages don't have an explicit PRE/AC-quality gate.
- A flat **one-line summary** of the cycle (RED → GREEN → REFACTOR) up front.
- An explicit **REFACTOR** step ("propose first, then implement") — the global pages don't cover REFACTOR at all.

## Smaller nits in atdd-at-cycle.md

- Line 33 header is `## RED: External System Driver` but should be `### RED: External System Driver` (sibling of the other RED sub-steps).
- Says "ATDD - CT Cycle"; the internal pages call it the "Contract Test sub-process" / "CT - RED - EXTERNAL DRIVER".
- Typo: "mechanicla" → "mechanical" (line 15).
