# ATDD Process Flow

> Generated from `internal/atdd/runtime/statemachine/process-flow.yaml` by `internal/atdd/runtime/diagram`. Do not edit by hand — edit the YAML and regenerate via `gh optivem process show > docs/process-diagram.md`.

Each section corresponds to one named process in the YAML. `call-activity` nodes appear as boxes pointing at the linked sub-process's heading.

## Legend

Node **shape** encodes the BPMN type; **fill color** encodes the executor.

- `((circle))` — start / end event
- `{diamond}` — gateway (decision)
- `[[subroutine]]` — service task — mechanical, automated step (white)
- `[rectangle]` — user task — LLM agent (dark blue) or human (yellow); `call_activity` rectangles are unfilled and link to a sub-process heading
- `[/skewed/]` — published outputs of a process (dashed border)

```mermaid
flowchart LR
    EVT((Start / End))
    GW{Gateway}
    SVC[["Service Task (Automated)"]]
    AGT["User Task (LLM Agent)"]
    HUM["User Task (Human)"]
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

## Main

```mermaid
flowchart TD
    END((End))
    IMPLEMENT_TICKET[IMPLEMENT_TICKET — see § implement-ticket]
    PICK_TOP_READY[[Pick top READY ticket]]
    START((Start))

    START -- Board --> PICK_TOP_READY
    START -- Specific Issue --> IMPLEMENT_TICKET
    PICK_TOP_READY --> IMPLEMENT_TICKET
    IMPLEMENT_TICKET --> END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class PICK_TOP_READY serviceNode
```

## Refine Ticket

```mermaid
flowchart TD
    MARK_IN_REFINEMENT[[MARK_IN_REFINEMENT]]
    MARK_READY[[MARK_READY]]
    REFINE_BACKLOG[REFINE_BACKLOG — see § refine-backlog]
    REFINE_TICKET_END((End))

    MARK_IN_REFINEMENT --> REFINE_BACKLOG
    REFINE_BACKLOG --> MARK_READY
    MARK_READY --> REFINE_TICKET_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class MARK_IN_REFINEMENT,MARK_READY serviceNode
```

## Implement Ticket

```mermaid
flowchart TD
    CALL_CHANGE_SYSTEM_BEHAVIOR[CALL_CHANGE_SYSTEM_BEHAVIOR — see § change-system-behavior]
    CALL_COVER_SYSTEM_BEHAVIOR[CALL_COVER_SYSTEM_BEHAVIOR — see § cover-system-behavior]
    CALL_ONBOARD_EXTERNAL_SYSTEM[CALL_ONBOARD_EXTERNAL_SYSTEM — see § onboard-external-system]
    CALL_REDESIGN_SYSTEM_STRUCTURE[CALL_REDESIGN_SYSTEM_STRUCTURE — see § redesign-system-structure]
    CALL_REFACTOR_SYSTEM_STRUCTURE[CALL_REFACTOR_SYSTEM_STRUCTURE — see § refactor-system-structure]
    CALL_REFACTOR_TEST_STRUCTURE[CALL_REFACTOR_TEST_STRUCTURE — see § refactor-test-structure]
    GATE_TICKET_KIND{"Ticket kind (type + optional task subtype)?"}
    IMPLEMENT_TICKET_END((End))
    MARK_IN_ACCEPTANCE[[MARK_IN_ACCEPTANCE]]
    MARK_IN_PROGRESS[[MARK_IN_PROGRESS]]
    PARSE_TICKET[[PARSE_TICKET]]

    MARK_IN_PROGRESS --> PARSE_TICKET
    PARSE_TICKET --> GATE_TICKET_KIND
    GATE_TICKET_KIND -- Story --> CALL_CHANGE_SYSTEM_BEHAVIOR
    GATE_TICKET_KIND -- Bug --> CALL_CHANGE_SYSTEM_BEHAVIOR
    GATE_TICKET_KIND -- Task / Legacy Coverage --> CALL_COVER_SYSTEM_BEHAVIOR
    GATE_TICKET_KIND -- Task / System Redesign --> CALL_REDESIGN_SYSTEM_STRUCTURE
    GATE_TICKET_KIND -- Task / System Refactor --> CALL_REFACTOR_SYSTEM_STRUCTURE
    GATE_TICKET_KIND -- Task / Test Refactor --> CALL_REFACTOR_TEST_STRUCTURE
    GATE_TICKET_KIND -- Task / External System Onboarding --> CALL_ONBOARD_EXTERNAL_SYSTEM
    CALL_CHANGE_SYSTEM_BEHAVIOR --> MARK_IN_ACCEPTANCE
    CALL_COVER_SYSTEM_BEHAVIOR --> MARK_IN_ACCEPTANCE
    CALL_REDESIGN_SYSTEM_STRUCTURE --> MARK_IN_ACCEPTANCE
    CALL_REFACTOR_SYSTEM_STRUCTURE --> MARK_IN_ACCEPTANCE
    CALL_REFACTOR_TEST_STRUCTURE --> MARK_IN_ACCEPTANCE
    CALL_ONBOARD_EXTERNAL_SYSTEM --> MARK_IN_ACCEPTANCE
    MARK_IN_ACCEPTANCE --> IMPLEMENT_TICKET_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class MARK_IN_ACCEPTANCE,MARK_IN_PROGRESS,PARSE_TICKET serviceNode
