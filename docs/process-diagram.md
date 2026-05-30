# ATDD Process Flow

> Generated from `internal/atdd/runtime/statemachine/process-flow.yaml` by `internal/atdd/runtime/diagram`. Do not edit by hand — edit the YAML and regenerate via `gh optivem process show > docs/process-diagram.md`.

Each section corresponds to one named process in the YAML. `call-activity` nodes appear as boxes pointing at the linked sub-process's heading.

## Legend

Node **shape** encodes the BPMN type; **fill color** encodes the executor; **border color** (orthogonal) encodes the TDD stage where the author marked one.

- `(( ))` — start / end event (BPMN plain start or end; empty circle, descriptive name lives in the YAML). Start vs end is read from position in the flow — start has no incoming edge, end has no outgoing edge.
- `((⚡))` — error end event (BPMN exceptional exit; red border). Two flavors: **Unknown** (defensive guard — an unhandled gateway branch fired; should never happen at runtime) and **Rejected** (hard-abort — a runtime condition that intentionally halts the run, e.g. agent output rejected post-approve). The descriptive name is in the YAML source; the diagram keeps the icon small.
- `{diamond}` — gateway (decision)
- `[[subroutine]]` — service task — mechanical, automated step (white)
- `[rectangle]` — user task — LLM agent (dark blue) or human (yellow); `call_activity` rectangles are unfilled and link to a sub-process heading
- `[/skewed/]` — published outputs of a process (dashed border)
- **TDD-stage border** — red = RED (failing test), green = GREEN (test passes), blue = REFACTOR (improve without changing behaviour). Only applied where the call site explicitly plays that role.

```mermaid
flowchart LR
    EVT(( ))
    ERR((⚡))
    GW{Gateway}
    SVC[["Service Task (Automated)"]]
    AGT["User Task (LLM Agent)"]
    HUM["User Task (Human)"]
    CALL[Call activity — sub-process]
    TDDR[RED step]
    TDDG[GREEN step]
    TDDF[REFACTOR step]
    OUT[/Outputs/]

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class SVC serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class AGT agentNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class HUM humanNode

    classDef errorEndNode fill:#ffffff,stroke:#dc3545,stroke-width:2px,color:#000000
    class ERR errorEndNode

    classDef tddRedNode stroke:#dc3545,stroke-width:3px
    class TDDR tddRedNode

    classDef tddGreenNode stroke:#28a745,stroke-width:3px
    class TDDG tddGreenNode

    classDef tddRefactorNode stroke:#007bff,stroke-width:3px
    class TDDF tddRefactorNode

    classDef outputNode fill:#e7f0ff,stroke:#004085,stroke-width:1px,stroke-dasharray:4 2,color:#000000
    class OUT outputNode
```

## Main

```mermaid
flowchart TD
    END(( ))
    IMPLEMENT_TICKET[Implement Ticket]
    START(( ))

    START --> IMPLEMENT_TICKET
    IMPLEMENT_TICKET --> END
```

## Refine Ticket

```mermaid
flowchart TD
    MARK_IN_REFINEMENT[[Mark IN REFINEMENT]]
    MARK_READY[[Mark READY]]
    REFINE_BACKLOG_ITEM[Refine Backlog Item]
    REFINE_TICKET_END(( ))

    MARK_IN_REFINEMENT --> REFINE_BACKLOG_ITEM
    REFINE_BACKLOG_ITEM --> MARK_READY
    MARK_READY --> REFINE_TICKET_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class MARK_IN_REFINEMENT,MARK_READY serviceNode
```

## Implement Ticket

