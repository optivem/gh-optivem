# Fix dead change-driven gate and `${agent}` template leak

## Motivation

Two unrelated bugs surfaced back-to-back during the v2b rehearsal run on `optivem/shop` issue #61 (`Redesigning New Order UI`, classified as `system-ui-task`) on 2026-04-30:

1. After the intake agent (`atdd-task`) committed, the orchestrator prompted:

       Change-driven AC produced by the intake agent? [y/N]: n

   The prompt is asked but its answer changes nothing — every outgoing edge from `GATE_CHANGE_DRIVEN` keys on `ticket_type`, not on the bool the gate just computed (see `internal/atdd/runtime/statemachine/testdata/process-flow.yaml:189-193`). The user is being interrupted to populate dead state.

2. Immediately after, the host `claude` session was dispatched with a prompt containing the literal placeholder:

       🤖 ENTERING AGENT: ${agent}  (interactive)
       …
       ❯ Launch the ${agent} subagent for the current ATDD phase.

   The host couldn't resolve the agent name and bounced back asking the operator which agent to launch — wasted dispatch, wasted tokens, broken UX.

Both are concrete bugs in the v2b state-machine + dispatcher pipeline, and both should be fixed before we burn any more soak runs on rehearsal tickets.

## Items

Ordered independently — either can land first.

### 1. Delete `GATE_CHANGE_DRIVEN` (or wire its outgoing edges to actually consult it)