```

## Refactor

```mermaid
flowchart TD
    CALL_REDESIGN_SYSTEM_STRUCTURE[CALL_REDESIGN_SYSTEM_STRUCTURE — see § redesign-system-structure]
    CALL_REFACTOR_SYSTEM_STRUCTURE[CALL_REFACTOR_SYSTEM_STRUCTURE — see § refactor-system-structure]
    CALL_REFACTOR_TEST_STRUCTURE[CALL_REFACTOR_TEST_STRUCTURE — see § refactor-test-structure]
    GATE_REFACTOR_TYPE_CHOICE{"Choose refactor type (loopable; none = exit)"}
    REFACTOR_TOP_END((End))

    GATE_REFACTOR_TYPE_CHOICE -- Refactor System Structure --> CALL_REFACTOR_SYSTEM_STRUCTURE
    GATE_REFACTOR_TYPE_CHOICE -- Refactor Test Structure --> CALL_REFACTOR_TEST_STRUCTURE
    GATE_REFACTOR_TYPE_CHOICE -- Redesign System Structure --> CALL_REDESIGN_SYSTEM_STRUCTURE
    GATE_REFACTOR_TYPE_CHOICE -- None --> REFACTOR_TOP_END
    CALL_REFACTOR_SYSTEM_STRUCTURE --> GATE_REFACTOR_TYPE_CHOICE
    CALL_REFACTOR_TEST_STRUCTURE --> GATE_REFACTOR_TYPE_CHOICE
    CALL_REDESIGN_SYSTEM_STRUCTURE --> GATE_REFACTOR_TYPE_CHOICE
```

## Refine Backlog

```mermaid
flowchart TD
    REFINE_ACCEPTANCE_CRITERIA[REFINE_ACCEPTANCE_CRITERIA — see § refine-acceptance-criteria]
    REFINE_BACKLOG_END((End))

    REFINE_ACCEPTANCE_CRITERIA --> REFINE_BACKLOG_END
```

## Onboard External System

```mermaid
flowchart TD
    CHECK_CHECKLIST_PROGRESS[[CHECK_CHECKLIST_PROGRESS]]
    DOCUMENT_EXTERNAL_SYSTEM_CONTRACT[Document external system contract]
    GATE_CHECKLIST_PARTIALLY_DONE{Checklist already partially done?}
    IDENTIFY_EXTERNAL_SYSTEM[Identify external system]
    ONBOARD_END((End))
    SETUP_EXTERNAL_SYSTEM_ACCESS["Set up external system access (credentials, endpoints, sandbox)"]
    STOP_CHECKLIST_PARTIALLY_DONE["${checklist_progress_summary} Approve re-running this cycle?"]
    VERIFY_EXTERNAL_SYSTEM_REACHABLE[Verify external system reachable]

    CHECK_CHECKLIST_PROGRESS --> GATE_CHECKLIST_PARTIALLY_DONE
    GATE_CHECKLIST_PARTIALLY_DONE -- Yes --> STOP_CHECKLIST_PARTIALLY_DONE
    GATE_CHECKLIST_PARTIALLY_DONE -- No --> IDENTIFY_EXTERNAL_SYSTEM
    STOP_CHECKLIST_PARTIALLY_DONE --> IDENTIFY_EXTERNAL_SYSTEM
    IDENTIFY_EXTERNAL_SYSTEM --> DOCUMENT_EXTERNAL_SYSTEM_CONTRACT
    DOCUMENT_EXTERNAL_SYSTEM_CONTRACT --> SETUP_EXTERNAL_SYSTEM_ACCESS
    SETUP_EXTERNAL_SYSTEM_ACCESS --> VERIFY_EXTERNAL_SYSTEM_REACHABLE
    VERIFY_EXTERNAL_SYSTEM_REACHABLE --> ONBOARD_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class CHECK_CHECKLIST_PROGRESS serviceNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class DOCUMENT_EXTERNAL_SYSTEM_CONTRACT,IDENTIFY_EXTERNAL_SYSTEM,SETUP_EXTERNAL_SYSTEM_ACCESS,STOP_CHECKLIST_PARTIALLY_DONE,VERIFY_EXTERNAL_SYSTEM_REACHABLE humanNode
