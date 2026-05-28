# Plan: Honest rehearsal banners (subprocess exit vs. test state)

## Context

Three layers of completion banners disagree with each other and conspire to make a failed acceptance-test run look like a successful implementation:

1. **Rehearsal script footer** — `scripts/atdd-rehearsal.sh:299-302`:
   ```bash
   if [[ $RC -eq 0 ]]; then
     log "implement succeeded."
   else
     log "implement exited with rc=$RC."
   fi
   ```
   "Succeeded" means *the `gh optivem implement` subprocess returned 0*, nothing more. It does **not** mean the AT passed. It cannot distinguish a green-finish run from a red-finish run from an unverified-fix run.

2. **BPMN trace exit lines** — `internal/atdd/runtime/trace/trace.go:339-346`:
   ```go
   func outcomeStatusLabel(out statemachine.Outcome) (string, color.Attribute) {
       switch out.Value {
       case "red":   return "RED", color.FgRed
       case "infra": return "INFRA", color.FgYellow
       }
       return "OK", color.FgGreen
   }
   ```
   `OK` means *"the node did not classify itself as RED or INFRA"*. End-events have nothing to classify, so they always print `OK`. Reads as evaluative, is structural.

3. **`command-succeeded=true` on the `IMPLEMENT_TICKET` activity output** — the boolean is *"actual outcome matched expectation"*. When the upstream pinned `expected-test-result=failure` (red wrapper) and the test indeed failed, this prints `true`. The English word reads as "the command worked"; the actual semantic is "the cycle's expectation was met, regardless of what the command did."

Observed in the 2026-05-28 rehearsal (`worktrees/rehearsal-20260528-135952.log`):
- `BUILD FAILED in 14s` from Gradle (a real test failure: Jackson `BeanDeserializer` crash on `BrowseOrdersResponse`).
- `OK IMPLEMENT_TICKET -> command-exit-code=1, command-succeeded=true, …, test-outcome=fail, expected-test-result=failure, …` immediately after.
- `[atdd-rehearsal] implement succeeded.` as the footer.

The operator-visible chain reads "succeeded / OK / succeeded" while the underlying test crashed in a way that's almost certainly not the red-state the cycle intended.

Goal: make each layer's banner say *what it actually measures*, so the loud signals match the truth.

## Items

### Item 1 — Rehearsal footer: replace "succeeded" with "finished" / "crashed"

**File:** `scripts/atdd-rehearsal.sh:299-302`

Replace:

```bash
if [[ $RC -eq 0 ]]; then
  log "implement succeeded."
else
  log "implement exited with rc=$RC."
fi
```

with:

```bash
if [[ $RC -eq 0 ]]; then
  log "implement finished (rc=0). See trace for test outcome."
else
  log "implement crashed (rc=$RC). Runtime error, not a test failure — see trace."
fi
```

Rationale: RC=0 vs RC≠0 is real information (clean exit vs. runtime error), so two states are appropriate. Neither word claims anything about test state. The trailing "See trace for test outcome" points operators at the source of truth.

### Item 2 — BPMN end-event trace line: show the end-event's name

**File:** `internal/atdd/runtime/trace/trace.go`

End-events all render as `OK NODE_ID -> (no result)` regardless of which end-event they are. The end-event names in `process-flow.yaml` are already descriptive (`VERIFY_PASS_END` = "AT Passes", `FIX_FAIL_END` = "Unexpected Failing Tests Fixed", etc.) but the trace doesn't print them on the exit line.

Extend the exit-line writer to append the end-event's `name:` field when the node kind is `end-event` or `error-end-event`. Example before/after for the rehearsal trace:

```
# Before
[trace 14:22:54] > IMPLEMENT_TICKET_END  kind=end-event
[trace 14:22:54] OK IMPLEMENT_TICKET_END -> (no result)  (0s)

# After
[trace 14:22:54] > IMPLEMENT_TICKET_END  kind=end-event
[trace 14:22:54] OK IMPLEMENT_TICKET_END -> "Ticket Marked IN ACCEPTANCE"  (0s)
```

The `name` text is the BPMN-modeller's stated meaning of reaching that node; surfacing it is honest and free.

