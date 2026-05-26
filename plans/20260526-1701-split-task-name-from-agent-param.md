# Split `task-name` from `agent` in process-flow.yaml

## Origin / intent

Follow-up to `plans/20260526-1653-rename-prompts-folder-to-agents.md`
(folder rename). That plan keeps the existing convention where one
identifier wears three hats:

> `task-name` (the BPMN task identifier) = `agent` name (templated as
> `agent: ${task-name}` on RUN_AGENT) = `.md` filename stem

This plan replaces the one-identifier convention with two explicit
fields:

- **`task-name`** (verb) — the unit of work. Used for scope lookup,
  fix-recovery tracing, BPMN identity, and substitution in
  documentation/logs.
- **`agent`** (noun) — the actor that performs the task. Used for
  dispatcher routing and brief-file resolution.

Workshop motivation: students should learn that BPMN tasks are verbs
(work) and agents are nouns (actors). The current 1-string-3-hats
collapse blurs this. Splitting also opens the door — though does not
require — reusing one agent across multiple tasks if a future
refactor finds genuinely shared behaviour.

## Pre-requisite

This plan assumes `plans/20260526-1653-rename-prompts-folder-to-agents.md`
has landed. That plan moves the brief files into
`internal/assets/runtime/agents/atdd/` (keeping verb-shaped
filenames). This plan then renames each file to its noun form.

## Observation

The current `execute-agent` sub-process resolves the agent by
templating from `task-name`:

```yaml
# process-flow.yaml:1754-1757
- id: RUN_AGENT
  type: user-task
  agent: ${task-name}             # ← templated from caller's task-name param
  documentation: "Run agent ${task-name}"
```

Every MID node passes `task-name` as a call-activity param into
`execute-agent`. The agent name and the filename stem both equal
the `task-name` string by convention. There is no place in YAML
that says "this task is performed by this specific agent" — the
mapping is implicit in shared naming.

## Resolution (settled during refinement)

### Mapping mode: 1:1 task → agent

Every task gets its own agent file. 17 tasks → 17 agents. Preserves
the focused-brief / token-efficiency property. Collapsing where
multiple tasks share behaviour stays available as a later
optimisation; first pass is purely a naming/structural change with
no semantic change.

### Noun-name mapping

| Task (verb) | Agent (noun) |
|---|---|
| `write-acceptance-tests` | `acceptance-test-writer` |
| `write-contract-tests` | `contract-test-writer` |
| `implement-dsl` | `dsl-implementer` |
| `implement-system` | `system-implementer` |
| `implement-system-driver-adapters` | `system-driver-adapter-implementer` |
| `implement-external-system-stubs` | `external-system-stub-implementer` |
| `implement-external-system-driver-adapters` | `external-system-driver-adapter-implementer` |
| `refactor-system` | `system-refactorer` |
| `refactor-tests` | `test-refactorer` |
| `disable-tests` | `test-disabler` |
| `enable-tests` | `test-enabler` |
| `refine-acceptance-criteria` | `acceptance-criteria-refiner` |
| `fix-command-failed` | `command-failure-diagnoser` |
| `fix-missing-output` | `missing-output-diagnoser` |
| `fix-scope-diff` | `scope-diff-diagnoser` |
| `fix-unexpected-failing-tests` | `unexpected-failing-test-diagnoser` |
| `fix-unexpected-passing-tests` | `unexpected-passing-test-diagnoser` |

**Why `-diagnoser` for the fix-* agents.** The brief in
`fix-command-failed.md` explicitly says "Your job is **diagnosis**,
not repair" — the agent emits a proposed change and exits; the
caller's PRE step approves it, the caller's verify step re-runs the
command. `-diagnoser` is the honest noun. The BPMN task verb `fix-*`
is itself misleading; that's not corrected here — see "Future plan"
below.

### YAML shape change

**Before** (1-string-3-hats):
```yaml
write-acceptance-tests:
  start: EXECUTE_AGENT
  nodes:
    - id: EXECUTE_AGENT
      type: call-activity
      process: execute-agent
      params:
        task-name: write-acceptance-tests
```

