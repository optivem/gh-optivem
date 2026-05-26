# ATDD Process Flow

> Generated from `internal/atdd/runtime/statemachine/process-flow.yaml` by `internal/atdd/runtime/diagram`. Do not edit by hand — edit the YAML and regenerate via `gh optivem process show > docs/process-diagram.md`.

Each section corresponds to one named process in the YAML. `call-activity` nodes appear as boxes pointing at the linked sub-process's heading.

## Legend

Node **shape** encodes the BPMN type; **fill color** encodes the executor.

- `((circle))` — start / end event
- `{diamond}` — gateway (decision)
- `[[subroutine]]` — service task — mechanical step run by the Go runtime (white)
- `[rectangle]` — user task — LLM agent (dark blue) or human STOP (yellow); `call-activity` rectangles are unfilled and link to a sub-process heading
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

## Runtime Bootstrap (legacy entry — collapses in Phase D)

```mermaid
flowchart TD
    END((End))
    IMPLEMENT_TICKET[IMPLEMENT_TICKET — see § implement-ticket]
    PICK_TOP_READY[[Pick top READY ticket]]
    START((Start))

    START -- board --> PICK_TOP_READY
    START -- specific-issue --> IMPLEMENT_TICKET
    PICK_TOP_READY --> IMPLEMENT_TICKET
    IMPLEMENT_TICKET --> END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class PICK_TOP_READY serviceNode
```

## refine-ticket

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

## implement-ticket

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

    MARK_IN_PROGRESS --> GATE_TICKET_KIND
    GATE_TICKET_KIND -- story --> CALL_CHANGE_SYSTEM_BEHAVIOR
    GATE_TICKET_KIND -- bug --> CALL_CHANGE_SYSTEM_BEHAVIOR
    GATE_TICKET_KIND -- task/legacy-coverage --> CALL_COVER_SYSTEM_BEHAVIOR
    GATE_TICKET_KIND -- task/system-redesign --> CALL_REDESIGN_SYSTEM_STRUCTURE
    GATE_TICKET_KIND -- task/system-refactor --> CALL_REFACTOR_SYSTEM_STRUCTURE
    GATE_TICKET_KIND -- task/test-refactor --> CALL_REFACTOR_TEST_STRUCTURE
    GATE_TICKET_KIND -- task/external-system-onboarding --> CALL_ONBOARD_EXTERNAL_SYSTEM
    CALL_CHANGE_SYSTEM_BEHAVIOR --> MARK_IN_ACCEPTANCE
    CALL_COVER_SYSTEM_BEHAVIOR --> MARK_IN_ACCEPTANCE
    CALL_REDESIGN_SYSTEM_STRUCTURE --> MARK_IN_ACCEPTANCE
    CALL_REFACTOR_SYSTEM_STRUCTURE --> MARK_IN_ACCEPTANCE
    CALL_REFACTOR_TEST_STRUCTURE --> MARK_IN_ACCEPTANCE
    CALL_ONBOARD_EXTERNAL_SYSTEM --> MARK_IN_ACCEPTANCE
    MARK_IN_ACCEPTANCE --> IMPLEMENT_TICKET_END

    classDef serviceNode fill:#ffffff,stroke:#000000,stroke-width:1px,color:#000000
    class MARK_IN_ACCEPTANCE,MARK_IN_PROGRESS serviceNode
```

## refactor

```mermaid
flowchart TD
    CALL_REDESIGN_SYSTEM_STRUCTURE[CALL_REDESIGN_SYSTEM_STRUCTURE — see § redesign-system-structure]
    CALL_REFACTOR_SYSTEM_STRUCTURE[CALL_REFACTOR_SYSTEM_STRUCTURE — see § refactor-system-structure]
    CALL_REFACTOR_TEST_STRUCTURE[CALL_REFACTOR_TEST_STRUCTURE — see § refactor-test-structure]
    GATE_REFACTOR_TYPE_CHOICE{"Choose refactor type (loopable; none = exit)"}
    REFACTOR_TOP_END((End))

    GATE_REFACTOR_TYPE_CHOICE -- refactor-system-structure --> CALL_REFACTOR_SYSTEM_STRUCTURE
    GATE_REFACTOR_TYPE_CHOICE -- refactor-test-structure --> CALL_REFACTOR_TEST_STRUCTURE
    GATE_REFACTOR_TYPE_CHOICE -- redesign-system-structure --> CALL_REDESIGN_SYSTEM_STRUCTURE
    GATE_REFACTOR_TYPE_CHOICE -- none --> REFACTOR_TOP_END
    CALL_REFACTOR_SYSTEM_STRUCTURE --> GATE_REFACTOR_TYPE_CHOICE
    CALL_REFACTOR_TEST_STRUCTURE --> GATE_REFACTOR_TYPE_CHOICE
    CALL_REDESIGN_SYSTEM_STRUCTURE --> GATE_REFACTOR_TYPE_CHOICE
