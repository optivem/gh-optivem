# ATDD Process Flow

> Generated from `internal/atdd/runtime/statemachine/process-flow.yaml` by `internal/atdd/runtime/diagram`. Do not edit by hand — edit the YAML and regenerate via `gh optivem atdd show diagram > docs/process-diagram.md`.

Each section corresponds to one named flow in the YAML. `call_activity` nodes appear as boxes pointing at the linked sub-flow's heading.

## Ticket Lifecycle

```mermaid
flowchart TD
    END((End))
    INTAKE[INTAKE — see § GitHub Intake]
    MOVE_TO_IN_PROGRESS[["Move ticket to In Progress (bottom of lane)"]]
    PICK_TOP_READY[[Pick top Ready ticket]]
    RUN_CYCLE[RUN_CYCLE — see § Run Cycle]
    RUN_LEGACY_CYCLE[RUN_LEGACY_CYCLE — see § Run Legacy Cycle]
    START((Start))
    TICKET_IN_ACCEPTANCE[[Tick checklist + move issue to IN ACCEPTANCE]]

    START -- board --> PICK_TOP_READY
    START -- specific_issue --> MOVE_TO_IN_PROGRESS
    PICK_TOP_READY --> MOVE_TO_IN_PROGRESS
    MOVE_TO_IN_PROGRESS --> INTAKE
    INTAKE --> RUN_LEGACY_CYCLE
    RUN_LEGACY_CYCLE --> RUN_CYCLE
    RUN_CYCLE --> TICKET_IN_ACCEPTANCE
    TICKET_IN_ACCEPTANCE --> END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class MOVE_TO_IN_PROGRESS,PICK_TOP_READY,TICKET_IN_ACCEPTANCE serviceNode
```

## GitHub Intake

```mermaid
flowchart TD
    CLASSIFY[[Read ticket type]]
    CLASSIFY_SUBTYPE[[Read ticket subtype]]
    GATE_CLASSIFY_CONFIDENT{Ticket type recognized?}
    GATE_PARSE_OK{Parsed OK?}
    GATE_SUBTYPE_OK{Subtype label detected?}
    GATE_TICKET_TYPE_INTAKE{Ticket type?}
    INTAKE_END((End))
    PARSE_BODY[[Parse ticket body sections]]
    REPORT_INTAKE_SUMMARY[[Report intake summary]]
    STOP_CLASSIFY_CONFLICT[STOP - HUMAN REVIEW — set issue type / re-run]
    STOP_PARSE_ERROR[STOP - HUMAN REVIEW — fix ticket body / re-run]
    STOP_SUBTYPE_MISSING[STOP - HUMAN REVIEW — apply exactly one subtype:* label / re-run]

    CLASSIFY --> GATE_CLASSIFY_CONFIDENT
    GATE_CLASSIFY_CONFIDENT -- Yes --> GATE_TICKET_TYPE_INTAKE
    GATE_CLASSIFY_CONFIDENT -- No --> STOP_CLASSIFY_CONFLICT
    STOP_CLASSIFY_CONFLICT --> CLASSIFY
    GATE_TICKET_TYPE_INTAKE -- story --> PARSE_BODY
    GATE_TICKET_TYPE_INTAKE -- bug --> PARSE_BODY
    GATE_TICKET_TYPE_INTAKE -- task --> CLASSIFY_SUBTYPE
    CLASSIFY_SUBTYPE --> GATE_SUBTYPE_OK
    GATE_SUBTYPE_OK -- Yes --> PARSE_BODY
    GATE_SUBTYPE_OK -- No --> STOP_SUBTYPE_MISSING
    STOP_SUBTYPE_MISSING --> CLASSIFY_SUBTYPE
    PARSE_BODY --> GATE_PARSE_OK
    GATE_PARSE_OK -- Yes --> REPORT_INTAKE_SUMMARY
    GATE_PARSE_OK -- No --> STOP_PARSE_ERROR
    STOP_PARSE_ERROR --> PARSE_BODY
    REPORT_INTAKE_SUMMARY --> INTAKE_END
    GITHUB_INTAKE_OUTPUTS[/"ticket_type, subtype (tasks), change_type, parsed body sections"/]
    INTAKE_END -. produces .-> GITHUB_INTAKE_OUTPUTS

    classDef outputNode fill:#e7f0ff,stroke:#004085,stroke-width:1px,stroke-dasharray:4 2,color:#000000
    class GITHUB_INTAKE_OUTPUTS outputNode

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class CLASSIFY,CLASSIFY_SUBTYPE,PARSE_BODY,REPORT_INTAKE_SUMMARY serviceNode

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
    AT_RED_DSL[AT - RED - DSL]
    AT_RED_SYSTEM_DRIVER[AT - RED - SYSTEM DRIVER]
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

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class AT_RED_DSL,AT_RED_SYSTEM_DRIVER agentNode
```