**After** (two explicit fields):
```yaml
write-acceptance-tests:
  start: EXECUTE_AGENT
  nodes:
    - id: EXECUTE_AGENT
      type: call-activity
      process: execute-agent
      params:
        task-name: write-acceptance-tests        # the work (verb)
        agent: acceptance-test-writer            # the actor (noun)
```

And inside `execute-agent`:
```yaml
- id: RUN_AGENT
  type: user-task
  agent: ${agent}                                # was: ${task-name}
  documentation: "Run agent ${agent} (task: ${task-name})"
```

The `task-name` param is **kept**, not removed — it's still used
for scope lookup (`engine.Scope(task-name)`), `originating-task-name`
fix-recovery plumbing, and human-readable trace/audit output. Only
the agent-resolution path changes.

## Items

### Item 1 — Add `agent:` param to every MID call-activity

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

For each of the 17 MID nodes that call `execute-agent`, add an
`agent:` param next to `task-name:`. Use the noun mapping above.

There's also one indirect call-site to handle: the `fix`
sub-process (around line 1880-1903) constructs its own dispatch:

```yaml
params:
  task-name: "fix-${failure-kind}"
  fix-on-failure: "false"
  originating-task-name: ${task-name}
  outputs: ${outputs}
```

Since `failure-kind` is one of five known values, the agent name is
derivable as `${failure-kind}-diagnoser` (or a more explicit table
mapping). Decision: add an `agent:` param computed the same way:

```yaml
params:
  task-name: "fix-${failure-kind}"
  agent: "${failure-kind}-diagnoser"
  ...
```

Verify ExpandParams handles the trailing-suffix substitution shape.
If not, fall back to a small lookup table populated by a binding —
note as a sub-item to resolve at execution time.

### Item 2 — Update `execute-agent` sub-process RUN_AGENT

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`
(around line 1754)

```yaml
- id: RUN_AGENT
  type: user-task
  agent: ${agent}
  documentation: "Run agent ${agent} (task: ${task-name})"
```

Change `${task-name}` → `${agent}` on the `agent:` line. Update the
documentation string to include both for trace clarity.

Also update the explanatory comment block at lines 1711-1716 to
describe the new two-field shape.

### Item 3 — Rename the 17 brief files

After plan `20260526-1653` lands, the files live at
`internal/assets/runtime/agents/atdd/<task-name>.md`. Rename via
`git mv`, one per row of the noun-mapping table.

```
git mv internal/assets/runtime/agents/atdd/write-acceptance-tests.md \
       internal/assets/runtime/agents/atdd/acceptance-test-writer.md
