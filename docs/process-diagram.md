# ATDD Process Flow

> Generated from `internal/atdd/runtime/statemachine/process-flow.yaml` by `internal/atdd/runtime/diagram`. Do not edit by hand — edit the YAML and regenerate via `gh optivem process show > docs/process-diagram.md`.

Each section corresponds to one named process in the YAML. `call_activity` nodes appear as boxes pointing at the linked sub-process's heading.

## Legend

Node **shape** encodes the BPMN type; **fill color** encodes the executor.

- `((circle))` — start / end event
- `{diamond}` — gateway (decision)
- `[[subroutine]]` — service task — mechanical step run by the Go runtime (white)
- `[rectangle]` — user task — LLM agent (dark blue) or human STOP (yellow); `call_activity` rectangles are unfilled and link to a sub-process heading
- `[/skewed/]` — published outputs of a process (dashed border)

```mermaid
flowchart LR
    EVT((Start / End))
    GW{Gateway}
    SVC[[Service task — Go runtime]]
    AGT[Agent task — LLM]
    HUM[Human STOP]
    CALL[Call activity — sub-process]
    OUT[/Outputs/]

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class SVC serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class AGT agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class HUM humanNode

    classDef outputNode fill:#e7f0ff,stroke:#004085,stroke-width:1px,stroke-dasharray:4 2,color:#000000
    class OUT outputNode
```

## Ticket Lifecycle

```mermaid
flowchart TD
    BACKLOG_REFINEMENT[BACKLOG_REFINEMENT — see § backlog_refinement]
    END((End))
    INTAKE[INTAKE — see § GitHub Intake]
    MOVE_TICKET_IN_ACCEPTANCE[[Tick checklist + move issue to IN ACCEPTANCE]]
    MOVE_TICKET_IN_PROGRESS[["Move ticket to In Progress (bottom of lane)"]]
    PICK_TOP_READY[[Pick top Ready ticket]]
    RUN_CYCLE[RUN_CYCLE — see § Run Cycle]
    RUN_LEGACY_CYCLE[RUN_LEGACY_CYCLE — see § Run Legacy Cycle]
    START((Start))

    START -- board --> PICK_TOP_READY
    START -- specific_issue --> MOVE_TICKET_IN_PROGRESS
    PICK_TOP_READY --> MOVE_TICKET_IN_PROGRESS
    MOVE_TICKET_IN_PROGRESS --> INTAKE
    INTAKE --> RUN_LEGACY_CYCLE
    RUN_LEGACY_CYCLE --> BACKLOG_REFINEMENT
    BACKLOG_REFINEMENT --> RUN_CYCLE
    RUN_CYCLE --> MOVE_TICKET_IN_ACCEPTANCE
    MOVE_TICKET_IN_ACCEPTANCE --> END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class MOVE_TICKET_IN_ACCEPTANCE,MOVE_TICKET_IN_PROGRESS,PICK_TOP_READY serviceNode
```

## GitHub Intake

