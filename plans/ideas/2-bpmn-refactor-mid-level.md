# BPMN - MID LEVEL

> **Naming convention (Q29).** All process-model identifiers use kebab-case lowercase across YAML, doc headings, prompt filenames, and in-prose references. Layer labels (TOP / CYCLE / HIGH / MID / LOW) remain organizational categories only and are not part of identifier names.

## Agent Tasks

Each of these agent tasks calls the low level `execute-agent` task, passing in agent name, scopes and outputs.

For example `write-acceptance-tests`
calls `execute-agent` with this:
- task-name: `write-acceptance-tests` (was `agent-name: at-red-test` — see Q28.a below)
- scopes: acceptance-tests, dsl-ports
- outputs: dsl-port-changed: true/false

Note (per Q13=A): the contract fields (`scopes`, `outputs`) are authored in `process-flow.yaml` as `user_task` metadata — that YAML is the single source of truth. They are shown inline here only for illustration. The post-execute BPMN verify step reads the same `outputs:` / `scopes:` from the YAML, so there is no separate `file:` field — permitted file scope is absorbed into `scopes:`.

Note (Q24 + Q28.a, resolved in child plan `plans/20260525-1130-bpmn-naming-doctrine.md`): the existing prompt names under `internal/assets/runtime/prompts/atdd/` (e.g., `at-red-test`, `at-red-dsl`, `ct-red-test`) get renamed to verb-based, exactly matching the MID task names (`at-red-test` → `write-acceptance-tests`, `at-red-dsl` + `ct-red-dsl` → `implement-dsl`, `ct-red-test` → `write-contract-tests`, etc.). Per Q28.a, the `agent-name:` YAML field is DROPPED entirely — runtime derives the prompt path deterministically from the MID task name as `task-name + ".md"`. Legacy `legacy-*` prompts collapse mechanically per Q16=B. Rename work executes in parent plan's Item 10 (Phase D downstream-alignment plan).

Here's the list of the agent tasks (calling `execute-agent`):
- `write-acceptance-tests`
- `write-contract-tests`
- `implement-dsl`
- `implement-system-drivers`
- `implement-external-system-drivers`
- `disable-tests`
- `enable-tests`
- `fix-unexpected-passing-tests`
- `fix-unexpected-failing-tests`

## Command Tasks

All these call the low level `execute-command` task, passing in command and params.

For example:
`compile` calls `execute-command` with this:
- command: gh optivem
- params: compile

`run-tests` is a single task with a polymorphic filter parameter (per Q5=A-modified). For example:
`run-tests` calls `execute-command` with this:
- command: gh optivem
- params: <test filter>

The `<test filter>` accepts one of three forms:
1. A test-type tag — one of `acceptance`, `contract`, `acceptance-api`, `acceptance-ui`, `contract-stub`, `contract-real`.
2. A list of specific test names — used by `change-system-behavior` when ACs dictate the exact tests to run.
3. No filter — runs all tests.

Full list:
- `compile`
- `compile-system`
- `compile-tests`
- `commit`
- `run-tests`