```mermaid
flowchart TD
    CHANGE_SYSTEM_BEHAVIOR[Change System Behavior]
    COVER_SYSTEM_BEHAVIOR[Cover System Behavior]
    GATE_TASK_SUBTYPE{Task Subtype?}
    GATE_TICKET_KIND{Ticket Kind?}
    IMPLEMENT_TICKET_END(( ))
    MARK_IN_ACCEPTANCE[[Mark IN ACCEPTANCE]]
    MARK_IN_PROGRESS[[Mark IN PROGRESS]]
    PARSE_TICKET[[Parse Ticket]]
    REDESIGN_EXTERNAL_SYSTEM_STRUCTURE[Redesign External-System Structure]
    REDESIGN_SYSTEM_STRUCTURE[Redesign System Structure]
    REFACTOR_SYSTEM_STRUCTURE[Refactor System Structure]
    REFACTOR_TEST_STRUCTURE[Refactor Test Structure]
    UNKNOWN_TASK_SUBTYPE((⚡))
    UNKNOWN_TICKET_KIND((⚡))

    MARK_IN_PROGRESS --> PARSE_TICKET
    PARSE_TICKET --> GATE_TICKET_KIND
    GATE_TICKET_KIND -- Story --> CHANGE_SYSTEM_BEHAVIOR
    GATE_TICKET_KIND -- Bug --> CHANGE_SYSTEM_BEHAVIOR
    GATE_TICKET_KIND -- Task --> GATE_TASK_SUBTYPE
    GATE_TICKET_KIND --> UNKNOWN_TICKET_KIND
    GATE_TASK_SUBTYPE -- Legacy Coverage --> COVER_SYSTEM_BEHAVIOR
    GATE_TASK_SUBTYPE -- System Redesign --> REDESIGN_SYSTEM_STRUCTURE
    GATE_TASK_SUBTYPE -- External System Redesign --> REDESIGN_EXTERNAL_SYSTEM_STRUCTURE
    GATE_TASK_SUBTYPE -- System Refactor --> REFACTOR_SYSTEM_STRUCTURE
    GATE_TASK_SUBTYPE -- Test Refactor --> REFACTOR_TEST_STRUCTURE
    GATE_TASK_SUBTYPE --> UNKNOWN_TASK_SUBTYPE
    CHANGE_SYSTEM_BEHAVIOR --> MARK_IN_ACCEPTANCE
    COVER_SYSTEM_BEHAVIOR --> MARK_IN_ACCEPTANCE
    REDESIGN_SYSTEM_STRUCTURE --> MARK_IN_ACCEPTANCE
    REDESIGN_EXTERNAL_SYSTEM_STRUCTURE --> MARK_IN_ACCEPTANCE
    REFACTOR_SYSTEM_STRUCTURE --> MARK_IN_ACCEPTANCE
    REFACTOR_TEST_STRUCTURE --> MARK_IN_ACCEPTANCE
    MARK_IN_ACCEPTANCE --> IMPLEMENT_TICKET_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class MARK_IN_ACCEPTANCE,MARK_IN_PROGRESS,PARSE_TICKET serviceNode

    classDef errorEndNode fill:#ffffff,stroke:#dc3545,stroke-width:2px,color:#000000
    class UNKNOWN_TASK_SUBTYPE,UNKNOWN_TICKET_KIND errorEndNode
```

## Refactor

```mermaid
flowchart TD
    GATE_REFACTOR_TYPE_CHOICE{Refactor Type?}
    REDESIGN_EXTERNAL_SYSTEM_STRUCTURE[Redesign External-System Structure]
    REDESIGN_SYSTEM_STRUCTURE[Redesign System Structure]
    REFACTOR_SYSTEM_STRUCTURE[Refactor System Structure]
    REFACTOR_TEST_STRUCTURE[Refactor Test Structure]
    REFACTOR_TOP_END(( ))

    GATE_REFACTOR_TYPE_CHOICE -- Refactor System Structure --> REFACTOR_SYSTEM_STRUCTURE
    GATE_REFACTOR_TYPE_CHOICE -- Refactor Test Structure --> REFACTOR_TEST_STRUCTURE
    GATE_REFACTOR_TYPE_CHOICE -- Redesign System Structure --> REDESIGN_SYSTEM_STRUCTURE
    GATE_REFACTOR_TYPE_CHOICE -- Redesign External System Structure --> REDESIGN_EXTERNAL_SYSTEM_STRUCTURE
    GATE_REFACTOR_TYPE_CHOICE -- None --> REFACTOR_TOP_END
    REFACTOR_SYSTEM_STRUCTURE --> GATE_REFACTOR_TYPE_CHOICE
    REFACTOR_TEST_STRUCTURE --> GATE_REFACTOR_TYPE_CHOICE
    REDESIGN_SYSTEM_STRUCTURE --> GATE_REFACTOR_TYPE_CHOICE
    REDESIGN_EXTERNAL_SYSTEM_STRUCTURE --> GATE_REFACTOR_TYPE_CHOICE
```

## Refine Backlog Item

```mermaid
flowchart TD
    REFINE_ACCEPTANCE_CRITERIA[Refine Acceptance Criteria]
    REFINE_BACKLOG_ITEM_END(( ))

    REFINE_ACCEPTANCE_CRITERIA --> REFINE_BACKLOG_ITEM_END
```

## Change System Behavior

```mermaid
flowchart TD
    CHANGE_SYSTEM_BEHAVIOR_END(( ))
    IMPLEMENT_AND_VERIFY_SYSTEM[Implement System — see § implement-and-verify-system]
    REFACTOR_OPPORTUNISTICALLY["Opportunistic Refactor (Loopable) — see § refactor"]
    WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL[Write Failing Acceptance Tests — see § write-and-verify-acceptance-tests-fail]

    WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL --> IMPLEMENT_AND_VERIFY_SYSTEM
    IMPLEMENT_AND_VERIFY_SYSTEM --> REFACTOR_OPPORTUNISTICALLY
    REFACTOR_OPPORTUNISTICALLY --> CHANGE_SYSTEM_BEHAVIOR_END

    classDef tddRedNode stroke:#dc3545,stroke-width:3px
    class WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL tddRedNode

    classDef tddGreenNode stroke:#28a745,stroke-width:3px
    class IMPLEMENT_AND_VERIFY_SYSTEM tddGreenNode

    classDef tddRefactorNode stroke:#007bff,stroke-width:3px
    class REFACTOR_OPPORTUNISTICALLY tddRefactorNode
```

