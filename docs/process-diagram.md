# ATDD Process Flow

> Generated from `internal/atdd/runtime/statemachine/process-flow.yaml` by `internal/atdd/runtime/diagram`. Do not edit by hand — edit the YAML and regenerate via `gh optivem atdd show diagram > docs/process-diagram.md`.

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
    RUN_LEGACY_CYCLE --> RUN_CYCLE
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
    GATE_CHANGE_TYPE -- system-implementation-change --> SUT_CYCLE
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
    CT_SUBPROCESS[CT_SUBPROCESS — see § Contract Test Sub-Process]
    GATE_DSL_AT{DSL Interface Changed?}
    GATE_EXT_AT{External System Driver Interface Changed?}
    GATE_SYS_AT{System Driver Interface Changed?}
    VERIFY_AT_DRIVER[[Verify: run targeted acceptance tests after driver-adapter change]]

    AT_RED_TEST --> GATE_DSL_AT
    GATE_DSL_AT -- No --> AT_GREEN_SYSTEM
    GATE_DSL_AT -- Yes --> AT_RED_DSL
    AT_RED_DSL --> GATE_EXT_AT
    GATE_EXT_AT -- Yes --> CT_SUBPROCESS
    GATE_EXT_AT -- No --> GATE_SYS_AT
    CT_SUBPROCESS --> GATE_SYS_AT
    GATE_SYS_AT -- Yes --> AT_RED_SYSTEM_DRIVER
    GATE_SYS_AT -- No --> AT_GREEN_SYSTEM
    AT_RED_SYSTEM_DRIVER --> VERIFY_AT_DRIVER
    VERIFY_AT_DRIVER --> AT_GREEN_SYSTEM
    AT_GREEN_SYSTEM --> AT_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class VERIFY_AT_DRIVER serviceNode
```

## AT - GREEN - SYSTEM

```mermaid
flowchart TD
    AT_GREEN_BACKEND["AT - GREEN - SYSTEM - WRITE (backend) — see § green_phase_cycle"]
    AT_GREEN_FRONTEND["AT - GREEN - SYSTEM - WRITE (frontend) — see § green_phase_cycle"]
    COMMIT[COMMIT — see § Commit Sub-Process]
    ENABLE_TESTS[[Re-enable tests disabled in AT - RED - SYSTEM DRIVER]]
    GS_END((End))
    MOVE_TICKET_IN_ACCEPTANCE[[Move ticket to TICKET STATUS - IN ACCEPTANCE]]
    TICK[[Tick acceptance-criteria checklist items]]

    ENABLE_TESTS --> AT_GREEN_BACKEND
    AT_GREEN_BACKEND --> AT_GREEN_FRONTEND
    AT_GREEN_FRONTEND --> COMMIT
    COMMIT --> TICK
    TICK --> MOVE_TICKET_IN_ACCEPTANCE
    MOVE_TICKET_IN_ACCEPTANCE --> GS_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class ENABLE_TESTS,MOVE_TICKET_IN_ACCEPTANCE,TICK serviceNode
