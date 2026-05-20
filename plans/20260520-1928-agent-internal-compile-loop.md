# Agent-internal compile loop (BPMN compile becomes verification backstop)

**Cross-references**:

- Sibling in-flight plans (no overlap, but touch the same surface area):
  - [`20260520-1825-process-flow-remarks.md`](20260520-1825-process-flow-remarks.md) —
    edits `process-flow.yaml` top-level edges; does not touch the
    `compile` sub-process or the WRITE agents.
  - [`20260520-1834-strip-init-paths-default-scaffolding.md`](20260520-1834-strip-init-paths-default-scaffolding.md)
    and [`20260520-1900-nest-paths-under-system-test.md`](20260520-1900-nest-paths-under-system-test.md) —
    edit `projectconfig`/`optivem_yaml.go`, not the runtime or agent prompts.
- Doctrine precedent: the `process-flow.yaml:10–12` header comment
  already declares that "the inner compile/run/disable loop of
  AT - RED - TEST is *not* encoded here … lives in the per-phase docs
  (at-red-test.md, at-red-dsl.md, …) and stays agent-internal." This
  plan re-aligns reality to that declared intent — it is **not** a
  new direction.

## Context

Today, every WRITE agent dispatches in two phases:

1. The agent writes (Edit/Write) and signals done. It does **not** run
   compile itself. The current `at-red-test.md` prompt only mentions
   compile retroactively: *"If your previous WRITE didn't compile,
   instead fix the broken/missing piece in your prior edits"*
   (`internal/assets/runtime/prompts/atdd/at-red-test.md:19`).
2. The orchestrator's `red_phase_cycle` / `green_phase_cycle` /
   `structural_cycle` runs the `compile` sub-process
   (`process-flow.yaml:1188–1222`) as a service_task. On failure it
   parks at `STOP_COMPILE_FAIL_REVIEW`, waits for human approval, then
   re-dispatches the same WRITE agent with `failure_type=compile`.

The re-dispatch is the cost. It pays:

- a ~1–2 minute human-approval round-trip on every compile failure;
- a cold-context re-dispatch of the WRITE agent — it has to re-read
  the files it just edited (observed in the 20260520-1914 trace:
  9+ files re-read after the gate);
- agent prompt + scope re-preparation (~4.7 KB per dispatch in the
  same trace).

These costs are paid for what is almost always the agent's own typo
or signature drift — i.e. work the agent could verify before
signalling done.

The header comment already says compile/run/disable is "agent-internal."
The YAML drifted toward orchestrator-owned compile. Pulling it back
matches the spec.

## The proposed change (two layers)

Two compile points, with different jobs:

- **Agent-internal compile** (new) — part of the WRITE loop. The agent
  runs the phase-specific `gh optivem … compile` invocation
  repeatedly while iterating, and only signals done when it's
  green. This is the *production* check — it stops the agent from
  claiming finished on broken code.
- **BPMN `COMPILE` service_task** (unchanged) — independent
  verification after the agent returns. This is the *audit* check —
  confirms the agent's self-report is true, catches env/path drift,
  and remains the only place a human gate fires
  (`STOP_COMPILE_FAIL_REVIEW`).