```

## refine-backlog

```mermaid
flowchart TD
    REFINE_ACCEPTANCE_CRITERIA[REFINE_ACCEPTANCE_CRITERIA — see § refine-acceptance-criteria]
    REFINE_BACKLOG_END((End))

    REFINE_ACCEPTANCE_CRITERIA --> REFINE_BACKLOG_END
```

## onboard-external-system

```mermaid
flowchart TD
    DOCUMENT_EXTERNAL_SYSTEM_CONTRACT[Document external system contract]
    IDENTIFY_EXTERNAL_SYSTEM[Identify external system]
    ONBOARD_END((End))
    SETUP_EXTERNAL_SYSTEM_ACCESS["Set up external system access (credentials, endpoints, sandbox)"]
    VERIFY_EXTERNAL_SYSTEM_REACHABLE[Verify external system reachable]

    IDENTIFY_EXTERNAL_SYSTEM --> DOCUMENT_EXTERNAL_SYSTEM_CONTRACT
    DOCUMENT_EXTERNAL_SYSTEM_CONTRACT --> SETUP_EXTERNAL_SYSTEM_ACCESS
    SETUP_EXTERNAL_SYSTEM_ACCESS --> VERIFY_EXTERNAL_SYSTEM_REACHABLE
    VERIFY_EXTERNAL_SYSTEM_REACHABLE --> ONBOARD_END

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class DOCUMENT_EXTERNAL_SYSTEM_CONTRACT,IDENTIFY_EXTERNAL_SYSTEM,SETUP_EXTERNAL_SYSTEM_ACCESS,VERIFY_EXTERNAL_SYSTEM_REACHABLE humanNode
```

## change-system-behavior

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
    GATE_OPPORTUNISTIC_REFACTOR -- refactor-system-structure --> OPP_REFACTOR_SYSTEM_STRUCTURE
    GATE_OPPORTUNISTIC_REFACTOR -- refactor-test-structure --> OPP_REFACTOR_TEST_STRUCTURE
    GATE_OPPORTUNISTIC_REFACTOR -- redesign-system-structure --> OPP_REDESIGN_SYSTEM_STRUCTURE
    GATE_OPPORTUNISTIC_REFACTOR -- none --> CHANGE_SYSTEM_BEHAVIOR_END
    OPP_REFACTOR_SYSTEM_STRUCTURE --> GATE_OPPORTUNISTIC_REFACTOR
    OPP_REFACTOR_TEST_STRUCTURE --> GATE_OPPORTUNISTIC_REFACTOR
    OPP_REDESIGN_SYSTEM_STRUCTURE --> GATE_OPPORTUNISTIC_REFACTOR
```

## cover-system-behavior

```mermaid
flowchart TD
    COVER_END((End))
    WRITE_AND_VERIFY_ACCEPTANCE_TESTS_PASS[Cover — write passing acceptance tests — see § write-and-verify-acceptance-tests-pass]

    WRITE_AND_VERIFY_ACCEPTANCE_TESTS_PASS --> COVER_END
```

## redesign-system-structure

```mermaid
flowchart TD
    IMPLEMENT_AND_VERIFY_SYSTEM[agent-action: implement-system — see § implement-and-verify-system]
    IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS[IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS — see § implement-external-system-driver-adapters]
    IMPLEMENT_SYSTEM_DRIVER_ADAPTERS[IMPLEMENT_SYSTEM_DRIVER_ADAPTERS — see § implement-system-driver-adapters]
    REDESIGN_END((End))

    IMPLEMENT_SYSTEM_DRIVER_ADAPTERS --> IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS
    IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS --> IMPLEMENT_AND_VERIFY_SYSTEM
    IMPLEMENT_AND_VERIFY_SYSTEM --> REDESIGN_END
```

## refactor-system-structure

```mermaid
flowchart TD
    IMPLEMENT_AND_VERIFY_SYSTEM[agent-action: refactor-system — see § implement-and-verify-system]
    REFACTOR_SYSTEM_STRUCTURE_END((End))

    IMPLEMENT_AND_VERIFY_SYSTEM --> REFACTOR_SYSTEM_STRUCTURE_END
```

## refactor-test-structure

```mermaid
flowchart TD
    REFACTOR_AND_VERIFY_TESTS[REFACTOR_AND_VERIFY_TESTS — see § refactor-and-verify-tests]
    REFACTOR_TEST_STRUCTURE_END((End))

    REFACTOR_AND_VERIFY_TESTS --> REFACTOR_TEST_STRUCTURE_END
```

