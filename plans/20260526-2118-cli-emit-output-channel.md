# Replace prose-YAML outputs with a `gh optivem emit-output` CLI channel

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

This plan replaces the prose channel with a CLI command:
`gh optivem emit-output KEY=VAL [KEY=VAL...]`. The agent invokes the
command (via its `Bash` tool); the command writes to a known file in
the current run directory; the dispatcher reads the file after the
agent exits. Prose parsing is removed.

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

## Resolution

### Channel: `gh optivem emit-output`

A new gh-optivem subcommand:

```
gh optivem emit-output KEY=VALUE [KEY=VALUE...]
gh optivem emit-output --file outputs.yaml          # one-shot bulk emit
gh optivem emit-output test_names=shouldRegisterCustomer,shouldRejectDuplicateCustomer   # commas → list
```

Behaviour:

- Resolves the current run directory via the `GH_OPTIVEM_RUN_DIR`
  environment variable. Missing or empty → non-zero exit with a clear
  message ("emit-output must run inside a gh-optivem agent dispatch").
- Reads `<rundir>/expected-outputs.txt` (one key per line, written by
  the dispatcher before the agent is launched). Any KEY not in that
  list → non-zero exit; the agent sees the error mid-run and can
  correct itself before the dispatch ends.
- Appends/merges the supplied keys into `<rundir>/outputs.yaml`. The
  write is atomic (temp-file + rename) so concurrent calls in a
  single agent don't corrupt the file.
- Repeated calls with the same key are last-write-wins, matching the
  existing fenced-block "last block wins" semantics.

Output key types follow the same coercion table as today
(`knownOutputKeys` in `outputs.go`): scalars become strings, comma-
or YAML-list inputs become `[]string` for keys typed as
`kindStringSlice` (today: `test_names`). Unregistered keys pass
through. The coercion table moves into the new subcommand and is
removed from the parser.

### Dispatcher changes (`driver.go`, `actions/bindings.go`)

- Before `RUN_AGENT`, the driver:
  - Creates `<rundir>/expected-outputs.txt` containing the keys from
    the call-activity's `outputs:` param (one per line).
  - Exports `GH_OPTIVEM_RUN_DIR=<absolute path>` into the `claude`
    subprocess env (both interactive and autonomous modes).
- After `RUN_AGENT`, `validateOutputsAndScopes` reads
  `<rundir>/outputs.yaml` (if present) and flattens it into
  `ctx.State` — replacing the current `clauderun.ParseOutputs(resultText)`
  call. Same downstream presence/scope checks.
