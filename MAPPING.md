# Migration: Language/Architecture-Agnostic Scaffolded Repos

The scaffolded repo output should contain **no language or architecture** in folder names, workflow filenames, or Docker image names. The user's choices (language, architecture, system name, etc.) appear **only in the README**.

## Shop Source Structure (unchanged)

The shop repo keeps all variants — language appears in folder/workflow names because it holds every combination:

```
shop/
  system/
    monolith/java/            # monolith backends
    monolith/dotnet/
    monolith/typescript/
    multitier/backend-java/   # multitier backends
    multitier/backend-dotnet/
    multitier/backend-typescript/
    multitier/frontend-react/ # multitier frontends
    external-real-sim/        # shared across all
    external-stub/            # shared across all
  system-test/
    java/                     # test suites per language
    dotnet/
    typescript/
  .github/workflows/
    monolith-{lang}-commit-stage.yml
    monolith-{testLang}-acceptance-stage.yml
    monolith-{testLang}-qa-stage.yml
    monolith-{testLang}-qa-signoff.yml
    monolith-{testLang}-prod-stage.yml
    monolith-{lang}-verify.yml
    backend-{backendLang}-commit-stage.yml
    multitier-frontend-{frontendLang}-commit-stage.yml
    multitier-{testLang}-acceptance-stage.yml
    multitier-{testLang}-qa-stage.yml
    multitier-{testLang}-qa-signoff.yml
    multitier-{testLang}-prod-stage.yml
    multitier-{testLang}-verify.yml
```

## Repo Naming Rules

The user supplies `--owner` and `--repo`. Component repo names are derived automatically:

| Combination | Repo | Name |
|---|---|---|
| **Monolith monorepo** | root repo | `{owner}/{repo}` |
| **Monolith multirepo** | root repo | `{owner}/{repo}` |
| | monolith repo | `{owner}/{repo}-system` |
| **Multitier monorepo** | root repo | `{owner}/{repo}` |
| **Multitier multirepo** | root repo | `{owner}/{repo}` |
| | backend repo | `{owner}/{repo}-backend` |
| | frontend repo | `{owner}/{repo}-frontend` |

The root repo is always `{repo}`. In multirepo setups, component repos append a suffix: `-system` (monolith), `-backend`, `-frontend` (multitier).

## Scaffolded Output (target — no language, no architecture)

### 1. Monolith Monorepo

Single repo with system code and tests together.

```
{repo}/
  system/                    # from: system/monolith/{lang}/
  external-real-sim/         # from: system/external-real-sim/
  external-stub/             # from: system/external-stub/
  system-test/               # from: system-test/{testLang}/
  .github/workflows/
    commit-stage.yml         # from: monolith-{lang}-commit-stage.yml
    acceptance-stage.yml     # from: monolith-{testLang}-acceptance-stage.yml
    qa-stage.yml             # from: monolith-{testLang}-qa-stage.yml
    qa-signoff.yml           # from: monolith-{testLang}-qa-signoff.yml
    prod-stage.yml           # from: monolith-{testLang}-prod-stage.yml
  VERSION
  README.md
```

### 2. Monolith Multirepo

Two repos: root repo (tests + pipeline stages) and monolith repo (system code + commit stage).

**Root repo** (`{repo}/`):
```
{repo}/
  external-real-sim/         # from: system/external-real-sim/
  external-stub/             # from: system/external-stub/
  system-test/               # from: system-test/{testLang}/
  .github/workflows/
    acceptance-stage.yml     # from: monolith-{testLang}-acceptance-stage.yml
    qa-stage.yml             # from: monolith-{testLang}-qa-stage.yml
    qa-signoff.yml           # from: monolith-{testLang}-qa-signoff.yml
    prod-stage.yml           # from: monolith-{testLang}-prod-stage.yml
  VERSION
  README.md
```

**System repo** (`{system-repo}/`):
```
{system-repo}/
  system/                    # from: system/monolith/{lang}/
  .github/workflows/
    commit-stage.yml         # from: monolith-{lang}-commit-stage.yml
  README.md
```

### 3. Multitier Monorepo

Single repo with backend, frontend, and tests together.

```
{repo}/
  backend/                     # from: system/multitier/backend-{backendLang}/
  frontend/                    # from: system/multitier/frontend-{frontendLang}/
  external-real-sim/           # from: system/external-real-sim/
  external-stub/               # from: system/external-stub/
  system-test/                 # from: system-test/{testLang}/
  .github/workflows/
    backend-commit-stage.yml   # from: backend-{backendLang}-commit-stage.yml
    frontend-commit-stage.yml  # from: multitier-frontend-{frontendLang}-commit-stage.yml
    acceptance-stage.yml       # from: multitier-{testLang}-acceptance-stage.yml
    qa-stage.yml               # from: multitier-{testLang}-qa-stage.yml
    qa-signoff.yml             # from: multitier-{testLang}-qa-signoff.yml
    prod-stage.yml             # from: multitier-{testLang}-prod-stage.yml
  VERSION
  README.md
```

