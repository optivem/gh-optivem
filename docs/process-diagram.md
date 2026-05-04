# ATDD Process Flow

> Generated from `internal/atdd/runtime/statemachine/process-flow.yaml` by `internal/atdd/runtime/diagram`. Do not edit by hand — edit the YAML and regenerate via `gh optivem atdd show diagram > docs/process-diagram.md`.

Each section corresponds to one named flow in the YAML. `call_activity` nodes appear as boxes pointing at the linked sub-flow's heading.

## Ticket Lifecycle

```mermaid
flowchart TD
    END((End))
    INTAKE[INTAKE — see § Intake]
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

## Intake

```mermaid
flowchart TD
    ATDD_BUG["atdd-bug (intake)"]
    ATDD_CHORE["atdd-chore (intake)"]
    ATDD_STORY["atdd-story (intake)"]
    ATDD_TASK["atdd-task (intake)"]
    CLASSIFY[[Auto-classify ticket]]
    GATE_CLASSIFY_CONFIDENT{Classification confident?}
    GATE_TICKET_TYPE{Ticket Type?}
    INTAKE_END((End))
    STOP_CLASSIFY_CONFLICT[STOP - HUMAN REVIEW — resolve classification conflict]
    STOP_INTAKE[STOP - HUMAN REVIEW — approve scenarios]

    CLASSIFY --> GATE_CLASSIFY_CONFIDENT
    GATE_CLASSIFY_CONFIDENT -- Yes --> GATE_TICKET_TYPE
    GATE_CLASSIFY_CONFIDENT -- No --> STOP_CLASSIFY_CONFLICT
    STOP_CLASSIFY_CONFLICT --> GATE_TICKET_TYPE
    GATE_TICKET_TYPE -- story --> ATDD_STORY
    GATE_TICKET_TYPE -- bug --> ATDD_BUG
    GATE_TICKET_TYPE -- system-api-task / system-ui-task / external-api-task --> ATDD_TASK
    GATE_TICKET_TYPE -- chore --> ATDD_CHORE
    ATDD_STORY --> STOP_INTAKE
    ATDD_BUG --> STOP_INTAKE
    ATDD_TASK --> STOP_INTAKE
    ATDD_CHORE --> STOP_INTAKE
    STOP_INTAKE --> INTAKE_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class CLASSIFY serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class ATDD_BUG,ATDD_CHORE,ATDD_STORY,ATDD_TASK agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_CLASSIFY_CONFLICT,STOP_INTAKE humanNode
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
    GATE_CHANGE_SUBTYPE{Structural change subtype?}
    GATE_CHANGE_TYPE{Cycle by change type}
    SUT_CYCLE[SUT_CYCLE — see § SUT Cycle]

    GATE_CHANGE_TYPE -- behavior --> AT_CYCLE
    GATE_CHANGE_TYPE -- structure --> GATE_CHANGE_SUBTYPE
    GATE_CHANGE_SUBTYPE -- interface --> DA_CYCLE
    GATE_CHANGE_SUBTYPE -- implementation --> SUT_CYCLE
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
    AT_RED_TEST[AT - RED - TEST]
    CT_SUBPROCESS[CT_SUBPROCESS — see § Contract Test Sub-Process]
    GATE_DSL_AT{DSL Interface Changed?}
    GATE_EXT_AT{External System Driver Interface Changed?}
    GATE_SYS_AT{System Driver Interface Changed?}

    AT_RED_TEST --> GATE_DSL_AT
    GATE_DSL_AT -- No --> AT_GREEN_SYSTEM
    GATE_DSL_AT -- Yes --> AT_RED_DSL
    AT_RED_DSL --> GATE_EXT_AT
    GATE_EXT_AT -- Yes --> CT_SUBPROCESS
    GATE_EXT_AT -- No --> GATE_SYS_AT
    CT_SUBPROCESS --> GATE_SYS_AT
    GATE_SYS_AT -- Yes --> AT_RED_SYSTEM_DRIVER
    GATE_SYS_AT -- No --> AT_GREEN_SYSTEM
    AT_RED_SYSTEM_DRIVER --> AT_GREEN_SYSTEM
    AT_GREEN_SYSTEM --> AT_END

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class AT_RED_DSL,AT_RED_SYSTEM_DRIVER,AT_RED_TEST agentNode
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

    ONBOARDING --> CT_RED_TEST
    CT_RED_TEST --> GATE_DSL_CT
    GATE_DSL_CT -- No --> CT_GREEN_STUBS
    GATE_DSL_CT -- Yes --> CT_RED_DSL
    CT_RED_DSL --> GATE_EXT_CT
    GATE_EXT_CT -- No --> CT_GREEN_STUBS
    GATE_EXT_CT -- Yes --> CT_RED_EXTERNAL_DRIVER
    CT_RED_EXTERNAL_DRIVER --> CT_GREEN_STUBS
    CT_GREEN_STUBS --> CT_END

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
    COMMIT_STRUCT[["COMMIT: <Ticket> | ${phase}"]]
    COMPILE[[Compile in-scope projects]]
    DRIFT[[Print drift warning if applicable]]
    GATE_TEST_MODE{"TEST mode? (full | compile | skip)"}
    SAMPLE[[Run sample suite]]
    STOP_STRUCT_REVIEW[STOP - HUMAN REVIEW — approve implementation]
    STOP_STRUCT_TEST[STOP - HUMAN REVIEW — review TEST results]
    STRUCT_END((End))
    STRUCT_WRITE["${phase} - WRITE"]
    TICK[[Tick checklist items]]

    STRUCT_WRITE --> STOP_STRUCT_REVIEW
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
    class ASK_COMMIT,COMMIT_STRUCT,COMPILE,DRIFT,SAMPLE,TICK serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class STRUCT_WRITE agentNode

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
    EXTAPI_CYCLE[EXTAPI_CYCLE — see § Contract Test Sub-Process]
    GATE_CHANGE_CHANNEL{System interface channel?}
    GATE_CHANGE_SCOPE{Driver Adapter scope?}
    SYSAPI_CYCLE["SYSAPI_CYCLE — see § Structural Cycle (shared)"]
    SYSUI_CYCLE["SYSUI_CYCLE — see § Structural Cycle (shared)"]

    GATE_CHANGE_SCOPE -- system --> GATE_CHANGE_CHANNEL
    GATE_CHANGE_SCOPE -- external_system --> EXTAPI_CYCLE
    GATE_CHANGE_CHANNEL -- api --> SYSAPI_CYCLE
    GATE_CHANGE_CHANNEL -- ui --> SYSUI_CYCLE
    SYSAPI_CYCLE --> DA_END
    SYSUI_CYCLE --> DA_END
    EXTAPI_CYCLE --> DA_END
```

## SUT Cycle

```mermaid
flowchart TD
    CHORE_CYCLE["CHORE_CYCLE — see § Structural Cycle (shared)"]
    SUT_END((End))

    CHORE_CYCLE --> SUT_END
```

