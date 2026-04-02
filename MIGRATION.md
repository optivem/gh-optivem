# Migration: Language/Architecture-Agnostic Scaffolding

Migrate gh-optivem so that scaffolded repos contain no language or architecture in folder names, workflow filenames, or Docker image names.

See [MAPPING.md](MAPPING.md) for the complete source-to-destination mapping for all 4 combinations (monolith monorepo, monolith multirepo, multitier monorepo, multitier multirepo).

## Files to Change

1. **`internal/steps/apply_template.go`** — Destination paths, workflow copy with rename
   - Monolith: `system/monolith/{lang}/` -> `system/`
   - Multitier: `system/multitier/backend-{lang}/` -> `backend/`, `frontend-{lang}/` -> `frontend/`
   - System test: `system-test/{testLang}/` -> `system-test/`
   - Implement monolith multirepo (currently missing)
   - Remove cross-language fixup functions (image names no longer contain language)

2. **`internal/templates/templates.go`** — Workflow copy with rename
   - `CopyWorkflows` needs to support source -> destination name mapping (not just copy)
   - Monolith: `monolith-{lang}-commit-stage.yml` -> `commit-stage.yml`
   - Multitier: `multitier-backend-{lang}-commit-stage.yml` -> `backend-commit-stage.yml`
   - Pipeline stages: `{arch}-{testLang}-acceptance-stage.yml` -> `acceptance-stage.yml`, etc.
   - Update `FixupMultirepoImageURLs` for new image names
   - Update `FixupCommitStageForStandalone` for new paths

3. **`internal/steps/finalize.go`** — README, badges, SonarCloud, verification
   - README: include all user-supplied parameters (owner, system name, arch, languages)
   - Badge URLs: use new workflow filenames (no language/arch)
   - SonarCloud project keys: `{owner}_{repo}-system`, `{owner}_{repo}-backend`, `{owner}_{repo}-frontend`
   - Workflow verification: use new workflow filenames

4. **`internal/config/config.go`** — Config and naming
   - Add monolith multirepo repo naming (`{repo}-system`)
   - Remove language from any derived names used in image URLs

5. **`main.go`** — No structural changes expected

6. **`internal/steps/replace_references.go`** — Reference replacement
   - Update paths for new folder structure
   - Update image name patterns

## Workflow Content Changes

Inside copied workflows, the following replacements are needed:
- Working directory: `system/multitier/backend-{lang}` -> `backend`, `system/monolith/{lang}` -> `system`
- Docker image names: `monolith-system-{lang}` -> `system`, `multitier-backend-{lang}` -> `backend`, `multitier-frontend-{lang}` -> `frontend`
- Docker compose paths: `system-test/{testLang}/` -> `system-test/`

## Migration Order

1. Update `config.go` (naming, monolith multirepo support)
2. Update `templates.go` (workflow rename support)
3. Update `apply_template.go` (new destination paths)
4. Update `finalize.go` (README, badges, SonarCloud, verification)
5. Update `replace_references.go` (new paths)
6. Update system tests to match new expectations
