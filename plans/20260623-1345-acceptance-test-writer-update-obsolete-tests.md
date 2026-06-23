# 2026-06-23 13:45:00 UTC — acceptance-test-writer: update obsolete tests on a behavior-change ticket

## TL;DR

**Why:** On a behavior-CHANGE ticket (#76: extend the Dec-31 cancellation blackout from 22:30 to 23:00) the `acceptance-test-writer` added a new red test for the new rule but left pre-existing `@isolated` boundary tests pinning the OLD rule. No single code value could satisfy both, so the system-implementers (test-write-blind, system-scope only) oscillated and the run halted after 30 min.
**End result:** `acceptance-test-writer.md` instructs the writer that when a ticket changes a rule already covered by existing acceptance tests, it must find and update those obsolete tests so the suite has one consistent definition of the rule — and the file's contradicting "mechanical 1:1 / don't touch untouched tests" guidance is reconciled to allow it.

## Outcomes

What we get out of this — the goals and deliverables:

- A behavior-change ticket never yields an internally-contradictory acceptance suite: the new test and any pre-existing tests of the same rule agree on one boundary/threshold/rule.
- `acceptance-test-writer` explicitly owns updating pre-existing acceptance tests (including `@isolated` parameterized/boundary tests in the same feature/area) when the ticket redefines a rule those tests encode.
- The existing "mechanical 1:1 translation … do not classify items" instruction carries a carve-out for the change-an-existing-rule case (so the new guidance doesn't contradict the old).
- The Outputs `test-names` note no longer excludes pre-existing tests: when a behavior-change ticket legitimately modifies an existing test, that test's method name is reported in `test-names`.
- Guidance is language-agnostic in prose (the per-language `@isolated` shapes already live in `isolated-marker-{typescript,java,csharp}.md`).

## ▶ Next executable step (resume here)

Edit `internal/atdd/assets/runtime/agents/atdd/acceptance-test-writer.md`: add a "behavior-change tickets — reconcile pre-existing tests" instruction to Step 1 (writer must locate pre-existing acceptance tests encoding the rule being changed — including `@isolated` boundary/parameterized tests in the same feature/area — and update them to the new rule), add a carve-out to the existing "mechanical 1:1 translation … do not classify items" sentence so it doesn't forbid this, and update the Outputs Notes `test-names` bullet so modified pre-existing tests are reported. No behavior outside this one file. Stop and let the user review the wording before committing.

## Steps

- [ ] Step 1: Re-read `internal/atdd/assets/runtime/agents/atdd/acceptance-test-writer.md` Step 1 + Outputs Notes to fix exact insertion points and current wording.
- [ ] Step 2: In Step 1, add a **behavior-change clause**: when the ticket changes an existing business rule (a bug fix / behavior change that moves or redefines a boundary, threshold, or rule already covered by existing acceptance tests), the writer must (a) **discover** the pre-existing acceptance tests that encode the OLD rule by **grepping the rule's distinguishing literals across `tests/**/acceptance`** — the boundary values/times and the error-message string the rule uses — explicitly including `@isolated` parameterized/boundary tests, and (b) update their data rows / assertions to the new rule so the whole suite agrees. **Trigger heuristic:** if that discovery finds a pre-existing acceptance test already asserting a value for this rule, treat the ticket as a behavior change and reconcile (no separate ticket-type signal required). Give the #76 shape as the worked example (22:30:01 moves from allowed→rejected; add in-window rows like 22:45 / 22:59:59; first fully-outside time becomes ≥23:00:01).
- [ ] Step 3: Add a **carve-out** to the existing "mechanical 1:1 translation … translate every AC item, and do not classify items" sentence so it explicitly does not forbid editing pre-existing tests of a rule the ticket redefines (the two instructions must not contradict).
- [ ] Step 4: Update the Outputs **Notes** bullet `test-names is every unqualified test method name added or modified by this ticket … not pre-existing tests the ticket did not touch` so that pre-existing tests the writer *modifies under the behavior-change clause* ARE reported in `test-names` (they are now touched by the ticket).
- [ ] Step 5: Re-read the whole file once for internal consistency (no remaining sentence implies "never touch existing tests"), confirm prose stays language-agnostic, and stop for user review.

## Open questions

_(resolved — folded into Step 2)_

- **Discovery scope** → grep the rule's distinguishing literals (boundary values/times + the error-message string) across `tests/**/acceptance`; that reliably finds the contradicting rows.
- **Behavior-change vs new rule** → key on discovery: if a pre-existing acceptance test already asserts a value for this rule, treat the ticket as a behavior change and reconcile.