```mermaid
flowchart TD
    CLASSIFY_TICKET_SUBTYPE[[Read ticket subtype]]
    CLASSIFY_TICKET_TYPE[[Read ticket type]]
    GATE_CLASSIFY_CONFIDENT{Ticket type recognized?}
    GATE_PARSE_OK{Parsed OK?}
    GATE_SUBTYPE_OK{Subtype label detected?}
    GATE_TICKET_TYPE_INTAKE{Ticket type?}
    INTAKE_END((End))
    READ_TICKET_BODY[[Parse ticket body sections]]
    REPORT_TICKET_DETAILS[[Report intake summary]]
    STOP_CLASSIFY_CONFLICT[STOP - HUMAN REVIEW — set issue type / re-run]
    STOP_PARSE_ERROR[STOP - HUMAN REVIEW — fix ticket body / re-run]
    STOP_SUBTYPE_MISSING[STOP - HUMAN REVIEW — apply exactly one subtype:* label / re-run]

    CLASSIFY_TICKET_TYPE --> GATE_CLASSIFY_CONFIDENT
    GATE_CLASSIFY_CONFIDENT -- Yes --> GATE_TICKET_TYPE_INTAKE
    GATE_CLASSIFY_CONFIDENT -- No --> STOP_CLASSIFY_CONFLICT
    STOP_CLASSIFY_CONFLICT --> CLASSIFY_TICKET_TYPE
    GATE_TICKET_TYPE_INTAKE -- story --> READ_TICKET_BODY
    GATE_TICKET_TYPE_INTAKE -- bug --> READ_TICKET_BODY
    GATE_TICKET_TYPE_INTAKE -- task --> CLASSIFY_TICKET_SUBTYPE
    CLASSIFY_TICKET_SUBTYPE --> GATE_SUBTYPE_OK
    GATE_SUBTYPE_OK -- Yes --> READ_TICKET_BODY
    GATE_SUBTYPE_OK -- No --> STOP_SUBTYPE_MISSING
    STOP_SUBTYPE_MISSING --> CLASSIFY_TICKET_SUBTYPE
    READ_TICKET_BODY --> GATE_PARSE_OK
    GATE_PARSE_OK -- Yes --> REPORT_TICKET_DETAILS
    GATE_PARSE_OK -- No --> STOP_PARSE_ERROR
    STOP_PARSE_ERROR --> READ_TICKET_BODY
    REPORT_TICKET_DETAILS --> INTAKE_END
    GITHUB_INTAKE_OUTPUTS[/"ticket_type, subtype (tasks), change_type, parsed body sections"/]
    INTAKE_END -. produces .-> GITHUB_INTAKE_OUTPUTS

    classDef outputNode fill:#e7f0ff,stroke:#004085,stroke-width:1px,stroke-dasharray:4 2,color:#000000
    class GITHUB_INTAKE_OUTPUTS outputNode

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class CLASSIFY_TICKET_SUBTYPE,CLASSIFY_TICKET_TYPE,READ_TICKET_BODY,REPORT_TICKET_DETAILS serviceNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_CLASSIFY_CONFLICT,STOP_PARSE_ERROR,STOP_SUBTYPE_MISSING humanNode
```

## Run Legacy Cycle

```mermaid
flowchart TD
    GATE_LEGACY_PRESENT{Legacy Acceptance Criteria section present?}
    LEGACY_CYCLE[LEGACY_CYCLE — see § Legacy Acceptance Criteria Cycle]
    RUN_LEGACY_END((End))

    GATE_LEGACY_PRESENT -- Yes --> LEGACY_CYCLE
    GATE_LEGACY_PRESENT -- No --> RUN_LEGACY_END
    LEGACY_CYCLE --> RUN_LEGACY_END
```

## Run Cycle

```mermaid
flowchart TD
    AT_CYCLE[AT_CYCLE — see § AT Cycle]
    CYCLE_END((End))
    DA_CYCLE[DA_CYCLE — see § DA Cycle]
    GATE_CHANGE_TYPE{Change type?}
    SUT_CYCLE[SUT_CYCLE — see § SUT Cycle]

    GATE_CHANGE_TYPE -- behavioral --> AT_CYCLE
    GATE_CHANGE_TYPE -- system-interface-redesign --> DA_CYCLE
    GATE_CHANGE_TYPE -- external-system-interface-redesign --> DA_CYCLE
    GATE_CHANGE_TYPE -- system-implementation-refactoring --> SUT_CYCLE
    AT_CYCLE --> CYCLE_END
    DA_CYCLE --> CYCLE_END
    SUT_CYCLE --> CYCLE_END
```

## AT Cycle

