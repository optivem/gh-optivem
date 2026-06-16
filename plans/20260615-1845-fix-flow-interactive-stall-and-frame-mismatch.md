# 2026-06-15 18:45:00 UTC — Fix-flow stall: interactive human-dispatch in unattended runs + fixer frame-mismatch

## TL;DR

**Why:** Run #69 (`Reject order with line quantity of 100`) burned **2h47m wall-clock and then halted on a bogus infra error**, and almost all of that time (2h14m58s) was a single `unexpected-failing-tests-fixer` dispatch that did **no measurable work** (no event stream, `—` tokens, `—` cost). Root cause is two independent defects:

1. **`fix-unexpected-failing-tests` is `category: human`** (`process-flow.yaml:2079`), which forces **interactive** mode (`driver.go:1234`: `Headless: opts.Headless && nodeParams["category"] != "human"`). In an **unattended rehearsal** the interactive `claude` TUI sat on the operator's TTY waiting for input that never came → ~2h14m of dead wall-clock. Nothing in the process bounds how long a human-category dispatch may block.
2. **The fixer's prompt only models a behaviour-preserving cycle** ("red = a regression; restore the previously-green behaviour, or update a test coupled to a reshaped surface"). It was dispatched from the **new-behaviour** green-verify (`verify-tests-pass`, verifying the brand-new `shouldRejectOrderWithLineQuantityOf100`), where the behaviour was **never green** — the system simply wasn't finished. With no branch for "implementation incomplete" and no exit-on-uncertainty, the fixer/human edited test sources + DTOs, **broke test compilation**, and the next run found zero discoverable tests → `empty test selection` → `TESTS_INFRA_HALT`.

**End result:** A fix-flow dispatch in an unattended run can no longer silently block for hours, the runner surfaces *why* a human-category node was reached without an operator, and the fixer prompt has a correct frame (or correct routing) for the new-behaviour case plus an explicit "if uncertain, halt for a human" exit — so it never guesses its way into a broken build.

## What happened — evidence

- `summary.md`: `#6 unexpected-failing-tests-fixer  api opus high  2h14m58s  —  —  —` and `#7 … 8m43s  —  —  —`. Agents #1–#5 all show real token/cost.
- Run dir: agents #1–#5 each have `NNN-*.events.jsonl` + `.events.log` + `.outputs.jsonl`. Fixers #6/#7 have **only `NNN-*.prompt.md`** — no event stream was ever written.
- `clauderun.go`: token/cost come from the stream-json `type:"result"` event's `usage` + `total_cost_usd`, emitted **only in headless** (`claude -p --output-format stream-json --verbose`). Interactive mode renders to the operator TTY and emits no machine-readable result. `driver.go:1841` states it outright: *"token + cost capture is headless-only; interactive runs show — for those columns"*. → the `—` columns are the fingerprint of an **interactive** dispatch, and the 2h14m is human-wait, not compute.
- `process-flow.yaml:2060-2079`: `fix-unexpected-failing-tests → execute-agent` with `category: human` (comment: *"human tier: fix-flow dispatch — signals upstream defect, never bypassable"*).
- `flow.txt`: first `RUN_TESTS` at `VERIFY_TESTS_PASS` → `test-outcome=fail` (a genuine red, `> Task :test FAILED`) → fixer #6 → re-run `fail` → fixer #7 → re-run → `No tests found … *.shouldRejectOrderWithLineQuantityOf100` → `test-outcome=infra` → `TESTS_INFRA_HALT`.
- Worktree git status: fixer touched `PlaceOrderNegativeIsolatedTest.java`, `PlaceOrderNegativeTest.java`, `BrowseOrdersResponse.java` (driver-port DTO), `PlaceOrderRequest.java` (core DTO). Gradle left `build/tmp/compileTestJava/compileTransaction/stash-dir/PlaceOrderNegativeIsolatedTest.class.uniqueId1` + `PlaceOrderNegativeTest.class.uniqueId0` — the **rollback stash Gradle writes when a test compile fails**. The method still exists in source (`PlaceOrderNegativeIsolatedTest.java:16`); it was simply **not compiled → not discoverable** → empty `--tests` selection → infra. "Not discoverable" = "didn't compile," not "didn't exist."

## Conclusion (corrected)

The first read — "a runaway agent burned ~2h of compute" — is **wrong**. Evidence-backed conclusion:

- The fixer did **near-zero work**. The `—` tokens/cost are the **fingerprint of an interactive dispatch** (token/cost are parsed from the stream-json `type:"result"` event, which **only headless** emits — `driver.go:1841`). So the 2h14m was an interactive, `category: human` dispatch **blocking on an absent operator** in an unattended rehearsal — pure human-wait, not LLM computation.
- The `empty test selection` / `TESTS_INFRA_HALT` is a **downstream symptom**, not the cause. The fix session broke test **compilation** (Gradle rollback stash present), so zero tests were discoverable. The infra-halt was correct — it just fired late and named the wrong villain.
- There are **two independent defects**: (1) **process/runtime** — nothing bounds a human-category dispatch in an unattended run, so it stalls silently for hours; (2) **agent instructions** — the regression-framed fixer is dispatched into the new-behaviour green-verify where its frame doesn't apply and it has no exit-on-uncertainty, so it guesses and corrupts the build.
- **Could BPMN have prevented it?** It can't make a human appear, but it *can* stop a human-gate blocking forever (run headless / fail-fast when there's no TTY), pre-flight that the named test is discoverable before dispatching anyone, and halt on no-progress/scope-drift. The single highest-value change is **Step 1** — it would have ended this run in minutes regardless of the frame-mismatch.

