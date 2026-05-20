# Plan: legacy coverage cycle

**Date:** 2026-05-18 (originally split from the AT-cycle Part 1 plan; that plan was subsequently pruned in commit `acd6fa4`).
**Context:** Defines the **legacy coverage cycle** as a **top-level phase that runs strictly upstream of the change cycle** — sequenced before any behavioural/structural/DA/SUT cycle is attempted (current BPMN: `INTAKE → RUN_LEGACY_CYCLE → BACKLOG_REFINEMENT → RUN_CYCLE`). Not a peer of AT. Triggered by **legacy acceptance criteria** in a ticket — retroactively writes acceptance tests (and external-system contract tests) for already-existing behaviour that lacks coverage. **Inverted RED-GREEN shape:** tests should **pass on first run** (the behaviour already exists); if they don't, the test is probably wrong and needs revision. No code-writing phase.

**Live cross-references:**
- [`internal/atdd/runtime/statemachine/process-flow.yaml`](../internal/atdd/runtime/statemachine/process-flow.yaml) — `legacy_acceptance_criteria` sub-process (lines 1343–1356) is the BPMN stub Item 1b fleshes out.
- [BPMN orchestration plan (deferred)](deferred/20260518-1144-atdd-bpmn-orchestration.md) — **item 7** (failing-legacy detector) is the downstream consumer of Item 2's marker schema; blocked on Item 2 landing.

> **Refined 2026-05-20:** Reframed legacy from "top-level sibling of the AT cycle (peer to structural)" to "top-level phase that runs strictly upstream of the change cycle" — matches the current BPMN sequencing. Dead cross-refs (Part 1, Part 2) removed; live cross-refs (process-flow.yaml, bpmn-orchestration deferred plan item 7) added.

## Scope

