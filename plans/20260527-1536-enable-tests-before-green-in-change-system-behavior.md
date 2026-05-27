# Enable acceptance tests before GREEN in `change-system-behavior`

## Context

The `change-system-behavior` CYCLE
(`internal/atdd/runtime/statemachine/process-flow.yaml:426-458`) chains
three steps:

1. **RED** — `write-and-verify-acceptance-tests-fail` (writes the failing
   AT, verifies it fails, then **disables** it before commit).
2. **GREEN** — `implement-and-verify-system` (runs `implement-system`,
   builds, starts, verifies all tests pass).
3. **REFACTOR** — `refactor` (loopable).

The RED step ends with `DISABLE_ACCEPTANCE_TESTS`
(`write-and-verify-acceptance-test-code:808` —
`VERIFY_TESTS_FAIL_ACCEPTANCE → DISABLE_ACCEPTANCE_TESTS →
COMMIT_TEST_CODE`). So by the time GREEN starts, the just-written AT is
disabled in the repo.

GREEN (`implement-and-verify-system:1012-1059`) is:

```
RUN_ACTION (implement-system) → BUILD_SYSTEM → START_SYSTEM →
VERIFY_TESTS_PASS (suite: "") → COMMIT_SYSTEM
```

There is no `ENABLE_TESTS` node. `VERIFY_TESTS_PASS` therefore runs
against a tree where the new AT is still disabled — it gets silently
skipped, the suite passes vacuously, and GREEN commits a system that
never actually had to make the new AT pass.

### Why this fix lands in the CYCLE, not in the HIGH

The natural-symmetric fix would be to add `ENABLE_TESTS` inside
`implement-and-verify-system` (one place, all four callers covered —
mirrors `implement-test-layer:1115-1195` which uses exactly that shape).
That option is **ruled out** by strict-mode `ExpandParams`
(`internal/atdd/runtime/statemachine/run.go:334-352`):

```go
if idx := strings.Index(s, "${"); idx >= 0 {
    end := strings.Index(s[idx:], "}")
    if end >= 0 {
        return s, fmt.Errorf("unresolved placeholder %s", s[idx:idx+end+1])
    }
}
```

Locked in by `TestExpandParams_UnresolvedPlaceholderErrors`
(`run_test.go:403`). The binding `test-names: ${test-names}` resolves
only if `test-names` is in `ctx.Params` or `ctx.State`; absence is fatal,
not silent-empty.

`implement-and-verify-system` has four callers
(`process-flow.yaml:438, 509, 547, 571`). Only `change-system-behavior`
upstream-emits `test-names` (via `write-acceptance-tests`' declared
output at line 1291). The three reshape/refactor callers
(`redesign-system-structure`, `redesign-external-system-structure`,
`refactor-system-structure`) never write tests, so a HIGH-level
`ENABLE_TESTS` with `test-names: ${test-names}` would crash all three on
dispatch with `unresolved placeholder ${test-names}`.

Placing the node in `change-system-behavior` instead:

- Confines the change to the one CYCLE that has the upstream disable.
- Uses the same `test-names: ${test-names}` binding shape as the
  symmetric `DISABLE_ACCEPTANCE_TESTS` upstream — same state-source, same
  resolution path, same precondition.
- Leaves the HIGH parametric and the three other callers untouched.

### Why not narrow `VERIFY_TESTS_PASS` to just the new AT

`implement-and-verify-system:1042-1043` binds `suite: ""` explicitly
(all suites). That is the non-regression guard — after GREEN's impl
lands, every suite is re-verified, not just the AT that triggered this
cycle. The just-enabled AT is included in "all suites," so narrowing
would lose regression coverage without gaining anything.

### Open questions resolved up-front

1. **CYCLE or HIGH?** CYCLE. Strict-mode `ExpandParams` rules out the
   HIGH placement (see above).
2. **Position within the CYCLE?** Between the RED node and the GREEN
   node — the symmetric counterpart to the DISABLE at the end of RED.
3. **`tdd-stage:` label on the new node?** None. It is a
   state-restoration step between stages, not a stage itself —
   equivalent in role to `compile-tests` / `start-system` plumbing
   nodes elsewhere, which also carry no `tdd-stage`.
4. **What if `test-names` is empty / unemitted?** Inherit the same
   precondition that the existing `DISABLE_ACCEPTANCE_TESTS` already
   relies on (line 780-785 — same binding, same source). If the
   AT-writer ever emits zero `test-names`, both the upstream disable
   and the new enable break together; this plan does not change that
   contract.
