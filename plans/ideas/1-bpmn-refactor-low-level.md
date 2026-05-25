# BPMN - LOW LEVEL

> **Design content only.** Open questions, doctrine choices, and decision rationale belong in the parent plan: `plans/20260525-1057-bpmn-refactor-design.md`. Do not record Q&A here. If you encounter a question while reading or editing this file, add it to the plan, not inline.

## approve

**Inputs:**
- prompt

**Outputs:** NONE

**Steps:**
1. Confirm Approval [HUMAN]
2. Approved?
    1. YES: END
    2. NO: HARD EXIT (print out EXIT because approval was not obtained)

## execute-agent

**Inputs:**
- task-name
- prompt
- scopes
- outputs

**Outputs:**
- Agent output values (as declared by caller's `outputs` input)

**Steps:**
1. `approve` (PRE): Do you approve task [task-name] to run?
2. Run agent for task [task-name] [prompt]
3. Validate outputs & scopes
    1. Outputs: are the required output variables present?
    2. Scopes: were the scope constraints satisfied? (diff)
4. Valid?
    1. NO: calls `fix` (input: `failure-context` — failed validation, missing/invalid outputs, scope-diff violations)
5. `approve` (POST)

## execute-command

**Inputs:**
- command
- params

**Outputs:** NONE

**Steps:**
1. `approve` (PRE)
2. Run command [command] [params]
3. Success?
    1. NO: calls `fix` (input: `failure-context` — command, params, stderr/exit code)

## fix

**Inputs:**
- failure-context (what failed, why — e.g., validation errors, command stderr/exit code, scope-diff violations)

**Outputs:** NONE

**Steps:**
1. `approve` (PRE): Do you approve `fix` to attempt remediation for [failure-summary]?
2. Run Fix Agent [failure-context]
3. END (single attempt, no recursion — terminates regardless of outcome)
