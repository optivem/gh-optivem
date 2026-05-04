# Plan — Run multiple test names per `gh optivem test system` invocation

**Date:** 2026-05-04
**Status:** proposed
**Owner:** unassigned
**Trigger:** Today `--test <name>` narrows execution to a single test. Users running a handful of named tests (re-running failures, ATDD flows that exercise 2–3 acceptance tests in sequence, curated subsets) have no first-class way to express "these N tests, in one invocation." The recent `--tests` → `--test-config` rename freed up `--tests` as the natural plural for this feature.

## Goal

Let one `gh optivem test system` invocation run a caller-specified list of test names — without forcing per-test reinvocations of the runner (which would re-pay setup / install / start cost per test).

## Problem

`runOneSuite` (internal/runner/tests.go:173) substitutes a single `<test>` placeholder into the suite's `testFilter` template, then appends the result to `suite.Command`. Today the substitution value comes from `opts.Test` (single string) or `suite.SampleTest`.

Users can already hand-roll multi-value behaviour for two of three runners by abusing the substitution:
- **dotnet** (`&DisplayName~<test>` fragment) — `|` is OR inside `--filter`, so `--test "T1|T2"` works.
- **playwright/jest** (`--grep '<test>'`) — value is parsed as a regex, so `T1|T2` is alternation.
- **gradle** (`--tests <test>`) — values are Ant-style globs, no OR. Multi-value requires the *whole flag* repeated (`--tests T1 --tests T2`). Pipes don't work.

So today's UX is "fine for some, broken for one, and it relies on users knowing the runner-specific operator." A first-class flag should make multi-value uniform across all three runners.

## Proposed changes

### Phase A — repeatable `--test` + multi-value template (~30–50 LOC)

- [ ] **Make `--test` repeatable** in `newTestSystemCmd` (runner_commands.go:257). Switch from `StringVar` to `StringSliceVar` — cobra accepts both repeated (`--test T1 --test T2`) and comma-separated (`--test T1,T2`) for free.
- [ ] **(Decision needed) Add `--tests` as a plural alias.** Pure ergonomics — `--tests T1,T2` reads more naturally for the multi case. Trivial: a second `StringSliceVar` bound to the same target.
- [ ] **Extend `TestOptions.Test`** from `string` to `[]string` (internal/runner/tests.go:22). Update `pickFilterValue` to return `[]string` (or `nil` for "no filter").
- [ ] **Teach `runOneSuite` how to fan out one filter value per name.** Today it does one substitution + one `appendTestFilter`. The new behaviour depends on per-runner join semantics — see next.
- [ ] **Add `testFilterJoin` to tests.json** (`internal/runner/config.go:49`). Two values:
  - `"or"` (default) — substitute once, joining test names with `|`. Covers dotnet `&DisplayName~T1|T2` and playwright `--grep 'T1|T2'`.
  - `"repeat"` — substitute the whole `testFilter` fragment once per name and concatenate. Covers gradle `--tests T1 --tests T2`.
- [ ] **Update sampleTest semantics** — when `--sample` is set and `Test` is empty, fall back to a single-element slice `[suite.SampleTest]` (no behaviour change for callers).
- [ ] **Tests for the matrix:** dotnet/playwright with 2 names + `or`, gradle with 2 names + `repeat`, single-name (regression: should match today's output exactly), empty-list (no `--filter`/`--grep`/`--tests` at all — runs the whole suite).
- [ ] **Docs** — update README (line 130) and runner_commands.go example block to show multi-test usage; mention Windows ~32K command-line cap (≈600 ~50-char names) as the practical limit.

### Phase B — `--test-file <path>` (defer until needed)

- [ ] Accept a path; treat each non-blank, non-`#` line as a test name; merge with values from `--test`.
- [ ] Useful for re-running failures (`grep FAIL last-run.log | … > failed.txt`). Skip until a real workflow needs it — Phase A handles 90% of the use case for handfuls of tests.

## Out of scope

- **Per-test isolation / orchestration** — the runner still hands one filter expression to the underlying test runner. Users wanting "run T1, then T2, with system restarted between" should keep using a shell loop with `--no-build --no-start --no-setup` (already supported).
- **Pattern globbing inside gh-optivem** (e.g. `T*` matching N tests). The native runners already do this; gh-optivem stays a thin pass-through.
- **shop tests.json updates** — shop's existing `testFilter` strings keep working unchanged (default `"or"` = today's substitute-once behaviour for dotnet/playwright; gradle suites need a one-line `"testFilterJoin": "repeat"` add to their `tests-*.json` to gain multi-value support).

## Risks

- **Windows command-line length** — `CreateProcess` caps the full command line at 32,767 chars (we go through `exec.Command` directly, no shell wrap). At ~50 chars per test name plus 1 separator, the practical ceiling is ~600 names. Document this; not a code concern.
- **Template-syntax surface area growth.** `testFilterJoin` is a new tests.json field that must be documented and defaulted. Alternative considered: a second placeholder convention (`<tests-or>` / `<tests-repeat>`) — more declarative but more surface area in docs and more parsing in the runner. Going with the explicit field.
- **Backward compat for sampleTest** — `--sample` today injects one value. After Phase A it still does, just via a one-element slice. No external behaviour change.

## Verification

- [ ] **Unit tests** for `appendTestFilter` and the new fan-out path: `or` join (one substitution), `repeat` join (N substitutions), empty list (passthrough).
- [ ] **Manual run against shop**, mirroring `scripts/manual-test-runner-shop.sh`:
  - dotnet suite: `gh optivem test system --suite acceptance-api --tests T1,T2`
  - playwright suite: `gh optivem test system --suite acceptance-ui --tests "shouldCreateOrder,shouldCancelOrder"`
  - (when shop adds Java legacy multi-value) gradle suite: same shape, with `"testFilterJoin": "repeat"` set in the corresponding `tests-legacy.json`.
- [ ] **Regression check**: existing `--test SingleName` invocations across shop's workflows still produce identical commands (no diff in `--filter` / `--grep` / `--tests` fragments).

## Pointers

- Single-test substitution: [internal/runner/tests.go:173-216](../internal/runner/tests.go) (`runOneSuite`, `pickFilterValue`, `appendTestFilter`).
- Cobra wiring: [runner_commands.go:216-265](../runner_commands.go) (`newTestSystemCmd`).
- TestsConfig schema: [internal/runner/config.go:40-50](../internal/runner/config.go) (`TestFilter` field — where `testFilterJoin` would slot in).
- Earlier discussion that prompted the `--tests` → `--test-config` rename: [conversation thread that led to commit 6bb55b3](https://github.com/optivem/gh-optivem/commit/6bb55b3) — the rename was the precondition for Phase A.
