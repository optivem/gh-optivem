# BPMN - LOW LEVEL

> **Design content only.** Open questions, doctrine choices, and decision rationale belong in the parent plan: `plans/20260525-1057-bpmn-refactor-design.md`. Do not record Q&A here. If you encounter a question while reading or editing this file, add it to the plan, not inline.

## approve

**Inputs:**
- Prompt for user

**Outputs:** NONE

**Steps:**
1. Confirm Approval [HUMAN]
2. Approved?
    1. YES: END
    2. NO: HARD EXIT (print out EXIT because approval was not obtained)

## execute-agent

**Inputs:**
- Task Name
- Prompt
- Scope
- Output

**Outputs:**
- Agent output values (as declared by caller's Output input)

**Steps:**
1. Approve (PRE): Do you approve task <Task Name> to run?
2. Run agent for task <Task Name> <Prompt>
3. Validate Output & Scope
    1. Output: are the required output variables present?
    2. Scope: were the scope constraints satisfied? (diff)
4. Valid?
    1. NO: calls `fix` (input: failure context — failed validation, missing/invalid outputs, scope-diff violations)
5. Approve (POST)

## execute-command

**Inputs:**
- Command
- Input Params

**Outputs:** NONE

**Steps:**
1. Approve (PRE)
2. Run Command <Command> <Input Params>
3. Success?
    1. NO: calls `fix` (input: failure context — command name, input params, stderr/exit code)

## fix

**Inputs:**
- Failure Context (what failed, why — e.g., validation errors, command stderr/exit code, scope-diff violations)

**Outputs:** NONE

**Steps:**
1. Approve (PRE): Do you approve `fix` to attempt remediation for <failure summary>?
2. Run Fix Agent <Failure Context>
3. END (single attempt, no recursion — terminates regardless of outcome)
