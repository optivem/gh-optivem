# AT/CT cycle — split creative work from mechanical work

Source: `plans/feedback/2026-05-05-process-feedback.md` AT Cycle 2.

## Problem

`AT - RED - TEST` (and its peers in the AT and CT cycles) is modeled in `process-flow.yaml` as a single `user_task` with `agent: atdd-test`. The agent prompt tells it to do the whole inner cycle: write tests, attempt to compile, add DSL prototypes if compile fails, get human approval, run tests, mark `@Disabled`, and finish. The CLI only takes over for `git commit`.

Two problems with this:

1. **It conflates creative with mechanical work.** Writing tests and writing DSL prototypes need LLM judgment. Compiling, running tests, marking `@Disabled`, and committing are deterministic — they don't need an LLM. Forcing the agent to drive them inflates each agent dispatch, makes the prompt confusing (*"don't commit, but DO run tests"*), and burns tokens on work that doesn't need them.
2. **It is BPMN-incorrect.** The inner sequence contains multiple distinct activity types (write, compile, run-tests, commit), internal gateways (compile passed?), and **human-gated STOPs** (REVIEW after WRITE, DSL approval after compile-fail). Those STOPs are not performer-internal procedure — they're process-level human interactions. In proper BPMN that's a Sub-Process, not a Task.

`structural_cycle` already follows the correct convention: only `STRUCT_WRITE` is agent-dispatched; `COMPILE`, `SAMPLE`, `DRIFT`, `ASK_COMMIT`, `COMMIT_STRUCT`, `TICK` are all `service_task`s; `STOP_STRUCT_REVIEW` and `STOP_STRUCT_TEST` are `user_task`s with `agent: human`. The AT and CT RED/WRITE phases violate this convention.

## Goal

Bring the AT and CT cycles into alignment with the principle `structural_cycle` already embodies:

- **Creative work (LLM-required) → `user_task` with an agent.** Writing tests, writing DSL prototypes, writing driver code.
- **Mechanical work (deterministic) → `service_task`.** Compile, run tests, mark `@Disabled`, commit.
- **Human gates → `user_task` with `agent: human`.** REVIEW STOPs become first-class nodes in the orchestration, not embedded in the agent prompt.

Per-agent prompts shrink correspondingly — they describe only the creative work, no compile/run/disable/commit instructions.

## Affected nodes

The same pattern lives in at least seven nodes today:

- `AT_RED_TEST` (`at_cycle`)
- `AT_RED_DSL` (`at_cycle`)
- `AT_RED_SYSTEM_DRIVER` (`at_cycle`)
- `CT_RED_TEST` (`ct_subprocess`)
- `CT_RED_DSL` (`ct_subprocess`)
- `CT_RED_EXTERNAL_DRIVER` (`ct_subprocess`)
- `CT_GREEN_STUBS` (`ct_subprocess`) — review separately; ownership/agent is already TBD
- `at_green_system`'s `ATDD_BACKEND` and `ATDD_FRONTEND` may need similar treatment — assess after the RED phases land.

All seven RED nodes share the same WRITE → REVIEW(STOP) → COMPILE → [compile-failed loop with DSL prototypes + STOP] → RUN → DISABLE → COMMIT structure. Decomposing each in place would copy the same internals seven times. The right shape is a **single shared parameterized sub-flow** (mirroring how `structural_cycle` is shared across `da_cycle` and `sut_cycle`), called from each phase's `call_activity` with phase-specific params.

## Proposed shared sub-flow

Sketch — refine during implementation:

```yaml
red_phase_cycle:
  start: WRITE
  nodes:
    - id: WRITE
      type: user_task
      agent: ${agent}
      phase_doc: ${phase_doc}
      description: "${phase_label} - WRITE"

    - id: REVIEW
      type: user_task
      agent: human
      role: review
      description: "${phase_label} - REVIEW (STOP)"

    - id: COMPILE
      type: service_task
      action: compile_targeted
      description: "Compile ${scope}"

    - id: GATE_COMPILE_OK
      type: gateway
      binding: compile_ok
      description: "Compile passed?"

    - id: WRITE_DSL_PROTOTYPES
      type: user_task
      agent: ${prototype_agent}        # often same as ${agent}
      phase_doc: ${prototype_phase_doc}
      description: "${phase_label} - DSL PROTOTYPES"

    - id: PROTOTYPE_REVIEW
      type: user_task
      agent: human
      role: review
      description: "${phase_label} - DSL PROTOTYPE REVIEW (STOP)"

    - id: RUN
      type: service_task
      action: run_targeted_tests
      description: "Run ${suite}"

    - id: GATE_RUN_OK
      type: gateway
      binding: tests_failed_runtime
      description: "Tests fail at runtime (not compile)?"

    - id: DISABLE
      type: service_task
      action: disable_change_driven
      description: "Disable change-driven scenarios"

    - id: COMMIT
      type: service_task
      action: commit_phase
      params:
        phase_label: ${phase_label}
      description: "COMMIT: <Ticket> | ${phase_label}"

    - id: RED_END
      type: end_event

  sequence_flows:
    - {from: WRITE,                to: REVIEW}
    - {from: REVIEW,               to: COMPILE}
    - {from: COMPILE,              to: GATE_COMPILE_OK}
    - {from: GATE_COMPILE_OK,      to: WRITE_DSL_PROTOTYPES, when: "compile_ok == false"}
    - {from: GATE_COMPILE_OK,      to: RUN,                  when: "compile_ok == true"}
    - {from: WRITE_DSL_PROTOTYPES, to: PROTOTYPE_REVIEW}
    - {from: PROTOTYPE_REVIEW,     to: COMPILE}                # loop
    - {from: RUN,                  to: GATE_RUN_OK}
    - {from: GATE_RUN_OK,          to: DISABLE,                when: "tests_failed_runtime == true"}
    # tests_failed_runtime == false is an exception STOP — out of scope here, sketch only
    - {from: DISABLE,              to: COMMIT}
    - {from: COMMIT,               to: RED_END}
```

Each existing RED node becomes a `call_activity` to `red_phase_cycle` with params: `agent`, `phase_doc`, `phase_label`, `scope`, `suite`, `prototype_agent`, `prototype_phase_doc`. Some params will collapse (e.g., `prototype_agent` is usually the same as `agent`) — finalize during implementation.

CT-specific behavior (real-vs-stub suite verification in `ct-red-test.md` step 2-3) needs to fit either as additional service_tasks in the shared sub-flow guarded by phase params, or as a CT-specific wrapper. Decide during implementation; do not pre-commit a structure here.

## CLI / runtime work

This is where the bulk of the engineering goes — the YAML restructure is shallow; the CLI gaining real responsibility is deep.

New `service_task` actions need implementations in `internal/atdd/runtime/actions/`:

- `compile_targeted(ctx)` — invoke the project's compile command for the relevant scope; return pass/fail.
- `run_targeted_tests(ctx)` — invoke `gh optivem test system --suite <suite> --test <name>` (or the equivalent) with parameters resolved from context; return pass/fail and runtime-vs-compile error classification.
- `disable_change_driven(ctx)` — apply the per-language `@Disabled`/`Skip`/`test.skip` markup (already documented in `language-equivalents.md`) to the change-driven scenarios identified at WRITE time. Deterministic.
- `commit_phase(ctx, phase_label)` — already exists for `structural_cycle`; reuse with phase-specific labels.

New gate bindings in `internal/atdd/runtime/gates/`:

- `compile_ok` — reads the result of `compile_targeted`.
- `tests_failed_runtime` — reads the result of `run_targeted_tests` and asserts the failure was runtime, not compile.

Context plumbing: each action needs structured access to `scope`, `suite`, `test_names`, etc. — currently embedded in the agent prompt; needs to become structured fields in `RunContext` (or equivalent) populated from the ticket parse + the WRITE phase's output.

## Per-agent prompt shrink

