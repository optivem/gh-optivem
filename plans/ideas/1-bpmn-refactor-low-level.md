# BPMN - LOW LEVEL

> **Naming convention (Q29).** All process-model identifiers use kebab-case lowercase across YAML, doc headings, prompt filenames, and in-prose references. Layer labels (TOP / CYCLE / HIGH / MID / LOW) remain organizational categories only and are not part of identifier names.

# approve

INPUT: Prompt for user

1. Confirm Approval [HUMAN]
2. Approved
    1. YES: END
    2. NO: HARD EXIT (print out EXIT because approval was not obtained)

TODO (Phase C revisit): `approve` is currently exit-only on NO; callers own any retry behaviour (Q3=A). Future options to reconsider: B) parameterized NO-action (`exit` | `retry-caller`), or C) split into two primitives `approve-or-exit` and `approve-or-retry`.

# execute-agent

INPUT: Task Name, Prompt, Scope, Output

Note: `Task Name` replaced the prior `Agent Name` per Q28.a (`agent-name:` YAML field dropped). Runtime derives the prompt path deterministically from `Task Name` (`prompt_path(task_name) = task_name + ".md"`).

1. Approve (PRE): Do you approve task <Task Name> to run?
2. Run agent for task <Task Name> <Prompt>
3. Validate Output & Scope
    1. Output: are the required output variables present?
    2. Scope: were the scope constraints satisfied? (diff)
4. Valid?
    1. NO: calls `fix` (input: failure context — failed validation, missing/invalid outputs, scope-diff violations)
5. Approve (POST)

# execute-command

<!-- Intentional asymmetry vs `execute-agent` (Q2=C): no Approve (POST), because commands have machine-checkable success whereas agents produce content needing human review. -->

1. Approve (PRE)
2. Run Command <Command> <Input Params>
3. Success?
    1. NO: calls `fix` (input: failure context — command name, input params, stderr/exit code)

# fix

INPUT: Failure Context (what failed, why — e.g., validation errors, command stderr/exit code, scope-diff violations)

1. Approve (PRE): Do you approve `fix` to attempt remediation for <failure summary>?
2. Run Fix Agent <Failure Context>
3. END (single attempt, no recursion — terminates regardless of outcome)
