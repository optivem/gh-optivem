# Plan: ATDD BPMN orchestration

> 🤖 **Picked up by agent (refine)** — `Valentina_Desk` at `2026-05-18T11:50:44Z`

> ⚠️ **PARTIALLY REFINED** — this plan was drafted from the Phase 7 BPMN bullets in [Part 1](20260516-1701-atdd-at-cycle-absorb-internal-assets.md) and the cross-reference in the [legacy-coverage-cycle plan](20260518-1116-legacy-coverage-cycle.md). The structural reframe has been applied (items 1 + 10 + Open Q1/Q2 removed — see Refined 2026-05-18 annotations); items 2–9 still need a per-item walk to pin down concrete node/binding names and the residual contract questions (Q3/Q4).

**Date:** 2026-05-18
**Context:** The BPMN-side orchestration work that the AT-cycle doc reframes (Part 1 items 1, 2, 4b) assume exists. Those doc items reference "the BPMN process diagram", a "post-RED-DSL gateway", a "post-phase scope check", and a "shared call activity". The BPMN orchestration **itself already exists** in this repo as a Go runtime: `internal/atdd/runtime/statemachine/process-flow.yaml` (BPMN-shaped spec with start_event / end_event / service_task / user_task / gateway / call_activity nodes + sequence_flows + predicates), executed by the Go state machine in `internal/atdd/runtime/statemachine/` with pluggable `gates/`, `actions/`, `agents/` registries, rendered as Mermaid by `internal/atdd/runtime/diagram/` (per [`docs/process-diagram.md`](../docs/process-diagram.md)). **This plan extends that existing runtime** with the missing gates, actions, and call_activity wiring — it does not introduce a new orchestration tool or artefact form.

**Sibling plans referenced:**
- [Part 1 — AT-cycle architecture & §Conventions](20260516-1701-atdd-at-cycle-absorb-internal-assets.md) — defines the §Conventions schemas this plan implements gates and steps against.
- [Part 2 — `atdd-at-cycle.md` per-phase content](20260518-1116-atdd-at-cycle-part2-per-phase-content.md) — independent doc-content work; no orchestration dependency.
- [Legacy coverage cycle](20260518-1116-legacy-coverage-cycle.md) — supplies the legacy marker convention needed by the failing-legacy detector below.

**Source:** Phase 7 of Part 1 (the four BPMN bullets), plus the cross-plan reference in the legacy-coverage plan.

## Open questions (residual, after structural reframe)

> **Refined 2026-05-18:** Q1 (artefact form) and Q2 (runtime location) removed. **Why:** "follow exactly how we've done bpmn stuff up to now, it's in go" (user). The artefact is `internal/atdd/runtime/statemachine/process-flow.yaml` (BPMN-shaped YAML); the runtime is the Go state machine in the same package. No new tool, no new artefact form.

Residual questions — answered per item during the walk, not pre-resolved:

1. **What is the shared envelope's interface contract** with each phase agent?
   - How are allowed paths injected into the agent's prompt (env var? prepended block? template variable?).
   - How does the agent emit a *scope-exception-requested* signal back (exit code? structured JSON to stdout? a marker file?).
   - How does the agent report the two phase-output flags from RED-DSL (same channel as the scope-exception signal? separate?).
