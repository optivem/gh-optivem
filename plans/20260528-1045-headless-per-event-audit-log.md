# Headless mode: persist per-event Claude stream as JSONL audit log

## Background

Today `runHeadless` (`internal/atdd/runtime/clauderun/clauderun.go`) invokes
`claude -p --output-format json`, which buffers the entire run into a single
JSON envelope. The dispatcher extracts `result` (final answer) + `usage`
(tokens/cost) and prints `result` to stdout. The trade-off is documented at
clauderun.go:1567: **"no streaming output during the run."**

For unattended `--auto --headless` rehearsals (the rehearsal commands in
`CONTRIBUTING.md`), this leaves an audit gap: the persisted prompt
(`.gh-optivem/runs/<N>-<agent>.prompt.md`), the final result text, the
outputs JSONL, and the exit-banner cost are all kept — but **not** the
per-tool-call stream (file reads, edits, bash commands, agent
reasoning). When a rehearsal goes off the rails, the operator can read
what was asked + what was committed, but not *how the agent got there*.

This plan switches headless dispatches to `--output-format stream-json
--verbose` and tees every event line to a per-dispatch JSONL file beside
the existing `.prompt.md`. The banner / usage / result-text behaviour is
preserved by parsing the terminal `type: "result"` event from the stream
instead of decoding a single envelope.

Always-on by design — no flag. The audit log is small (a few MB for a
chatty agent), the parsing surface is bounded, and the gap it closes is
load-bearing for unattended runs.

## Scope

- `internal/atdd/runtime/clauderun/clauderun.go` — `runHeadless`,
  `parseClaudeJSON`, `RunOpts`, `Options` (plumb new `EventsLogPath`).
- `internal/atdd/runtime/clauderun/clauderun_test.go` — unit tests for the
  streaming parser + the tee-to-file behaviour. Replace any
  envelope-shape fixture with a stream-shape fixture.
- `internal/atdd/runtime/driver/driver.go` — compute the per-dispatch
  `EventsLogPath` the same way `PromptLogPath` is computed (next to
  `<N>-<agent>.prompt.md`) and pass it through to `clauderun.Options`.
- `internal/atdd/runtime/driver/driver_test.go` — adjust expectations if
  any test asserts on the runState path computation.

## Items

1. **Add `EventsLogPath` to `clauderun.Options` and `clauderun.RunOpts`.**
   Empty string = "skip the events log" (mirrors `PromptLogPath`). Wire
   it through `Dispatch` → `deps.Claude.Run` so the field reaches the
   runner the same way `OutputFilePath` does. Document the load-bearing
   contract in the field comment (next to `PromptLogPath`).

2. **Replace `parseClaudeJSON` with a streaming line parser
   `parseClaudeStreamJSON`.** Reads NDJSON (one JSON object per line).
   For each line, decode just enough to read `type`. The terminal
   `type: "result"` event carries the same `result` + `usage` +
   `total_cost_usd` fields the single envelope used to. Earlier events
   (`type: "assistant"`, `type: "user"`, `type: "system"`, etc.) are
   accumulated raw — the parser returns the final `TokenUsage` +
   `ResultText` from the result event, exactly like today. Malformed
   lines: skip silently (don't crash the audit on a CLI hiccup). Empty
   stream (CLI died before emitting result): return zero values, let
   the existing non-zero-exit error path surface.

3. **Rewrite `runHeadless` to stream + tee.** Swap the args:
   `-p <prompt> --output-format stream-json --verbose`. Replace the
   single `bytes.Buffer` stdout sink with a `io.MultiWriter` that fans
   out to (a) a `bufio.Scanner`-fronted in-memory accumulator that
   `parseClaudeStreamJSON` consumes line-by-line at the end and (b) an
   `os.File` opened at `opts.EventsLogPath` (when non-empty; create
   parent dirs as needed; non-fatal warning to `opts.Stderr` on
   open/write failure — diagnostics must not break dispatch, same
   policy as `writePromptLog`). The agent's final `result` text still
   goes to `opts.Stdout` after the run so the operator sees it inline.
   `Stdin` / `Dir` / `Env` plumbing is unchanged.

4. **Driver: compute `EventsLogPath` in the runState dispatcher
   wrapper.** Find the call site that sets `PromptLogPath` (currently
   around `driver.go:981`) and add a sibling line setting
   `EventsLogPath` to the same directory with the `.events.jsonl`
   suffix instead of `.prompt.md`. nil `rs` → empty path (mirrors the
   `PromptLogPath` test-fallback comment at driver.go:622-624 and
   :769-771).

5. **Update / add unit tests in `clauderun_test.go`.** Cover:
   (a) `parseClaudeStreamJSON` against a multi-line stream-json fixture
   ending in a `result` event — assert `Usage` + `ResultText` match;
   (b) malformed mid-stream line is skipped, terminal result still
   parses; (c) `runHeadless` writes the events file when
   `EventsLogPath` is set (use a temp dir + a fake `claude` shim, the
   same pattern existing tests already use to fake the binary); (d)
   `runHeadless` survives an unwritable `EventsLogPath` with a stderr
   warning rather than a hard error. Delete any test that asserts the
   old `--output-format json` argv shape; replace with assertions on
   the new `--output-format stream-json --verbose` shape.

6. **Update the `runHeadless` doc comment.** The current comment at
   clauderun.go:1558-1567 says "The trade-off is no streaming output
   during the run." Rewrite to describe the stream-json shape, the
   per-event JSONL audit log, and the parser's terminal-`result`
   extraction. Also update the package doc at clauderun.go:1-19 if any
   line there still implies headless is single-envelope.

## Out of scope

- Interactive mode. Continues to use the TUI; ANSI escapes make
  log-mirroring noisy, and the operator is by definition present.
- Log rotation / size capping. The `--keep-runs` flag already prunes
  the parent `runs/` directories; the per-dispatch `.events.jsonl`
  inherits that lifecycle.
- A user-facing flag to opt out of the events log. Always-on by
  design (see Background). Add a flag later only if a real use case
  surfaces.
- CONTRIBUTING.md / README.md prose pointing operators at the new file.
  Deferred until the runner change lands and the file path is final.

## Verification

- After implementation, run one rehearsal command from `CONTRIBUTING.md`
  (e.g. `bash ../gh-optivem/scripts/atdd-rehearsal.sh 65 --config
  gh-optivem-monolith-java.yaml --auto --headless`) and confirm:
  - `.gh-optivem/runs/<N>-<agent>.events.jsonl` files appear next to
    the `.prompt.md` files.
  - Each `.events.jsonl` is well-formed NDJSON terminating in a
    `type:"result"` event.
  - Exit banners still report token usage + cost identically to the
    pre-change baseline (parser regression check).
- `go test -p 2 ./internal/atdd/runtime/clauderun/...` is green.