5. **Any caller other than `change-system-behavior` affected?** No.
   `cover-system-behavior`'s `expected-test-result: success` branch
   never disables; reshape/refactor CYCLEs never write tests. Only the
   RED-fail → GREEN-pass sequence inside `change-system-behavior` has
   the disabled-before-GREEN hazard.

## Items

### 1. Add `ENABLE_TESTS` node and rewire `change-system-behavior` sequence-flows

**Files touched:**

- `internal/atdd/runtime/statemachine/process-flow.yaml`
  (the `change-system-behavior:` block, lines 426-458)

**Change:** insert a new `ENABLE_TESTS` node between
`WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL` and
`IMPLEMENT_AND_VERIFY_SYSTEM`, and rewire the two adjacent
sequence-flows. Result (full block, new lines marked `# NEW` /
`# changed`):

```yaml
  change-system-behavior:
    name: "Change System Behavior"
    start: WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL
    nodes:
      - id: WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL
        type: call-activity
        process: write-and-verify-acceptance-tests-fail
        name: "Write Failing Acceptance Tests"
        tdd-stage: red

      # Symmetric counterpart to the DISABLE_ACCEPTANCE_TESTS at the    # NEW
      # end of RED (write-and-verify-acceptance-test-code:780-785).     # NEW
      # Re-enables the just-written AT before GREEN verifies — without  # NEW
      # this, VERIFY_TESTS_PASS in implement-and-verify-system runs     # NEW
      # against a still-disabled AT, passes vacuously, and GREEN        # NEW
      # commits a system that never had to satisfy the new behaviour.   # NEW
      - id: ENABLE_TESTS                                                # NEW
        type: call-activity                                             # NEW
        process: enable-tests                                           # NEW
        name: "Enable Tests"                                            # NEW
        params:                                                         # NEW
          test-names: ${test-names}                                     # NEW

      - id: IMPLEMENT_AND_VERIFY_SYSTEM
        type: call-activity
        process: implement-and-verify-system
        params:
          task-name: implement-system
          action: implement-system
        name: "Implement System"
        tdd-stage: green

      - id: REFACTOR_OPPORTUNISTICALLY
        type: call-activity
        process: refactor
        name: "Opportunistic Refactor (Loopable)"
        tdd-stage: refactor

      - id: CHANGE_SYSTEM_BEHAVIOR_END
        type: end-event
        name: "System Behavior Changed"

    sequence-flows:
      - {from: WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL, to: ENABLE_TESTS}              # changed
      - {from: ENABLE_TESTS,                           to: IMPLEMENT_AND_VERIFY_SYSTEM}# NEW
      - {from: IMPLEMENT_AND_VERIFY_SYSTEM,            to: REFACTOR_OPPORTUNISTICALLY}
      - {from: REFACTOR_OPPORTUNISTICALLY,             to: CHANGE_SYSTEM_BEHAVIOR_END}
```

The `# NEW` / `# changed` markers above are guidance for the executor —
do not bake them into the committed YAML; the actual block-comment to
keep is the 5-line rationale above the `ENABLE_TESTS` node.

### 2. Confirm structural-integrity tests cover the new node

**Files touched (audit only — no expected edits):**

- `internal/atdd/runtime/statemachine/transitions_test.go`

**Change:** none expected. The existing structural-integrity tests
(`TestStructuralIntegrity_StartNodesExist:104`,
`TestStructuralIntegrity_NonEndNodesHaveSuccessor:127`,
`TestStructuralIntegrity_GatewaysHaveOutgoingEdges:113`) walk every
process loaded from the embedded YAML, so they validate the new
`ENABLE_TESTS` node automatically: it has an incoming edge from
`WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL` and an outgoing edge to
`IMPLEMENT_AND_VERIFY_SYSTEM`, and it is not a gateway.

`TestLoadSnapshot_AllProcessesParse:34` enumerates expected processes,
not expected nodes within a process, so it requires no update.

Run `go test ./internal/atdd/runtime/statemachine/... -run
TestStructuralIntegrity -p 2` after the YAML edit lands to confirm.
(Memory: never run unbounded `go test ./...` on Windows — scope to the
package or use `-p 2`.)

### 3. Audit other call sites of `enable-tests` / `change-system-behavior` for inline-YAML fixtures