- Malformed YAML in `outputs.yaml` → `Outcome.Err` ("agent emitted
  malformed outputs.yaml"); behaviour matches today's malformed-block
  hard-error path.
- `scope_exception` rides the same channel: the agent emits via
  `gh optivem emit-output scope_exception_files=path/a,path/b
  scope_exception_reason="..."`. The flattening that
  `ParseOutputs` does today (envelope → `scope_exception_files` /
  `scope_exception_reason`) is now done by the subcommand at write
  time, not by the parser at read time. The downstream
  `scope_exception_requested` gate reads the same state keys it does
  today; no gate change needed.

### Prompt template changes (3 writing-agent prompts + 1 fix prompt)

Replace the "emit a fenced YAML block" section with explicit CLI
instructions:

```
## Outputs

When your work is complete, run these commands (one per output key)
to record what this ticket exercises:

  gh optivem emit-output test_names=shouldRegisterCustomer,shouldRejectDuplicateCustomer
  gh optivem emit-output dsl-port-changed=false

You MUST call `gh optivem emit-output` for every key listed below
before exiting. A missing call will halt the cycle.

Expected output keys:
  - test_names         (comma-separated list of unqualified test
                        method names this ticket added or modified)
  - dsl-port-changed   (true if you added or modified DSL Port
                        methods, false otherwise)
```

Files to update:

- `internal/assets/runtime/agents/atdd/write-acceptance-tests.md`
- `internal/assets/runtime/agents/atdd/write-contract-tests.md`
- `internal/assets/runtime/agents/atdd/implement-dsl.md`
- `internal/assets/runtime/agents/atdd/fix-missing-output.md` —
  diagnosis section reframes "YAML block missing" failure mode as
  "the agent forgot one or more `gh optivem emit-output` calls."
- `internal/assets/runtime/shared/scope.md` — scope-exception
  emission example switches to the CLI command.

### Files deleted / shrunk

- `internal/atdd/runtime/clauderun/outputs.go` — DELETE. The
  fence-extraction and key-coercion logic moves into the new
  subcommand (where coercion is the single seam between agent input
  and ctx.State).
- `internal/atdd/runtime/clauderun/outputs_test.go` — DELETE; the
  tests cover the parser that no longer exists.
- `clauderun.RunResult.ResultText` — KEEP (still useful for the
  exit-banner result echo in autonomous mode), but the dispatcher no
  longer parses it.
- The dispatcher call to `clauderun.ParseOutputs` in
  `driver.go:889` — DELETE; replaced by the file read in
  `validateOutputsAndScopes`.

### Tests

New unit tests:

- `internal/cli/emit_output_test.go` (or wherever the subcommand
  lands) — covers: missing `GH_OPTIVEM_RUN_DIR`, unknown key
  (not in `expected-outputs.txt`), atomic merge with prior writes,
  comma-list parsing for `kindStringSlice` keys, scope-exception
  envelope.
- `internal/atdd/runtime/actions/bindings_test.go` — extend the
  existing `validateOutputsAndScopes` tests to read `outputs.yaml`
  from a tempdir instead of using `ctx.State`-prepopulated values.
- `internal/atdd/runtime/clauderun/clauderun_test.go` — verify
  `GH_OPTIVEM_RUN_DIR` is exported into both `runInteractive` and
  `runAutonomous` subprocess envs.

## Items

1. **Add `emit-output` subcommand.** New `internal/cli/emit_output.go`
   (or similar) wired into the existing Cobra root. Implements the
   parsing, validation, atomic merge, and exit codes described above.
   Unit-tested in isolation. No driver changes yet.

2. **Plumb `GH_OPTIVEM_RUN_DIR` + `expected-outputs.txt`.** Driver
   writes the expected-keys file under the existing per-run directory
   (the same dir that already holds `001-*.prompt.md`) before
   calling `clauderun.Dispatch`. `clauderun.Run` (both interactive
   and autonomous) exports the env var into the subprocess.

3. **Switch `validateOutputsAndScopes` to read `outputs.yaml`.** The
   action reads `<rundir>/outputs.yaml` (key obtained from the same
   ctx.State entry the dispatcher writes in Item 2) and merges into
   `ctx.State`. Existing presence/scope-check logic is unchanged.
   The malformed-YAML hard-error path is preserved.

4. **Update writing-agent prompts.** Edit the three agent `.md` files
   to instruct `gh optivem emit-output` invocations. Each prompt
   gains an explicit "Expected output keys" list (already implicit
   in the call-activity's `outputs:` param — the prompts repeat it
   so the agent doesn't need to discover it).

5. **Update `fix-missing-output.md`.** Replace YAML-emission slip
   diagnostic language with CLI-emission-slip language. The three
   failure modes (work-landed-emission-slip / work-not-landed /
   work-partial) remain — only the recovery hint changes.

6. **Update `shared/scope.md` scope-exception example.** Switch the
   emission template from fenced YAML to the CLI command. No gate
   change.

7. **Delete the prose parser.** Remove
   `internal/atdd/runtime/clauderun/outputs.go`,
   `internal/atdd/runtime/clauderun/outputs_test.go`, and the
   `ParseOutputs` call in `driver.go`. Migrate the `knownOutputKeys`
   coercion table into the subcommand (Item 1 already references it;
   this item is the cleanup pass).

8. **Verify on a real cycle.** Run `gh optivem implement --issue 69`
   (or an equivalent rehearsal ticket) in **both** interactive and
   autonomous modes. Confirm:
   - Interactive mode now passes outputs validation (it currently
     cannot, per "Why now" above).
   - Autonomous mode still passes.
   - A deliberately-omitted `emit-output` call still trips
     `fix-missing-output` with the correct missing-keys list.

## Out of scope

- Changing what the outputs *mean* (no new keys, no removed keys, no
  type changes). The contract between writing agents and downstream
  consumers stays identical.
- BPMN process-flow changes. `process-flow.yaml` is untouched; only
  the under-the-hood plumbing of how the agent's output reaches
  `ctx.State` changes.
- Generalising `emit-output` beyond ATDD. The subcommand is
  ATDD-specific for now (it depends on `expected-outputs.txt` written
  by the ATDD dispatcher). A future TDD/DDD flow can reuse the same
  subcommand by writing its own `expected-outputs.txt`.
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
