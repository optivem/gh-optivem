# 2026-06-19 21:22:00 UTC — Fix no-progress-guard signature normaliser (missing MM:SS.mmm suite-duration token)

## TL;DR

**Why:** The `check-fix-progress` no-progress guard never fires when it should, because `fixFailureSignature` (`internal/atdd/process/actions/fix_progress.go`) normalises away volatile tokens before comparing fix-pass signatures but **misses the `MM:SS.mmm` suite-duration token** the test runner prints in its Suite Results table (e.g. `latest - Contract (real)  FAILED  00:02.158`). That column varies run-to-run, so byte-identical failures hash to *different* signatures, the guard reports `fix-loop-progressing=true` every pass, and `FIX_LOOP_NO_PROGRESS_EXHAUSTED` never halts.
**End result:** The normaliser strips the `MM:SS(.mmm)?` suite-duration token, so a fix pass that changes nothing produces an identical signature and the no-progress guard halts on attempt 1 — tightening *every* never-green fix loop, not just the contract-escape case from #65.

## Outcomes

What we get out of this — the goals and deliverables:

- `fixFailureSignature` normalises the `MM:SS.mmm` (and `MM:SS`) suite-duration token to a stable placeholder, so two runs whose only difference is that timing column produce **identical** signatures.
- The `check-fix-progress` no-progress guard fires `FIX_LOOP_NO_PROGRESS_EXHAUSTED` on the first pass that makes no edits and yields a byte-identical failure — instead of falling through to the count-cap `FIX_LOOP_EXHAUSTED` on attempt 2.
- A `fixFailureSignature` test case pins the new behaviour: the exact run-#65 Suite Results lines (`00:02.145` / `.158` / `.259` / `.405`) collapse to one signature.
- Existing signature normalisation (ANSI, `HH:MM:SS` clock, unit-suffixed durations, whitespace) is unchanged — the new token is additive and doesn't over-normalise real failure-content differences.

## ▶ Next executable step (resume here)

Edit `internal/atdd/process/actions/fix_progress.go`: add a `suiteDurationToken` regex matching `\b\d{1,2}:\d{2}(\.\d{1,3})?\b` (the `MM:SS(.mmm)?` suite-duration format) and apply it inside `fixFailureSignature` alongside the existing `clockToken` / `durationToken` strips, ordered so it doesn't collide with the `HH:MM:SS` clock strip. Then add a `fixFailureSignature` test case in `internal/atdd/process/actions/fix_progress_test.go` feeding the run-#65 Suite Results lines and asserting the four variants collapse to one signature. Gate: `go test ./internal/atdd/process/actions/ -run FixFailureSignature` (scoped — never unbounded `go test ./...` on Windows). Unblocks: closing the #65 Layer-3b follow-up and deleting plan `20260619-1934-contract-test-system-driver-escape.md`.

## Steps

- [ ] Step 1: Read `fix_progress.go` — confirm the exact shape of `clockToken`, `durationToken`, and how `fixFailureSignature` chains the replacements, so the new token slots in without double-matching the `HH:MM:SS` clock.
- [ ] Step 2: Add a `suiteDurationToken` regex for `MM:SS(.mmm)?` and apply it in `fixFailureSignature`. Pick anchoring/ordering that strips `00:02.158` but leaves genuine `HH:MM:SS` clocks to the existing `clockToken`.
- [ ] Step 3: Add a `fixFailureSignature` test case using the run-#65 lines (`00:02.145`, `.158`, `.259`, `.405`) asserting one collapsed signature; keep an assertion that a real content difference still produces distinct signatures (guard against over-normalising).
- [ ] Step 4: Run the scoped test (`go test ./internal/atdd/process/actions/ -run FixFailureSignature`, or `-p 2`).
- [ ] Step 5 (optional): `bpmn-logic-audit` pass to confirm the no-progress guard's State/Params wiring around `check-fix-progress` is sound now that it can actually fire on attempt 1.
- [ ] Step 6: On green, delete the completed parent plan `20260619-1934-contract-test-system-driver-escape.md`.

## Open questions

- **Regex boundary risk:** `\d{1,2}:\d{2}(\.\d{1,3})?` could partially match the tail of an `HH:MM:SS` clock (`12:34:56` → the `34:56` segment). Resolve in Step 2 by ordering the `HH:MM:SS` strip first, or by anchoring on the runner's table column. Decide during encoding (executor's discretion per repo norms).