Notes that informed the steps below:
- **Token/cost `—` = interactive, by design** — not a capture bug. Headless-only (`clauderun.go` result event; `driver.go:1841`).
- **"Not discoverable" = "didn't compile"**, not "didn't exist" — method present at `PlaceOrderNegativeIsolatedTest.java:16`; Gradle `compileTransaction/stash-dir` rollback stashes prove a failed test compile.
- **A wall-clock bound belongs at the dispatch layer, not the prompt** — the model can't see elapsed time. The prompt gets a *behaviour* bound ("one diagnosis, smallest single fix, then exit; halt-for-human if uncertain"); the orchestrator gets the idle/stall watchdog.
- **First-run and retries share one classification path** — `verify-tests-pass` re-enters the same `RUN_TESTS → GATE_TESTS_OUTCOME` nodes each loop, so the pre-flight discovery guard goes once inside `run-tests` (no duplication).
- **A halt here is resumable, not destructive.** The engine has a **git-state-derived resume mechanism** (`internal/atdd/runtime/driver/scoped.go`): every phase commits as it completes, and a resume skips phases already DONE in the committed tree and re-enters at the first unfinished slice. Run #69's committed phases are `ACCEPTANCE TESTS` → `DSL` → `SYSTEM DRIVER ADAPTERS` (`527a9f2c`); the hung fix/verify step is after that and was never committed. So halting at the human gate loses **nothing earned** — a human resumes later, the committed phases are skipped, and they re-enter the human gate *with an operator present*. The only thing discarded is the broken uncommitted fixer edits (which we want gone — they're what broke test compilation). This means "human can proceed" is satisfied by **resume**, not by blocking a live TTY for hours (which an unattended overnight session can't usefully do anyway — there is no mid-dispatch checkpoint).

## Outcomes

- A **human-category dispatch in an unattended run** (rehearsal / CI / any non-TTY context) does not block indefinitely: it either runs headless or halts fast with a clear "human gate reached, no operator present" message — never a multi-hour silent stall.
- The eventual halt is attributed to the **real** cause (interactive stall / incomplete implementation), not to a downstream `empty test selection` infra symptom.
- The `unexpected-failing-tests-fixer` prompt is correct for **every** cycle that dispatches it — including new-behaviour green-verify — or that cycle routes to a different responder. Either way the fixer never "guesses a side" when neither of its two diagnoses fits.
- The fixer has an explicit **exit-on-uncertainty** path (halt-for-human envelope) so a single pass bails instead of editing blindly and corrupting the build.
- A **pre-flight test-discovery check** turns "named test doesn't compile/exist" into an immediate, clearly-worded halt **before** any fixer is dispatched — added in one shared place, so the first run and every retry get identical treatment (no duplicated classification).

## ▶ Next executable step (resume here)

**Status: diagnosis complete and evidence-backed; no code changed yet. Next action is decision work, not editing.** Resolve the four **STILL PENDING** Open questions (below) via `/refine-plan`, then implement Steps 1–5.

Where the conversation left off:
- **Q1 (unattended human gate)** is the live one. Leading candidate = **(a) idle-timeout → resumable halt + notify + reset-to-last-DONE-commit on resume**. Established that a halt is *resumable* (git-state resume, `scoped.go`), so the human proceeds via resume, not by parking a live TTY. Still to pick: idle-timeout value, how "no operator" is detected (no TTY vs. explicit unattended flag), and whether a notification is in scope now.
- **Q2 (frame-mismatch: prompt vs routing)** — not yet chosen. Before deciding, optionally confirm from `process-flow.yaml` whether new-behaviour green-verify is *meant* to use `unexpected-failing-tests-fixer` at all.
- **Q3 (watchdog scope)** and **Q4 (first-run infra classification — verify)** — untouched.

Step 1 (unattended human-dispatch guard) is independently valuable and can land first regardless of how Q2 resolves.

## Steps