```

## Change System Behavior

```mermaid
flowchart TD
    CHANGE_SYSTEM_BEHAVIOR_END((End))
    GATE_OPPORTUNISTIC_REFACTOR{"Opportunistic refactor? (loopable; none = end cycle)"}
    IMPLEMENT_AND_VERIFY_SYSTEM[GREEN — agent-action: implement-system — see § implement-and-verify-system]
    OPP_REDESIGN_SYSTEM_STRUCTURE[OPP_REDESIGN_SYSTEM_STRUCTURE — see § redesign-system-structure]
    OPP_REFACTOR_SYSTEM_STRUCTURE[OPP_REFACTOR_SYSTEM_STRUCTURE — see § refactor-system-structure]
    OPP_REFACTOR_TEST_STRUCTURE[OPP_REFACTOR_TEST_STRUCTURE — see § refactor-test-structure]
    WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL[RED — write failing acceptance tests — see § write-and-verify-acceptance-tests-fail]

    WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL --> IMPLEMENT_AND_VERIFY_SYSTEM
    IMPLEMENT_AND_VERIFY_SYSTEM --> GATE_OPPORTUNISTIC_REFACTOR
    GATE_OPPORTUNISTIC_REFACTOR -- Refactor System Structure --> OPP_REFACTOR_SYSTEM_STRUCTURE
    GATE_OPPORTUNISTIC_REFACTOR -- Refactor Test Structure --> OPP_REFACTOR_TEST_STRUCTURE
    GATE_OPPORTUNISTIC_REFACTOR -- Redesign System Structure --> OPP_REDESIGN_SYSTEM_STRUCTURE
    GATE_OPPORTUNISTIC_REFACTOR -- None --> CHANGE_SYSTEM_BEHAVIOR_END
    OPP_REFACTOR_SYSTEM_STRUCTURE --> GATE_OPPORTUNISTIC_REFACTOR
    OPP_REFACTOR_TEST_STRUCTURE --> GATE_OPPORTUNISTIC_REFACTOR
    OPP_REDESIGN_SYSTEM_STRUCTURE --> GATE_OPPORTUNISTIC_REFACTOR
```

## Cover System Behavior

```mermaid
flowchart TD
    COVER_END((End))
    WRITE_AND_VERIFY_ACCEPTANCE_TESTS_PASS[Cover — write passing acceptance tests — see § write-and-verify-acceptance-tests-pass]

    WRITE_AND_VERIFY_ACCEPTANCE_TESTS_PASS --> COVER_END
```

## Redesign System Structure

```mermaid
flowchart TD
    CHECK_CHECKLIST_PROGRESS[[CHECK_CHECKLIST_PROGRESS]]
    GATE_CHECKLIST_PARTIALLY_DONE{Checklist already partially done?}
    IMPLEMENT_AND_VERIFY_SYSTEM[agent-action: implement-system — see § implement-and-verify-system]
    IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS[IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS — see § implement-external-system-driver-adapters]
    IMPLEMENT_SYSTEM_DRIVER_ADAPTERS[IMPLEMENT_SYSTEM_DRIVER_ADAPTERS — see § implement-system-driver-adapters]
    REDESIGN_END((End))
    STOP_CHECKLIST_PARTIALLY_DONE["${checklist_progress_summary} Approve re-running this cycle?"]

    CHECK_CHECKLIST_PROGRESS --> GATE_CHECKLIST_PARTIALLY_DONE
    GATE_CHECKLIST_PARTIALLY_DONE -- Yes --> STOP_CHECKLIST_PARTIALLY_DONE
    GATE_CHECKLIST_PARTIALLY_DONE -- No --> IMPLEMENT_SYSTEM_DRIVER_ADAPTERS
    STOP_CHECKLIST_PARTIALLY_DONE --> IMPLEMENT_SYSTEM_DRIVER_ADAPTERS
    IMPLEMENT_SYSTEM_DRIVER_ADAPTERS --> IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS
    IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS --> IMPLEMENT_AND_VERIFY_SYSTEM
    IMPLEMENT_AND_VERIFY_SYSTEM --> REDESIGN_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class CHECK_CHECKLIST_PROGRESS serviceNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_CHECKLIST_PARTIALLY_DONE humanNode
