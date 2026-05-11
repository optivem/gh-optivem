# Shop: rename `frontend-react` → `frontend-typescript` (and related workflow names)

> ⚠️ **Deferred.** gh-optivem already accepts `--frontend-lang typescript` (the
> user-facing flag value). The shop template still uses the legacy `react`
> token internally for the framework directory + workflow filename. This
> plan finishes the rename by removing the gh-optivem ↔ shop mismatch.

## Background

In the gh-optivem-only pass (2026-05-11):
- CLI flag `--frontend-lang` now accepts `typescript` (was `react`).
- `gh-optivem.yaml`'s `system.frontend.lang` is `typescript`.
- `cfg.FrontendLang` carries `typescript` end-to-end.
- The shop-side framework dir (`shop/system/multitier/frontend-react/`) and
  workflow filenames (`multitier-frontend-react-*.yml`) were left untouched.

To keep the scaffolder working in the interim, three sites in gh-optivem
hardcode the literal `"react"` for shop-path interpolation:

- `internal/steps/names.go:VarsForCfg` — `"frontendLang": "react"` for
  template `${frontendLang}` placeholders (feeds shop paths/filenames).
- `internal/steps/apply_template.go:applyMultitierMonorepo` and
  `applyMultitierMultirepo` — local `frontendLang := "react"`.
- `internal/steps/apply_template.go:forbiddenTemplateRefs` — passes
  `"react"` to `multitierForbiddenRefs` so leftover shop tokens get caught.

These hardcodes are the bookmark for this plan: once the shop rename lands,
they collapse back into `cfg.FrontendLang`.

## Scope (shop repo: `../shop`)

### Directory rename

- `shop/system/multitier/frontend-react/` → `shop/system/multitier/frontend-typescript/`

### Workflow filename renames

- `shop/.github/workflows/multitier-frontend-react-bump-patch-version.yml` →
  `multitier-frontend-typescript-bump-patch-version.yml`
- `shop/.github/workflows/multitier-frontend-react-commit-stage.yml` →
  `multitier-frontend-typescript-commit-stage.yml`

### Content rewrites across shop

`grep -rln "frontend-react\|multitier-frontend-react"` lists ~64 files.
Replace every `frontend-react` substring with `frontend-typescript`. Files
of note:

- All `multitier-*-{prod,qa,acceptance,bump-patch}-stage*.yml` reference
  the frontend workflow / image / VERSION path by name.
- `_prerelease-pipeline.yml`, `_meta-prerelease-pipeline.yml`,
  `prerelease-pipeline-multitier-*.yml` — pipeline composition refs.
- `bump-patch-version.yml`, `cleanup.yml`, `meta-bump-all.yml`,
  `cross-lang-system-verification.yml` — orchestration refs.
- `docker/{java,dotnet,typescript}/multitier/docker-compose.*.yml` —
  build contexts / image names referencing `frontend-react`.
- `gh-optivem-multitier-*.yaml` (shop's own scaffold configs) — frontend
  paths used by the shop's CI flows.
- `ACTIONS_PLAN.md`, `CLAUDE.md`, `README.md`,
  `docs/atdd/architecture/{diagram-architecture.md,system.md}` — docs.
- `plans/*.md` — historical plan docs; rename-in-place is fine since the
  plans describe the layout, not the migration.
- `.claude/agents/workflow-comparator.md` — agent prompt mentions the dir.

## Scope (gh-optivem repo)

### Collapse the three hardcoded `"react"` sites back into `cfg.FrontendLang`

- `internal/steps/names.go:VarsForCfg` — `"frontendLang": cfg.FrontendLang`.
  Drop the explanatory comment block that justifies the hardcode.
- `internal/steps/apply_template.go:applyMultitierMonorepo` — restore
  `frontendLang := cfg.FrontendLang`; drop the explanatory comment.
- `internal/steps/apply_template.go:applyMultitierMultirepo` — same.
- `internal/steps/apply_template.go:forbiddenTemplateRefs` — restore
  `multitierForbiddenRefs(cfg.BackendLang, cfg.FrontendLang, cfg.TestLang)`;
  drop the explanatory comment.

### Tests that pass `"react"` directly to internal-template helpers

These bypass `cfg` and call the helpers with literal `"react"` to assert
the shop-path interpolation. After the rename they should pass
`"typescript"`:

- `internal/steps/replacements_test.go` — every
  `multitierContentReplacements(..., "react", ...)`, every
  `multitierDockerComposeReplacements(..., "react", ...)`, every
  `multitierSonarKeyReplacements(..., "react")`, every literal
  `"multitier-frontend-react"` / `"system/multitier/frontend-react"` /
  `"frontend-react"` in `in`/`want` fixtures.
- `internal/steps/names_test.go` — `"frontendLang": "react"` map values,
  `"multitier-frontend-react-commit-stage.yml"` expected strings.
- `internal/atdd/runtime/preflight/preflight_test.go` — `Path:
  "system/multitier/frontend-react"` (this is testing a real shop YAML
  layout, so renames with shop).
- `internal/projectconfig/config_test.go:58` — `path:
  system/multitier/frontend-react` in a YAML fixture.

### Docs

- `MAPPING.md` — line 18 (`multitier/frontend-react/`) and any other
  references to `multitier-frontend-{frontendLang}` examples.
- `internal/atdd/runtime/agents/prompts/atdd-task.md` — uses
  `frontend-react` in narrative about the multitier layout.

## Coordination

Both repos must land together. Suggested sequence:

1. Branch shop: rename dir, rename workflows, content rewrite, push.
2. Branch gh-optivem: collapse the three hardcoded sites, update tests
   and docs, push.
3. Run `bash scripts/manual-test.sh --owner ... --arch multitier
   --backend-lang dotnet --frontend-lang typescript ...` against both
   branches before merge — that exercises the full
   shop→gh-optivem→GitHub roundtrip.
4. Merge shop first, then gh-optivem (gh-optivem at `main` only works if
   the shop default branch already has `frontend-typescript/`; the
   `--shop-ref` flag can pin a feature branch during testing).

## Verification

- `go build ./...` and `go test ./...` in gh-optivem.
- `manual-test.sh` end-to-end against `--frontend-lang typescript` for
  both monorepo and multirepo, dotnet/java/typescript backend variants.
- `actionlint` over the renamed shop workflows.
- `grep -r "frontend-react\|multitier-frontend-react"` returns no hits
  in either repo (except possibly historical plan docs, which can be
  excluded or rewritten).

## Why deferred

The gh-optivem-only fix unblocks `manual-test.sh` immediately. The shop
rename is mechanical but touches 60+ files across a separate repo whose
CI flows depend on every workflow filename — best done as a focused PR
with a clean baseline.
