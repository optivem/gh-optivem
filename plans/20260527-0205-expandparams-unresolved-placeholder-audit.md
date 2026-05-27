# Audit unresolved-placeholder leaks in ExpandParams + make it strict

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-05-27T07:16:04Z`

## Decisions (2026-05-27, batch-resolved before execution)

- **`${failure}` leak in `fix:2129` → Option A.** Rewrite the question to use `${failure-kind}`. Zero new bindings; `feedback_schema_fields_earn_slot` rules out the alias-only Option B; Option C is speculative.
- **Doctrine comment (Item 4) → fold into Item 2's commit.** Lands the rule in the YAML at the same time the strict-mode runtime goes live, so a future reader hitting `unresolved placeholder ${suite}` finds the comment immediately.
- **Out-of-band rehearsal confirmation → non-blocking.** The audit is the systematic check; strict-mode `ExpandParams` will surface any other latent placeholder at first dispatch with a precise error message. No need to wait for a manual rehearsal pass.

## Why

The ATDD-rehearsal trace observed at 2026-05-27 ~01:55 CEDT executed:

```
gh optivem test run --suite='${suite}' --test='[shouldRejectOrderWithQuantityOf100]'
```

— the literal `${suite}` reached the CLI because `ExpandParams`
(`internal/atdd/runtime/statemachine/run.go:316-328`) silently leaves
unresolved placeholders in place when neither `params` nor `state`
contains the key. `runCommand` (`internal/atdd/runtime/actions/bindings.go:724`)
then saw a non-empty string and appended `--suite='${suite}'`, which
`gh optivem test run` rejected.

The surgical fix (commit `bd1c958`) bound `suite: ""` at the two
no-param callers of `verify-tests-pass`
(`implement-and-verify-system` line 1039, `refactor-and-verify-tests`
line 1081) so the literal no longer leaks. That fix is local to two
sites; the underlying class of bug — "any unresolved `${name}` becomes
a literal string that downstream consumers treat as a real value" — is
systemic and will re-appear at the next call-site that omits a
forwarded param.

This plan does two things:

1. **Audit** every `${name}` reference in `process-flow.yaml` against
   every caller's params/state chain to find any remaining latent
   leak.
2. **Tighten the runtime** so the bug class can't recur: `ExpandParams`
   errors on unresolved placeholders, plumbed through dispatch with a
   precise "node X in process Y references unresolved ${name}" message,
   plus a regression test covering the
   `implement-and-verify-system` → `verify-tests-pass` empty-binding
   path.

## Scope

In scope:

- `internal/atdd/runtime/statemachine/process-flow.yaml` — every
  `${name}` reference and every call-activity params block.
- `internal/atdd/runtime/statemachine/run.go` — `ExpandParams` and
  `wrapCallActivity` (the call-activity params-push site).
- `internal/atdd/runtime/actions/bindings.go` — `runCommand` and any
  other action that reads `ctx.Params[...]` as a flag value, to confirm
  the strict mode doesn't double-up on existing defensive checks.
- `internal/atdd/runtime/clauderun/clauderun.go` —
  `findUnfilledPlaceholders` is the parallel mechanism for agent prompts;
  align so the two checks live at the same layer.
- `internal/atdd/runtime/statemachine/run_test.go` — regression tests.

Out of scope:

- The disable-marker examples change in `clauderun.go` /
  `test-disabler.md` / `test-enabler.md` / `typescript.md`
  (parallel-session work; do not bundle).
- Agent prompt-body placeholders (`findUnfilledPlaceholders` already
  errors fast for those; only the YAML-level params expansion is
  silently permissive).

## Items

### 2. Make `ExpandParams` strict

Change `ExpandParams` to return `(string, error)`:

- After both passes (params, then state), scan for any remaining
  `${...}` substring. If found, return an error naming the unresolved
  key.
- Update every caller to propagate the error. Callers found by grep
  (from the diagnosis):
  - `statemachine/run.go` (line 58 — service-task action template; line
    298 — wrapCallActivity params push; other sites)
  - `actions/bindings.go` (runCommand and any sibling action that calls
    ExpandParams)
  - `clauderun/clauderun.go`
  - `agents/embed.go`
  - `driver/driver.go`
  - `trace/trace.go`
  - `statemachine/context.go`
- Plumb the error to the dispatcher so the runtime fails fast with a
  message like
  `unresolved placeholder ${suite} at node VERIFY_TESTS_PASS in process implement-and-verify-system`.

Risk: any silently-unresolved placeholder that currently works because
its downstream consumer happens to tolerate the literal will now fail.
This is precisely why Item 1 (audit) precedes Item 2 — surface the
sites first, fix them with explicit empty bindings, then flip the
runtime to strict.

### 3. Add regression tests

In `internal/atdd/runtime/statemachine/run_test.go`:

- Unit test: `ExpandParams("${foo}", nil, nil)` returns a non-nil error
  mentioning `foo`.
- Unit test: `ExpandParams("${foo}", {"foo": ""}, nil)` returns `""`
  (empty value is a valid binding; the error fires only on unresolved
  keys, not empty values).
- Integration-style test: dispatch
  `implement-and-verify-system` with no upstream `suite` binding,
  assert the inner `runCommand` call receives `ctx.Params["suite"] ==
  ""` and the rendered command line is `gh optivem test run` with no
  `--suite=` flag.

### 4. Update doctrine comment in `run-tests`

The existing comment at `process-flow.yaml:1819-1825` says "Both absent
⇒ run all tests" — accurate but doesn't mention the
explicit-binding-required rule. Append a line: every caller MUST bind
`suite` and `test-names` to a string (use `""` to mean "all"); omitting
the param is no longer accepted by the strict runtime.

### 5. Coordinate with the parallel-session changes

The dirty tree at audit-plan-write time contains a parallel session's
edits to `clauderun.go` / `test-disabler.md` / `test-enabler.md` /
`typescript.md` (disable-marker examples). Those don't touch
`ExpandParams` or the YAML, but the strict-mode change in
`clauderun.go` callers (Item 2) may conflict with the parallel
session's edits to the same file. Sequence: wait for the parallel
session to commit, rebase, then proceed.

## Out-of-band: confirm rehearsal can proceed

The current rehearsal trace was paused at the `[ASK_HUMAN]` prompt for
`unexpected-failing-tests-fixer`. With commit `bd1c958` in place, a
fresh rehearsal run from the same point should no longer hit the
`$suite` error. Confirm before starting this plan.

## Sequencing

1. ~~(Item 1) Audit~~ — **done 2026-05-27.** See "Audit results" below.
2. ~~Close the `${failure}` leak via Option A — rewrite to `${failure-kind}`.~~ **Done.**
3. (Item 2) Strict-mode `ExpandParams`. Single commit, plumbed through
   every caller. Doctrine comment (Item 4) folded in.
4. (Item 3) Regression tests. Single commit.

## Audit results (Item 1 — completed 2026-05-27)

Audit run against `process-flow.yaml` HEAD (`bd1c958`). Each row pairs
a `${placeholder}` reference with the subprocess that uses it, the
binding mechanism that resolves it at dispatch, and the residual risk
under strict-mode `ExpandParams`.

| Subprocess | Placeholder | Reference site | Binding mechanism | Risk under strict mode |
|------------|-------------|----------------|-------------------|------------------------|
| `approve` | `${question}` | line 1962 | callers at 1996, 2000-ish, 2068, 2129 pass literal | none |
| `verify-tests-pass` | `${suite}` | line 1220 | `suite: ""` bound at impl-and-verify-system:1050 + refactor:1098 (bd1c958) | none |
| `verify-tests-pass` | `${test-names}` | line 1221 | state — stashed by writing-agent output declaration; consumer is `optional: true` | none |
| `verify-tests-fail` | `${suite}` | line 1259 | as above (bd1c958) | none |
| `verify-tests-fail` | `${test-names}` | line 1260 | state — as above | none |
| `run-tests` | `${suite}`, `${test-names}` | lines 1850-1851 | via state — set by verify-tests-{pass,fail} caller chain | none |
| `implement-and-verify-dsl` | `${expected-test-result}`, `${tests}` | lines 684-685 | callers forward / literal `tests: 'acceptance'` | none |
| `implement-and-verify-system-driver-adapters` | `${expected-test-result}`, `${tests}` | lines 698-699 | forwarded / literal | none |
| `implement-and-verify-external-system-driver-adapters` | `${expected-test-result}`, `${tests}` | lines 712-713 | forwarded / literal | none |
| `implement-test-layer` | `${action}` | line 1129 | templated process name — callers (change-system-behavior:440, etc.) bind `action:`; resolves via call-activity dispatch | confirm strict-mode covers dispatch path at `run.go:58` |
| `implement-test-layer` | `${test-names}` | lines 1137, 1163, 1170, 1178 | state — writing-agent output | none |
| `implement-test-layer` | `${expected-test-result}`, `${tests}`, `${cycle_phase}` | 825/850/872 etc. | forwarded | none |
| `implement-and-verify-system` | `${action}` | line 1026 | templated dispatch, same shape as above | confirm dispatch path |
| **`fix`** | **`${failure}`** | **line 2129** (`question:`) | **NONE** — no caller (1996, 2092) binds `failure`; only `failure-kind` exists in state | **LEAK — strict mode would break the approve question render** |
| `fix` | `${failure-kind}` | lines 2150, 2151 | via state — `runCommand:741` or `validateOutputsAndScopes:897/946` writes it before every reachable failure path | none |
| `execute-agent` | `${agent}`, `${task-name}` | lines 1976-1977, 1962, 2004 | callers bind explicitly | none |
| `execute-command` | `${command}` | lines 2068, 2078 | callers bind explicitly | none |

### Findings

1. **One residual leak:** `${failure}` in `fix:2129` (the human-readable
   `question:` passed into `approve`). Today it renders as the literal
   string `"Do you approve fix to attempt remediation for ${failure} ?"`
   — ugly but functional, because the approve gate routes on
   `approval-outcome`, not on the question text. Under strict-mode
   `ExpandParams` this becomes a dispatch-time error.
2. **No `${suite}`-shaped leaks remain.** The bd1c958 fix is
   exhaustive — all `verify-tests-pass`/`-fail` call-sites now bind
   `suite` explicitly or inherit it from a binding caller.
3. **State-fallback writers are sound.** `failure-kind` and
   `test-names` are written on every reachable path that reaches a
   consumer; no unguarded branches.
4. **Templated `${action}` dispatch needs confirmation in Item 2.** The
   call-activity dispatcher at `run.go:58` resolves `process: ${action}`
   through `ExpandParams`; flipping the function strict must not break
   that path.
5. **Strict-mode flip is safe once the `${failure}` leak is closed**
   (or the question is rewritten to use `${failure-kind}`).

## References

- Commit `bd1c958` — surgical fix for the two no-param `verify-tests-pass` callers.
- Memory `feedback_paths_deterministic_no_guessing` — values must be deterministic, not guessed.
- Memory `feedback_schema_fields_earn_slot` — silent empty-substitution is a slot that doesn't earn its keep.
- `process-flow.yaml:1819-1825` — run-tests doctrine comment ("Both absent ⇒ run all tests").
- `internal/atdd/runtime/statemachine/run.go:316-328` — current ExpandParams implementation.
- `internal/atdd/runtime/actions/bindings.go:724` — runCommand's suite-flag emission.
