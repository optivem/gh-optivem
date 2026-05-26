# BPMN process review — feedback capture

> ✅ **Items 1, 6, 7 landed (2026-05-26).** Main-process start/end label
> renames (`Runtime Bootstrap` → `Ticket Ready`, `Runtime Complete` →
> `Ticket In Acceptance`, `Ticket Marked READY` → `Ticket Ready`) plus
> `tdd-stage:` annotations completing the red-green-REFACTOR coloring
> across `change-system-behavior` (REFACTOR), `redesign-system-structure`
> (RED + GREEN), `redesign-external-system-structure` (RED + GREEN), and
> `refactor-test-structure` (REFACTOR). `cover-system-behavior` audit
> confirmed already-annotated. Diagram + SVGs regenerated.
>
> **Remaining:** Items 2, 4, 5, 8 — all touch runtime code.

Captures feedback from the user's review of the BPMN process (`internal/atdd/runtime/statemachine/process-flow.yaml` and adjacent state-machine wiring / agent prompts).

## Context

User is walking through the BPMN process and surfacing observations as they read. This plan is the running log; items will be added as feedback arrives. Each item should be self-contained enough to be picked up later via `/execute-plan` or refined via `/refine-plan`.

## Items

### 1. Rename main process start/end events to mirror ticket state

**Observation.** "Runtime Bootstrap" / "Runtime Complete" are implementation jargon; BPMN events are named for the triggering event/resulting state, not the init/teardown machinery. The main process consumes a READY ticket and produces an IN-ACCEPTANCE ticket, so naming both endpoints after the ticket state gives the diagram a coherent vocabulary:

- **Start**: `Runtime Bootstrap` → `Ticket Ready` (mirrors what `refine-ticket` produces).
- **End**: `Runtime Complete` → `Ticket In Acceptance` (mirrors what `implement-ticket` produces — the subprocess marks IN ACCEPTANCE just before END).

Slight deviation from strict BPMN past-tense convention ("Ticket Marked Ready" / "Ticket Marked In Acceptance"), but state-naming reads more directly for the student audience and stays consistent across both endpoints.

**Action.**
- `internal/atdd/runtime/statemachine/process-flow.yaml`:
  - `START.documentation`: `"Runtime Bootstrap"` → `"Ticket Ready"`
  - `END.documentation`: `"Runtime Complete"` → `"Ticket In Acceptance"`
  - `REFINE_TICKET_END.documentation`: `"Ticket Marked READY"` → `"Ticket Ready"` (align with the main-process start)
- `docs/process-diagram.md`: update the `((Runtime Bootstrap))`, `((Runtime Complete))`, and `Ticket Marked READY` labels to match.
- Regenerate the SVGs under `docs/images/` for the affected diagrams.

**Files touched.** `internal/atdd/runtime/statemachine/process-flow.yaml`, `docs/process-diagram.md`, `docs/images/process-diagram-2-main.svg`, `docs/images/process-diagram-*-refine-*.svg` (verify exact filenames at execute time).

**Depends on.** Independent of item 2 (the rename is correct either way).

---

### 2. Remove Board mode — single-entry main process

**Observation.** The main process currently has two entry flows from `START`: `mode == board` → `PICK_TOP_READY` → `IMPLEMENT_TICKET`, and `mode == specific-issue` → `IMPLEMENT_TICKET`. Board mode hides which ticket got picked and why; for a teaching tool the explicit per-issue invocation is more honest. Dropping it collapses the start to a single flow, removes a service task, and removes the `pick-top-ready` action and the "pick top READY" contract from both tracker backends.

