Take an ATDD run failure, root-cause it, decide whether the **BPMN process flow**, the **`gh optivem` commands**, or the **ATDD agents** (or a combination) could be changed to prevent it from recurring, confirm the chosen layer(s) with the user, then hand off to `/create-plan` to draft the prevention plan. No code changes — this command produces a *diagnosis* and a *plan*, not a fix.

Use this after a rehearsal-loop / orchestration run halts (e.g. `TESTS_INFRA_HALT`, a `command-failed` verdict, a scope-diff halt, an agent that produced the wrong artifact). It is for **systemic** prevention — "how do we stop this class of failure" — not for re-running the failed ticket.

## Input

The failure is provided as `$ARGUMENTS`. Two accepted shapes — **a path is strongly preferred**:

- **A path** (preferred): a rehearsal run directory (`.gh-optivem/runs/<ts>/`), its `summary.md` (digest) or `flow.txt` (execution trace), a worktree `.log` file, or a worktree root. The command reads the artifacts itself.
- **Pasted failure text**: the trace/digest block copied from the terminal. Parse it, then follow the `Run digest:` / `Execution flow:` / `Keeping <worktree>` lines it contains back to the on-disk artifacts and read those too.

If `$ARGUMENTS` is empty, ask the user for a path or pasted failure in one line, then proceed.

## Phase 0 — Locate the artifacts

Resolve, from whatever was passed, as many of these as exist (each points to the next):

- **Run digest** — `<run-dir>/summary.md`: the ticket, acceptance criteria, the step + agent summary, and the halt path.
- **Execution flow** — `<run-dir>/flow.txt`: the BPMN trace. The last `FAIL`/`HALT` lines name the exact node and error end event (e.g. `VERIFY_TESTS_PASS -> ... TESTS_INFRA_HALT`), and the `command:` / `stderr tail:` lines give the failing command and its output.
- **Worktree root** — the parent of `.gh-optivem/` (the `Keeping <worktree>` / `Log file:` lines in the loop output point here). This holds the **actual test and system files** the agents produced — the ground truth for root-causing.

Read the digest and the tail of the flow first; they tell you which node halted and why.

## Phase 1 — Root-cause (pin to file:line)

1. From the flow, identify the **halt**: the BPMN node, the error end event, the failing command line, the `verdict=` / `failure-kind=` / `test-outcome=`, and the `stderr tail`.
2. Read the **actual artifacts in the worktree** that the failure points at — the acceptance test file(s), the system files an implementer touched, the suite/channel registration, the contract slice. **Pin the cause to a concrete `file:line`.** Never propose a fix from the trace alone (per the repo's fail-loud rule: surface the true cause).
3. Classify the defect — they get fixed at different layers:
   - **Test-authoring error** — wrong test name, wrong channel scope (`forChannels(...)`), wrong suite, a marker/gate misplaced. Produced by an agent.
   - **Orchestration mis-step** — the flow ran/verified something that *can't* succeed given what was authored (e.g. unrolled a channel the test was never registered for; ran `--test=X` in a suite that doesn't contain X). A BPMN/interpreter defect.
   - **Command mis-classification** — a benign or authoring condition reported as a hard infra halt (e.g. "test exists but not in this channel's suite" surfaced as `TESTS_INFRA_HALT` instead of a skip or an authoring error routed to a fixer).
   - **Genuine system bug / flake** — the system code is actually wrong, or a transient failure. (If it's a real flake, prevention may belong in retry config, not BPMN/commands/agents — say so.)

## Phase 2 — Triage across the three layers

For every candidate fix, map it to **exactly one layer** and one file. A single defect is often preventable at more than one layer (defense in depth) — list each option with its layer and tradeoff (narrow agent fix vs. class-catching BPMN fix vs. command reclassification).

- **BPMN** — `internal/atdd/process/process-flow.yaml` and its interpreter Go under `internal/atdd/process/` (e.g. `channels.go` for channel unrolling). Owns: control flow, gateway branch completeness, which suites/tests/channels get run, channel unrolling, and verdict→halt routing. *Use the `bpmn-logic-audit` agent if the suspected defect is in gateway/data-flow logic.*
- **commands** — the `gh optivem` subcommands (`*_commands.go` in the repo root, e.g. `implement_commands.go`, `compile_commands.go`) and the rehearsal wrappers (`scripts/atdd-rehearsal*.sh`). Owns: what a command *does*, how it classifies an outcome (infra vs test-failure vs authoring), its flags, and its messages.
- **agents** — runtime prompts `internal/atdd/assets/runtime/agents/atdd/*.md` (what the orchestration dispatches — `acceptance-test-writer`, `system-implementer`, the `*-fixer`s, …) and the shared chunks they're concatenated with `internal/atdd/assets/runtime/shared/*.md` (`preamble`, `scope`, `wip-gate-*`, `isolated-marker-*`). Owns: what an agent is *instructed to produce*. (Note: a permanent, intentional instruction — like the `wip-gate-*` chunk, lifted by the orchestrator setting `GH_OPTIVEM_RUN_WIP_TESTS=1` during verify — is **not** a defect; don't propose removing intentional behavior. Verify intent before blaming an agent chunk.)

