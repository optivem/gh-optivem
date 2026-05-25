# BPMN - LOW LEVEL

# APPROVE

INPUT: Prompt for user

1. Confirm Approval [HUMAN]
2. Approved
    1. YES: END
    2. NO: HARD EXIT (print out EXIT because approval was not obtained)

TODO (Phase C revisit): APPROVE is currently exit-only on NO; callers own any retry behaviour (Q3=A). Future options to reconsider: B) parameterized NO-action (`exit` | `retry-caller`), or C) split into two primitives `APPROVE-OR-EXIT` and `APPROVE-OR-RETRY`.

# EXECUTE AGENT

INPUT: Agent Name, Prompt, Scope, Output

1. Approve (PRE): Do you approve Agent <Agent Name> to run?
2. Run Agent <Agent Name> <Prompt>
3. Validate Output & Scope
    1. Output: are the required output variables present?
    2. Scope: were the scope constraints satisfied? (diff)
4. Valid?
    1. NO: calls FIX (input: failure context — failed validation, missing/invalid outputs, scope-diff violations)
5. Approve (POST)

# EXECUTE COMMAND

<!-- Intentional asymmetry vs EXECUTE AGENT (Q2=C): no Approve (POST), because commands have machine-checkable success whereas agents produce content needing human review. -->

1. Approve (PRE)
2. Run Command <Command> <Input Params>
3. Success?
    1. NO: calls FIX (input: failure context — command name, input params, stderr/exit code)

# FIX

INPUT: Failure Context (what failed, why — e.g., validation errors, command stderr/exit code, scope-diff violations)

1. Approve (PRE): Do you approve FIX to attempt remediation for <failure summary>?
2. Run Fix Agent <Failure Context>
3. END (single attempt, no recursion — terminates regardless of outcome)
