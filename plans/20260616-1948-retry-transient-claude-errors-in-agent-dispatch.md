# Retry transient Claude errors in agent dispatch

## Motivation

A rehearsal run on issue #71 ("Gift-wrap an order") crashed mid-flow:

```
[agent]  enter  acceptance-test-writer  (headless)
API Error: 500 Internal server error. This is a server-side issue, usually temporary — try again in a moment.
[agent]  FAIL   acceptance-test-writer  (18s, 0 in / 0 out, $0.00)
         exit status 1
```

The `500` aborted the entire BPMN tree (`RUN_AGENT → EXECUTE_AGENT → … → IMPLEMENT_TICKET`), and the rehearsal wrapper offered to delete the worktree. Nothing was actually wrong with the ticket, the worktree, or the prompt — it was a transient server-side error that "usually temporary, try again in a moment" describes exactly. The operator's only recourse was to re-run the whole rehearsal by hand.

This is the recurring transient-failure pain: every other external-call path in the repo (git / gh / sonar / docker, via `internal/kernel/shell` and `.github/scripts/*-retry.sh`) already retries transient failures, but the **agent-dispatch path** — the single longest, most failure-prone external call in the system — has no retry at all.

## Current behaviour (verified against code)

`clauderun.Dispatch` (`internal/atdd/process/clauderun/clauderun.go:644`) calls the subprocess exactly once and returns up the stack on any non-zero exit:

```go
runResult, runErr := deps.Claude.Run(ctx, RunOpts{...})
if runErr != nil {
    writeExitBanner(...)
    if classified := classifyRunError(stderrCapture.Bytes()); classified != nil {
        return runResult, fmt.Errorf("clauderun: %s: %w", opts.Agent, classified)
    }
    return runResult, fmt.Errorf("clauderun: %s exited non-zero: %w", opts.Agent, runErr)
}
```

Two gaps compound:

1. **No retry loop.** One call; any error returns immediately and the run dies.
2. **The classifier can't even *see* transient errors.** `classifyRunError` (`clauderun.go:1300`) only matches two families:
   - `rateLimitSignatures` (`clauderun.go:1273`) — `"rate limit"`, `"too many requests"`, … → "weekly cap exhausted" message.
   - `authSignatures` (`clauderun.go:1287`) — `"not authenticated"`, … → "run `claude /login`".

   A `500 Internal server error`, `529 overloaded`, connection reset, or timeout matches **neither**, so it falls straight through to the generic `"exited non-zero"` wrapper. There is no third "transient — retry me" category.

Retry infrastructure already exists and is the consolidation target: `internal/kernel/shell/retrycore.go` exposes `RetryWithPolicy(transient, hardFail *regexp.Regexp, prefix string, fn func() (string, error))` with a canonical 4-attempt / 5s→15s→45s backoff that mirrors `optivem/actions/shared/retry-core.sh`. The agent-dispatch path was simply never wired into it.

## Why this case is safe to retry

The failed agent reported `0 in / 0 out, $0.00` — it never started, never wrote, never committed (clauderun forbids agent commits regardless). A clean re-dispatch from scratch loses nothing.

General case: the agent subprocess is headless (`claude -p`), one-shot, and never commits, so even an agent that dies after partial work can only be re-run from scratch — which is correct. The cost is re-burned tokens on the retried attempt; acceptable, and the only option for a one-shot subprocess.

## Items

### 1. Add a transient-error signature set

**File:** `internal/atdd/process/clauderun/clauderun.go` (near `rateLimitSignatures` / `authSignatures`, ~line 1273).

Add `transientSignatures` (case-insensitive substrings). **Decision (refine 2026-06-16): anchored phrases only** — match the message form the `claude` CLI actually emits on stderr, never bare numeric codes or bare network tokens. A bare `"500"` would match an agent that prints "500 units" or a `$500` price in test data; a bare `"eof"`/`"timeout"` would match a legitimately-failed agent (e.g. a test that timed out), and retrying that burns tokens with no chance of success. Anchoring keeps the set to genuine infrastructure blips.

