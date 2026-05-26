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

## Coordination with in-flight plans

**Cannot run in parallel with `plans/20260526-1448-agent-prompt-fixes.md`.**
Plan 1448's own execution strategy declares cross-session parallel
windows blocked by file ownership: "`process-flow.yaml`, prompt
frontmatter+body in the same `.md` files, and the plan file itself
are all shared surfaces." This plan touches both shared surfaces:

- All 17 brief `.md` files — renamed (verb → noun) in Item 3.
- `process-flow.yaml` — `agent:` param added to every MID
  call-activity in Item 1, RUN_AGENT updated in Item 2.

**Execution order (strict):**

1. `plans/20260526-1448-agent-prompt-fixes.md` lands fully (all
   sessions complete, pickup marker removed).
2. `plans/20260526-1653-rename-prompts-folder-to-agents.md` lands
   (folder rename `prompts/atdd/` → `agents/atdd/`).
3. This plan executes against the post-1653 tree.

Attempting parallel execution risks rename-vs-content conflicts on
the same 17 files (1448 edits bodies; 1701 changes filenames) and
overlapping edits on `process-flow.yaml`.

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
derivable as `${failure-kind}-diagnoser`. Add an `agent:` param
with the templated form:

```yaml
params:
  task-name: "fix-${failure-kind}"
  agent: "${failure-kind}-diagnoser"
  ...
```

**ExpandParams compatibility confirmed during refinement.**
`internal/atdd/runtime/statemachine/run.go:306-308` implements
substitution as a literal `strings.ReplaceAll` over `${key}` within
any surrounding string, and
`TestExpandParams_NilStateBehavesLikeOldSignature`
(`run_test.go:368-376`) already exercises the `${var}-${var}` shape.
The suffix form `${failure-kind}-diagnoser` is the simpler
single-substitution case and resolves cleanly to
`command-failed-diagnoser` etc. No fallback table needed; no
execution-time verification required.

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

### Item 3 — Rename the 17 brief files (mechanical move)

After plan `20260526-1653` lands, the files live at
`internal/assets/runtime/agents/atdd/<task-name>.md`. Rename via
`git mv`, one per row of the noun-mapping table.

```
git mv internal/assets/runtime/agents/atdd/write-acceptance-tests.md \
       internal/assets/runtime/agents/atdd/acceptance-test-writer.md
# ... (17 total)
```