**Action.** Drop Board mode end-to-end:
1. `process-flow.yaml`: remove `PICK_TOP_READY` node; remove both `when:` flows from `START`; leave a single `START → IMPLEMENT_TICKET` edge.
2. `docs/process-diagram.md` + regenerated SVGs: same.
3. Remove the `pick-top-ready` action from `internal/atdd/runtime/actions/` (registry, bindings, tests).
4. Remove `mode` selection from the driver (`internal/atdd/runtime/driver/`), `clauderun` (`internal/atdd/runtime/clauderun/`), and the CLI flag (`implement_commands.go` / `main.go`).
5. Tracker: drop the "pick top READY" method on both `internal/atdd/runtime/tracker/markdown/` and `internal/atdd/runtime/tracker/github/` backends, and update `tracker.go` interface.
6. Update tests: `bindings_test.go`, `driver_test.go`, `clauderun_test.go`, `transitions_test.go`, `embedded_smoke_test.go`, `preflight_test.go` (any `mode == board` assertions).
7. Update `README.md` and `CONTRIBUTING.md` references to Board mode.

**Files touched.** ~15+ files across `internal/atdd/runtime/{statemachine,actions,driver,clauderun,tracker,preflight,gates,trace}/`, `docs/`, root CLI files, and tests. Full list to be enumerated at execute time.