## AT - GREEN - SYSTEM

```mermaid
flowchart TD
    ATDD_BACKEND["AT - GREEN - SYSTEM - WRITE (backend)"]
    ATDD_FRONTEND["AT - GREEN - SYSTEM - WRITE (frontend)"]
    ATDD_RELEASE["AT - GREEN - SYSTEM - COMMIT (atdd-release)"]
    GS_END((End))
    STOP_GREEN_REVIEW[STOP - HUMAN REVIEW — approve implementation]

    ATDD_BACKEND --> ATDD_FRONTEND
    ATDD_FRONTEND --> STOP_GREEN_REVIEW
    STOP_GREEN_REVIEW --> ATDD_RELEASE
    ATDD_RELEASE --> GS_END

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class ATDD_BACKEND,ATDD_FRONTEND,ATDD_RELEASE agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_GREEN_REVIEW humanNode
```

## Contract Test Sub-Process

```mermaid
flowchart TD
    CT_END((End))
    CT_GREEN_STUBS[CT - GREEN - STUBS]
    CT_RED_DSL[CT - RED - DSL]
    CT_RED_EXTERNAL_DRIVER[CT - RED - EXTERNAL DRIVER]
    CT_RED_TEST[CT - RED - TEST]
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
    class CT_GREEN_STUBS,CT_RED_DSL,CT_RED_EXTERNAL_DRIVER,CT_RED_TEST agentNode
```

## External System Onboarding Sub-Process

```mermaid
flowchart TD
    ASK_SUPPORT[Ask user for support and STOP]
    COMMIT_ONBOARD[["COMMIT: External System Onboarding | <name>"]]
    DEFINE_IFACE[Define minimal Driver interface]
    GATE_DRIVER_EXISTS{External System Driver exists?}
    GATE_INSTANCE_ACCESSIBLE{Test Instance accessible?}
    GATE_SMOKE_PASS{Smoke Test passes?}
    IMPL_DRIVER[Implement Driver impl for Smoke Test]
    ONBOARD_END((End))
    PROVISION[Provision dockerized stand-in]
    RUN_SMOKE[[Run Smoke Test]]
    STOP_ONBOARD_REVIEW[STOP - HUMAN REVIEW — approve onboarding artifacts]
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
    GATE_SMOKE_PASS -- Yes --> STOP_ONBOARD_REVIEW
    STOP_ONBOARD_REVIEW --> COMMIT_ONBOARD
    COMMIT_ONBOARD --> ONBOARD_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class COMMIT_ONBOARD,RUN_SMOKE serviceNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class ASK_SUPPORT,DEFINE_IFACE,IMPL_DRIVER,PROVISION,STOP_ONBOARD_REVIEW,WRITE_SMOKE humanNode
```

## Structural Cycle (shared)