## Cover System Behavior

```mermaid
flowchart TD
    COVER_END(( ))
    WRITE_AND_VERIFY_ACCEPTANCE_TESTS_PASS[Write Passing Acceptance Tests — see § write-and-verify-acceptance-tests-pass]

    WRITE_AND_VERIFY_ACCEPTANCE_TESTS_PASS --> COVER_END

    classDef tddGreenNode stroke:#28a745,stroke-width:3px
    class WRITE_AND_VERIFY_ACCEPTANCE_TESTS_PASS tddGreenNode
```

## Redesign System Structure

```mermaid
flowchart TD
    IMPLEMENT_AND_VERIFY_SYSTEM[Update System — see § implement-and-verify-system]
    REDESIGN_END(( ))
    UPDATE_SYSTEM_DRIVER_ADAPTERS[Update System Driver Adapters]

    UPDATE_SYSTEM_DRIVER_ADAPTERS --> IMPLEMENT_AND_VERIFY_SYSTEM
    IMPLEMENT_AND_VERIFY_SYSTEM --> REDESIGN_END

    classDef tddRedNode stroke:#dc3545,stroke-width:3px
    class UPDATE_SYSTEM_DRIVER_ADAPTERS tddRedNode

    classDef tddGreenNode stroke:#28a745,stroke-width:3px
    class IMPLEMENT_AND_VERIFY_SYSTEM tddGreenNode
```

## Refactor System Structure

```mermaid
flowchart TD
    IMPLEMENT_AND_VERIFY_SYSTEM[Refactor System — see § implement-and-verify-system]
    REFACTOR_SYSTEM_STRUCTURE_END(( ))

    IMPLEMENT_AND_VERIFY_SYSTEM --> REFACTOR_SYSTEM_STRUCTURE_END

    classDef tddRefactorNode stroke:#007bff,stroke-width:3px
    class IMPLEMENT_AND_VERIFY_SYSTEM tddRefactorNode
```

## Refactor Test Structure

```mermaid
flowchart TD
    REFACTOR_AND_VERIFY_TESTS[Refactor and Verify Tests]
    REFACTOR_TEST_STRUCTURE_END(( ))

    REFACTOR_AND_VERIFY_TESTS --> REFACTOR_TEST_STRUCTURE_END

    classDef tddRefactorNode stroke:#007bff,stroke-width:3px
    class REFACTOR_AND_VERIFY_TESTS tddRefactorNode
```

## Write and Verify Acceptance Tests Fail

```mermaid
flowchart TD
    WAV_AT_FAIL_END(( ))
    WRITE_AND_VERIFY_ACCEPTANCE_TESTS[Write and Verify Acceptance Tests]

    WRITE_AND_VERIFY_ACCEPTANCE_TESTS --> WAV_AT_FAIL_END
```

## Write and Verify Acceptance Tests Pass

```mermaid
flowchart TD
    WAV_AT_PASS_END(( ))
    WRITE_AND_VERIFY_ACCEPTANCE_TESTS[Write and Verify Acceptance Tests]

    WRITE_AND_VERIFY_ACCEPTANCE_TESTS --> WAV_AT_PASS_END
```

## Write and Verify Acceptance Tests

```mermaid
flowchart TD
    GATE_DSL_PORT_CHANGED{DSL Port Changed?}
    GATE_EXTERNAL_DRIVER_PORTS_CHANGED{External Driver Ports Changed?}
    GATE_SYSTEM_DRIVER_PORTS_CHANGED{System Driver Ports Changed?}
    IMPLEMENT_AND_VERIFY_DSL[Implement and Verify DSL]
    IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS[Implement and Verify External-System Driver Adapters]
    IMPLEMENT_AND_VERIFY_SYSTEM_DRIVER_ADAPTERS[Implement and Verify System Driver Adapters]
    WAV_AT_END(( ))
    WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE[Write and Verify Acceptance Test Code]

    WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE --> GATE_DSL_PORT_CHANGED
    GATE_DSL_PORT_CHANGED -- Yes --> IMPLEMENT_AND_VERIFY_DSL
    GATE_DSL_PORT_CHANGED -- No --> WAV_AT_END
    IMPLEMENT_AND_VERIFY_DSL --> GATE_EXTERNAL_DRIVER_PORTS_CHANGED
    GATE_EXTERNAL_DRIVER_PORTS_CHANGED -- Yes --> IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS
    GATE_EXTERNAL_DRIVER_PORTS_CHANGED -- No --> GATE_SYSTEM_DRIVER_PORTS_CHANGED
    IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS --> GATE_SYSTEM_DRIVER_PORTS_CHANGED
    GATE_SYSTEM_DRIVER_PORTS_CHANGED -- Yes --> IMPLEMENT_AND_VERIFY_SYSTEM_DRIVER_ADAPTERS
    GATE_SYSTEM_DRIVER_PORTS_CHANGED -- No --> WAV_AT_END
    IMPLEMENT_AND_VERIFY_SYSTEM_DRIVER_ADAPTERS --> WAV_AT_END
```

