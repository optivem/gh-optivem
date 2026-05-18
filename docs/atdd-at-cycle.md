# ACCEPTANCE TEST CYCLE

RED - GREEN - REFACTOR

Every phase agent operates within a declared allowed-path scope; see [§Conventions → Phase scope policy](#phase-scope-policy) for the per-phase table, and the BPMN process diagram for how it is enforced.

Inside each of these steps:

## PRE

1. Analyze Acceptance Criteria, is it written with Gherkin GIVEN-WHEN-THEN.
2. Does it have adequate positive and negative scenarios.

## RED

Between RED sub-phases, change-driven tests are disabled (and re-enabled at the start of the next phase) per [§Conventions → Disable-reason convention](#disable-reason-convention). This is BPMN orchestration plumbing — a separate scripted step around the commit, not the phase agent's responsibility. See the BPMN process diagram.

### RED: Test
1. For every Acceptance Criterion, write a corresponding Acceptance Test. This should be a mechanicla 1:1 translation.
2. If you need to add methods to DSL interface, then implement the DSL Core by implementing method prototypes by throwing a runtime exception  `"TODO: DSL"`, so that compilation works.
3. Set flag: `DSL Interface Changed: yes|no`


### RED: DSL
1. Implement the DSL Core for real — replace each "TODO: DSL" prototype with actual logic.
2. If you need add additional Driver interface methods:
   (a) In the System Driver Interface: implement prototype methods by throwing `"TODO: System Driver"` exception
   (b) In the External System Driver Interface: implement prototype methods by throwing `"TODO: External System Driver"` exception
3. Set both flags defined in [§Conventions → Phase-output flags](#phase-output-flags). Both **MUST** be set before completing the phase — unset is a bug, not a default `no`. The BPMN gateway downstream validates both flags are set and branches on their values; see the BPMN process diagram.
   (a) Set flag: `System Driver Interface Changed: yes|no`
   (b) Set flag: `External System Driver Interface Changed: yes|no`

### RED: System Driver
1. Implement the System Driver Adapters for real - replace each "TODO: System Driver" prototype with actual logic.


## RED: External System Driver
1. Go to the ATDD - CT Cycle

## GREEN
1. Implement the System - do the simplest implementation possible with the goal of making the Acceptance Tests pass.
2. **Tests, DSL, and Drivers are frozen during GREEN.** Do not modify acceptance test files, DSL Core, DSL interfaces, System Driver interfaces, or System Driver adapters to make GREEN pass. Production system code only.
3. **Escalation:** if you cannot make the tests pass without touching tests/DSL/Drivers, **stop and ask the user** — do not patch around it. Needing to touch a frozen layer signals that an earlier RED phase was wrong; the user decides whether to rewind to that phase (see [§Conventions → Phase scope policy](#phase-scope-policy) escalation options).

## REFACTOR
1. Refactor the System (if any improvements are seen) - propose first, then implement

## Conventions

Normative schemas that apply to **every cycle and sub-cycle** (AT, CT, Legacy, Structural). The AT cycle is the first to reference them, but sibling-cycle docs point here too.

### Disable-reason convention

Change-driven tests are disabled between RED sub-phases with the following annotation reason:

```
@Disabled("<TICKET-ID> - AT - <LOOP> - <PHASE>")
```

- **Separator:** ` - ` (space-hyphen-space) between every segment.
- **`<TICKET-ID>`:** verbatim from the tracker (e.g. `OPV-123`, `#42`, `SHOP-7`). Leads so the re-enable step can filter `startsWith("<TICKET-ID> - ")` and ignore tests belonging to other tickets.
- **`AT`:** the cycle (Acceptance Test). Reserves the slot for `CT` (Contract Test) under the same convention later.
- **`<LOOP>`:** `RED` | `GREEN`. Currently only `RED` uses disable; the slot is reserved for schema regularity.
- **`<PHASE>`:** `TEST` | `DSL` | `SYSTEM DRIVER` (uppercase; internal space allowed).

Examples:
- `@Disabled("OPV-123 - AT - RED - TEST")`
- `@Disabled("OPV-123 - AT - RED - DSL")`
- `@Disabled("OPV-123 - AT - RED - SYSTEM DRIVER")`

Re-enable filter (used by the BPMN re-enable step at the start of the next phase):

```
startsWith("<CURRENT-TICKET-ID> - AT - RED - <PREV-PHASE>")
```

Never strip annotations whose prefix belongs to a different ticket.

### Phase-output flags

After RED-DSL, the work-agent MUST set both flags below. They are read by the BPMN gateway downstream of RED-DSL to branch onto the right next phase; the gateway treats *unset* as an error (no implicit default).

| Flag name | Domain | Read by | Meaning when `yes` |
|---|---|---|---|
| `System Driver Interface Changed` | `yes` \| `no` | BPMN gateway after RED-DSL | RED-SYSTEM-DRIVER phase must run (new System Driver methods need real impls) |
| `External System Driver Interface Changed` | `yes` \| `no` | BPMN gateway after RED-DSL | Hand off to the CT cycle (external driver belongs to the CT sub-process) |

### Phase scope policy

**Every phase agent operates within a declared scope — no exceptions.** The table below is the complete source of truth: every phase has a row, and every agent's prompt is constructed with its row's allowed paths injected automatically. An agent without a declared scope is a configuration bug, not a default-allow.

Two layers enforce the policy; both converge on the same user-facing prompt — they differ only in who noticed the out-of-scope edit first.

- **Layer 1 — agent-triggered (in-agent recognition, BPMN-handled prompt):** the work-agent's prompt names the allowed paths for its phase. When the agent recognises it needs to edit out of scope, it does **not** wait inline for approval. Instead, it exits with a structured *scope-exception-requested* signal naming the intended out-of-scope file(s) and the reason. BPMN sees the signal and runs the same human-task prompt as Layer 2.
- **Layer 2 — BPMN post-phase scope check (catches what Layer 1 missed):** after each phase agent finishes normally, BPMN runs a scripted step that diffs the modified files (`git diff --name-only` vs the pre-phase ref) against the allowed-path policy. On violation, BPMN halts and runs the same human-task prompt.

In both cases, BPMN never auto-allows and never auto-reverts — the user always decides. Options:

- **Accept (continue from current phase)** — the agent's out-of-scope change is judged correct (e.g. RED-SYSTEM-DRIVER discovered the DSL or driver-port interface was wrong; GREEN discovered the test was wrong). Record the exception and continue from the current phase.
- **Rewind to upstream phase** — accept the out-of-scope change, then restart the cycle from the phase whose output was wrong (e.g. accept a DSL edit made during RED-SYSTEM-DRIVER, then rerun RED-DSL to re-validate the corrected DSL, then continue). This is the most principled response when the violation reveals an upstream bug — it preserves the per-phase RED guarantee instead of carrying an unvalidated upstream change forward.
- **Revert + rerun** — discard the out-of-scope changes and rerun the current phase agent.
- **Abort** — stop the cycle, escalate to human review.

Allowed-path policy by phase:

| Phase | Allowed paths |
|---|---|
| RED-TEST | acceptance test files; DSL prototype stubs (interface + `"TODO: DSL"` throw) |
| RED-DSL | DSL Core impls; driver-port interface declarations |
| RED-SYSTEM-DRIVER | `${driver_port}/${sut_namespace}/` and `${driver_adapter}/${sut_namespace}/<channel>` |
| GREEN | production system code only; tests/DSL/drivers are frozen (see GREEN section) |
| CT-RED-TEST / CT-RED-DSL / CT-RED-EXTERNAL-DRIVER / CT-GREEN-STUBS | `external/**` only |

This table is the source of truth for the policy schema; the BPMN scope-check step loads it at runtime.
