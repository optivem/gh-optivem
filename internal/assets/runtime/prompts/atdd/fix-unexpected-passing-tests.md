---
model: opus
effort: high
---
You are running the `fix-unexpected-passing-tests` task. A test that the upstream WRITE-phase asserted must FAIL has instead PASSED. Diagnose, present the diagnosis, and exit.

## Inputs

### Scope

${scope_block}

- `${verify_results}` — one block per verify command. The relevant blocks are the ones reporting the unexpectedly-passing test(s): suite, test name, captured stdout/stderr showing the assertion that should have tripped but did not.
- `${changed_files}` — the working-tree diff the WRITE phase just produced (the new test, plus whatever supporting code it touched). Cross-reference the assertion against the SUT path it exercises.

### Verify results to address

${verify_results}

### Changed files from the WRITE phase

${changed_files}

## Steps

1. **Identify the asserting line.** From `${verify_results}` and the diff in `${changed_files}`, find the exact assertion the test expected to trip (e.g. an expected exception, an expected error return, an expected validation rejection). Name it precisely.

2. **Trace why the SUT accepted the input.** Walk from the test's entry point into the SUT and identify which branch, guard, or validation was expected to reject the case but did not. Common shapes:
   - The guard exists but is keyed off a different field/condition than the test assumed.
   - The guard was removed or weakened by an earlier change that escaped the cycle that authored the test.
   - The test set up an input that does not actually exercise the path it names (mis-targeted assertion).
   - The SUT's contract already allows the case the test wants to reject — the requirement encoded in the test is wrong or out of date.

3. **Present the diagnosis as one paragraph.** State (a) what the test asserted, (b) why the SUT accepted the input, (c) whether the fix belongs in the SUT (tighten the guard) or in the test (the case is already allowed by contract and the test is wrong). Do not apply the fix.

## Additional Notes

### Why you were dispatched

The calling CYCLE's WRITE step authored a test predicting a specific failure in the system under test (SUT), then handed control to its verify step. The verify step ran the selected test commands and observed the new test passing — the opposite of what was asserted. That mismatch is what brought you here: the production system is more lenient than the test predicted, so the test cannot drive the change it was written to drive.

This is one of the closed `fix-*` failure-kinds. Your job is **diagnosis**, not repair:

- You get **one** attempt. You do not retry. You do not re-run verify — the caller re-validates after you exit.
- You present a one-paragraph diagnosis to the human and exit cleanly. Approval gates upstream of you (the PRE step) decide whether the proposed change lands.
- You do not edit the SUT and you do not edit the test in this task. Diagnose only.

### Exception to the anti-rediscovery rule

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

### Anti-patterns

- **Editing anything.** This task diagnoses. The caller's PRE step decides what lands; the caller's verify step re-runs the tests.
- **Retrying.** One attempt. If your diagnosis is wrong, the human takes over.
- **Re-running verify yourself.** Per the FIX contract, the caller re-validates. Re-running here wastes the budget and obscures who owns the signal.
- **Speculating about the operator's intent.** If the test's assertion is ambiguous, say so in the diagnosis and stop — do not guess which side (SUT or test) is wrong.
