# Replace prose-YAML outputs with a `gh optivem output write` CLI channel + BPMN outputs SSoT

## Origin / intent

Today, ATDD agents emit structured outputs (`outputs:`, `scope_exception:`)
as a fenced YAML block at the tail of their final Markdown response.
The dispatcher parses that block via regex
(`internal/atdd/runtime/clauderun/outputs.go::ParseOutputs`) and flattens
it into `ctx.State` for downstream actions and gates.

This channel has failed repeatedly in practice. The most recent failure:
a `write-acceptance-tests` interactive run on issue #69 emitted
`outputs:` as inline indented Markdown (no triple-backtick fences),
which `extractFencedYAMLBlocks` cannot see — the validator then halted
the cycle with `missing-output: dsl-port-changed` even though the agent
had done the work and emitted the value (just not in the format the
parser requires).

**Underneath the prose-parser bug, a deeper architectural problem:**
output keys live in **three** partially-overlapping sources of truth
today, with no enforcement that they stay in sync:

1. **BPMN `outputs:` param** (per call-activity) — presence-check
   subset (keys that MUST be present, else `fix-missing-output` fires).
2. **Agent prompt "Outputs" section** — what the agent is told to
   emit (the full set; today's `acceptance-test-writer.md` instructs
   the agent to emit `test_names` AND `dsl-port-changed`, but BPMN
   only declares `dsl-port-changed`).
3. **Go `knownOutputKeys` table** in `outputs.go` — type-coercion
   rules for downstream consumers.

This three-way drift means adding a new output key requires touching
up to three places, and the contract is implicit: today's `test_names`
is emitted-but-not-presence-checked, with no first-class way to
declare that intent.

This plan does two things together:

- **Replace the prose channel** with `gh optivem output write KEY=VAL`.
  Agent invokes via its `Bash` tool; CLI writes to a per-invocation
  JSONL file the dispatcher pre-computed; the dispatcher reads after
  the agent exits. Fixes the interactive-mode bug.
- **Make BPMN the single source of truth for output declarations**,
  with per-output metadata (`key`, `type`, `optional`). Kills the
  three-way drift. The Go coercion table is deleted entirely;
  prompt "Expected output keys" sections are auto-injected from
  BPMN via `${expected_outputs}` at render time. Per-output context
  lives as YAML comments next to the declaration, not as a schema
  field. Output keys are normalized to kebab-case alongside this
  change (today's `test_names` / `scope_exception_*` snake-case
  keys move to kebab).

`output write` is the first verb under a noun-grouped `output`
subcommand; a sibling `output read KEY` is a planned follow-up (out
of scope for this plan, but the grouping is chosen with it in mind).

## Why now — the smoking gun

The current prose-YAML channel is **architecturally broken in
interactive mode**, not just unreliable.

`clauderun.RunResult.ResultText` is documented as:

> Populated only in autonomous mode (interactive mode prints directly
> to the operator's TTY and has no envelope to parse, so structured
> output is an autonomous-only channel).
> — `internal/atdd/runtime/clauderun/clauderun.go:348-354`

In the dispatcher path
(`internal/atdd/runtime/driver/driver.go:889`), interactive mode passes
an empty string into `clauderun.ParseOutputs`. Every interactive run of
an agent that declares outputs (`write-acceptance-tests`,
`write-contract-tests`, `implement-dsl`) therefore **always** fails the
post-RUN validation with `missing-output`. The recent #69 failure is
the surface symptom of this — not a one-off formatting slip.

Loosening the parser to accept un-fenced YAML in interactive mode would
require us to capture the agent's TTY transcript, which we don't do
and shouldn't start doing. A file-based channel works in both modes
uniformly because the agent writes the file with a `Bash` tool call
that survives regardless of whether stdout is a JSON envelope or a
TTY.

The BPMN-SSoT side rides along now (rather than as a follow-up
plan) because the CLI channel design forces the question "what's
the contract for output keys?" Building the CLI with a loose
allow-list now and tightening later would require two passes through
the same agent prompts and dispatcher code. Doing both together is
one coherent change, single review.

## Prior thinking

