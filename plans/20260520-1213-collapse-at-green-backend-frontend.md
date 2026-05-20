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

**Decision (refined 2026-05-20):** **Drop `suite:` from `AT_GREEN`'s params entirely.**

Rationale: agent prompts don't run tests; the BPMN orchestrator's
`run_targeted_tests` action does. With one collapsed `AT_GREEN` node, there
is no per-channel dispatch to parameterise, so the cleanest shape is no
`suite:` key at all. Channel-based test execution (running only the
api or ui suite for a given component) is a separate concern and gets a
dedicated future plan.

**Future plan (NOT this one):** channel-based execution of tests — when
per-component fanout lands (or before), introduce a `suite:` key on the
fanned-out nodes (e.g. `<acceptance-${component}>` or sentinel
`<acceptance>` resolving to a channel-aware union). Today, with one
unified `AT_GREEN`, the absent-key form is sufficient.

**Inputs to verify in Item 8** (runtime support):

- Read `run_targeted_tests` in `internal/atdd/runtime/actions/bindings.go`
  (around line 711) — confirm absent/empty `suite:` is supported, or
  add the minimal default-behavior support (run all acceptance suites
  when the key is absent).
- Read `internal/atdd/runtime/testselect/suite.go:32-57` to confirm
  `AcceptanceSuites()` already represents the "all acceptance suites" set
  the runtime would fall back to.

**Follow-on impact** (settled by this decision):

- Item 2's new `AT_GREEN` YAML block has no `suite:` line.
- Item 8 narrows to: verify (or add) absent-suite support in
  `run_targeted_tests`.
- Items 5, 6, 7 are unaffected by this choice.

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
          rebuild_before_run: "true"
          compile_action: compile_system
        documentation: "AT - GREEN - SYSTEM - WRITE"
```

Notes on the new params:

- **No `suite:` key.** Per Item 1's decision, drop `suite:` entirely. The
  runtime falls back to running all acceptance suites (Item 8 verifies /
  backfills this default).
- **No `phase_doc:` key.** The target file
  (`docs/atdd/process/change/behavior/at-green-system.md`) was swept by the
  earlier inlining sweep; the merged prompt is self-contained.

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

**Sub-task — `${phase_doc}` absent-key verification:** verify that
`${phase_doc}` substitution inside `green_phase_cycle` (lines 1007, 1033)
tolerates an absent key. If it does not, the fallback is to pin
`phase_doc: ""` on the new AT_GREEN node (preferred; one-line fix) rather
than introducing a conditional inside the cycle. Out of scope for this
plan: the same dangling `phase_doc:` appears on `AT_RED_TEST`, `AT_RED_DSL`,
`AT_RED_SYSTEM_DRIVER`, `CT_RED_*`, `CT_GREEN_EXTERNAL_SYSTEM_STUB`,
`SYSTEM_INTERFACE_REDESIGN`, and `EXTERNAL_SYSTEM_INTERFACE_REDESIGN` —
leave those alone (Item 4 residual of the parent plan).

---

### Item 3 — Edit `phase-scopes.yaml`: swap AT_GREEN_SYSTEM row for AT_GREEN

Line 27 today:

```yaml
  AT_GREEN_SYSTEM:        [system_path]                         # monolith GREEN; AT_GREEN_BACKEND / AT_GREEN_FRONTEND allowlisted per plans/deferred/20260518-1530-multitier-green-scope.md
```

**Grounding** (from `internal/atdd/phase_scopes_test.go:100-112` —
`TestPhaseScopes_ReverseFK_WritingAgentsScopedOrAllowlisted`):

- The scope file is validated against **writing-agent node ids** —
  call_activity nodes count only when they carry `agent:` in their
  `params:`.
- Outer `AT_GREEN_SYSTEM` (line 391-393 of process-flow.yaml) is a bare
  `call_activity` with `process: at_green_system` and NO `params:`. It is
  NOT a writing-agent node; the current row passes only the forward-FK
  existence check, never the reverse-FK scope check. Effectively dead
  weight.
- Inner `AT_GREEN_BACKEND` and `AT_GREEN_FRONTEND` ARE writing-agent
  nodes (they have `agent: at-green-system` in `params:`); that's why
  they sit on `PhasesDeferredByPlan` instead of in this file.

**Decision (refined 2026-05-20):** the new inner writing-agent node is
named `AT_GREEN` (decision: keep outer/inner separation; outer call_activity
stays `AT_GREEN_SYSTEM`). Refactor the file as:

```yaml
  AT_GREEN:               [system_path]
