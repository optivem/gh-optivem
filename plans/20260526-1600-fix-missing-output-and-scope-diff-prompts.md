# Author `fix-missing-output.md` and `fix-scope-diff.md` prompts

## Origin / intent

`plans/20260526-1530-fix-recovery-wire-failure-kind.md` Item 4 verified
that the `execute-agent` → `fix` → `execute-agent` recovery branch
dispatches the correct task-name for both `validate-outputs-and-scopes`
failure modes:

- `missing-output` → `fix-missing-output`
- `scope-diff`    → `fix-scope-diff`

The dispatch wiring works
(`TestExecuteAgent_ValidationFailureDispatchesFixForFailureKind` pins it).
What's missing are the two prompt files. Without them, a real run that
hits a validation failure crashes the same way the `command-failed`
path used to:

```
FAIL RUN_AGENT -> dispatcher: load tuning for "fix-missing-output":
                  agents: no embedded prompt for "fix-missing-output"
```

## Why a separate plan

The 1530 plan absorbed the plumbing (`failure-kind` write, `ExpandParams`
state fallback, doc updates) plus one new prompt (`fix-command-failed.md`).
The two prompts here are authoring exercises, not plumbing — each
remediation prompt needs its own designed body (what should the agent
do for a scope-diff vs a missing-output failure?). That writing happens
on its own timetable.

## Scope

In scope:

- `internal/assets/runtime/prompts/atdd/fix-missing-output.md` — new
  prompt. Receives via state-fallback:
  - `${task-name}` — the writing-agent that produced incomplete outputs
    (the OUTER `execute-agent`'s task-name, captured before `fix`
    dispatch).
  - `${missing-outputs}` (or similar) — the comma-separated list of
    expected output keys the agent failed to emit. `validate-outputs-and-scopes`
    currently logs this to stderr only (`bindings.go:756-758`); to feed
    it into the prompt the action must also stash it in state under a
    fresh key.
- `internal/assets/runtime/prompts/atdd/fix-scope-diff.md` — new
  prompt. Receives:
  - `${task-name}` — the writing-agent whose diff violated scopes.
  - `${violating-paths}` — paths outside the declared scopes.
    `validate-outputs-and-scopes` already computes the violating set
    (via `resolveLayerPaths` + diff intersect, `bindings.go:773-`)
    but does not stash it in state — must add a key like
    `scope-violating-paths`.
- `internal/atdd/runtime/actions/bindings.go::validateOutputsAndScopes`
  — write the prompt-feeding state keys alongside the existing
  `failure-kind` write (mirrors `runCommand`'s diagnostic payload of
  `command-line`/`command-exit-code`/`command-stderr-tail`).
- `internal/atdd/runtime/agents/embed_test.go::TestFixKindPromptsExist`
  — extend `wantKinds` with `"missing-output"` and `"scope-diff"`.
- `internal/atdd/runtime/clauderun/clauderun.go` — `Options` fields for
  the new prompt placeholders, mirroring the `Command*` field pattern
  added in plan 1530 Items 5–6 (load-bearing only when dispatched
  agent is `fix-missing-output` / `fix-scope-diff`).
- `internal/atdd/runtime/driver/driver.go::newClaudeRunDispatcher` —
  populate the new options from `ctx.State` when dispatching either kind.

Out of scope:

- Re-walking the recovery wiring (closed by plan 1530).
- Tuning frontmatter for the new prompts beyond the existing fix-*
  conventions (sonnet/medium or whatever the other fix-* prompts use).

## Prompt-body design notes (rough)

- **`fix-missing-output.md`**: an agent failed to emit the YAML outputs
  declared in its `outputs:` block. The fix attempt re-runs the
  writing-agent's work with the missing keys highlighted, or stashes a
  diagnostic and lets a human pick up. Body should clearly list which
  keys are missing and quote the outer agent's task-name so the fix
  agent knows what context to reload.

- **`fix-scope-diff.md`**: an agent edited files outside its declared
  scopes. The fix attempt should revert the out-of-scope edits and
  re-run, OR escalate to a human if the diff is non-trivial. Body
  should list the violating paths and reference the gh-optivem.yaml
  `paths:` keys the agent was supposed to stay within.

Both bodies follow `fix-unexpected-failing-tests.md`'s structure
(Diagnose / Investigate / Remediate / COMMIT outputs).

## Sequencing

No upstream dependencies — plan 1530's plumbing already landed.

## Verification

- `TestFixKindPromptsExist` covers both new prompts at the embed level.
- `TestExecuteAgent_ValidationFailureDispatchesFixForFailureKind` already
  covers the dispatch wiring (lands as part of plan 1530); once the
  prompts exist, the real `agents.Lookup` will resolve them and the
  rehearsal scenario can run end-to-end without the missing-prompt
  crash.

## Cross-references

- Origin plan: `20260526-1530-fix-recovery-wire-failure-kind.md` (deleted
  on landing — see the `atdd/runtime: wire fix-command-failed recovery
  path end-to-end` commit for the plumbing landed by Items 1, 2, 5, 6
  and the follow-up commit that closed Items 3, 4, 7).
- Memory: `feedback_no_layer_coding_in_names` — `missing-output` /
  `scope-diff` describe scope of failure, not layer; no
  `_lowprimitive` or similar suffixes.