```mermaid
flowchart TD
    AT_END((End))
    AT_GREEN_SYSTEM[AT_GREEN_SYSTEM — see § AT - GREEN - SYSTEM]
    AT_RED_DSL[AT - RED - DSL — see § red_phase_cycle]
    AT_RED_SYSTEM_DRIVER[AT - RED - SYSTEM DRIVER — see § red_phase_cycle]
    AT_RED_TEST[AT - RED - TEST — see § red_phase_cycle]
    AT_REFACTOR_SYSTEM[AT_REFACTOR_SYSTEM — see § at_refactor_system]
    CT_SUBPROCESS[CT_SUBPROCESS — see § Contract Test Sub-Process]
    GATE_DSL_AT{DSL Interface Changed?}
    GATE_DSL_FLAGS_PRESENT{RED-DSL phase-output flags emitted?}
    GATE_EXT_AT{External System Driver Interface Changed?}
    GATE_SYS_AT{System Driver Interface Changed?}
    STOP_FLAG_UNSET["STOP - HUMAN REVIEW — AT - RED - DSL phase-output flags missing; re-run with reminder"]
    VERIFY_AT_DRIVER[[Verify: run targeted acceptance tests after driver-adapter change]]

    AT_RED_TEST --> GATE_DSL_AT
    GATE_DSL_AT -- No --> AT_GREEN_SYSTEM
    GATE_DSL_AT -- Yes --> AT_RED_DSL
    AT_RED_DSL --> GATE_DSL_FLAGS_PRESENT
    GATE_DSL_FLAGS_PRESENT -- Yes --> GATE_EXT_AT
    GATE_DSL_FLAGS_PRESENT -- No --> STOP_FLAG_UNSET
    STOP_FLAG_UNSET --> AT_RED_DSL
    GATE_EXT_AT -- Yes --> CT_SUBPROCESS
    GATE_EXT_AT -- No --> GATE_SYS_AT
    CT_SUBPROCESS --> GATE_SYS_AT
    GATE_SYS_AT -- Yes --> AT_RED_SYSTEM_DRIVER
    GATE_SYS_AT -- No --> AT_GREEN_SYSTEM
    AT_RED_SYSTEM_DRIVER --> VERIFY_AT_DRIVER
    VERIFY_AT_DRIVER --> AT_GREEN_SYSTEM
    AT_GREEN_SYSTEM --> AT_REFACTOR_SYSTEM
    AT_REFACTOR_SYSTEM --> AT_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class VERIFY_AT_DRIVER serviceNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_FLAG_UNSET humanNode
```

## AT - GREEN - SYSTEM

```mermaid
flowchart TD
    AT_GREEN[AT - GREEN - SYSTEM - WRITE — see § green_phase_cycle]
    COMMIT[COMMIT — see § Commit Sub-Process]
    ENABLE_TESTS[Re-enable tests disabled in AT - RED - SYSTEM DRIVER]
    GS_END((End))
    MOVE_TICKET_IN_ACCEPTANCE[[Move ticket to TICKET STATUS - IN ACCEPTANCE]]
    TICK[[Tick acceptance-criteria checklist items]]

    ENABLE_TESTS --> AT_GREEN
    AT_GREEN --> COMMIT
    COMMIT --> TICK
    TICK --> MOVE_TICKET_IN_ACCEPTANCE
    MOVE_TICKET_IN_ACCEPTANCE --> GS_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class MOVE_TICKET_IN_ACCEPTANCE,TICK serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class ENABLE_TESTS agentNode
```

## Contract Test Sub-Process

```mermaid
flowchart TD
    CT_END((End))
    CT_GREEN_EXTERNAL_SYSTEM_STUB[CT - GREEN - STUBS]
    CT_RED_DSL[CT - RED - DSL — see § red_phase_cycle]
    CT_RED_EXTERNAL_SYSTEM_DRIVER[CT - RED - EXTERNAL SYSTEM DRIVER — see § red_phase_cycle]
    CT_RED_TEST[CT - RED - TEST — see § red_phase_cycle]
    GATE_DSL_CT{DSL Interface Changed?}
    GATE_EXT_CT{External System Driver Interface Changed?}
    ONBOARDING[ONBOARDING — see § External System Onboarding Sub-Process]
    VERIFY_CT_DRIVER[[Verify: run targeted contract tests after external driver-adapter change]]

    ONBOARDING --> CT_RED_TEST
    CT_RED_TEST --> GATE_DSL_CT
    GATE_DSL_CT -- No --> CT_GREEN_EXTERNAL_SYSTEM_STUB
    GATE_DSL_CT -- Yes --> CT_RED_DSL
    CT_RED_DSL --> GATE_EXT_CT
    GATE_EXT_CT -- No --> CT_GREEN_EXTERNAL_SYSTEM_STUB
    GATE_EXT_CT -- Yes --> CT_RED_EXTERNAL_SYSTEM_DRIVER
    CT_RED_EXTERNAL_SYSTEM_DRIVER --> VERIFY_CT_DRIVER
    VERIFY_CT_DRIVER --> CT_GREEN_EXTERNAL_SYSTEM_STUB
    CT_GREEN_EXTERNAL_SYSTEM_STUB --> CT_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class VERIFY_CT_DRIVER serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class CT_GREEN_EXTERNAL_SYSTEM_STUB agentNode
```

