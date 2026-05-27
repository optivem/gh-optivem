# Audit unresolved-placeholder leaks in ExpandParams + make it strict

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

### 1. Audit every `${name}` against its caller chain

Goal: enumerate every `${name}` reference in `process-flow.yaml`, the
subprocess that references it, every call-site of that subprocess, and
whether each caller binds the placeholder (directly or via state
fallback). Produce a table in this file showing where the contract
holds and where it leaks.

Method:

- Grep `process-flow.yaml` for `\$\{[a-zA-Z_-]+\}` and group by
  referencing subprocess.
- For each subprocess, list call-sites (the `process: <name>` invocations)
  and inspect each one's `params:` block.
- For placeholders that fall back to `ctx.State` (e.g. `${failure-kind}`,
  `${test-names}`), trace the writer that stashes the key (typically
  `validateOutputsAndScopes` or `runCommand` in `bindings.go`) and
  confirm the writer fires on every reachable path leading to the
  consumer.

Output: an Audit Table section in this plan listing for each
placeholder: `subprocess` / `placeholder` / `caller site` / `binding
present?` / `state-fallback writer` / `risk`.

Known leak from the rehearsal incident: confirmed only the two sites
just patched (commit `bd1c958`). The audit must confirm no other sites
share the same shape.

Likely-safe-but-needs-verification candidates (from initial grep):

- `${tests}` in `implement-test-layer` (lines 1148, 1156) — every caller
  at 826/851/873 passes `tests: ${tests}` which itself depends on the
  caller-of-the-caller binding `tests`. Trace one level up.
- `${question}` in `approve` (line 1891) — every caller passes
  `question:` literally. Verify.
- `${failure}` / `${failure-kind}` in `fix` (lines 2115/2128/2136/2137)
  — state-fallback via `validateOutputsAndScopes` and `runCommand`.
  Verify both writers fire on every reachable failure path.
- `${action}` template in `process: ${action}` for
  `implement-and-verify-system` (line 1026) and `implement-test-layer`
  (line 1115) — different shape (templated process name, not param
  value). The runtime resolves it via the call-activity dispatch path
  at `run.go:58`; verify ExpandParams strict mode covers that site too.
- `${expected-test-result}` — appears in many params blocks; verify
  every caller binds it (callers at 825/850/872/etc.).

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

1. (Item 1) Audit — fill in the Audit Table below. ~30 min of focused
   grep + read.
2. For every leak the audit finds, add the corresponding `params: { X: "" }`
   binding at the broken call-site, mirroring the surgical fix in
   commit `bd1c958`. Commit per-batch.
3. (Item 2) Strict-mode `ExpandParams`. Single commit, plumbed through
   every caller.
4. (Item 3) Regression tests. Single commit.
5. (Item 4) Doctrine comment update. Folded into the same commit as
   Item 2 or 3.

## Audit Table (to be filled in during Item 1)

| Subprocess | Placeholder | Caller site (file:line) | Binding present? | State-fallback writer | Risk |
|------------|-------------|-------------------------|------------------|-----------------------|------|
| _(populate during Item 1)_ | | | | | |

## References

- Commit `bd1c958` — surgical fix for the two no-param `verify-tests-pass` callers.
- Memory `feedback_paths_deterministic_no_guessing` — values must be deterministic, not guessed.
- Memory `feedback_schema_fields_earn_slot` — silent empty-substitution is a slot that doesn't earn its keep.
- `process-flow.yaml:1819-1825` — run-tests doctrine comment ("Both absent ⇒ run all tests").
- `internal/atdd/runtime/statemachine/run.go:316-328` — current ExpandParams implementation.
- `internal/atdd/runtime/actions/bindings.go:724` — runCommand's suite-flag emission.