Signatures:

- `"api error: 500"`, `"api error: 502"`, `"api error: 503"`, `"api error: 504"`, `"api error: 529"` (the rehearsal-71 log shows the literal form `API Error: 500 Internal server error`)
- `"internal server error"`
- `"overloaded"` (covers `overloaded_error`)
- `"connection reset"`, `"connection refused"`
- `"temporarily unavailable"`

Explicitly **excluded** (too ambiguous to retry safely): bare `"500"`/`"502"`/…, bare `"eof"`, bare `"timeout"`/`"timed out"`. If a real-world transient surfaces only as one of these in practice, add the *anchored* form of it then — driven by an observed log, not speculation.

### 2. Order matters: rate-limit and auth must still fail fast

In whatever classification the retry decision uses, **rate-limit and auth win over transient**. Retrying a rate-limit burns quota for nothing; retrying an auth failure can't succeed without re-login. So the precedence is: auth → rate-limit (both hard-fail, no retry) → transient (retry) → generic (no retry). Keep `classifyRunError`'s existing message-mapping behaviour for the operator-facing wrapper unchanged; the new transient category only gates the retry loop.

### 3. Wrap the dispatch call in a bounded retry

**File:** `internal/atdd/process/clauderun/clauderun.go` → `Dispatch` (line ~644).

**Decision (refine 2026-06-16): reuse `shell.RetryWithPolicy`.** One canonical retry mechanism — do not roll a second backoff loop. Fall back to a small local loop **only** if the import `internal/atdd/process/clauderun → internal/kernel/shell` turns out to introduce a cycle (shell is a kernel package, so it should be importable; confirm first). If the fallback is taken, reuse the canonical backoff schedule — do not invent a new one.

`RetryWithPolicy(transient, hardFail *regexp.Regexp, prefix string, fn func() (string, error))` maps onto this case cleanly because clauderun's classification is **substring-based, not typed** (unlike the gh path, which needs `errors.As` and therefore stays on `classifyGHError` — see the note in `retrycore.go`):

- **`hardFail` regex** = the existing rate-limit + auth signatures (`rateLimitSignatures` ∪ `authSignatures`). `RetryWithPolicy` checks `hardFail` first and returns "no retry" on a match — this *is* Item 2's precedence (auth/rate-limit win over transient), expressed as the two-regex contract. No separate ordering code needed.
- **`transient` regex** = the anchored phrases from Item 1.
- **`fn`** runs `deps.Claude.Run`, resets+captures a fresh `stderrCapture` each attempt, and returns `(stderrCapture.String(), runErr)` so both regexes classify against the captured stderr. `RunResult` is captured via closure into an outer variable so `Dispatch` can still return it (token usage from the final attempt, etc.).