## External System Onboarding Sub-Process

```mermaid
flowchart TD
    ASK_SUPPORT[Ask user for support and STOP]
    COMMIT[COMMIT — see § Commit Sub-Process]
    DEFINE_IFACE[Define minimal Driver interface]
    GATE_DRIVER_EXISTS{External System Driver exists?}
    GATE_INSTANCE_ACCESSIBLE{Test Instance accessible?}
    GATE_SMOKE_PASS{Smoke Test passes?}
    IMPL_DRIVER[Implement Driver impl for Smoke Test]
    ONBOARD_END((End))
    PROVISION[Provision dockerized stand-in]
    RUN_SMOKE[[Run Smoke Test]]
    WRITE_SMOKE[Write single Smoke Test]

    GATE_DRIVER_EXISTS -- Yes --> ONBOARD_END
    GATE_DRIVER_EXISTS -- No --> GATE_INSTANCE_ACCESSIBLE
    GATE_INSTANCE_ACCESSIBLE -- Yes --> DEFINE_IFACE
    GATE_INSTANCE_ACCESSIBLE -- No --> PROVISION
    PROVISION --> DEFINE_IFACE
    DEFINE_IFACE --> IMPL_DRIVER
    IMPL_DRIVER --> WRITE_SMOKE
    WRITE_SMOKE --> RUN_SMOKE
    RUN_SMOKE --> GATE_SMOKE_PASS
    GATE_SMOKE_PASS -- No --> ASK_SUPPORT
    GATE_SMOKE_PASS -- Yes --> COMMIT
    COMMIT --> ONBOARD_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class RUN_SMOKE serviceNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class ASK_SUPPORT,DEFINE_IFACE,IMPL_DRIVER,PROVISION,WRITE_SMOKE humanNode
```

## Structural Cycle (shared)

```mermaid
flowchart TD
    APPROVE_CHANGE[STOP - HUMAN REVIEW — approve implementation]
    BUILD_SYSTEM[["Build system (gh optivem system build --rebuild)"]]
    CHOOSE_TESTS[["Operator picks scope (all / some suites / specific tests / skip)"]]
    COMMIT[COMMIT — see § Commit Sub-Process]
    COMPILE["Compile in-scope projects (human-gated on fail) — see § compile"]
    FIX_TEST[Dispatch fix agent on test RED — structural cycle expects green]
    GATE_STRUCT_VERIFY{"Test outcome? (ok | red — fix and retry)"}
    GATE_TESTS_SELECTED{Operator selected tests to run?}
    RUN_TESTS[["Run selected tests; classify pass/fail for the verify gate"]]
    START_SYSTEM[["Start system (gh optivem system start --restart)"]]
    STOP_TEST_FAIL_REVIEW[STOP - HUMAN REVIEW — tests RED, dispatch fix agent?]
    STRUCT_END((End))
    TICK_CHECKLIST[[Tick checklist items]]
    WRITE["${change_type} - WRITE"]

    WRITE --> APPROVE_CHANGE
    APPROVE_CHANGE --> COMPILE
    COMPILE --> CHOOSE_TESTS
    CHOOSE_TESTS --> GATE_TESTS_SELECTED
    GATE_TESTS_SELECTED -- Yes --> BUILD_SYSTEM
    GATE_TESTS_SELECTED -- No --> COMMIT
    BUILD_SYSTEM --> START_SYSTEM
    START_SYSTEM --> RUN_TESTS
    RUN_TESTS --> GATE_STRUCT_VERIFY
    GATE_STRUCT_VERIFY -- ok --> COMMIT
    GATE_STRUCT_VERIFY -- red --> STOP_TEST_FAIL_REVIEW
    STOP_TEST_FAIL_REVIEW --> FIX_TEST
    FIX_TEST --> BUILD_SYSTEM
    COMMIT --> TICK_CHECKLIST
    TICK_CHECKLIST --> STRUCT_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class BUILD_SYSTEM,CHOOSE_TESTS,RUN_TESTS,START_SYSTEM,TICK_CHECKLIST serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class FIX_TEST,WRITE agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class APPROVE_CHANGE,STOP_TEST_FAIL_REVIEW humanNode
```