```

## Refactor System Structure

```mermaid
flowchart TD
    CHECK_CHECKLIST_PROGRESS[[CHECK_CHECKLIST_PROGRESS]]
    GATE_CHECKLIST_PARTIALLY_DONE{Checklist already partially done?}
    IMPLEMENT_AND_VERIFY_SYSTEM[agent-action: refactor-system — see § implement-and-verify-system]
    REFACTOR_SYSTEM_STRUCTURE_END((End))
    STOP_CHECKLIST_PARTIALLY_DONE["${checklist_progress_summary} Approve re-running this cycle?"]

    CHECK_CHECKLIST_PROGRESS --> GATE_CHECKLIST_PARTIALLY_DONE
    GATE_CHECKLIST_PARTIALLY_DONE -- Yes --> STOP_CHECKLIST_PARTIALLY_DONE
    GATE_CHECKLIST_PARTIALLY_DONE -- No --> IMPLEMENT_AND_VERIFY_SYSTEM
    STOP_CHECKLIST_PARTIALLY_DONE --> IMPLEMENT_AND_VERIFY_SYSTEM
    IMPLEMENT_AND_VERIFY_SYSTEM --> REFACTOR_SYSTEM_STRUCTURE_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class CHECK_CHECKLIST_PROGRESS serviceNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_CHECKLIST_PARTIALLY_DONE humanNode
```

## Refactor Test Structure

```mermaid
flowchart TD
    CHECK_CHECKLIST_PROGRESS[[CHECK_CHECKLIST_PROGRESS]]
    GATE_CHECKLIST_PARTIALLY_DONE{Checklist already partially done?}
    REFACTOR_AND_VERIFY_TESTS[REFACTOR_AND_VERIFY_TESTS — see § refactor-and-verify-tests]
    REFACTOR_TEST_STRUCTURE_END((End))
    STOP_CHECKLIST_PARTIALLY_DONE["${checklist_progress_summary} Approve re-running this cycle?"]

    CHECK_CHECKLIST_PROGRESS --> GATE_CHECKLIST_PARTIALLY_DONE
    GATE_CHECKLIST_PARTIALLY_DONE -- Yes --> STOP_CHECKLIST_PARTIALLY_DONE
    GATE_CHECKLIST_PARTIALLY_DONE -- No --> REFACTOR_AND_VERIFY_TESTS
    STOP_CHECKLIST_PARTIALLY_DONE --> REFACTOR_AND_VERIFY_TESTS
    REFACTOR_AND_VERIFY_TESTS --> REFACTOR_TEST_STRUCTURE_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class CHECK_CHECKLIST_PROGRESS serviceNode

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class STOP_CHECKLIST_PARTIALLY_DONE humanNode
```

## Write and Verify Acceptance Tests Fail

```mermaid
flowchart TD
    CALL_PARAMETERISED_CORE[CALL_PARAMETERISED_CORE — see § write-and-verify-acceptance-tests]
    WAV_AT_FAIL_END((End))

    CALL_PARAMETERISED_CORE --> WAV_AT_FAIL_END
```

## Write and Verify Acceptance Tests Pass

```mermaid
flowchart TD
    CALL_PARAMETERISED_CORE[CALL_PARAMETERISED_CORE — see § write-and-verify-acceptance-tests]
    WAV_AT_PASS_END((End))

    CALL_PARAMETERISED_CORE --> WAV_AT_PASS_END