## Write and Verify Acceptance Test Code

```mermaid
flowchart TD
    COMMIT_TEST_CODE[Commit Test Code — see § commit]
    COMPILE_TESTS[Compile Tests]
    GATE_EXPECTED_TEST_RESULT{Expected Test Result?}
    START_SYSTEM[Start System]
    UNKNOWN_EXPECTED_TEST_RESULT((⚡))
    VERIFY_TESTS_FAIL_ACCEPTANCE[Verify Acceptance Tests Fail — see § verify-tests-fail]
    VERIFY_TESTS_PASS_ACCEPTANCE[Verify Acceptance Tests Pass — see § verify-tests-pass]
    WAV_AT_CODE_END(( ))
    WRITE_ACCEPTANCE_TESTS[Write Acceptance Tests]

    WRITE_ACCEPTANCE_TESTS --> COMPILE_TESTS
    COMPILE_TESTS --> START_SYSTEM
    START_SYSTEM --> GATE_EXPECTED_TEST_RESULT
    GATE_EXPECTED_TEST_RESULT -- Success --> VERIFY_TESTS_PASS_ACCEPTANCE
    GATE_EXPECTED_TEST_RESULT -- Failure --> VERIFY_TESTS_FAIL_ACCEPTANCE
    GATE_EXPECTED_TEST_RESULT --> UNKNOWN_EXPECTED_TEST_RESULT
    VERIFY_TESTS_PASS_ACCEPTANCE --> COMMIT_TEST_CODE
    VERIFY_TESTS_FAIL_ACCEPTANCE --> COMMIT_TEST_CODE
    COMMIT_TEST_CODE --> WAV_AT_CODE_END

    classDef errorEndNode fill:#ffffff,stroke:#dc3545,stroke-width:2px,color:#000000
    class UNKNOWN_EXPECTED_TEST_RESULT errorEndNode
```

## Implement and Verify DSL

```mermaid
flowchart TD
    IMPLEMENT_TEST_LAYER[Implement DSL Layer — see § implement-test-layer]
    IMPL_DSL_END(( ))

    IMPLEMENT_TEST_LAYER --> IMPL_DSL_END
```

## Implement and Verify System Driver Adapters

```mermaid
flowchart TD
    IMPLEMENT_TEST_LAYER[Implement System Driver Adapter Layer — see § implement-test-layer]
    IMPL_SYS_DRIVER_END(( ))

    IMPLEMENT_TEST_LAYER --> IMPL_SYS_DRIVER_END
```

## Implement and Verify External-System Driver Adapters

```mermaid
flowchart TD
    IMPLEMENT_TEST_LAYER[Implement External-System Driver Adapter Layer — see § implement-test-layer]
    IMPL_EXT_DRIVER_END(( ))

    IMPLEMENT_TEST_LAYER --> IMPL_EXT_DRIVER_END
```

## Implement and Verify External-System Driver Adapters Contract Tests

