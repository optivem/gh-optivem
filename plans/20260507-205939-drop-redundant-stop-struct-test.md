# Drop `STOP_STRUCT_TEST` — `ASK_COMMIT` already covers the same gate

## Symptom

In the structural cycle the operator hits two consecutive prompts before
any commit:

1. `STOP_STRUCT_TEST` (`agent: human, role: review`) → "STOP — press
   Enter to continue, or type `abort` to halt".
2. `ASK_COMMIT` (action `ask_can_i_commit`) → "Can I commit? [y/N]".

Press Enter, then type `y`, then the commit happens. Two prompts where
one would do.

## Root cause

The structural cycle has *two* pre-commit gates with overlapping intent:

| Node | Type | Stated purpose |
| --- | --- | --- |
| `STOP_STRUCT_TEST` (`process-flow.yaml:701-705`) | `user_task` `agent: human, role: review` | Passive pause: "review test/compile output before continuing." |
| `ASK_COMMIT` (`bindings.go:707-714`, action `ask_can_i_commit`) | `service_task` (action prompts inside) | Explicit Y/N gate: *"the explicit 'ask before every commit' gate from user memory"* (per the binding's doc comment). |

`ASK_COMMIT` is load-bearing because of the user-memory rule "ask before
every commit, halt on no". `STOP_STRUCT_TEST` predates that gate and
became redundant once `ASK_COMMIT` was added — the operator can review
the just-scrolled output *while* deciding y/N at `ASK_COMMIT`, so the
extra Enter at `STOP_STRUCT_TEST` doesn't add a real review opportunity.

For comparison, the AT cycle's commit gate is single-step:
`STOP_GREEN_REVIEW → COMMIT_GREEN` (`at_green_system`,
`process-flow.yaml` / transitions_test.go:218-219). No `ASK_COMMIT`
between them, no double prompt. Structural is the outlier.

## Reachability today

`STOP_STRUCT_TEST` is reached on three branches; all flow into
`ASK_COMMIT`:

- `GATE_TEST_MODE compile → COMPILE → STOP_STRUCT_TEST → ASK_COMMIT` —
  compile-only mode, the operator sees the build output.
- `RUN_TESTS → GATE_STRUCT_VERIFY ok → STOP_STRUCT_TEST → ASK_COMMIT` —
  full mode after a green test run.
- (`GATE_TEST_MODE skip → ASK_COMMIT` already bypasses `STOP_STRUCT_TEST`,
  precedent for the consolidated shape.)

The red branch goes via `STOP_STRUCT_VERIFY_REVIEW → FIX_STRUCT_VERIFY →
CHOOSE_TESTS` (the fix loop) and never reaches `STOP_STRUCT_TEST`. So
`STOP_STRUCT_TEST` is exclusively a green-path / compile-only-path pause.

## Proposal

Delete `STOP_STRUCT_TEST` from `structural_cycle`. Re-target every edge
that currently points at it directly at `ASK_COMMIT`.

**YAML changes** (`internal/atdd/runtime/statemachine/process-flow.yaml`):

- Delete the `STOP_STRUCT_TEST` node (lines 701-705).
- Sequence-flow rewrites:
  - `{from: COMPILE, to: STOP_STRUCT_TEST, when: "structural_test_mode == compile"}` → `{from: COMPILE, to: ASK_COMMIT, when: "structural_test_mode == compile"}`
  - `{from: GATE_STRUCT_VERIFY, to: STOP_STRUCT_TEST, when: "structural_verify_outcome == ok"}` → `{from: GATE_STRUCT_VERIFY, to: ASK_COMMIT, when: "structural_verify_outcome == ok"}`
  - Delete `{from: STOP_STRUCT_TEST, to: ASK_COMMIT}` (now unreachable).

**Test updates:**

- `internal/atdd/runtime/statemachine/transitions_test.go` —
  - Replace the `from: COMPILE, to: STOP_STRUCT_TEST, [compile]` row with `from: COMPILE, to: ASK_COMMIT, [compile]`.
  - Replace the `from: GATE_STRUCT_VERIFY, to: STOP_STRUCT_TEST, [ok]` row with `from: GATE_STRUCT_VERIFY, to: ASK_COMMIT, [ok]`.
  - Delete the `from: STOP_STRUCT_TEST, to: ASK_COMMIT` row.
- `internal/atdd/runtime/statemachine/structural_cycle_test.go` — drop
  `"structural_cycle.STOP_STRUCT_TEST"` from the expected `want` trail.
- `internal/atdd/runtime/statemachine/transitions_test.go` comment block
  — update the "structural-cycle escape" comment that still mentions
  STOP_STRUCT_TEST in the bypassed-set.

**Diagram regeneration:** `go run . atdd show diagram >
docs/process-diagram.md` (the regenerate-diagram workflow does this on
push to main; do it locally for any other branch).

**No Go-binding changes** — `STOP_STRUCT_TEST` resolved via `humanStop`,
which stays registered for every other `agent: human` STOP.

## Trade-offs

**Pros:**

- One pre-commit gate instead of two (same as AT cycle).
- BPMN-cleaner: no two consecutive "are you sure?" gates with
  overlapping intent.
- Fewer keystrokes for the operator on every structural cycle.

**Cons:**

- Loses the explicit "review TEST results" label on the diagram. The
  operator still has the same review window (between test output
  scrolling by and the `Can I commit?` prompt), but the BPMN diagram
  no longer names that pause as a distinct activity. Mitigated by the
  fact that `ASK_COMMIT` is itself a halt-on-input — it *is* the
  review window.
- Anyone whose mental model expected a "review-before-commit-question"
  separation may need to re-orient. Acceptable: the AT cycle already
  works this way.

## Out of scope

- Whether `ASK_COMMIT` should itself be remodelled as a `user_task`
  (it's a service_task that prompts internally — a BPMN smell, but
  shared by the legacy_acceptance / commit paths and not unique to
  structural). Separate concern; not blocked by this change.
- Whether the at_green_system cycle needs an `ASK_COMMIT`-style explicit
  gate (currently `STOP_GREEN_REVIEW → COMMIT_GREEN` directly, which
  arguably violates the "ask before every commit" rule). Separate
  decision.

## Acceptance

- `go build ./...` clean.
- `go test -p 2 ./internal/atdd/...` green.
- `docs/process-diagram.md` regenerated and matches the YAML.
- `go run . atdd show diagram` shows three branches into `ASK_COMMIT`
  (skip / compile / verify-ok) with no `STOP_STRUCT_TEST` node.
