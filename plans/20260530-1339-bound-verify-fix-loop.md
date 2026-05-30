# Bound the verify→fix loop (semantic fix-attempt cap)

> **Status: refined (2026-05-30).** Spun off from
> [[20260530-1313-atdd-fixer-agents-audit]] (finding Q2, its
> highest-severity finding). That audit is prompt-side only; this work is
> harness-side and was deliberately kept out of it
> ([[feedback_new_plan_not_extend]]). Design questions resolved below —
> ready to execute.

## Problem

> ⚠️ **Premise correction (from the original skeleton).** The skeleton
> claimed the loop has **"no max-iteration guard."** That is wrong. The
> engine already has one: `maxDispatchesPerProcess = 10000` in
> `internal/atdd/runtime/statemachine/run.go` (the `runProcess` loop
> fails fast once a single process invocation exceeds it). Its doc
> comment names the *exact* 20GB-RAM loopback hazard this plan was filed
> against. So the catastrophic-RAM / infinite-hang case is **already
> mitigated engine-wide.** This plan is no longer "add the missing guard"
> — it is "add a *semantic* cap underneath the existing catastrophe
> backstop."

The `verify-tests-pass` and `verify-tests-fail` subprocesses route a
failed verification into a fixer and loop straight back to re-run the
tests:

- `process-flow.yaml:1256` — `{from: FIX_UNEXPECTED_FAILING_TESTS, to: RUN_TESTS}` (in `verify-tests-pass`, lines 1216–1256)
- `process-flow.yaml:1300` — `{from: FIX_UNEXPECTED_PASSING_TESTS, to: RUN_TESTS}` (in `verify-tests-fail`, lines 1260–1300)

The real gap is the **granularity** of the only existing guard:

- `maxDispatchesPerProcess` is a **catastrophe backstop**, set "orders of
  magnitude above any legitimate single-process trail" (its own comment).
  At ~3 dispatches per lap (`RUN_TESTS → GATE_TESTS_OUTCOME → FIX_* →`
  back) it tolerates **~3300 fix attempts** before tripping.
- Both fixers run at **opus · high** — the most expensive tuning in the
  tree. A fixer that repeatedly mis-picks which side to fix (SUT vs.
  test) would burn **thousands of opus·high passes** before the backstop
  ever fires. Financially that is the catastrophe, even though the RAM
  one is contained.
- Unlike the stuck-gate loops the backstop targets, this loop **does
  reset its deciding state each lap**: the fixer edits code, `RUN_TESTS`
  re-runs, `GATE_TESTS_OUTCOME` re-classifies fresh. So it is a
  "fixer fails to converge" loop, not a "gate never changes" loop —
  invisible to any guard that only catches non-progressing loops.

Per [[feedback_statemachine_test_loop_hazard]] the test-suite RAM hazard
is real but is the case the existing backstop already bounds; the new,
unbounded-in-practice risk is **cost**, not RAM.

## Goal

After **N = 2** failed fix attempts on the same verification, stop
looping and route to a human-visible halt instead of dispatching a third
opus·high fixer pass — layered **under** the existing
`maxDispatchesPerProcess` backstop, not replacing it.

## Resolved design decisions

> Resolved in one pass, best long-term + performance, per
> [[feedback_autonomous_best_long_term]]. Rationale recorded inline so
> execution doesn't re-litigate.

### D1 — Where the counter lives → **generic per-node visit cap in the engine, declared in YAML**

Add a per-node, declarative cap to the engine, mirroring the existing
`maxDispatchesPerProcess` pattern (engine-level, generic, one place):

- A node may declare `max-visits: N` and `on-max-visits: <target-node>`
  in its YAML record (new `RawNode` fields).
- `runProcess` tracks a per-invocation `visits map[string]int` keyed by
  node ID (it already tracks total `dispatches`). **Before** dispatching
  a node whose visit count would exceed `max-visits`, it routes to
  `on-max-visits` instead of executing the node body — so the
  **(N+1)th** fixer pass is never spent.

**Why this over the alternatives:**

- vs. a **per-loop counter in `ctx.State`** (a service-task that
  increments + a gateway that reads it, authored per loop): that
  duplicates machinery across every loop and re-introduces the *exact*
  "deciding state not reset between iterations" bug class the
  `maxDispatchesPerProcess` comment warns about (you must hand-place the
  increment and a reset). The engine-tracked count cannot be mis-reset.
- vs. a **bespoke `FIX_* → RUN_*` back-edge guard**: too narrow. The
  same `max-visits` attribute covers the two verify loops **and** every
  other legitimate loopback the engine already carries (the run.go
  comment lists `STOP_FLAG_UNSET → AT_RED_DSL`,
  `STOP_SCOPE_VIOLATION → WRITE`) **and** the scope-exception loop the
  audit flagged ([[20260530-1313-atdd-fixer-agents-audit]] Q1) — one
  mechanism, no new bespoke field per loop. This is the "narrowest
  mechanism that covers both loops" the skeleton asked for, resolved in
  favour of *generic-but-declarative*.
- **Routing stays in the edge/attribute layer, not inside a NodeFn** —
  consistent with the engine's stated philosophy ("Routing decisions
  live in the edge list, not inside NodeFn", `types.go`). No
  predicate-language change is needed (the cap is an engine attribute,
  not a `when:` expression), so the `>=` numeric-comparator gap in the
  predicate evaluator is avoided entirely.
- **Performance:** one `map[string]int` increment + compare per
  dispatch — O(1), negligible next to a subprocess dispatch.

### D2 — N → **2**