```

## Write and Verify Acceptance Tests

```mermaid
flowchart TD
    GATE_DSL_PORT_CHANGED{DSL port changed?}
    GATE_EXTERNAL_DRIVER_PORTS_CHANGED{External system driver ports changed?}
    GATE_SYSTEM_DRIVER_PORTS_CHANGED{System driver ports changed?}
    IMPLEMENT_AND_VERIFY_DSL[IMPLEMENT_AND_VERIFY_DSL — see § implement-and-verify-dsl]
    IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS[IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS — see § implement-and-verify-external-system-driver-adapters]
    IMPLEMENT_AND_VERIFY_SYSTEM_DRIVER_ADAPTERS[IMPLEMENT_AND_VERIFY_SYSTEM_DRIVER_ADAPTERS — see § implement-and-verify-system-driver-adapters]
    WAV_AT_END((End))
    WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE[WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE — see § write-and-verify-acceptance-test-code]

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
    COMMIT_TEST_CODE[COMMIT_TEST_CODE — see § commit]
    COMPILE_TESTS[COMPILE_TESTS — see § compile-tests]
    DISABLE_ACCEPTANCE_TESTS[DISABLE_ACCEPTANCE_TESTS — see § disable-tests]
    GATE_EXPECTED_TEST_RESULT{Expected test result?}
    VERIFY_TESTS_FAIL_ACCEPTANCE[VERIFY_TESTS_FAIL_ACCEPTANCE — see § verify-tests-fail]
    VERIFY_TESTS_PASS_ACCEPTANCE[VERIFY_TESTS_PASS_ACCEPTANCE — see § verify-tests-pass]
    WAV_AT_CODE_END((End))
    WRITE_ACCEPTANCE_TESTS[WRITE_ACCEPTANCE_TESTS — see § write-acceptance-tests]

    WRITE_ACCEPTANCE_TESTS --> COMPILE_TESTS
    COMPILE_TESTS --> GATE_EXPECTED_TEST_RESULT
    GATE_EXPECTED_TEST_RESULT -- Success --> VERIFY_TESTS_PASS_ACCEPTANCE
    GATE_EXPECTED_TEST_RESULT -- Failure --> VERIFY_TESTS_FAIL_ACCEPTANCE
    VERIFY_TESTS_PASS_ACCEPTANCE --> COMMIT_TEST_CODE
    VERIFY_TESTS_FAIL_ACCEPTANCE --> DISABLE_ACCEPTANCE_TESTS
    DISABLE_ACCEPTANCE_TESTS --> COMMIT_TEST_CODE
    COMMIT_TEST_CODE --> WAV_AT_CODE_END
```

## Implement and Verify DSL

```mermaid
flowchart TD
    IMPLEMENT_TEST_LAYER[IMPLEMENT_TEST_LAYER — see § implement-test-layer]
    IMPL_DSL_END((End))

    IMPLEMENT_TEST_LAYER --> IMPL_DSL_END
```

## Implement and Verify System Driver Adapters

```mermaid
flowchart TD
    IMPLEMENT_TEST_LAYER[IMPLEMENT_TEST_LAYER — see § implement-test-layer]
    IMPL_SYS_DRIVER_END((End))

    IMPLEMENT_TEST_LAYER --> IMPL_SYS_DRIVER_END
```

## Implement and Verify External System Driver Adapters

```mermaid
flowchart TD
    IMPLEMENT_TEST_LAYER[IMPLEMENT_TEST_LAYER — see § implement-test-layer]
    IMPL_EXT_DRIVER_END((End))

    IMPLEMENT_TEST_LAYER --> IMPL_EXT_DRIVER_END