**Files touched (audit only — edit if matches found):**

- `internal/atdd/runtime/driver/driver_test.go`
- `internal/atdd/runtime/clauderun/clauderun_test.go`
- any other `*_test.go` under `internal/atdd/` containing an inline
  `change-system-behavior:` YAML block (none expected — `grep` at
  plan-authoring time found `change-system-behavior:` only in
  `process-flow.yaml`).

**Change:** for any inline fixture that mirrors the production
`change-system-behavior` node list, add the `ENABLE_TESTS` node and the
two sequence-flow edits from Item 1. For occurrences that name
`change-system-behavior` only as a string literal in a process-lookup
test (no inline node redefinition), leave untouched.

### 4. Do not regenerate diagrams in this plan

Per repository convention (memory: "Plans must not include diagram
regeneration steps"), the `regenerate-diagram` GitHub Actions workflow
will pick up the new node and update
`docs/process-diagram.md` plus
`docs/images/process-diagram-7-change-system-behavior.svg` on push to
`main`. No local regen step.

## Verification

Out-of-scope for agent execution; for the operator after Items 1-3 land:

- Run `go test ./internal/atdd/runtime/statemachine/... -run
  TestStructuralIntegrity -p 2` and confirm pass.
- Re-run an active rehearsal whose change-cycle reaches GREEN (e.g. the
  `gift-wrap an order` rehearsal at
  `worktrees/rehearsal-20260527-135607`). Inspect the trace log under
  `.gh-optivem/runs/<ts>/` and confirm:
  - An `enable-tests` MID dispatch appears between the
    `write-and-verify-acceptance-tests-fail` group and the
    `implement-and-verify-system` group.
  - The `test-names` parameter resolves to the AT name(s) emitted by
    `write-acceptance-tests` (e.g.
    `placingAnOrderWithGiftWrapShouldMarkItAsGiftWrapped,
    placingAnOrderWithGiftWrapShouldAddFiveDollarPackagingFee`).
  - The subsequent `VERIFY_TESTS_PASS` in
    `implement-and-verify-system` actually exercises the new AT (the AT
    appears in the JUnit/test-runner output rather than being skipped
    as `@Disabled`).

## Non-goals

- **Adding `ENABLE_TESTS` to `implement-and-verify-system`.** Ruled out
  by strict-mode `ExpandParams` (see Context). The three reshape /
  refactor callers do not emit `test-names`, so a HIGH-level binding
  would crash them on dispatch.
- **Widening `implement-and-verify-system`'s `VERIFY_TESTS_PASS` to
  filter on `test-names`.** Keeping `suite: ""` (all suites) is
  deliberate — the GREEN verify is the non-regression guard, and the
  just-enabled AT is included in the broad sweep.
- **Touching `cover-system-behavior` or the reshape/refactor CYCLEs.**
  None of them disable tests upstream, so none need an enable.
- **Changing the `enable-tests` MID itself** (`process-flow.yaml:1583`).
  Its read/write scope (`[at-test, ct-test]`) and `test-enabler` agent
  binding are correct for this use case as-is.
- **Promoting `test-names` from `optional: true` to required on
  `write-acceptance-tests`**
  (`process-flow.yaml:1291-1293`). The existing
  `DISABLE_ACCEPTANCE_TESTS` already relies on the same binding; this
  plan inherits the precondition, it does not tighten it.

## Cross-references

- Symmetric upstream disable:
  `write-and-verify-acceptance-test-code:780-808` (the
  `DISABLE_ACCEPTANCE_TESTS` node and its sequence-flow).
- Canonical "enable-tests right after the agent runs" precedent:
  `implement-test-layer:1124-1129` and the comment cluster at lines
  1108-1114.
- Strict-mode `ExpandParams`: `run.go:334-352`, locked in by
  `run_test.go:403` (`TestExpandParams_UnresolvedPlaceholderErrors`)
  and the `${suite}` hazard comment at
  `process-flow.yaml:1037-1041`.
- HIGH being parameterised across four callers:
  `process-flow.yaml:438` (CSB), `509` (redesign-system),
  `547` (redesign-external-system), `571` (refactor-system-structure).
- Related but separate plan (read-scope widening on the same MID):
  `plans/20260527-1507-widen-implement-system-read-scope.md` — does
  not overlap; that plan touches `implement-system`'s read list, this
  plan touches `change-system-behavior`'s node graph.