```mermaid
flowchart TD
    BUILD_SYSTEM_AFTER_DRIVER[Build System]
    BUILD_SYSTEM_AFTER_STUBS[Build System]
    GATE_DSL_PORT_CHANGED{DSL Port Changed?}
    IMPLEMENT_AND_VERIFY_DSL[Implement and Verify DSL]
    IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS[Implement External-System Driver Adapters]
    IMPLEMENT_EXTERNAL_SYSTEM_STUBS[Implement External-System Stubs]
    IMPL_EXT_DRIVER_CT_END(( ))
    START_SYSTEM_AFTER_DRIVER[Start System — see § start-system-restart]
    START_SYSTEM_AFTER_STUBS[Start System — see § start-system-restart]
    START_SYSTEM_BEFORE_STUB_FAIL[Start System]
    VERIFY_TESTS_FAIL_CONTRACT_STUB[Verify Contract Tests Fail Against the Stub — see § verify-tests-fail]
    VERIFY_TESTS_PASS_CONTRACT_REAL[Verify Contract Tests Pass Against the Real System — see § verify-tests-pass]
    VERIFY_TESTS_PASS_CONTRACT_STUB[Verify Contract Tests Pass Against the Stub — see § verify-tests-pass]
    WRITE_CONTRACT_TESTS[Write Contract Tests]

    WRITE_CONTRACT_TESTS --> GATE_DSL_PORT_CHANGED
    GATE_DSL_PORT_CHANGED -- Yes --> IMPLEMENT_AND_VERIFY_DSL
    GATE_DSL_PORT_CHANGED -- No --> IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS
    IMPLEMENT_AND_VERIFY_DSL --> IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS
    IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS --> BUILD_SYSTEM_AFTER_DRIVER
    BUILD_SYSTEM_AFTER_DRIVER --> START_SYSTEM_AFTER_DRIVER
    START_SYSTEM_AFTER_DRIVER --> VERIFY_TESTS_PASS_CONTRACT_REAL
    VERIFY_TESTS_PASS_CONTRACT_REAL --> START_SYSTEM_BEFORE_STUB_FAIL
    START_SYSTEM_BEFORE_STUB_FAIL --> VERIFY_TESTS_FAIL_CONTRACT_STUB
    VERIFY_TESTS_FAIL_CONTRACT_STUB --> IMPLEMENT_EXTERNAL_SYSTEM_STUBS
    IMPLEMENT_EXTERNAL_SYSTEM_STUBS --> BUILD_SYSTEM_AFTER_STUBS
    BUILD_SYSTEM_AFTER_STUBS --> START_SYSTEM_AFTER_STUBS
    START_SYSTEM_AFTER_STUBS --> VERIFY_TESTS_PASS_CONTRACT_STUB
    VERIFY_TESTS_PASS_CONTRACT_STUB --> IMPL_EXT_DRIVER_CT_END
```

## Implement and Verify System

```mermaid
flowchart TD
    BUILD_SYSTEM[Build the System — see § build-system]
    COMMIT_SYSTEM[Commit System Changes — see § commit]
    IMPL_AND_VERIFY_SYSTEM_END(( ))
    RUN_ACTION["Run the Configured Agent — see § ${action}"]
    START_SYSTEM[Start the System — see § start-system-restart]
    VERIFY_TESTS_PASS[Verify Tests Pass]

    RUN_ACTION --> BUILD_SYSTEM
    BUILD_SYSTEM --> START_SYSTEM
    START_SYSTEM --> VERIFY_TESTS_PASS
    VERIFY_TESTS_PASS --> COMMIT_SYSTEM
    COMMIT_SYSTEM --> IMPL_AND_VERIFY_SYSTEM_END
```

## Refactor and Verify Tests

```mermaid
flowchart TD
    COMMIT_TESTS[Commit Test Changes — see § commit]
    COMPILE_TESTS[Compile Tests]
    REFACTOR_AND_VERIFY_TESTS_END(( ))
    REFACTOR_TESTS[Refactor Tests]
    START_SYSTEM[Start System]
    VERIFY_TESTS_PASS[Verify Tests Pass]

    REFACTOR_TESTS --> COMPILE_TESTS
    COMPILE_TESTS --> START_SYSTEM
    START_SYSTEM --> VERIFY_TESTS_PASS
    VERIFY_TESTS_PASS --> COMMIT_TESTS
    COMMIT_TESTS --> REFACTOR_AND_VERIFY_TESTS_END
```

## Implement Test Layer

```mermaid
flowchart TD
    COMMIT_LAYER[Commit Layer Changes — see § commit]
    COMPILE_TESTS[Compile Tests]
    GATE_EXPECTED_TEST_RESULT{Expected Test Result?}
    IMPLEMENT_TEST_LAYER_END(( ))
    RUN_ACTION["Run the Configured Agent — see § ${action}"]
    START_SYSTEM[Start System]
    UNKNOWN_EXPECTED_TEST_RESULT((⚡))
    VERIFY_TESTS_FAIL_FILTERED[Verify Tests Fail]
    VERIFY_TESTS_PASS_FILTERED[Verify Tests Pass]

    RUN_ACTION --> COMPILE_TESTS
    COMPILE_TESTS --> START_SYSTEM
    START_SYSTEM --> GATE_EXPECTED_TEST_RESULT
    GATE_EXPECTED_TEST_RESULT -- Success --> VERIFY_TESTS_PASS_FILTERED
    GATE_EXPECTED_TEST_RESULT -- Failure --> VERIFY_TESTS_FAIL_FILTERED
    GATE_EXPECTED_TEST_RESULT --> UNKNOWN_EXPECTED_TEST_RESULT
    VERIFY_TESTS_PASS_FILTERED --> COMMIT_LAYER
    VERIFY_TESTS_FAIL_FILTERED --> COMMIT_LAYER
    COMMIT_LAYER --> IMPLEMENT_TEST_LAYER_END

    classDef errorEndNode fill:#ffffff,stroke:#dc3545,stroke-width:2px,color:#000000
    class UNKNOWN_EXPECTED_TEST_RESULT errorEndNode
```