## Commit Sub-Process

```mermaid
flowchart TD
    APPROVE_COMMIT[STOP - HUMAN REVIEW — approve commit?]
    COMMIT_END((End))
    EXECUTE_COMMIT[["COMMIT: <Ticket> | ${change_type}"]]

    APPROVE_COMMIT --> EXECUTE_COMMIT
    EXECUTE_COMMIT --> COMMIT_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class EXECUTE_COMMIT serviceNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class APPROVE_COMMIT humanNode
```

## Legacy Acceptance Criteria Cycle

```mermaid
flowchart TD
    GATE_LEGACY_AT_PRESENT{Legacy AT-style acceptance criteria present?}
    GATE_LEGACY_CT_PRESENT{Legacy CT-style acceptance criteria present?}
    LEGACY_AT_CYCLE[LEGACY_AT_CYCLE — see § legacy_at_cycle]
    LEGACY_CT_CYCLE[LEGACY_CT_CYCLE — see § legacy_ct_cycle]
    LEGACY_END((End))

    GATE_LEGACY_AT_PRESENT -- Yes --> LEGACY_AT_CYCLE
    GATE_LEGACY_AT_PRESENT -- No --> GATE_LEGACY_CT_PRESENT
    LEGACY_AT_CYCLE --> GATE_LEGACY_CT_PRESENT
    GATE_LEGACY_CT_PRESENT -- Yes --> LEGACY_CT_CYCLE
    GATE_LEGACY_CT_PRESENT -- No --> LEGACY_END
    LEGACY_CT_CYCLE --> LEGACY_END
```

## at_refactor_system

```mermaid
flowchart TD
    AR_END((End))
    AT_REFACTOR[AT - REFACTOR - SYSTEM - WRITE — see § green_phase_cycle]
    COMMIT[COMMIT — see § Commit Sub-Process]
    GATE_REFACTOR_CHANGED{Refactor Changed?}

    AT_REFACTOR --> GATE_REFACTOR_CHANGED
    GATE_REFACTOR_CHANGED -- Yes --> COMMIT
    GATE_REFACTOR_CHANGED -- No --> AR_END
    COMMIT --> AR_END
```

## backlog_refinement

```mermaid
flowchart TD
    BACKLOG_REFINEMENT["Refine acceptance criteria (Gherkin + coverage rubric)"]
    BR_END((End))
    CONFIRM_REFINEMENT[Confirm refined acceptance criteria]
    GATE_REFINEMENT_CHANGED{Refinement Changed?}
    MATERIALIZE_PARSED_CONCEPTS[[Materialize parsed-concepts artifact for refine-acc / update-ticket]]
    UPDATE_TICKET[Write refined ACs back to ticket source]

    MATERIALIZE_PARSED_CONCEPTS --> BACKLOG_REFINEMENT
    BACKLOG_REFINEMENT --> CONFIRM_REFINEMENT
    CONFIRM_REFINEMENT --> GATE_REFINEMENT_CHANGED
    GATE_REFINEMENT_CHANGED -- Yes --> UPDATE_TICKET
    GATE_REFINEMENT_CHANGED -- No --> BR_END
    UPDATE_TICKET --> BR_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class MATERIALIZE_PARSED_CONCEPTS serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class BACKLOG_REFINEMENT,UPDATE_TICKET agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class CONFIRM_REFINEMENT humanNode
```

## compile

