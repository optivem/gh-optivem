# Surface the assertion in failure capture, without losing the true error

## TL;DR

**Problem:** When an acceptance test fails (red phase or a real break), the
trace/HALT banner and the fixer-agent prompts show the *trailing* lines of the
runner output — which for Gradle/JUnit is the `BUILD FAILED` banner, the
`Deprecated Gradle features…` warnings, and JVM stream frames
(`ForEachOps`/`AbstractPipeline`). The actual assertion
(`AssertionError: Expected result to be failure but was success`) sits in the
*middle* of the output and never reaches the summary.

**Not a correctness bug — an ergonomics gap.** The true error is **never lost
on disk**: it is teed verbatim into the rehearsal `.log` (line 658 of the
2026-06-08 run) and into the JUnit XML/HTML reports. Only the *bounded summary*
in `ctx.State` misses it.

**Proposed fix (one option):** make `formatVerifyFailureOutput` *additive* —
prepend a short window anchored on the first error marker, keep the existing
tail unchanged. Adds the assertion; removes nothing.

## Background — how capture works today

`runCommand` (`internal/atdd/runtime/actions/bindings.go`) populates two
diagnostic keys on a failed command:

- `command-stderr-tail` (`bindings.go:951`) — `lastNLines(stderr, 20)`, feeds
  the HALT banner (`trace.go:298 writeInfraHaltBanner`) and the
  `command-failed-fixer` prompt.
- `verify_failure_output` (`bindings.go:977`, built by
  `formatVerifyFailureOutput` at `bindings.go:995`) —
  `lastNLines(stdout,20) + "--- stderr ---" + lastNLines(stderr,20)`, feeds the
  `unexpected-failing-tests-fixer` prompt.

`lastNLines` (`bindings.go:1020`) returns the **trailing** N lines. For a
Gradle run the trailing lines are always the build banner + deprecation
warnings, so the assertion (mid-output) is structurally excluded.

### Key safety fact (why this is low-risk to touch)

The state machine **never branches on the content** of these strings — they are
display/diagnostic only. Infra-vs-red classification, which *does* drive the
gateway downstream of `run-tests`, reads the **raw** `result.Stderr` bytes via
`classifyShellErr` (`bindings.go:973`), **not** the truncated slice. So
reshaping the summary cannot change any pipeline decision.

## The options

### Option A — Do nothing (zero risk)

Leave capture untouched. Rationale: the true error is already on disk (rehearsal
log + JUnit XML); this is only a "where do I look" ergonomics cost. No hot-path
change, no test surface.

- **Pro:** zero blast radius on a path that fires for every failing command in
  every rehearsal.
- **Con:** trace banner and fixer-agent prompts stay less informative than they
  could be; operators keep getting alarmed by the `ForEachOps` tail on an
  expected red.

### Option B — Cosmetic only: quiet Gradle chaff

Add `--warning-mode none` to the Gradle invocation in the test runner so the
`Deprecated Gradle features… Gradle 9.0` lines stop printing.

- **Pro:** removes pure noise; cannot affect any logic.
- **Con:** does **not** fix the head/tail mismatch — the assertion is still
  absent from the summary; you still get the `BUILD FAILED` banner + JVM frames.
- Note: applies to Gradle/Java only; .NET and Playwright unaffected.

### Option C — Additive head+tail capture (the real fix)

Change `formatVerifyFailureOutput` to emit **two** blocks: a head window
anchored on the first error marker, then the existing tail unchanged.

```go
func formatVerifyFailureOutput(stdout, stderr []byte) string {
    combined := string(stdout) + "\n" + string(stderr)
    head := firstErrorWindow(combined, errorMarkers, headLines) // NEW
    tail := lastNLines(combined, commandStderrTailLines)        // UNCHANGED
    switch {
    case head == "":           // no marker → identical to today
        return tail
    case overlaps(head, tail): // short output: head == tail → don't double-print
        return tail
    default:
        return head + "\n--- (later output) ---\n" + tail
    }
}

// firstErrorWindow returns the first line matching any marker plus the next
// `headLines` lines, or "" if none match.
var errorMarkers = []string{"AssertionError", "Exception", "FAILED", "panic:", "Error:"}
```

Three properties that make it *additive*, not "smarter-and-riskier":

1. **Tail unchanged** — every line surfaced today still appears (second block).
2. **No-marker → tail-only** — falls back to exactly today's behaviour; can
   never produce *less*.
3. **Head is purely extra** — can only prepend the missing assertion; never
   drops anything.

Contrast with the **rejected** variant (replace tail with head): on an infra
failure the true cause is often the *last* line with no exception marker to
anchor on, so a head-only window would drop it. That variant is explicitly out.

- **Pro:** assertion always present; improves both the trace banner **and**
  fixer-agent diagnosis (they currently reason from a Gradle banner).
  Language-agnostic (one text heuristic covers Java/Gradle, .NET, Playwright).
- **Con:** hot, shared path → broad blast radius; needs unit tests. Payload
  grows ~20 → ~35 lines (budget head and tail separately so neither truncates).
  Possible regex misfire anchoring on the wrong "Error" line (bounded: tail
  retained). Fixer prompts assume the current shape — must re-read for drift.

## Decisions

- **Q-6 (meta-plan, resolved):** Option C (additive head+tail capture) **plus** the Option B Gradle `--warning-mode none` tidy. Q1-Q4 are settled by this: act; B+C; marker set `{AssertionError, Exception, FAILED, panic:, Error:}` with a 15-line head window; the suppress-stderr plan stays independent.
- `command-stderr-tail` (HALT banner / `command-failed-fixer`) stays tail-only: the infra path's cause is usually the last line.

## Shipped

- Steps 1-5 (helpers `firstErrorWindow`, additive `formatVerifyFailureOutput` in `internal/atdd/process/actions/command.go`, unit tests in `bindings_test.go`, fixer prompts re-read: no tail-shape wording, no change needed).
- Step 6 (Option B): shop `de90fbe7` adds `--warning-mode none` to every Gradle command in `system-test/java/tests.yaml` and `tests.legacy.yaml`.
- Probe verification (2026-11-05): a throwaway failing JUnit test run with the real `acceptance-parallel-api` command from `tests.yaml`, its stdout/stderr fed through `formatVerifyFailureOutput`. The head window leads with `CaptureProbeTest > probeExpectedRed() FAILED` / `AssertionFailedError: [Expected result to be failure but was success]`, and the `Deprecated Gradle features...` block is gone (still present without the flag).

## Remaining

- Full ATDD trace check: re-run the `#76` red phase (`gh optivem implement 76 --target test` on `gh-optivem-monolith-java.yaml`) and confirm `verify_failure_output` in the trace leads with the assertion. Not run autonomously: launching the headless-claude `implement` run was refused by the auto-mode permission check, so it needs an attended run. Infra-failure tail-only path is covered by unit tests.
