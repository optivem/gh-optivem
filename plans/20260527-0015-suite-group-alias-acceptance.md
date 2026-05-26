# Suite-group alias `acceptance` in `gh optivem test run`

**Filed:** 2026-05-27

## Cross-references

- **`internal/atdd/runtime/statemachine/process-flow.yaml:761,769`** — the two `suite: acceptance` literals in `write-and-verify-acceptance-test-code` that currently produce `--suite=acceptance` and crash with `ERROR: suite(s) not found: acceptance`.
- **`internal/atdd/runtime/testselect/suite.go:60–69`** — `AcceptanceSuites()` already returns `["acceptance-api", "acceptance-ui"]`. The doc-comment explicitly anticipates this work: *"A future channel-execution plan may introduce sentinel suites like `<acceptance>` that union these explicitly."*
- **`test_commands.go:91`** — `cmd.Flags().StringSliceVar(&suites, "suite", ...)`; the flag already accepts a list, so the only missing piece is alias expansion.
- **`test_commands.go:140–148`** — `validateSuiteTestCombo` currently rejects `--test` alongside multiple `--suite` values; the alias-expansion path needs to bypass or relax this for group expansions.
- **`internal/atdd/runtime/actions/bindings.go:724–725`** — `runCommand` formats `--suite=…` from `ctx.Params["suite"]`; no change needed (it stays single-valued; the binary does the expansion).

## Problem

The `--suite=acceptance` arg passed by BPMN's `VERIFY_TESTS_PASS_ACCEPTANCE` / `VERIFY_TESTS_FAIL_ACCEPTANCE` is a stale alias that the binary never learnt: the registered suite ids are `acceptance-api`, `acceptance-ui`, `acceptance-isolated-api`, `acceptance-isolated-ui`, etc. — never bare `acceptance`. `RUN_COMMAND` crashes:

```
ERROR: suite(s) not found: acceptance. Available: smoke-stub, smoke-real,
acceptance-api, acceptance-ui, acceptance-isolated-api, acceptance-isolated-ui,
contract-stub, contract-stub-isolated, contract-real, e2e-api, e2e-ui
```

Hard-coding two BPMN nodes per leg (one for `acceptance-api`, one for `acceptance-ui`) would work but duplicates the call-activity and embeds the suite-set decision in process-flow.yaml. The suite ids are binary-defined and `AcceptanceSuites()` already exists as the canonical list — the grouping belongs in the binary, not the BPMN.

## Decision

Teach `gh optivem test run` that `--suite=acceptance` is a **group alias** that expands to the suites returned by `AcceptanceSuites()`. The BPMN literal at process-flow.yaml:761,769 starts working as-is. No new yaml schema slot, no new BPMN nodes, no operator-configurable groups.

The grouping lives with the suite list (binary-defined). Adding a new acceptance suite later (e.g. `acceptance-mobile`) is a one-line edit in `AcceptanceSuites()`.

## Items

### Item 1 — Public alias-resolution helper

**Where:** new function + package-level registry in `internal/atdd/runtime/testselect/suite.go` (alongside `AcceptanceSuites`).

**Registry shape — decided: package-level map.** The alias concept is generic; a map is the natural structure. One entry today (`"acceptance"`), adding `contract` / `e2e` later is a one-line edit with no shape change.

```go
// suiteGroups is the registry of group-alias names. Each alias maps to
// the canonical suite ids it expands to. Today the only group is
// "acceptance"; the registry is shaped this way so adding contract /
// e2e groups later is a one-line edit.
var suiteGroups = map[string][]string{
    "acceptance": AcceptanceSuites(),
}

// ExpandSuiteGroups maps known group-alias names from `suiteGroups` to
// their constituent suite ids and passes any non-alias name through
// unchanged. Duplicates after expansion are de-duped while preserving
// first-seen order. Unknown names pass through so that the downstream
// "suite(s) not found" check in the runner still catches typos.
func ExpandSuiteGroups(names []string) []string { ... }
```

- Map evaluated at package init — `AcceptanceSuites()` returns a fixed list, no init-order hazard.
- Pass-through for non-alias names so plain suite ids like `acceptance-api` work unchanged.
- De-dupe preserving order: `--suite=acceptance --suite=acceptance-api` → `[acceptance-api, acceptance-ui]` (acceptance-api seen once).

**Tests:** new `suite_test.go` cases:
- empty input → empty output
- pure alias → expanded
- mixed alias + explicit → expanded, deduped
- unknown name passes through (no error here; the existing "suite(s) not found" check downstream still catches genuine typos)

### Item 2 — Wire expansion into `test run`

**Where:** `test_commands.go`, `newTestRunCmd()`.

**Layer decision — expansion at CLI surface, not in `runner`.** The runner stays pure: `runner.TestOptions{Suite: ...}` only ever sees canonical suite ids. Aliases are sugar at the CLI boundary. This keeps `runner` testable in isolation and prevents alias semantics from leaking into the test-runner core.

