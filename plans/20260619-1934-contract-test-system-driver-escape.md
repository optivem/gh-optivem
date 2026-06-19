# 2026-06-19 19:34:00 UTC — Prevent contract tests from escaping into the system-under-test

🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-06-19T19:51:26Z`

## TL;DR

**Why:** Rehearsal #65 burned ~16 min on an unwinnable fix loop because the `contract-test-writer` authored an ERP contract test that drove the *my-shop System Driver* (`.when().viewProductList()`) instead of the *external (ERP) driver port*. The call hit an unimplemented `TODO: System Driver` stub that **no contract-phase agent is scoped to implement**, so the test could never go green.
**End result:** Contract tests are authored given→then against the external-system driver port only — never via `.when()` system-under-test action chains — and if one ever escapes again, the pipeline fails it fast with a clear adjudication message instead of spending a full fix loop.

## Outcomes

What we get out of this — the goals and deliverables:

- The `contract-test-writer` agent never authors a contract test that uses a `.when().<systemAction>()` chain (or otherwise routes through the my-shop System Driver). Contract tests are given→then against the external-system driver port only. **(Confirmed by user: `.when()` is never legitimate in a contract test.)**
- A contract test that *does* escape into the system-under-test is caught **fast** — halted on first failure with a message that names the violation ("contract test routed through the my-shop System Driver; contract tests must exercise only the external-system driver port") — instead of consuming the 2-attempt fix loop (~16 min).
- The adjudication message a human sees on halt points at the real cause (test shape), not a generic "tests still red".
- **Scope:** this plan ships **Layer 1 + Layer 2**. A separate no-progress-guard bug surfaced while diagnosing this run (see Follow-up) is spun out into its own investigation, not bundled here.

## ▶ Next executable step (resume here)

**Layer 1 + Layer 2 are implemented, tested, and committed — this plan's scope is complete.** The only remaining work is the **Follow-up (3b) no-progress-guard bug**, which is *out of scope for this plan* and is a fresh design task, not a mechanical edit here. To pursue it: `/create-plan` a new plan for the `check-fix-progress` signature-normalization defect (the `fixFailureSignature` normaliser in `internal/atdd/process/actions/fix_progress.go` misses the `MM:SS.mmm` suite-duration token, e.g. `FAILED 00:02.158`, so byte-identical failures get different signatures and the guard never fires). Otherwise this file can be deleted.

## Follow-up (separate plan — out of scope here)

- **No-progress-guard bug.** In run #65 `check-fix-progress` reported `fix-loop-progressing=true` even though the fixer made **no edits** and the failure signature was **byte-identical** across both passes — so `FIX_LOOP_NO_PROGRESS_EXHAUSTED` never fired and the count-cap `FIX_LOOP_EXHAUSTED` caught it on attempt 2 instead. The no-progress guard exists precisely to halt on attempt 1 when a fix pass changes nothing, so this looks like a standalone defect in `check-fix-progress` (`internal/atdd/process/`) with **broad benefit** — it would tighten *every* never-green fix loop, not just this contract case. Worth its own `/create-plan` + likely a `bpmn-logic-audit` pass. (This is the "Layer 3b" we split out; the scope-aware-halt idea, "3a", was dropped as redundant with Layer 2.)

  **Root cause confirmed (2026-06-19):** `fixFailureSignature` in `internal/atdd/process/actions/fix_progress.go` normalises away volatile tokens (ANSI, `HH:MM:SS`, unit-suffixed durations, whitespace) before comparing signatures, but it **misses the `MM:SS.mmm` suite-duration format** the test runner prints in its Suite Results table (e.g. `latest - Contract (real)  FAILED  00:02.158`). `clockToken` requires three colon-groups (`\d{1,2}:\d{2}:\d{2}`) so it doesn't match `00:02.158`, and `durationToken` requires a unit suffix. In run #65 that column varied across the three runs (`00:02.145` / `.158` / `.259` / `.405`), so the "identical failure" never compared equal → guard reported `progressing=true` every pass. Fix = extend the normaliser to strip the `MM:SS(.mmm)?` suite-duration token (add a regex + a `fixFailureSignature` test case).

## Decisions log

- **Layer scope:** ship **Layer 1 + Layer 2**. Layer 1 is the root-cause fix; Layer 2 is a cheap fail-fast that contains the blast radius regardless of which agent misbehaves. (Resolved 2026-06-19.)
- **Layer 3:** **3a (scope-aware halt) dropped** — redundant with Layer 2. **3b (no-progress-guard bug) spun out** to its own plan (see Follow-up). (Resolved 2026-06-19.)
- **`.when()` in contract tests:** confirmed by user — **never legitimate**. Contract tests are given→then against the external port only. Layer 1 is therefore a flat prohibition. (Resolved 2026-06-19.)
- **Marker cross-language:** `TODO: System Driver` is byte-identical across all three stacks → Layer 2 is a single substring match. (Verified 2026-06-19.)
