# Plan: Drop `CHOOSE_REFACTOR_TYPE` user-task; start `refactor` at the gateway

## Context

In the `refactor` TOP process (`internal/atdd/runtime/statemachine/process-flow.yaml:358-428`), the BPMN shape is:

1. `CHOOSE_REFACTOR_TYPE` — `user-task` with `agent: human`, named `"Choose refactor type (loopable; none = exit)"`. The driver wraps every `agent: human` user-task with `newHumanStopDispatcher` (`driver/driver.go:646-647`), which is a generic STOP banner — it prints the node ID/description and asks `Approve? [y/n]`, capturing nothing.
2. `GATE_REFACTOR_TYPE_CHOICE` — `gateway` bound to `refactor-type-choice` (`bindings.go:371-391`). This binding is the one that **actually** asks the operator the menu: `"Refactor type? (refactor-system-structure | refactor-test-structure | redesign-system-structure | none) [none]: "`, then routes on the answer. Empty input defaults to `"none"`, which exits the loop cleanly.

So the operator's interactive experience today is:

```
[CHOOSE_REFACTOR_TYPE] Choose refactor type (loopable; none = exit)
  Approve? [y/n]: y
Refactor type? (refactor-system-structure | refactor-test-structure | redesign-system-structure | none) [none]:
```

Two prompts for one decision. Worse, answering `n` to the meaningless `Approve?` aborts the *entire* run — that's how the symptom showed up in this morning's rehearsal:

```
[trace 11:27:23] FAIL CHOOSE_REFACTOR_TYPE -> user aborted at CHOOSE_REFACTOR_TYPE
[trace 11:27:23] FAIL REFACTOR_OPPORTUNISTICALLY -> ...
[trace 11:27:23] FAIL CHANGE_SYSTEM_BEHAVIOR -> ...
ERROR: process "main" node "IMPLEMENT_TICKET": ... user aborted at CHOOSE_REFACTOR_TYPE
```

The operator was looking at a chooser banner, expected to be asked the choice, typed `n` (a valid menu response to "do you want to refactor?" in their head), and the runtime treated it as "abort the whole ticket."

**Goal.** Delete `CHOOSE_REFACTOR_TYPE`. Start the `refactor` process directly at `GATE_REFACTOR_TYPE_CHOICE` and re-point the four refactor-CYCLE loopback edges at the gateway. The gateway binding already does the right thing in both modes — prompts in interactive mode, defaults to `"none"` on empty input under `--auto` (because `Prompter.Ask` returns empty on closed stdin) — so this is a pure structural cleanup.

This supersedes [`20260528-1150-auto-default-on-loopable-choosers.md`](20260528-1150-auto-default-on-loopable-choosers.md), which targeted the same symptom in `--auto` mode by introducing a per-node `auto-default:` opt-in. 1150's Seam A (the `RawAutoDefault` schema, `validateAutoDefault` parse-time check, and `run.go` gate-binding cross-check) was committed locally in `7bf8f9e` alongside an unrelated BPMN-banner commit, and the YAML node now carries the `auto-default:` block. 1200 removes both the mechanism and the node — under the new structure, the gateway is the chooser, and the gateway's binding already defaults empty input to `"none"` (`bindings.go:380-382`), so no new mechanism is needed.

### Why this beats 1150's `auto-default:` design

1150 explicitly rejected the "drop the user-task" option with this objection:

> "Removing the user-task entirely would mean the gateway binding's prompter (`gates/bindings.go:375`) becomes the operator-facing chooser. … in interactive mode the operator loses the labeled chooser node in the diagram and trace — the gateway prompt is less discoverable and shows up as a one-shot prompt instead of a loopable menu."

That objection holds only if the user-task banner is materially more informative than the gateway's own trace line. It isn't:

- The trace already names every node it enters. `[trace] > GATE_REFACTOR_TYPE_CHOICE kind=gateway` is the same shape of breadcrumb as `[trace] > CHOOSE_REFACTOR_TYPE kind=user-task` — the loopable structure is visible from the *edges* (four loopback flows targeting the gateway), not from the node kind.
- The diagram renders the gateway with the four outgoing branches (one to each refactor sub-process, plus `none → REFACTOR_TOP_END`). That's the loopable menu, visualized.
- The user-task adds no information beyond its name. Its name is *currently* `"Choose refactor type (loopable; none = exit)"`, which is just narration of what the gateway is about to do.