```

## Implement and Verify External System Driver Adapters Contract Tests

```mermaid
flowchart TD
    GATE_DSL_PORT_CHANGED{DSL port changed?}
    IMPLEMENT_AND_VERIFY_DSL[IMPLEMENT_AND_VERIFY_DSL — see § implement-and-verify-dsl]
    IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS[IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS — see § implement-external-system-driver-adapters]
    IMPLEMENT_EXTERNAL_SYSTEM_STUBS[IMPLEMENT_EXTERNAL_SYSTEM_STUBS — see § implement-external-system-stubs]
    IMPL_EXT_DRIVER_CT_END((End))
    VERIFY_TESTS_FAIL_CONTRACT_STUB[VERIFY_TESTS_FAIL_CONTRACT_STUB — see § verify-tests-fail]
    VERIFY_TESTS_PASS_CONTRACT_REAL[VERIFY_TESTS_PASS_CONTRACT_REAL — see § verify-tests-pass]
    VERIFY_TESTS_PASS_CONTRACT_STUB[VERIFY_TESTS_PASS_CONTRACT_STUB — see § verify-tests-pass]
    WRITE_CONTRACT_TESTS[WRITE_CONTRACT_TESTS — see § write-contract-tests]

    WRITE_CONTRACT_TESTS --> GATE_DSL_PORT_CHANGED
    GATE_DSL_PORT_CHANGED -- Yes --> IMPLEMENT_AND_VERIFY_DSL
    GATE_DSL_PORT_CHANGED -- No --> IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS
    IMPLEMENT_AND_VERIFY_DSL --> IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS
    IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS --> VERIFY_TESTS_PASS_CONTRACT_REAL
    VERIFY_TESTS_PASS_CONTRACT_REAL --> VERIFY_TESTS_FAIL_CONTRACT_STUB
    VERIFY_TESTS_FAIL_CONTRACT_STUB --> IMPLEMENT_EXTERNAL_SYSTEM_STUBS
    IMPLEMENT_EXTERNAL_SYSTEM_STUBS --> VERIFY_TESTS_PASS_CONTRACT_STUB
    VERIFY_TESTS_PASS_CONTRACT_STUB --> IMPL_EXT_DRIVER_CT_END
```

## Implement and Verify System

```mermaid
flowchart TD
    BUILD_SYSTEM[BUILD_SYSTEM — see § build-system]
    CALL_AGENT_ACTION["agent-action: ${agent-action} — see § ${agent-action}"]
    COMMIT_SYSTEM[COMMIT_SYSTEM — see § commit]
    IMPL_AND_VERIFY_SYSTEM_END((End))
    START_SYSTEM[START_SYSTEM — see § start-system]
    VERIFY_TESTS_PASS[VERIFY_TESTS_PASS — see § verify-tests-pass]

    CALL_AGENT_ACTION --> BUILD_SYSTEM
    BUILD_SYSTEM --> START_SYSTEM
    START_SYSTEM --> VERIFY_TESTS_PASS
    VERIFY_TESTS_PASS --> COMMIT_SYSTEM
    COMMIT_SYSTEM --> IMPL_AND_VERIFY_SYSTEM_END
```

## Refactor and Verify Tests

```mermaid
flowchart TD
    COMMIT_TESTS[COMMIT_TESTS — see § commit]
    COMPILE_TESTS[COMPILE_TESTS — see § compile-tests]
    REFACTOR_AND_VERIFY_TESTS_END((End))
    REFACTOR_TESTS[REFACTOR_TESTS — see § refactor-tests]
    VERIFY_TESTS_PASS[VERIFY_TESTS_PASS — see § verify-tests-pass]

    REFACTOR_TESTS --> COMPILE_TESTS
    COMPILE_TESTS --> VERIFY_TESTS_PASS
    VERIFY_TESTS_PASS --> COMMIT_TESTS
    COMMIT_TESTS --> REFACTOR_AND_VERIFY_TESTS_END
```

## Implement Test Layer

```mermaid
flowchart TD
    CALL_AGENT_ACTION["agent-action: ${agent-action} — see § ${agent-action}"]
    COMMIT_LAYER[COMMIT_LAYER — see § commit]
    COMPILE_TESTS[COMPILE_TESTS — see § compile-tests]
    DISABLE_TESTS[DISABLE_TESTS — see § disable-tests]
    ENABLE_TESTS[ENABLE_TESTS — see § enable-tests]
    GATE_EXPECTED_TEST_RESULT{Expected test result?}
    IMPLEMENT_TEST_LAYER_END((End))
    VERIFY_TESTS_FAIL_FILTERED[VERIFY_TESTS_FAIL_FILTERED — see § verify-tests-fail]
    VERIFY_TESTS_PASS_FILTERED[VERIFY_TESTS_PASS_FILTERED — see § verify-tests-pass]

    CALL_AGENT_ACTION --> ENABLE_TESTS
    ENABLE_TESTS --> COMPILE_TESTS
    COMPILE_TESTS --> GATE_EXPECTED_TEST_RESULT
    GATE_EXPECTED_TEST_RESULT -- Success --> VERIFY_TESTS_PASS_FILTERED
    GATE_EXPECTED_TEST_RESULT -- Failure --> VERIFY_TESTS_FAIL_FILTERED
    VERIFY_TESTS_PASS_FILTERED --> COMMIT_LAYER
    VERIFY_TESTS_FAIL_FILTERED --> DISABLE_TESTS
    DISABLE_TESTS --> COMMIT_LAYER
    COMMIT_LAYER --> IMPLEMENT_TEST_LAYER_END