```mermaid
flowchart TD
    COMPILE[["Compile (${compile_action})"]]
    COMPILE_END((End))
    FIX_COMPILE[FIX compile errors]
    GATE_COMPILE_OK{Compile passed?}
    STOP_COMPILE_FAIL_REVIEW["STOP - HUMAN REVIEW — compile failed, dispatch ${fix_agent}?"]

    COMPILE --> GATE_COMPILE_OK
    GATE_COMPILE_OK -- Yes --> COMPILE_END
    GATE_COMPILE_OK -- No --> STOP_COMPILE_FAIL_REVIEW
    STOP_COMPILE_FAIL_REVIEW --> FIX_COMPILE
    FIX_COMPILE --> COMPILE

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class COMPILE serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class FIX_COMPILE agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_COMPILE_FAIL_REVIEW humanNode
```

## DA Cycle

```mermaid
flowchart TD
    DA_END((End))
    EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE["EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE — see § Structural Cycle (shared)"]
    GATE_CHANGE_TYPE_DA{System or external-system interface?}
    SYSTEM_INTERFACE_REDESIGN_CYCLE["SYSTEM_INTERFACE_REDESIGN_CYCLE — see § Structural Cycle (shared)"]

    GATE_CHANGE_TYPE_DA -- system-interface-redesign --> SYSTEM_INTERFACE_REDESIGN_CYCLE
    GATE_CHANGE_TYPE_DA -- external-system-interface-redesign --> EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE
    SYSTEM_INTERFACE_REDESIGN_CYCLE --> DA_END
    EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE --> DA_END
```

## green_phase_cycle

```mermaid
flowchart TD
    CHECK_PHASE_SCOPE[["Check ${phase_label} scope vs allowed paths"]]
    COMPILE["Compile ${phase_label} (human-gated on fail) — see § compile"]
    GATE_PHASE_SCOPE_CLEAN{Phase scope clean?}
    GATE_SCOPE_EXCEPTION{Agent signalled scope exception?}
    GATE_TESTS_PASS{All tests passed?}
    GREEN_END((End))
    RUN[["Run targeted tests against ${suite}"]]
    STOP_GREEN_TEST_FAIL["STOP - HUMAN REVIEW — ${phase_label} tests failed"]
    STOP_SCOPE_VIOLATION["STOP - HUMAN REVIEW — ${phase_label} scope violation (Layer 1 agent-signalled or Layer 2 post-phase diff)"]
    WRITE["${phase_label} - WRITE"]

    WRITE --> GATE_SCOPE_EXCEPTION
    GATE_SCOPE_EXCEPTION -- Yes --> STOP_SCOPE_VIOLATION
    GATE_SCOPE_EXCEPTION -- No --> COMPILE
    STOP_SCOPE_VIOLATION --> WRITE
    COMPILE --> RUN
    RUN --> GATE_TESTS_PASS
    GATE_TESTS_PASS -- Yes --> CHECK_PHASE_SCOPE
    GATE_TESTS_PASS -- No --> STOP_GREEN_TEST_FAIL
    STOP_GREEN_TEST_FAIL --> WRITE
    CHECK_PHASE_SCOPE --> GATE_PHASE_SCOPE_CLEAN
    GATE_PHASE_SCOPE_CLEAN -- Yes --> GREEN_END
    GATE_PHASE_SCOPE_CLEAN -- No --> STOP_SCOPE_VIOLATION

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class CHECK_PHASE_SCOPE,RUN serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class WRITE agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_GREEN_TEST_FAIL,STOP_SCOPE_VIOLATION humanNode
```

## legacy_at_cycle

