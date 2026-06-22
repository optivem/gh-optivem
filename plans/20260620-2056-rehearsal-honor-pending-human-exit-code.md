# 2026-06-20 20:56:11 UTC — Rehearsal harness: honor the pending-human exit code (stop reporting pauses as crashes)

## TL;DR

**Why:** When an unattended rehearsal reaches a `category:human` approval gate, the binary yields cleanly with the dedicated `ExitCodePendingHuman = 2` — a *normal, expected* outcome. But the rehearsal shell harness treats every non-zero rc identically: the wrapper prints "implement crashed … Runtime error", and the loop records `FAIL (exit 2)` and hard-stops. So an intentional human-gate pause is mislabeled as a runtime crash.
**End result:** The rehearsal wrapper and loop recognize exit code `2` as "paused, awaiting operator" — reported honestly, never as a crash, never bucketed as a failure, and not a loop-stopping abort. Real crashes (rc=1) keep their existing crash messaging untouched.

## Outcomes

What we get out of this — the goals and deliverables:

- `scripts/atdd-rehearsal.sh` reports a `rc==2` run as a pending-human **pause** ("reached a human-approval gate with no operator TTY — expected for an unattended rehearsal; resume with an operator present"), distinct from both `rc==0` (finished) and `rc==1`/other (crashed / Runtime error — unchanged).
- `scripts/atdd-rehearsal-loop.sh` captures the child rc explicitly (no longer collapsing 1 and 2), records a distinct `PENDING (exit 2 — awaiting operator)` result row, does **not** count it as a failure, and does **not** hard-stop the loop on a pause.
- The loop's `print_summary` / final exit handling no longer lumps pending-human runs into "Some ticket(s) failed."
- The magic number `2` is expressed as a clearly-named, commented constant referencing `internal/atdd/runtime/driver/driver.go` `ExitCodePendingHuman`, so the linkage is discoverable.
- Intentional behavior is preserved exactly: the `category:human` fixer gate and the pend-on-non-TTY yield are untouched. No BPMN or agent changes.

## ▶ Next executable step (resume here)

Edit `scripts/atdd-rehearsal.sh` around lines 388–398: introduce a named constant for the pending-human exit code (e.g. `EXIT_PENDING_HUMAN=2`, commented to reference `internal/atdd/runtime/driver/driver.go` `ExitCodePendingHuman`), then replace the binary `if [[ $RC -eq 0 ]]; then … else "crashed" …` with a three-way branch: `0` → finished (existing message), `2` → paused/awaiting-operator (new message), else → crashed / Runtime error (existing message). Stop there for review before touching the loop script.

## Steps

- [ ] **Step 1 — Wrapper three-way branch (`scripts/atdd-rehearsal.sh:388-398`).** Add `EXIT_PENDING_HUMAN=2` (named + commented, referencing `driver.go` `ExitCodePendingHuman`). Replace the `rc==0 / else` split with: `rc==0` → existing "implement finished (rc=0, verdict=…)"; `rc==EXIT_PENDING_HUMAN` → new "implement paused (rc=2): reached a human-approval gate with no operator TTY — expected for an unattended rehearsal; resume with an operator present. See trace."; any other non-zero → existing "implement crashed (rc=$RC). Runtime error, not a test failure — see trace." Leave the `Log file:` line and the cleanup trap behavior as-is.
- [ ] **Step 2 — Loop: classify exit 2 as PENDING, not FAIL (`scripts/atdd-rehearsal-loop.sh:251-265`).** Replace the `if bash "$REHEARSAL" …; then PASS; else rc=$?; FAIL` shape with an explicit rc capture (run the command, then `rc=$?`) so `1` and `2` are distinguishable. On `rc==0` → `#N PASS`. On `rc==2` → `#N PENDING (exit 2 — awaiting operator)`, do not set the failure exit state, and continue to the next ticket (do **not** hard-stop). On any other non-zero → existing FAIL behavior (record `FAIL (exit $rc)`, stop unless `--continue-on-failure`).
- [ ] **Step 3 — Loop summary + final exit (`scripts/atdd-rehearsal-loop.sh:268-274`).** Ensure `print_summary` shows PENDING rows distinctly and the closing message/exit code does not treat pending-human runs as failures ("Some ticket(s) failed" must not fire for a run whose only non-pass outcomes were pauses). Decide the loop's final exit code when the only non-pass outcomes are pauses (see Open questions).
- [ ] **Step 4 — Sanity check.** Re-read both scripts end-to-end to confirm: real crashes (rc=1) are unchanged; the pending-human path is reachable and labeled; no other call site assumed the old binary success/fail split. Shell-lint if a linter is configured.

## Open questions

- **Loop final exit code on a pure-pause run.** When the loop finishes and the only non-PASS outcomes were pending-human pauses (no real failures), what should the loop's overall exit code be? Options: (a) `0` — nothing failed, pauses are expected; (b) a dedicated non-zero (e.g. propagate `2`) so an outer CI caller can see "some runs are awaiting a human." Recommendation: **(a) exit 0** for "no failures," but surface the pause count loudly in the summary — unless you want an outer automation layer to act on pending runs, in which case (b). *Which do you want?*
- **Default loop continuation on pause.** The plan assumes a pause should **continue** to the next ticket by default (a pause isn't a failure). Confirm that's right, vs. stopping the loop on the first pause with an honest "awaiting human" message. (`--continue-on-failure` governs *failures*; pauses are proposed to continue regardless of that flag.)