```

(drops both the trailing allowlist comment AND the now-dead
`AT_GREEN_SYSTEM` row; replaces with a real `AT_GREEN` row for the new
writing-agent node).

**Executor verification step:** before editing, re-grep
`phase_scopes_test.go` to confirm the reverse-FK check still uses
`writingAgentNodeIDs(...)` keyed by `concreteAgent(node)`. If the
validator has been changed to key against outer call_activity ids, fall
back to the conservative option: keep the `AT_GREEN_SYSTEM: [system_path]`
row AND add `AT_GREEN: [system_path]`.

**Co-ordination with Item 4:** dropping the `AT_GREEN_BACKEND` /
`AT_GREEN_FRONTEND` entries from `PhasesDeferredByPlan` (Item 4) MUST land
together with this row swap. The reverse-FK test fails if the two inner
phase ids are removed from the BPMN without their allowlist entries also
being removed; equally, it fails if the allowlist entries are removed
without the BPMN nodes being removed.

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

No new `AT_GREEN` allowlist entry is needed — Item 3 adds a real
`AT_GREEN: [system_path]` row to `phase-scopes.yaml`, which satisfies the
reverse-FK validator on its own.

**Co-ordination with Item 3:** this edit MUST land together with Item 3's
row swap. The reverse-FK test fails if either side lands alone (BPMN
nodes removed without allowlist entries removed → stale-allowlist test
fails; allowlist entries removed without new phase-scopes row →
reverse-FK test fails because the new `AT_GREEN` node has no scope).

---

### Item 5 — Drop the stale frontmatter comments

`internal/assets/runtime/prompts/atdd/at-green-system.md` carries two
comments that the collapse + Item 9 closure make stale. Drop both rather
than relocate (deferred-follow-up tracking belongs in the `plans/` tree,
not in prompt frontmatter — per the user's standing preference, drop don't
relocate).

**Line 4 today:**

```
# TODO: future split into at-green-system + at-green-component variants — deferred.
```

→ delete the line entirely. The deferred per-component fanout follow-up
is already tracked in this plan's "Out of scope (deliberately)" section
and will reappear in whichever future plan promotes it; the prompt
frontmatter doesn't need to carry the cross-ref.

**Line 7 today:**

```yaml
scope: {}   # multitier GREEN scope deferred — see plans/deferred/20260518-1530-multitier-green-scope.md
```

→ drop the trailing comment, keep `scope: {}` bare:

```yaml
scope: {}
```

After Item 9 closes the deferred plan, the citation dangles regardless;
drop it now.

**Note on `scope: {}` itself:** the empty-map value stays. It is the
correct shape under the universal `scope.md` injection (cf.
`plans/20260520-0907-runtime-shared-scope-injection.md`) — the per-phase
scope projection is computed from `phase-scopes.yaml` + project paths,
not declared in the prompt frontmatter. Empty-map = "use the universal
projection". No change needed here.

---

### Item 6 — Edit the dispatch / behavioral-cycle / transitions tests

`internal/atdd/runtime/statemachine/dispatch_spy_test.go`:

- Replace `atGreenBackendParams()` (lines 301-312) and `atGreenFrontendParams()`
  (lines 314-325) with a single `atGreenParams()` that mirrors Item 2's
  collapsed YAML shape exactly:

  ```go
  func atGreenParams() map[string]string {
      return map[string]string{
          "agent":              "at-green-system",
          "phase_label":        "AT - GREEN - SYSTEM",
          "phase_id":           "AT_GREEN",
          "rebuild_before_run": "true",
          "compile_action":     "compile_system",
      }
  }
  ```

  No `suite:` key (Item 1 decision: drop). No `phase_doc:` key (Item 2:
  merged prompt is self-contained). If Item 2's sub-task settles on
  `phase_doc: ""` as a fallback (cycle doesn't tolerate absent key), add
  the matching `"phase_doc": ""` entry here.

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

### Item 7 — Substitute `bindings_test.go:1764` fixture (interim)

`internal/atdd/runtime/actions/bindings_test.go:1764` today:

```go
ctx.Params["phase_id"] = "AT_GREEN_BACKEND" // in PhasesDeferredByPlan
```

After Item 4 removes the two AT_GREEN_* entries, this fixture's string
dangles.

**Doctrinal note:** the test
(`TestCheckPhaseScope_AllowlistedPhaseIsNoop`) exercises the
`PhasesDeferredByPlan` allowlist short-circuit — a mechanism the project
is removing per `feedback-no-deferred-mechanism`. The full removal is
scoped to `plans/20260520-1053-remove-phases-deferred-by-plan.md`
(which deletes this test, the allowlist map, and the runtime
short-circuit, after pinning real scopes for the three remaining
formerly-allowlisted cycles).

**This plan's action:** interim substitution only. Replace
`"AT_GREEN_BACKEND"` with `"CHORE_CYCLE"` and update the trailing
comment:

```go
ctx.Params["phase_id"] = "CHORE_CYCLE" // arbitrary allowlisted phase; test exercises the allowlist mechanism, not the cycle itself. Test removed by plans/20260520-1053-remove-phases-deferred-by-plan.md.
```

The choice of `CHORE_CYCLE` over the two other surviving entries
(`SYSTEM_INTERFACE_REDESIGN_CYCLE`, `EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE`)
is arbitrary — the test is mechanism-level and channel-agnostic. The
comment makes that explicit so a future reader doesn't infer significance
from the cycle id, and forward-refs the follow-up plan that retires the
fixture entirely.

If the follow-up plan lands first (or alongside), this item collapses to a
no-op — no fixture to substitute because the test is deleted.

---

### Item 8 — Verify `run_targeted_tests` handles absent `suite:`

Item 1 pinned the decision to **drop `suite:` from `AT_GREEN`'s params**.
Narrowed scope: verify the runtime tolerates an absent/empty suite key, and
backfill the default if it doesn't.

**Steps:**

1. Read `run_targeted_tests` in `internal/atdd/runtime/actions/bindings.go`
   (around line 711) — trace how `CtxKeySuite` is consumed when absent. Note
   the line 1358 suite-resolver path too.
2. Read `internal/atdd/runtime/testselect/suite.go:32-57` —
   `AcceptanceSuites()` is the natural "all acceptance suites" set to fall
   back to.
3. **Decision branch:**
   - **If absent/empty suite already runs all acceptance suites** → no code
     change; just record the confirmation in the commit message.
   - **If absent/empty suite errors or runs nothing** → add the minimal
     default: when the orchestrator dispatches without `suite:`, resolve to
     `AcceptanceSuites()`. Keep the change tight — no new flags, no opt-in;
     this is the unconditional default for the absent-key case.

**Gating:** this item **lands before Item 2**. Item 2's collapse drops the
`suite:` key from the new `AT_GREEN` node, and that change must not ship
before the runtime supports the absent-key case.

**Out of scope (future channel-execution plan):** sentinel suites like
`<acceptance>` / `<acceptance-${component}>` that union or scope channels
explicitly. Today's collapse intentionally uses the absent-key default;
sentinels arrive when per-component fanout does.

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

### Item 10 — Regenerate `docs/process-diagram.md` + the SVGs

Both `docs/process-diagram.md` and `docs/images/process-diagram-*.svg`
are deterministically generated from `process-flow.yaml`. Do NOT
hand-edit the mermaid fences or the SVG; once Item 2's YAML change
lands, run the two-step regen:

```bash
gh optivem process show > docs/process-diagram.md
bash scripts/render-svgs.sh
```

(The .md header at line 3 self-documents the first command; the SVG
regen script renders every Mermaid fence under
`docs/process-diagram.md` to `docs/images/process-diagram-*.svg`,
pinning `@mermaid-js/mermaid-cli@11.14.0` to match the
`.github/workflows/regenerate-diagram.yml` workflow.)

**Verify after regen:**

- `docs/process-diagram.md` no longer contains `AT_GREEN_BACKEND` or
  `AT_GREEN_FRONTEND`.
- `docs/process-diagram.md` contains a single `AT_GREEN` node with the
  expected label.
- `docs/images/process-diagram-7-at-green-system.svg` exists and is
  visually consistent with the new mermaid source.

**Note on the earlier hand-edit description:** the original Item 10 text
referenced specific line numbers (188-198) and `gh optivem architecture
show` — both wrong. `architecture show` renders the layered-architecture
diagram, not the process flow; the process flow is generated by
`process show`. Hand-editing line ranges is also wrong shape: regen is
the entire change.

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
