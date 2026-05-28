# Plan: Phase-qualify BPMN commit messages

## Context

Spun off from `plans/20260527-1052-bpmn-commit-message-binding.md` (the rehearsal-unblocking change that binds `[${ticket_id}] ${issue_title}` to all four BPMN commit sites).

That plan deliberately scoped down to a single uniform message across all four call sites so the rehearsal stops failing on `commit message is required`. It left the phase suffix out because the grain is a separate decision.

Goal here: enrich the four messages so an operator scanning `git log` on a rehearsal branch can tell which BPMN phase produced each commit.

## Prerequisite

`plans/20260527-1052-bpmn-commit-message-binding.md` must land first. This plan assumes:

- The `commit` subprocess has `message: ${message}` as a required input.
- All four call sites (`COMMIT_TEST_CODE`, `COMMIT_SYSTEM`, `COMMIT_TESTS`, `COMMIT_LAYER`) already bind `message:`.
- `runCommand` already splices `ctx.Params["message"]` via `shellEscape` for `gh optivem commit`.

## Items

### Item 1 — Decide the phase grain

Pick one of the two shapes for `COMMIT_LAYER` (the only site that runs across multiple suites + multiple phases):

| Shape                                        | Example                                                       | Tradeoff                                                                                                 |
|----------------------------------------------|---------------------------------------------------------------|----------------------------------------------------------------------------------------------------------|
| **A.** `… - ${cycle_phase} layer`            | `[69] Add product search - SYSTEM DRIVER layer`               | Minimal — one extra param dep, but ambiguous in a multi-suite rehearsal (DSL layer for which suite?).    |
| **B.** `… - ${suite} ${cycle_phase} layer`   | `[69] Add product search - contract-real SYSTEM DRIVER layer` | Self-disambiguating in `git log`. Costs one more param binding at `COMMIT_LAYER` (`suite: ${suite}`).    |

The other three sites are not parameterised by suite/phase, so they use fixed-string suffixes regardless:

| Site               | Suffix                       |
|--------------------|------------------------------|
| `COMMIT_TEST_CODE` | `- acceptance test code`     |
| `COMMIT_SYSTEM`    | `- system implementation`    |
| `COMMIT_TESTS`     | `- test refactor`            |

**Recommendation:** Shape B. Operators reading a long rehearsal branch's `git log` should not need to cross-reference the BPMN to know which suite a `DSL layer` commit belongs to. `${suite}` is already in scope at `COMMIT_LAYER`'s caller path (see `process-flow.yaml:1858` where `run-tests` consumes `${suite}`).

### Item 2 — Wire the suffix at the four call sites

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

Update each `process: commit` call-activity to extend its `message:` literal:

```yaml
# COMMIT_TEST_CODE (~:789)
params:
  message: "[${ticket_id}] ${issue_title} - acceptance test code"

# COMMIT_SYSTEM (~:1053)
params:
  message: "[${ticket_id}] ${issue_title} - system implementation"

# COMMIT_TESTS (~:1100)
params:
  message: "[${ticket_id}] ${issue_title} - test refactor"

# COMMIT_LAYER (~:1182) — under Shape B
params:
  message: "[${ticket_id}] ${issue_title} - ${suite} ${cycle_phase} layer"
```

If Shape A wins instead, drop `${suite}` from the COMMIT_LAYER line.

### Item 3 — Verify `${suite}` reaches `COMMIT_LAYER` (Shape B only)

`${cycle_phase}` is already bound by callers of `implement-test-layer` (`process-flow.yaml:830, 852, 874`). Confirm `${suite}` is too — grep the same call sites; if any caller binds `cycle_phase` but not `suite`, add the binding there, or fall back to Shape A.

### Item 4 — Tests

**File:** `internal/atdd/runtime/actions/bindings_test.go`

Update the simple-message case from `plans/20260527-1052-…`'s Item 4 to use the suffixed shape, e.g. `[69] Add product search - acceptance test code`.

**File:** `internal/atdd/runtime/statemachine/run_test.go`

If any fixture asserts the dispatched command line for the `commit` subprocess, update the expected suffix.

### Item 5 — Re-run the rehearsal

`bash scripts/atdd-rehearsal.sh 69 --config gh-optivem-monolith-typescript.yaml` and inspect `git log` on the rehearsal branch — expect phase-qualified commits across all four sites.

## Out of scope

- **Writing-agent-authored commit messages.** Same exclusion as the parent plan — letting agents emit a `commit-message` output is a richer change tracked separately.
- **Branch/PR title alignment.** Branch naming and the agent's PR title carry their own conventions; this plan only touches commit-message construction.

## Verification

- `go test ./internal/atdd/...` passes (use `-p 2` per Windows test memory).
- `git log` on a fresh rehearsal branch shows phase-qualified commits.