1150 also fixes only autonomous mode. The interactive-mode redundancy (the operator's actual complaint) stays. 1200 fixes both.

### Companion fix: missing `redesign-external-system-structure` in the binding's prompt + switch

The `refactor` process has five branches at `GATE_REFACTOR_TYPE_CHOICE`:

- `refactor-system-structure` → REFACTOR_SYSTEM_STRUCTURE
- `refactor-test-structure` → REFACTOR_TEST_STRUCTURE
- `redesign-system-structure` → REDESIGN_SYSTEM_STRUCTURE
- `redesign-external-system-structure` → REDESIGN_EXTERNAL_SYSTEM_STRUCTURE
- `none` → REFACTOR_TOP_END

But the binding (`gates/bindings.go:375` and the switch at 383-388) only lists four — `redesign-external-system-structure` is missing from both the menu string and the accepted answers. An operator who knows the YAML and types the right answer gets `"refactor-type-choice: unrecognised value"`. Folded into this plan because it's the same area of code and the test that should have caught it (`bindings_test.go:522`) iterates over the same incomplete list.

## Items

1. **Delete `CHOOSE_REFACTOR_TYPE` from `process-flow.yaml`.**
   - File: `internal/atdd/runtime/statemachine/process-flow.yaml`.
   - Delete the node block at lines 362-387 (the 1150-added doc comment + the `- id: CHOOSE_REFACTOR_TYPE` node with its `auto-default:` block). Leave the surrounding nodes (`GATE_REFACTOR_TYPE_CHOICE`, the four call-activities, `REFACTOR_TOP_END`) untouched.
   - Change `start: CHOOSE_REFACTOR_TYPE` (line 360) to `start: GATE_REFACTOR_TYPE_CHOICE`.
   - In `sequence-flows:` (lines 418-428):
     - Delete the edge `{from: CHOOSE_REFACTOR_TYPE, to: GATE_REFACTOR_TYPE_CHOICE}` (line 419).
     - Rewrite the four loopback edges (lines 425-428) to target `GATE_REFACTOR_TYPE_CHOICE` instead of `CHOOSE_REFACTOR_TYPE`.

2. **Add the missing `redesign-external-system-structure` case to the `refactor-type-choice` binding.**
   - File: `internal/atdd/runtime/gates/bindings.go`.
   - Line 375: extend the prompt string to list `redesign-external-system-structure` between `redesign-system-structure` and `none`.
   - Lines 383-388: add `"redesign-external-system-structure"` to the case list.
   - File: `internal/atdd/runtime/gates/bindings_test.go:522` — append `"redesign-external-system-structure"` to the slice of valid answers iterated by the prompt sub-tests.

3. **Remove the now-dead `auto-default:` schema from the loader.** — ⏳ Deferred: needs separate discussion before executing.
   - **Status (2026-05-28).** Plan 1150's Seam A — exactly the code this item proposes to delete — was committed in `e737f0a` after this plan was authored. The deletion is still defensible under the structural-fix doctrine: once `CHOOSE_REFACTOR_TYPE` is gone (Item 1), no caller carries `auto-default:`, and the loader's `validateAutoDefault` check guards no live use. But: the schema may still earn its keep as a per-node opt-in for *other* future loopable choosers, in which case removing it here is premature. **Discuss separately before executing Item 3.** Items 1, 2, 4 are unaffected and can proceed.
   - File: `internal/atdd/runtime/statemachine/load.go`.
     - Delete the `AutoDefault` field on `RawNode` (line 67) and its doc comment block (lines 56-66).
     - Delete the `RawAutoDefault` struct + its doc comment (lines 70-83).
     - Delete the `validateAutoDefault` call site (line 211) and the function itself (lines 306-334).
   - File: `internal/atdd/runtime/statemachine/run.go`.
     - Delete the `AutoDefault != nil` gate-binding cross-check block (lines 78-93). The surrounding bind loop continues unchanged.
   - File: `internal/atdd/runtime/statemachine/load_test.go` — delete every test case added by 1150 for `auto-default:` validation (search for `AutoDefault` / `auto-default` / `validateAutoDefault` in this file and remove the corresponding `t.Run` / table entries).
   - File: `internal/atdd/runtime/statemachine/run_test.go` — same; delete every test for the bind-time cross-check.

4. **Audit for any other consumer of `auto-default:` and remove.**
   - `grep -rn 'AutoDefault\|auto-default' internal/ docs/` after Item 3. The only remaining hits should be inside this plan and 1150. If anything else surfaces (driver-side dispatcher branches, prompt docs, schema reference), drop it — the field is fully retired.

## Verification

- `go test ./internal/atdd/runtime/...` passes. (Use `scripts/test.sh` or `-p 2` on Windows per project convention.)
- `gh optivem rehearsal` reaches the refactor step and shows exactly **one** prompt — the menu from the binding — with no `Approve? [y/n]` ahead of it. Typing `n` is no longer a valid response; typing empty / Enter exits the loop via the `none` branch.
- `gh optivem implement --auto` reaches `REFACTOR_OPPORTUNISTICALLY` and exits the refactor loop without hanging (closed-stdin reads return empty, binding defaults to `"none"`).
- `grep -n CHOOSE_REFACTOR_TYPE internal/ docs/` returns hits only in autogenerated diagram files (`docs/process-diagram.md`, `docs/images/process-diagram-5-refactor.svg`) — those regenerate on push to main via the existing diagram workflow and need no manual edit.

## Out of scope (deliberately)

- Reverting commit `7bf8f9e` wholesale. That commit also carried the BPMN-banner additions (its actual intent per message body), and unpicking it would also unpick that unrelated work. Items 1-4 remove the 1150-shaped lines forward in a new commit; net effect is the same.
- Diagram regeneration steps. The auto-regen GH Actions workflow handles `docs/process-diagram.md` and `docs/images/*.svg` on push to main; local regen here races it.
- Any change to `newHumanStopDispatcher` or `humanStop` (registry). The `agent: human` invariant stays exactly as it is for the remaining human-STOP sites and for `ASK_HUMAN` inside the `approve` primitive.
- Any change to `--auto` / `--confirm` semantics. The structural fix removes the need for a per-node exemption, so the operator-side flag surface is untouched.
- Renaming or reshaping `GATE_REFACTOR_TYPE_CHOICE`. Its name already reads as a chooser ("Refactor Type?"); no relabeling needed.
