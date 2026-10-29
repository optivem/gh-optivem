# `--no-legacy` should also omit legacy artifacts from the scaffolded repo

## Why
Today `--no-legacy` (`internal/config/config.go:616`) only skips the legacy *runs* (local tests, acceptance-stage verification — `main.go:547`, `steps/verify.go:507`). The scaffolded repo still contains the legacy artifacts. A user who opts out of legacy wants none of it in the output.

## End result
With `--no-legacy`, the scaffolded repo contains none of:
1. `gh-optivem.legacy.yaml` (`steps/optivem_yaml.go:47-86`)
2. legacy tests: `system-test/tests.legacy.yaml` and the legacy test sources/suites copied from the template
3. legacy workflows: `acceptance-stage-legacy.yml` (`steps/apply_template.go:195-197`, called at 256, 324, 439, 530) and its README badge (`steps/readme.go:63`)

Without the flag, output is byte-identical to today.

## Decisions (defaults — proceed unless the user objects)
- **Extend `--no-legacy`**, do not add a new flag. Recommended: one flag, one meaning, no half-states. Update the flag help text and `docs/cli-reference.md:147` to "exclude legacy from runs and from the scaffolded output".
- No new fail-loud paths needed; omission is deterministic from `cfg.NoLegacy`.

## Steps (execute in order; use `Edit`, not `Write`; no subagents — each is a small localized change)
1. **Read only** `steps/apply_template.go` (lines ~190-260, 695-720), `steps/optivem_yaml.go`, `steps/readme.go` (~55-70), and grep `legacy` under the template-copy code to find how legacy test files get copied. Do not read other files.
2. `addLegacyWorkflow`: early-return when `cfg.NoLegacy`. Add the `cfg`/flag parameter (it already receives `cfg.Deploy`; pass `cfg` fields minimally).
3. `optivem_yaml.go`: skip building/writing `gh-optivem.legacy.yaml` when `NoLegacy`; fix the log lines ("Writing gh-optivem.yaml + ..." / "Wrote ...").
4. Legacy tests: skip copying `tests.legacy.yaml` and any legacy-suite sources in the template copy step; make the rename rules at `apply_template.go:701-708` tolerate absence.
5. `readme.go`: omit the `acceptance-stage-legacy` badge when `NoLegacy`.
6. Check other references to the legacy config/workflow names (commit-stage workflow comments at `apply_template.go:768, 911` only mention it — verify no workflow *input* points at the legacy file when it is absent).
7. Tests: add table cases to `internal/scaffolding/steps/replacements_test.go` (or the nearest existing test for these functions) asserting absence with `NoLegacy=true` and presence with `false`. Run `go test ./internal/scaffolding/... ./internal/config/...` only.
8. Docs: update `docs/cli-reference.md:147`, `docs/how-it-works.md` rows 64/67/83, and the `--no-legacy` help string. Do not run the readme-updater agent (a surgical `Edit` is ~20x cheaper).
9. `gofmt` only the files edited. `go vet` on touched packages.

## Token / autonomy rules
- Direct `Grep`/`Read` with line ranges; never read whole files over ~300 lines.
- No agents, no CI runs, no manual-test scaffolding run (slow, costly).
- No questions mid-run: the decisions above are final. If a template layout contradicts an assumption (e.g. legacy tests are interleaved with latest tests and cannot be skipped cleanly), stop and report that one fact rather than guessing.
- When done: report which files changed, test results, then use `/commit` (never raw git). Delete this plan file as part of the same commit.
