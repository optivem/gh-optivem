# `--no-legacy` should also omit legacy artifacts from the scaffolded repo

## Why
`--no-legacy` used to skip only the legacy *runs*; the scaffolded repo still carried the legacy artifacts. All implementation steps have shipped: with `--no-legacy` the scaffold omits `gh-optivem.legacy.yaml`, `system-test/tests.legacy.yaml` + the legacy test source tree, the `acceptance-stage-legacy.yml` workflow and its README badge. Without the flag, output is unchanged.

## Decisions (resolved)
- Legacy test sources are removed post-copy via a per-testLang path table (`legacyTestPaths` in `internal/scaffolding/steps/apply_template.go`). Shop keeps them fully self-contained (`java/.../systemtest/legacy`, `dotnet/SystemTests/Legacy`, `typescript/tests/legacy`); latest tests never import from them, so no interleaving problem.
- The `optivemConfigRewrites` legacy rule is a plain string replacement and is a no-op when no legacy workflow is copied; non-legacy scaffolded workflows reference no legacy file. No change needed.
- Out of scope (owned elsewhere): `.github/actions/acceptance-test/action.yml` input description "Exclude legacy stage verification" still describes only the run-skip behaviour.

## Verification (operator)
- Scaffold one docker monolith and one docker multitier repo with `--no-legacy`; confirm no `gh-optivem.legacy.yaml`, no `system-test/tests.legacy.yaml`, no legacy test dir, no `.github/workflows/acceptance-stage-legacy.yml`, no legacy README badge, and that the latest system tests still build/run.
- Scaffold the same flavors without the flag; confirm output matches a pre-change scaffold.
