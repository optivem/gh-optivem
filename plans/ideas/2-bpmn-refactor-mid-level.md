# BPMN - MID LEVEL

## Agent Tasks

Each of these agent tasks calls the low level EXECUTE AGENT task, passing in agent name, scopes and outputs.

For example Write Acceptance Tests
calls EXECUTE AGENT with this:
- agent-name: at-red-test
- scopes: acceptance-tests, dsl-ports
- outputs: dsl-port-changed: true/false

Note (per Q13=A): the contract fields (`agent-name`, `scopes`, `outputs`) are authored in `process-flow.yaml` as `user_task` metadata — that YAML is the single source of truth. They are shown inline here only for illustration. The post-execute BPMN verify step reads the same `outputs:` / `scopes:` from the YAML, so there is no separate `file:` field — permitted file scope is absorbed into `scopes:`.

Note (Q24 — deferred to Phase D's downstream-alignment plan): the existing prompt names under `internal/assets/runtime/prompts/atdd/` (e.g., `at-red-test`, `at-red-dsl`, `ct-red-test`) are noun-based cycle-phase identifiers. Doctrine: prompts rename to verb-based, exactly matching the MID task names (`at-red-test` → `write-acceptance-tests`, `at-red-dsl` + `ct-red-dsl` → `implement-dsl`, `ct-red-test` → `write-contract-tests`, etc.). The `agent-name:` YAML field likely renames to `task-name:` or `executor:` (decided in Phase D). Legacy `legacy-*` prompts collapse mechanically per Q16=B since task names are agnostic about expected test result. Rename work tracked in Item 10's downstream-alignment plan.

Here's the list of the agent tasks (calling EXECUTE AGENT):
- Write Acceptance Tests
- Write Contract Tests
- Implement DSL
- Implement System Drivers
- Implement External System Drivers
- Disable Tests
- Enable Tests
- Fix Unexpected Passing Tests
- Fix Unexpected Failing Tests

## Command Tasks

All these call the low level EXECUTE COMMAND task, passing in command and params.

For example:
COMPILE calls the EXECUTE COMMAND task with this:
- command: gh optivem
- params: compile

Run Tests is a single task with a polymorphic filter parameter (per Q5=A-modified). For example:
Run Tests calls EXECUTE COMMAND with this:
- command: gh optivem
- params: <test filter>

The `<test filter>` accepts one of three forms:
1. A test-type tag — one of `acceptance`, `contract`, `acceptance-api`, `acceptance-ui`, `contract-stub`, `contract-real`.
2. A list of specific test names — used by CHANGE SYSTEM BEHAVIOR when ACs dictate the exact tests to run.
3. No filter — runs all tests.

Full list:
- Compile
- Compile System
- Compile Tests
- Commit
- Run Tests