```mermaid
flowchart TD
    GATE_DSL_LEGACY_AT{DSL Interface Changed?}
    GATE_SYS_LEGACY_AT{System Driver Interface Changed?}
    GATE_VERIFY_LEGACY_AT{"Legacy AT verify outcome? (ok = passed on first run | red = test/DSL/driver wrong)"}
    LEGACY_AT_DSL[LEGACY - AT - DSL]
    LEGACY_AT_END((End))
    LEGACY_AT_SYSTEM_DRIVER[LEGACY - AT - SYSTEM DRIVER]
    LEGACY_AT_TEST[LEGACY - AT - TEST]
    STOP_LEGACY_AT_VERIFY_FAILED["STOP - HUMAN REVIEW — legacy AT verify RED; the test/DSL/driver is suspect (SUT is never modified in a legacy cycle). Edit the offending layer and re-run the legacy cycle from scratch."]
    VERIFY_LEGACY_AT[[Verify: run assembled legacy AT tests — inverted-RED, expected to PASS on first run]]

    LEGACY_AT_TEST --> GATE_DSL_LEGACY_AT
    GATE_DSL_LEGACY_AT -- Yes --> LEGACY_AT_DSL
    GATE_DSL_LEGACY_AT -- No --> GATE_SYS_LEGACY_AT
    LEGACY_AT_DSL --> GATE_SYS_LEGACY_AT
    GATE_SYS_LEGACY_AT -- Yes --> LEGACY_AT_SYSTEM_DRIVER
    GATE_SYS_LEGACY_AT -- No --> VERIFY_LEGACY_AT
    LEGACY_AT_SYSTEM_DRIVER --> VERIFY_LEGACY_AT
    VERIFY_LEGACY_AT --> GATE_VERIFY_LEGACY_AT
    GATE_VERIFY_LEGACY_AT -- ok --> LEGACY_AT_END
    GATE_VERIFY_LEGACY_AT -- red --> STOP_LEGACY_AT_VERIFY_FAILED
    STOP_LEGACY_AT_VERIFY_FAILED --> LEGACY_AT_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class VERIFY_LEGACY_AT serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class LEGACY_AT_DSL,LEGACY_AT_SYSTEM_DRIVER,LEGACY_AT_TEST agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_LEGACY_AT_VERIFY_FAILED humanNode
```

## legacy_ct_cycle

```mermaid
flowchart TD
    GATE_DSL_LEGACY_CT{DSL Interface Changed?}
    GATE_EXT_LEGACY_CT{External System Driver Interface Changed?}
    GATE_VERIFY_LEGACY_CT{"Legacy CT verify outcome? (ok = passed on first run | red = test/DSL/driver/stub wrong)"}
    LEGACY_CT_DSL[LEGACY - CT - DSL]
    LEGACY_CT_END((End))
    LEGACY_CT_EXTERNAL_SYSTEM_DRIVER[LEGACY - CT - EXTERNAL SYSTEM DRIVER]
    LEGACY_CT_EXTERNAL_SYSTEM_STUB[LEGACY - CT - EXTERNAL SYSTEM STUB]
    LEGACY_CT_TEST[LEGACY - CT - TEST]
    STOP_LEGACY_CT_VERIFY_FAILED["STOP - HUMAN REVIEW — legacy CT verify RED; the test/DSL/driver/stub is suspect (SUT is never modified in a legacy cycle). Edit the offending layer and re-run the legacy cycle from scratch."]
    VERIFY_LEGACY_CT[[Verify: run assembled legacy CT tests — inverted-RED, expected to PASS on first run]]

    LEGACY_CT_TEST --> GATE_DSL_LEGACY_CT
    GATE_DSL_LEGACY_CT -- Yes --> LEGACY_CT_DSL
    GATE_DSL_LEGACY_CT -- No --> LEGACY_CT_EXTERNAL_SYSTEM_STUB
    LEGACY_CT_DSL --> GATE_EXT_LEGACY_CT
    GATE_EXT_LEGACY_CT -- Yes --> LEGACY_CT_EXTERNAL_SYSTEM_DRIVER
    GATE_EXT_LEGACY_CT -- No --> LEGACY_CT_EXTERNAL_SYSTEM_STUB
    LEGACY_CT_EXTERNAL_SYSTEM_DRIVER --> LEGACY_CT_EXTERNAL_SYSTEM_STUB
    LEGACY_CT_EXTERNAL_SYSTEM_STUB --> VERIFY_LEGACY_CT
    VERIFY_LEGACY_CT --> GATE_VERIFY_LEGACY_CT
    GATE_VERIFY_LEGACY_CT -- ok --> LEGACY_CT_END
    GATE_VERIFY_LEGACY_CT -- red --> STOP_LEGACY_CT_VERIFY_FAILED
    STOP_LEGACY_CT_VERIFY_FAILED --> LEGACY_CT_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class VERIFY_LEGACY_CT serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class LEGACY_CT_DSL,LEGACY_CT_EXTERNAL_SYSTEM_DRIVER,LEGACY_CT_EXTERNAL_SYSTEM_STUB,LEGACY_CT_TEST agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_LEGACY_CT_VERIFY_FAILED humanNode
```

