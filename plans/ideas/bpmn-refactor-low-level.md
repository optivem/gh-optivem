# BPMN - LOW LEVEL

*FUTURE IDEA: For FIX, currently one task. Maybe 2, to separate planning vs execution, so that human approves…*

# APPROVE

INPUT: Prompt for user 

1. Confirm Approval [HUMAN]
2. Approved
    1. YES: END
    2. NO: HARD EXIT (print out EXIT because approval was not obtained)

# EXECUTE AGENT

INPUT: Agent Name, Prompt, Scope, Output

1. Approve (PRE): Do you approve Agent <Agent Name> to run?
2. Run Agent <Agent Name> <Prompt>
3. Validate Output & Scope
    1. Output: are the required output variables present?
    2. Scope: were the scope constraints satisfied? (diff)
4. Valid?
    1. NO: Fix Agent Run (AGENT)
5. Approve (POST)

# EXECUTE COMMAND

1. Approve (PRE)
2. Run Command <Command> <Input Params>
3. Success?
    1. NO: Fix Command Run (AGENT)