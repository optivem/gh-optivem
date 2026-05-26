# Approve-prompt: name the kind (agent vs command vs fix)

## Origin / intent

Conversation with user (2026-05-26) triggered by a rehearsal trace:

```
[trace 12:45:11] > ASK_HUMAN  kind=user-task agent=human
[ASK_HUMAN] Do you approve task implement-system-driver-adapters to run?
```

The operator could not tell from the prompt text whether they were
authorising an agent dispatch, a shell command, or a fix attempt — the
word "task" is ambiguous. The trace metadata (`kind=user-task
agent=human`) describes the BPMN node type, not the semantic of the
upcoming action.

The `approve` LOW primitive is caller-driven: `newApproveDispatcher`
(`internal/atdd/runtime/driver/driver.go:775`) just prints the
`${question}` string the parent process passes through `params:`. So
the kind signal must live in each caller's `question:` string in
`internal/atdd/runtime/statemachine/process-flow.yaml`.

Three call-sites today:

| Caller            | Line | Old text                                                      | Status        |
|-------------------|------|---------------------------------------------------------------|---------------|
| `execute-agent` PRE  | 1579 | `Do you approve task ${task-name} to run?`                  | ambiguous     |
| `execute-agent` POST | 1609 | `Approve agent output for task ${task-name}?`               | mixed wording |
| `execute-command` PRE | 1640 | `Do you approve running command ${command} ?`              | already explicit |
| `fix` PRE         | 1684 | `Do you approve fix to attempt remediation for ${failure} ?`  | already explicit |

The user does not need the full expanded shell command — just the
**kind** (agent / command / fix). User quote: "I just want the human
to know if command or agent".

## Scope

In scope:

- `internal/atdd/runtime/statemachine/process-flow.yaml` — replace
  "task" with "agent" in the two `execute-agent` `question:` strings so
  every approve prompt self-identifies as one of agent / command / fix.
- `internal/atdd/runtime/driver/driver_test.go` — the
  `TestApproveDispatcher_QuestionExpandsParams` fixture (line 1002) and
  its assertion (line 1011) embed the old wording. Update to match.
- `docs/process-diagram.md` — auto-generated from the YAML; regenerate
  via `gh optivem process show > docs/process-diagram.md` so the
  rendered call-out boxes match.

Out of scope:

- Adding kind metadata to the trace line itself (e.g. teaching
  `newApproveDispatcher` to print `[ASK_HUMAN kind=agent]`). The
  caller-driven `${question}` already carries the signal; threading
  kind through the dispatcher would be redundant.
- Printing the full agent prompt body or expanded shell command in the
  approve prompt. The user explicitly said the kind alone is enough.
- `plans/ideas/1-bpmn-refactor-low-level.md:31` — uses the old wording
  in a historical spec doc inside `plans/ideas/`. Not load-bearing;
  leave as-is.

## Items

### Item 1 — YAML: rename "task" → "agent" in execute-agent prompts [DONE in this session]

`internal/atdd/runtime/statemachine/process-flow.yaml`:

- Line 1579: `Do you approve task ${task-name} to run?` →
  `Do you approve agent ${task-name} to run?`
- Line 1609: `Approve agent output for task ${task-name}?` →
  `Approve output from agent ${task-name}?`

Already applied in the working tree before this plan was written.
Uncommitted.

### Item 2 — Test: realign `TestApproveDispatcher_QuestionExpandsParams`

`internal/atdd/runtime/driver/driver_test.go`:

- Line 1002 `strings.Replace` argument: change
  `"Do you approve task ${task-name} to run?"` →
  `"Do you approve agent ${task-name} to run?"`.
- Line 1011 substring assertion: change
  `"task write-acceptance-tests"` → `"agent write-acceptance-tests"`.

Run the package to confirm green:

```
go test ./internal/atdd/runtime/driver/...
```

(per [feedback_go_test_windows] — do not run `go test ./...` unbounded
on Windows.)

### Item 3 — Regenerate process diagram

Run from repo root:

```
gh optivem process show > docs/process-diagram.md
```

Verify the diff only touches the two prompt strings inside the
`execute-agent` section (call-out labels on `APPROVE_PRE` / `APPROVE_POST`).
No other node labels should change.

### Item 4 — Commit

Single commit covering YAML + test + regenerated diagram. Suggested
message:

```
atdd/runtime: name the kind in approve prompts (agent/command/fix)

execute-agent's PRE/POST approve questions said "task" — ambiguous
with the command and fix call-sites that already self-identify.
Rename to "agent" so every approve prompt opens with one of three
unambiguous lead-ins. Test fixture and regenerated process diagram
follow.
```

Gate for explicit user approval before staging
(per [feedback_no_commit_without_approval]).