**File:** `internal/atdd/runtime/statemachine/testdata/process-flow.yaml` (the cycle flow's `nodes:` and `sequence_flows:` blocks).
**Plus:** `internal/atdd/runtime/gates/bindings.go` (drop `changeDrivenACProduced` + its registration at line 96 + its prompt at 218-222) and the corresponding entry in `gates/bindings_test.go:330`.

Today's wiring (`process-flow.yaml:124-127, 188-193`):

```yaml
- id: GATE_CHANGE_DRIVEN
  type: gateway
  binding: change_driven_ac_produced
  description: "Change-driven AC produced?"
…
- {from: LEGACY_CYCLE,         to: GATE_CHANGE_DRIVEN}
- {from: GATE_CHANGE_DRIVEN,   to: AT_CYCLE,       when: "ticket_type in [story, bug]"}
- {from: GATE_CHANGE_DRIVEN,   to: SYSAPI_CYCLE,   when: "ticket_type == system-api-task"}
- {from: GATE_CHANGE_DRIVEN,   to: SYSUI_CYCLE,    when: "ticket_type == system-ui-task"}
- {from: GATE_CHANGE_DRIVEN,   to: EXTAPI_CYCLE,   when: "ticket_type == external-api-task"}
- {from: GATE_CHANGE_DRIVEN,   to: CHORE_CYCLE,    when: "ticket_type == chore"}
```

The gate computes `change_driven_ac_produced`, writes it to Context (`run.go:wrapGateway`), then nothing reads it. Routing is `ticket_type`-only.

**Recommended fix — delete it.** Replace with a per-type fan-out at the same junction:

```yaml
- {from: STOP_INTAKE,          to: GATE_LEGACY}
- {from: GATE_LEGACY,          to: LEGACY_CYCLE,   when: "legacy_coverage_section_present == true"}
- {from: GATE_LEGACY,          to: GATE_TYPE_CYCLE,when: "legacy_coverage_section_present == false"}
- {from: LEGACY_CYCLE,         to: GATE_TYPE_CYCLE}
- {from: GATE_TYPE_CYCLE,      to: AT_CYCLE,       when: "ticket_type in [story, bug]"}
- {from: GATE_TYPE_CYCLE,      to: SYSAPI_CYCLE,   when: "ticket_type == system-api-task"}
- … etc.
```

`GATE_TYPE_CYCLE` becomes a `binding: ticket_type` gateway re-using the existing `ticket_type` binding (`gates/bindings.go:94`), which is already populated by the upstream `CLASSIFY` action. No new prompt, no dead state.

**Alternative — keep the gate semantic.** If the original intent was *"a story/bug whose intake produced no change-driven AC should fall through to a structural cycle, not AT_CYCLE"*, the fix is to:
- Wire the intake agents (`atdd-story` / `atdd-bug` / `atdd-task` / `atdd-chore`) to emit `change_driven_ac_produced=true|false` in their commit trailer or stdout JSON, parsed by the dispatcher into Context state.
- Rewrite the outgoing edges to actually read it: `to: AT_CYCLE, when: "ticket_type in [story, bug] && change_driven_ac_produced == true"`, with a fallback edge to the structural cycle.

I recommend **deletion**. Reason:
- The "story without change-driven AC" path is undefined today — no test, no docs, no observed shop ticket. Encoding capacity for a case nobody has hit yet violates "don't add features beyond what the task requires" (CLAUDE.md / `CONTRIBUTING.md`).
- The intake-agent → Context plumbing doesn't exist; building it is meaningful scope.
- A `system-ui-task` like #61 is *by definition* not change-driven AC — that's why the classification exists. The bool is redundant with `ticket_type`.

**Estimated effort:** 90 minutes including:
- YAML edit (delete node, replace 5 edges with 5 edges off the new gateway).
- Drop the binding + prompt + test entry.
- Update `transitions_test.go` cases that name `GATE_CHANGE_DRIVEN` (search shows ~3 references — verify and renumber).
- One state-machine integration test asserting that `system-ui-task` reaches `SYSUI_CYCLE` without any prompt round-trip.

**Risk:** none beyond test churn. The behaviour change is "operator no longer sees a prompt that has no effect."

### 2. Expand `${agent}`, `${phase_doc}`, `${phase}` in both dispatcher paths

**Files:**
- `internal/atdd/runtime/statemachine/run.go` — export `expandParams` (rename to `ExpandParams` at line 236, update the in-package caller at line 67).
- `internal/atdd/runtime/driver/driver.go`:
  - `newClaudeRunDispatcher` (lines 335-359): expand `raw.Agent`, `raw.PhaseDoc`, `raw.Description` against `ctx.Params` before populating `cOpts`.
  - `newManualAgentDispatcher` → `promptForAgent` (lines 388-401): same expansion before printing the dispatch banner.

Today (`driver.go:344`):

```go
cOpts := clauderun.Options{
    Agent:           raw.Agent,         // literal "${agent}" inside structural_cycle
    PhaseDoc:        raw.PhaseDoc,      // literal "${phase_doc}"
    NodeDescription: raw.Description,   // may interpolate ${phase}
    …
}
```

Fix:

```go
cOpts := clauderun.Options{
    Agent:           statemachine.ExpandParams(raw.Agent, ctx.Params),
    PhaseDoc:        statemachine.ExpandParams(raw.PhaseDoc, ctx.Params),
    NodeDescription: statemachine.ExpandParams(raw.Description, ctx.Params),
    …
}
```

Same three calls inside `promptForAgent`. The state-machine engine already does `expandParams(node.Raw.Agent, ctx.Params)` for the registry lookup at `run.go:67`; the dispatcher needs to do the same for the *user-facing* string. The leak is invisible in the unit tests at `clauderun_test.go:93` and `driver_test.go:155` because they exercise non-templated `agent: atdd-test` nodes only — add a regression case that drives a templated node and asserts the rendered prompt + banner contain the substituted name, not `${agent}`.

**Estimated effort:** 2 hours including:
- Rename + export `ExpandParams` (the function is a 5-line utility; no API users outside the package today).
- Three call sites in `driver.go`.
- Regression test: a structural-cycle node dispatched with `Params={"agent":"atdd-task","phase_doc":"docs/atdd/process/sysui-redesign.md"}` should produce a prompt containing `Launch the atdd-task subagent` and a banner containing `🤖 ENTERING AGENT: atdd-task`.
- Test in `driver_test.go` for the manual path printing the substituted `DISPATCH: atdd-task` line.

**Risk:** trivial. `expandParams` is referentially transparent and idempotent on already-substituted strings (no `${…}` placeholders → identity). If `ctx.Params` is nil (root-level dispatch outside a `call_activity`), the function is a no-op. No behaviour change for non-templated nodes.

## Tradeoff

Both items reduce the surface area: item 1 deletes a gate, item 2 deletes a class of leak. The only judgment call is item 1 — keep-and-rewire vs delete. I recommend delete for the reasons above; happy to revisit if a real shop ticket emerges where a story/bug shouldn't run AT_CYCLE.

## Phased rollout

Single PR. Both changes are pure gh-optivem, independent of shop, and shippable atomically. Soak: one rehearsal run of `gh optivem atdd implement-ticket --issue <N>` against a `system-ui-task` (verifies item 1 — no spurious prompt) and one against a `story` (verifies item 2 — `${agent}` properly substituted in `at_cycle`'s templated nodes, if any; otherwise the structural-cycle path covers it).

## See also

- Origin: rehearsal run on `optivem/shop` #61 (`Redesigning New Order UI`), 2026-04-30. Commit `a1e70687` on `rehearsal/atdd-cli` is the v2b artifact that exposed both issues.
- Adjacent plans:
  - `plans/20260430-150508-minimize-tokens-and-latency-in-clauderun-dispatch.md` — same dispatch path, orthogonal optimisations.
  - `plans/20260430-144514-v2b-operational-hardening.md` — soak/observability follow-ups; the soak instrumentation in §3 there is what would have flagged item 1 sooner.
- Source files:
  - Dead gate: `internal/atdd/runtime/statemachine/testdata/process-flow.yaml:124-127, 188-193`; `internal/atdd/runtime/gates/bindings.go:96, 218-222`.
  - Template leak: `internal/atdd/runtime/driver/driver.go:344-346, 392-399`; `internal/atdd/runtime/statemachine/run.go:67, 232-240`.
