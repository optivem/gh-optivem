# Plan: bring `docs/atdd-at-cycle.md` to parity with internal/assets — Part 2: per-phase content

> ⚠️ **NOT YET REFINED** — these phases were promoted out of [Part 1](20260516-1701-atdd-at-cycle-absorb-internal-assets.md) without per-item refinement. Run `/refine-plan` on this file before `/execute-plan`. Items may need restructuring once they are discussed in the context of the cycle architecture and §Conventions established in Part 1.

**Date:** 2026-05-18 (split from Part 1 during refinement)
**Context:** Phases 2–6 of the original "bring `docs/atdd-at-cycle.md` to parity with internal/assets" plan. Part 1 ([20260516-1701-atdd-at-cycle-absorb-internal-assets.md](20260516-1701-atdd-at-cycle-absorb-internal-assets.md)) covers the cycle architecture and §Conventions (disable-reason, phase-output flags, phase scope policy). This Part 2 covers the per-phase content of `atdd-at-cycle.md`. Independent of Part 1 — can run in parallel or after.
**Source:** Gap analysis in [reports/atdd-at-cycle-gap-analysis.md](../reports/atdd-at-cycle-gap-analysis.md).

## Phase 2 — Per-phase rules (prevent common mistakes)

6. **RED-TEST: Unit of work = the ticket.** All scenarios written as a batch, no per-scenario inner loop.
7. **RED-TEST: Scenario ordering in the test class** — Legacy → existing-DSL → new-DSL. <!-- VJ-2026-05-18 refine note: legacy tests in the class are pre-authored by prior legacy-cycle runs (see [legacy-coverage-cycle plan](20260518-1116-legacy-coverage-cycle.md)), not written by AT-RED-TEST. Wording should reflect that the AT-RED-TEST agent orders new change-driven tests after pre-existing legacy ones. -->
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

17. Decide migration targets for those supporting docs (likely `docs/atdd-glossary.md`, `docs/atdd-language-equivalents/`, `docs/atdd-architecture-*.md`). **Out of scope for the `atdd-at-cycle.md` edit itself**, but the doc should reference the final names — so this decision blocks Phase 5 work.

## Phase 6 — Mechanical fixes (do anytime, low risk)

18. Line 33: `## RED: External System Driver` → `### RED: External System Driver`.
19. Line 15: "mechanicla" → "mechanical".
20. "ATDD - CT Cycle" → link to `atdd-ct-cycle.md` (consistent with internal page terminology "Contract Test sub-process" / "CT - RED - EXTERNAL DRIVER").

## Recommendation on doc shape

After Part 1 + this Part 2 land, `atdd-at-cycle.md` will grow from ~40 lines to ~250 lines. That is still one readable file, and matches the total size of the four global process pages it replaces. If it goes much beyond that during Phase 4 (examples) or Phase 5 (cross-refs to migrated supporting docs), consider splitting `docs/atdd/process/` to mirror the internal structure — but default to keeping it one file unless it gets unwieldy.

## Open questions before starting

- Should the migration plan for the supporting docs (Phase 7 bullet in Part 1) be written as a separate report, or held off until after the at-cycle work lands?
- For Phase 4 examples — Java only, or should the doc be language-neutral and defer all code examples to language-equivalents pages?
