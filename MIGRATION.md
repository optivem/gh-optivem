# Migration: Language/Architecture-Agnostic Scaffolding

Migrate gh-optivem so that scaffolded repos contain no language or architecture in folder names, workflow filenames, or Docker image names.

See [MAPPING.md](MAPPING.md) for the complete source-to-destination mapping for all 4 combinations (monolith monorepo, monolith multirepo, multitier monorepo, multitier multirepo).

## Files to Change

1. **`internal/config/config.go`** ✅
   - Added monolith multirepo repo naming (`{repo}-system`) — `SystemRepo`, `SystemFullRepo`, `SystemRepoDir` fields
   - GHCR_TOKEN now required for all multirepo setups (not just multitier)

2. **`internal/templates/templates.go`** ✅
   - `CopyWorkflows` now accepts `map[string]string` (source -> destination name mapping)
   - Added `FixupWorkflowContent` and `FixupDockerComposeContent` for batch replacements
   - `FixupMultirepoImageURLs` updated for language-agnostic image names (`backend`, `frontend`)
   - Added `FixupMonolithMultirepoImageURLs` and `FixupMonolithMultirepoDockerCompose`
   - `FixupMultirepoDockerCompose` updated for language-agnostic image names
   - `FixupCommitStageForStandalone` simplified — takes `componentDir` instead of `component` + `lang`

3. **`internal/steps/apply_template.go`** ✅
   - Complete rewrite with 4 clear functions: `applyMonolithMonorepo`, `applyMonolithMultirepo`, `applyMultitierMonorepo`, `applyMultitierMultirepo`
   - Monolith: `system/monolith/{lang}/` -> `system/`
   - Multitier: `system/multitier/backend-{lang}/` -> `backend/`, `frontend-{lang}/` -> `frontend/`
   - System test: `system-test/{testLang}/` -> `system-test/`
   - External dirs: `system/external-*` -> top-level `external-*`
   - Monolith multirepo fully implemented (root repo + system repo)
   - Cross-language fixup simplified to port mapping only (image names handled by content replacements)
   - Workflow content replacements handle paths, image names, and SonarCloud key suffixes

4. **`internal/steps/finalize.go`** ✅
   - SonarCloud keys: `{owner}_{repo}-system`, `{owner}_{repo}-backend`, `{owner}_{repo}-frontend`
   - Badge URLs use new workflow filenames (no language/arch)
   - Verification uses new workflow filenames
   - README includes all user-supplied parameters (owner, system name, arch, languages)
   - Monolith multirepo README and cleanup fully implemented
   - CommitAndPush handles monolith multirepo system repo

5. **`internal/steps/replacements.go`** ✅
   - Monolith multirepo: replaces references in system repo, fixes docker-compose URLs
   - Namespace replacement routes monolith code to correct repo (root vs system)
   - TypeScript package.json names updated for new folder structure

6. **`internal/steps/github_setup.go`** ✅
   - Monolith multirepo: creates, clones, and sets secrets on system repo (`{repo}-system`)

7. **`main.go`** ✅
   - Banner and summary output show system repo for monolith multirepo

8. **Tests** ✅
   - Added `TestMonolithMultirepoRepoNames` and `TestSonarProjectKeys`
   - All existing tests pass
   - System tests already cover monolith multirepo configurations

## Migration Order (all completed)

1. ✅ Update `config.go` (naming, monolith multirepo support)
2. ✅ Update `templates.go` (workflow rename support)
3. ✅ Update `apply_template.go` (new destination paths)
4. ✅ Update `finalize.go` (README, badges, SonarCloud, verification)
5. ✅ Update `replacements.go` (new paths)
6. ✅ Update `github_setup.go` (monolith multirepo)
7. ✅ Update `main.go` (banner)
8. ✅ Update tests to match new expectations