2. **Where do the human-task prompts live?** Each escalation prompt (scope violation, failing legacy, flag-unset, GREEN-can't-pass-without-touching-frozen-layer) needs a concrete UX — re-uses the existing `user_task: agent: human` STOP mechanism in the state machine, but the prompt content + option set still need defining.

## Items

> **Refined 2026-05-18 (applies to items 2–9):** All items below extend the existing Go BPMN runtime — new nodes in `internal/atdd/runtime/statemachine/process-flow.yaml`, new bindings in `internal/atdd/runtime/gates/` / `actions/` / `agents/`, rendered via the existing `internal/atdd/runtime/diagram/`. No new orchestration tool, no new artefact form.

### ~~1. Choose and document the orchestration form (decision blocker)~~ — removed

> **Refined 2026-05-18:** Deleted. **Why:** The artefact + runtime already exist (`process-flow.yaml` + Go state machine in `internal/atdd/runtime/statemachine/`, Mermaid via `internal/atdd/runtime/diagram/`). "Follow exactly how we've done bpmn stuff up to now, it's in go" (user). The choice this item proposed was a non-decision.

### 2. Extend `red_phase_cycle` + `green_phase_cycle` with scope + legacy enforcement

The shared per-phase wrapper already exists as two sub-processes in `internal/atdd/runtime/statemachine/process-flow.yaml`: `red_phase_cycle` (line 815) and `green_phase_cycle` (line 936). Every AT/CT phase already invokes them via `call_activity` with per-phase `params:` (`agent`, `phase_doc`, `phase_label`, `change_type`/`suite`, `compile_action`). This item extends those wrappers with the scope + legacy enforcement they currently lack.

**Additions to each wrapper:**

a. **New `allowed_paths` param** threaded from the parent's call site (in `at_cycle`, `at_green_system`, `ct_subprocess`, …) through to the `WRITE` user_task. The agent reads it from its prompt template. (Resolves the agent-prompt half of Open Q3.)

b. **Post-WRITE scope-exception gateway (Layer 1)** — new `gateway` node immediately after `WRITE` (or after the human-review STOP), with a binding that reads the agent's scope-exception signal. On `signal == true`, branch to a new `STOP_SCOPE_VIOLATION` human task (shared across both wrappers). Detail of the signal channel is the second half of Open Q3 — see item 6.

c. **Pre-COMMIT scope check (Layer 2)** — new `service_task` between `DISABLE` and `COMMIT` (red) / between the verify-passes gate and the parent's COMMIT (green), running the post-phase scope check action. On hit, branch to the same `STOP_SCOPE_VIOLATION`. Detail in item 5.

d. **Pre-COMMIT failing-legacy check** — new `service_task` right before `COMMIT`, running the failing-legacy detector. On hit, branch to a new `STOP_LEGACY_FAILED` human task. Detail in item 7.

**Two new human-task STOPs**, defined once and reused across the two wrappers:

- `STOP_SCOPE_VIOLATION` — context: violating paths + allowed paths + (when Layer-1-triggered) the agent's reason. Options: Accept / Rewind to upstream phase / Revert + rerun / Abort.
- `STOP_LEGACY_FAILED` — context: failing legacy test name + failure output. Options: Treat as real regression (rewind) / Mark legacy test as needing revision (escalate to legacy cycle) / Abort.

**Adding a new phase** stays the same as today: add a `call_activity` to the wrapper with per-phase `params:` (now also `allowed_paths`); no bespoke per-phase orchestration.

> **Refined 2026-05-18:** Reframed from "define a new envelope" → "extend the two existing wrappers". **Why:** `red_phase_cycle` + `green_phase_cycle` already are the shared envelope; this item adds the missing scope/legacy enforcement to them. The original "Refined later" note on disable/enable placement is also dropped — the existing code already settles it: `DISABLE` lives inside `red_phase_cycle` (line 876); `ENABLE_TESTS` lives in the parent (`at_green_system` line 409) between phases.

### 3. Update `disable_change_driven` + `enable_change_driven` actions to §Conventions disable-reason format

Both nodes already exist in `process-flow.yaml`: `DISABLE` (line 876 inside `red_phase_cycle`, `action: disable_change_driven`) and `ENABLE_TESTS` (line 409 inside `at_green_system`, `action: enable_change_driven`). This item updates the **action implementations** under `internal/atdd/runtime/actions/` — no new nodes in `process-flow.yaml`.

- **`disable_change_driven`** (runs at end of phase, before COMMIT — already wired): grep the project for test files, annotate change-driven tests with `@Disabled("<TICKET-ID> - <CYCLE> - <LOOP> - <PHASE>")` per [§Conventions → Disable-reason convention](../docs/atdd-at-cycle.md#disable-reason-convention). **Precondition:** RED proof has been observed (test ran, failed at runtime). Skip legacy tests entirely (per the legacy-coverage plan's domain restriction).
- **`enable_change_driven`** (runs at start of next phase — already wired): grep for `@Disabled` annotations whose reason matches `startsWith("<CURRENT-TICKET-ID> - <CYCLE> - <LOOP> - <PREV-PHASE>")` and remove them. Never strip annotations for other tickets; never strip legacy markers.

Inputs (ticket ID, cycle, loop, phase) come from the action's context — extend the action signatures / context if not all four are currently threaded through.

Language-specific syntax for `@Disabled` (Java) / `@pytest.mark.skip` (Python) / `[Ignore]` (.NET) / etc. is delegated to the existing `language-equivalents/` material.

> **Refined 2026-05-18:** Reframed from "add disable/enable steps" → "update existing actions". **Why:** the nodes are already in the YAML and wired into the wrappers — only the action bodies need to follow the new §Conventions disable-reason format and the legacy-skip rule.

### 4. Post-RED-DSL gateway (flag validation + branching)

A scripted gateway step that runs immediately after the RED-DSL phase agent completes (inside the shared envelope from item 2, between steps 5 and 7, or as a separate post-envelope step — refinement decision).

- **Inputs:** the two phase-output flags emitted by the RED-DSL agent per [§Conventions → Phase-output flags](../docs/atdd-at-cycle.md#phase-output-flags).
- **Validation:** both flags MUST be present. If either is unset, halt and route to the **flag-unset human task** (escalation: rerun the RED-DSL agent with a "you forgot to set the flags" reminder, or accept whatever the user marks).
- **Branching:**
  - `System Driver Interface Changed = yes` → enqueue RED-SYSTEM-DRIVER as the next phase.
  - `External System Driver Interface Changed = yes` → enqueue the CT sub-cycle (CT-RED-TEST onwards) as the next phase (sequential or parallel with RED-SYSTEM-DRIVER — refinement decision).
  - both `no` → skip directly to GREEN.

### 5. Post-phase scope check (the Layer 2 enforcement step in item 2)

Pure scripted (no LLM). Inputs: phase name, pre-phase git ref.

1. Run `git diff --name-only <pre-phase-ref> HEAD` to list modified files (and `git status --porcelain` to catch un-staged modifications, depending on where in the phase pipeline the check fires — refinement decision).
2. For each modified path, check it matches the phase's allowed-paths row from §Conventions → Phase scope policy. Variables like `${driver_port}`, `${sut_namespace}`, `<channel>` get resolved from project config.
3. On any path outside the allowed set, halt and route to the **scope-violation human task** with: the violating paths, the allowed paths for the phase, and the four options:
   - Accept (continue from current phase)
   - Rewind to upstream phase (which phase)
   - Revert + rerun (current phase)
   - Abort

The human task is BPMN's standard human-task pattern — drop into the chosen runtime UX (Open Question 4).

### 6. Layer 1 scope-exception signal handling

The agent-side escape hatch (per [§Conventions → Phase scope policy](../docs/atdd-at-cycle.md#phase-scope-policy) Layer 1). Defines:

- The signal format (Open Question 3) — exit code, marker file, or structured JSON to stdout.
- The envelope's branch (item 2, step 4): when the signal is detected, *skip the post-phase scope check* (item 5) and route directly to the same scope-violation human task with the agent's stated out-of-scope files and reason as context.
- Behavioural rule: the agent must **not** wait inline for approval. It either edits within scope and completes normally, or it emits the signal and exits. No in-agent human interaction (matches the runtime-prompt rule "no approval inside the agent").

### 7. Failing-legacy detector

Cross-plan reference: legacy marker convention is defined in the [legacy-coverage-cycle plan](20260518-1116-legacy-coverage-cycle.md) (still to be designed — annotation / naming / directory; that plan's Open Questions).

Once the marker convention is fixed:

1. After each phase (item 2, step 6), run all tests filtered by the legacy marker.
2. On any legacy test failure, halt and route to the **failing-legacy human task** with: the failing legacy test name, the failure output, and options:
   - Treat as real regression (rewind to the phase that introduced it; investigate)
   - Mark the legacy test as needing revision (escalate to a legacy-cycle re-run on that test)
   - Abort

**Never `@Disabled` a failing legacy test** — that's the AT-side guardrail also in the legacy-coverage-cycle plan.

### 8. Wire each AT phase through the envelope

Once items 2–6 exist, rewrite the AT cycle's BPMN diagram (item 1's artifact) so every phase agent is invoked via the shared "Run Phase Agent" call activity from item 2. Phases: RED-TEST, RED-DSL, RED-SYSTEM-DRIVER, GREEN. (REFACTOR is a propose-first step with different shape — refinement decision whether it goes through the envelope or has a lighter wrapper.)

### 9. Wire each CT phase through the envelope

Symmetric to item 8 for the CT sub-cycle: CT-RED-TEST, CT-RED-DSL, CT-RED-EXTERNAL-DRIVER, CT-GREEN-STUBS. Same envelope, different per-phase allowed-paths rows from §Conventions.

### ~~10. Document the orchestration~~ — removed

> **Refined 2026-05-18:** Deleted. **Why:** No new doc. The existing `docs/process-diagram.md` + the header docstring of `internal/atdd/runtime/statemachine/process-flow.yaml` already document the orchestration vocabulary and rendering. Where `docs/atdd/at-cycle.md` / `ct-cycle.md` reference "the BPMN process diagram", that pointer goes to `docs/process-diagram.md` (no new artefact needed). Any new node-type vocabulary introduced by items 2–7 gets a one-line update in `process-flow.yaml`'s header docstring as part of that item — not as a separate doc-writing step.

## Out of scope

- Per-language `@Disabled` syntax — delegated to `language-equivalents/`.
- Legacy marker convention design — owned by the [legacy-coverage-cycle plan](20260518-1116-legacy-coverage-cycle.md).
- Structural cycle and cycle router orchestration — separate plans (signposted in [Part 1 Phase 7](20260516-1701-atdd-at-cycle-absorb-internal-assets.md)).
- Migration of supporting docs (architecture, language-equivalents, glossary, etc.) out of `internal/assets/` — separate concern.

## Hand-off

Before executing this plan, the open questions must be answered (item 1 essentially does that, but the choice should be discussed with the user during `/refine-plan` rather than executed cold). Items 2 onwards are concrete once item 1 is settled.