**Open questions.**
- Is anyone (a workflow, a teacher's local script) invoking the tool in Board mode today? If yes, this is a breaking change worth announcing.
- After removal, do we want the CLI to accept just `<issue>` positionally (since there's no `--mode` to disambiguate), or keep the existing flag layout?

**Depends on.** Independent of item 1, but if both land, item 1's rename rationale strengthens (single entry → cleaner pairing with `refine-ticket` end).

---

---

### 3. Fix `— see §` suffix inconsistency across sibling call-activities

**Observation.** In the Refactor and Implement-Ticket diagrams, sibling call-activity nodes render inconsistently: `REDESIGN_EXTERNAL_SYSTEM_STRUCTURE` shows `— see § redesign-external-system-structure` while its three siblings (`REDESIGN_SYSTEM_STRUCTURE`, `REFACTOR_SYSTEM_STRUCTURE`, `REFACTOR_TEST_STRUCTURE`) don't. Reads as a bug; it's actually a label-spelling mismatch.

**Root cause.** `internal/atdd/runtime/diagram/diagram.go:414-427` drops the `— see §` suffix when the call-activity's `documentation:` matches the auto-Title-Case of the target process ID. Documentation labels use the hyphenated compound `External-System`, but auto-Title-Case of `external-system` produces `External System` (no hyphen). The mismatch keeps the suffix on every `External-System` label and drops it from every sibling without a hyphenated compound.

Affected labels (currently inconsistent):
- `REDESIGN_EXTERNAL_SYSTEM_STRUCTURE` documentation
- `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS` documentation
- `IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS` documentation
- Any other call-activity documentation containing `External-System` (audit at execute time)

**Action — two options, pick one:**

- **Option A (smaller change): drop the hyphen in documentation labels.** Change every `External-System` in `documentation:` strings to `External System`. Pros: minimal, aligns with auto-Title-Case, no renderer change. Cons: loses the compound-noun hyphenation that's arguably more correct.
- **Option B (preserve compound spelling): teach the renderer to hyphenate `external-system` when Title-Casing.** Special-case the substring (or generalise: treat multi-word compounds the same way). Pros: keeps the `External-System` spelling. Cons: more renderer complexity, hard to generalise without a list of compound nouns.

**Files touched (Option A).** `internal/atdd/runtime/statemachine/process-flow.yaml` (all `documentation:` strings containing `External-System`), `docs/process-diagram.md` (regenerated), `docs/images/*.svg` (regenerated).

**Files touched (Option B).** `internal/atdd/runtime/diagram/diagram.go` (auto-Title-Case helper), plus a renderer test, plus regenerated `docs/process-diagram.md` + SVGs.

**Open questions.**
- Option A vs Option B — which to take? Default lean: Option A.
- Are there other compound-noun labels (e.g. `In-Acceptance`, `Driver-Adapter`) hitting the same issue? Audit at execute time.

**Depends on.** Independent.

---

---

### 4. Make `name:` explicit everywhere — kill auto-Title-Case and aliases

**Observation.** Two problems are really one:

1. **Field name is BPMN-wrong.** Our YAML uses `documentation:` to carry the short visible diagram label, but in BPMN 2.0 vocabulary that field is **`name`**. BPMN's `<documentation>` element is reserved for longer prose (tooltip / hover text / generated reference docs). A BPMN-literate reader would expect `documentation:` to hold a sentence-or-paragraph description, not a one-or-two-word label.
2. **Half-explicit / half-auto is inconsistent.** The renderer auto-Title-Cases the kebab-case process ID to derive (a) diagram section headings and (b) the heading-form used to decide whether to drop the `— see § …` suffix on call-activities. Empirical scan of `process-flow.yaml` (2026-05-26): only ~1/3 of current labels match the auto-Title-Case of their ID. The other ~2/3 deliberately differ — past-tense end-event outcomes, call-site role relabeling, sentence-case user-task labels, gateway labels naming the binding, ALL-CAPS state names, parenthetical clarifiers, interpolated prompts, templated targets. Auto-deriving everywhere would destroy that information; explicit-everywhere preserves it.

| Our YAML | BPMN equivalent | Actual role today |
|---|---|---|
| `id:` | `id` ✓ | Element ID |
| `documentation:` | **`name`** (misnamed) | Visible diagram label |
| _(none)_ | `<documentation>` | Long-form prose (no slot today; out of scope here) |

**Action — explicit `name:` everywhere; no auto-derivation, no aliases:**

1. **Rename field.** `documentation:` → `name:` everywhere in `internal/atdd/runtime/statemachine/process-flow.yaml`. Mechanical find-and-replace; all values stay identical.
2. **Struct + tag rename.** `internal/atdd/runtime/statemachine/types.go`: `Documentation` → `Name`, YAML tag `documentation` → `name`.
3. **Add process-level `name:`.** Every process definition (~30) gets a `name:` field giving the human-readable heading text. Examples: `main:` → `name: "Main"`, `implement-ticket:` → `name: "Implement Ticket"`, `redesign-external-system-structure:` → `name: "Redesign External-System Structure"`. Author-controlled — no kebab-to-Title machinery.
4. **Fill in the ~6 service-task gaps.** Nodes currently falling back to ID get an explicit `name:`:
   - `MARK_IN_PROGRESS`, `MARK_IN_REFINEMENT`, `MARK_READY`, `MARK_IN_ACCEPTANCE` — pick canonical labels (likely `"Mark IN PROGRESS"`, `"Mark IN REFINEMENT"`, `"Mark READY"`, `"Mark IN ACCEPTANCE"` to mirror the ALL-CAPS state convention already used in end events).
   - `PARSE_TICKET` — `"Parse Ticket"`.
   - `CHECK_CHECKLIST_PROGRESS` — `"Check Checklist Progress"`.
   - Audit at execute time for any other ID-fallback nodes missed by the grep.
5. **Schema validation.** `internal/atdd/runtime/statemachine/load.go`: require `name:` on **every** node (not just call-activity/start-event/end-event/error-end-event) and on **every** process. No fallback to ID; missing `name:` is a load-time error.
6. **Renderer simplification.** `internal/atdd/runtime/diagram/diagram.go`:
   - Drop `autoTitleCase` entirely.
   - Drop the `processAlias` map (it only exists to override the auto-derived form, which no longer exists).
   - Section headings come from `process.Name` directly.
   - The `— see § …` suffix rule collapses to trivial equality: `node.Name != targetProcess.Name → emit suffix`.
7. **Suffix link text.** The `see § <link>` link text should be the **process ID** (kebab-case), not the new process `name:`, because GitHub Markdown heading anchors are derived from the heading text but the kebab-case ID is also a stable, unambiguous reference. Open question — confirm at execute time which renders correctly in GitHub.
8. **Tests.** Update any test constructing nodes with `Documentation:` or asserting field-name; add tests for the new "missing `name:`" load-time error.
9. **Regenerate** `docs/process-diagram.md` + SVGs. Expected diff: zero label changes (all current labels preserved), but the `— see § …` suffix should now drop for `Redesign External-System Structure` (item 3 dissolves into this item — the renderer no longer has an auto-derived heading to clash with).

**Files touched.** `internal/atdd/runtime/statemachine/{process-flow.yaml,types.go,load.go}`, `internal/atdd/runtime/diagram/diagram.go` + its tests, `docs/process-diagram.md` + SVGs, any other `internal/atdd/runtime/**` consumer that reads the field (audit at execute time — likely `trace/`, `driver/`, possibly `clauderun/`).

**Open questions.**
- Back-compat alias for `documentation:`? Per `feedback_teaching_repo_no_legacy.md` — no, hard rename.
- Suffix link text: process ID vs process name? (Step 7 above.)
- Canonical labels for the 6 ID-fallback service-tasks — confirm at execute time.

**Supersedes / absorbs.** Item 3 (External-System hyphen mismatch) — once the renderer compares two explicit `name:` strings instead of label-vs-auto-derived-heading, the hyphenated `External-System` label simply matches its process-level `name:` of the same spelling, suffix drops naturally. Item 3 can be deleted from the plan once this lands.

**Depends on.** Independent of items 1 and 2. Items 1–2 should use the renamed `name:` field. Recommend executing item 4 **first** because everything else benefits from the renamed/cleaned schema.

---

---

### 5. Remove `onboard-external-system` from BPMN (stash for later)

**Observation.** The `onboard-external-system` subprocess is four `agent: human` placeholder steps from an earlier brainstorm. The shape hasn't been thought through (which steps should be agents, what outputs each produces, whether the CHECKLIST_PROGRESS prefix even fits a one-shot ceremony, etc.). Better to remove the placeholder and redesign cleanly later than carry it forward.

**Action.** Delete from `process-flow.yaml`:
1. The entire `onboard-external-system:` subprocess block (~lines 411-464).
2. The `ONBOARD_EXTERNAL_SYSTEM` call-activity node under `implement-ticket.nodes`.
3. The `GATE_TASK_SUBTYPE → ONBOARD_EXTERNAL_SYSTEM` edge (`when: "task-subtype == external-system-onboarding"`).
4. The convergence edge `ONBOARD_EXTERNAL_SYSTEM → MARK_IN_ACCEPTANCE`.
5. The `external-system-onboarding → onboard-external-system` row in the `implement-ticket` block-comment gateway-lookup table (~line 213).

Also remove related code-side references (audit via grep `onboard-external-system|ONBOARD_EXTERNAL_SYSTEM|external-system-onboarding`):
- `internal/atdd/runtime/actions/bindings.go`, `gates/bindings.go`, `gates/bindings_test.go`, `clauderun/clauderun.go`, `clauderun/clauderun_test.go`, `statemachine/transitions_test.go`.
- Delete `docs/images/process-diagram-7-onboard-external-system.svg` (regenerated docs won't reference it).

Regenerate `docs/process-diagram.md` + remaining SVGs.

**Verify before deleting.** Run the statemachine test suite to confirm no fixture still expects the `external-system-onboarding` route — per `feedback_statemachine_test_loop_hazard.md`, audit fixtures first; deleting an edge without an explicit catch-all path on the gateway is fine here because `UNKNOWN_TASK_SUBTYPE` already absorbs unknown subtypes.

**Files touched.** Same set as item 2 (Board-mode removal) — multi-package change across `internal/atdd/runtime/{statemachine,actions,gates,clauderun}/`, docs, SVGs. Full list to be enumerated at execute time.

**Future rebuild scope.** Captured in `plans/20260526-1746-rebuild-onboard-external-system.md` — full YAML block, all call-site wiring, code touch-points, and open design questions for the redesign. Do not pick that plan up until the new design is ready.

**Depends on.** Independent of items 1, 2, 4. Should execute on the schema produced by item 4 (use `name:` instead of `documentation:` when writing the spinoff document's example YAML — update the spinoff at execute time).

---

### 6. Add `tdd-stage: refactor` to `REFACTOR_OPPORTUNISTICALLY` (and audit for siblings)

**Observation.** In `change-system-behavior`, the YAML comment at `process-flow.yaml:466` explicitly calls the shape "Classic red-green-REFACTOR triad". The first two nodes carry the meta annotation:

```yaml
- id: WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL
  tdd-stage: red
- id: IMPLEMENT_AND_VERIFY_SYSTEM
  tdd-stage: green
- id: REFACTOR_OPPORTUNISTICALLY
  # ← missing `tdd-stage: refactor`
```

The renderer's blue REFACTOR border classDef already exists (`diagram.go`, legend line 17 in `docs/process-diagram.md`); it just isn't being triggered because the YAML didn't annotate the third member. Pure missing-data bug.

**Action.**
1. Add `tdd-stage: refactor` to the `REFACTOR_OPPORTUNISTICALLY` node in `change-system-behavior`.
2. **Audit for similar omissions across all subprocesses.** Likely candidates worth checking:
   - `refactor-test-structure`'s `REFACTOR_AND_VERIFY_TESTS` node — does it carry `tdd-stage: refactor`? (`refactor-system-structure`'s `IMPLEMENT_AND_VERIFY_SYSTEM` already does — see diagram line 289.)
   - Anywhere else the rendered diagram shows a refactor-stage step without a blue border.
3. Regenerate `docs/process-diagram.md` + affected SVGs.

**Files touched.** `internal/atdd/runtime/statemachine/process-flow.yaml`, `docs/process-diagram.md`, `docs/images/process-diagram-*.svg` (affected ones only).

**Depends on.** Independent. Cheap, mechanical.

---

---

### 7. Add `tdd-stage: red` / `tdd-stage: green` to redesign cycles

**Observation.** Both `redesign-system-structure` and `redesign-external-system-structure` follow a RED→GREEN shape: reshape the (system-side or external-side) driver adapters → re-build/re-verify the system through `implement-and-verify-system` with `action: update-system`. The first step puts tests in a failing state (port surface has shifted under them); the second step makes them pass again. Today these nodes carry no `tdd-stage:` annotation, so the diagram shows them with neutral borders even though they semantically play RED and GREEN roles.

(Pairs with item 6, which adds the missing REFACTOR annotation to `change-system-behavior`. Together items 6 + 7 give all the change-shaped cycles a complete red-green-REFACTOR or red-green stage coloring.)

**Note on rendered-diagram drift.** The current `docs/process-diagram.md` still shows the pre-verb-split labels (`IMPLEMENT_SYSTEM_DRIVER_ADAPTERS` / `Implement System`) for `redesign-system-structure`. The YAML actually says `UPDATE_SYSTEM_DRIVER_ADAPTERS` / `Update System`. Regeneration at execute time will resync them — no separate item needed.

**Action.**
1. In `redesign-system-structure`:
   - Add `tdd-stage: red` to `UPDATE_SYSTEM_DRIVER_ADAPTERS`.
   - Add `tdd-stage: green` to `IMPLEMENT_AND_VERIFY_SYSTEM` (the one with `params.action: update-system`).
2. In `redesign-external-system-structure`:
   - Add `tdd-stage: red` to `UPDATE_EXTERNAL_SYSTEM_DRIVER_ADAPTERS`.
   - Add `tdd-stage: green` to `IMPLEMENT_AND_VERIFY_SYSTEM` (the one with `params.action: update-system`).
3. Regenerate `docs/process-diagram.md` + affected SVGs (will also pick up the stale-label resync).

**Also confirmed (user-flagged 2026-05-26):**
- In `refactor-test-structure`: add `tdd-stage: refactor` to `REFACTOR_AND_VERIFY_TESTS` (blue REFACTOR border).

**Audit at execute time.** Sweep the remaining CYCLE-layer subprocesses for similarly missing TDD-stage annotations. `cover-system-behavior` has only `WRITE_AND_VERIFY_ACCEPTANCE_TESTS_PASS` (single GREEN step, likely needs `tdd-stage: green`). Confirm and annotate as needed.

**Files touched.** `internal/atdd/runtime/statemachine/process-flow.yaml`, `docs/process-diagram.md`, `docs/images/process-diagram-*.svg`.

**Depends on.** Independent of items 1–5. Logically pairs with item 6 — could be committed together as one "TDD-stage annotation completeness sweep" if execution is sequential.

---

---

### 8. Cut checklist progress-tracking machinery (stash for later)

**Observation.** The `CHECK_CHECKLIST_PROGRESS` + `GATE_CHECKLIST_PARTIALLY_DONE` + `STOP_CHECKLIST_PARTIALLY_DONE` triad is copy-pasted into four CYCLEs (`redesign-system-structure`, `redesign-external-system-structure`, `refactor-system-structure`, `refactor-test-structure`). The matching post-CYCLE auto-tick (`Tracker.MarkChecklistComplete` inside `move-to-in-acceptance`) runs at the end of every ticket. Both exist to support partial-progress resumption — but the agent today completes tickets **atomically** (commits at the end of the CYCLE, not per-item), so the resume gate only ever fires on already-completed tickets (operationally noise), and the auto-tick is symbolic record-keeping, not real progress tracking.

Per the "all or none" framing: drop the machinery. Keep the `Checklist` section as **spec/input** (the agent reads it to know what to do); the BPMN treats it as opaque.

**Action.** Cut from BPMN:
1. The four duplicated `CHECK_CHECKLIST_PROGRESS` + `GATE_CHECKLIST_PARTIALLY_DONE` + `STOP_CHECKLIST_PARTIALLY_DONE` triads (in `redesign-system-structure`, `redesign-external-system-structure`, `refactor-system-structure`, `refactor-test-structure`).
2. Their sequence-flows.
3. Each affected CYCLE's `start:` field updates to point at the now-first real step (e.g. `UPDATE_SYSTEM_DRIVER_ADAPTERS` instead of `CHECK_CHECKLIST_PROGRESS`).

Cut from code:
4. The `MarkChecklistComplete` call inside `moveToInAcceptance` (`internal/atdd/runtime/actions/bindings.go:349-351` — keep the `SetStatus` half).
5. The `check-checklist-progress` action registration in `actions/bindings.go`.
6. The `checklist-partially-done` gate binding in `gates/bindings.go`.
7. The `MarkChecklistComplete` method from the `Tracker` interface (`tracker/tracker.go`) and both implementations (`tracker/markdown/markdown.go`, `tracker/github/github.go`).
8. All related tests (`actions/bindings_test.go`, `gates/bindings_test.go`, tracker backend tests, `statemachine/transitions_test.go` fixtures for checklist-partially-done routes).

Keep (still useful as spec input):
- `PARSE_TICKET` parsing of the `Checklist` section (`internal/atdd/runtime/intake/parse.go`, `sections.go`).
- The `ticket_checklist` ctx-state stash.
- The `Checklist` section in the ticket body shape.

Regenerate `docs/process-diagram.md` + affected SVGs.

**Verify before deleting.** Per `feedback_statemachine_test_loop_hazard.md`, audit `transitions_test.go` fixtures first — deleting gates without updating fixtures can deadlock the test suite.

**Future rebuild scope.** Captured in `plans/20260526-1754-rebuild-checklist-progress-tracking.md` — full YAML snippets, all code touch-points, and open design questions for re-introduction (per-CYCLE vs hoisted, per-task-kind guard, per-item-commit semantics, auto-tick semantics). Do not pick that plan up until atomicity actually breaks.

**Files touched.** Multi-package change across `internal/atdd/runtime/{statemachine,actions,gates,tracker}/`, docs, SVGs. Full list to be enumerated at execute time.

**Supersedes.** This item replaces the earlier proposal to *hoist* the checklist gate to `implement-ticket` (the original framing of item 8 during the discussion). Cut-and-stash is cleaner than centralize-and-keep given the "all or none" agent semantics.

**Depends on.** Independent of items 1, 2, 3, 6, 7. Should execute on the schema produced by item 4 (use `name:` instead of `documentation:` when writing the spinoff document's example YAML — update the spinoff at execute time).

---

## Open questions

- See item 2 above (Board-mode usage, CLI shape).
- See item 4 above (suffix link text: name vs ID; canonical labels for ID-fallback service-tasks).
- See item 5 above (none — spinoff plan owns the redesign questions).
- See item 6 above (audit for other missing `tdd-stage` annotations).
- See item 7 above (audit `cover-system-behavior` for similar missing annotations; `refactor-test-structure`'s `REFACTOR_AND_VERIFY_TESTS` already confirmed).
- See item 8 above (none — spinoff plan owns the re-introduction questions).
