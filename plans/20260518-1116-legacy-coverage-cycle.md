# Plan: legacy coverage cycle

**Date:** 2026-05-18 (originally split from the AT-cycle Part 1 plan; that plan was subsequently pruned in commit `acd6fa4`).
**Context:** Defines the **legacy coverage cycle** as a **top-level phase that runs strictly upstream of the change cycle** — sequenced before any behavioural/structural/DA/SUT cycle is attempted (current BPMN: `INTAKE → RUN_LEGACY_CYCLE → BACKLOG_REFINEMENT → RUN_CYCLE`). Not a peer of AT. Triggered by **legacy acceptance criteria** in a ticket — retroactively writes acceptance tests (and external-system contract tests) for already-existing behaviour that lacks coverage. **Inverted RED-GREEN shape:** tests should **pass on first run** (the behaviour already exists); if they don't, the test is probably wrong and needs revision. No code-writing phase.

**Live cross-references:**
- [`internal/atdd/runtime/statemachine/process-flow.yaml`](../internal/atdd/runtime/statemachine/process-flow.yaml) — `legacy_acceptance_criteria` sub-process (lines 1343–1356) is the BPMN stub Item 1b fleshes out.
- [BPMN orchestration plan (deferred)](deferred/20260518-1144-atdd-bpmn-orchestration.md) — item 7 (failing-legacy detector) was originally the consumer of Item 2's marker schema. With Item 2 resolved as "no marker" (2026-05-20), item 7 is **no longer blocked on this plan** — it must either retire entirely or key on a non-marker signal when the deferred plan reactivates. See Item 2 below.

> **Refined 2026-05-20:** Reframed legacy from "top-level sibling of the AT cycle (peer to structural)" to "top-level phase that runs strictly upstream of the change cycle" — matches the current BPMN sequencing. Dead cross-refs (Part 1, Part 2) removed; live cross-refs (process-flow.yaml, bpmn-orchestration deferred plan item 7) added.

## Scope

