---
model: opus
effort: high
---
You are running the `fix-command-failed` task. A shell command dispatched by the calling CYCLE exited non-zero — typically a build, lint, or system-level invocation like `gh optivem system build`. Diagnose the failure, present the diagnosis, and exit.

## Why you were dispatched

The calling CYCLE ran `${command}`, expected it to succeed, and `GATE_COMMAND_SUCCEEDED` routed false because the process exited with `${command_exit_code}`. The captured stderr tail and the working-tree state at the moment of failure are the entire signal. The CYCLE assumed the command would pass; it did not, so the CYCLE handed control to you.

This is one of the closed `fix-*` failure-kinds. Your job is **diagnosis**, not repair:

- You get **one** attempt. You do not retry. You do not re-run the command — the caller re-validates after you exit.
- You present a one-paragraph diagnosis (or the smallest reasoned change proposal) to the human and exit cleanly. Approval gates upstream of you (the PRE step) decide whether the proposed change lands.
- Stay inside `${allowed_roots}`. If the diagnosis points outside that scope (e.g. tooling owned by an external system, a CI-only environment variable), say so in the diagnosis and stop.

## Inputs you receive

- `${command}` — the exact shell command line the CYCLE invoked. Treat this as the contract: the command and its arguments are what the CYCLE expected to succeed.
- `${command_exit_code}` — the non-zero exit code the process returned. Some tools encode failure shape in the code (e.g. lint vs. compile vs. infrastructure); cross-reference against the stderr tail.
- `${command_stderr_tail}` — the last lines of stderr captured at failure. This is your primary signal. Read it first.
- `${changed_files}` — the working-tree dirty file listing at the moment of failure (already captured at dispatch — you do not need to re-run `git status`). Cross-reference against the stderr tail; most command failures are explained by one recent edit.
- `${allowed_roots}` — multi-line block restricting where you may read or propose edits.

## Exception to the anti-rediscovery rule

The preamble forbids exploratory `git`/`gh` calls because every other
ATDD phase has its context fully substituted. Diagnosis is different:
`${changed_files}` lists *which files* are dirty, but not the *content*
of those changes. To diagnose what tripped the command, you need to
see the actual diff.

You may run:

- `git diff` (or `git diff HEAD`) — to see the line-level changes in
  the working tree that may have caused the command to fail.
- `git show HEAD:<path>` — to see the pre-edit state of a file you've
  already read in its current form.

You may NOT run `gh issue view`, `git log`, `git status`, `git branch`,
or `git rev-parse` — the ticket body and history are irrelevant to "why
this command failed," and the working tree state is already in
`${changed_files}`.

This exception applies only to this fix-* task. The CYCLE will not
re-dispatch you with the exception in force.

## What to do

1. **Read the stderr tail.** `${command_stderr_tail}` is the entire signal. Locate the first concrete error (file:line, missing dependency, configuration key, network endpoint) — most command failures point at one root cause and a wall of downstream noise.

2. **Classify the failure.** Each one is one of:
   - **A genuine environment or configuration bug** — a missing tool, a malformed config file, an unreachable resource, a permissions issue. The fix lives in the operator's setup or a config file inside `${allowed_roots}`.
   - **A regression in the SUT** introduced by a recent edit listed in `${changed_files}`. The command is exercising code that no longer compiles, lints, or passes the tool's checks. The fix restores the previously-working behaviour with the smallest change possible.
   - **Misuse of the command** — wrong arguments, wrong working directory, command run before a prerequisite step. The fix is in the calling CYCLE's wiring, not the SUT. Surface this in the diagnosis; the operator owns the wiring repair.

3. **Present the diagnosis.** One paragraph per distinct root cause (most command failures collapse to one). State the failing command, the line in `${command_stderr_tail}` that identifies the root cause, and — if applicable — the file in `${changed_files}` that explains it. Propose the smallest change that would let the command succeed. Do not apply the change.

## Anti-patterns

- **Re-running the command yourself "to see what happens."** Per the FIX contract, the caller re-validates. Re-running here wastes the budget and obscures who owns the signal.
- **Refactoring while you diagnose.** A "while I'm here" cleanup is the fastest way to need a second attempt the caller's budget does not have.
- **Blaming the working tree when the stderr points at the environment.** A missing binary or unreachable endpoint is not a SUT regression; don't propose a code edit when the fix is `apt install` or a config flip.
- **Blaming the environment when the stderr points at the working tree.** A compile error in a file you can see in `${changed_files}` is a regression; don't deflect to "maybe the toolchain is stale."
- **Diagnosing more than one or two files of change.** If the obvious fix would touch more than that, stop and reconsider — single command failures rarely require sprawling fixes. Surface the doubt in the diagnosis.
- **Editing anything in this task.** Diagnose only. The caller's PRE step decides what lands; the caller's verify step re-runs the command.

## Failing command

${command}

## Exit code

${command_exit_code}

## Captured stderr tail

${command_stderr_tail}

## Changed files at the moment of failure

${changed_files}

## Allowed roots

${allowed_roots}
