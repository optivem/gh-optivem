# Plan: Shared `compile` sub-process + collapse RED two-step write

## Background

The COMPILE → GATE_COMPILE_OK pattern is inlined three times in
`internal/atdd/runtime/statemachine/process-flow.yaml`, with **inconsistent
fail-handling across the three phases**:

| Phase | Sub-process | On compile-fail | Human stop? | Fixer |
|---|---|---|---|---|
| Structural | `structural_cycle` (line 654) | `STOP_COMPILE_FAIL_REVIEW` → `FIX_COMPILE` → COMPILE | yes | `atdd-fix-verify` agent |
| RED | `red_phase_cycle` (line 820) | `WRITE_PROTOTYPES` → `STOP_PROTOTYPE_REVIEW` → COMPILE | **no — auto-proceeds** | same agent (`${agent}`), writes stubs |
| GREEN | `green_phase_cycle` (line 958) | `STOP_GREEN_COMPILE_FAIL` → `WRITE` → COMPILE | yes | loops to `WRITE` (same agent retries) |

RED is the odd one out: compile-fail auto-dispatches the fixer with no human
gate, because **RED treats compile-fail as the expected path**. The test
deliberately references DSL surface that doesn't exist yet, so compile fails,
then `WRITE_PROTOTYPES` adds minimal stubs. Caught during a rehearsal run on
2026-05-15 where a TS compile error sent the flow straight into
`WRITE_PROTOTYPES` without operator review — which is the *designed* behaviour
today, but the asymmetry surfaces the real question:

> Should RED's WRITE step produce a *compiling* failing test (test + DSL
> stubs together), so that compile-fail becomes unexpected in RED too — the
> same as STRUCT and GREEN?

If yes, the asymmetry collapses at the source. RED becomes shape-identical
to GREEN: WRITE → STOP → COMPILE (expected pass) → next. `WRITE_PROTOTYPES`
and `STOP_PROTOTYPE_REVIEW` disappear. The shared `compile` sub-process
handles compile-fail uniformly across all three phases — and a compile-fail
in *any* phase is a genuine signal (agent bug, missing stub, etc.) worth
halting on.

This plan does both: extract the shared sub-process **and** collapse the
RED two-step write.

## Principles

1. **Every compile failure is human-gated before a fixer is dispatched.**
   Enforced *by construction*, the same way every commit is human-gated by
   construction via the `commit` sub-process (line 1026): callers reference
   the sub-process, not the underlying mechanical task, so the gate cannot
   be skipped.

2. **Compile-fail is an exception, not a planned path.** Each phase's WRITE
   step is responsible for producing code that compiles. In RED that means
   the test agent writes both the test *and* the DSL prototype stubs it
   needs, in one creative step. The compile that follows is expected to
   pass; failure indicates an agent bug (forgotten stub, typo, etc.) and
   trips the shared fail-loop.

## Design

### New shared `compile` sub-process

Add a new sub-process parallel to `commit`:

```yaml
# ===========================================================================
# compile
# Shared sub-process called wherever code is compiled. Pairs the mechanical
# COMPILE service task with the GATE + human-review-on-fail by construction —
# callers cannot run compile without the fail-loop, so every compile failure
# is human-gated before a fixer is dispatched. Returns only when compile_ok.
#
# Callers supply via `params:`:
#   - compile_action — which compile_* action to invoke
#   - fix_agent      — agent dispatched when compile fails
#   - phase_doc      — phase doc passed to fix_agent
# ===========================================================================
compile:
  start: COMPILE
  nodes:
    - id: COMPILE
      type: service_task
      action: ${compile_action}
      documentation: "Compile (${compile_action})"

    - id: GATE_COMPILE_OK
      type: gateway
      binding: compile_ok
      documentation: "Compile passed?"

    - id: STOP_COMPILE_FAIL_REVIEW
      type: user_task
      agent: human
      role: review
      documentation: "STOP - HUMAN REVIEW — compile failed, dispatch ${fix_agent}?"

    - id: FIX_COMPILE
      type: user_task
      agent: ${fix_agent}
      phase_doc: ${phase_doc}
      documentation: "FIX compile errors"

    - id: COMPILE_END
      type: end_event

  sequence_flows:
    - {from: COMPILE,                  to: GATE_COMPILE_OK}
    - {from: GATE_COMPILE_OK,          to: COMPILE_END,              when: "compile_ok == true"}
    - {from: GATE_COMPILE_OK,          to: STOP_COMPILE_FAIL_REVIEW, when: "compile_ok == false"}
    - {from: STOP_COMPILE_FAIL_REVIEW, to: FIX_COMPILE}
    - {from: FIX_COMPILE,              to: COMPILE}
```

### RED's WRITE step gains the stub-writing responsibility

The WRITE agent in `red_phase_cycle` (substituted at call time as `${agent}`,
e.g. `atdd-test-at`) updates its contract:

> Produce a **compiling** failing test. This means writing both:
> 1. The acceptance test itself, expressing the desired behaviour, and
> 2. Any DSL prototype stubs the test references that don't exist yet —
>    minimum signature only, no behaviour. The test must compile.
>
> The RED state is proven later by the runtime test failure
> (`tests_failed_runtime`), not by compile failure.

Single human review (`STOP_RED_REVIEW`) sees the full test plus DSL delta
together — which is the unit reviewers actually care about ("is this the
test I want, including its new vocabulary?").

## Call-site migrations

### Structural cycle (line 654)

**Before** — inlined:
```yaml
- id: COMPILE                       { type: service_task, action: compile_all }
- id: GATE_COMPILE_OK               { type: gateway, binding: compile_ok }
- id: STOP_COMPILE_FAIL_REVIEW      { type: user_task, agent: human, role: review }
- id: FIX_COMPILE                   { type: user_task, agent: atdd-fix-verify, ... }
...
- {from: COMPILE,                   to: GATE_COMPILE_OK}
- {from: GATE_COMPILE_OK,           to: CHOOSE_TESTS,             when: "compile_ok == true"}
- {from: GATE_COMPILE_OK,           to: STOP_COMPILE_FAIL_REVIEW, when: "compile_ok == false"}
- {from: STOP_COMPILE_FAIL_REVIEW,  to: FIX_COMPILE}
- {from: FIX_COMPILE,               to: COMPILE}
```

**After**:
```yaml
- id: COMPILE
  type: call_activity
  process: compile
  params:
    compile_action: compile_all
    fix_agent: atdd-fix-verify
    phase_doc: ${phase_doc}
...
- {from: APPROVE_CHANGE, to: COMPILE}
- {from: COMPILE,        to: CHOOSE_TESTS}
```

### RED phase cycle (line 820)

**Before** — two-step write with planned compile-fail:
```yaml
- id: WRITE                         { agent: ${agent} }
- id: STOP_RED_REVIEW               { agent: human }
- id: COMPILE                       { action: ${compile_action} }
- id: GATE_COMPILE_OK               { binding: compile_ok }
- id: WRITE_PROTOTYPES              { agent: ${agent} }
- id: STOP_PROTOTYPE_REVIEW         { agent: human }
...
- {from: WRITE,                     to: STOP_RED_REVIEW}
- {from: STOP_RED_REVIEW,           to: COMPILE}
- {from: COMPILE,                   to: GATE_COMPILE_OK}
- {from: GATE_COMPILE_OK,           to: WRITE_PROTOTYPES,          when: "compile_ok == false"}
- {from: GATE_COMPILE_OK,           to: GATE_VERIFY_REAL_REQUIRED, when: "compile_ok == true"}
- {from: WRITE_PROTOTYPES,          to: STOP_PROTOTYPE_REVIEW}
- {from: STOP_PROTOTYPE_REVIEW,     to: COMPILE}
```

**After** — single WRITE that includes stubs, compile expected to pass:
```yaml
- id: WRITE
  type: user_task
  agent: ${agent}
  phase_doc: ${phase_doc}
  documentation: "${phase_label} - WRITE test + DSL stubs"
- id: STOP_RED_REVIEW
  type: user_task
  agent: human
  role: review
- id: COMPILE
  type: call_activity
  process: compile
  params:
    compile_action: ${compile_action}
    fix_agent: ${agent}
    phase_doc: ${phase_doc}
...
- {from: WRITE,           to: STOP_RED_REVIEW}
- {from: STOP_RED_REVIEW, to: COMPILE}
- {from: COMPILE,         to: GATE_VERIFY_REAL_REQUIRED}
```

Removed: `WRITE_PROTOTYPES`, `STOP_PROTOTYPE_REVIEW`, four sequence_flows,
the false/true compile_ok branching.

### GREEN phase cycle (line 958)

**Before**:
```yaml
- id: COMPILE                      { action: ${compile_action} }
- id: GATE_COMPILE_OK              { binding: compile_ok }
- id: STOP_GREEN_COMPILE_FAIL      { agent: human }
...
- {from: COMPILE,                  to: GATE_COMPILE_OK}
- {from: GATE_COMPILE_OK,          to: STOP_GREEN_COMPILE_FAIL, when: "compile_ok == false"}
- {from: GATE_COMPILE_OK,          to: RUN,                    when: "compile_ok == true"}
- {from: STOP_GREEN_COMPILE_FAIL,  to: WRITE}
```

**After**:
```yaml
- id: COMPILE
  type: call_activity
  process: compile
  params:
    compile_action: ${compile_action}
    fix_agent: ${agent}
    phase_doc: ${phase_doc}
...
- {from: WRITE,    to: COMPILE}
- {from: COMPILE,  to: RUN}
```

## Implementation steps

### Step 1 — Add the `compile` sub-process

Add the `compile:` block to
`internal/atdd/runtime/statemachine/process-flow.yaml`, placed near the
existing `commit:` block (line 1026) so the two shared
"construct-enforced human gate" sub-processes sit together.

### Step 2 — Migrate `structural_cycle` (line 654)

Replace inlined `COMPILE / GATE_COMPILE_OK / STOP_COMPILE_FAIL_REVIEW /
FIX_COMPILE` nodes with one `call_activity` to `compile`. Remove the four
corresponding sequence_flows; keep `{from: COMPILE, to: CHOOSE_TESTS}` since
the call_activity exits successfully only when compile passes.

### Step 3 — Collapse RED two-step write & migrate `red_phase_cycle` (line 820)

Two changes together:
- Replace inlined `COMPILE / GATE_COMPILE_OK` with `call_activity` to
  `compile`.
- **Remove** `WRITE_PROTOTYPES` and `STOP_PROTOTYPE_REVIEW` nodes entirely.
- Remove the four sequence_flows that connected them.
- Add `{from: COMPILE, to: GATE_VERIFY_REAL_REQUIRED}` (direct, no
  conditional).

### Step 4 — Migrate `green_phase_cycle` (line 958)

Replace inlined `COMPILE / GATE_COMPILE_OK / STOP_GREEN_COMPILE_FAIL` with
`call_activity` to `compile`. Remove the corresponding sequence_flows;
`{from: COMPILE, to: RUN}` is direct.

### Step 5 — Update RED-phase WRITE agent contracts

The `${agent}` parameter for `red_phase_cycle` is supplied by each caller
that embeds it. Enumerate callers:

```
grep -n "process: red_phase_cycle" internal/atdd/runtime/statemachine/process-flow.yaml
```

For each caller (likely `at_cycle`, possibly others), find the corresponding
agent prompt/definition under `.claude/agents/` (or wherever ATDD agent
definitions live in this repo) and update its contract to require:

- Writing the acceptance test.
- Writing any DSL prototype stubs the test references that don't yet exist
  (minimum signature, no behaviour).
- Ensuring the result compiles.

Update the corresponding phase_doc (path supplied as `phase_doc` param) to
match.

### Step 6 — Update state-machine tests

Touched test files (per `grep GATE_COMPILE_OK`):
- `internal/atdd/runtime/statemachine/structural_cycle_test.go`
- `internal/atdd/runtime/statemachine/behavioral_cycle_test.go`
- `internal/atdd/runtime/statemachine/transitions_test.go`
- `internal/atdd/runtime/statemachine/dispatch_expect_test.go`

Update expected node lists and transitions to reflect:
- The call_activity factoring (all three phases).
- The removal of `WRITE_PROTOTYPES` and `STOP_PROTOTYPE_REVIEW` from
  `red_phase_cycle`.

Add a new test asserting the `compile` sub-process's structure (start, nodes,
flows) — analogous to whatever exercises the `commit` sub-process today.