## Verify Tests Pass

```mermaid
flowchart TD
    FIX_LOOP_EXHAUSTED((⚡))
    FIX_UNEXPECTED_FAILING_TESTS[Fix Unexpected Test Failures — see § fix-unexpected-failing-tests]
    GATE_TESTS_OUTCOME{Test Outcome?}
    RUN_TESTS[Run Tests]
    TESTS_INFRA_HALT((⚡))
    UNKNOWN_TESTS_OUTCOME((⚡))
    VERIFY_PASS_END(( ))

    RUN_TESTS --> GATE_TESTS_OUTCOME
    GATE_TESTS_OUTCOME -- Pass --> VERIFY_PASS_END
    GATE_TESTS_OUTCOME -- Fail --> FIX_UNEXPECTED_FAILING_TESTS
    GATE_TESTS_OUTCOME -- Infra --> TESTS_INFRA_HALT
    GATE_TESTS_OUTCOME --> UNKNOWN_TESTS_OUTCOME
    FIX_UNEXPECTED_FAILING_TESTS --> RUN_TESTS

    classDef errorEndNode fill:#ffffff,stroke:#dc3545,stroke-width:2px,color:#000000
    class FIX_LOOP_EXHAUSTED,TESTS_INFRA_HALT,UNKNOWN_TESTS_OUTCOME errorEndNode
```

## Verify Tests Fail

```mermaid
flowchart TD
    FIX_LOOP_EXHAUSTED((⚡))
    FIX_UNEXPECTED_PASSING_TESTS[Fix Unexpectedly Passing Tests — see § fix-unexpected-passing-tests]
    GATE_TESTS_OUTCOME{Test Outcome?}
    RUN_TESTS[Run Tests]
    TESTS_INFRA_HALT((⚡))
    UNKNOWN_TESTS_OUTCOME((⚡))
    VERIFY_FAIL_END(( ))

    RUN_TESTS --> GATE_TESTS_OUTCOME
    GATE_TESTS_OUTCOME -- Pass --> FIX_UNEXPECTED_PASSING_TESTS
    GATE_TESTS_OUTCOME -- Fail --> VERIFY_FAIL_END
    GATE_TESTS_OUTCOME -- Infra --> TESTS_INFRA_HALT
    GATE_TESTS_OUTCOME --> UNKNOWN_TESTS_OUTCOME
    FIX_UNEXPECTED_PASSING_TESTS --> RUN_TESTS

    classDef errorEndNode fill:#ffffff,stroke:#dc3545,stroke-width:2px,color:#000000
    class FIX_LOOP_EXHAUSTED,TESTS_INFRA_HALT,UNKNOWN_TESTS_OUTCOME errorEndNode
```

## Write Acceptance Tests

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    WAT_END(( ))

    EXECUTE_AGENT --> WAT_END
    WRITE-ACCEPTANCE-TESTS_OUTPUTS[/dsl-port-changed: bool, test-names?: string-list, scope-exception-files?: string-list, scope-exception-reason?: string/]
    WAT_END -. produces .-> WRITE-ACCEPTANCE-TESTS_OUTPUTS

    classDef outputNode fill:#e7f0ff,stroke:#004085,stroke-width:1px,stroke-dasharray:4 2,color:#000000
    class WRITE-ACCEPTANCE-TESTS_OUTPUTS outputNode
```

## Write Contract Tests

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    WCT_END(( ))

    EXECUTE_AGENT --> WCT_END
    WRITE-CONTRACT-TESTS_OUTPUTS[/dsl-port-changed: bool, test-names?: string-list, scope-exception-files?: string-list, scope-exception-reason?: string/]
    WCT_END -. produces .-> WRITE-CONTRACT-TESTS_OUTPUTS

    classDef outputNode fill:#e7f0ff,stroke:#004085,stroke-width:1px,stroke-dasharray:4 2,color:#000000
    class WRITE-CONTRACT-TESTS_OUTPUTS outputNode
```

## Implement DSL

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    IMPL_DSL_END(( ))

    EXECUTE_AGENT --> IMPL_DSL_END
    IMPLEMENT-DSL_OUTPUTS[/system-driver-port-changed: bool, external-driver-port-changed: bool, scope-exception-files?: string-list, scope-exception-reason?: string/]
    IMPL_DSL_END -. produces .-> IMPLEMENT-DSL_OUTPUTS

    classDef outputNode fill:#e7f0ff,stroke:#004085,stroke-width:1px,stroke-dasharray:4 2,color:#000000
    class IMPLEMENT-DSL_OUTPUTS outputNode
```

## Implement System

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    IMPL_SYS_END(( ))

    EXECUTE_AGENT --> IMPL_SYS_END
```