### 4. Multitier Multirepo

Three repos: root repo, backend repo, frontend repo.

**Root repo** (`{repo}/`):
```
{repo}/
  external-real-sim/         # from: system/external-real-sim/
  external-stub/             # from: system/external-stub/
  system-test/               # from: system-test/{testLang}/
  .github/workflows/
    acceptance-stage.yml     # from: multitier-{testLang}-acceptance-stage.yml
    qa-stage.yml             # from: multitier-{testLang}-qa-stage.yml
    qa-signoff.yml           # from: multitier-{testLang}-qa-signoff.yml
    prod-stage.yml           # from: multitier-{testLang}-prod-stage.yml
  VERSION
  README.md
```

**Backend repo** (`{backend-repo}/`):
```
{backend-repo}/
  backend/                   # from: system/multitier/backend-{backendLang}/
  .github/workflows/
    backend-commit-stage.yml # from: backend-{backendLang}-commit-stage.yml
  README.md
```

**Frontend repo** (`{frontend-repo}/`):
```
{frontend-repo}/
  frontend/                  # from: system/multitier/frontend-{frontendLang}/
  .github/workflows/
    frontend-commit-stage.yml # from: multitier-frontend-{frontendLang}-commit-stage.yml
  README.md
```

## Docker Image Names

No language or architecture in image names:

| Current (in shop) | Target (in scaffolded repo) |
|---|---|
| `sysapp-{lang}` | `system` |
| `backend-{lang}` | `backend` |
| `multitier-frontend-{lang}` | `frontend` |

GHCR URLs become: `ghcr.io/{owner}/{repo}/system`, `ghcr.io/{owner}/{repo}/backend`, `ghcr.io/{owner}/{repo}/frontend`.

## Workflow Content Changes

Inside copied workflows, replace:
- Workflow names: `monolith-{lang}-commit-stage` -> `commit-stage`, `multitier-{testLang}-acceptance-stage` -> `acceptance-stage`, etc.
- Working directory paths: `system/multitier/backend-{lang}` -> `backend`, `system/monolith/{lang}` -> `system`, etc.
- Docker image names: `sysapp-{lang}` -> `system`, `backend-{lang}` -> `backend`, `multitier-frontend-{lang}` -> `frontend`
- System test paths: `system-test/{testLang}/` -> `system-test/`
- SonarCloud key suffixes: `-monolith-{lang}` -> `-system`, `-backend-{lang}` -> `-backend`, `-multitier-frontend-{lang}` -> `-frontend`
- Path filters and self-references: `.github/workflows/{old-name}.yml` -> `.github/workflows/{new-name}.yml`
- Concurrency groups: `{old-name}-${{ github.ref }}` -> `{new-name}-${{ github.ref }}`

## README

The README should display all user-supplied parameters:
- Owner
- System name
- Architecture (monolith / multitier)
- Repo strategy (monorepo / multirepo)
- Language(s) (backend lang, frontend lang, test lang)
- Pipeline badges

## SonarCloud Project Keys

**Monolith monorepo:**

| Current | Target |
|---|---|
| `{owner}_{repo}-monolith-{lang}` | `{owner}_{repo}-system` |

**Monolith multirepo:**

| Current | Target |
|---|---|
| (not implemented yet) | `{owner}_{monolith-repo}-system` |

**Multitier monorepo:**

| Current | Target |
|---|---|
| `{owner}_{repo}-backend-{lang}` | `{owner}_{repo}-backend` |
| `{owner}_{repo}-multitier-frontend-{lang}` | `{owner}_{repo}-frontend` |

**Multitier multirepo:**

| Current | Target |
|---|---|
| `{owner}_{backend-repo}-backend-{lang}` | `{owner}_{backend-repo}-backend` |
| `{owner}_{frontend-repo}-multitier-frontend-{lang}` | `{owner}_{frontend-repo}-frontend` |

Note: The root repo does not get a SonarCloud project — only code components (system, backend, frontend) are analyzed.

## Documentation

The scaffolder copies docs from the shop into the root repo:

Source: `shop/docs/{arch}/` + `shop/docs/shared/` → `{repo}/docs/`

Scaffolded docs structure (all combinations):

```
{repo}/
  docs/
    use-cases.md              # from: docs/shared/use-cases.md
    use-case-narrative.md     # from: docs/shared/use-case-narrative.md
    project-registration.md   # from: docs/shared/project-registration.md
    architecture.md           # from: docs/{arch}/architecture.md
```

