# ❓❓❓ NEEDS DISCUSSION — NO DECISION YET ❓❓❓

> **Status (2026-06-08):** Discussion only. The problem is confirmed real,
> but VJ has **not** decided whether to act, or which option. Three live
> choices are on the table (do-nothing / cosmetic-only / additive capture).
> **Do not implement until VJ picks an option.**

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

## Open questions (for VJ)

1. **Act at all, or accept Option A?** Is the diagnostic/ergonomics gain worth
   touching a path that runs on every failing command?
2. **If acting: B, C, or B+C?** B is cosmetic-safe; C is the substantive fix;
   they compose (do C, fold in B as a one-line tidy).
3. **Marker set & window size** — is `{AssertionError, Exception, FAILED,
   panic:, Error:}` the right anchor set across all three SUT languages, and is
   ~15 head lines enough for a stack-trace head without bloating prompts?
4. **Relationship to the suppress-stderr plan**
   (`plans/backlog/20260528-1302-suppress-subprocess-stderr-non-verbose.md`) —
   different concern (live streaming vs. failure capture), but both touch the
   stderr story; confirm they stay independent.

## Items (only if Option C is chosen — DO NOT START YET)

1. Add `firstErrorWindow(s, markers, n)` and `overlaps(head, tail)` helpers in
   `internal/atdd/runtime/actions/bindings.go` near `lastNLines`.
2. Rewrite `formatVerifyFailureOutput` (`bindings.go:995`) to the additive shape
   above. Budget head and tail line caps separately.
3. Decide whether `command-stderr-tail` (`bindings.go:951`, HALT banner path)
   gets the same additive treatment or stays tail-only — the HALT banner is the
   infra path, where the tail is usually the right thing.
4. Unit tests: marker-found (assertion surfaces), no-marker (tail-only, byte
   identical to today), short-output overlap (no double-print), infra-style
   failure (tail still carries the cause).
5. Re-read and, if needed, adjust the fixer prompts that consume the payload:
   `internal/assets/runtime/agents/atdd/command-failed-fixer.md` and
   `internal/assets/runtime/agents/atdd/unexpected-failing-tests-fixer.md`
   (wording currently assumes the tail shape).
6. (Optional, Option B) Add `--warning-mode none` to the Gradle test invocation.

## Verification (only if Option C is chosen)

- Re-run the `#76` cancellation-blackout rehearsal red phase; confirm
  `verify_failure_output` in the trace leads with
  `AssertionError: Expected result to be failure but was success`.
- Force an infra failure (break the Gradle build / rename the test source) and
  confirm the tail still carries the real cause (no marker → tail-only path).
- Confirm `classifyShellErr` infra-vs-red verdicts are unchanged (it reads raw
  stderr, so they must be) — existing `verify_classify_test.go` stays green.
