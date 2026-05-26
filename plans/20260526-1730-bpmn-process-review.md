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
> ✅ **Item 5 landed (2026-05-26).** `onboard-external-system` CYCLE
> removed from BPMN (subprocess block + call-activity + edges) and from
> the process-order lists in `diagram.go` + `transitions_test.go`. Code-
> side cleanup of `gates/bindings.go` subtypes slice + test case still
> pending — deferred behind item 8's concurrent edits to the same file.
> Rebuild scope captured in `plans/upcoming/20260526-1746-rebuild-onboard-external-system.md`.
>
> ✅ **Item 4 landed (2026-05-26).** Schema rename `documentation:` →
> `name:` everywhere in `process-flow.yaml` (183 nodes) + new
> process-level `name:` on all 52 processes. `RawNode.Documentation` →
> `RawNode.Name`, `Process.Name` repurposed as display name with
> `Process.ID` holding the kebab map key. `load.go` requires non-empty
> `name:` on every non-gateway node and every process (gateways may be
> silent — Item 13). Renderer simplified: `processAlias` map dropped,
> auto-Title-Case dropped from process headings and call-activity
> suffix; suffix rule collapses to trivial equality
> (`node.Name != targetProcess.Name → emit "see § <id>"`). Item 3
> (External-System hyphen mismatch) absorbed and dissolved by the
> renderer simplification. Diagram regen handled by
> `regenerate-diagram.yml` workflow on push.
>
> **Remaining:** Items 2, 8 — both touch runtime code.

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

**Depends on.** Independent of items 1, 2, 6, 7. Item 4 (the `name:` schema) has landed, so the spinoff plan's example YAML should already use `name:` — update the spinoff at execute time if any `documentation:` remained.

---

## Open questions

- See item 2 above (Board-mode usage, CLI shape).
- See item 8 above (none — spinoff plan owns the re-introduction questions).
