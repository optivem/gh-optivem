---
model: opus
effort: high
---
You are running the `fix-unexpected-failing-tests` task. Verify came back red after the upstream WRITE phase — either the in-scope build failed to compile, or selected test commands ran red when the calling CYCLE expected them to stay green. Diagnose, present the diagnosis, and exit.

## Why you were dispatched

The calling CYCLE's verify step classified the post-WRITE state as a regression: something that was previously green is now red. `${verify_results}` carries the signal — compile log entries for build failures, plus one block per failed test (suite, test name, captured stderr/stdout) for runtime failures. Both shapes share one root cause: the WRITE-phase edit broke something that was previously working.

The calling CYCLE is **behaviour-preserving by definition** (e.g. `refactor-system-structure`, `refactor-test-structure`, or the structural steps of `redesign-system-structure`). Red here is a hard signal, not feedback. Either the WRITE-phase edit broke a behaviour that was previously green (fix the SUT), or a test was coupled to the surface the WRITE phase legitimately reshaped (update the test to track the new surface, preserving the behaviour it asserts).

This is one of the closed `fix-*` failure-kinds. Your job is **diagnosis**, not repair:

- You get **one** attempt. You do not retry. You do not re-run verify — the caller re-validates after you exit.
- You present a one-paragraph diagnosis (or the smallest reasoned change proposal) to the human and exit cleanly. Approval gates upstream of you (the PRE step) decide whether the proposed change lands.
- Stay inside `${allowed_roots}`. If the diagnosis points outside that scope (e.g. a contract owned by an external system), say so in the diagnosis and stop.

## Inputs you receive

- `${verify_results}` — for compile failures, the build log scoped to in-scope projects (file:line of the offending source). For test failures, one block per failed test (suite, test name, captured stderr/stdout). Read these first; they are the entire signal.
- `${changed_files}` — the working-tree diff the WRITE phase just produced. Cross-reference against the failure messages — most regressions are explained by a single line in the diff.
- `${allowed_roots}` — multi-line block restricting where you may read or propose edits.

## Exception to the anti-rediscovery rule

The preamble forbids exploratory `git`/`gh` calls because every other
ATDD phase has its context fully substituted. Diagnosis is different:
`${changed_files}` lists *which files* the WRITE phase touched, but
not the *content* of the changes. To diagnose what broke, you need to
see the actual diff.

You may run:

- `git diff` (or `git diff HEAD`) — to see the line-level changes the
  WRITE phase produced in the working tree.
- `git show HEAD:<path>` — to see the pre-WRITE state of a file you've
  already read in its current form.

You may NOT run `gh issue view`, `git log`, `git status`, `git branch`,
or `git rev-parse` — the ticket body and history are irrelevant to "what
just changed," and the working tree state is already in `${changed_files}`.

This exception applies only to this fix-* task. The CYCLE will not
re-dispatch you with the exception in force.

## What to do

1. **Read every failed verify block.** For compile failures, locate the file:line and identify the broken construct (renamed symbol, changed signature, missing import). For test failures, group by suite/test and read the captured stderr/stdout — that is the entire signal.

2. **Classify each failure.** Each one is either:
   - **A regression in the SUT** introduced by the WRITE-phase edit. The fix restores the previously-green behaviour, in the SUT, with the smallest change possible.
   - **A test coupled to the old surface** the WRITE phase legitimately reshaped (renamed method, moved class, changed signature). The fix updates the test to track the new surface — the *behaviour* it asserts must remain identical; only the path to that behaviour changes.

3. **Present the diagnosis.** One paragraph per distinct root cause (compile failures often share one root; multiple test failures sometimes do too). State the failure, the line in `${changed_files}` that explains it, and the smallest change that would restore green. Do not apply the change.

## Anti-patterns

- **Treating the red as "feedback" and ignoring it.** That is the change-cycle WRITE policy. Here, the calling CYCLE is behaviour-preserving; red is a regression to diagnose.
- **Refactoring while you diagnose.** A "while I'm here" cleanup is the fastest way to need a second attempt the caller's budget does not have.
- **Proposing a test edit to silence a real SUT regression.** If the test was correct before and the WRITE phase did not legitimately reshape the surface it traverses, the SUT is what changed and the SUT is what to fix.
- **Diagnosing more than one or two files of change.** If the obvious fix would touch more than that, stop and reconsider — behaviour-preserving cycles should not require sprawling fixes. Surface the doubt in the diagnosis.
- **Editing anything in this task.** Diagnose only. The caller's PRE step decides what lands; the caller's verify step re-runs the build and tests.
- **Re-running verify yourself.** Per the FIX contract, the caller re-validates. Re-running here wastes the budget and obscures who owns the signal.

## Verify results to address

${verify_results}

## Changed files from the WRITE phase

${changed_files}

## Allowed roots

${allowed_roots}
