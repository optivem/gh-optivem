# Plan: bring `docs/atdd-at-cycle.md` to parity with internal/assets

**Date:** 2026-05-16
**Context:** The goal is to eventually delete `internal/assets/`. `docs/atdd-at-cycle.md` is intended to become the canonical home for the AT cycle process spec, replacing the four global process pages under `internal/assets/global/docs/atdd/process/at-{red,green}-*.md`.
**Source:** Gap analysis in [reports/atdd-at-cycle-gap-analysis.md](../reports/atdd-at-cycle-gap-analysis.md).

Phasing is by impact, so the work can stop at any phase and decide to split.

## Phase 1 — Critical mechanics (load-bearing; the cycle breaks without them)

Without these, an agent following `atdd-at-cycle.md` literally would produce wrong output.

1. **Add the disable/re-enable mechanism.** Each RED sub-phase ends with change-driven tests `@Disabled("AT - RED - <phase>")`. Each next phase begins with "Enable the tests marked disabled with reason `…`". This is the boundary marker — without it the phases don't compose.
2. **Make flag-setting explicit and gated.** In RED-DSL: both `External System Driver Interface Changed` AND `System Driver Interface Changed` MUST be set — unset is a bug, they gate downstream phases.
3. **Add "tests/DSL/Drivers frozen in GREEN" rule** + escalation: if GREEN can't pass without touching them, ask the user (don't patch around).
4. **Add file-scope constraint to RED-SYSTEM-DRIVER:** only `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/<channel>`. Do not touch `external/` (CT sub-process owns that). Do not touch `system/`.
5. **Add legacy-coverage concept:** distinct from change-driven scenarios; stays enabled throughout RED; failing legacy = real bug → STOP (do not `@Disabled`).

## Phase 2 — Per-phase rules (prevent common mistakes)

6. **RED-TEST: Unit of work = the ticket.** All scenarios written as a batch, no per-scenario inner loop.
7. **RED-TEST: Scenario ordering in the test class** — Legacy → existing-DSL → new-DSL.
8. **RED-TEST: One-to-one Gherkin→test mapping.** Every precondition appears in the test; no interpretation.
9. **RED-TEST: Minimum-data rule.** Only inputs/assertions tied to Given/When/Then. Trust DSL defaults.
10. **RED-TEST: WRITE must compile.** RED is proven by runtime failure, not compile failure. (Replaces current "implement DSL prototypes so compilation works" with the same idea but explicit.)
11. **RED-DSL: Driver interface changes must be minimal** — only what new DSL actually calls.
12. **RED-SYSTEM-DRIVER: Do NOT read backend/frontend source code.** Model new methods on sibling Driver methods in the same file.
13. **GREEN: Split into backend and frontend steps** (with separate "ask the user if blocked" guards).

## Phase 3 — Framing per phase

14. Add **Purpose** (1–2 lines) and **What it produces** (post-condition: working tree + test state) for each of: RED-TEST, RED-DSL, RED-SYSTEM-DRIVER, GREEN.
15. Add **Anti-patterns** list to each phase (the global pages have 3–4 per phase; these are high-value because they encode lessons learned).

## Phase 4 — Examples

16. Add **one before/after code example** per phase (Java is fine as the default in-doc; cross-language guidance lives in the language-equivalents pages — see Phase 5).

## Phase 5 — Cross-references

The internal pages link to `glossary.md`, `language-equivalents/`, `architecture/test.md`, `architecture/dsl-core.md`, `architecture/driver-port.md`. These need targets:

17. Decide migration targets for those supporting docs (likely `docs/atdd-glossary.md`, `docs/atdd-language-equivalents/`, `docs/atdd-architecture-*.md`). **Out of scope for the atdd-at-cycle.md edit itself**, but the doc should reference the final names — so this decision blocks Phase 5 work.

## Phase 6 — Mechanical fixes (do anytime, low risk)

18. Line 33: `## RED: External System Driver` → `### RED: External System Driver`.
19. Line 15: "mechanicla" → "mechanical".
20. "ATDD - CT Cycle" → link to `atdd-ct-cycle.md` (consistent with internal page terminology "Contract Test sub-process" / "CT - RED - EXTERNAL DRIVER").

## Phase 7 — NOT in this file (flagged as related work)

- **Runtime prompt content** (compile-fix retry policy, batch-edits hint, "no approval inside agent", model/effort): these are agent-operational, not process-spec. They belong in the prompt files. The prompt files themselves are a separate migration concern — if `internal/assets/runtime/` is going away, those need a new generation mechanism, not relocation into `docs/`.
- **Supporting docs migration** (architecture/, language-equivalents/, glossary.md, testkit-*, placeholders.md, cycles.md, task-and-chore-cycles.md, system-interface-redesign.md, diagram-phase-details.md): 22 files in `internal/assets/global/docs/atdd/` that aren't process pages. Each needs a `docs/` home decided before internal/assets can be deleted.
- **Parallel CT-cycle work**: `atdd-ct-cycle.md` likely has the same gaps vs its four internal CT pages (ct-red-test, ct-red-dsl, ct-red-external-driver, ct-green-stubs). Worth a parallel gap analysis.

## Recommendation on doc shape

After Phases 1–6, `atdd-at-cycle.md` will grow from ~40 lines to ~250 lines. That's still one readable file, and matches the total size of the four global process pages it replaces. If it goes much beyond that during Phase 4 (examples) or Phase 5 (cross-refs to migrated supporting docs), consider splitting `docs/atdd/process/` to mirror the internal structure — but default to keeping it one file unless it gets unwieldy.

## Open questions before starting

- Should the migration plan for the supporting docs (Phase 7) be written as a separate report, or held off until after the at-cycle work lands?
- For Phase 4 examples — Java only, or should the doc be language-neutral and defer all code examples to language-equivalents pages?
