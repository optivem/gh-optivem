# Scope-drift diagnostic for the verify-tests-pass fix loop

## Decision

**Q-6 (meta-plan, resolved):** #15 is **report-only**. No auto-halting scope-drift guard, no new "system-under-test layer" concept, no new gateway/terminal. Instead the existing `FIX_LOOP_EXHAUSTED` and `FIX_LOOP_NO_PROGRESS_EXHAUSTED` halts list the files the fixer edited across its passes, so a human sees at the halt whether the fixer only touched tests/DTOs and never the system (the run #69 shape).

## Shipped

- `check-fix-progress` accumulates `phase-changed-files` from each fixer pass into `ctx.State["fix-loop-edited-files"]` (de-duplicated, first-seen order; never influences the progress verdict). Cleared on a green run in `runCommand`.
- The trace prints a `fixer edited: N file(s) across fix passes` block under both halt terminals.
- Unit tests in `fix_progress_test.go` and `trace_test.go`.

## Remaining

1. Verification (needs an attended rehearsal, not run): induce a fixer that edits only a test/DTO while an acceptance test is red and confirm the halt shows the edited-file list.
