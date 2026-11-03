# 2026-08-20 13:45 CEST — `component-test`: inert `--sample`/`--test`, and bare `run` ignoring `suiteGroups`

## TL;DR

**Why:** At the component tier, `--sample` and `--test` are silently no-ops — the suite runs in
full and still reports `PASSED`, so a "sample" run costs full-suite time while claiming to be a
sample. Separately, a bare `component-test run` executes *every declared suite*, so opt-in suites
a config deliberately excluded from `suiteGroups.all` (slow, image-building, Docker-heavy) run
anyway unless the caller remembers `--suite all`.

**End result:** `--sample`/`--test` narrow a component suite to the named test the same way they do
at the system-test tier, and a bare `component-test run` and `--suite all` agree on what "the
default gate" means — with whichever semantics is chosen stated explicitly in `--help`.

## Problem 1 — `testFilter` is parsed but never applied

`internal/build/componenttest/config.go:35` parses `testFilter` (and `testFilterJoin` at :36) off
`component-tests.yaml`. But `internal/build/componenttest/run.go:121` calls:

```go
cmd := runner.ApplyTestFilter(s.Command, "", "", pickFilterValue(s, opts))
```

— passing `""` for `testFilter`. `applyTestFilter` (`internal/build/runner/tests.go:531`) returns
the command unchanged when `testFilter == ""`. So `pickFilterValue` correctly resolves the suite's
`sampleTest` or the `--test` values, and then the result is discarded.

The comment at run.go:119-121 claims "the per-suite filter here is supplied via the shared runner
helper", which is what the empty argument was presumably meant to mean — but the config's own
`TestFilter`/`TestFilterJoin` fields are the values that helper needs, and nothing reads them.

**Observed** (shop, `backend-clean-java`, `external-contract` suite):

```
gh optivem component-test run -c gh-optivem-multitier-clean-java.yaml --suite external-contract --sample
→ PASSED; build/test-results/contractTest/ holds all 6 stub classes / 8 tests
```

Identical to the unfiltered run. Both `backend-java` and `backend-clean-java` carry `sampleTest`
values on every suite and a `testFilter: --tests '*.<test>'` line, so both are affected.

**Fix:** pass `cfg.TestFilter` / `cfg.TestFilterJoin` through to `ApplyTestFilter`. `runSuite`
currently receives only `(Component, Suite, Options)`, so the config has to be threaded in (or the
two fields copied onto `Suite` at load time). Then fail loud rather than silently when a filter was
requested but the config declares no `testFilter` — a requested-but-undeliverable narrowing is
exactly the indeterminate case that must not report `PASSED`.

**Note on Gradle semantics:** several suites already carry a `--tests` pattern in their command
(e.g. `contractTest --tests '*StubParityContractTest'`). Gradle ORs multiple `--tests` flags, so
appending `--tests '*.<name>'` *widens* rather than narrows. Whatever the fix, it must be verified
against a suite whose command already has a `--tests` flag, not only against a bare one like
`unit`.

## Open questions

- **Q-4 (Problem 2) — DECIDED: option (a).** A bare `run` resolves `suiteGroups.all` when the config declares it, falling back to every declared suite when it does not.

## Verification

- A component suite with `--sample` executes one test (assert on the JUnit XML count, not on
  `PASSED`) — including for a suite whose command already carries a `--tests` flag.
- `--test <name>` narrows the same way; `--test` naming nothing that exists fails loud.
- Existing `componenttest` unit tests still pass.

## Related

- shop `system/multitier/backend-clean-java/component-tests.yaml` — added 2026-08-20; both defects
  were found while verifying it, and its `sampleTest` values are correct-but-inert until Problem 1
  is fixed.
- shop `system/multitier/backend-java/component-tests.yaml` — same shape, same exposure.
