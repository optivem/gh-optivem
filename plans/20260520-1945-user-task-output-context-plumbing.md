# user_task output → Context plumbing

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-05-20T19:41:45Z`

**Date:** 2026-05-20
**Trigger:** `gh optivem atdd-rehearsal implement` failed at AT_RED_TEST → RUN with `run_targeted_tests: test_names not set in Context`. The fast-fail in `internal/atdd/runtime/actions/bindings.go:822` is doing its job — no production code populates `test_names` or `suite` for the RED RUN, and no production code populates `scope_exception_files` for the RED GATE_SCOPE_EXCEPTION either. The whole "user_task emits structured output → engine flattens into Context.State" infrastructure is unbuilt.

## Cross-references

- Acknowledged TODO in source: `internal/atdd/runtime/statemachine/process-flow.yaml:926-930` —
  *"Context plumbing TODO: `run_targeted_tests` reads `suite` and `test_names`; the `disable-tests` and `enable-tests` agents (user_task dispatch) read `language`, `ticket_id`, `loop`, `phase` / `prev_phase`, and `disable_targets` via template substitution. Today nothing populates these — see the AT/CT split plan's 'context plumbing' follow-up."*
- Existing doctrine for the shape of structured agent output: `internal/assets/runtime/shared/scope.md` (the `scope_exception:` YAML block emitted in the agent's final output).
- Existing gate that documents the contract but has no producer:
  `internal/atdd/runtime/gates/bindings.go:661-678` (`scopeExceptionRequested`)
  and the WRITE TODO comments at L661-674.
- Related but separable: the deferred deterministic-disable-enable plan
  [`deferred/20260520-0002-deterministic-disable-enable-fallback.md`](deferred/20260520-0002-deterministic-disable-enable-fallback.md)
  — we are NOT reviving per-language extraction code here.
- Predecessor: `20260505-230100-at-ct-cycle-creative-mechanical-split.md` —
  the AT/CT split that introduced this gap as a documented follow-up.

## The gap, sized

Three Context keys are read by production code paths but written by nothing:

| Key | Type | Reader | Source-of-truth |
|---|---|---|---|
| `test_names` | `[]string` | `runTargetedTests`, `verifyRealSuitePasses` | WRITE agent (which test methods did it just author?) |
| `suite` | `string` | `runTargetedTests`, `verifyRealSuitePasses` | Static per-phase (could be call-site param OR agent output) |
| `scope_exception_files` | `[]string` | `scopeExceptionRequested` gate | WRITE agent (only set on exception path) |
| `scope_exception_reason` | `string` | (informational only) | WRITE agent (only set on exception path) |

A larger set of keys *would* be read by no-op production code if dispatched, but degrade gracefully via boolGate's prompt fallback — these are not blocking the rehearsal, but they will be once the rehearsal is run non-interactively (`--autonomous` / CI):

| Key | Type | Reader | Source-of-truth |
|---|---|---|---|
| `dsl_interface_changed` | `bool` | `GATE_DSL_AT` / `GATE_DSL_CT` / `GATE_DSL_LEGACY_*` | RED-TEST agent's COMMIT output |
| `external_system_driver_interface_changed` | `bool` | `GATE_EXT_AT` / `GATE_EXT_CT` | RED-DSL agent's COMMIT output |
| `system_driver_interface_changed` | `bool` | `GATE_SYS_AT` | RED-DSL agent's COMMIT output |
| `refactor_changed` | `bool` | `GATE_REFACTOR_CHANGED` | at-refactor-system agent's COMMIT output |
| `refinement_changed` | `bool` | post-BACKLOG_REFINEMENT gate | refine-acc agent's COMMIT output |

All of these share one root cause: the user_task dispatcher (`internal/atdd/runtime/driver/driver.go:770-841 newClaudeRunDispatcher`) calls `clauderun.Dispatch` and discards the agent's final result text. `clauderun.RunResult` exposes only `Usage` (token counts); the parsed `result` field from the `claude -p --output-format json` envelope is *printed* to stdout (clauderun.go:1145) and then thrown away for programmatic purposes.

## Proposed long-term solution

**Structured agent output, parsed by the user_task dispatcher into `ctx.State`.** Pattern:

1. Every agent prompt emits a single fenced YAML block at the end of its final response under a known wrapper key (`outputs:`):

   ```yaml
   outputs:
     test_names: [shouldRegisterCustomer, shouldRejectDuplicateCustomer]
     suite: <acceptance-api>
     dsl_interface_changed: false
   ```

   Agents that hit a scope exception emit an additional sibling block (already specified by `scope.md`):

   ```yaml
   scope_exception:
     files:
       - path/to/out-of-scope.go
     reason: <one-line rationale>
   ```

2. The dispatcher (after `clauderun.Dispatch` returns) parses the `outputs:` block out of the captured final result text and writes each key/value into `ctx.State` with the right Go type. The `scope_exception` block is flattened into the existing `scope_exception_files` / `scope_exception_reason` keys.

3. The downstream gates and actions read `ctx.State` exactly as their unit tests already do today — no API change to the readers.

### Why this direction (vs alternatives)

- **vs. mechanical post-WRITE scan** (git-diff the working tree, extract new test method names per language pattern): the deterministic per-language extraction code was deliberately deleted on 2026-05-20 ([deferred/20260520-0002](deferred/20260520-0002-deterministic-disable-enable-fallback.md)) in favor of agent-driven language-agnostic mechanisms. Re-introducing per-language pattern code here for the WRITE→RUN handoff contradicts that direction and re-introduces the per-language extensibility cost we just shed.
- **vs. drop `--test` granularity and just run the whole suite:** loses the ability to detect "agent wrote a test that silently passed" (the RED-loop's whole purpose), and creates long suite runs in the per-iteration RED loop.
- **vs. punt:** the dispatcher has no other mechanism to learn agent intent, and every gate in the table above is blocked the same way. Punting just defers the same plan.

The structured-output direction is also the BPMN-idiomatic answer: user_task **outputs** are a first-class concept in BPMN process state, and this is exactly how the engine should expose them.

### Why not just pass `suite` as a call-activity param

`suite` is one of the three missing keys, and it is genuinely static per-phase — every call site of `red_phase_cycle` already knows whether it is dispatching against `<acceptance-api>`, `<acceptance-ui>`, or `<suite-contract-real>`. So it *could* be plumbed as a call_activity param with no agent cooperation.

That works as a tactical unblocker, but we should still solve the general problem (`test_names`, `scope_exception_*`, the five boolean gate flags), and passing `suite` via call_activity creates a duplicate channel: agent output for some keys, call_activity params for others, with no clear rule for which goes where. Cleaner to route everything that is *phase-derived* through structured agent output and reserve call_activity params for *truly static* knobs (agent name, phase label, compile_action, rebuild_before_run, change_type — all of which are properties of the call site, not the phase work).

Decision: **fold `suite` into the structured-output infrastructure**, not into call_activity params. Open to revisiting in Q3 below.

## Scope of this plan

**Narrowed 2026-05-20** (see "Carved out" below for why). This plan
now lands only the AT-RED-TEST `test_names` slice:

- `at-red-test` prompt emits `outputs: { test_names: [...] }` at the end of
  its final response.
- AT-RED-DSL and AT-RED-SYSTEM-DRIVER inherit the value via shared
  ctx.State (the `wrapCallActivity` engine saves/restores Params but not
  State, so sibling sub-process calls share `ctx.State` end-to-end). No
  prompt amendment needed for those impl agents.
- The TODO comment at `process-flow.yaml:926-930` is updated, not
  removed — work is partial.

**Explicitly out of scope** (separate plans, listed because we're touching adjacent code):

- Wiring the five boolean gate flags (`dsl_interface_changed`, `external_system_driver_interface_changed`, `system_driver_interface_changed`, `refactor_changed`, `refinement_changed`) to the same `outputs:` block. Once the parser exists, this becomes per-prompt amendment + a one-line registration per key. Defer to a follow-up so the infrastructure lands first and gets exercised by one consumer before generalizing.
- Plumbing the `language`, `ticket_id`, `loop`, `phase`, `prev_phase`, `disable_targets` keys that `disable-tests` / `enable-tests` agents read via template substitution. Those are *consumed* (template substitution) not *emitted* — they need a separate plan about WHERE the upstream values are produced.
- The `green_phase_cycle` `suite` plumbing. Same shape, same fix; defer until the RED path is proven.

## Carved out — follow-up plans needed

Originally in this plan's scope but deferred during execution
(2026-05-20) because they require additional design work — primarily
making suite vocabulary project-configurable — that doesn't belong
under "context plumbing":

- **ct-red-test prompt amendment** to emit `outputs: { test_names, suite }`.
  Same shape as at-red-test, but blocked behind project-configurable
  suite resolution: without it, the prompt would hardcode
  `<suite-contract-stub>` (the WRITE-phase RUN target per
  `testselect/suite.go:18-19`), which is exactly the kind of fixed
  vocabulary the surrounding architecture is trying to shed.

- **Project-configurable suite vocabulary.** Suite tokens
  (`<acceptance-api>`, `<acceptance-ui>`, `<suite-contract-stub>`,
  `<suite-contract-real>`) and the `testselect.AcceptanceSuites()`
  hardcoded fallback (`["acceptance-api", "acceptance-ui"]`) assume
  the shop project's channel naming. Non-shop projects with
  `acceptance-mobile` or differently-named channels do not work today.
  The fix replaces *both* this plan's would-be `suite:` emission from
  ct-red-test AND the pre-existing hardcoded
  `verify_real_suite: "<suite-contract-real>"` literal at CT_RED_TEST in
  `process-flow.yaml`. Symmetric treatment in one plan.

- **`disable-tests` / `enable-tests` template substitution keys**:
  `language`, `ticket_id`, `loop`, `phase` / `prev_phase`,
  `disable_targets`. These are *consumed* (substituted into prompts) not
  *emitted* — they need a separate plan that identifies where each
  upstream value is produced.

- **4 impl agents** (`at-red-dsl`, `at-red-system-driver`, `ct-red-dsl`,
  `ct-red-external-system-driver`) intentionally not amended — they
  inherit `test_names` (and any future shared keys) from the upstream
  RED-TEST agent in the same cycle via shared ctx.State. Documented
  here so a future plan author doesn't redo the analysis.

## Items

8. - [ ] **Re-run the rehearsal.** `gh optivem atdd-rehearsal implement` from
   the same starting state that produced the original failure. Expect
   AT_RED_TEST to reach DISABLE / COMMIT. The next gap will surface either at
   CT_RED_TEST → RUN (no `test_names` / no `suite` for CT — addressed by the
   carved-out follow-up plans above) or at a `disable-tests` /
   `enable-tests` template substitution (also carved out).

## Open questions

(Q1 and Q2 resolved during implementation of items 1-4; recorded in the
parser doc comment at `clauderun/outputs.go` for posterity. Q3 below is
folded into the carved-out "project-configurable suite vocabulary"
follow-up.)

3. **Q3: How should `suite` be resolved per phase?** The original plan
   proposed agent-emitted `suite` tokens. During execution we realized
   suite vocabulary is project-configurable (channel naming varies:
   `acceptance-mobile`, alternative conventions) and should not be baked
   into prompts. The carved-out follow-up addresses this: replaces both
   the to-be-added `suite:` emission and the pre-existing hardcoded
   `verify_real_suite: <suite-contract-real>` literal with a single
   project-config-driven resolution.

## Sequencing

Items 1-4 (infrastructure: surface agent result text, add YAML outputs
parser, wire into user_task dispatcher) landed pre-execution. This
session lands the at-red-test prompt amendment + the
process-flow.yaml comment update. Item 8 is a manual verification step
the user runs when ready. Everything else moved to the carved-out
follow-up plans listed above.

## Why this plan instead of expanding 20260505-230100

Per [feedback_new_plan_not_extend](../../../../Users/valen_4rjvn9e/.claude/projects/C--GitHub-optivem-academy-gh-optivem/memory/feedback_new_plan_not_extend.md): broadening scope = fresh plan that cross-references the original. The 230100 plan was about decomposing AT/CT phases into creative/mechanical halves; that work is largely shipped. This plan picks up the explicit follow-up ("context plumbing") as its own scoped artifact rather than mutating the predecessor.
