# ❓❓❓ NEEDS DISCUSSION ❓❓❓

> 📋 **Deferred-plan review (2026-06-04): KEEP — live, unresolved decision.** Code still matches the plan's "current state" exactly: `bindings.go:154` wires `realShell{stdout: d.Out.Detail, stderr: d.Stderr}` (Option 1), the zero-value fallback at `:1345` uses `os.Stderr`, and the `realShell.Run` "stream live to the operator's terminal" comment is unchanged. The load-bearing forensics path (`command-stderr-tail` / `writeInfraHaltBanner`) still exists. Just needs VJ to pick Option 1 (keep stderr always visible) vs Option 2 (route through the level-filtered sink); Option 2's 5-step implementation is ready to execute.

**Open question (2026-05-28):** When `gh optivem implement` runs without
`--verbose`, should the subprocess **stderr** stream be routed through
`Out.Detail` (level-filtered, hidden at non-verbose) or stay wired to
`os.Stderr` (always visible)?

This determines whether `docker compose up` progress lines (`Container
… Started`, `Network … Created`, image build status) and similar
chatty-on-stderr child output appear on the operator terminal in the
default clean view, or only show up with `-v`.

VJ's initial intent: stderr always visible (stream convention), stdout
filtered. But docker compose writes its progress UI to stderr, so the
stream convention does **not** quiet the docker firehose — it only
quiets stdout, which is usually empty for compose-up. Decide which of
the two options below before executing.

**Claude's recommendation (2026-05-28):** Option 2. The POSIX
"stderr = noteworthy" convention assumes child processes use stderr
for diagnostics, but modern tools (docker compose, gradle, npm)
deliberately route progress UI to stderr so stdout stays pipeable —
so the convention no longer maps onto "what the operator wants to see
at non-verbose." The existing sink architecture already has the right
axis (`Phase` vs `Detail` = "operator-relevant" vs "firehose"), which
cuts cleaner than "stdout vs stderr." And the HALT banner's
`command-stderr-tail` path means hiding live stderr does **not** lose
forensics on failure — it defers them to the failure moment instead
of streaming byte-by-byte. The one real cost — a hung compose-up
showing nothing live on the terminal until the watchdog trips — is
covered by `--verbose` and `--log-file` as escape hatches.

**Do not execute until this is resolved.**

---

# Suppress subprocess stderr at non-verbose (or don't)

## TL;DR

**Why:** At non-verbose, `gh optivem implement` wires subprocess **stderr** straight to `os.Stderr` (bypassing the level-filtered sink), so docker-compose progress, gradle/npm banners and other chatty-on-stderr child output bury the clean BPMN trace view that the default is supposed to give the operator. An open design question blocks the fix: filter by stream (POSIX convention) or by meaning.
**End result:** A decision is recorded and applied. Option 1 — keep stderr always visible — is a no-op that just closes the plan. Option 2 — route stderr through `Out.Detail` — hides both subprocess streams at non-verbose (visible only with `-v`/`--log-file`) and relies on the HALT banner's existing `command-stderr-tail` to surface stderr on failure, so the trace becomes the primary clean operator UI.

## Background

The level-tagged sink architecture (`internal/atdd/runtime/outlog`)
defines two levels — `Phase` (headlines: BPMN trace, agent banners,
prompts) and `Detail` (firehose: subprocess byte streams, prompt-prep
banners). `gh optivem implement` defaults the **terminal sink** to
`Phase` and lifts it to `Detail` with `--verbose`
(`implement_commands.go:125-128`).

`realShell.Run` in `internal/atdd/runtime/actions/bindings.go` is the
LOW-primitive `execute-command` body that fires for every BPMN
`run-command` service-task (test runs, docker compose up/down, gradle,
`gh optivem commit`, etc.). Its current stdio wiring
(`internal/atdd/runtime/actions/bindings.go:1295-1304`):

```go
stdoutSink, stderrSink := r.stdout, r.stderr
if stdoutSink == nil {
    stdoutSink = os.Stdout
}
if stderrSink == nil {
    stderrSink = os.Stderr
}
var stdoutBuf, stderrBuf bytes.Buffer
cmd.Stdout = io.MultiWriter(stdoutSink, &stdoutBuf)
cmd.Stderr = io.MultiWriter(stderrSink, &stderrBuf)
```

Routed via `Deps.withDefaults`
(`internal/atdd/runtime/actions/bindings.go:161`):

```go
d.Shell = realShell{stdout: d.Out.Detail, stderr: d.Stderr}
```

