# Plan: Consolidate refactor-chooser fix — drop `CHOOSE_REFACTOR_TYPE` and retire `auto-default:`

## Context

This plan supersedes and consolidates two earlier plans that have been deleted from `plans/` in the same change as this file landing. The git history captures both in full; the salient state of each at consolidation time:

- **`20260528-1150-auto-default-on-loopable-choosers`** — paused. Introduced a per-node `auto-default:` schema to fix the `--auto`-mode hang at `CHOOSE_REFACTOR_TYPE`. Seam A (schema + parse-time/bind-time validation + YAML annotation) was committed in `e737f0a`, but dispatcher wiring (its Items 3, 4, 6, 8) was never done.
- **`20260528-1200-drop-choose-refactor-type-user-task`** — structural fix. Deletes `CHOOSE_REFACTOR_TYPE` and starts `refactor` at the gateway. Its Item 3 (delete the now-dead `auto-default:` schema) was left "deferred pending discussion."

Both plans target the same symptom from two angles:

- **Interactive mode:** `CHOOSE_REFACTOR_TYPE` prints a `Approve? [y/n]` STOP banner *before* `GATE_REFACTOR_TYPE_CHOICE` asks the real menu. Two prompts for one decision; answering `n` to the meaningless first aborts the whole ticket (this morning's rehearsal symptom).
- **Autonomous mode (`--auto`):** the same STOP banner halts forever waiting for stdin because `CategoryHuman` cannot be auto-confirmed.

Decision (2026-05-28, after reviewing both plans): take 1200's structural fix and finish 1150's now-dead Seam A by deleting it. The `auto-default:` schema currently in HEAD has no live consumer once `CHOOSE_REFACTOR_TYPE` is gone, and per the "schema fields must earn their slot" doctrine we drop it rather than keeping it dormant. If a second loopable chooser appears later that genuinely needs the same opt-in, a fresh plan can re-add it; nothing here forecloses that.

### Behaviour after this plan lands

- **Interactive (`gh optivem rehearsal`, `gh optivem implement` without `--auto`):** operator hits `GATE_REFACTOR_TYPE_CHOICE` directly. One prompt — the menu from the binding (`gates/bindings.go:371-391`). Empty/Enter → `none` → loop exits.
- **Autonomous (`gh optivem implement --auto`):** closed-stdin `Prompter.Ask` returns empty, binding defaults to `"none"`, gateway routes to `REFACTOR_TOP_END`. Opportunistic refactor that iteration: **skipped (none)** — same outcome 1150 was engineering, achieved structurally.
- The autonomous trace will not carry a 1150-style `[auto-default] refactor-type-choice = none` breadcrumb. The gateway trace line (`> GATE_REFACTOR_TYPE_CHOICE kind=gateway`) is the audit signal.

### Companion fix folded in (unchanged from plan 1200)

`GATE_REFACTOR_TYPE_CHOICE` has five outgoing edges in the YAML, including `REDESIGN_EXTERNAL_SYSTEM_STRUCTURE` (`process-flow.yaml:423`), but the binding (`gates/bindings.go:375`, `383-388`) and the prompt-iteration test (`bindings_test.go:522`) only list four. An operator who knows the YAML and types `redesign-external-system-structure` gets `"refactor-type-choice: unrecognised value"`. Same area of code; bundled here.

## Items

1. **Delete `CHOOSE_REFACTOR_TYPE` from `process-flow.yaml` and re-point flows.**
   - File: `internal/atdd/runtime/statemachine/process-flow.yaml`.
   - Delete lines 362-388 — the 1150-added doc comment block (362-380) and the `- id: CHOOSE_REFACTOR_TYPE` node with its `auto-default:` block (381-387) plus the trailing blank line (388).
   - Change `start: CHOOSE_REFACTOR_TYPE` (line 360) to `start: GATE_REFACTOR_TYPE_CHOICE`.
   - In `sequence-flows:` (lines 418-428):
     - Delete `{from: CHOOSE_REFACTOR_TYPE, to: GATE_REFACTOR_TYPE_CHOICE}` (line 419).
     - Rewrite the four loopback edges (lines 425-428) to target `GATE_REFACTOR_TYPE_CHOICE` instead of `CHOOSE_REFACTOR_TYPE`.

2. **Add the missing `redesign-external-system-structure` case to the `refactor-type-choice` binding.**
   - File: `internal/atdd/runtime/gates/bindings.go`.
     - Line 375: extend the prompt string to list `redesign-external-system-structure` between `redesign-system-structure` and `none`.
     - Lines 383-388: add `"redesign-external-system-structure"` to the case list.
   - File: `internal/atdd/runtime/gates/bindings_test.go:522` — append `"redesign-external-system-structure"` to the `prompt/<ans>` table slice so the prompt sub-tests cover it.

3. **Remove the `auto-default:` schema from the loader.**
   - File: `internal/atdd/runtime/statemachine/load.go`.
     - Delete the `AutoDefault` field + its doc-comment block on `RawNode` (lines 56-67).
     - Delete the `RawAutoDefault` struct + its doc comment (lines 70-84).
     - Delete the `validateAutoDefault` call site (line 211) and the function + its doc comment (lines 306-334).
   - File: `internal/atdd/runtime/statemachine/run.go`.
     - Delete the `AutoDefault != nil` gate-binding cross-check block (lines 78-94) inside the `UserTask` branch of `Engine.resolve`. The surrounding bind loop continues unchanged.

4. **Remove the `auto-default:` tests.**
   - File: `internal/atdd/runtime/statemachine/load_test.go` — delete:
     - The `procWithNode` helper (lines 364-381) if no other test uses it (grep first; if other tests use it, keep it).
     - `TestLoadBytes_AutoDefault_ValidOnHumanUserTaskParses` (lines 383-408).
     - `TestLoadBytes_AutoDefault_ServiceTaskRejected` (lines 410-428).
     - `TestLoadBytes_AutoDefault_TemplatedAgentRejected` (lines 430-449).
     - `TestLoadBytes_AutoDefault_MissingBindingRejected` (lines 451-468).
     - `TestLoadBytes_AutoDefault_MissingValueRejected` (lines 470-486).
     - `TestLoadDefault_AutoDefault_ChooseRefactorTypeAnnotated` (lines 488-522) — including the trailing "no other node carries `auto-default:`" guard, since the schema no longer exists.
   - File: `internal/atdd/runtime/statemachine/run_test.go` — delete:
     - The `autoDefaultYAML` const (lines 1137-1157).
     - `TestBind_AutoDefault_KnownBindingResolves` (lines 1159-1177).
     - `TestBind_AutoDefault_UnknownBindingErrors` (lines 1179-1197).

5. **Audit for stragglers.**
   - Run `grep -rn 'AutoDefault\|auto-default\|RawAutoDefault\|validateAutoDefault' internal/ docs/` after Items 3-4.
   - Expected remaining hits: only inside the three plan files (`plans/20260528-1150-*.md`, `plans/20260528-1200-*.md`, this plan). If anything else surfaces — driver-side dispatcher hooks, schema reference docs, BPMN authoring docs — remove or update it. The field is fully retired.

## Verification

- `go test ./internal/atdd/runtime/...` passes (use `scripts/test.sh` or `-p 2` on Windows per project convention; never bare `go test ./...`).
- `grep -n CHOOSE_REFACTOR_TYPE internal/ docs/` returns hits only in autogenerated diagram files (`docs/process-diagram.md`, `docs/images/process-diagram-5-refactor.svg`) — those regenerate on push to main via the existing diagram workflow and need no manual edit here.
- `grep -rn 'AutoDefault\|auto-default' internal/ docs/` returns no hits in non-plan files.
- `gh optivem rehearsal` reaches the refactor step and shows exactly **one** prompt — the menu from the binding — with no `Approve? [y/n]` ahead of it. Typing empty / Enter exits the loop via the `none` branch; typing `redesign-external-system-structure` routes to that branch instead of erroring.
- `gh optivem implement --auto` against a fixture ticket that goes through `change-system-behavior` reaches `REFACTOR_OPPORTUNISTICALLY` and exits the refactor loop without hanging (closed-stdin reads return empty, binding defaults to `"none"`, gateway routes to `REFACTOR_TOP_END`).

## Out of scope (deliberately)

- Reverting commit `e737f0a` wholesale. That commit also carried unrelated BPMN-banner additions; unpicking it would unpick that work too. Items 3-4 remove the 1150-shaped lines forward in a new commit; net effect is the same.
- Diagram regeneration steps. The regenerate-diagram GH Actions workflow handles `docs/process-diagram.md` and `docs/images/*.svg` on push to main; local regen here races it.
- Any change to `newHumanStopDispatcher` or `humanStop` (registry). The `agent: human` invariant stays exactly as it is for the remaining human-STOP sites and for `ASK_HUMAN` inside the `approve` primitive.
- Any change to `--auto` / `--confirm` semantics. The structural fix removes the need for a per-node exemption, so the operator-side flag surface is untouched.
- Renaming or reshaping `GATE_REFACTOR_TYPE_CHOICE`. Its name already reads as a chooser ("Refactor Type?"); no relabeling needed.
- Re-introducing a generic per-node "autonomous default" mechanism. If a second loopable chooser surfaces later, write a fresh plan with that node as the concrete use case; do not pre-emptively keep `auto-default:` alive on a single hypothetical future consumer.