### Item 3 — IMPLEMENT_TICKET activity exit line: lead with test-outcome reality

**File:** `internal/atdd/runtime/trace/trace.go` (output formatter for call-activity outcomes)

`command-succeeded=true` reads as evaluative when it's actually a "matched expectation" flag. Today's output line for the call-activity buries the load-bearing facts (`test-outcome`, `expected-test-result`) inside a long comma-separated tail. Two changes:

1. **Prepend a derived `verdict=` chip** to the call-activity exit line, derived as:
   - `verdict=green-as-expected` when `expected-test-result=success` and `test-outcome=pass`
   - `verdict=red-as-expected` when `expected-test-result=failure` and `test-outcome=fail`
   - `verdict=unexpected-fail` when `expected-test-result=success` and `test-outcome=fail` (and no FIX downstream)
   - `verdict=unexpected-pass` when `expected-test-result=failure` and `test-outcome=pass`
   - `verdict=infra` when `test-outcome=infra`
   - `verdict=n/a` when either field is unset (non-test phases — e.g. refactor sub-processes that don't run tests)

2. **Rename the inner field** in the rendered line from `command-succeeded` to `expectation-met`. The YAML / Go binding name (`command-succeeded`) stays — only the rendered label in the trace changes, so existing gateway bindings and tests keep working.

Result for the 2026-05-28 rehearsal trace line:

```
OK IMPLEMENT_TICKET -> verdict=red-as-expected, command-exit-code=1, expectation-met=true,
  test-outcome=fail, expected-test-result=failure, …
```

An operator scanning the line sees the verdict in the first 30 characters and learns whether the cycle is in the state it was supposed to reach.

### Item 4 — Rehearsal footer: also print the verdict (depends on Item 3)

**File:** `scripts/atdd-rehearsal.sh`

After Item 3 surfaces a `verdict=` chip in the trace, the rehearsal footer can grep it out of the log and append:

```bash
if [[ $RC -eq 0 ]]; then
  VERDICT="$(grep -oE 'verdict=[a-z-]+' "$LOG_FILE" | tail -1)"
  log "implement finished (rc=0, ${VERDICT:-verdict=unknown})."
else
  log "implement crashed (rc=$RC). Runtime error, not a test failure — see trace."
fi
```

This is the only place the script learns *anything* about test state. Without Item 3 it can't (the verdict doesn't exist yet); with Item 3 it's a one-line grep.

### Item 5 — Tests

**File:** `internal/atdd/runtime/trace/trace_test.go`

- Add a case for each `verdict=` mapping in Item 3.
- Add a case for the end-event-name printing in Item 2 (cover both `end-event` and `error-end-event`).

**File:** `scripts/atdd-rehearsal.sh` test coverage — there isn't a unit test harness for the bash script today; rely on the next rehearsal run for verification.

## Out of scope

- **Restructuring the `command-succeeded` gateway.** Item 3's relabel is cosmetic at the trace layer. Renaming the binding name itself, or splitting it into `command-exit-zero` + `expectation-met`, is a larger refactor of BPMN gateway semantics — separate plan if the trace relabel proves insufficient.
- **The "re-run AT after fix" loopback.** Tracked in `plans/20260528-1447-verify-tests-rerun-after-fix-loop.md`. Honest banners don't replace the missing verification step; they complement it.
- **Color-coding the verdict chip.** Could render `unexpected-*` in red, `*-as-expected` in green — defer until the chip's text has settled.
- **Diagram regeneration.** Auto-handled by the `regenerate-diagram` GH Actions workflow on push to main.

## Verification

- `go test ./internal/atdd/runtime/trace/... -p 2` passes.
- `go test ./internal/atdd/... -p 2` passes (no incidental regressions in run_test.go fixtures that parse trace output).
- Rehearsal: `bash scripts/atdd-rehearsal.sh <issue> --config gh-optivem-monolith-java.yaml` on a ticket whose green AT is expected to pass. Confirm footer reads `implement finished (rc=0, verdict=green-as-expected).` and that a red-wrapper-only run reads `verdict=red-as-expected`.