After the refactor, the `atdd-test` prompt (`internal/atdd/runtime/agents/prompts/atdd-test.md`) should describe **only** writing tests — no compile, no run, no disable, no commit, no STOP semantics. Reduces prompt size meaningfully and removes the confusing "don't commit / DO run tests" rule.

The `atdd-test` agent will also be dispatched a second time inside the same cycle for DSL prototypes (the `WRITE_DSL_PROTOTYPES` node). Either:

- Reuse `atdd-test` with a phase param distinguishing the dispatch, OR
- Split into `atdd-test` (writes tests) + `atdd-test-dsl-prototype` (writes prototypes).

Decide during implementation. Splitting is cleaner if the prompts diverge meaningfully; reusing is fine if the only difference is the input.

Same shrink applies to `atdd-dsl`, `atdd-driver` and the other RED-phase agents.

## Tradeoffs accepted

1. **More dispatches per phase.** Today: 1 agent dispatch covers WRITE → REVIEW → COMPILE → DSL → RUN → DISABLE → COMMIT. After: 1 dispatch for WRITE, optionally 1 for DSL prototypes, plus N service_task invocations and 1-2 STOP gates. More moving parts in exchange for clarity.
2. **CLI complexity grows.** Compile/run/disable orchestration becomes the CLI's job — it has to know language-specific compile commands, test invocation patterns, and disable syntax. `language-equivalents.md` already documents most of this; the CLI must consume it programmatically.
3. **The orchestrator/agent boundary moves.** Today the agent owns its inner cycle; after, the orchestrator owns it. This is the explicit point of the refactor — making the boundary match the creative/mechanical line.
4. **Token cost is probably net-favorable** but should be measured. Each dispatch's prompt shrinks; total dispatches per phase grow. Net direction depends on prompt size deltas.

## Sequencing

This refactor is too large for a single commit. Suggested split:

1. **Build `red_phase_cycle` infrastructure** — new actions (`compile_targeted`, `run_targeted_tests`, `disable_change_driven`), new gate bindings (`compile_ok`, `tests_failed_runtime`), context plumbing. Land without wiring any RED node to it.
2. **Migrate `AT_RED_TEST` first** — single node converted to `call_activity: red_phase_cycle`. Shrink `atdd-test` prompt. Decide on DSL-prototype dispatch (same agent vs split). End-to-end test on a known ticket before continuing.
3. **Migrate `AT_RED_DSL` and `AT_RED_SYSTEM_DRIVER`** — same pattern, smaller increments now that the framework is proven.
4. **Migrate the CT RED phases** — `CT_RED_TEST`, `CT_RED_DSL`, `CT_RED_EXTERNAL_DRIVER`. Address CT-specific real-vs-stub verification (extension to the shared sub-flow vs CT-specific wrapper).
5. **Reassess `at_green_system`'s `ATDD_BACKEND` / `ATDD_FRONTEND`** — likely benefit from the same split but evaluate after the RED phases settle. Out of scope until then.
6. **Reassess `CT_GREEN_STUBS`** — currently has a TBD agent (`atdd-stubs`); fold ownership decision into this work.

Each step is a separate commit and is independently mergeable.

## Verification

- `go test ./internal/atdd/runtime/...` clean at every step.
- End-to-end manual run against a story ticket, a bug ticket, and at least one of each task subtype — confirm the orchestrator drives compile/run/disable/commit while the agent only writes.
- Generated diagram shows the expanded `red_phase_cycle` sub-flow with creative steps in agent-blue, mechanical steps in service-white, and STOPs in human-yellow.
- Token-cost sample on one ticket end-to-end pre/post refactor; record in this plan as an addendum.

## Out of scope for this plan

- Changes to `structural_cycle` (already follows the convention).
- Diagram-renderer styling changes.
- Anything in the feedback plan (`20260505-230000-process-flow-feedback-intake-and-run-cycle.md`) — that lands first.
- Deeper changes to per-language test invocation (e.g., supporting new test runners). Adopt `language-equivalents.md` as-is for now.