`plans/archived/20260525-1057-bpmn-refactor-design.md` resolved Q13
as "contract blocks live in `process-flow.yaml` as `user_task`
metadata (`scopes:`, `outputs:`); single source of truth for
prompt-prep + post-execute validation." That design was never
executed (the execution plan `20260525-1517-bpmn-refactor-yaml-and-diagrams.md`
was retired). This plan picks up the SSoT direction Q13 settled,
but designs the per-output schema from current first principles
rather than mechanically adopting any specific shape from the
archived discussion.

## Supersedes

- `plans/upcoming/20260520-1945-user-task-output-context-plumbing.md`.
  That plan's Items 1–4 (the prose-YAML parser + dispatcher wiring
  in `outputs.go`) are torn out by this plan. Its Item 8 (manual
  rehearsal) folds into this plan's Item 10. The "Not superseded"
  appendix in that file (added 2026-05-25) predates this plan's
  conception and is no longer accurate.

## Related (NOT superseded)

- `plans/20260526-2156-verify-tests-by-name.md` — already declares
  a dependency on 2118 landing first. That plan covers the
  input-consumption side (`bindings.go::runCommand` flag vocabulary
  swap, `process-flow.yaml` call-activity `params:` edits). This
  plan covers the output-declaration side. The two touch
  `process-flow.yaml` for different reasons; ship in sequence.

## Resolution

### BPMN outputs SSoT — schema

Replace the current `outputs:` CSV string on every `execute-agent`
call-activity with a list of per-output declarations:

```yaml
# Today (string-CSV form):
- id: WAT_RUN
  process: execute-agent
  params:
    task-name: write-acceptance-tests
    agent: acceptance-test-writer
    outputs: "dsl-port-changed"

# After (list-of-objects form):
- id: WAT_RUN
  process: execute-agent
  params:
    task-name: write-acceptance-tests
    agent: acceptance-test-writer
    outputs:
      - key: dsl-port-changed
        type: bool
      # May or may not be emitted depending on whether the agent
      # wrote new tests this iteration
      - key: test-names
        type: string-list
        optional: true
      # Scope-exception envelope — emitted only when the agent had
      # to modify files outside its permitted scope
      - key: scope-exception-files
        type: string-list
        optional: true
      - key: scope-exception-reason
        type: string
        optional: true
```

Field semantics:

- **`key`** — the ctx.State key the value flattens into.
- **`type`** — one of `string`, `bool`, `string-list`. Used by the
  CLI (write-time value coercion) and the dispatcher's reader
  (read-time type guarantee for downstream consumers).
- **`optional`** — `true` means absence is allowed; dispatcher does
  not fire `missing-output-diagnoser` for this key. Default is
  `false` (required) when omitted.

Per-output context that doesn't fit in `key` lives as a YAML comment
above the entry — informal, doesn't pollute the schema.

### Key naming convention

All output keys use **kebab-case** (matching BPMN call-activity
params: `task-name`, `expected-test-result`, `originating-task-name`).
Today's snake_case keys (`test_names`, `scope_exception_files`,
`scope_exception_reason`) are renamed to kebab as part of this
plan — see Item 2 for the rename list and downstream-consumer
update points. The `*-changed` keys are already kebab.

### Channel: `gh optivem output write`

A new gh-optivem subcommand grouped under a parent `output` noun
(siblings such as `output read KEY` are planned but out of scope here):

```
gh optivem output write KEY=VALUE [KEY=VALUE...]
gh optivem output write test-names=shouldRegisterCustomer,shouldRejectDuplicateCustomer   # commas → list
gh optivem output write test-names=foo,bar dsl-port-changed=false   # multi-key → one JSONL line
```

Behaviour:

- Resolves the target file via the `GH_OPTIVEM_OUTPUT_FILE` env var
  (absolute path; see "Dispatcher changes" for how it's composed).
  Missing or empty → non-zero exit with a clear message
  ("output write must run inside a gh-optivem agent dispatch").
- Reads the allow-list + per-key types from `GH_OPTIVEM_OUTPUT_KEYS`
  (set by the dispatcher from the BPMN `outputs:` list). Shape:
  `key1:type1,key2:type2,...`. Any KEY not in that list → non-zero
  exit ("unknown output key 'foo'; declared keys: test-names,
  dsl-port-changed"). A value that can't be coerced to the declared
  type → non-zero exit ("output key 'dsl-port-changed' expects bool,
  got 'notabool'"). The agent sees the error mid-run and can
  correct itself before the dispatch ends.
- Appends one JSON object per call to the file (one line per call,
  JSONL format). No read-modify-write; the append is naturally
  concurrency-safe via POSIX `O_APPEND` semantics for line-sized
  writes, eliminating the temp-file+rename complexity the YAML
  approach would have required.
- Repeated calls with the same key are last-write-wins at *read* time
  (the dispatcher's reader walks the file in order and keeps the last
  value seen per key), matching the existing fenced-block "last block
  wins" semantics.

### Dispatcher changes (`driver.go`, `actions/bindings.go`)

- Before `RUN_AGENT`, the driver composes the per-invocation output
  path from the same pieces it already uses for the prompt log
  (`driver.go::promptLogPath`, `<repoPath>/.gh-optivem/runs/<run-ts>/<seq>-<agent>.prompt.md`)
  and exports two env vars into the `claude` subprocess (both
  interactive and autonomous modes):
  - `GH_OPTIVEM_OUTPUT_FILE=<repoPath>/.gh-optivem/runs/<run-ts>/<seq>-<agent>.outputs.jsonl`
  - `GH_OPTIVEM_OUTPUT_KEYS=key1:type1,key2:type2,...` (derived
    from the BPMN call-activity's `outputs:` list — `key` and
    `type` fields only; `optional` is dispatcher-side only)
- The output file is **not** pre-created. If the agent makes no
  `output write` calls, the file simply doesn't exist after the run
  (a missing file is treated identically to an empty file by the
  reader).
- After `RUN_AGENT`, `validateOutputsAndScopes`:
  1. Reads the file path from `ctx.State[output_file_path]` (stashed
     at export time), walks the JSONL lines applying last-write-wins
     per key, and flattens the result into `ctx.State`.
  2. Runs presence-check against every non-`optional` key in the
     BPMN `outputs:` list. Missing → triggers
     `missing-output-diagnoser` as today.
  3. The `scope-exception-files` / `scope-exception-reason` keys
     ride the same channel as any other optional output. The
     downstream `scope_exception_requested` gate reads them from
     `ctx.State` as today (its read-side key names update with the
     kebab rename — see Item 9).
- A malformed JSONL line → `Outcome.Err` ("agent emitted malformed
  output line"); behaviour matches today's malformed-block hard-error
  path.

### Prompt template changes (auto-injection)

The prompt template gains a `${expected_outputs}` placeholder,
rendered at dispatch time from the BPMN `outputs:` list. Format
(minimal contract table; no narrative prose):

```
Required outputs:
  dsl-port-changed: bool

Optional outputs:
  test-names: string-list
  scope-exception-files: string-list
  scope-exception-reason: string

Emit: gh optivem output write KEY=VAL [KEY=VAL...]
```

When all outputs are required, the "Optional outputs:" block is
omitted (and vice versa). The renderer reads the BPMN `outputs:`
list, splits by `optional`, formats each entry as `key: type`, and
substitutes the result. Prompt authors never write this section by
hand — it's derived. This kills the prompt/BPMN drift permanently.

Files to update (remove the hand-written "Outputs" section and
ensure `${expected_outputs}` appears at the right spot):

- `internal/assets/runtime/agents/atdd/acceptance-test-writer.md`
- `internal/assets/runtime/agents/atdd/contract-test-writer.md`
- `internal/assets/runtime/agents/atdd/dsl-implementer.md`
- `internal/assets/runtime/agents/atdd/missing-output-diagnoser.md` —
  diagnosis section reframes "YAML block missing" failure mode as
  "the agent forgot one or more `gh optivem output write` calls."
  This prompt also gets `${expected_outputs}` if the diagnoser
  declares outputs (it doesn't today).
- `internal/assets/runtime/shared/scope.md` — scope-exception
  emission example switches to the CLI command. The
  `scope-exception-*` keys are now first-class declared in BPMN
  (kebab-case), so the prose references them by their new names.

### Files deleted / shrunk

- `internal/atdd/runtime/clauderun/outputs.go` — DELETE entirely.
  Fence-extraction, `knownOutputKeys`, `coerceKnownKey` all go
  away; types now live in BPMN as the SSoT.
- `internal/atdd/runtime/clauderun/outputs_test.go` — DELETE; the
  tests cover the parser that no longer exists.
- `clauderun.RunResult.ResultText` — KEEP (still useful for the
  exit-banner result echo in autonomous mode), but the dispatcher no
  longer parses it.
- The dispatcher call to `clauderun.ParseOutputs` in
  `driver.go:889` — DELETE; replaced by the JSONL read in
  `validateOutputsAndScopes`.

### Tests

New / extended unit tests:

- **BPMN parser tests** (`internal/atdd/runtime/statemachine/load_test.go`
  or wherever the parser is tested) — accept the new list-of-objects
  `outputs:` shape; emit clear error on legacy string-CSV form
  ("BPMN `outputs:` must be a list of {key, type, ...} objects;
  string form deprecated"); validate each entry has `key` + `type`;
  default `optional: false`.
- **`output_commands_test.go`** (repo root, matching the
  `*_commands_test.go` convention) — covers: missing
  `GH_OPTIVEM_OUTPUT_FILE`, missing `GH_OPTIVEM_OUTPUT_KEYS`,
  unknown key, value-type-mismatch coercion failures per declared
  type, multi-key single-call JSONL line shape, append semantics
  with prior writes, scope-exception emission.
- **`internal/atdd/runtime/actions/bindings_test.go`** — extend the
  existing `validateOutputsAndScopes` tests to read a JSONL file
  from a tempdir; cover the four edge cases from Item 5 (absent
  path, missing file, empty/whitespace lines, malformed line);
  cover `optional` vs required presence-check behavior using
  BPMN-derived metadata; verify the `${expected_outputs}` injection
  groups keys into Required / Optional sections correctly.
- **`internal/atdd/runtime/clauderun/clauderun_test.go`** — verify
  both `GH_OPTIVEM_OUTPUT_FILE` and `GH_OPTIVEM_OUTPUT_KEYS` are
  exported into both `runInteractive` and `runAutonomous`
  subprocess envs; verify `GH_OPTIVEM_OUTPUT_KEYS` shape is
  `key:type,...` not just `key,...`.

## Items

1. **Extend BPMN `outputs:` schema (parser + validator).** Today
   the raw YAML struct in `internal/atdd/runtime/statemachine/load.go:55`
   declares `Outputs []string` and the in-memory type at
   `internal/atdd/runtime/statemachine/types.go:80` mirrors it.
   Replace both with a richer `[]OutputSpec` shape:

   ```go
   type OutputSpec struct {
       Key      string `yaml:"key"`
       Type     string `yaml:"type"`     // "string" | "bool" | "string-list"
       Optional bool   `yaml:"optional,omitempty"`
   }
   ```

   Type must be one of `string`, `bool`, `string-list` (extensible
   later — add a closed enum + validation). `optional` defaults to
   `false` (i.e. required) when omitted. Required fields are `key`
   and `type` only. The legacy string-CSV form is **rejected with a
   clear error** at parse time (yaml.v3's natural "cannot unmarshal
   !!str into []OutputSpec" wrapped with context) so no silent
   backward-compat path exists. Unit-tested in isolation. No
   call-site edits yet — those land in Item 2.

2. **Migrate existing call-activity outputs declarations + key
   rename to kebab.** Rewrite every `outputs:` in
   `process-flow.yaml` from string-CSV to the new list-of-objects
   shape, AND rename snake_case keys to kebab-case in the same
   pass. Concretely:
   - `acceptance-test-writer` (WAT_RUN, ~line 1172): declare
     `dsl-port-changed` (bool), `test-names` (string-list,
     optional), `scope-exception-files` (string-list, optional),
     `scope-exception-reason` (string, optional).
   - `contract-test-writer` (WCT_RUN, ~line 1195): same key set as
     acceptance.
   - `dsl-implementer` (IMPL_DSL_RUN, ~line 1217): declare
     `system-driver-ports-changed` (bool),
     `external-driver-ports-changed` (bool), `scope-exception-files`
     (string-list, optional), `scope-exception-reason` (string,
     optional).
   - Any other `execute-agent` call-activities found during the
     edit (the search pass should be exhaustive across
     `process-flow.yaml`).

   Snake→kebab rename list to apply globally in this item:
   - `test_names` → `test-names`
   - `scope_exception_files` → `scope-exception-files`
   - `scope_exception_reason` → `scope-exception-reason`

   Downstream consumer updates (read sites that cast
   `ctx.State["test_names"]` etc.) are handled in Item 9's sweep.

3. **Add `output write` subcommand.** New `output_commands.go` at
   repo root (matching the `*_commands.go` convention) defining a
   parent `outputCmd` and a `write` child. Implements env-var
   resolution (`GH_OPTIVEM_OUTPUT_FILE`, `GH_OPTIVEM_OUTPUT_KEYS`
   in `key:type,...` shape), key-against-allow-list validation,
   per-key value-type coercion (string / bool / string-list with
   comma-split), JSONL append, and generic non-zero exits with
   clear stderr messages (no distinct exit codes — no consumer
   needs the distinction). JSONL line shape: one
   `{"key": value, ...}` object per call (a single invocation with
   multiple `KEY=VAL` args writes one combined line, preserving
   the agent's emit-intent). Coercion logic is **self-contained**
   in this file — types come from the env var, not from a shared
   Go table. The legacy `knownOutputKeys` table in `outputs.go`
   stays untouched until Item 8 deletes the whole parser; there is
   no shared dependency between the two during the transition.
   Unit-tested in isolation. No driver changes yet.

4. **Plumb `GH_OPTIVEM_OUTPUT_FILE` + `GH_OPTIVEM_OUTPUT_KEYS`.**
   Driver gains a new helper `outputFilePath(repoPath, runTS, seq,
   agentName)` that mirrors `promptLogPath` (same composition
   rules, `.outputs.jsonl` extension instead of `.prompt.md`) and
   exports both env vars into the `claude` subprocess.
   `GH_OPTIVEM_OUTPUT_KEYS` is derived from the BPMN `outputs:`
   list — `key` and `type` fields joined as `key:type,...`.
   `clauderun.Run` (both interactive and autonomous) passes them
   through. The file is not pre-created; the path is also stashed
   on `ctx.State` under the key `output_file_path` for Item 5 to
   read. When a call-activity has no `outputs:` declared (or an
   empty list), `GH_OPTIVEM_OUTPUT_KEYS` is **unset** (not empty
   string) so the CLI can distinguish "no allow-list configured"
   from "empty allow-list"; in that case `output write` fails with
   "no outputs declared for this agent" and the path-stash step is
   also skipped.

5. **Switch `validateOutputsAndScopes` to read the JSONL file +
   use BPMN-derived presence rules.** The action reads the file
   path from `ctx.State[output_file_path]`, walks the JSONL lines
   applying last-write-wins per key, and merges into `ctx.State`.
   Presence-check now uses the BPMN `outputs:` list directly:
   every non-`optional` key must be present; everything else is
   allowed to be absent. Edge cases:
   - **Absent `output_file_path`** (Item 4 skipped the stash
     because the agent had no `outputs:` declared) → no-op.
   - **Missing file** (path stashed but agent emitted nothing) →
     empty result map, no error. Required-key presence-check still
     fires for missing keys.
   - **Empty/whitespace-only lines** → tolerated (skipped).
   - **Malformed line** (not valid JSON, or valid JSON but not an
     object after trimming whitespace) → `Outcome.Err` ("agent
     emitted malformed output line").

6. **Add `${expected_outputs}` prompt-template injection.** The
   prompt renderer (`clauderun.renderPrompt` or its successor)
   gains a new substitution that reads the call-activity's BPMN
   `outputs:` list and emits the minimal Required / Optional
   contract table shown in "Prompt template changes" above. Pure
   rendering logic; unit-tested with fixtures covering: empty
   outputs list (placeholder expands to empty string), all required,
   all optional, mix of required and optional.

7. **Update writing-agent prompts.** Edit the three writing-agent
   `.md` files to replace any hand-written "Outputs" section with
   the `${expected_outputs}` placeholder:
   - `internal/assets/runtime/agents/atdd/acceptance-test-writer.md`
   - `internal/assets/runtime/agents/atdd/contract-test-writer.md`
   - `internal/assets/runtime/agents/atdd/dsl-implementer.md`

   Plus the diagnoser:
   - `internal/assets/runtime/agents/atdd/missing-output-diagnoser.md` —
     diagnosis section reframes "YAML block missing" failure mode
     as "the agent forgot one or more `gh optivem output write`
     calls." Recovery hint rewrite covers both the diagnostic
     narrative and any embedded example commands.

   Plus the shared scope doc:
   - `internal/assets/runtime/shared/scope.md` — scope-exception
     emission example switches to the CLI command, with the kebab
     key names (`scope-exception-files`, `scope-exception-reason`).

8. **Delete the prose parser + coercion table.** Remove
   `internal/atdd/runtime/clauderun/outputs.go`,
   `internal/atdd/runtime/clauderun/outputs_test.go`, and the
   `ParseOutputs` call in `driver.go`. The `knownOutputKeys` table
   goes with the file — types now live in BPMN.

9. **Sweep references to retired vocabulary + snake key names.**
   Two sweeps in one item (single grep pass, single review):
   - **Fenced-YAML + knownOutputKeys references:** doc-comments,
     examples, test fixtures referencing the old `outputs:`
     fenced-YAML shape or the deleted Go table. Targets include
     `internal/assets/runtime/shared/preamble.md`,
     `validateOutputsAndScopes` comments, BPMN doc-comments
     referencing the CSV form.
   - **Snake key names:** every read site casting
     `ctx.State["test_names"]`, `ctx.State["scope_exception_files"]`,
     `ctx.State["scope_exception_reason"]` updates to kebab.
     Known sites: `runTargetedTests`, `verifyRealSuitePasses`,
     `scopeExceptionRequested` gate (`internal/atdd/runtime/gates/bindings.go`),
     and any test fixtures that pre-populate `ctx.State` with the
     snake names.

10. **Verify on a real cycle.** Run `gh optivem implement --issue 69`
    (or an equivalent rehearsal ticket) in **both** interactive and
    autonomous modes. Confirm:
    - Interactive mode now passes outputs validation (it currently
      cannot, per "Why now" above).
    - Autonomous mode still passes.
    - A deliberately-omitted required-key `output write` call still
      trips `missing-output-diagnoser` with the correct missing-keys
      list.
    - An `output write` call with a typo'd key name fails the
      agent mid-run with a clear error (typo protection — the
      new behavior the strict allow-list buys us).
    - The auto-injected `${expected_outputs}` section in the
      rendered prompt log matches the BPMN declaration.

## Out of scope

- Generalising `output write` beyond ATDD. The subcommand is
  ATDD-specific for now (it depends on the env-var protocol the
  ATDD dispatcher sets). A future TDD/DDD flow can reuse the same
  subcommand by exporting its own `GH_OPTIVEM_OUTPUT_FILE` /
  `GH_OPTIVEM_OUTPUT_KEYS`.
- `gh optivem output read KEY` — symmetric read-back of an emitted
  value. The noun-grouped `output` parent is chosen with this
  sibling in mind, but the read side is not built in this plan.
  When added later it will reuse `GH_OPTIVEM_OUTPUT_FILE` for the
  read source.
- **Input consumption vocabulary.** The downstream side
  (`bindings.go::runCommand` consuming `test-names` to build
  `--test=foo,bar` flags, BPMN call-activity `params:` edits to
  thread `${test-names}` through) is handled by
  `plans/20260526-2156-verify-tests-by-name.md`, which already
  declares its dependency on this plan. The two ship in sequence.
  **Coordination note:** 2156's body currently references
  `test_names` (snake) throughout — that plan needs a refine pass
  to update to `test-names` (kebab) before execution, since this
  plan renames the key. Flag during 2156's next /refine-plan run
  (it was picked up at 2026-05-26T20:02Z, so the refiner is
  active).
- **Symmetric `read-input` CLI for agent inputs.** Inputs are already
  delivered reliably via `${placeholder}` substitution in the
  pre-rendered prompt (`clauderun.renderPrompt`). By the time the
  agent's process starts, every input — static (`${scope_block}`,
  `${acceptance_criteria}`) and dispatcher-computed
  (`${failing-task-name}`, `${changed_files}`) — is already
  prose-baked into the prompt file. There is no reliability problem
  on the input side to solve, and adding a parallel CLI channel would
  split inputs across two SSOTs (the prompt log and live CLI calls),
  weakening prompt-log-based replay. The asymmetry between inputs
  (push from dispatcher, known pre-dispatch, prose-substituted) and
  outputs (push from agent, known mid-dispatch, file-channel) is
  intentional and matches the data-flow direction.

## Open questions

None — every design decision is settled above. The plan is ready
for execution.
