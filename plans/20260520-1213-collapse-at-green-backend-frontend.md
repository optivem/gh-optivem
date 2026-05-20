# Plan: collapse `AT_GREEN_BACKEND` + `AT_GREEN_FRONTEND` into a single `AT_GREEN` node

**Date:** 2026-05-20 12:13 UTC
**Promoted from:** `plans/20260519-1537-post-meta-bpmn-topics.md` Item 6 (immediate-collapse half). Folds in Item 11 (hardcoded backend/frontend node names) and Item 12 (`<acceptance-api>` / `<acceptance-ui>` suite labels).
**Sibling plan (still inbound):** `plans/20260520-1109-ac-refinement-and-at-refactor-agent-steps.md` — orthogonal; touches different phases. No file conflicts expected.
**Deferred follow-up (NOT this plan):** per-component fanout (Item 6's stretch goal) — wait for concrete multi-component requirements + SSoT shape stabilisation.

## Purpose

The `at_green_system` sub-process currently dispatches `green_phase_cycle`
**twice** in sequence — once as `AT_GREEN_BACKEND` (`suite: "<acceptance-api>"`)
and once as `AT_GREEN_FRONTEND` (`suite: "<acceptance-ui>"`) — even though:

- The merged prompt `internal/assets/runtime/prompts/atdd/at-green-system.md`
  is already a **single, generic, channel-agnostic implementation agent** with
  `scope: {}` and no backend/frontend mention (the prompt-level merge landed
  in an earlier sweep; the state-machine duality is residual drift).
- Both nodes run **unconditionally**. Monolith projects still execute the
  `AT_GREEN_FRONTEND` dispatch even when there is no frontend channel to
  implement — wasted dispatch + wasted RUN + wasted CHECK_PHASE_SCOPE.
- Both nodes reference `phase_doc: docs/atdd/process/change/behavior/at-green-system.md`,
  which **no longer exists in the asset tree** (swept by Item 5's inlining).

This plan collapses the duality at the state-machine level. After this
plan lands, the AT cycle's GREEN segment is:

```
ENABLE_TESTS → AT_GREEN → COMMIT → TICK → MOVE_TICKET_IN_ACCEPTANCE → GS_END
```

One `call_activity` into `green_phase_cycle`. `green_phase_cycle` itself is
unchanged — its parameterisation is already the correct shape for either
today's single-call use or future per-component fanout.

## Out of scope (deliberately)

- **Per-component fanout** (Item 6 stretch goal). `gh-optivem.yaml`
  `system.path` → `system.components: [{name, path, language?}, …]`, parameterised
  loop over components in `process-flow.yaml`, per-component `suite:` labels.
  Deferred to a future dated plan; promote when concrete multi-component
  requirements arrive.
- **Systematic sweep of dangling `phase_doc:` references** across other
  phases (`at-red-test`, `at-red-dsl`, `at-red-system-driver`, `ct-red-*`,
  structural cycle). After Item 5's inlining, every phase doc target under
  `docs/atdd/process/change/behavior/` is gone except `at-refactor.md`.
  This plan cleans up the **local** drift on the AT_GREEN node only.
  The systematic walk is Item 4's residual (noted, not promoted; pick up
  the next time a dangling-reference bug bites).
- **`testkit` suite labels** (`acceptance-api` / `acceptance-ui` as test-
  classification labels in `internal/atdd/runtime/testselect/`). These are
  testkit concerns — tagging tests by channel for selective execution.
  They **survive** the collapse. The collapse only removes the *dispatch*
  side (the state-machine `suite:` param that fans out two runs); the
  classification side stays so a developer can still run `gh optivem test
  run --suite acceptance-ui` manually.
- **The `plans/deferred/20260518-1530-multitier-green-scope.md` content
  itself.** Its premise (multitier projects have AT_GREEN_BACKEND + AT_GREEN_FRONTEND
  as separate state-machine nodes that need per-tier scope) dissolves after
  this plan. This plan **closes** that deferred plan (marks it RESOLVED-BY
  + cross-link); the scope-key vocabulary question re-emerges only if/when
  per-component fanout lands.

## Current state (line numbers as of HEAD `1d75018`)

**process-flow.yaml** (`internal/atdd/runtime/statemachine/process-flow.yaml`):

- `391-393` — top-level `AT_GREEN_SYSTEM` `call_activity` into `at_green_system`.
  Unchanged by this plan.
- `424-483` — `at_green_system` sub-process. Today: `ENABLE_TESTS` → `AT_GREEN_BACKEND` →
  `AT_GREEN_FRONTEND` → `COMMIT` → `TICK` → `MOVE_TICKET_IN_ACCEPTANCE` → `GS_END`.
- `432-444` — `AT_GREEN_BACKEND` node. Params: `agent: at-green-system`,
  `phase_doc: docs/atdd/process/change/behavior/at-green-system.md` *(dangling)*,
  `phase_label: "AT - GREEN - SYSTEM (backend)"`, `phase_id: AT_GREEN_BACKEND`,
  `suite: "<acceptance-api>"`, `rebuild_before_run: "true"`, `compile_action: compile_system`.
- `445-456` — `AT_GREEN_FRONTEND` node. Same shape with `(frontend)` label and
  `suite: "<acceptance-ui>"`.
- `478-480` — sequence flows: `ENABLE_TESTS → AT_GREEN_BACKEND → AT_GREEN_FRONTEND → COMMIT`.

**Prompt** (`internal/assets/runtime/prompts/atdd/at-green-system.md`):

- Already merged (channel-agnostic body, `scope: {}`). Frontmatter carries a TODO comment
  about a future `at-green-system` + `at-green-component` split — this comment
  is a follow-up artifact and should be reworded (not deleted; per-component
  fanout is the deferred follow-up, not "won't happen"). See item 5 below.

**phase-scopes doctrine** (`internal/atdd/phase-scopes.yaml`):

- `27` — comment trailing `AT_GREEN_SYSTEM`: "monolith GREEN; AT_GREEN_BACKEND / AT_GREEN_FRONTEND
  allowlisted per plans/deferred/20260518-1530-multitier-green-scope.md". The allowlist
  citation will go stale once the two phases stop existing.

**phase-scopes loader** (`internal/atdd/phase_scopes.go`):

- `38-39` — `PhasesDeferredByPlan` map entries for `AT_GREEN_BACKEND` and `AT_GREEN_FRONTEND`,
  both citing `plans/deferred/20260518-1530-multitier-green-scope.md`. Both rows go away.

**State-machine tests:**

- `internal/atdd/runtime/statemachine/behavioral_cycle_test.go:160-182` — `atGreenSystem()`
  helper invokes `greenCycle("AT_GREEN_BACKEND", …)` then `greenCycle("AT_GREEN_FRONTEND", …)`.
  The expectation walks the spec literally — both dispatches must collapse to one.
- `internal/atdd/runtime/statemachine/behavioral_cycle_test.go:26-29` — comment refers
  to the dual cycle.
- `internal/atdd/runtime/statemachine/dispatch_spy_test.go:301-325` — `atGreenBackendParams()`
  and `atGreenFrontendParams()` helpers. Replace with a single `atGreenParams()`.
- `internal/atdd/runtime/statemachine/transitions_test.go:222-229` — transition-coverage
  spec lists both nodes; collapses to one.
- `internal/atdd/runtime/actions/bindings_test.go:1764` — `ctx.Params["phase_id"] = "AT_GREEN_BACKEND"`
  used as a fixture for `PhasesDeferredByPlan` lookup. Needs a substitute fixture (or
  the test reframes around a still-deferred phase, e.g. one of the `*_INTERFACE_REDESIGN_CYCLE`
  entries that survive in `PhasesDeferredByPlan`).

**Docs:**

- `docs/process-diagram.md:188-198` — mermaid AT_GREEN_BACKEND / AT_GREEN_FRONTEND
  nodes + edges.
- `docs/images/process-diagram-7-at-green-system.svg` — regenerated; not hand-edited.

**testkit (out of scope but listed so the reader sees the boundary):**

- `internal/atdd/runtime/testselect/` (`suite.go`, `testselect.go`, `tracer.go`, +
  `*_test.go`) — references `acceptance-api` / `acceptance-ui` as **test-classification
  labels**. **Survive**. The collapse only removes the dispatch-side `suite:` value.
- `docs/gh-monitoring-process.md:52` — user-facing example `gh optivem test run --suite acceptance-ui`.
  **Survives**.

---

## Items to walk

Items are stubs deliberately — refine each one (with `/refine-plan`) to nail
down the decisions before executing. Items 1, 5, 8 carry open design questions
that should be settled first; items 2, 3, 4, 6, 7, 9 are mechanical edits
downstream of those decisions.

### Item 1 — Pick the new `suite:` value (or drop the param)

**Question:** what does `suite:` reduce to on the new `AT_GREEN` node?

Three candidate options, each with consequences for `run_targeted_tests`:

- **(a) Drop `suite:` from `AT_GREEN`'s params entirely.** `green_phase_cycle`
  uses `${suite}` only inside the RUN node's documentation string (line 1039)
  and via the runtime's `CtxKeySuite` (see `internal/atdd/runtime/actions/bindings.go:711,1358`).
  Need to verify whether `run_targeted_tests` can run **all acceptance suites**
  when the suite key is absent or empty; today every caller passes a value.
- **(b) Single sentinel `<acceptance>`** (or `<acceptance-all>`) that resolves
  at runtime to the union of `acceptance-api` + `acceptance-ui` (+ any future
  acceptance-channel suite). Requires a one-line addition to the suite
  classifier / `gh optivem test run --suite <acceptance>` plumbing.
- **(c) Keep both today, run twice still.** No-op on test cost; defeats the
  collapse rationale. Listed for completeness, not recommended.

**Recommendation seed:** option (b) — single sentinel — is the cleanest fit
for the existing `<...>`-enclosed suite convention (`<acceptance-api>`,
`<acceptance-ui>`, `<suite-contract-real>` are all sentinel-shaped). It also
forward-compatible with per-component fanout (the future `<acceptance-${component}>`).

**Inputs to verify before deciding:**

- Read `run_targeted_tests` action in `internal/atdd/runtime/actions/bindings.go`
  (around line 711) to confirm how the suite param flows through to `gh
  optivem test run --suite <…>` and whether multi-suite or absent-suite is
  already supported.
- Read `internal/atdd/runtime/testselect/suite.go:32-57` to see what `AcceptanceSuites()`
  returns — there is already a known "both acceptance suites" notion at the
  testkit level.

**Decision sink:** pin in this item once walked; items 2, 5, 6, 7 follow.

---

### Item 2 — Edit `process-flow.yaml`: collapse the two nodes

Replace the AT_GREEN_BACKEND + AT_GREEN_FRONTEND block (lines 432-456) and
its sequence-flows (lines 478-480) with:

```yaml
      - id: AT_GREEN
        type: call_activity
        process: green_phase_cycle
        params:
          agent: at-green-system
          phase_label: "AT - GREEN - SYSTEM"
          phase_id: AT_GREEN
          suite: <decided in Item 1>
          rebuild_before_run: "true"
          compile_action: compile_system
        documentation: "AT - GREEN - SYSTEM - WRITE"
```

Sequence-flows:

```yaml
      - {from: ENABLE_TESTS,         to: AT_GREEN}
      - {from: AT_GREEN,             to: COMMIT}
```

(other AT_GREEN-system flows from line 481 onward are unchanged: `COMMIT →
TICK → MOVE_TICKET_IN_ACCEPTANCE → GS_END`.)

Update the comment block at lines 416-423 to drop the "backend + frontend
implementation through the shared green_phase_cycle (one call_activity per
channel)" wording.

**Also:** drop `phase_doc:` from the new AT_GREEN node (the target file is
gone; the merged prompt is self-contained). Verify that `${phase_doc}`
substitution inside `green_phase_cycle` (lines 1007, 1033) tolerates an
absent key — if it does not, either pin `phase_doc: ""` on the new node OR
introduce a conditional in the cycle. Out-of-scope: the same dangling
`phase_doc:` appears on `AT_RED_TEST`, `AT_RED_DSL`, `AT_RED_SYSTEM_DRIVER`,
`CT_RED_*`, `CT_GREEN_EXTERNAL_SYSTEM_STUB`, `SYSTEM_INTERFACE_REDESIGN`,
and `EXTERNAL_SYSTEM_INTERFACE_REDESIGN` — leave those alone.

---

### Item 3 — Edit `phase-scopes.yaml`: drop the allowlist citation

Line 27 today:

```yaml
  AT_GREEN_SYSTEM:        [system_path]                         # monolith GREEN; AT_GREEN_BACKEND / AT_GREEN_FRONTEND allowlisted per plans/deferred/20260518-1530-multitier-green-scope.md
```

After this plan: drop the trailing allowlist comment (no more
AT_GREEN_BACKEND / AT_GREEN_FRONTEND to allowlist). The row itself
becomes:

```yaml
  AT_GREEN_SYSTEM:        [system_path]
```

Question to settle: does `AT_GREEN` (the new sub-process node) need a row
in this file? Today the per-phase scope is keyed by **state-machine node
id**, not BPMN sub-process name. `AT_GREEN_SYSTEM` (top-level call_activity)
already exists in the file; `AT_GREEN` (new inner node) is a different
identity. Inspect what `check_phase_scope` keys against before deciding.

---

### Item 4 — Edit `internal/atdd/phase_scopes.go`: drop the two `PhasesDeferredByPlan` entries

Remove lines 38-39:

```go
"AT_GREEN_BACKEND":                         "plans/deferred/20260518-1530-multitier-green-scope.md",
"AT_GREEN_FRONTEND":                        "plans/deferred/20260518-1530-multitier-green-scope.md",
```

The three remaining entries (`SYSTEM_INTERFACE_REDESIGN_CYCLE`,
`EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE`, `CHORE_CYCLE`) stay — they
cite a different deferred plan.

If Item 3 adds an `AT_GREEN` row to `phase-scopes.yaml`, no new
`PhasesDeferredByPlan` entry is needed. If Item 3 doesn't, then
`AT_GREEN` may need a new entry citing this plan as its "deferred by"
reference (since `AT_GREEN` is a writing-agent phase id that needs to
satisfy the build-time cross-validator). Settle in Item 3.

---

### Item 5 — Edit the prompt frontmatter comment

`internal/assets/runtime/prompts/atdd/at-green-system.md` line 4 today:

```
# TODO: future split into at-green-system + at-green-component variants — deferred.
```

Reword to reflect the new state — the state-machine collapse is done; the
deferred shape is per-component fanout, not per-channel split:

```
# Per-component fanout (system.components: [{name, path, language?}, …]) deferred — see <future plan ref>.
```

Also revisit the `scope: {}` line + its trailing comment (line 7):

```yaml
scope: {}   # multitier GREEN scope deferred — see plans/deferred/20260518-1530-multitier-green-scope.md
```

Once that deferred plan is closed (Item 9), this comment dangles. Either
drop it ("scope: {}" with no comment) or repoint it to the per-component
fanout follow-up.

---

### Item 6 — Edit the dispatch / behavioral-cycle / transitions tests

`internal/atdd/runtime/statemachine/dispatch_spy_test.go`:

- Replace `atGreenBackendParams()` (lines 301-312) and `atGreenFrontendParams()`
  (lines 314-325) with a single `atGreenParams()` returning the new shape
  decided in Items 1 + 2.

`internal/atdd/runtime/statemachine/behavioral_cycle_test.go`:

- `atGreenSystem()` helper (lines 165-182): replace the two `greenCycle(...)`
  lines with one: `greenCycle("AT_GREEN", atGreenParams())`.
- Update the comment at lines 26-29 to reflect a single green_phase_cycle
  dispatch.

`internal/atdd/runtime/statemachine/transitions_test.go`:

- Lines 222 and 227-229: replace the two transitions (`ENABLE_TESTS →
  AT_GREEN_BACKEND` + `AT_GREEN_BACKEND → AT_GREEN_FRONTEND` + `AT_GREEN_FRONTEND
  → COMMIT`) with two transitions (`ENABLE_TESTS → AT_GREEN` + `AT_GREEN →
  COMMIT`).

---

### Item 7 — Edit `bindings_test.go:1764` fixture

`internal/atdd/runtime/actions/bindings_test.go:1764` today:

```go
ctx.Params["phase_id"] = "AT_GREEN_BACKEND" // in PhasesDeferredByPlan
```

After Item 4 removes the two AT_GREEN_* entries, this fixture's reference
goes stale. Pick a surviving `PhasesDeferredByPlan` entry as substitute
(e.g. `"CHORE_CYCLE"` or `"SYSTEM_INTERFACE_REDESIGN_CYCLE"`). Inspect the
surrounding test to confirm the substitution is semantically OK (the test
just needs *some* deferred-by-plan phase id; channel-specificity should
not matter).

---

### Item 8 — Verify `run_targeted_tests` behavior under the new `suite:` value

Decision from Item 1 may require runtime support that doesn't exist yet.
Cases:

- **If Item 1 = (a) drop:** verify `run_targeted_tests` does not assume a
  non-empty suite. If it does, either backfill a default or change the
  default behavior to "all acceptance suites when absent".
- **If Item 1 = (b) sentinel:** verify the suite resolver (in
  `bindings.go` around line 1358) accepts a sentinel like `<acceptance>`
  and unions the underlying physical suites. If not, this plan adds the
  minimal sentinel handling.

Land the runtime-support change **first** in the same plan execution (or as
a sub-item before Item 2 — Item 2's collapse must not land before the
runtime supports whichever suite shape it dispatches).

---

### Item 9 — Close the deferred plan

`plans/deferred/20260518-1530-multitier-green-scope.md` — add a "RESOLVED-BY"
header at the top citing this plan:

```
**Status:** RESOLVED-BY plans/20260520-1213-collapse-at-green-backend-frontend.md (the AT_GREEN_BACKEND / AT_GREEN_FRONTEND duality these scope rows were waiting on has been collapsed to a single AT_GREEN node).
```

Leave the body intact for archival reference; the scope-key vocabulary
discussion (options a/b/c in that plan) re-emerges only if/when per-component
fanout lands, and that future plan can cross-reference back here.

---

### Item 10 — Regenerate `docs/process-diagram.md` + the SVG

`docs/process-diagram.md` lines 188-198:

- Drop the `AT_GREEN_BACKEND` and `AT_GREEN_FRONTEND` mermaid nodes.
- Replace with a single `AT_GREEN["AT - GREEN - SYSTEM - WRITE — see § green_phase_cycle"]`.
- Update edges: `ENABLE_TESTS --> AT_GREEN`, `AT_GREEN --> COMMIT`.

`docs/images/process-diagram-7-at-green-system.svg` is generated — re-run
whatever produces it (likely `gh optivem architecture show` or equivalent).
If regeneration is non-trivial, defer the SVG refresh and call it out in
the commit message.

---

## Sequencing (within this plan)

Walk Items 1 + 8 first (the design + runtime-support questions). Once those
settle, the rest is mechanical:

```
Item 1 (suite: decision) ─┬─→ Item 8 (run_targeted_tests support, if any)
                          │
                          └─→ Items 2, 3, 4, 5, 6, 7 (mechanical edits)
                                  └─→ Items 9, 10 (housekeeping)
```

Items 9 and 10 are independent of each other and can land last in either
order.

## Sequencing (vs other in-flight plans)

- **Independent of:** `plans/20260520-1109-ac-refinement-and-at-refactor-agent-steps.md`
  (different phases, different files).
- **Downstream of:** `plans/20260520-0907-runtime-shared-scope-injection.md`
  (universal `scope.md` injection). That plan should land first so `scope: {}`
  semantics are stable before we touch the AT_GREEN prompt's frontmatter
  (Item 5).
- **Upstream of:** any per-component fanout follow-up plan. That future
  plan will revisit the new `AT_GREEN` node's shape (turn the single
  `call_activity` into a parameterised loop). Nothing in this plan should
  bake in assumptions that block that follow-up.