## red_phase_cycle

```mermaid
flowchart TD
    CHECK_PHASE_SCOPE[["Check ${phase_label} scope vs allowed paths"]]
    COMMIT[COMMIT — see § Commit Sub-Process]
    COMPILE["Compile ${phase_label} (human-gated on fail) — see § compile"]
    DISABLE[Disable change-driven scenarios]
    GATE_PHASE_SCOPE_CLEAN{Phase scope clean?}
    GATE_RUN_FAILED_RUNTIME{"Tests fail at runtime (not compile)?"}
    GATE_SCOPE_EXCEPTION{Agent signalled scope exception?}
    GATE_VERIFY_REAL_PASS{Real suite passes?}
    GATE_VERIFY_REAL_REQUIRED{Verify against real suite first?}
    RED_END((End))
    RUN[[Run targeted tests]]
    STOP_RED_NOT_RUNTIME_FAIL["STOP - HUMAN REVIEW — ${phase_label} tests not runtime-failing"]
    STOP_RED_REVIEW["STOP - HUMAN REVIEW — ${phase_label} test + DSL stubs"]
    STOP_SCOPE_VIOLATION["STOP - HUMAN REVIEW — ${phase_label} scope violation (Layer 1 agent-signalled or Layer 2 post-phase diff)"]
    STOP_VERIFY_REAL_FAIL["STOP - HUMAN REVIEW — ${phase_label} real-suite contract problem"]
    VERIFY_REAL[["Verify ${verify_real_suite} passes"]]
    WRITE["${phase_label} - WRITE"]

    WRITE --> GATE_SCOPE_EXCEPTION
    GATE_SCOPE_EXCEPTION -- Yes --> STOP_SCOPE_VIOLATION
    GATE_SCOPE_EXCEPTION -- No --> STOP_RED_REVIEW
    STOP_SCOPE_VIOLATION --> WRITE
    STOP_RED_REVIEW --> COMPILE
    COMPILE --> GATE_VERIFY_REAL_REQUIRED
    GATE_VERIFY_REAL_REQUIRED -- Yes --> VERIFY_REAL
    GATE_VERIFY_REAL_REQUIRED -- No --> RUN
    VERIFY_REAL --> GATE_VERIFY_REAL_PASS
    GATE_VERIFY_REAL_PASS -- Yes --> RUN
    GATE_VERIFY_REAL_PASS -- No --> STOP_VERIFY_REAL_FAIL
    STOP_VERIFY_REAL_FAIL --> WRITE
    RUN --> GATE_RUN_FAILED_RUNTIME
    GATE_RUN_FAILED_RUNTIME -- Yes --> DISABLE
    GATE_RUN_FAILED_RUNTIME -- No --> STOP_RED_NOT_RUNTIME_FAIL
    STOP_RED_NOT_RUNTIME_FAIL --> WRITE
    DISABLE --> CHECK_PHASE_SCOPE
    CHECK_PHASE_SCOPE --> GATE_PHASE_SCOPE_CLEAN
    GATE_PHASE_SCOPE_CLEAN -- Yes --> COMMIT
    GATE_PHASE_SCOPE_CLEAN -- No --> STOP_SCOPE_VIOLATION
    COMMIT --> RED_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class CHECK_PHASE_SCOPE,RUN,VERIFY_REAL serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class DISABLE,WRITE agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_RED_NOT_RUNTIME_FAIL,STOP_RED_REVIEW,STOP_SCOPE_VIOLATION,STOP_VERIFY_REAL_FAIL humanNode
```

## SUT Cycle

```mermaid
flowchart TD
    SUT_END((End))
    SYSTEM_IMPLEMENTATION_REFACTORING_CYCLE["SYSTEM_IMPLEMENTATION_REFACTORING_CYCLE — see § Structural Cycle (shared)"]

    SYSTEM_IMPLEMENTATION_REFACTORING_CYCLE --> SUT_END
```

