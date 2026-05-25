# BPMN - MID LEVEL

## Agent Tasks

Each of these agent tasks inherits the low level EXECUTE AGENT task, passing in agent naem, file, scopes and outputs

For example Write Acceptance Tests
calls EXECUTE AGENT with this:
- agent-name: at-red-test
- file: _________ (maybe not needed?)
- scopes: acceptance-tests, dsl-ports
- outputs: dsl-port-changed: true/false


Here's list of the agent tasks (inheriting EXECUTE AGENT):
- Write Acceptance Tests
- Implement DSL
- Implement System Drivers
- Implement External System Drivers
- Disable Tests
- Enable Tests
- Fix Unexpected Passing Tests
- Fix Unexpected Failing Tests

## Command Tasks

All these inherit the low level command tasks, passing in command and params

For example:
COMPILE inehrits the EXECUTE COMMAND task with this:
- command: gh optivem
- params: compile

Full list:
- Compile
- Compile System
- Compile Tests
- Commit
- Run Tests (it can run all tests or we pass some filter to be selective... not sure if one task or multiple)