```

## Verify Tests Pass

```mermaid
flowchart TD
    FIX_UNEXPECTED_FAILING_TESTS[FIX_UNEXPECTED_FAILING_TESTS — see § fix-unexpected-failing-tests]
    GATE_TESTS_OUTCOME{All tests passed?}
    RUN_TESTS[RUN_TESTS — see § run-tests]
    VERIFY_PASS_END((End))

    RUN_TESTS --> GATE_TESTS_OUTCOME
    GATE_TESTS_OUTCOME -- Pass --> VERIFY_PASS_END
    GATE_TESTS_OUTCOME -- Fail --> FIX_UNEXPECTED_FAILING_TESTS
    FIX_UNEXPECTED_FAILING_TESTS --> VERIFY_PASS_END
```

## Verify Tests Fail

```mermaid
flowchart TD
    FIX_UNEXPECTED_PASSING_TESTS[FIX_UNEXPECTED_PASSING_TESTS — see § fix-unexpected-passing-tests]
    GATE_TESTS_OUTCOME{Any tests passed?}
    RUN_TESTS[RUN_TESTS — see § run-tests]
    VERIFY_FAIL_END((End))

    RUN_TESTS --> GATE_TESTS_OUTCOME
    GATE_TESTS_OUTCOME -- Pass --> FIX_UNEXPECTED_PASSING_TESTS
    GATE_TESTS_OUTCOME -- Fail --> VERIFY_FAIL_END
    FIX_UNEXPECTED_PASSING_TESTS --> VERIFY_FAIL_END
```

## Write Acceptance Tests

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    WAT_END((End))

    EXECUTE_AGENT --> WAT_END
```

## Write Contract Tests

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    WCT_END((End))

    EXECUTE_AGENT --> WCT_END
```

## Implement DSL

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    IMPL_DSL_END((End))

    EXECUTE_AGENT --> IMPL_DSL_END
```

## Implement System

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    IMPL_SYS_END((End))

    EXECUTE_AGENT --> IMPL_SYS_END
```

## Implement System Driver Adapters

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    IMPL_SYS_DA_END((End))

    EXECUTE_AGENT --> IMPL_SYS_DA_END
```

## Implement External System Driver Adapters

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    IMPL_EXT_DA_END((End))

    EXECUTE_AGENT --> IMPL_EXT_DA_END
```

## Implement External System Stubs

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    IMPL_STUBS_END((End))

    EXECUTE_AGENT --> IMPL_STUBS_END
```

## Disable Tests

```mermaid
flowchart TD
    DISABLE_END((End))
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]

    EXECUTE_AGENT --> DISABLE_END
```

## Enable Tests

```mermaid
flowchart TD
    ENABLE_END((End))
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]

    EXECUTE_AGENT --> ENABLE_END
```

## Fix Unexpected Passing Tests

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    FIX_PASS_END((End))

    EXECUTE_AGENT --> FIX_PASS_END
```

## Fix Unexpected Failing Tests

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    FIX_FAIL_END((End))

    EXECUTE_AGENT --> FIX_FAIL_END
```

## Refactor Tests

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    REFACTOR_TESTS_END((End))

    EXECUTE_AGENT --> REFACTOR_TESTS_END
```

## Refactor System

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    REFACTOR_SYS_END((End))

    EXECUTE_AGENT --> REFACTOR_SYS_END
```

## Refine Acceptance Criteria

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    REFINE_AC_END((End))

    EXECUTE_AGENT --> REFINE_AC_END
```

## Compile

```mermaid
flowchart TD
    COMPILE_MID_END((End))
    EXECUTE_COMMAND[EXECUTE_COMMAND — see § execute-command]

    EXECUTE_COMMAND --> COMPILE_MID_END
```

## Compile System

```mermaid
flowchart TD
    COMPILE_SYS_END((End))
    EXECUTE_COMMAND[EXECUTE_COMMAND — see § execute-command]

    EXECUTE_COMMAND --> COMPILE_SYS_END