Before presenting options, sanity-check each proposed fix against the actual file so you don't propose changing behavior that is deliberate.

## Phase 3 — Ask the user (mandatory)

This step is **required** — never skip straight to `/create-plan`. Use `AskUserQuestion` to ask which layer(s) the prevention plan should target, presenting the concrete candidate fixes from Phase 2 grouped by layer:

> "#`<n>` halted at `<node>` because `<root cause @ file:line>`. Which layer(s) should the prevention plan change?"

Make it **multiSelect** — options are the concrete fixes (e.g. *"BPMN: derive verify channels from the test's `forChannels` registration"*, *"command: `gh optivem test run` treats zero-match-by-channel as a skip, not infra-halt"*, *"agent: `acceptance-test-writer` must register the scenario for every channel the orchestration will verify"*). Each option label names the layer + the change; the description carries the tradeoff. The user's selection is the scope of the plan — honor it exactly; don't smuggle back in a layer they deselected.

## Phase 4 — Hand off to /create-plan

Invoke the **`create-plan`** skill (via the Skill tool) with a synthesized idea string built from the diagnosis and the user's selected fixes. Give it enough grounding that its draft is concrete:

- the failing ticket + the halt (node + error end event),
- the root cause pinned to `file:line`,
- the selected layer(s) and the specific change(s) for each, with the file each change lives in,
- the prevention goal ("so a future run never …").

`/create-plan` owns the plan file (it writes `plans/YYYYMMDD-HHMM-<slug>.md` in the gh-optivem repo) and its own confirm-before-commit gate. This command stops once the plan is drafted — report the plan path and let the user refine/commit via the plan commands.

## Rules

- **No code changes.** Output is a diagnosis + a plan. If a fix is obvious, capture it as a plan step — don't implement it here.
- **Pin the root cause to `file:line` in the worktree before proposing anything.** Never propose from the trace alone.
- **One fix → one layer → one file.** If a fix spans layers, split it into separate options so the user can pick per-layer.
- **Don't propose removing intentional behavior.** Confirm a suspect instruction is a defect (not a deliberate, orchestrator-lifted gate or a documented convention) before blaming an agent/command/BPMN node.
- **The Phase 3 question is mandatory.** The whole point is to let the user choose whether BPMN, commands, and/or agents change — never auto-decide and skip to the plan.
- **Honor the selection.** The plan handed to `/create-plan` contains only the layers the user picked.

## Worked example (#76 — order-cancellation blackout)

A rehearsal halted at `VERIFY_TESTS_PASS → TESTS_INFRA_HALT` on the **UI** channel:
`gh optivem test run --suite=acceptance-ui --test=shouldNotCancelOrderAt2245OnDec31st` → *"requested test never executed — not found in any selected suite."*

Root cause (`system-test/typescript/tests/.../cancel-order-negative-test.spec.ts:47`): the new scenario is registered `forChannels(ChannelType.API)` — **API-only**, matching the cancel-negative convention (error-message assertions are API-channel). But the orchestration **unrolled the UI channel independently**: it dispatched a 12-minute UI `system-implementer` *and* ran the `--test=…` filter against the UI suite, where the test cannot exist. The WIP gate on the test is **not** a cause — it is the intentional `wip-gate-typescript` chunk, lifted during verify.

Candidate fixes by layer (Phase 3 options):
- **BPMN** (`internal/atdd/process/channels.go` / `process-flow.yaml`) — derive the channels to implement-and-verify from each new test's actual `forChannels(...)` registration, so an API-only scenario never unrolls a UI implementer or UI verify. *(Catches the whole class; also kills the wasted 12-min UI implement.)*
- **command** (`gh optivem test run`, `*_commands.go`) — when `--test=X` matches zero tests in the selected suite **because X is registered for a different channel**, treat it as a clean skip / authoring signal, not `TESTS_INFRA_HALT`.
- **agent** (`acceptance-test-writer.md`) — require the scenario's `forChannels(...)` scope to cover every channel the orchestration will verify (or make channel scope explicit so the orchestration can read it). *(Narrowest; doesn't fix the wasted UI implement.)*