```

## Contract Test Sub-Process

```mermaid
flowchart TD
    CT_END((End))
    CT_GREEN_STUBS[CT - GREEN - STUBS]
    CT_RED_DSL[CT - RED - DSL — see § red_phase_cycle]
    CT_RED_EXTERNAL_DRIVER[CT - RED - EXTERNAL DRIVER — see § red_phase_cycle]
    CT_RED_TEST[CT - RED - TEST — see § red_phase_cycle]
    GATE_DSL_CT{DSL Interface Changed?}
    GATE_EXT_CT{External System Driver Interface Changed?}
    ONBOARDING[ONBOARDING — see § External System Onboarding Sub-Process]
    VERIFY_CT_DRIVER[[Verify: run targeted contract tests after external driver-adapter change]]

    ONBOARDING --> CT_RED_TEST
    CT_RED_TEST --> GATE_DSL_CT
    GATE_DSL_CT -- No --> CT_GREEN_STUBS
    GATE_DSL_CT -- Yes --> CT_RED_DSL
    CT_RED_DSL --> GATE_EXT_CT
    GATE_EXT_CT -- No --> CT_GREEN_STUBS
    GATE_EXT_CT -- Yes --> CT_RED_EXTERNAL_DRIVER
    CT_RED_EXTERNAL_DRIVER --> VERIFY_CT_DRIVER
    VERIFY_CT_DRIVER --> CT_GREEN_STUBS
    CT_GREEN_STUBS --> CT_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class VERIFY_CT_DRIVER serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class CT_GREEN_STUBS agentNode
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
    COMPILE[[Compile in-scope projects]]
    FIX_COMPILE[Dispatch fix agent on compile RED — structural cycle expects green]
    FIX_TEST[Dispatch fix agent on test RED — structural cycle expects green]
    GATE_COMPILE_OK{Compile passed?}
    GATE_STRUCT_VERIFY{"Test outcome? (ok | red — fix and retry)"}
    GATE_TESTS_SELECTED{Operator selected tests to run?}
    RUN_TESTS[["Run selected tests; classify pass/fail for the verify gate"]]
    START_SYSTEM[["Start system (gh optivem system start --restart)"]]
    STOP_COMPILE_FAIL_REVIEW[STOP - HUMAN REVIEW — compile RED, dispatch fix agent?]
    STOP_TEST_FAIL_REVIEW[STOP - HUMAN REVIEW — tests RED, dispatch fix agent?]
    STRUCT_END((End))
    TICK_CHECKLIST[[Tick checklist items]]
    WRITE["${change_type} - WRITE"]

    WRITE --> APPROVE_CHANGE
    APPROVE_CHANGE --> COMPILE
    COMPILE --> GATE_COMPILE_OK
    GATE_COMPILE_OK -- Yes --> CHOOSE_TESTS
    GATE_COMPILE_OK -- No --> STOP_COMPILE_FAIL_REVIEW
    STOP_COMPILE_FAIL_REVIEW --> FIX_COMPILE
    FIX_COMPILE --> COMPILE
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
    class BUILD_SYSTEM,CHOOSE_TESTS,COMPILE,RUN_TESTS,START_SYSTEM,TICK_CHECKLIST serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class FIX_COMPILE,FIX_TEST,WRITE agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class APPROVE_CHANGE,STOP_COMPILE_FAIL_REVIEW,STOP_TEST_FAIL_REVIEW humanNode
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
    LEGACY_END((End))
    LEGACY_TBD[STOP - HUMAN REVIEW — Legacy Acceptance Criteria Cycle TBD]

    LEGACY_TBD --> LEGACY_END

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class LEGACY_TBD humanNode
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
    COMPILE[["Compile (${compile_action})"]]
    GATE_COMPILE_OK{Compile passed?}
    GATE_TESTS_PASS{All tests passed?}
    GREEN_END((End))
    RUN[["Run targeted tests against ${suite}"]]
    STOP_GREEN_COMPILE_FAIL["STOP - HUMAN REVIEW — ${phase_label} compile failed"]
    STOP_GREEN_TEST_FAIL["STOP - HUMAN REVIEW — ${phase_label} tests failed"]
    WRITE["${phase_label} - WRITE"]

    WRITE --> COMPILE
    COMPILE --> GATE_COMPILE_OK
    GATE_COMPILE_OK -- No --> STOP_GREEN_COMPILE_FAIL
    GATE_COMPILE_OK -- Yes --> RUN
    STOP_GREEN_COMPILE_FAIL --> WRITE
    RUN --> GATE_TESTS_PASS
    GATE_TESTS_PASS -- Yes --> GREEN_END
    GATE_TESTS_PASS -- No --> STOP_GREEN_TEST_FAIL
    STOP_GREEN_TEST_FAIL --> WRITE

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class COMPILE,RUN serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class WRITE agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_GREEN_COMPILE_FAIL,STOP_GREEN_TEST_FAIL humanNode
```

## red_phase_cycle

```mermaid
flowchart TD
    COMMIT[COMMIT — see § Commit Sub-Process]
    COMPILE[["Compile (${compile_action})"]]
    DISABLE[[Disable change-driven scenarios]]
    GATE_COMPILE_OK{Compile passed?}
    GATE_RUN_FAILED_RUNTIME{"Tests fail at runtime (not compile)?"}
    GATE_VERIFY_REAL_PASS{Real suite passes?}
    GATE_VERIFY_REAL_REQUIRED{Verify against real suite first?}
    RED_END((End))
    RUN[[Run targeted tests]]
    STOP_PROTOTYPE_REVIEW["STOP - HUMAN REVIEW — ${phase_label} prototypes"]
    STOP_RED_NOT_RUNTIME_FAIL["STOP - HUMAN REVIEW — ${phase_label} tests not runtime-failing"]
    STOP_RED_REVIEW["STOP - HUMAN REVIEW — ${phase_label} tests"]
    STOP_VERIFY_REAL_FAIL["STOP - HUMAN REVIEW — ${phase_label} real-suite contract problem"]
    VERIFY_REAL[["Verify ${verify_real_suite} passes"]]
    WRITE["${phase_label} - WRITE"]
    WRITE_PROTOTYPES["${phase_label} - PROTOTYPES"]

    WRITE --> STOP_RED_REVIEW
    STOP_RED_REVIEW --> COMPILE
    COMPILE --> GATE_COMPILE_OK
    GATE_COMPILE_OK -- No --> WRITE_PROTOTYPES
    GATE_COMPILE_OK -- Yes --> GATE_VERIFY_REAL_REQUIRED
    WRITE_PROTOTYPES --> STOP_PROTOTYPE_REVIEW
    STOP_PROTOTYPE_REVIEW --> COMPILE
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
    DISABLE --> COMMIT
    COMMIT --> RED_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class COMPILE,DISABLE,RUN,VERIFY_REAL serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class WRITE,WRITE_PROTOTYPES agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_PROTOTYPE_REVIEW,STOP_RED_NOT_RUNTIME_FAIL,STOP_RED_REVIEW,STOP_VERIFY_REAL_FAIL humanNode
```

## SUT Cycle

```mermaid
flowchart TD
    CHORE_CYCLE["CHORE_CYCLE — see § Structural Cycle (shared)"]
    SUT_END((End))

    CHORE_CYCLE --> SUT_END
```

