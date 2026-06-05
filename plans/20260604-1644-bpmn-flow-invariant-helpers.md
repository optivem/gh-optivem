# BPMN flow invariant helpers

## TL;DR

**Why:** Cross-process invariants in the statemachine graph are asserted ad hoc, one named site at a time — so adding a new `commit` / `FIX` / halt site that breaks the rule slips through, because the tests enumerate *known* sites rather than quantifying over *all* of them.
**End result:** A small set of plain Go invariant helpers (`invariants.go`) over the loaded `*Engine` graph, each quantifying over every matching node, plus a test (`invariants_test.go`) that runs them against the embedded snapshot — so a new rule-violating site fails the suite with no test edit.

**Status:** Executed — helpers landed (commit 3cc54a1) and Item 3 reconciliation done 2026-06-05
**Created:** 2026-06-04 16:44 CEDT

> Supersedes **Direction B** of the discussion doc
> `plans/backlog/20260525-1418-structured-bpmn-execution-trace.md` (per
> `feedback_new_plan_not_extend` — fresh plan, the backlog doc is retired once
> **both** this plan **and** the readable-trace plan
> `plans/20260604-1632-readable-execution-flow-trace.md` (which supersedes that
> doc's Direction A) have landed). The backlog doc's three locked decisions were
> A (ship a structured trace), B (ship invariant **helpers**, not a DSL), and C
> (reject spy state-capture). The readable-trace plan carries A; this plan carries
> **B and the C rejection**. Once both land, nothing in the backlog doc is
> un-captured and it can be deleted.

## Re-aim note (read first)

The backlog doc settled Direction B on **plain Go helpers over a `[]DispatchEvent`
slice** — a runtime spy that captured one event per dispatch (`dispatch_spy_test.go`
/ `dispatch_expect_test.go`), so invariants could be checked against a captured run.

**That substrate no longer exists.** The BPMN five-level refactor
(`plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md` Item 3) deleted the spy
and its expected-event lists wholesale; the top-of-file comment in
`internal/atdd/runtime/statemachine/transitions_test.go` (lines 4-11) records this.
Today the statemachine is tested by **static graph assertions** over the loaded
`*Engine` / `*Process` — `wantEdge` / `notEdge` over `proc.OutgoingByNode`, plus
structural-integrity sweeps over `eng.Processes`.

This plan therefore **re-aims B onto the substrate that actually exists**, exactly
the way the readable-trace plan re-aimed A:

- **Now-buildable half — graph-level invariant helpers.** B's flagship invariant
  ("every `commit` is preceded by a successful verify in the same scope") turns out
  to be a *pure static-graph property* under the new shape. The four `commit`
  call-sites — `COMMIT_TEST_CODE`, `COMMIT_SYSTEM`, `COMMIT_TESTS`, `COMMIT_LAYER`
  — are each reached only via inbound edges from a `verify-tests-*` call-activity
  (`process-flow.yaml` lines 884-885, 1115, 1169, 1243). So the invariant is
  checkable today against the embedded snapshot, no runtime capture needed. This is
  the half this plan builds.
- **Deferred half — runtime-event invariants.** Asserting over what a *real run*
  actually dispatched (B's original "over a captured trail" intent) needs a
  captured event slice. The readable-trace plan introduces exactly that seam — a
  shared per-dispatch `Event` record (its Item 1 / D2). Runtime-level invariant
  helpers are **deferred** until that seam lands a reusable `[]Event`; building
  them now has no substrate. The graph helpers built here keep their *vocabulary*
  (precedes / reaches / loops-back / terminal-kind) so the runtime form can mirror
  it later.

This preserves B's locked decision — **plain helpers, no fluent DSL** — and B's
diagnosis (the recurring "new call-site forgot the upstream gate" regression),
while honestly retargeting the form to the post-refactor codebase.

## Problem

The statemachine's cross-process invariants are asserted **ad hoc, per-site**.
`transitions_test.go` already hand-checks several of them one process at a time:

- `TestFixDispatch_LoopsBackToOriginatingStep` — each `FIX` dispatch loops back to
  its originating step (4 sites spelled out by hand).
- `TestFixDispatch_LoopsAreBounded` / `TestVerifyTests_InfraOutcomeRoutesToHalt` /
  `TestExecuteAgent_ScopeExceptionRoutesToStopViolation` — deliberate-halt
  terminals (`*_EXHAUSTED`, `*_INFRA_HALT`, `*_REJECTED_END`, `STOP_*`) must be
  `ErrorEndEvent`, not soft `EndEvent` — re-spelled per process.

The recurring failure mode the backlog doc named is exactly the gap this leaves:
**add a new call-site and forget the rule that should hold at it.** A fifth
`commit` node wired without an upstream verify, a new `FIX` loop that routes to a
soft end-event, a new bounded loop whose exhausted-terminal is an `EndEvent` — none
of these is caught, because the assertion enumerates *known* sites rather than
quantifying over *all* sites. The check is "these four are right," not "every one
is right."

## Goal

A small set of **plain Go invariant helpers** over the loaded `*Engine` /
`*Process` graph, each quantifying over **every** matching site, plus a test that
runs them against the embedded snapshot. Adding a new `commit` / `FIX` / halt site
that violates the rule then fails the suite **without anyone editing the test** —
the helper already covers the new site because it quantifies over the graph.

No fluent builder, no DSL, no YAML-lint subsystem — a handful of exported functions
over the types that already exist (`Engine`, `Process`, `Node`, `Edge`,
`OutgoingByNode`, `NodeKind`).

## Decisions (locked)

- **D1 — Plain helpers, not a DSL (carried from backlog Direction B).** Exported
  Go funcs over `*Engine` / `*Process`. No `EveryCallTo(...).IsPrecededBy(...)`
  chain syntax. The "easy to over-engineer" con the backlog doc named stands.
- **D2 — Graph-level now; runtime-event level deferred (re-aim).** Build the
  static-graph helpers against the loaded snapshot. The runtime-`[]Event` form is
  deferred behind the readable-trace plan's `Event` seam (its D2); this plan does
  **not** introduce a runtime capture.
- **D3 — Quantify over all sites, not a named list.** Each helper walks
  `eng.Processes` and checks every matching node. The value is catching the
  *unenumerated* new site; a helper that takes a hand-list of sites would just
  restate today's ad-hoc tests.
- **D4 — Non-test package, so the vocabulary is reusable.** Helpers live in a
  non-`_test` file (`invariants.go`) returning structured violations, not calling
  `t.Errorf`. The test file (`invariants_test.go`) asserts "no violations" against
  the embedded snapshot. Non-test placement is deliberate: it lets the deferred
  runtime-event checker reuse the same precedence/terminal vocabulary later (the
  backlog doc's "callable from both captured and static sources" intent, retargeted
  to graph + future-event rather than spy + JSONL).
- **D5 — Direction C stays rejected.** No spy state-capture knob, no `StateAware`
  per-node opt-in. The orchestration/action test split stays load-bearing. This
  plan records the rejection so it survives the backlog doc's deletion; it adds
  nothing for C.
- **D6 — Three invariants first (carry-forward enumeration).** Seed the helper set
  with exactly the three below — each generalizes an assertion that exists ad hoc
  today, so each is grounded and immediately earns its weight, and the three span
  distinct shapes (precedence, loop-back, terminal-kind) to stress the helper API
  before more are added.

## The three seed invariants (D6)

1. **`commit` is verified.** Every `CallActivity` whose `process == "commit"` is
   reachable only through a `verify-tests-*` call-activity — i.e. every inbound
   edge chain (through non-dispatching gateway/intermediate nodes) traces back to a
   `verify-tests-pass` or `verify-tests-fail` call-activity within the same process.
   Grounds on `COMMIT_TEST_CODE` / `COMMIT_SYSTEM` / `COMMIT_TESTS` / `COMMIT_LAYER`.
2. **`FIX` loops back.** Every `FIX` (and `FIX_UNEXPECTED_*`) node has an outgoing
   edge back to the step that originates its failure within the same process — the
   generalization of `TestFixDispatch_LoopsBackToOriginatingStep` from a 4-row table
   to a quantifier over all fix sites.
3. **Deliberate-halt terminals are `ErrorEndEvent`.** Every terminal whose id marks
   an intentional halt (`*_EXHAUSTED`, `*_INFRA_HALT`, `STOP_*`) has
   `Kind == ErrorEndEvent`, never `EndEvent` — generalizing the per-process
   checks scattered across `TestFixDispatch_LoopsAreBounded`,
   `TestVerifyTests_InfraOutcomeRoutesToHalt`, and
   `TestExecuteAgent_ScopeExceptionRoutesToStopViolation`.
   > **Resolved during execution:** `*_REJECTED_END` was dropped from the halt
   > markers. Rejection is bimodal in the snapshot — `EXECUTE_AGENT_REJECTED_END`
   > / `EXECUTE_COMMAND_REJECTED_END` are *deliberately* soft PRE-rejection skips
   > (no artifact produced), while `FIX_REJECTED_END` is a hard halt — so the
   > suffix is not a reliable halt signal and the literal pattern would have made
   > the test red against a correct snapshot. `FIX_REJECTED_END`'s error-end kind
   > stays covered by `TestFixDispatch_LoopsAreBounded`.

## Concurrency note

Active uncommitted work on
`plans/20260527-1147-dsl-implementer-ct-system-driver-scope.md` touches
`internal/atdd/runtime/statemachine/process-flow.yaml`,
`statemachine/transitions_test.go`, `actions/bindings.go`, `diagram/diagram.go`.
This plan **adds** `invariants.go` + `invariants_test.go` (new files — low
collision) and only **reads** `process-flow.yaml`. Item 3's optional cleanup
(folding the now-redundant ad-hoc assertions out of `transitions_test.go`) is the
one place that edits a contended file — keep it deferred / coordinate, and re-check
`git log` before committing per `feedback_concurrent_agent_collision`. Items 1 and
2 do not collide.

## Items

### Item 3 — Reconcile with the ad-hoc assertions — ✅ Done (2026-06-05)
- [x] Plan 1147 has landed (its plan file is gone, its `TestSharedContract_*` test
      is committed) so the contention is resolved; `transitions_test.go` was clean.
      Per-assertion decisions taken:
      - **`TestVerifyTests_InfraOutcomeRoutesToHalt`** — removed the
        `TESTS_INFRA_HALT` `ErrorEndEvent`-kind check (now covered by
        `halt-terminals-are-error-end`, `_INFRA_HALT` marker); **kept** the
        process-specific routing edge `GATE_TESTS_OUTCOME → TESTS_INFRA_HALT`.
      - **`TestExecuteAgent_ScopeExceptionRoutesToStopViolation`** — removed the
        `STOP_SCOPE_VIOLATION` kind check (covered by the rule, `STOP_` marker);
        **kept** the gate node + binding and validation routing edges.
      - **`TestFixDispatch_LoopsAreBounded`** — removed the `COMMAND_FIX_EXHAUSTED`
        / `AGENT_FIX_EXHAUSTED` kind checks (covered, `_EXHAUSTED` marker); **kept**
        `max-visits == 2`, `on-max-visits` target, edges, and the
        **`FIX_REJECTED_END` kind check** (the rule deliberately *excludes*
        `_REJECTED_END`, so this is not redundant).
      - **`TestFixDispatch_LoopsBackToOriginatingStep`** — **kept in full** as the
        focused regression that pins the *exact* origin edge per site; the
        reachability-based `fix-loops-back` rule only asserts a loopback exists.
      Each trimmed site left a breadcrumb comment pointing at the quantified rule.
      Package suite green (`go test ./internal/atdd/runtime/statemachine/ -p 2`).

## Out of scope / explicitly not doing
- **Fluent DSL / builder syntax** (backlog Direction B's rejected sketch shape) —
  plain helpers only (D1).
- **Runtime `[]Event` invariant checking** — deferred behind the readable-trace
  plan's `Event` seam (D2); no runtime capture is introduced here.
- **Spy state-capture / `StateAware` knob** (backlog Direction C) — rejected there,
  rejected here, not revived (D5).
- **YAML build-time lint pass** — the backlog doc allowed purely-structural
  invariants to migrate into the loader later, but this plan keeps them as
  test-time graph checks; promoting any rule to a load-time error is a separate
  decision.
- **Diagram regeneration** — the regenerate-diagram workflow owns
  `docs/process-diagram.md` + `docs/images/*.svg` (`feedback_plans_no_diagram_regen`).