## write-and-verify-acceptance-tests-fail

```mermaid
flowchart TD
    CALL_PARAMETERISED_CORE[CALL_PARAMETERISED_CORE — see § write-and-verify-acceptance-tests]
    WAV_AT_FAIL_END((End))

    CALL_PARAMETERISED_CORE --> WAV_AT_FAIL_END
```

## write-and-verify-acceptance-tests-pass

```mermaid
flowchart TD
    CALL_PARAMETERISED_CORE[CALL_PARAMETERISED_CORE — see § write-and-verify-acceptance-tests]
    WAV_AT_PASS_END((End))

    CALL_PARAMETERISED_CORE --> WAV_AT_PASS_END
```

## write-and-verify-acceptance-tests

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

## write-and-verify-acceptance-test-code

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
    GATE_EXPECTED_TEST_RESULT -- success --> VERIFY_TESTS_PASS_ACCEPTANCE
    GATE_EXPECTED_TEST_RESULT -- failure --> VERIFY_TESTS_FAIL_ACCEPTANCE
    VERIFY_TESTS_PASS_ACCEPTANCE --> COMMIT_TEST_CODE
    VERIFY_TESTS_FAIL_ACCEPTANCE --> DISABLE_ACCEPTANCE_TESTS
    DISABLE_ACCEPTANCE_TESTS --> COMMIT_TEST_CODE
    COMMIT_TEST_CODE --> WAV_AT_CODE_END
```

## implement-and-verify-dsl

```mermaid
flowchart TD
    IMPLEMENT_TEST_LAYER[IMPLEMENT_TEST_LAYER — see § implement-test-layer]
    IMPL_DSL_END((End))

    IMPLEMENT_TEST_LAYER --> IMPL_DSL_END
```

## implement-and-verify-system-driver-adapters

```mermaid
flowchart TD
    IMPLEMENT_TEST_LAYER[IMPLEMENT_TEST_LAYER — see § implement-test-layer]
    IMPL_SYS_DRIVER_END((End))

    IMPLEMENT_TEST_LAYER --> IMPL_SYS_DRIVER_END
```

## implement-and-verify-external-system-driver-adapters

```mermaid
flowchart TD
    IMPLEMENT_TEST_LAYER[IMPLEMENT_TEST_LAYER — see § implement-test-layer]
    IMPL_EXT_DRIVER_END((End))

    IMPLEMENT_TEST_LAYER --> IMPL_EXT_DRIVER_END
```

## implement-and-verify-external-system-driver-adapters-contract-tests

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

## implement-and-verify-system

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

## refactor-and-verify-tests

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

## implement-test-layer

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
    GATE_EXPECTED_TEST_RESULT -- success --> VERIFY_TESTS_PASS_FILTERED
    GATE_EXPECTED_TEST_RESULT -- failure --> VERIFY_TESTS_FAIL_FILTERED
    VERIFY_TESTS_PASS_FILTERED --> COMMIT_LAYER
    VERIFY_TESTS_FAIL_FILTERED --> DISABLE_TESTS
    DISABLE_TESTS --> COMMIT_LAYER
    COMMIT_LAYER --> IMPLEMENT_TEST_LAYER_END
```

## verify-tests-pass

```mermaid
flowchart TD
    FIX_UNEXPECTED_FAILING_TESTS[FIX_UNEXPECTED_FAILING_TESTS — see § fix-unexpected-failing-tests]
    GATE_TESTS_OUTCOME{All tests passed?}
    RUN_TESTS[RUN_TESTS — see § run-tests]
    VERIFY_PASS_END((End))

    RUN_TESTS --> GATE_TESTS_OUTCOME
    GATE_TESTS_OUTCOME -- pass --> VERIFY_PASS_END
    GATE_TESTS_OUTCOME -- fail --> FIX_UNEXPECTED_FAILING_TESTS
    FIX_UNEXPECTED_FAILING_TESTS --> VERIFY_PASS_END
```

## verify-tests-fail

```mermaid
flowchart TD
    FIX_UNEXPECTED_PASSING_TESTS[FIX_UNEXPECTED_PASSING_TESTS — see § fix-unexpected-passing-tests]
    GATE_TESTS_OUTCOME{Any tests passed?}
    RUN_TESTS[RUN_TESTS — see § run-tests]
    VERIFY_FAIL_END((End))

    RUN_TESTS --> GATE_TESTS_OUTCOME
    GATE_TESTS_OUTCOME -- pass --> FIX_UNEXPECTED_PASSING_TESTS
    GATE_TESTS_OUTCOME -- fail --> VERIFY_FAIL_END
    FIX_UNEXPECTED_PASSING_TESTS --> VERIFY_FAIL_END
```