```mermaid
flowchart TD
    ASK_COMMIT[[Ask: Can I commit?]]
    COMMIT_STRUCT[["COMMIT: <Ticket> | ${change_type}"]]
    COMPILE[[Compile in-scope projects]]
    DRIFT[[Report drift warning if applicable]]
    FIX_STRUCT_VERIFY[Dispatch fix agent on verify RED — structural cycle expects green]
    GATE_STRUCT_VERIFY{"Verify outcome? (ok | red — fix and retry)"}
    GATE_TEST_MODE{"TEST mode? (full | compile | skip)"}
    SAMPLE[[Run sample suite]]
    STOP_STRUCT_REVIEW[STOP - HUMAN REVIEW — approve implementation]
    STOP_STRUCT_TEST[STOP - HUMAN REVIEW — review TEST results]
    STRUCT_END((End))
    STRUCT_WRITE["${change_type} - WRITE"]
    TICK[[Tick checklist items]]
    VERIFY_STRUCT_DRIVER[["Verify: run targeted tests if driver-adapter changed (no-op for chore)"]]

    STRUCT_WRITE --> VERIFY_STRUCT_DRIVER
    VERIFY_STRUCT_DRIVER --> GATE_STRUCT_VERIFY
    GATE_STRUCT_VERIFY -- ok --> STOP_STRUCT_REVIEW
    GATE_STRUCT_VERIFY -- red --> FIX_STRUCT_VERIFY
    FIX_STRUCT_VERIFY --> VERIFY_STRUCT_DRIVER
    STOP_STRUCT_REVIEW --> GATE_TEST_MODE
    GATE_TEST_MODE -- skip --> ASK_COMMIT
    GATE_TEST_MODE -- compile / full --> COMPILE
    COMPILE -- full --> SAMPLE
    COMPILE -- compile --> DRIFT
    SAMPLE --> DRIFT
    DRIFT --> STOP_STRUCT_TEST
    STOP_STRUCT_TEST --> ASK_COMMIT
    ASK_COMMIT --> COMMIT_STRUCT
    COMMIT_STRUCT --> TICK
    TICK --> STRUCT_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class ASK_COMMIT,COMMIT_STRUCT,COMPILE,DRIFT,SAMPLE,TICK,VERIFY_STRUCT_DRIVER serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class FIX_STRUCT_VERIFY,STRUCT_WRITE agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_STRUCT_REVIEW,STOP_STRUCT_TEST humanNode
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
    EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE[EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE — see § Contract Test Sub-Process]
    GATE_CHANGE_TYPE_DA{System or external-system interface?}
    SYSTEM_INTERFACE_REDESIGN_CYCLE["SYSTEM_INTERFACE_REDESIGN_CYCLE — see § Structural Cycle (shared)"]

    GATE_CHANGE_TYPE_DA -- system-interface-redesign --> SYSTEM_INTERFACE_REDESIGN_CYCLE
    GATE_CHANGE_TYPE_DA -- external-system-interface-redesign --> EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE
    SYSTEM_INTERFACE_REDESIGN_CYCLE --> DA_END
    EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE --> DA_END
```

## red_phase_cycle

```mermaid
flowchart TD
    COMMIT[["COMMIT: <Ticket> | ${change_type}"]]
    COMPILE[[Compile in scope]]
    DISABLE[[Disable change-driven scenarios]]
    GATE_COMPILE_OK{Compile passed?}
    GATE_RUN_FAILED_RUNTIME{"Tests fail at runtime (not compile)?"}
    RED_END((End))
    RUN[[Run targeted tests]]
    STOP_DSL_PROTOTYPE_REVIEW["STOP - HUMAN REVIEW — ${phase_label} DSL prototypes"]
    STOP_RED_NOT_RUNTIME_FAIL["STOP - HUMAN REVIEW — ${phase_label} tests not runtime-failing"]
    STOP_RED_REVIEW["STOP - HUMAN REVIEW — ${phase_label} tests"]
    WRITE["${phase_label} - WRITE"]
    WRITE_DSL_PROTOTYPES["${phase_label} - DSL PROTOTYPES"]

    WRITE --> STOP_RED_REVIEW
    STOP_RED_REVIEW --> COMPILE
    COMPILE --> GATE_COMPILE_OK
    GATE_COMPILE_OK -- No --> WRITE_DSL_PROTOTYPES
    GATE_COMPILE_OK -- Yes --> RUN
    WRITE_DSL_PROTOTYPES --> STOP_DSL_PROTOTYPE_REVIEW
    STOP_DSL_PROTOTYPE_REVIEW --> COMPILE
    RUN --> GATE_RUN_FAILED_RUNTIME
    GATE_RUN_FAILED_RUNTIME -- Yes --> DISABLE
    GATE_RUN_FAILED_RUNTIME -- No --> STOP_RED_NOT_RUNTIME_FAIL
    STOP_RED_NOT_RUNTIME_FAIL --> WRITE
    DISABLE --> COMMIT
    COMMIT --> RED_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class COMMIT,COMPILE,DISABLE,RUN serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class WRITE,WRITE_DSL_PROTOTYPES agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_DSL_PROTOTYPE_REVIEW,STOP_RED_NOT_RUNTIME_FAIL,STOP_RED_REVIEW humanNode
```

## SUT Cycle

```mermaid
flowchart TD
    CHORE_CYCLE["CHORE_CYCLE — see § Structural Cycle (shared)"]
    SUT_END((End))

    CHORE_CYCLE --> SUT_END
```