So **stdout** goes through `Out.Detail` (level-filtered: hidden on
terminal at non-verbose) but **stderr** goes through `Deps.Stderr`
(defaults to `os.Stderr`, bypasses outlog entirely — always visible).

This asymmetry is what the operator sees during a rehearsal:
`[trace …] > RUN_COMMAND` is followed by docker compose's progress lines
on the terminal even at non-verbose. The progress lines are docker's
stderr, not its stdout.

## The two options

### Option 1 — Honor the stream convention strictly

Keep `stderr → os.Stderr` (always visible). Accept that any child that
writes progress / status to stderr (docker compose, gradle daemon
banners, npm progress bars) will leak through at non-verbose. Operator
gets a noisy compose-up regardless of `-v`.

Pro: matches the POSIX convention that stderr = noteworthy. Forensics
on a hung compose-up don't require `-v`.

Con: defeats the "clean operator view" goal of the default. The trace
banner gets buried in 20+ docker progress lines per `run-command`.

### Option 2 — Filter by meaning, not by stream

Route subprocess stderr through `Out.Detail` too. Both streams hidden
at non-verbose; both visible with `-v`. Surface failures via the
trace's existing diagnostic path: `runCommand`
(`internal/atdd/runtime/actions/bindings.go:781-787`) already stamps
`failure-kind`, `command-line`, `command-exit-code`, and
`command-stderr-tail` on failure, and `writeInfraHaltBanner`
(`internal/atdd/runtime/trace/trace.go:298`) already prints the stderr
tail in the HALT banner. So suppressing the live stderr stream does
not lose diagnostic information — it defers it to the moment something
fails, instead of streaming it byte-by-byte while the command is
running.

Pro: actually quiets the compose-up firehose. The trace becomes the
operator's primary UI; subprocess noise is on-demand via `-v` or
`--log-file`.

Con: a hung compose-up shows nothing on the terminal until the watchdog
trips. Operator either uses `-v` or watches the log file to see
liveness during long-running commands.

## Items

If **Option 1** is chosen — close this plan. The current wiring already
implements it; the only change would be to delete the asymmetry note
from `realShell.Run`'s comment block (the comment currently implies
stderr should eventually route through `Out.Detail`).

If **Option 2** is chosen:

1. Change `Deps.withDefaults` in `internal/atdd/runtime/actions/bindings.go:161`
   from `realShell{stdout: d.Out.Detail, stderr: d.Stderr}` to
   `realShell{stdout: d.Out.Detail, stderr: d.Out.Detail}`. The Detail
   sink already mirrors both streams to the same downstream writer set,
   so they end up co-mingled in the log file just as they would in a
   plain `2>&1`.

2. Update the zero-value fallback inside `realShell.Run`
   (`internal/atdd/runtime/actions/bindings.go:1299-1301`) so
   `stderrSink == nil` falls back to `stdoutSink` (matching the stream
   marriage at the level above) rather than `os.Stderr` — keeps the
   no-`Deps` path consistent with the production routing.

3. Update the comment block on `realShell.Run`
   (`internal/atdd/runtime/actions/bindings.go:1282-1294`) to drop the
   "stream live to the operator's terminal" wording — at non-verbose
   they no longer do; only `--verbose` or `--log-file` see them.

4. Update `--verbose` flag help in `implement_commands.go:157` to
   accurately enumerate what the firehose now includes (subprocess
   stdout **and** stderr, prompt-prep banners, agent body) — the
   current text already lists "subprocess output", which is fine, but
   the doctrine shift should be reflected in any doc that
   distinguishes the two streams (search for any `stderr` mentions in
   `docs/atdd/` referencing the old behavior).

5. Confirm the BPMN HALT path: `writeInfraHaltBanner`
   (`internal/atdd/runtime/trace/trace.go:298`) already prints
   `command-stderr-tail` from `ctx.State`, so an infra failure (test
   runner couldn't start, compose-up timed out) still surfaces the
   relevant stderr lines on the terminal at non-verbose. No code
   change needed — but flag in the plan that this is the load-bearing
   path that justifies hiding live stderr.

## Verification

- Run a `gh optivem implement` rehearsal without `-v` against an issue
  that triggers `docker compose up`. Confirm the trace stream contains
  no `Container … Started` / `Network … Created` lines between
  `> RUN_COMMAND` and `OK RUN_COMMAND`.
- Re-run with `-v`. Confirm both stdout and stderr of the subprocess
  stream live to the terminal.
- Force a compose-up failure (rename the compose file or break a
  Dockerfile). Confirm the HALT banner prints `stderr tail:` with the
  failing stderr content even though the live stream was hidden.
