# Plan: ATDD BPMN orchestration

> ✅ **REFINED 2026-05-18** — every item walked one-by-one; the plan now extends the existing Go BPMN runtime (`internal/atdd/runtime/statemachine/process-flow.yaml` + `gates/` / `actions/` / `agents/`) rather than introducing a new orchestration tool. Items 1 and 10 are strikethrough stubs (deleted with rationale); item 9 Part B (CT_GREEN_STUBS) is ⏳ Deferred pending stubs-ownership. One pre-execute action remains: the cross-cutting Part 1 edit to remove `<channel>` from `docs/atdd/process/shared/conventions.md:65` (Hand-off).

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

### 4. Add post-RED-DSL flag-presence validation gateway

The branching this item describes is **already wired** in `at_cycle` (line 316): `GATE_DSL_AT` (binding `dsl_interface_changed`), `GATE_EXT_AT` (binding `external_system_driver_interface_changed`), `GATE_SYS_AT` (binding `system_driver_interface_changed`) with sequence_flows wiring exactly the three branches (route to RED-SYSTEM-DRIVER / CT_SUBPROCESS / AT_GREEN_SYSTEM). The current gateways consume flag *values* but do not verify the agent emitted them.

This item **adds the missing validation**:

- New `gateway` node `GATE_DSL_FLAGS_PRESENT` placed between `AT_RED_DSL` (the `red_phase_cycle` call_activity) and the existing `GATE_DSL_AT` in `at_cycle`.
- New binding `dsl_flags_present` in `internal/atdd/runtime/gates/` that reads the [§Conventions → Phase-output flags](../docs/atdd-at-cycle.md#phase-output-flags) emitted by `at-red-dsl` and returns `true` only if all three flags (`dsl_interface_changed`, `external_system_driver_interface_changed`, `system_driver_interface_changed`) are explicitly set.
- New `user_task: agent: human` STOP `STOP_FLAG_UNSET` — "STOP - HUMAN REVIEW — AT - RED - DSL phase-output flags missing; re-run with reminder". Loopback to `AT_RED_DSL`.
- Sequence_flows: `AT_RED_DSL → GATE_DSL_FLAGS_PRESENT`; `GATE_DSL_FLAGS_PRESENT → GATE_DSL_AT when present == true`; `GATE_DSL_FLAGS_PRESENT → STOP_FLAG_UNSET when present == false`; `STOP_FLAG_UNSET → AT_RED_DSL`.

One validation gateway covers all three flags (they're emitted together by the same RED-DSL phase). The existing `GATE_DSL_AT` / `GATE_EXT_AT` / `GATE_SYS_AT` then consume validated values without change.

> **Refined 2026-05-18:** Reframed from "add a post-RED-DSL gateway (validation + branching)" → "add only the flag-presence-validation gateway". **Why:** The three branching gateways are already in `at_cycle`; only the flag-presence check is new. Sequential-vs-parallel between RED-SYSTEM-DRIVER and CT sub-cycle was already settled in the existing wiring (`CT_SUBPROCESS` runs first, then `GATE_SYS_AT`, then `AT_RED_SYSTEM_DRIVER`) — that's a pre-existing decision, not something this plan re-litigates.

### 5. Implement the post-phase scope check action

Concrete implementation of item 2's bullet (c). Pure scripted (no LLM). Lives as:

- **Action** `check_phase_scope` in `internal/atdd/runtime/actions/` — reads `allowed_paths` from the node's `params:` (threaded from item 2 bullet (a)), runs `git diff --name-only <pre-phase-ref> HEAD` (and `git status --porcelain` for un-staged modifications), and writes a structured result to context.
- **Gate binding** `phase_scope_clean` in `internal/atdd/runtime/gates/` — reads the structured result; returns `true` if all modified paths fall within the allowed set, else `false`.
- **Wired position** (already pinned in item 2): between `DISABLE` and `COMMIT` in `red_phase_cycle`; before the parent's COMMIT in green flows.

**Path-variable resolution:**

- `${driver_port}`, `${driver_adapter}`, `${external_driver_port}`, `${external_driver_adapter}` resolve against `projectconfig.Config.Paths` (already populated by `internal/projectconfig/paths_defaults.go:DefaultPaths()`; docs in `internal/assets/global/docs/atdd/process/placeholders.md`).
- `${sut_namespace}` — **unresolved sub-question.** Appears in the §Conventions RED-SYSTEM-DRIVER row (`docs/atdd/process/at-cycle.md:100`) but is not yet sourced. Candidate: a new canonical key in `projectconfig.Paths` (or a sibling map). **Must be decided before `/execute-plan`** — pin the source, extend `placeholders.md`, and update `paths_defaults.go` if a new canonical key is added.
- `<channel>` — **dropped 2026-05-18.** No longer in the allowed-paths row (see Hand-off cross-cutting note for the Part 1 edit to `at-cycle.md:100`). Channel boundary is enforced by ticket scope + the human review STOP, not by path regex.

**Pre-phase ref:** captured by a small upstream service_task at WRITE-time (or read from the most recent COMMIT baseline) — pick one before execute; the choice affects whether the check sees the working-tree state alone or the full set of changes since the phase started.

STOP options (Accept / Rewind / Revert + rerun / Abort) live on `STOP_SCOPE_VIOLATION` (defined in item 2), not duplicated here.

> **Refined 2026-05-18:** Reframed from "post-phase scope check" → "implement the action + gate binding behind item 2 bullet (c)". **Why:** Item 2 already pins placement and STOP; this item is the action-body detail. Surfaced an unresolved sub-question (`${sut_namespace}` / `<channel>` source) that the original draft glossed as "get resolved from project config" — they're not in the project config schema yet.

### 6. Implement the Layer 1 scope-exception signal contract

Concrete implementation of item 2's bullet (b) — the agent-side escape hatch per [§Conventions → Phase scope policy](../docs/atdd/process/at-cycle.md#phase-scope-policy) Layer 1.

**Signal channel:** reuse the existing agent→runtime channel. Phase agents already emit a structured COMMIT output that the state machine consumes (per `process-flow.yaml` header: "what dispatches next, gated by which flag"). The Layer 1 signal becomes a structured field in that output payload:

```
scope_exception:
  files: [path/to/out-of-scope.go, ...]
  reason: "<one-line rationale>"
```

When absent, the phase ran within scope. No new IPC mechanism (no exit codes, marker files, or extra stdout channels).

**Gate binding:** new `scope_exception_requested` in `internal/atdd/runtime/gates/` reads the agent's output and returns `true` if `scope_exception` is non-empty.

**Wiring (already pinned in item 2 bullet (b)):** the new gateway sits immediately after `WRITE`; on `true`, branch to `STOP_SCOPE_VIOLATION` (skipping the post-phase scope check from item 5 and bypassing `DISABLE` / `COMMIT`); on `false`, continue down the normal path.

**Agent prompt template update:** the per-phase agent prompts (currently under `internal/assets/runtime/prompts/atdd/` or wherever they end up after the Part 1 Phase 7 prompt-slimming work) need a section instructing the agent to:
1. Edit only within `allowed_paths`.
2. If unavoidably blocked, emit the `scope_exception` field and exit.
3. **Never** ask inline for approval. Matches the runtime-prompt rule "no approval inside the agent".

> **Refined 2026-05-18:** Reframed from "define the signal format + envelope branch + behavioural rule" → "implement the channel + binding + prompt instruction". **Why:** The wiring is now in item 2 (b); this item is the contract detail. Open Q3 closed: signal channel = structured field in existing agent output (not a new IPC mechanism).

### 7. Implement the failing-legacy detector action

Concrete implementation of item 2's bullet (d). Lives as:

- **Action** `detect_failing_legacy` in `internal/atdd/runtime/actions/` — runs the test suite filtered by the legacy marker, writes structured result to context.
- **Gate binding** `failing_legacy_present` in `internal/atdd/runtime/gates/` — `true` if any legacy test failed.
- **Wired position** (already pinned in item 2 bullet (d)): right before COMMIT in both wrappers.
- **STOP options** (Treat as real regression / Mark for revision / Abort) live on `STOP_LEGACY_FAILED` (defined in item 2).

**Hard dependency (execute blocker):** the legacy marker convention — what "legacy marker" concretely means in the test suite — is owned by the [legacy-coverage-cycle plan](20260518-1116-legacy-coverage-cycle.md) (its §Conventions tightening + its Open Question on annotation/naming/directory). The action body cannot be written until that plan settles the marker. This plan consumes the marker as a typed dependency — it does not co-own the design.

**Behavioural guardrail (already enforced elsewhere):** "never `@Disabled` a failing legacy test" is enforced by (i) the `disable_change_driven` action's legacy-skip rule (item 3) and (ii) `STOP_LEGACY_FAILED`'s option set deliberately omitting `@Disabled`. Not restated here.

> **Refined 2026-05-18:** Reframed from "cross-plan reference + marker design alternatives" → "thin consumer of the legacy plan's marker convention". **Why:** user clarified the marker convention is fully owned by the legacy-coverage-cycle plan; this item just consumes it. Dropped the "annotation/naming/directory" restatement so the design call lives in one place.

### 8. Thread `allowed_paths` param into every AT-phase invocation

Every AT phase already invokes the shared wrapper via `call_activity`: `at_cycle` (line 316) wires `AT_RED_TEST`, `AT_RED_DSL`, `AT_RED_SYSTEM_DRIVER` as `call_activity → red_phase_cycle`, and `at_green_system` (line 406) wires `AT_GREEN_BACKEND` + `AT_GREEN_FRONTEND` as `call_activity → green_phase_cycle`. The envelope wiring itself is **already done**; this item adds the new `allowed_paths` param (item 2 (a)) to each invocation.

**Edits to `process-flow.yaml`:**

| call_activity | Add to `params:` |
|---|---|
| `AT_RED_TEST`           | `allowed_paths: <§Conventions row for AT - RED - TEST>` |
| `AT_RED_DSL`            | `allowed_paths: <§Conventions row for AT - RED - DSL>` |
| `AT_RED_SYSTEM_DRIVER`  | `allowed_paths: <§Conventions row for AT - RED - SYSTEM DRIVER>` |
| `AT_GREEN_BACKEND`      | `allowed_paths: <§Conventions row for AT - GREEN - SYSTEM (backend)>` |
| `AT_GREEN_FRONTEND`     | `allowed_paths: <§Conventions row for AT - GREEN - SYSTEM (frontend)>` |

The string values come from the §Conventions Phase scope policy table (`docs/atdd/process/at-cycle.md`). Placeholder resolution (`${driver_port}` etc.) happens in the `check_phase_scope` action (item 5), not at YAML-load time.

> **Refined 2026-05-18:** Reframed from "rewrite the AT cycle's BPMN diagram so every phase agent goes through the call activity" → "thread `allowed_paths` param into the already-existing call_activity invocations". **Why:** the call_activity wiring already exists (the original wording implied it didn't). Also dropped the REFACTOR parenthetical — refactoring isn't part of the AT cycle in the current process-flow; it lives in `structural_cycle` (line 660), which is out of scope for this plan.

### 9. Thread `allowed_paths` into CT phases — split A (RED phases) + B (CT_GREEN_STUBS deferred)

**Part A — mechanical, parallel to item 8:**

| call_activity | Add to `params:` |
|---|---|
| `CT_RED_TEST`              | `allowed_paths: <§Conventions row for CT - RED - TEST>` |
| `CT_RED_DSL`               | `allowed_paths: <§Conventions row for CT - RED - DSL>` |
| `CT_RED_EXTERNAL_DRIVER`   | `allowed_paths: <§Conventions row for CT - RED - EXTERNAL DRIVER>` |

These three already go through `red_phase_cycle` (see `ct_subprocess` line 471). Threading `allowed_paths` is identical to item 8's pattern.

**Part B — `CT_GREEN_STUBS`: ⏳ Deferred.** Two pre-existing issues block this from being a mechanical edit:

1. **Not currently wrapped.** `CT_GREEN_STUBS` (line 533) is a bare `user_task` directly with `agent: ct-green-stubs`, NOT a `call_activity → green_phase_cycle`. Adding `allowed_paths` here gives no enforcement without first rewiring through the wrapper.
2. **Ownership TBD.** The node carries the comment "Ownership TBD per process-audit gap on stubs ownership. Placeholder agent name; resolve before the Go runtime ships."

**Deferred until the stubs-ownership decision lands.** When it does, `CT_GREEN_STUBS` should be rewired as a `call_activity → green_phase_cycle` with `params: {agent, phase_doc, phase_label, suite, compile_action, allowed_paths}` — same shape as `AT_GREEN_BACKEND` / `AT_GREEN_FRONTEND`. Tracked here as a known gap; Part A is independent of it and can proceed.

> **Refined 2026-05-18:** Reframed from "symmetric to item 8 across four CT phases" → "Part A mechanical (three RED phases) + Part B deferred (CT_GREEN_STUBS pending stubs-ownership)". **Why:** the original wording assumed CT_GREEN_STUBS is a `call_activity → green_phase_cycle` like the AT GREEN phases; it isn't, and the ownership gap is upstream of this plan. Surfaced as Deferred so Part A can ship without waiting on B.

### ~~10. Document the orchestration~~ — removed

> **Refined 2026-05-18:** Deleted. **Why:** No new doc. The existing `docs/process-diagram.md` + the header docstring of `internal/atdd/runtime/statemachine/process-flow.yaml` already document the orchestration vocabulary and rendering. Where `docs/atdd/at-cycle.md` / `ct-cycle.md` reference "the BPMN process diagram", that pointer goes to `docs/process-diagram.md` (no new artefact needed). Any new node-type vocabulary introduced by items 2–7 gets a one-line update in `process-flow.yaml`'s header docstring as part of that item — not as a separate doc-writing step.

## Out of scope

- Per-language `@Disabled` syntax — delegated to `language-equivalents/`.
- Legacy marker convention design — owned by the [legacy-coverage-cycle plan](20260518-1116-legacy-coverage-cycle.md).
- Structural cycle and cycle router orchestration — separate plans (signposted in [Part 1 Phase 7](20260516-1701-atdd-at-cycle-absorb-internal-assets.md)).
- Migration of supporting docs (architecture, language-equivalents, glossary, etc.) out of `internal/assets/` — separate concern.

## Hand-off

Before executing this plan, the residual Open Questions (Q1 contract details — agent output `scope_exception` channel is settled in item 6; Q2 human-task UX reuses the existing `user_task: agent: human` STOP pattern) must be answered as they're walked. The artefact + runtime choice is settled (extends the existing Go BPMN runtime — see header).

**Cross-cutting Part 1 edit required** (refined 2026-05-18, walked under item 5/6):

- `docs/atdd/process/at-cycle.md:100` — remove `<channel>` from the §Conventions RED-SYSTEM-DRIVER allowed-paths row. New row reads `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/`. **Why:** the channel boundary is better enforced by ticket scope + the existing human review STOP than by a per-call path regex, and dropping `<channel>` eliminates a placeholder that has no source in `projectconfig` or in `params:`. Item 5's path-variable resolution sub-question shrinks to just `${sut_namespace}`.