## Risks / things to watch

- **Test counts.** `CHECK_PHASE_SCOPE` and the `run_targeted_tests` action
  currently fire **twice** per AT cycle (once per channel). After the
  collapse they fire **once**. If any test or fixture counts dispatches,
  it will break — search for `CHECK_PHASE_SCOPE` and `run_targeted_tests`
  count assertions before executing Item 2.
- **`phase_id` consumers.** `phase_id` was `AT_GREEN_BACKEND` / `AT_GREEN_FRONTEND`;
  after the collapse it's `AT_GREEN` (or whatever Item 2 settles). Anywhere
  the runtime keys behaviour off `phase_id` (e.g. `check_phase_scope`'s
  scope-key lookup, dispatch logging, commit-message scaffolding) needs
  to know the new id exists. Quick grep before executing.
- **Statemachine test loop hazard.** Per the user's memory, new loopback
  edges in `process-flow.yaml` can deadlock statemachine tests and consume
  20GB+ RAM. This plan removes nodes; it doesn't add loops. Still — audit
  gate fixtures (in particular `phase_scope_clean` and `tests_pass`) once
  the new node lands, and kill any test run on RAM climb.

## Hand-off

Next concrete step:

```
/refine-plan plans/20260520-1213-collapse-at-green-backend-frontend.md
```

Walk Item 1 first (the `suite:` decision) — once that's pinned, Item 8's
runtime-support sub-tasks become concrete, and the rest of the items
fall in line.