# ... (17 total)
```

### Item 4 — Update embed loader resolution

**File:** `internal/atdd/runtime/agents/embed.go`

The current `Prompt(name)` / `LoadTuning(name)` functions look up
`<name>.md` under the embedded dir. After this plan, `name` is the
**agent** noun (e.g., `acceptance-test-writer`), not the task
verb. The functions don't change shape — just the values their
callers pass.

Verify that `Names()` enumerates the renamed files correctly. The
function should still return all 17 names; it's just the strings
that change.

### Item 5 — Update the driver dispatch wiring

**File:** `internal/atdd/runtime/driver/driver.go`

`registerAgentDispatchers` (around line 672) iterates `agents.Names()`
and registers a no-op base dispatcher per agent. No code change
needed — the iteration is name-agnostic — but verify that the
clauderun shell-out (`agent: ${agent}` resolution path) reads from
the right param after Item 2's templating change.

Trace the agent-name plumbing from `RUN_AGENT`'s `agent:` field
through to the `claude -p` invocation to confirm the substitution
order doesn't double-expand or skip.

### Item 6 — Update tests

**File:** `internal/atdd/runtime/agents/embed_test.go`

- `TestFixKindAgentsExist` (renamed from `TestFixKindPromptsExist`
  per the folder-rename plan) — the `wantKinds` list currently
  contains failure-kinds like `"command-failed"` and expects
  `"fix-" + kind` to be an embedded name. After this plan, the
  embedded name is `<kind>-diagnoser`, so update the assertion:
  ```go
  for _, kind := range wantKinds {
      agentName := kind + "-diagnoser"   // was: "fix-" + kind
      if !names[agentName] {
          t.Errorf("missing agent %q for failure-kind %q", agentName, kind)
      }
  }
  ```
- `TestLoadTuning_WriteAcceptanceTests` — rename to
  `TestLoadTuning_AcceptanceTestWriter` and update the `LoadTuning`
  argument to `"acceptance-test-writer"`.
- `TestLoadTuning_EveryAgentDeclaresTuning` — name-agnostic, no
  change.

**File:** other tests that hard-code task or agent names — grep for
the 17 verb names and update where the test asserts on agent name
specifically (vs. task name).

### Item 7 — Update process-diagram regeneration

**File:** `docs/process-diagram.md` (auto-regenerated)

Re-run `gh optivem architecture show` (or whichever command
regenerates the diagram from the YAML) so the diagram reflects the
new `agent:` params. Do not edit the diagram by hand — per
CONTRIBUTING.md.

### Item 8 — Update doc references

Grep for the 17 verb names across `docs/atdd/process/`,
`docs/atdd/architecture/`, and CONTRIBUTING.md. For each hit,
decide whether the doc is referring to the **task** (keep verb name)
or the **agent** (update to noun). Most process docs talk about
tasks; architecture docs may talk about agents.

Likely-affected files (re-grep at execution time):
- `docs/atdd/process/*.md` (process narrative)
- `docs/atdd/architecture/*.md` (component descriptions)
- `docs/how-it-works.md` (if it walks through dispatch)

### Item 9 — Build + test

```
go build ./...
scripts/test.sh
```

Per repo guidance: never `go test ./...` unbounded on Windows.

### Item 10 — End-to-end smoke

```
gh optivem implement --issue <test-issue> --dry-run     # or equivalent
```

Verify the trace output shows `agent: acceptance-test-writer (task:
write-acceptance-tests)` style output, not the old collapsed form.

## Out of scope (explicitly)

- **Renaming `fix-*` tasks to `diagnose-*`.** The task verbs are
  misleading (they say "fix" but the agents diagnose). That's a
  separate follow-up plan — see "Future plan" below. This plan only
  renames the agents to honest nouns; the task verbs stay as-is.
- **Changing diagnosis-only behaviour to actual repair.** The current
  diagnose-and-propose design has real safety properties (human
  approval gate on every change, bounded scope, bisection clarity).
  Removing those is a design conversation, not part of this plan.
- **Collapsing multiple tasks under one agent.** Stays 1:1 in this
  pass. If a future refactor finds two tasks doing essentially
  identical work, a separate plan can collapse them — the `agent:`
  field already supports it.
- **Consumer-repo schema changes** (`task_prompts:` field,
  `config/prompts/` paths in CONTRIBUTING.md). Operator-facing
  schema; separate decision.

## Future plan (referenced, not scheduled)

A separate small plan (suggested filename
`plans/<YYYYMMDD-HHMM>-rename-fix-tasks-to-diagnose.md`) should
rename the `fix-*` BPMN tasks to `diagnose-*` so the task verb
matches what the agent does. Scope:

- Rename MID YAML keys: `fix-command-failed:` → `diagnose-command-failure:`
  (and four siblings).
- Update `failure-kind` values and the templated `task-name:
  "fix-${failure-kind}"` form.
- Update originating-task-name references and scope-lookup keys.
- Update tests, docs, and diagram.

That plan may also be extended to revisit the diagnosis-only design
itself — i.e., whether the agents should apply the proposed change
instead of just emitting it. That's a real architecture change with
trade-offs (loses approval-gate safety, loses bisection clarity)
and should not be folded into a naming plan.

## Rollback

`git revert` on the merge commit restores both the YAML shape and
the brief filenames. Asset embedding picks up the restored tree on
rebuild; no separate registration to unwind.

## Pickup marker

(Add `**Pickup:** YYYY-MM-DD HH:MM <agent>` here when execution
begins.)