1. **Legacy cycle definition.** Three sub-items; **1c is a precondition** for 1a and 1b.

   **1c. Phase shape — decided 2026-05-20.** Two parallel legacy cycles, each mirroring the corresponding change cycle's RED-side phases (no GREEN/REFACTOR — SUT already exists by premise):

   - **`legacy_at`** — `LEGACY_AT_TEST` → `LEGACY_AT_DSL` (gated on `dsl_interface_changed`) → `LEGACY_AT_SYSTEM_DRIVER` (gated on `system_driver_interface_changed`) → inverted-RED verify gate.
   - **`legacy_ct`** — `LEGACY_CT_TEST` → `LEGACY_CT_DSL` (gated on `dsl_interface_changed`) → `LEGACY_CT_EXTERNAL_SYSTEM_DRIVER` (gated on `external_system_driver_interface_changed`) → `LEGACY_CT_EXTERNAL_SYSTEM_STUB` (always — the stub is test infrastructure, not production code) → inverted-RED verify gate.

   **Sub-question (was Open question 3) — resolved.** Legacy contract tests are their own sub-cycle (`legacy_ct`), parallel to `legacy_at`, not a layer inside it. Justification: change-cycle AT and CT already diverge in phase count and in what their final phase writes; a single legacy cycle that branched AT-vs-CT internally would re-encode that split twice.

   **Inverted-RED verify gate (per cycle, not per phase).** Each RED-style phase authors one layer; the test isn't runnable until all gated layers are in place. The "expected to pass" check happens once, at end-of-cycle, on the assembled test — equivalent to the existing `VERIFY_AT_DRIVER` / `VERIFY_CT_DRIVER` gates but with the opposite expected outcome. Pass → cycle ends green. Fail → escalate; the test / DSL / driver / stub is wrong, **never** the SUT. No production code is ever authored or modified in a legacy cycle.

   **Phase gating reuses existing flags.** `dsl_interface_changed`, `system_driver_interface_changed`, `external_system_driver_interface_changed` already gate which RED phases run in the change cycle. The legacy variants gate identically — if a legacy AT test reuses an existing DSL helper, `LEGACY_AT_DSL` is skipped just as `AT_RED_DSL` is.

   This decision pins 1a (seven new per-phase prompts: three legacy-AT + four legacy-CT; verify gates are BPMN-side, no prompt) and 1b (two BPMN sub-processes replacing the current `legacy_acceptance_criteria` stub, dispatched on AT-vs-CT criterion type).

   **1a. Per-phase legacy-cycle prompts.** Author seven runtime prompts under `internal/assets/runtime/prompts/atdd/`, one per phase pinned in 1c. The RED/GREEN infix from the change-cycle names drops — every legacy phase is implicitly inverted-RED, and the distinction has no remaining bite once the SUT-authoring half of the cycle is gone.

   - `legacy-at-test.md` (phase id `LEGACY_AT_TEST`)
   - `legacy-at-dsl.md` (phase id `LEGACY_AT_DSL`)
   - `legacy-at-system-driver.md` (phase id `LEGACY_AT_SYSTEM_DRIVER`)
   - `legacy-ct-test.md` (phase id `LEGACY_CT_TEST`)
   - `legacy-ct-dsl.md` (phase id `LEGACY_CT_DSL`)
   - `legacy-ct-external-system-driver.md` (phase id `LEGACY_CT_EXTERNAL_SYSTEM_DRIVER`)
   - `legacy-ct-external-system-stub.md` (phase id `LEGACY_CT_EXTERNAL_SYSTEM_STUB`)

   **Preamble strategy — decided 2026-05-20.** Each prompt carries the cycle's prose preamble *inline*, mirroring the post-`4b44722` self-contained-prompt layout used by `at-red-*.md` / `ct-red-*.md`. No shared-snippet include mechanism is introduced — that would diverge from the inlined-prompt doctrine for the sake of saving a few lines of duplication. The preamble in each prompt covers:

   - **Cycle shape**: legacy phases author test-side artifacts only; on the verify gate, the assembled test is expected to pass on first run.
   - **Inverted-RED escalation**: if the verify gate fails, the test / DSL / driver / stub is suspect; revise it. The SUT is never modified — a legacy cycle that wants to change production code is a category error and must be re-routed through the change cycle.
   - **Sequencing relative to AT/CT change cycles**: legacy runs strictly upstream of the change cycle (BPMN: `INTAKE → RUN_LEGACY_CYCLE → BACKLOG_REFINEMENT → RUN_CYCLE`). Change-cycle phases may encounter legacy tests sitting in the suite but never author them; `disable-tests.md:40`'s "don't disable tests you didn't author" rule already covers that direction.

   This matches the post-`4b44722` "process docs inlined into prompt readers" layout — there is no standalone `docs/atdd/process/legacy-coverage-cycle.md` to author.

   **Scope-row pairing.** Alongside each new prompt, add a corresponding row in `internal/atdd/phase-scopes.yaml` (required by memory `feedback_no_deferred_mechanism.md` — every writing-agent phase must have its scope explicitly pinned). Each row reuses the change-cycle counterpart's canonical layer keys verbatim (mapping table in the Open Questions wrap-up below). `internal/assets/runtime/shared/scope.md` doctrine prose needs no edit — no new layer category is introduced.

   **1b. Flesh out the BPMN `legacy_acceptance_criteria` sub-process.** Replace the current STOP — HUMAN REVIEW stub at `internal/atdd/runtime/statemachine/process-flow.yaml:1343-1356` with the real sub-process body. Decisions pinned 2026-05-20:

   - **Dispatch shape (keep wrapper, branch inside it):** `legacy_acceptance_criteria` stays as the named sub-process called from `RUN_LEGACY_CYCLE`. Its body becomes `start → gateway-on-criterion-type → two call_activity branches → join → end`, dispatching to two new sub-processes:
     - `legacy_at_cycle` — phases `LEGACY_AT_TEST` → `LEGACY_AT_DSL` (gated) → `LEGACY_AT_SYSTEM_DRIVER` (gated) → `VERIFY_LEGACY_AT`.
     - `legacy_ct_cycle` — phases `LEGACY_CT_TEST` → `LEGACY_CT_DSL` (gated) → `LEGACY_CT_EXTERNAL_SYSTEM_DRIVER` (gated) → `LEGACY_CT_EXTERNAL_SYSTEM_STUB` → `VERIFY_LEGACY_CT`.
     The upstream `RUN_LEGACY_CYCLE` caller stays oblivious to the AT/CT split — the wrapper owns it.
   - **Verify-gate naming:** `VERIFY_LEGACY_AT` and `VERIFY_LEGACY_CT`, keyed by cycle (mirroring `VERIFY_AT_DRIVER` / `VERIFY_CT_DRIVER` but named after what's being verified — the whole assembled test passing on first run, not a single layer).
   - **Verify-fail route (STOP — HUMAN REVIEW, no loopback):** On fail, route to a `LEGACY_*_VERIFY_FAILED` user_task (one per cycle) with `agent: human, role: review`, then end. The human edits the offending layer and re-runs the legacy cycle from scratch from `RUN_LEGACY_CYCLE`. **No loopback edge from VERIFY back to TEST** — explicitly chosen to avoid the statemachine-test loop hazard documented in memory `feedback_statemachine_test_loop_hazard.md`.

   **Open during execution (not blocking):** the discriminator variable name for the criterion-type gateway (e.g. `legacy_criterion_type ∈ {at, ct}` vs. two booleans). 1b execution picks the form that matches the existing process-flow.yaml conventions for gateway variables.

   Re-render `docs/process-diagram.md` after the YAML edit.

   > **Refined 2026-05-18:** Reframed from "create a doc" → "create a prose doc *parallel* to atdd-at-cycle.md/atdd-ct-cycle.md, AND flesh out the BPMN `LEGACY_CYCLE` sub-process in parallel". **Why:** the legacy cycle should follow the established pattern — every cycle has both a BPMN representation in `process-flow.yaml` and a prose doc; legacy already has a BPMN stub but no flesh, and was missing the prose doc entirely.
   >
   > **Refined 2026-05-20:** Split into 1a/1b/1c after the prose-doc target moved. `docs/atdd/process/atdd-at-cycle.md` / `atdd-ct-cycle.md` no longer exist — process docs were inlined into prompt readers in commit `4b44722`. 1a retargets to that inlined-prompt layout. The original "Phase 7 BPMN dependency" cross-reference is no longer valid because the Part 1 plan was pruned in commit `acd6fa4`; the BPMN flesh-out work moves into this plan as 1b. The phase-shape question (1c) was implicit before and is now surfaced as an explicit precondition.

2. **Legacy-marker schema — no marker. Decided 2026-05-20.**

   Legacy tests are authored into the same folders as change-cycle tests and are **indistinguishable** from them at the test-suite level:

   - `LEGACY_AT_TEST` writes into `tests/acceptance/` (same path as `AT_RED_TEST`).
   - `LEGACY_CT_TEST` writes into `tests/contract/` (same path as `CT_RED_TEST`).
   - No annotation (`@LegacyCoverage`), no filename suffix (`*_LegacyTest`), no separate directory (`tests/legacy/`), no mix.

   **Why.** The legacy cycle is an *authoring path*, not a runtime distinction. Once the test is authored and the cycle's verify gate clears (see Item 1b: `VERIFY_LEGACY_AT` / `VERIFY_LEGACY_CT`), the test is a normal member of the suite. The "inverted-RED expected-to-pass" property only applies once, at authoring time, inside the legacy cycle's own BPMN — never at any subsequent CI run. Downstream consumers (CI, code review, the test runner) treat legacy and change-cycle tests identically because they *are* identical at that layer.

   This decision is captured as durable user feedback in memory: `feedback_legacy_tests_no_marker.md`.

   **Knock-on for the BPMN failing-legacy detector** (the consumer that originally motivated this item): `plans/deferred/20260518-1144-atdd-bpmn-orchestration.md` item 7 either retires entirely (no marker = no detector — a failing test is a failing test, handled uniformly regardless of authoring origin) or keys on a non-marker signal (e.g. an authoring-time ledger that records which cycle authored which test id, stored outside the test files themselves). That decision belongs to the deferred plan when it reactivates — it is **not** in scope here, and Item 2 no longer blocks it.

> **Renumber pass 2026-05-20:** Original Item 3 (AT-side updates to `atdd-at-cycle.md`) deleted during the refine walk — legacy runs strictly upstream of AT, so the boundary-statement and failing-legacy-guardrail sub-bullets both collapse (boundary has no peer to declare; AT-RED-* never special-cases legacy, and `disable-tests.md:40` already covers "don't disable tests you didn't author"). Original Item 4 (BPMN failing-legacy detector cross-plan pointer) deleted and folded into Item 2's "Consumer pointer" block. After renumbering, only items 1 and 2 remain.

## Out of scope

- `internal/assets/runtime/shared/scope.md` doctrine prose edits — Item 2's "same paths as change cycle" decision means no new layer category is needed, so `scope.md` stays untouched. (The per-phase rows in `phase-scopes.yaml` *are* in scope as a 1a knock-on.)
- Implementing the BPMN failing-legacy detector — orchestration code lives in [BPMN orchestration plan (deferred)](deferred/20260518-1144-atdd-bpmn-orchestration.md) item 7. After Item 2's "no marker" decision (2026-05-20), item 7 is no longer blocked on this plan; the deferred plan owns its own retire-or-rekey decision.
- Structural cycle definition and dispatcher/router — separate plan; the bpmn-orchestration plan's residual scope handles the dispatcher side.

## Open questions

*(All open questions resolved during the 2026-05-20 refine walk. Resolutions folded into Scope items above.)*

> **Refined 2026-05-20 walk:**
> - Marker convention → Item 2 (no marker; legacy tests live alongside AT/CT tests).
> - Legacy contract tests (same cycle vs. own sub-cycle) → Item 1c (own sub-cycle, `legacy_ct`, parallel to `legacy_at`).
> - **Scope-policy row resolved.** Each of the seven new phases (Item 1a) gets its own row in `internal/atdd/phase-scopes.yaml` — required by memory `feedback_no_deferred_mechanism.md` (every writing-agent phase must have its scope explicitly pinned, no aliasing or inheritance). The row *values* reuse the change-cycle counterpart's canonical layer keys verbatim because Item 2 pinned legacy tests to the same paths as change-cycle tests:
>   - `LEGACY_AT_TEST` ↔ same layers as `AT_RED_TEST`.
>   - `LEGACY_AT_DSL` ↔ same layers as `AT_RED_DSL`.
>   - `LEGACY_AT_SYSTEM_DRIVER` ↔ same layers as `AT_RED_SYSTEM_DRIVER`.
>   - `LEGACY_CT_TEST` ↔ same layers as `CT_RED_TEST`.
>   - `LEGACY_CT_DSL` ↔ same layers as `CT_RED_DSL`.
>   - `LEGACY_CT_EXTERNAL_SYSTEM_DRIVER` ↔ same layers as `CT_RED_EXTERNAL_SYSTEM_DRIVER`.
>   - `LEGACY_CT_EXTERNAL_SYSTEM_STUB` ↔ same layers as `CT_GREEN_EXTERNAL_SYSTEM_STUB`.
>   `scope.md` doctrine prose needs no edit — the same-paths-as-change-cycle property requires no new doctrinal layer.
