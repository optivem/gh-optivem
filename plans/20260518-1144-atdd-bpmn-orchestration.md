# Plan: ATDD BPMN orchestration

> ⚠️ **NOT YET REFINED** — this plan was drafted from the Phase 7 BPMN bullets in [Part 1](20260516-1701-atdd-at-cycle-absorb-internal-assets.md) and the cross-reference in the [legacy-coverage-cycle plan](20260518-1116-legacy-coverage-cycle.md). Run `/refine-plan` on this file before `/execute-plan` to walk each item and pin down concrete file locations, naming, and the shape of the BPMN artifacts (XML? Mermaid? Camunda? something else?).

**Date:** 2026-05-18
**Context:** The BPMN-side orchestration work that the AT-cycle doc reframes (Part 1 items 1, 2, 4b) assume exists. Those doc items reference "the BPMN process diagram", a "post-RED-DSL gateway", a "post-phase scope check", and a "shared call activity" — none of which currently exist as code or as an artifact in this repo. This plan creates them.

**Sibling plans referenced:**
- [Part 1 — AT-cycle architecture & §Conventions](20260516-1701-atdd-at-cycle-absorb-internal-assets.md) — defines the §Conventions schemas this plan implements gates and steps against.
- [Part 2 — `atdd-at-cycle.md` per-phase content](20260518-1116-atdd-at-cycle-part2-per-phase-content.md) — independent doc-content work; no orchestration dependency.
- [Legacy coverage cycle](20260518-1116-legacy-coverage-cycle.md) — supplies the legacy marker convention needed by the failing-legacy detector below.

**Source:** Phase 7 of Part 1 (the four BPMN bullets), plus the cross-plan reference in the legacy-coverage plan.

## Open questions (decide before refinement)

These shape every item below. Refinement should answer these first.

1. **What form does "the BPMN process diagram" take?** Options:
   - Camunda / Zeebe XML (real BPMN 2.0, runnable by a workflow engine).
   - Mermaid `flowchart` / `stateDiagram` in a Markdown doc (documentation only, not runnable; matches the rest of `docs/`).
   - Plain Markdown step list with explicit gateways called out (cheapest; matches the way `atdd-at-cycle.md` itself is written).
   - Something else (Activiti, n8n, custom YAML).
2. **Where does the orchestration *run*?** Options:
   - A real BPMN engine spun up per cycle (heavy).
   - A thin scripted runner (shell / Node / Go) that walks the gateway logic from a config file. Cheap; matches the "scripted / Haiku" tone of the doc.
   - Composed inside a Claude orchestrator agent that reads the gateway rules and calls phase sub-agents.
3. **What is the shared envelope's interface contract** with each phase agent?
   - How are allowed paths injected into the agent's prompt (env var? prepended block? template variable?).
   - How does the agent emit a *scope-exception-requested* signal back (exit code? structured JSON to stdout? a marker file?).
   - How does the agent report the two phase-output flags from RED-DSL (same channel as the scope-exception signal? separate?).
4. **Where do the human-task prompts live?** Each escalation prompt (scope violation, failing legacy, flag-unset, GREEN-can't-pass-without-touching-frozen-layer) needs a concrete UX — CLI prompt? Slack message? GitHub issue comment? — and a place to define the option set.

## Items

### 1. Choose and document the orchestration form (decision blocker)

Resolve Open Questions 1 + 2. Pick one of the four artefact options for "the BPMN process diagram" and one of the runtime options. Capture the decision in `docs/atdd-bpmn-orchestration.md` (new doc) with a single paragraph of rationale.

**Why this is item 1:** every other item references "the diagram" or "the runner"; without this decision they can't have concrete file paths or schemas.

### 2. Define the shared "Run Phase Agent" call activity / envelope

A single reusable wrapper every phase reuses. Inputs: phase name, ticket ID. Steps (in order):

1. Look up the phase's allowed-paths row from [§Conventions → Phase scope policy](../docs/atdd-at-cycle.md#phase-scope-policy).
2. Construct the phase agent's prompt, injecting the allowed-paths block.
3. Run the phase agent.
4. **Handle scope-exception signal** (Layer 1) — if the agent exited with the signal, route to the scope-violation human task with the agent's named out-of-scope files and reason.
5. **Post-phase scope check** (Layer 2) — diff modified files (`git diff --name-only` vs the pre-phase ref) against allowed-paths. On violation, route to the scope-violation human task.
6. **Post-phase failing-legacy check** — run the failing-legacy detector (item 7). On hit, route to the failing-legacy human task.
7. Emit normal completion (or the escalation outcome from one of the human tasks).

Adding a new phase becomes: add a row to §Conventions → Phase scope policy + one call to this envelope from the cycle diagram. No bespoke per-phase orchestration.

> **Refined later:** decide whether disable/enable bookkeeping (item 3) lives inside the envelope or as a separate step around it. Probably "around it, in the cycle diagram between phases", since enable depends on the *previous* phase's identity and disable depends on the *current* phase's identity.

### 3. Disable/enable steps around the commit

Cheap implementation (scripted; Haiku at most). Inputs: ticket ID, cycle (AT|CT), loop (RED|GREEN), phase.

- **Disable step** (runs at end of phase, before commit): grep the project for test files, annotate change-driven tests with `@Disabled("<TICKET-ID> - <CYCLE> - <LOOP> - <PHASE>")` per [§Conventions → Disable-reason convention](../docs/atdd-at-cycle.md#disable-reason-convention). **Precondition:** RED proof has been observed (test ran, failed at runtime). Skip legacy tests entirely (per the legacy-coverage plan's domain restriction).
- **Enable step** (runs at start of next phase): grep for `@Disabled` annotations whose reason matches `startsWith("<CURRENT-TICKET-ID> - <CYCLE> - <LOOP> - <PREV-PHASE>")` and remove them. Never strip annotations for other tickets; never strip legacy markers.

Language-specific syntax for `@Disabled` (Java) / `@pytest.mark.skip` (Python) / `[Ignore]` (.NET) / etc. is delegated to the existing `language-equivalents/` material.

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

### 10. Document the orchestration

Single doc `docs/atdd-bpmn-orchestration.md` (created in item 1) covers:

- The chosen artefact + runtime form (item 1).
- The shared envelope (item 2).
- Each gate/check/step (items 3–7) with the human-task escalation prompts.
- How the AT and CT cycle diagrams compose phase calls (items 8, 9).
- Pointers back to §Conventions in `docs/atdd-at-cycle.md` for the schemas.

This doc is what `docs/atdd-at-cycle.md` and `docs/atdd-ct-cycle.md` mean when they say "see the BPMN process diagram".

## Out of scope

- Per-language `@Disabled` syntax — delegated to `language-equivalents/`.
- Legacy marker convention design — owned by the [legacy-coverage-cycle plan](20260518-1116-legacy-coverage-cycle.md).
- Structural cycle and cycle router orchestration — separate plans (signposted in [Part 1 Phase 7](20260516-1701-atdd-at-cycle-absorb-internal-assets.md)).
- Migration of supporting docs (architecture, language-equivalents, glossary, etc.) out of `internal/assets/` — separate concern.

## Hand-off

Before executing this plan, the open questions must be answered (item 1 essentially does that, but the choice should be discussed with the user during `/refine-plan` rather than executed cold). Items 2 onwards are concrete once item 1 is settled.