```

## Compile Tests

```mermaid
flowchart TD
    COMPILE_TESTS_END((End))
    EXECUTE_COMMAND[EXECUTE_COMMAND — see § execute-command]

    EXECUTE_COMMAND --> COMPILE_TESTS_END
```

## Build System

```mermaid
flowchart TD
    BUILD_SYS_END((End))
    EXECUTE_COMMAND[EXECUTE_COMMAND — see § execute-command]

    EXECUTE_COMMAND --> BUILD_SYS_END
```

## Start System

```mermaid
flowchart TD
    EXECUTE_COMMAND[EXECUTE_COMMAND — see § execute-command]
    START_SYS_END((End))

    EXECUTE_COMMAND --> START_SYS_END
```

## Commit

```mermaid
flowchart TD
    COMMIT_MID_END((End))
    EXECUTE_COMMAND[EXECUTE_COMMAND — see § execute-command]

    EXECUTE_COMMAND --> COMMIT_MID_END
```

## Run Tests

```mermaid
flowchart TD
    EXECUTE_COMMAND[EXECUTE_COMMAND — see § execute-command]
    RUN_TESTS_END((End))

    EXECUTE_COMMAND --> RUN_TESTS_END
```

## Approve

```mermaid
flowchart TD
    APPROVE_OK_END((End))
    APPROVE_REJECT_END((End))
    ASK_HUMAN["${question}"]
    GATE_APPROVED{Approved?}

    ASK_HUMAN --> GATE_APPROVED
    GATE_APPROVED -- Approved --> APPROVE_OK_END
    GATE_APPROVED -- Rejected --> APPROVE_REJECT_END

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class ASK_HUMAN humanNode
```

## Execute Agent

```mermaid
flowchart TD
    APPROVE_POST[APPROVE_POST — see § approve]
    APPROVE_PRE[APPROVE_PRE — see § approve]
    CALL_FIX[CALL_FIX — see § fix]
    EXECUTE_AGENT_END((End))
    GATE_FIX_ON_FAILURE{Fix on failure?}
    GATE_OUTPUTS_AND_SCOPES_VALID{"Outputs & scopes valid?"}
    RUN_AGENT["Run agent ${task-name}"]
    VALIDATE_OUTPUTS_AND_SCOPES[["Validate outputs (${outputs}) & scopes (${scopes})"]]

    APPROVE_PRE --> RUN_AGENT
    RUN_AGENT --> VALIDATE_OUTPUTS_AND_SCOPES
    VALIDATE_OUTPUTS_AND_SCOPES --> GATE_OUTPUTS_AND_SCOPES_VALID
    GATE_OUTPUTS_AND_SCOPES_VALID -- Yes --> APPROVE_POST
    GATE_OUTPUTS_AND_SCOPES_VALID -- No --> GATE_FIX_ON_FAILURE
    GATE_FIX_ON_FAILURE -- Yes --> CALL_FIX
    GATE_FIX_ON_FAILURE -- No --> APPROVE_POST
    CALL_FIX --> APPROVE_POST
    APPROVE_POST --> EXECUTE_AGENT_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class VALIDATE_OUTPUTS_AND_SCOPES serviceNode

    classDef agentNode fill:#004085,stroke:#002752,stroke-width:2px,color:#ffffff
    class RUN_AGENT agentNode
```

## Execute Command

```mermaid
flowchart TD
    APPROVE_PRE[APPROVE_PRE — see § approve]
    CALL_FIX[CALL_FIX — see § fix]
    EXECUTE_COMMAND_END((End))
    GATE_COMMAND_SUCCEEDED{Command succeeded?}
    RUN_COMMAND[["Run command ${command}"]]

    APPROVE_PRE --> RUN_COMMAND
    RUN_COMMAND --> GATE_COMMAND_SUCCEEDED
    GATE_COMMAND_SUCCEEDED -- Yes --> EXECUTE_COMMAND_END
    GATE_COMMAND_SUCCEEDED -- No --> CALL_FIX
    CALL_FIX --> EXECUTE_COMMAND_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class RUN_COMMAND serviceNode
```

## Fix

```mermaid
flowchart TD
    APPROVE_PRE[APPROVE_PRE — see § approve]
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    FIX_END((End))

    APPROVE_PRE --> EXECUTE_AGENT
    EXECUTE_AGENT --> FIX_END
```