1. **Legacy cycle definition.** Three sub-items; **1c is a precondition** for 1a and 1b.

   **1c. Design the legacy cycle's phase shape** *(precondition)*. Decide how many phases the legacy-coverage cycle has and what each one writes. Candidate shapes:
   - One phase (TEST only — author the missing test, expect pass).
   - Two phases (TEST + SYSTEM-DRIVER — for external-system contract gaps).
   - Three phases (mirror AT: TEST + DSL + SYSTEM-DRIVER).

   Sub-question (was Open question 3): are legacy *contract* tests the same cycle with a different test layer, or their own sub-cycle within legacy? The answer feeds 1c directly.

   Output: a decision recorded in this plan (or its successor) that pins the phase list before 1a/1b can land.

   **1a. Per-phase legacy-cycle prompts.** Once 1c is decided, author one runtime prompt per phase under `internal/assets/runtime/prompts/atdd/` (following the AT/CT naming pattern, e.g. `legacy-test.md`). Each prompt carries the cycle's prose preamble — cycle shape (test → expect pass; if fails, revise test), behavioural expectations, escalation rules, and sequencing relative to AT (legacy runs strictly upstream of the change cycle; AT-RED-TEST may encounter legacy tests sitting in the suite but does not author them). This matches the post-`4b44722` "process docs inlined into prompt readers" layout — there is no standalone `docs/atdd/process/legacy-coverage-cycle.md` to author.

   **1b. Flesh out the BPMN `legacy_acceptance_criteria` sub-process.** Replace the current STOP — HUMAN REVIEW stub at `internal/atdd/runtime/statemachine/process-flow.yaml:1343-1356` with the real sub-process body once 1c is decided: a node per phase, gateway(s) for inverted-RED (test expected to pass on first run), and an escalation route when the test fails (the behaviour didn't already exist, so the test or its assertion is suspect). Re-render `docs/process-diagram.md`.

   > **Refined 2026-05-18:** Reframed from "create a doc" → "create a prose doc *parallel* to atdd-at-cycle.md/atdd-ct-cycle.md, AND flesh out the BPMN `LEGACY_CYCLE` sub-process in parallel". **Why:** the legacy cycle should follow the established pattern — every cycle has both a BPMN representation in `process-flow.yaml` and a prose doc; legacy already has a BPMN stub but no flesh, and was missing the prose doc entirely.
   >
   > **Refined 2026-05-20:** Split into 1a/1b/1c after the prose-doc target moved. `docs/atdd/process/atdd-at-cycle.md` / `atdd-ct-cycle.md` no longer exist — process docs were inlined into prompt readers in commit `4b44722`. 1a retargets to that inlined-prompt layout. The original "Phase 7 BPMN dependency" cross-reference is no longer valid because the Part 1 plan was pruned in commit `acd6fa4`; the BPMN flesh-out work moves into this plan as 1b. The phase-shape question (1c) was implicit before and is now surfaced as an explicit precondition.

2. **Design the legacy-marker schema.** A machine-checkable annotation / naming / location convention so the BPMN failing-legacy detector (see *Consumer pointer* below) can recognize legacy tests at runtime.

   Candidate forms (was Open question 1): annotation (`@LegacyCoverage`), naming (`*_LegacyTest`), directory (`tests/legacy/`), or a mix. Each has tradeoffs for the BPMN detector — annotation is least ambiguous but most invasive; directory is cheap but loses locality with the AT tests; naming is easy to grep but easy for humans to break. Decide here.

   > **Refined 2026-05-20:** Dropped the disable-reason sub-bullet (was: *"applies only to change-driven scenarios; never to legacy. The re-enable filter must not match legacy markers."*). **Why:** the legacy cycle is single-shot — tests retroactively cover existing behaviour, so they pass on first run; no staged multi-phase RED build-up, so no `@Disabled` is ever authored on the legacy side. The "applies only to change-driven" rule has nothing to bite on, and the "re-enable filter must not match legacy markers" property is vacuous because legacy tests are never disabled in the first place. Open Question 1 (marker form) folded in here since it's the live design.
   >
   > **Refined 2026-05-20 (later):** Consumer set narrowed. Originally listed two consumers (the BPMN detector and an AT-side guardrail); the AT-side guardrail was Item 3, which was deleted because legacy runs strictly upstream of AT and the AT-RED-* agents never special-case legacy. Only the BPMN detector remains as a consumer, which loosens the design constraint — the marker just needs to be machine-detectable by the runtime, not human-readable on the AT side.
   >
   > **Consumer pointer (folded in from former Item 4):** `plans/deferred/20260518-1144-atdd-bpmn-orchestration.md` **item 7 (failing-legacy detector)** — that plan was executed 2026-05-19 with all other items landed; item 7 is explicitly blocked on this Item 2 settling the marker convention. Path may shift when the deferred plan reactivates.

> **Renumber pass 2026-05-20:** Original Item 3 (AT-side updates to `atdd-at-cycle.md`) deleted during the refine walk — legacy runs strictly upstream of AT, so the boundary-statement and failing-legacy-guardrail sub-bullets both collapse (boundary has no peer to declare; AT-RED-* never special-cases legacy, and `disable-tests.md:40` already covers "don't disable tests you didn't author"). Original Item 4 (BPMN failing-legacy detector cross-plan pointer) deleted and folded into Item 2's "Consumer pointer" block. After renumbering, only items 1 and 2 remain.

## Out of scope

- Legacy cycle's own scope-policy row in `internal/atdd/phase-scopes.yaml` and `internal/assets/runtime/shared/scope.md` doctrine — handled when 1a authors the per-phase prompts (each prompt's `scope:` frontmatter pulls from those sources).
- Implementing the BPMN failing-legacy detector — orchestration code lives in [BPMN orchestration plan (deferred)](deferred/20260518-1144-atdd-bpmn-orchestration.md) item 7, blocked on Item 2 here.
- Structural cycle definition and dispatcher/router — separate plan; the bpmn-orchestration plan's residual scope handles the dispatcher side.

## Open questions

- Does the legacy cycle have its own scope-policy row (per `internal/assets/runtime/shared/scope.md` + `internal/atdd/phase-scopes.yaml`)? Probably yes — same shape, different allowed-paths.

> **Refined 2026-05-20:** Two questions folded into Scope items:
> - Marker convention (annotation / naming / directory) → folded into Item 2.
> - Legacy contract tests (same cycle vs. own sub-cycle) → folded into Item 1c as a sub-question of phase-shape design.
>
> The remaining question (scope-policy row) was also retargeted: "§Conventions → Phase scope policy in Part 1" no longer exists; the live home is `internal/assets/runtime/shared/scope.md` (doctrine) + `internal/atdd/phase-scopes.yaml` (layer mapping).