Behaviour falls out of `RetryWithPolicy`:
- Retries **only** when stderr matches `transient` and not `hardFail`; generic errors match neither → no retry (preserves today's behaviour for real failures).
- Backoff: the canonical 4-attempt / 5s→15s→45s schedule, for free.
- Inter-attempt `log.Warnf` (`[clauderun] attempt N/4 failed, retrying in 5s`) comes built-in via `runWithRetryLoop` — pass an informative `prefix`.

**Open question for implementation:** the exit banner currently writes once per dispatch. Decide whether each failed attempt writes its own `[agent] FAIL … retrying` banner (clearer audit trail) or only the final outcome is bannered (less noise). Lean toward a short per-attempt "transient error, retrying (N/4)" line plus the normal final banner. Note that `RetryWithPolicy`'s own `log.Warnf` already covers the minimal "retrying in Ns" signal, so the per-attempt banner is additive polish, not load-bearing.

### 4. Tests

**File:** `internal/atdd/process/clauderun/clauderun_test.go`.

Using the existing injectable `ClaudeRunner` fake:
- Fake emits a transient signature on stderr (e.g. `API Error: 500 Internal server error` — the anchored form from Item 1) on attempt 1, success on attempt 2 → `Dispatch` returns success; assert the runner was called twice and the sleep seam was invoked.
- Fake returns transient on all attempts → `Dispatch` fails after the cap; assert attempt count == max.
- Fake returns a **rate-limit** signature → `Dispatch` fails **immediately**, runner called once (no retry); assert the existing rate-limit message still surfaces.
- Fake returns an **auth** signature → same fast-fail, called once.
- Fake returns a generic non-transient, non-rate-limit error → fails once, no retry (preserves today's behaviour for real failures).

**Sleep seam (decision, refine 2026-06-16):** because Item 3 routes through `shell.RetryWithPolicy`, the backoff sleep is `shell.sleepFn` — unexported, so clauderun's test in another package can't reach it. Add a small **exported** test seam in `internal/kernel/shell` (e.g. `SetSleepForTest(func(time.Duration)) (restore func())`, or an exported `var SleepFn`) so the clauderun test can no-op the backoff and the suite stays fast. This is a prerequisite sub-task of Item 3's "reuse `RetryWithPolicy`" decision, not optional — without it the retry-path tests either sleep for real (5s+/attempt) or can't assert the sleep fired. Keep the seam minimal and clearly test-only (doc comment), mirroring the existing `sleepFn`/`nowFn` seam convention in the codebase.

## Alternatives considered

### BPMN max-visits loop (rejected — keep in-process)

The fixer nodes already loop via the engine's generic `max-visits` / `on-max-visits` mechanism, and `Options.AttemptNumber`/`AttemptMax` + the `${attempt-block}` placeholder are already wired. The natural question is: model transient-error retry the same way, as a loop node, instead of an in-process retry.

It was rejected. The decisive constraint: **the fixer loops do not loop on errors — they loop on a graceful outcome binding.** `RUN_TESTS` writes `test-outcome`, `GATE_TESTS_OUTCOME` branches on it, the FIX node (`max-visits: 2`, `on-max-visits: FIX_LOOP_EXHAUSTED`) runs, and a back-edge returns to `RUN_TESTS`. No node in that loop ever returns a Go error. In the engine (`internal/engine/statemachine/run.go:305`), a node returning `out.Err != nil` is **fatal** — it propagates up and aborts the whole tree (exactly what the 500 did). Errors are not loopable; only outcomes are.

So a fixer-style retry would require the dispatch action to *stop returning the transient error* and instead write a `dispatch-outcome == transient-error` binding, then add a gateway + back-edge + `max-visits` + an exhausted terminal. That's strictly more work than the in-process wrap, and it has real downsides for this failure class:

- **No native backoff.** The engine routes instantly; getting the 5s→15s→45s schedule would need a dedicated sleep/wait node. `shell.RetryWithPolicy` gives backoff for free.
- **Re-approval churn.** A back-edge re-enters `APPROVE_PRE`, so the operator would re-approve the agent on every transient blip unless the loop is carefully restructured.
- **Trace/diagram pollution.** Each retry becomes a node, and the process diagram is regenerated from the YAML — so infrastructure noise (a 500) would show up as first-class ATDD workflow structure.

The division of labour: **BPMN fix-loops are for *semantic* failures** (tests still red, scope violated, command failed) where a fresh agent pass with diagnostic context might change the outcome. **A transient 500 carries zero diagnostic value** — re-running the identical prompt after a short backoff is the entire remedy, so it belongs in a localized in-process retry, not the workflow graph. The transient-error *classification* (Items 1–2) is needed regardless of which mechanism is chosen; only the *loop* lives in-process.

## Out of scope

- Retrying genuine agent failures (compile errors, scope violations, missing outputs) — those have their own BPMN fix-loops and must NOT be retried by this mechanism.
- Changing the rehearsal wrapper's "delete worktree?" prompt — that's downstream; with dispatch-level retry the prompt simply fires far less often.
- Any change to the `claude` CLI's own internal retry behaviour (not ours to change).

## Estimated effort

Half a day: signature set + retry wiring + tests. Low risk — additive, gated on a new signature category, and fully covered by the injectable-runner fakes already used in `clauderun_test.go`.
