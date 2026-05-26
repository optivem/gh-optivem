# Strip duplicated banner comments from `process-flow.yaml`

## Origin / intent

Spun off from `plans/20260526-1448-agent-prompt-fixes.md` Item 12
during refinement (2026-05-26). The parent plan is scoped to
agent-prompt fixes under
`internal/assets/runtime/prompts/atdd/*.md`; banner comments in
`internal/atdd/runtime/statemachine/process-flow.yaml` are
unrelated and warrant their own plan.

This plan has **zero file overlap** with the agent-prompt-fixes
parent plan. It can refine and execute independently.

## Observation

`process-flow.yaml` carries per-node banner comments that
duplicate the node's structured fields. Concrete example
(`process-flow.yaml:1270-1276`):

```
# ===========================================================================
# MID — write-contract-tests
# ===========================================================================
#
# Scope: read + write = [ct-test, dsl-port, dsl-core]. Outputs:
# dsl-port-changed.
# ===========================================================================
write-contract-tests:
  start: EXECUTE_AGENT
  nodes:
    - id: EXECUTE_AGENT
      type: call-activity
      process: execute-agent
      params:
        task-name: write-contract-tests
        outputs: "dsl-port-changed"
      read:  [ct-test, dsl-port, dsl-core]
      write: [ct-test, dsl-port, dsl-core]
```

**Audit of each banner element:**

| Banner content | Duplication shape | Verdict |
|----------------|-------------------|---------|
| `===` separator lines | Visual noise | Cosmetic; deletable |
| `MID — write-contract-tests` | Layer label + process name | Name duplicates YAML key one line below; "MID" is descriptive metadata redundant with file position |
| `Scope: read + write = [...]` | Duplicates `read:` and `write:` fields verbatim | WHAT duplication |
| `Outputs: dsl-port-changed` | Duplicates `outputs:` param verbatim | WHAT duplication |
| Occasional WHY notes (e.g. `write-acceptance-tests:1248-1249` "deliberately identical at the fold — parent plan 20260526-1448 Item 3 refines the split") | Genuine WHY content | **Keep** as short standalone comment |

**Document-level header comments (lines 1-130)** are different in
character — they explain the TOP/CYCLE/HIGH/MID/LOW taxonomy,
encoding conventions, and known runtime contract gaps. That is
genuine WHY content, not WHAT duplication. **Keep unchanged.**

## Resolution (settled during refinement)

**Strip-to-WHY.** Per-node banner comments shrink from 6+ lines
to 0-2 lines:

- Delete the `===` separator lines (above and below each banner).
- Delete the `MID — <name>` (and `HIGH — <name>`, `CYCLE — <name>`,
  `TOP — <name>`, `LOW — <name>`) labels — the process name is
  one line below, and the level is inferable from file position
  (the file is sectioned by level).
- Delete `Scope: read + write = [...]` lines (WHAT duplication
  with the node's `read:` and `write:` fields).
- Delete `Outputs: <name>` lines (WHAT duplication with the
  node's `outputs:` param).
- **Preserve any genuine WHY notes** that explain *why* a node is
  shaped a particular way — these go above the node as short
  standalone comments (no banner framing).

Examples of WHY notes to preserve (re-grep during execution):

- `write-acceptance-tests:1248-1249`: "The read/write lists are
  deliberately identical at the fold — parent plan 20260526-1448
  Item 3 refines the split."
- `disable-tests:1423-...`: "The [at-test, ct-test] pin is
  forward-looking…"

(Audit the full corpus during execution; the WHY notes are
sparse and identifiable.)

## Out of scope

- Document-level header comments (lines 1-130) — keep as-is.
- Agent prompt fixes (parent plan
  `20260526-1448-agent-prompt-fixes.md`).
- The `phase-scopes.yaml` fold (separate spinoff plan
  `20260526-1536-fold-phase-scopes-into-process-flow.md`).
- Layer-coded *names* (process names) — they're already clean
  (e.g. `write-contract-tests`, not `mid-write-contract-tests`);
  no rename work required.

## Coordination with other in-flight plans

- **`20260526-1536-fold-phase-scopes-into-process-flow.md`**:
  the fold spinoff is currently in execution and edits
  `process-flow.yaml`. **This plan should land AFTER the fold
  completes** — otherwise the fold's structural edits could
  conflict with this plan's comment-stripping edits in the same
  file.
- **`20260526-1448-agent-prompt-fixes.md`**: zero file overlap;
  parallel refinement and execution safe.

## Acceptance

- `grep -nE '^# (TOP|CYCLE|HIGH|MID|LOW) — ' internal/atdd/runtime/statemachine/process-flow.yaml`
  returns zero hits (the layer-label banners are stripped).
- `grep -nE '^# Scope: read|^# Outputs:' internal/atdd/runtime/statemachine/process-flow.yaml`
  returns zero hits (the WHAT-duplication lines are stripped).
- `grep -nE '^# ===' internal/atdd/runtime/statemachine/process-flow.yaml`
  returns zero hits (separator lines stripped).
- Document-level header (lines 1-130) preserved.
- Genuine WHY comments preserved as short standalone comments;
  spot-check the file shows the WHY content survives.
- `statemachine.LoadDefault()` still parses the file without
  error (no structural changes; comment-only edit).
- Downstream test suite green (comment edits are inert to the
  parser).
