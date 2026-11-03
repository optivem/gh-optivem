# 2026-06-23 19:25:44 UTC — Survive transient API outages in agent dispatch: resume-from-last-commit (Layer 2; Layer 1 wider retry shipped)

## TL;DR

**Why:** A sustained Anthropic API 529 "overloaded" outage (≈3.5 min) crashed the `stub-fidelity-test-writer` dispatch on the #65 rehearsal, and the existing transient-retry wrapper did not save it — only one attempt ran. The whole 31-minute run (3 completed agents + their commits: acceptance tests, DSL, contract tests; $3.96) was discarded and the loop stopped.
**End result:** When a crash still happens, re-running `gh optivem implement` on the same branch/worktree (no new flag) resumes at the first incomplete top-level call-activity instead of re-running completed ones. A call-activity counts as done only if its commit marker exists on the branch and the tree is clean; anything else (including a partially committed channel/external-system unit) is re-run whole. On resume the run logs each skipped step and the commit that proved it done. A fresh branch has no markers and starts over, unchanged. A late-step transient infra blip never throws away committed multi-agent work.

## Outcomes

What we get out of this — the goals and deliverables:

- **Re-running `implement` resumes, not restarts.** On a branch that already has earlier steps committed (acceptance / DSL / contract), a fresh `gh optivem implement` skips the completed call-activities and resumes at the failed step (`write-stub-fidelity-tests`). The completed agents are not re-paid for.
- **No false sense of safety.** The resume path is explicit about what it skips and why; a partial/uncommitted step is re-run, not silently assumed done.

## ▶ Next executable step (resume here)

Layer 1 shipped (clauderun now has its own wider retry schedule via `shell.WithSchedule`; the single-attempt cause was the result-text classification gap fixed in commit 4c1d66d0). Only Layer 2 remains and is still in **design**: the checkpoint signal and the re-entry skip mechanism were decided (see Resolved decisions: commit markers, top-level call-activity granularity, same-branch re-run). Next: Step 4 reduces to a short survey of which commit each call-activity already produces (run digest steps 8, 13) to define the marker, then Steps 5-6 in `internal/atdd/process/`. Run `/execute-plan` on this file.

## Steps

### Layer 2 — orchestration: resume from last commit

- [ ] **Step 4 (design, blocking): choose the checkpoint signal.** Survey what the driver already persists per completed step — the per-step `commit` call-activities (run digest steps 8, 13), the `.gh-optivem/runs/<ts>/` artifacts, the per-run `summary.md`, branch commit history — and pick a signal the driver can read on re-entry to know which call-activities are already done. Decide granularity: resume at call-activity boundaries (e.g. `write-acceptance-tests`, `implement-dsl`, `write-contract-tests`, `write-stub-fidelity-tests`) keyed on their commit markers.
- [ ] **Step 5: re-entry skip mechanism.** Make `gh optivem implement` on a branch with completed-step markers skip those call-activities and resume at the first incomplete one. Touch points: internal/atdd/process/process-flow.yaml + the driver under internal/atdd/process/ + the implement command path. A partial/uncommitted step is re-run (never assumed done).
- [ ] **Step 6: make the skip auditable.** On resume, log which steps were skipped and why (the marker that proved them done), so an operator can see the run resumed rather than silently restarted. Add coverage for the resume path.

## Resolved decisions

- **Q-5 (meta-plan Item 11, U12):** build Layer 1 only; Layer 2 (resume) stays separate in this plan. Layer 1 is done.
- **Checkpoint signal (Step 4): git commit markers on the branch.** Commit history is the only state that survives a crash, a fresh process, or a re-clone, and it is precisely the work not to re-pay for. Run artifacts (`.gh-optivem/runs/<ts>/`, `summary.md`) are per-run and untracked, so they are not authoritative.
- **Resume granularity & safety: top-level call-activity boundaries, skip only if provably complete.** A unit (acceptance / DSL / contract / stub-fidelity) is skipped only if its commit marker exists AND the working tree has no uncommitted changes. Unrolled channel / external-system sub-steps are never skipped individually; a partially committed unit is re-run whole. Worst case is repaying one unit, never skipping unfinished work.
- **Operator gesture: automatic, no new flag.** Re-run `gh optivem implement` on the same branch/worktree (the rehearsal loop already keeps the failed worktree). Markers live on that branch, so it resumes; a fresh worktree/branch has no markers and starts over, which is unchanged behaviour.
