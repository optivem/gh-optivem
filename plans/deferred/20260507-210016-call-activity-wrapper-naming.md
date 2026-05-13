# Call Activity wrapper naming — open question

**Status: DECISION PENDING — human to decide.** This plan captures the trade-off analysis but does not resolve it. No code, YAML, tests, or docs should be edited based on this plan until the decision is made and recorded under "Resolution" below.

## Motivation

While reading `internal/atdd/runtime/statemachine/behavioral_cycle_test.go` and walking the at_cycle trail, the question came up: should the call_activity nodes that wrap shared sub-process dispatches (e.g. `AT_RED_TEST` → `red_phase_cycle`) be renamed to something verb-object-y that names the artifact being produced (e.g. `WRITE_ACCEPTANCE_TEST`, `WRITE_DSL`, `WRITE_CONTRACT_TEST`)?

The proposal was driven by a fair instinct — "WRITE" inside the shared `red_phase_cycle` is generic by design, and a reader of `at_cycle`'s diagram alone can't tell what each `*_RED_*` call_activity actually produces without descending into `red_phase_cycle`. Naming the wrapper after the artifact would let the parent diagram self-document.

This plan settles **the analysis** so the human can pick a direction and so the eventual rename pass tracked in `plans/20260501-144322-process-flow-node-id-rename-open-questions.md` doesn't pick up either choice by accident before the decision is made.

## Background — what BPMN says

BPMN's naming rule for **Call Activity** (the node type in BPMN that maps to our `call_activity:` YAML kind) is: name the call_activity for the **work the called sub-process accomplishes as a whole**, not for one step inside it. The current YAML literally implements the BPMN Call Activity + data input ("inheritance") pattern:

```yaml
at_cycle:
  - id: AT_RED_TEST                  # wrapper (parent-level, specific name)
    type: call_activity
    process: red_phase_cycle         # shared body
    params:                          # data input customising the body
      agent: atdd-test
      phase_label: "AT - RED - TEST"
```

The whole `red_phase_cycle` does:

```
agent WRITE
  → human STOP_RED_REVIEW
  → runtime COMPILE
  → runtime GATE_COMPILE_OK / loop into WRITE_PROTOTYPES on miss
  → runtime GATE_VERIFY_REAL_REQUIRED / VERIFY_REAL on the CT_RED_TEST branch
  → runtime RUN
  → runtime GATE_RUN_FAILED_RUNTIME (verifies redness — runtime, not agent)
  → runtime DISABLE
  → runtime COMMIT
```

The artifact produced when control returns to the parent is **a committed, runtime-failing test on disk** (or DSL bindings, or driver adapter). `WRITE` is the first of seven steps and is the only one the agent owns; the other six belong to the runtime + one human review.

This is also reinforced by the verification-ownership point: confirming the test is genuinely red (`tests_failed_runtime == true` via the `run_targeted_tests` action) is a **CLI-side responsibility**, not an agent step. A name that picks the agent's contribution misrepresents who owns the activity.

## Candidates

### Option 1 — keep current names: `AT_RED_TEST` / `CT_RED_DSL` / `AT_RED_SYSTEM_DRIVER` / etc.

| | |
|---|---|
| BPMN-correct? | ✅ Names the phase as a whole. |
| Reader-self-documenting at the parent diagram? | Partial — reader knows the *phase* but must descend into `red_phase_cycle` to see what `WRITE` actually writes. |
| Aligned with project canon? | ✅ Uses the AT/CT phase identifiers from `docs/atdd/process/*.md`. |
| Migration cost | ✅ Zero — status quo. |

### Option 2 — `WRITE_ACCEPTANCE_TEST` / `WRITE_DSL` / `WRITE_CONTRACT_TEST` family