**Scope: file move only.** No content changes in this item. Body
self-references that contradict the new filename (e.g.
`fix-command-failed.md`'s opening "You are running the
`fix-command-failed` task") are addressed in Item 3a after the
moves land.

### Item 3a — Update brief-body self-references

**Depends on:** Item 3 (files renamed first).

Some briefs open by naming themselves with the old verb identifier
— e.g. `fix-command-failed.md:5` says "You are running the
`fix-command-failed` task." After Item 3 renames the file to
`command-failure-diagnoser.md`, that opening sentence is
inconsistent with the filename.

**What to update.** For each renamed brief, audit the first
section (typically a one-sentence opener and any "Why you were
dispatched" / "Role in the flow" paragraph) for self-references
to the old verb identifier. Rewrite to use the noun identity:

- `fix-command-failed.md` opener: "You are running the
  `fix-command-failed` task" → "You are the
  `command-failure-diagnoser` agent."
- Repeat for the other four fix-* briefs (`-missing-output`,
  `-scope-diff`, `-unexpected-failing-tests`,
  `-unexpected-passing-tests`).
- Spot-check the 12 non-fix briefs: many do not have an "You are
  running the X task" opener (e.g. `write-acceptance-tests.md`
  goes straight to "The Acceptance Criteria below…"). Where there
  is no self-reference, no edit needed.

**Strict scope.** Edits are confined to opening sentences /
opening paragraphs that name the brief by its old verb identifier.
**Do not** rewrite Steps, Outputs, Anti-patterns, or any other
body section — those describe the work and stay verb-shaped
("Diagnose the failure", "Write tests for them directly"), which
is correct.

**Grep to find candidates at execution time:**

```
grep -nE "You are running the.*task" internal/assets/runtime/agents/atdd/*.md
grep -nE "the [a-z-]+-[a-z]+ task" internal/assets/runtime/agents/atdd/*.md
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

### Item 5 — Update the driver dispatch wiring + regression test

**File:** `internal/atdd/runtime/driver/driver.go`

`registerAgentDispatchers` (around line 672) iterates `agents.Names()`
and registers a no-op base dispatcher per agent. No code change
needed there — the iteration is name-agnostic.

**Required new test** (`internal/atdd/runtime/statemachine/run_test.go`):

Add `TestWrapUserTask_AgentResolvesFromAgentParam` (or similar)
that exercises the new `agent: ${agent}` templating path on
RUN_AGENT and asserts the dispatched agent name equals the
`agent:` param value, NOT the `task-name:` value. Test shape:

- Build a small in-memory process with one MID node that passes
  both `task-name: write-acceptance-tests` and `agent:
  acceptance-test-writer` into `execute-agent`.
- Record the dispatched-agent name via a recording AgentFn
  registered on the engine.
- Assert the recorded name is `"acceptance-test-writer"`, not
  `"write-acceptance-tests"`.

This pins the new wiring against the same class of regression that
"incident 2026-05-11 in the rehearsal of issue #61" caused
(`run_test.go:13-16` — literal `${change-type}` leaked into a
child scope when `ExpandParams` was skipped). The 1701 split
introduces a second name flowing through the same templating
machinery; without a dedicated test, a future refactor could
silently revert to `${task-name}` and the dispatched agent
would still resolve to *something* — the wrong something.

Companion negative-path test (optional but cheap): assert that if
`agent:` is missing from the call-activity params, the engine
fails fast with a clear error rather than falling back to
`task-name`.

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

**Other tests that hard-code task or agent names.**

Grep at refinement time surfaced **11 files** that reference one or
more of the 17 verb identifiers:

```
internal/atdd/runtime/clauderun/clauderun_test.go
internal/atdd/runtime/driver/driver_test.go
internal/atdd/runtime/actions/bindings_test.go
internal/atdd/runtime/statemachine/transitions_test.go
internal/atdd/runtime/statemachine/run_test.go
internal/atdd/runtime/diagram/diagram_test.go
internal/atdd/runtime/trace/trace_test.go
internal/atdd/runtime/agents/embed_test.go
internal/projectconfig/config_test.go
implement_commands_test.go
process_commands_test.go
```

**Re-grep at execution time** (the list may shift if other plans
land first):

```
grep -rnE '"(write-acceptance-tests|write-contract-tests|implement-dsl|implement-system|implement-system-driver-adapters|implement-external-system-stubs|implement-external-system-driver-adapters|refactor-system|refactor-tests|disable-tests|enable-tests|refine-acceptance-criteria|fix-command-failed|fix-missing-output|fix-scope-diff|fix-unexpected-failing-tests|fix-unexpected-passing-tests)"' --include='*_test.go'
```

**Per-file classification rule** (apply consistently):

For each hit, the assertion either references the **task identity**
or the **agent identity**:

- **Task identity** → keep the verb name. Examples: BPMN node
  lookups, scope-key assertions (`engine.Scope("write-acceptance-tests")`),
  `originating-task-name` traces, process-flow YAML parsing tests,
  trace-output assertions that include `(task: ${task-name})`.
- **Agent identity** → update to the noun name. Examples:
  dispatched-agent recorders, `agents.Names()` membership checks,
  `LoadTuning(name)` calls, `Prompt(name)` calls, claude-CLI shell
  argv assertions.

When ambiguous, the trace output added in Item 2
(`"Run agent ${agent} (task: ${task-name})"`) is the disambiguator:
whichever half of the string the test asserts on indicates which
identity it cares about.

### Item 7 — Regenerate the process diagram

**File:** `docs/process-diagram.md` (regenerated, not hand-edited)

Per CONTRIBUTING.md:191, the canonical regeneration command is:

```
gh optivem process show > docs/process-diagram.md
```

Run it after Items 1, 2, and 3 land so the diagram reflects:
- The new `agent:` param on every MID call-activity.
- The updated `RUN_AGENT.agent` templating (`${agent}` not
  `${task-name}`).
- The new noun-form agent names appearing in node labels / docs.

Verify the resulting diff is consistent with the YAML changes — if
the diagram diff is much larger than expected, that's a signal
that the renderer is encoding something derived from `task-name`
that needs separate attention. Surface as a finding rather than
hand-editing the diagram.

CONTRIBUTING.md:405 says the diagram is regenerated "automatically
whenever the canonical YAML changes." Confirm at execution time
whether that's CI-driven (in which case this item is a no-op once
the PR lands) or operator-driven (in which case run the command
above and commit the regenerated diagram in the same change).

### Item 8 — Update doc references in CONTRIBUTING.md

Grep at refinement time found exactly **two** docs containing
verb-name references:

- `docs/process-diagram.md` — regenerated by Item 7, no manual edit
  needed here.
- `CONTRIBUTING.md` — the only non-regenerated doc with hits.

`docs/atdd/process/*.md`, `docs/atdd/architecture/*.md`, and
`docs/how-it-works.md` returned no hits and are out of scope.

**For CONTRIBUTING.md**, apply the same task-vs-agent classification
rule as Item 6:

- **Task identity** → keep the verb name (BPMN process narrative,
  scope-key examples, `task-name:` field documentation).
- **Agent identity** → update to the noun name (any text describing
  the agent that performs work, e.g. "the write-acceptance-tests
  agent" → "the acceptance-test-writer agent").

**Re-grep at execution time** to catch anything that has landed in
docs since this refinement:

```
grep -rnE '(write-acceptance-tests|write-contract-tests|implement-dsl|implement-system|implement-system-driver-adapters|implement-external-system-stubs|implement-external-system-driver-adapters|refactor-system|refactor-tests|disable-tests|enable-tests|refine-acceptance-criteria|fix-command-failed|fix-missing-output|fix-scope-diff|fix-unexpected-failing-tests|fix-unexpected-passing-tests)' docs/ CONTRIBUTING.md
```

If new hits appear in `docs/atdd/**` or `docs/how-it-works.md`,
treat them the same way (task identity vs. agent identity) — the
classification rule generalises.

### Item 9 — Build + test

```
go build ./...
scripts/test.sh
```

Per repo guidance: never `go test ./...` unbounded on Windows.

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
