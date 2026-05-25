# BPMN - LOW LEVEL

> **Design content only.** Open questions, doctrine choices, and decision rationale belong in the parent plan: `plans/20260525-1057-bpmn-refactor-design.md`. Do not record Q&A here. If you encounter a question while reading or editing this file, add it to the plan, not inline.

## approve

**Inputs:**
- question

**Outputs:** NONE

**Steps:**
1. Confirm Approval [HUMAN]
2. Approved?
    1. YES: END
    2. NO: END ERROR (because approval was not obtained)

## execute-agent

**Inputs:**
- task-name
- params
- scopes
- outputs
- fix-on-failure (default: true)

**Outputs:**
- Agent output values (as declared by caller's `outputs` input)

**Steps:**
1. `approve` (PRE): Do you approve task [task-name] to run?
2. Run agent for task [task-name] [params]
3. Validate outputs & scopes
    1. Outputs: are the required [outputs] variables present?
    2. Scopes: were the [scopes] constraints satisfied? (diff)
4. Valid?
    1. NO (if [fix-on-failure]): calls `fix` (failure=<{kind: missing-output | scope-diff, ...}>, scopes=[scopes], outputs=[outputs])
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
    1. NO: calls `fix` (failure=<{kind: command-failed, command, params, stderr/exit code}>)

## fix

**Inputs:**
- failure (payload includes `kind` field used to derive the fix-* task: e.g., `missing-output`, `scope-diff`, `command-failed`, `unexpected-passing-tests`, `unexpected-failing-tests`)
- scopes (passed through from failing task; omitted when caller is `execute-command`)
- outputs (passed through from failing task; omitted when caller is `execute-command`)

**Outputs:** NONE

**Steps:**
1. `approve` (PRE): Do you approve `fix` to attempt remediation for [failure]?
2. `execute-agent` (task-name="fix-" + [failure].kind, params=[failure], scopes=[scopes], outputs=[outputs], fix-on-failure=false)
3. END (single attempt, no recursion — terminates regardless of inner outcome)
