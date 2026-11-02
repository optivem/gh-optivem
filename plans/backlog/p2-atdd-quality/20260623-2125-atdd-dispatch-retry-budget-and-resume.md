# 2026-06-23 19:25:44 UTC — Survive transient API outages in agent dispatch: wider clauderun retry + resume-from-last-commit

## TL;DR

**Why:** A sustained Anthropic API 529 "overloaded" outage (≈3.5 min) crashed the `stub-fidelity-test-writer` dispatch on the #65 rehearsal, and the existing transient-retry wrapper did not save it — only one attempt ran. The whole 31-minute run (3 completed agents + their commits: acceptance tests, DSL, contract tests; $3.96) was discarded and the loop stopped.
**End result:** Agent dispatch rides out a multi-minute API outage via a clauderun-specific (wider) retry budget, and — when a crash still happens — re-running `gh optivem implement` on the same branch resumes at the failed step instead of re-running all completed steps from scratch. A late-step transient infra blip never throws away committed multi-agent work.

## Outcomes

What we get out of this — the goals and deliverables:

- **The single-attempt regression is understood and pinned.** We know exactly why the existing `RetryWithPolicy` wrap (which 529/overloaded signatures already match, and which unit tests prove retries 4×) re-dispatched only once on the real run — classification gap in the live headless path, build/path staleness, or budget exhaustion — documented with the evidence.
- **Agent dispatch survives a multi-minute outage.** `clauderun` has its OWN retry budget (more attempts and/or longer max backoff), sized to ride out an outage on the order of the observed ~3.5 min, without widening the shared `retrycore` default that `gh`/`docker`/`sonar` reuse.
- **A regression test reproduces the real shape.** A test mirrors the actual failure (headless, result-event 529 with `is_error:true`, non-zero exit after the CLI's own internal retries are exhausted) and asserts the wrapper re-dispatches the configured number of times.
- **Re-running `implement` resumes, not restarts.** On a branch that already has earlier steps committed (acceptance / DSL / contract), a fresh `gh optivem implement` skips the completed call-activities and resumes at the failed step (`write-stub-fidelity-tests`). The completed agents are not re-paid for.
- **No false sense of safety.** The resume path is explicit about what it skips and why; a partial/uncommitted step is re-run, not silently assumed done.

## ▶ Next executable step (resume here)

Still in **design/refinement**, not mechanical edits. Two things must be settled before `/execute-plan`:
1. **Layer 1 investigation is a prerequisite, not an assumption** — Step 1 (reproduce/pin why only one retry fired) gates the *shape* of Step 2 (a classification gap is a code fix; pure budget-exhaustion is a tuning change; build staleness is neither). Do Step 1 first and let its finding rewrite Step 2.
2. **Layer 2 (resume) scope is open** — the checkpoint signal and the re-entry skip mechanism need a design pass against the actual driver before any edit. See Open questions.

Run `/refine-plan` on this file to resolve the open questions, then `/execute-plan`.

## Steps

### Layer 1 — clauderun: wider, smarter agent-dispatch retry

- [ ] **Step 1 (investigate, blocking): pin why only one wrapper attempt ran.** Reproduce against the real headless path, not the fake `scriptedClaude`. Candidate causes to rule in/out:
  - **Classification gap in the live path** — confirm `lastClassifyText` (clauderun.go:729-733: stderr tail + `runResult.ResultText` + stdout tail) actually contains a transient signature when the real `claude -p --output-format stream-json` run dies on a 529 result event. Verify `parseClaudeStreamJSON` returns the `result` text for `is_error:true / subtype:"success"` (clauderun.go:2423-2459) and that `runHeadless` populates `RunResult.ResultText` on the non-zero-exit path (clauderun.go:2062-2077).
  - **Budget exhaustion** — was the wrapper actually entered and did it just hard-fail? (Evidence says no: `.log` had zero `[clauderun] attempt N/M … retrying` warnings and RUN_AGENT 211.5s ≈ one 209.8s claude call. Confirm `log.Warnf` from `runWithRetryLoop` (retrycore.go:78) lands in the rehearsal `.log`, else its absence isn't proof.)
  - **Build/path staleness** — confirm the rehearsal-built binary (scripts/atdd-rehearsal.sh:37-38,354 build `gh-optivem.exe` from the gh-optivem working tree) actually included commit 4c1d66d's result-text folding at run time.
  - Output: a short written finding in this plan's Open questions → Outcomes, naming the cause at `file:line`. **This finding rewrites Step 2.**
- [ ] **Step 2 (fix, shape depends on Step 1): give agent dispatch its own wider retry budget.** Add a clauderun-specific attempts/backoff policy (e.g. a dedicated `retrycore` schedule or a `RetryWithPolicy` variant that takes explicit attempts/delays) — do NOT bump `defaultRetryAttempts` / `defaultRetryDelays` in retrycore.go (gh/docker/sonar mirror `optivem/actions/shared/retry-core.sh`; retrycore.go:10-22). Size it to ride out a ~multi-minute outage (more attempts and/or longer tail backoff). If Step 1 found a classification gap, fix that first — a wider budget is worthless if the wrapper never classifies the failure as retryable.
- [ ] **Step 3: regression test mirroring the real shape.** A `Dispatch`-level test where the runner returns a result-event 529 (`is_error:true`) with a non-zero exit and EMPTY stderr (the rehearsal-#65 shape), asserting the wrapper re-dispatches the configured number of times before falling through. Extend `scriptedClaude` if needed so it can model "result-text 529 forever" up to the new cap.

### Layer 2 — orchestration: resume from last commit

- [ ] **Step 4 (design, blocking): choose the checkpoint signal.** Survey what the driver already persists per completed step — the per-step `commit` call-activities (run digest steps 8, 13), the `.gh-optivem/runs/<ts>/` artifacts, the per-run `summary.md`, branch commit history — and pick a signal the driver can read on re-entry to know which call-activities are already done. Decide granularity: resume at call-activity boundaries (e.g. `write-acceptance-tests`, `implement-dsl`, `write-contract-tests`, `write-stub-fidelity-tests`) keyed on their commit markers.
- [ ] **Step 5: re-entry skip mechanism.** Make `gh optivem implement` on a branch with completed-step markers skip those call-activities and resume at the first incomplete one. Touch points: internal/atdd/process/process-flow.yaml + the driver under internal/atdd/process/ + the implement command path. A partial/uncommitted step is re-run (never assumed done).
- [ ] **Step 6: make the skip auditable.** On resume, log which steps were skipped and why (the marker that proved them done), so an operator can see the run resumed rather than silently restarted. Add coverage for the resume path.

## Open questions

- **Layer 1 — what actually caused the single attempt?** Resolved by Step 1. Until then, Step 2's shape (code fix vs. pure tuning vs. nothing-but-build-hygiene) is undetermined. *This is the highest-priority unknown.*
- **Layer 1 — budget sizing.** How many attempts / what tail backoff is "enough" without making a genuinely-dead API hang the run for too long? The CLI already does 10 internal retries (~209s) before the wrapper even sees a failure, so the wrapper's budget stacks on top of that. Pick a number that bounds total worst-case wait sensibly.
- **Layer 2 — checkpoint signal.** Commit markers vs. run-artifact state vs. summary.md — which is authoritative and cheapest for the driver to read on re-entry? (Step 4.)
- **Layer 2 — resume granularity & safety.** Call-activity boundary is the natural unit, but channel-unrolled and per-external-system steps (the run had `IMPLEMENT_AND_VERIFY_EXTERNAL_DRIVER_ADAPTERS_ERP`) complicate "is this step done?". How do we avoid skipping a step that committed partially? Default to re-running anything not provably complete.
- **Layer 2 — interaction with the rehearsal worktree.** The loop already keeps the failed worktree (atdd-rehearsal-loop.sh). Is resume meant to be driven by re-running `implement` in that same worktree/branch, or a fresh one? Confirm the intended operator gesture.
- **Sequencing.** Layer 1 is small and high-value (prevents the loss in the first place); Layer 2 is heavier (limits the cost when a crash still happens). Land Layer 1 first; Layer 2 can be a follow-up if scope grows.
