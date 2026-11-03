# 2026-06-23 19:25:44 UTC — Survive transient API outages in agent dispatch: resume-from-last-commit (Layer 2; Layer 1 wider retry shipped)

## TL;DR

**Why:** A sustained Anthropic API 529 "overloaded" outage (≈3.5 min) crashed the `stub-fidelity-test-writer` dispatch on the #65 rehearsal, and the existing transient-retry wrapper did not save it — only one attempt ran. The whole 31-minute run (3 completed agents + their commits: acceptance tests, DSL, contract tests; $3.96) was discarded and the loop stopped.
**End result:** When a crash still happens, re-running `gh optivem implement` on the same branch resumes at the failed step instead of re-running all completed steps from scratch. A late-step transient infra blip never throws away committed multi-agent work.

## Outcomes

What we get out of this — the goals and deliverables:

- **Re-running `implement` resumes, not restarts.** On a branch that already has earlier steps committed (acceptance / DSL / contract), a fresh `gh optivem implement` skips the completed call-activities and resumes at the failed step (`write-stub-fidelity-tests`). The completed agents are not re-paid for.
- **No false sense of safety.** The resume path is explicit about what it skips and why; a partial/uncommitted step is re-run, not silently assumed done.

## ▶ Next executable step (resume here)

Layer 1 shipped (clauderun now has its own wider retry schedule via `shell.WithSchedule`; the single-attempt cause was the result-text classification gap fixed in commit 4c1d66d0). Only Layer 2 remains and is still in **design**: the checkpoint signal and the re-entry skip mechanism need a design pass against the actual driver (`internal/atdd/process/`) before any edit. See Open questions. Run `/refine-plan` on this file, then `/execute-plan`.

## Steps

### Layer 2 — orchestration: resume from last commit

- [ ] **Step 4 (design, blocking): choose the checkpoint signal.** Survey what the driver already persists per completed step — the per-step `commit` call-activities (run digest steps 8, 13), the `.gh-optivem/runs/<ts>/` artifacts, the per-run `summary.md`, branch commit history — and pick a signal the driver can read on re-entry to know which call-activities are already done. Decide granularity: resume at call-activity boundaries (e.g. `write-acceptance-tests`, `implement-dsl`, `write-contract-tests`, `write-stub-fidelity-tests`) keyed on their commit markers.
- [ ] **Step 5: re-entry skip mechanism.** Make `gh optivem implement` on a branch with completed-step markers skip those call-activities and resume at the first incomplete one. Touch points: internal/atdd/process/process-flow.yaml + the driver under internal/atdd/process/ + the implement command path. A partial/uncommitted step is re-run (never assumed done).
- [ ] **Step 6: make the skip auditable.** On resume, log which steps were skipped and why (the marker that proved them done), so an operator can see the run resumed rather than silently restarted. Add coverage for the resume path.

## Open questions

- **Decision Q-5 (meta-plan Item 11, U12): resolved.** Build Layer 1 only; Layer 2 (resume) stays separate in this plan. Layer 1 is done.
- **Layer 2 — checkpoint signal.** Commit markers vs. run-artifact state vs. summary.md — which is authoritative and cheapest for the driver to read on re-entry? (Step 4.)
- **Layer 2 — resume granularity & safety.** Call-activity boundary is the natural unit, but channel-unrolled and per-external-system steps (the run had `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS_ERP`) complicate "is this step done?". How do we avoid skipping a step that committed partially? Default to re-running anything not provably complete.
- **Layer 2 — interaction with the rehearsal worktree.** The loop already keeps the failed worktree (atdd-rehearsal-loop.sh). Is resume meant to be driven by re-running `implement` in that same worktree/branch, or a fresh one? Confirm the intended operator gesture.