**Order — validate raw, then expand.** See Item 3 — `validateSuiteTestCombo` runs on the **pre-expansion** slice. Expansion happens *after* validation, on the way into `runner.TestOptions`.

```go
// validate operator intent (raw input)
exitOnError(validateSuiteTestCombo(suites, test))
// expand aliases before handing to the runner
suites = testselect.ExpandSuiteGroups(suites)
opts := runner.TestOptions{Suite: suites, Test: test, Sample: sample}
```

### Item 3 — No `validateSuiteTestCombo` change needed

**Resolved.** Earlier drafts of this plan proposed loosening the validator. That's unnecessary if expansion happens *after* validation (per Item 2). The existing rule already does the right thing:

- Operator types `--suite=acceptance --test=foo` → raw is `["acceptance"]` (single value), `--test` is allowed, validator passes. Expansion then yields `[acceptance-api, acceptance-ui]` for the runner.
- Operator types `--suite=acceptance-api --suite=acceptance-ui --test=foo` → raw is `[acceptance-api, acceptance-ui]` (two values), `--test` is rejected with the existing error.

Validator keeps its current signature (`rawSuites, tests`). No code change here — but add one test case in `test_commands_test.go` confirming the `--suite=acceptance --test=foo` path is accepted by validation (regression guard against a future "tighten the validator" patch that forgets the alias case).

### Item 4 — Update `--help` examples and short-help text

**Documentation surface — decided: both Examples and flag Usage text, with the doc-comment on `ExpandSuiteGroups` as the canonical source.** Help text echoes the doc-comment; the registry map in `suite.go` is the literal source of truth for which aliases exist.

**Where:** `test_commands.go:60–66` `Example:` block; `test_commands.go:91` `--suite` flag Usage string.

**What:** add one example line:

```
  gh optivem test run --suite acceptance --test shouldRejectOrderWithQuantityOf100
```

…and extend the `--suite` flag `Usage` text:

```
"Run only the suite(s) with these id(s); repeatable, also accepts comma-separated values, and the group alias `acceptance` (expands to all acceptance-* suites)"
```

**Out of scope (intentional):** `gh optivem test run --list` continues to print only registered suite ids from tests.yaml, not aliases. Mixing aliases into `--list` would conflate two concepts (registered suites vs. CLI sugar). Aliases are advertised through `--help` only.

### Item 5 — Smoke-verify the BPMN path

**Verification mode — decided: manual run + captured trace.** An automated end-to-end BPMN test would need a fixture harness that isn't built today; bundling that work here would inflate the plan. The unit-level guarantee (Items 1, 3) plus a one-shot manual run is sufficient for this plan; a fuller BPMN integration-test harness is its own future plan.

**What:** run the originally-failing command from the trace —

```
gh optivem test run --suite=acceptance --test=shouldRejectOrderWithQuantityOf100
```

— and confirm it dispatches against both `acceptance-api` and `acceptance-ui`. Then exercise `VERIFY_TESTS_PASS_ACCEPTANCE` end-to-end on a fixture to confirm the BPMN node now passes through `RUN_COMMAND` without changes to process-flow.yaml. Capture the resulting BPMN trace block in the PR description as evidence.

**No edits** to `process-flow.yaml:761,769` — those literals stay as `suite: acceptance` and start working once Items 1–2 land.

## Non-goals

- **Operator-configurable suite groups in `gh-optivem.yaml`.** Suite ids are binary-defined; the group definition belongs in the binary. No schema slot.
- **Sentinel-bracket syntax (`<acceptance>`).** The doc-comment in `suite.go:64–66` mentions `<acceptance>` as one possible shape. This plan goes with the plain name `acceptance` because the existing BPMN literal already uses that form and there's no collision with a real suite id. The `<…>` sentinel can be revisited if a future collision arises.
- **BPMN fan-out (one node per suite).** Considered and rejected — would double the call-activity count without adding flexibility, and would move the suite-set decision into process-flow.yaml where it doesn't belong.
- **Contract / e2e group aliases.** Out of scope; today there's no caller that needs them. Add when a caller materialises.

## Acceptance criteria

- `gh optivem test run --suite=acceptance` runs `acceptance-api` and `acceptance-ui` and exits non-zero iff either fails.
- `gh optivem test run --suite=acceptance --test=<name>` runs the named test in both suites.
- `gh optivem test run --suite=acceptance,acceptance-isolated-api` works (alias + explicit, de-duped if overlapping).
- The BPMN trace from the failing session re-runs to completion: `VERIFY_TESTS_PASS_ACCEPTANCE` → `RUN_COMMAND` → `command-succeeded=true`.
- `--help` lists the `acceptance` alias.
- No edits to `internal/atdd/runtime/statemachine/process-flow.yaml`.