### Step 7 — Update process diagram

Regenerate `docs/process-diagram.md` and the per-phase SVGs:
- `docs/images/process-diagram-10-structural-cycle-shared.svg`
- `docs/images/process-diagram-14-green-phase-cycle.svg`
- `docs/images/process-diagram-15-red-phase-cycle.svg`

Verify the diagram:
- Surfaces the new `compile` sub-process box on each cycle (like `commit` is
  surfaced today).
- No longer shows `WRITE_PROTOTYPES` or `STOP_PROTOTYPE_REVIEW` in the RED
  cycle diagram.

### Step 8 — Verify with rehearsal

Re-run a rehearsal flow analogous to the one that triggered this work
(rehearsal-20260515-095931, TypeScript view-product-list, two missing DSL
methods). Confirm:
- The WRITE agent produces a compiling failing test (test + stubs together).
- COMPILE passes on the first attempt — no fixer dispatched in the happy
  path.
- If compile fails (agent bug), the flow halts at `STOP_COMPILE_FAIL_REVIEW`
  for human review; approval dispatches the fixer; retry loops back to
  COMPILE; pass on retry exits cleanly to `GATE_VERIFY_REAL_REQUIRED`.

## Out of scope

- The action and gate bindings (`compile_all`, `compile_system`,
  `compile_system_tests`, `compile_ok`) — no changes needed; the new
  sub-process reuses them as-is.
- Other duplicated patterns in process-flow.yaml that might warrant similar
  extraction (test-fail-review loops, etc.) — separate plans if and when they
  come up.
- The deferred plans
  `plans/deferred/20260507-210016-call-activity-wrapper-naming.md` and
  `plans/deferred/20260501-144322-process-flow-node-id-rename-open-questions.md`
  reference `WRITE_PROTOTYPES` / `STOP_PROTOTYPE_REVIEW`. They are deferred;
  leave them as-is. If/when un-deferred, they'll need refreshing against the
  new RED shape.

## Status

- Design: agreed on 2026-05-15
  - Shared `compile` sub-process: confirmed
  - WRITE_TEST writes stubs (collapse two-step RED write): confirmed
- Ready for execution
