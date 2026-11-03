# 2026-06-23 19:25:44 UTC — Survive transient API outages in agent dispatch: resume-from-last-commit (Layer 2; Layer 1 wider retry shipped)

## TL;DR

**Why:** A sustained Anthropic API 529 "overloaded" outage (≈3.5 min) crashed the `stub-fidelity-test-writer` dispatch on the #65 rehearsal, and the existing transient-retry wrapper did not save it — only one attempt ran. The whole 31-minute run (3 completed agents + their commits: acceptance tests, DSL, contract tests; $3.96) was discarded and the loop stopped.
**End result:** When a crash still happens, re-running `gh optivem implement` on the same branch/worktree (no new flag) resumes at the first incomplete top-level call-activity instead of re-running completed ones. A call-activity counts as done only if its commit marker exists on the branch and the tree is clean; anything else (including a partially committed channel/external-system unit) is re-run whole. On resume the run logs each skipped step and the commit that proved it done. A fresh branch has no markers and starts over, unchanged. A late-step transient infra blip never throws away committed multi-agent work.

## Outcomes

What we get out of this — the goals and deliverables:

- **Re-running `implement` resumes, not restarts.** On a branch that already has earlier steps committed (acceptance / DSL / contract), a fresh `gh optivem implement` skips the completed call-activities and resumes at the failed step (`write-stub-fidelity-tests`). The completed agents are not re-paid for.
- **No false sense of safety.** The resume path is explicit about what it skips and why; a partial/uncommitted step is re-run, not silently assumed done.

## ▶ Next executable step (resume here)

Layers 1 and 2 are implemented and verified on shop #76 (2026-11-03 rehearsal, monolith-java): killed after the ACCEPTANCE TESTS + DSL commit, re-ran in the same worktree, log showed `resume: skipping write-acceptance-tests-and-dsl@WRITE_AND_VERIFY_ACCEPTANCE_TEST_CODE: already done in commit <sha>`, then the run continued to the system implementer, committed SYSTEM (api), rc=0. Two fixes came out of it: `WorkingTreeClean` ignores `.atdd-prompt-*.tmp.md`, and the acceptance unit snapshots the per-channel RED report names into `Atdd-State` (`acceptance-red-reports`) because the on-disk reports are build output that does not survive. One decision remains (below); a fresh-clone resume has not been run.

## Steps

- [ ] Decide the tracked-partial-edit case. A crash mid-way through the next unit leaves tracked edits (e.g. a half-written `OrderService.java`), the tree is dirty, and resume re-runs the finished unit instead of skipping it (`resume: re-running …: working tree has uncommitted changes`). Today the operator must `git checkout HEAD -- <file>` first. Options: leave as is and document it, or have resume discard the partial unit's tracked edits itself (destructive — needs a decision).
- [ ] Optional: confirm a fresh clone resumes from the trailer alone (should now work since the reports are in `Atdd-State`).


## Resolved decisions

- **Q-5 (meta-plan Item 11, U12):** build Layer 1 only; Layer 2 (resume) stays separate in this plan. Layer 1 is done.
- **Checkpoint signal (Step 4): git commit markers on the branch.** Commit history is the only state that survives a crash, a fresh process, or a re-clone, and it is precisely the work not to re-pay for. Run artifacts (`.gh-optivem/runs/<ts>/`, `summary.md`) are per-run and untracked, so they are not authoritative.
- **Resume granularity & safety: top-level call-activity boundaries, skip only if provably complete.** A unit (acceptance / DSL / contract / stub-fidelity) is skipped only if its commit marker exists AND the working tree has no uncommitted changes. Unrolled channel / external-system sub-steps are never skipped individually; a partially committed unit is re-run whole. Worst case is repaying one unit, never skipping unfinished work.
- **Operator gesture: automatic, no new flag.** Re-run `gh optivem implement` on the same branch/worktree (the rehearsal loop already keeps the failed worktree). Markers live on that branch, so it resumes; a fresh worktree/branch has no markers and starts over, which is unchanged behaviour.
- **State restore on skip (added at execution, 2026-11-03): commit trailer carries marker + state.** Skipping a unit loses the in-memory `ctx.State` it produced (`test-names`, `dsl-port-changed`, `system-driver-port-changed`, `external-driver-port-changed`, `external-system-touched`, `isolated-test-names`, `real-kind`, `stub-fidelity-tests-present`, acceptance RED-verify reports), which later steps read. So the `commit` action (`process-flow.yaml` ~L2881, `gh optivem commit --yes --all --include-untracked`) emits a trailer, e.g. `Atdd-Unit: <process>@<node>` plus `Atdd-State: <json of that unit's outputs>`; resume rehydrates `ctx.State` from it. Old commits have no trailer and simply never count as markers (backward compatible). Skip hook goes in `wrapCallActivity` / `runProcess` (`internal/engine/statemachine/run.go`).
- **Skippable units: only units that end in a commit.** Acceptance tests (`write-and-verify-acceptance-test-code`), the `implement-and-verify-dsl` and `implement-and-verify-system` call sites, `implement-and-verify-system-driver-adapters`, and `refactor-and-verify-tests`. Contract-test and stub-fidelity writers (and external-system driver adapters / stubs) have no commit of their own, so they re-run as part of the external-system unit whole. Skip only if trailer present on the branch AND tree clean. The existing `--target` resume in `internal/atdd/runtime/driver/scoped.go` is content-addressed and is not touched.