## write-acceptance-tests

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    WAT_END((End))

    EXECUTE_AGENT --> WAT_END
```

## write-contract-tests

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    WCT_END((End))

    EXECUTE_AGENT --> WCT_END
```

## implement-dsl

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    IMPL_DSL_END((End))

    EXECUTE_AGENT --> IMPL_DSL_END
```

## implement-system

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    IMPL_SYS_END((End))

    EXECUTE_AGENT --> IMPL_SYS_END
```

## implement-system-driver-adapters

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    IMPL_SYS_DA_END((End))

    EXECUTE_AGENT --> IMPL_SYS_DA_END
```

## implement-external-system-driver-adapters

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    IMPL_EXT_DA_END((End))

    EXECUTE_AGENT --> IMPL_EXT_DA_END
```

## implement-external-system-stubs

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    IMPL_STUBS_END((End))

    EXECUTE_AGENT --> IMPL_STUBS_END
```

## disable-tests

```mermaid
flowchart TD
    DISABLE_END((End))
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]

    EXECUTE_AGENT --> DISABLE_END
```

## enable-tests

```mermaid
flowchart TD
    ENABLE_END((End))
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]

    EXECUTE_AGENT --> ENABLE_END
```

## fix-unexpected-passing-tests

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    FIX_PASS_END((End))

    EXECUTE_AGENT --> FIX_PASS_END
```

## fix-unexpected-failing-tests

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    FIX_FAIL_END((End))

    EXECUTE_AGENT --> FIX_FAIL_END
```

## refactor-tests

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    REFACTOR_TESTS_END((End))

    EXECUTE_AGENT --> REFACTOR_TESTS_END
```

## refactor-system

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    REFACTOR_SYS_END((End))

    EXECUTE_AGENT --> REFACTOR_SYS_END
```

## refine-acceptance-criteria

```mermaid
flowchart TD
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    REFINE_AC_END((End))

    EXECUTE_AGENT --> REFINE_AC_END
```

## compile

```mermaid
flowchart TD
    COMPILE_MID_END((End))
    EXECUTE_COMMAND[EXECUTE_COMMAND — see § execute-command]

    EXECUTE_COMMAND --> COMPILE_MID_END
```

## compile-system

```mermaid
flowchart TD
    COMPILE_SYS_END((End))
    EXECUTE_COMMAND[EXECUTE_COMMAND — see § execute-command]

    EXECUTE_COMMAND --> COMPILE_SYS_END
```

## compile-tests

```mermaid
flowchart TD
    COMPILE_TESTS_END((End))
    EXECUTE_COMMAND[EXECUTE_COMMAND — see § execute-command]

    EXECUTE_COMMAND --> COMPILE_TESTS_END
```

## build-system

```mermaid
flowchart TD
    BUILD_SYS_END((End))
    EXECUTE_COMMAND[EXECUTE_COMMAND — see § execute-command]

    EXECUTE_COMMAND --> BUILD_SYS_END
```

## start-system

```mermaid
flowchart TD
    EXECUTE_COMMAND[EXECUTE_COMMAND — see § execute-command]
    START_SYS_END((End))

    EXECUTE_COMMAND --> START_SYS_END
```

## commit

```mermaid
flowchart TD
    COMMIT_MID_END((End))
    EXECUTE_COMMAND[EXECUTE_COMMAND — see § execute-command]

    EXECUTE_COMMAND --> COMMIT_MID_END
```

## run-tests

```mermaid
flowchart TD
    EXECUTE_COMMAND[EXECUTE_COMMAND — see § execute-command]
    RUN_TESTS_END((End))

    EXECUTE_COMMAND --> RUN_TESTS_END
```

## approve

```mermaid
flowchart TD
    APPROVE_OK_END((End))
    APPROVE_REJECT_END((End))
    ASK_HUMAN["${question}"]
    GATE_APPROVED{Approved?}

    ASK_HUMAN --> GATE_APPROVED
    GATE_APPROVED -- approved --> APPROVE_OK_END
    GATE_APPROVED -- rejected --> APPROVE_REJECT_END

    classDef humanNode fill:#ffeb3b,stroke:#fbc02d,stroke-width:2px,color:#000000
    class ASK_HUMAN humanNode
```

## execute-agent

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

## execute-command

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

## fix

```mermaid
flowchart TD
    APPROVE_PRE[APPROVE_PRE — see § approve]
    EXECUTE_AGENT[EXECUTE_AGENT — see § execute-agent]
    FIX_END((End))

    APPROVE_PRE --> EXECUTE_AGENT
    EXECUTE_AGENT --> FIX_END
```