## Implement System Driver Adapters

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    IMPL_SYS_DA_END(( ))

    EXECUTE_AGENT --> IMPL_SYS_DA_END
```

## Implement External-System Driver Adapters

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    IMPL_EXT_DA_END(( ))

    EXECUTE_AGENT --> IMPL_EXT_DA_END
```

## Implement External-System Stubs

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    IMPL_STUBS_END(( ))

    EXECUTE_AGENT --> IMPL_STUBS_END
```

## Fix Unexpected Passing Tests

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    FIX_PASS_END(( ))

    EXECUTE_AGENT --> FIX_PASS_END
```

## Fix Unexpected Failing Tests

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    FIX_FAIL_END(( ))

    EXECUTE_AGENT --> FIX_FAIL_END
```

## Refactor Tests

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    REFACTOR_TESTS_END(( ))

    EXECUTE_AGENT --> REFACTOR_TESTS_END
```

## Refactor System

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    REFACTOR_SYS_END(( ))

    EXECUTE_AGENT --> REFACTOR_SYS_END
```

## Refine Acceptance Criteria

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    REFINE_AC_END(( ))

    EXECUTE_AGENT --> REFINE_AC_END
```

## Compile

```mermaid
flowchart TD
    COMPILE_MID_END(( ))
    EXECUTE_COMMAND[Dispatch the Command — see § execute-command]

    EXECUTE_COMMAND --> COMPILE_MID_END
```

## Compile System

```mermaid
flowchart TD
    COMPILE_SYS_END(( ))
    EXECUTE_COMMAND[Dispatch the Command — see § execute-command]

    EXECUTE_COMMAND --> COMPILE_SYS_END
```

## Compile Tests

```mermaid
flowchart TD
    COMPILE_TESTS_END(( ))
    EXECUTE_COMMAND[Dispatch the Command — see § execute-command]

    EXECUTE_COMMAND --> COMPILE_TESTS_END
```

## Build System

```mermaid
flowchart TD
    BUILD_SYS_END(( ))
    EXECUTE_COMMAND[Dispatch the Command — see § execute-command]

    EXECUTE_COMMAND --> BUILD_SYS_END
```

## Start System

```mermaid
flowchart TD
    EXECUTE_COMMAND[Dispatch the Command — see § execute-command]
    START_SYS_END(( ))

    EXECUTE_COMMAND --> START_SYS_END
```

## Commit

```mermaid
flowchart TD
    COMMIT_MID_END(( ))
    EXECUTE_COMMAND[Dispatch the Command — see § execute-command]

    EXECUTE_COMMAND --> COMMIT_MID_END
```

## Run Tests

```mermaid
flowchart TD
    EXECUTE_COMMAND[Dispatch the Command — see § execute-command]
    RUN_TESTS_END(( ))

    EXECUTE_COMMAND --> RUN_TESTS_END
```

## Approve

```mermaid
flowchart TD
    APPROVE_OK_END(( ))
    APPROVE_REJECT_END(( ))
    ASK_HUMAN["${question}"]
    GATE_APPROVED{Approval Outcome?}

    ASK_HUMAN --> GATE_APPROVED
    GATE_APPROVED -- Approved --> APPROVE_OK_END
    GATE_APPROVED -- Rejected --> APPROVE_REJECT_END

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class ASK_HUMAN humanNode
```

## Execute Agent

```mermaid
flowchart TD
    APPROVE_POST[Confirm Approval — see § approve]
    APPROVE_PRE[Request Approval — see § approve]
    EXECUTE_AGENT_END(( ))
    EXECUTE_AGENT_OUTPUT_REJECTED_END((⚡))
    EXECUTE_AGENT_REJECTED_END(( ))
    FIX[Fix the Failure — see § fix]
    GATE_APPROVED_POST{Approval Outcome?}
    GATE_APPROVED_PRE{Approval Outcome?}
    GATE_FIX_ON_FAILURE{Fix on Failure Enabled?}
    GATE_OUTPUTS_AND_SCOPES_VALID{Outputs and Scopes Valid?}
    RUN_AGENT["Run agent ${agent} (task: ${task-name})"]
    SNAPSHOT_WORKING_TREE[["Snapshot working tree (per-phase baseline)"]]
    VALIDATE_OUTPUTS_AND_SCOPES[["Validate outputs & scopes"]]

    APPROVE_PRE --> GATE_APPROVED_PRE
    GATE_APPROVED_PRE -- Approved --> SNAPSHOT_WORKING_TREE
    GATE_APPROVED_PRE -- Rejected --> EXECUTE_AGENT_REJECTED_END
    SNAPSHOT_WORKING_TREE --> RUN_AGENT
    RUN_AGENT --> VALIDATE_OUTPUTS_AND_SCOPES
    VALIDATE_OUTPUTS_AND_SCOPES --> GATE_OUTPUTS_AND_SCOPES_VALID
    GATE_OUTPUTS_AND_SCOPES_VALID -- Yes --> APPROVE_POST
    GATE_OUTPUTS_AND_SCOPES_VALID -- No --> GATE_FIX_ON_FAILURE
    GATE_FIX_ON_FAILURE -- Yes --> FIX
    GATE_FIX_ON_FAILURE -- No --> APPROVE_POST
    FIX --> RUN_AGENT
    APPROVE_POST --> GATE_APPROVED_POST
    GATE_APPROVED_POST -- Approved --> EXECUTE_AGENT_END
    GATE_APPROVED_POST -- Rejected --> EXECUTE_AGENT_OUTPUT_REJECTED_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class SNAPSHOT_WORKING_TREE,VALIDATE_OUTPUTS_AND_SCOPES serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class RUN_AGENT agentNode

    classDef errorEndNode fill:#ffffff,stroke:#dc3545,stroke-width:2px,color:#000000
    class EXECUTE_AGENT_OUTPUT_REJECTED_END errorEndNode
```