- [ ] **Step 1 — Bound human-category dispatch in unattended runs.** In `driver.go` (around the `Headless: opts.Headless && nodeParams["category"] != "human"` decision, line ~1234), detect "no operator TTY available" (rehearsal / non-interactive stdin) and bound the wait per Open Q1. **Leading candidate (pending decision): idle-timeout → resumable halt + notify**, and on resume **reset the working tree to the last DONE commit** so the human re-enters the human gate from a clean, compiling tree (not the half-broken state a timed-out attempt leaves behind). Do **not** let it block silently. (Resume itself already exists — `scoped.go` — so the halt just needs to fire cleanly and the broken uncommitted edits to be reset.)
- [ ] **Step 2 — Pre-flight test-discovery guard (single shared path).** Before the `verify-tests-pass` fix loop can dispatch a fixer, assert the named `${test-names}` is actually discoverable in `${suite}` (a cheap `--tests … ` dry-run / test listing). On empty selection, halt with a precise message ("named test not discoverable — did it compile / is it named correctly?"). Add it once inside `run-tests` (every invocation — first run and `↻ retry N` re-enter the *same* `RUN_TESTS → GATE_TESTS_OUTCOME` nodes, so there is no first-run/retry duplication to keep in sync) so the classification logic stays singular.
- [ ] **Step 3 — Fix the fixer frame-mismatch** (shape depends on Open Q2):
  - **3a (prompt):** add a third diagnosis branch to `unexpected-failing-tests-fixer.md` for *"the new behaviour was never implemented / the test was never green"* — whose action is **not** "edit the test" but "report incomplete implementation" (or hand back to the system-implementer), plus an explicit **exit-on-uncertainty** instruction: when none of the readings clearly fits, emit a halt-for-human envelope and exit rather than guess. **OR**
  - **3b (routing):** make the new-behaviour green-verify (`implement-and-verify-system` → `verify-tests-pass`) route a still-red **new** test back to `system-implementer` / a dedicated responder, and reserve `unexpected-failing-tests-fixer` for the behaviour-preserving (refactor) cycles its prompt actually describes.
- [ ] **Step 4 — No-progress / scope-drift guard for the fix loop.** The flow already captures `pre-agent-fingerprint` and `phase-changed-files` per dispatch. Add a guard so the loop halts when consecutive fix passes produce no change to test state, or when the fixer's only edits fall **outside** the system-under-test scope (e.g. it edited a driver-port DTO but the red test is an API acceptance test). Layer under the existing `max-visits: 2` count cap — that bounds *count*, not *relevance* or *wall-clock*.
- [ ] **Step 5 — Tests / verification.** Unit-test the unattended human-dispatch decision (TTY present vs absent → headless/halt per Q1); test the pre-flight discovery guard (named test missing/uncompilable → clean halt, not a fixer dispatch); regression-check that a normal headless fix flow is unchanged. Scope `go test` per-package (no unbounded `./...` on Windows).

## Open questions — STILL PENDING (awaiting decision)

None of these are decided yet. Resolve via `/refine-plan` before the corresponding step is implemented.

1. **Unattended human-category behaviour — what happens when the human isn't at the computer?** ⏳ *pending.*
   Key fact established: **a halt is resumable** (git-state-derived resume, `scoped.go`; committed phases are skipped, the human re-enters the human gate on resume). So "can the human proceed?" → **yes, via resume**, not via an indefinitely-blocked live TTY.
   Options:
   - **(a) idle-timeout → resumable halt + notify** *(leading candidate)* — wait a bounded idle period for an operator, push a notification, then halt cleanly and reset the working tree to the last DONE commit; human resumes later. Preserves human supervision, frees the machine, loses no committed work.
   - **(b) halt fast** — halt the instant a human gate is reached with no TTY (no wait at all). Simplest; gives the operator zero chance to step in live.
   - **(c) run headless instead** — drop human-category nodes to `claude -p` in unattended mode so automation continues unsupervised. Conflicts with the *"never bypassable"* intent the category encodes.
   - **(d) no halt — keep waiting** — current behaviour; the 2h14m stall. Rejected unless there's a reason to keep a live session parked.
   Sub-decisions if (a): **idle-timeout value** (e.g. 10 min?), **how "no operator" is detected** (no TTY vs. an explicit unattended/rehearsal flag), and **whether the notification** (push/email/none) is in scope now.
2. **Prompt fix vs routing fix (Step 3a vs 3b).** ⏳ *pending.* Is the new-behaviour green-verify *supposed* to use `unexpected-failing-tests-fixer` at all? If the design intends the system-implementer to fully green the new test, a still-red new test is "implementation incomplete," not "unexpected failure" → argues for **3b** (routing). If the fixer is meant to be the universal red-responder → **3a** (prompt branch + escape hatch). Determines whether we touch the prompt, the YAML, or both. *(I can confirm the design intent from the YAML before you choose, if useful.)*
3. **Watchdog scope.** ⏳ *pending.* Idle/stall watchdog specific to human-category dispatches, or a general per-dispatch wall-clock backstop for *all* agents? (There is currently **none** — no `timeout`/`budget`/`deadline`/`no-progress` concept anywhere in `process-flow.yaml`; only `max-visits` and the engine-wide `maxDispatchesPerProcess` *count* caps exist.)
4. **First-run infra classification.** ⏳ *pending verification.* Confirm the first `fail` was a genuine assertion failure (it was: `> Task :test FAILED` with executed tests), not an empty selection mislabeled `fail`. The pre-flight guard (Step 2) makes this moot for behaviour, but worth confirming the classifier buckets empty-selection as `infra` on the **first** run too, not only retries.