A must-fail test still green after 2 opus·high fix attempts (or a
behaviour-preserving WRITE still red after 2) is almost certainly a
genuine SUT/spec disagreement a human must adjudicate — a third opus
pass rarely resolves what two could not. Encoded as `max-visits: 2` on
the `FIX_*` node, so the policy is visible in the YAML **next to the
loop it governs** (attempt 1, attempt 2, then halt before attempt 3).

### D3 — Halt target → **reuse the existing `error-end-event` mechanism**

Add one `FIX_LOOP_EXHAUSTED` `error-end-event` per verify subprocess and
point `on-max-visits` at it. This reuses the path `runProcess` already
has for `ErrorEndEvent` (terminates the process with a descriptive error
naming the process + node `name:`) — the **same mechanism** the two
subprocesses already use for `TESTS_INFRA_HALT` / `UNKNOWN_TESTS_OUTCOME`.
No new halt machinery, no new node kind.

Escalation context (which fixer, attempt count, last verify tail) rides
`ctx.State` — already recorded by the trace and already populated for the
fix prompt — surfaced through the `error-end-event`'s `name:` /
diagnostic. (Confirm during execution that the verify tail is still in
`ctx.State` at the halt and isn't `Unset` on the prior lap.)

### D4 — Test-suite safety → **backstop-protected fixture, asserts the semantic cap**

The proving fixture is **inherently bounded** by the existing
`maxDispatchesPerProcess = 10000`, so even a mis-authored `max-visits`
cannot 20GB the suite. The fixture must assert the run terminates at
`FIX_LOOP_EXHAUSTED` within **N+1 `FIX_*` dispatches**, *not* via the
10000 backstop (asserting on the specific halt proves the new cap, not
the old one). Still audit the fixture before running and kill on memory
climb per [[feedback_statemachine_test_loop_hazard]].

## Items

- [ ] **Item 1: Add the `max-visits` / `on-max-visits` engine capability.**
  New `RawNode` fields (`load.go` / `types.go`); `runProcess` tracks a
  per-invocation per-node visit count and routes to `on-max-visits`
  before dispatching the over-cap node. Layered under
  `maxDispatchesPerProcess` (the backstop stays). Unit-test the engine
  capability in isolation (a tiny two-node loop fixture) before wiring
  the real flow. **Content change — list files touched and gate for
  review before commit** ([[feedback_renames_autonomous_content_gated]],
  [[feedback_no_commit_without_approval]]).

- [ ] **Item 2: Apply the cap to both verify subprocesses
  (`process-flow.yaml`).**
  On `FIX_UNEXPECTED_FAILING_TESTS` (verify-tests-pass) and
  `FIX_UNEXPECTED_PASSING_TESTS` (verify-tests-fail): add
  `max-visits: 2` and `on-max-visits: FIX_LOOP_EXHAUSTED`; add a
  `FIX_LOOP_EXHAUSTED` `error-end-event` node to each subprocess with a
  `name:` that tells the human what to adjudicate. Leave the
  `FIX_* → RUN_TESTS` back-edges in place (the cap intercepts before
  re-dispatch). **No diagram regeneration step** — the
  regenerate-diagram workflow owns `docs/process-diagram.md` +
  `docs/images/*.svg` on push to main ([[feedback_plans_no_diagram_regen]]).

- [ ] **Item 3: Confirm escalation context survives to the halt.**
  Verify the last verify tail + attempt count are present in `ctx.State`
  at `FIX_LOOP_EXHAUSTED` (not `Unset` on the success/clear path of the
  prior lap), and that the `error-end-event` diagnostic surfaces fixer
  identity. If the tail is cleared too early, stash it under a
  halt-dedicated key rather than widening the fixer's output contract
  ([[feedback_parse_full_refine_narrow]]).

- [ ] **Item 4: Statemachine fixture that proves the cap.**
  A fixture whose `FIX_*` node would re-dispatch forever under today's
  flow and that terminates at `FIX_LOOP_EXHAUSTED` within N+1 `FIX_*`
  dispatches under the guard. Assert on the specific halt, not the 10000
  backstop. Audit for the loop hazard and kill on memory climb before
  running ([[feedback_statemachine_test_loop_hazard]]); never run
  unbounded `go test ./...` ([[feedback_go_test_windows]] — use `-p 2`
  or scope to the `statemachine` package).

## Verification

- Engine unit test for `max-visits` routing (Item 1) and the
  flow-level fixture (Item 4) pass under
  `go test ./internal/atdd/runtime/statemachine/... -p 2`.
- `process-flow.yaml` still loads/binds cleanly (the existing
  load/transitions tests) after adding the two `error-end-event` nodes.

## Out of scope

- **Generalising the cap to the other loopbacks** (`STOP_FLAG_UNSET →
  AT_RED_DSL`, `STOP_SCOPE_VIOLATION → WRITE`, the scope-exception loop).
  The `max-visits` mechanism is built generic so they *can* adopt it,
  but wiring caps onto loops outside the two verify subprocesses is a
  separate decision — do not retrofit them here.
- **Model/effort tuning of the fixers** (whether the fix loop should run
  cheaper than opus·high) — owned by
  [[20260530-1313-atdd-fixer-agents-audit]] Item 7 (T5).

## Cross-references

- Source finding: [[20260530-1313-atdd-fixer-agents-audit]] Q2 / Item 6.
- Related hazard: [[feedback_statemachine_test_loop_hazard]].
- Plan-authoring conventions: [[feedback_new_plan_not_extend]],
  [[feedback_plans_no_diagram_regen]], [[feedback_autonomous_best_long_term]].