In the happy path the BPMN compile becomes a silent ✔ trace line —
no re-dispatch, no human approval. The expensive ceremony only fires
when the agent's compile and the orchestrator's compile actually
disagree, which should be rare and is exactly the situation that
warrants human attention (the agent lied, or the orchestrator's
environment diverges from the agent's).

## Per-agent compile command mapping

The mapping is **static per phase** — it already lives at the
call sites in `process-flow.yaml` as the `${compile_action}`
template param. Pushing it into the agent does not introduce a
runtime decision; it shifts the same hardcoded line from the YAML's
`params:` block into the per-phase agent prompt.

Source of truth (`internal/atdd/runtime/actions/bindings.go:758–768`):

| `compile_action`         | Shell command                | Phases (`process-flow.yaml`)                                |
| ------------------------ | ---------------------------- | ----------------------------------------------------------- |
| `compile_system_tests`   | `gh optivem test compile`    | `AT_RED_TEST`, `CT_RED_TEST`                                |
| `compile_system`         | `gh optivem system compile`  | `AT_RED_DSL`, `AT_RED_SYSTEM_DRIVER`, all GREEN phases, `CT_RED_DSL`, `CT_RED_EXTERNAL_SYSTEM_DRIVER`, `CT_GREEN_EXTERNAL_SYSTEM_STUB`, `AT_REFACTOR_SYSTEM` |
| `compile_all`            | `gh optivem compile`         | `structural_cycle` (only)                                   |

Per-agent → command resolution (matches the WRITE agent for each
phase block in `process-flow.yaml`):

- `at-red-test`, `ct-red-test` → `gh optivem test compile`
- `at-red-dsl`, `at-red-system-driver`, `at-green-system`,
  `at-refactor-system`, `ct-red-dsl`,
  `ct-red-external-system-driver`,
  `ct-green-external-system-stub` → `gh optivem system compile`
- `fix-verify` (structural cycle's WRITE agent) → `gh optivem compile`

Legacy agents (`legacy-at-test`, `legacy-at-dsl`, … —
`internal/assets/runtime/prompts/atdd/legacy-*.md`) follow the same
rules as their non-legacy counterparts.

## Files touched

### Agent prompts (one new "self-verify compile" step per WRITE agent)

Each prompt gets a new step *immediately before signalling done* that
runs the phase-specific compile in a loop until it passes. The
existing line 19 of `at-red-test.md` — "If your previous WRITE didn't
compile, instead fix the broken/missing piece" — becomes redundant
and is removed (the loop in the new step handles it).

- `internal/assets/runtime/prompts/atdd/at-red-test.md`
- `internal/assets/runtime/prompts/atdd/at-red-dsl.md`
- `internal/assets/runtime/prompts/atdd/at-red-system-driver.md`
- `internal/assets/runtime/prompts/atdd/at-green-system.md`
- `internal/assets/runtime/prompts/atdd/at-refactor-system.md`
- `internal/assets/runtime/prompts/atdd/ct-red-test.md`
- `internal/assets/runtime/prompts/atdd/ct-red-dsl.md`
- `internal/assets/runtime/prompts/atdd/ct-red-external-system-driver.md`
- `internal/assets/runtime/prompts/atdd/ct-green-external-system-stub.md`
- `internal/assets/runtime/prompts/atdd/fix-verify.md`
- `internal/assets/runtime/prompts/atdd/legacy-at-test.md`
- `internal/assets/runtime/prompts/atdd/legacy-at-dsl.md`
- `internal/assets/runtime/prompts/atdd/legacy-at-system-driver.md`
- `internal/assets/runtime/prompts/atdd/legacy-ct-test.md`
- `internal/assets/runtime/prompts/atdd/legacy-ct-dsl.md`
- `internal/assets/runtime/prompts/atdd/legacy-ct-external-system-driver.md`
- `internal/assets/runtime/prompts/atdd/legacy-ct-external-system-stub.md`

Open question: see Q1 below on templating vs hardcoding the command.

### BPMN (rewording only — no structural change)

`internal/atdd/runtime/statemachine/process-flow.yaml`:

- `STOP_COMPILE_FAIL_REVIEW.documentation` (line 1205): the message
  "STOP - HUMAN REVIEW — compile failed, dispatch ${fix_agent}?" now
  describes a *divergence* between agent-self-compile and BPMN
  verification compile, not a first-time compile failure. Reword to
  reflect that — see Q2 below for exact wording.
- Optional: tighten the header comment block at lines 10–12 to say
  compile is now *actually* agent-internal (the comment is correct
  today; this just removes the "drifted" subtext).

No structural change. No edges removed. No new nodes. The `compile`
sub-process stays as-is, including the `FIX_COMPILE` re-dispatch path
— it remains the divergence-recovery escape hatch.

### Tests

- `internal/atdd/runtime/statemachine/structural_cycle_test.go`,
  `transitions_test.go`, `dispatch_spy_test.go`,
  `run_test.go` — should be unaffected by the prompt change. Re-run
  to confirm no statemachine fixtures depend on the
  `STOP_COMPILE_FAIL_REVIEW` documentation string. Watch for
  loopback hazards per [[feedback_statemachine_test_loop_hazard]].
- `internal/atdd/runtime/actions/bindings_test.go:141` — covers
  `gh optivem compile`; no behavioural change to actions, should pass
  unchanged.

### Diagram regeneration

If the `STOP_COMPILE_FAIL_REVIEW` documentation string changes,
regenerate `docs/process-diagram.md` and the affected SVGs in
`docs/images/` (the compile sub-process diagram is
`process-diagram-15-compile.svg`).

## Open questions

### Q1. How does the agent prompt know which `gh optivem … compile` command to run?

The per-phase compile target is **already pinned** at every `compile`
call site in `process-flow.yaml` as the `compile_action:` template
param — e.g. `AT_RED_TEST` declares `compile_action: compile_system_tests`
(line 401), the shared green wrapper declares `compile_action:
${compile_action}` forwarded from its caller (line 1123), and
`structural_cycle` declares `compile_action: compile_all` (line 800).
The `compile_action → shell command` mapping lives in
`internal/atdd/runtime/actions/bindings.go:758–768`.

So the agent doesn't need a *new* config to know which command to
run — the orchestrator already knows. The question is only how the
prompt-prep layer surfaces it to the agent.

**Option A — hardcode in each prompt file**: every prompt names its
own command in plain text. Duplicates the mapping across 17 prompt
files; renaming `compile_system` (or its shell command) requires
touching every prompt. Wrong axis — compile target is a function of
the phase, not of the agent.

**Option B — new `${compile_command}` Family B placeholder**: add
yet another placeholder to `gh-optivem.yaml`, resolved per-phase
from a parallel table. Introduces a second source of truth alongside
`compile_action` that has to agree with it. Violates
[[feedback_paths_deterministic_no_guessing]] (parallel config that
can drift).

**Option C — reuse the existing `compile_action` call-site param ⇒
recommended**. The dispatcher resolves the prompt template at
dispatch time, when it already has the `compile_action` value from
`process-flow.yaml`. Substitute it (or the resolved shell command
from the `bindings.go` table) into the prompt as `${compile_command}`
*at prompt-prep time*, not as a YAML-level Family B placeholder. One
source of truth (the call-site param), no drift, no duplication, one
place to change if a compile action is renamed. Each WRITE prompt
then ends with a uniform line like "Run `${compile_command}` until it
passes." while the actual command varies by phase.

This drops the "follow-up plan for more targeted compilation"
framing entirely. *Tier*-level targeting is already specified — this
plan just plumbs the existing per-phase value through to the agent.
*Sub-tier* targeting (per-package compile via `tsc --build
tsconfig.x.json`, `dotnet build proj.csproj`, etc.) would need new
compile commands (`gh optivem compile --layer dsl_port` or
equivalent) and is genuinely future work — but not blocking v1 and
not part of this plan.

### Q2. Reword `STOP_COMPILE_FAIL_REVIEW` for the divergence case?

Today the message reads:
> STOP - HUMAN REVIEW — compile failed, dispatch ${fix_agent}?

In the post-change world, this gate only fires when the BPMN
compile fails *after* the WRITE agent claimed compile passed. That
is a different situation: the agent and orchestrator disagree.
Re-dispatching the same agent is the right default *if* the agent
gave up and signalled done with broken code; investigating the
environment is the right default *if* the agent's compile genuinely
passed.

Proposed wording (one line, keeps the same `[y/n]` dispatch shape):
> STOP - HUMAN REVIEW — BPMN compile failed after ${agent} signalled done; re-dispatch ${fix_agent}? (n → investigate divergence)

Confirm wording before editing.

### Q3. Where should the agent's compile output go?

The agent's compile output (`tsc` errors, dotnet build diagnostics,
…) currently *would* land in the agent's tool log only, which is
ephemeral. The orchestrator captures nothing.

Options:
- **Do nothing** — agent's output is ephemeral; trust the BPMN
  verification compile to surface failures.
- **Tee to a run log** — agent writes its compile output to
  `.gh-optivem/runs/<run>/NNN-<agent>.compile.log` so post-mortem
  debugging is possible. Mirrors the existing
  `NNN-<agent>.prompt.md` convention seen in the 20260520 trace.

Recommendation: **do nothing for v1**. Add the run log if a
divergence case actually shows up in practice. Cheap to add later.

### Q4. What about agents that don't WRITE code?

`disable-tests`, `enable-tests`, `update-ticket`, `refine-acc`,
`task-*` (task-* are read-only design agents). None of these write
compilable code. Confirm by reading each prompt — current list
suggests they don't need the new step.

`fix-verify` is the structural cycle's WRITE agent — included
above. The `task-*` agents are read-only and excluded.

## Sequencing

Single PR. The prompt edits are mechanical and independent per
agent; the BPMN reword is one line. Tests should pass unchanged.

1. Resolve Q1 and Q2.
2. Edit one prompt as a pilot (`at-red-test.md` — the prompt that
   triggered this plan via the 20260520-1914 trace). Verify by
   running an AT - RED - TEST dispatch end-to-end and confirming
   the BPMN compile is now a silent ✔ in the happy path.
3. Roll out to the remaining WRITE agents in one commit.
4. Reword `STOP_COMPILE_FAIL_REVIEW.documentation` per Q2; regen
   diagrams.

## Risks and notes

- **Agent context blow-up on noisy compile output**: TypeScript
  compile output for a half-finished test suite can be 50+ errors
  pointing at the same root cause. Agent prompts should advise:
  "fix root-cause errors first; do not transcribe every error into
  edits." Mirror the existing "make multiple edits in one
  Write/Edit call" guidance in `at-red-test.md:21`.
- **Trust transition**: the trace currently shows compile
  outcomes as orchestrator-owned (`OK COMPILE -> (no result)`).
  After the change, those lines will almost always be `compile_ok=true`
  silently; the interesting signal moves into the agent's tool log.
  Trace readers used to "watch the compile flips" will need to
  adjust — but the divergence case (gate fires) becomes louder, which
  is the right tradeoff.
- **No legacy-alias machinery** per
  [[feedback_teaching_repo_no_legacy]]. This is a teaching repo;
  the change is uniform across all WRITE agent prompts.
- **No `--offline` escape hatches** per
  [[feedback_always_online_no_offline_flags]]. `gh optivem … compile`
  is the agent's only compile path; no alternate command for
  air-gapped runs.