| | |
|---|---|
| BPMN-correct? | ❌ Names only step 1 of 7. Hides that the runtime + human own the rest. Misattributes ownership (suggests an agent activity, but it's a mostly-runtime activity with one creative step at the front). |
| Reader-self-documenting at the parent diagram? | ✅ Best of the three — reader sees exactly what artifact is being produced. |
| Aligned with project canon? | Mixed — abandons AT/CT phase vocabulary; introduces artifact-centric vocabulary. |
| Migration cost | Moderate — YAML edits + transitions / structural / behavioral test trail updates + driver / atdd_commands references + 14 SVG diagram regenerations. |

### Option 3 — `PRODUCE_FAILING_ACCEPTANCE_TEST` / `PRODUCE_FAILING_DSL` / etc.

| | |
|---|---|
| BPMN-correct? | ✅ Pure verb-object; names the artifact the phase produces. The most BPMN-idiomatic in strict lexical terms. |
| Reader-self-documenting at the parent diagram? | ✅ Names both the action ("produce failing") and the artifact. |
| Aligned with project canon? | Partial — verbose, abandons AT/CT phase vocabulary used throughout the docs. |
| Migration cost | Same as Option 2. |

### Option 4 — `WRITE_*` wrapper + split `red_phase_cycle` into `write_subprocess` + `red_mechanical_tail`

This would let `WRITE_ACCEPTANCE_TEST` honestly name only the WRITE step, with the mechanical tail factored separately.

| | |
|---|---|
| BPMN-correct? | ✅ Each wrapper now honestly names what it dispatches. |
| Splits the cohesive RED-phase unit? | ❌ Today `red_phase_cycle`'s diagram tells one complete story (agent writes → human reviews → runtime processes). Option 4 splits that story across two diagrams, hiding the rhythm. |
| Where does `STOP_RED_REVIEW` live? | Awkward — belongs with WRITE conceptually (reviews what the agent wrote before the runtime touches it), but if it stays in `write_subprocess` then the "shared write" sub-process is no longer just write. If it moves to `red_mechanical_tail`, the tail starts with a review step disconnected from the WRITE it's reviewing. |
| Migration cost | High — structural change to the YAML, not just a rename. New sub-process, new edges, all 14 SVGs regenerated, possibly new tests. |

### Option 5 — six fully-specialised RED cycles (one per dispatch)

`red_test_cycle`, `red_dsl_cycle`, `red_driver_cycle`, plus the CT trio. Each could have a fully-specific WRITE node ID inside.

| | |
|---|---|
| BPMN-correct? | ✅ Most flexible long-term — each cycle can diverge freely. |
| WRITE node ID can be specific? | ✅ Yes — `WRITE_ACCEPTANCE_TEST` etc. become the actual node IDs inside their own cycles. |
| Mechanical-tail duplication | ❌ 14 nodes + 14 edges of mechanical tail × 6 = 84 nodes / 84 edges of duplication. Every change to the RED-phase rhythm becomes a 6-place edit. |
| Migration cost | Very high. |

## Recommendation (advisory only — human decides)

The author's lean: **Option 1 (keep current names)**, with **Option 3 (`PRODUCE_FAILING_*`)** as a defensible alternative if the human weights strict BPMN verb-object purity highly enough to justify the migration cost and the AT/CT canonical-vocabulary trade-off.

Option 2 (`WRITE_*`) is recommended against on BPMN grounds (names the wrong layer — the agent's slice — for a mostly-runtime activity).

Option 4 is recommended against today on cohesion grounds, with a possible re-evaluation later (see "When to revisit Options 4–5" below).

Option 5 is recommended against today on duplication grounds, same caveat.

This is an analysis, not a decision. The human is asked to weigh the trade-offs and pick.

## When to revisit Options 4–5

Today `red_phase_cycle` has one fork-by-param: `verify_real_required` is set only for `CT_RED_TEST` (via the `verify_real_suite` param). One fork is fine — the shared body is still readable. If two or three more accumulate (different STOP shapes per dispatch, different gate sequences per artifact type, divergent commit semantics), the shared body becomes harder to read than six small specialised bodies, and Options 4 or 5 become the right move.

This is a watch-this-space item — divergence count visible directly in `red_phase_cycle`'s YAML — not a scheduled action.

## Resolution

**(empty — to be filled in by the human)**

Once the human picks an option, record the choice here with one or two sentences of reasoning, and then add the action items below.

## Action items (conditional on chosen option)

### If Option 1 (keep current)

1. Add a short comment at the top of the first `call_activity` wrapper site in `internal/atdd/runtime/statemachine/process-flow.yaml` (likely `at_cycle.AT_RED_TEST`) explaining the BPMN Call Activity naming rule used throughout the file: wrapper names describe the work the called sub-process accomplishes as a whole, not one step inside it; the artifact-being-produced lives in `params.phase_label`, not in the wrapper ID. This prevents Options 2/3 from resurfacing on every fresh read of the file.
2. Cross-reference from `plans/20260501-144322-process-flow-node-id-rename-open-questions.md`: add a short subsection "Call activity wrappers — out of scope for the rename pass; see `20260507-210016-call-activity-wrapper-naming.md`" so the rename batch doesn't accidentally pick this up.

### If Option 2 (`WRITE_*`) or Option 3 (`PRODUCE_FAILING_*`)

1. Update node IDs in `internal/atdd/runtime/statemachine/process-flow.yaml` for all six (or twelve, with GREEN counterparts) call_activity wrappers.
2. Update sequence_flow `from`/`to` references in the same file.
3. Update `internal/atdd/runtime/statemachine/structural_cycle_test.go` and `behavioral_cycle_test.go` step-history expectations (the qualified trail entries change).
4. Update `internal/atdd/runtime/statemachine/transitions_test.go` references (table entries reference these IDs by name).
5. Update `internal/atdd/runtime/driver/driver.go` and `atdd_commands.go` if either references these IDs by string.
6. Regenerate the 14 SVG process diagrams under `docs/images/process-diagram-*.svg` and the source `docs/process-diagram.md` Mermaid.
7. Update `docs/atdd/process/*.md` references if any phase-label-style strings spill into prose (cross-check with grep).

### If Option 4 (split `red_phase_cycle`)

Larger structural plan — should be promoted to its own plan file before any work begins. Sketch only, not actionable here:

1. Define `write_subprocess` (single user_task WRITE + outgoing edge to caller) and `red_mechanical_tail` (existing red_phase_cycle minus WRITE).
2. Decide where `STOP_RED_REVIEW` lives (this plan flagged it as awkward in either subprocess).
3. Update at_cycle / ct_subprocess to dispatch the new `WRITE_*` wrapper followed by `red_mechanical_tail`.
4. Migration cost across YAML, tests, diagrams, docs — substantial.

### If Option 5 (six specialised cycles)

Same caveat as Option 4 — promote to its own plan file. Sketch only:

1. Duplicate `red_phase_cycle` six times into `red_test_cycle`, `red_dsl_cycle`, `red_driver_cycle`, `ct_red_test_cycle`, `ct_red_dsl_cycle`, `ct_red_external_driver_cycle`.
2. Specialise WRITE node ID per cycle.
3. Update at_cycle / ct_subprocess wrappers to point at their specialised cycles.
4. Accept the mechanical-tail duplication and document the rule for keeping them in sync.

## Out of scope (regardless of chosen option)

- Generic node IDs *inside* shared sub-processes (`WRITE`, `COMPILE`, `RUN`, etc.) when the sub-process is shared — must remain generic per the BPMN reusable-sub-process pattern; this only changes if Option 5 lands and the sub-processes are no longer shared.
- The CT counterparts `CT_RED_TEST` / `CT_RED_DSL` / `CT_RED_EXTERNAL_DRIVER` are treated as a single decision with the AT side; they share the same rationale and pick the same option.
- The GREEN counterparts (`AT_GREEN_BACKEND`, `AT_GREEN_FRONTEND`) — same rationale, same option, same migration cost; assume they ride along with whichever option the human picks for the RED side.