## Execute Command

```mermaid
flowchart TD
    APPROVE_PRE[Request Approval — see § approve]
    EXECUTE_COMMAND_END(( ))
    EXECUTE_COMMAND_REJECTED_END(( ))
    FIX[Fix the Failure — see § fix]
    GATE_APPROVED_PRE{Approval Outcome?}
    GATE_COMMAND_SUCCEEDED{Command Succeeded?}
    GATE_FIX_ON_FAILURE{Fix on Failure Enabled?}
    RUN_COMMAND[["Run command ${command}"]]

    APPROVE_PRE --> GATE_APPROVED_PRE
    GATE_APPROVED_PRE -- Approved --> RUN_COMMAND
    GATE_APPROVED_PRE -- Rejected --> EXECUTE_COMMAND_REJECTED_END
    RUN_COMMAND --> GATE_COMMAND_SUCCEEDED
    GATE_COMMAND_SUCCEEDED -- Yes --> EXECUTE_COMMAND_END
    GATE_COMMAND_SUCCEEDED -- No --> GATE_FIX_ON_FAILURE
    GATE_FIX_ON_FAILURE -- Yes --> FIX
    GATE_FIX_ON_FAILURE -- No --> EXECUTE_COMMAND_END
    FIX --> RUN_COMMAND

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class RUN_COMMAND serviceNode
```

## Fix

```mermaid
flowchart TD
    APPROVE_PRE[Request Approval — see § approve]
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    FIX_END(( ))
    FIX_REJECTED_END(( ))
    GATE_APPROVED_PRE{Approval Outcome?}

    APPROVE_PRE --> GATE_APPROVED_PRE
    GATE_APPROVED_PRE -- Approved --> EXECUTE_AGENT
    GATE_APPROVED_PRE -- Rejected --> FIX_REJECTED_END
    EXECUTE_AGENT --> FIX_END
```

## Redesign External-System Structure

```mermaid
flowchart TD
    IMPLEMENT_AND_VERIFY_SYSTEM[Update System — see § implement-and-verify-system]
    REDESIGN_EXTERNAL_END(( ))
    UPDATE_EXTERNAL_SYSTEM_DRIVER_ADAPTERS[Update External-System Driver Adapters]

    UPDATE_EXTERNAL_SYSTEM_DRIVER_ADAPTERS --> IMPLEMENT_AND_VERIFY_SYSTEM
    IMPLEMENT_AND_VERIFY_SYSTEM --> REDESIGN_EXTERNAL_END

    classDef tddRedNode stroke:#dc3545,stroke-width:3px
    class UPDATE_EXTERNAL_SYSTEM_DRIVER_ADAPTERS tddRedNode

    classDef tddGreenNode stroke:#28a745,stroke-width:3px
    class IMPLEMENT_AND_VERIFY_SYSTEM tddGreenNode
```

## Start System (Restart)

```mermaid
flowchart TD
    EXECUTE_COMMAND[Dispatch the Command — see § execute-command]
    START_SYS_RESTART_END(( ))

    EXECUTE_COMMAND --> START_SYS_RESTART_END
```

## Update External-System Driver Adapters

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    UPDATE_EXT_DA_END(( ))

    EXECUTE_AGENT --> UPDATE_EXT_DA_END
```

## Update System

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    UPDATE_SYS_END(( ))

    EXECUTE_AGENT --> UPDATE_SYS_END
```

## Update System Driver Adapters

```mermaid
flowchart TD
    EXECUTE_AGENT[Dispatch the Agent — see § execute-agent]
    UPDATE_SYS_DA_END(( ))

    EXECUTE_AGENT --> UPDATE_SYS_DA_END
```

