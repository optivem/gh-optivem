# Scope paths in `gh-optivem.yaml` + `implement-ticket` preflight

## Context

`gh optivem atdd implement-ticket` resolves three scope axes from `gh-optivem.yaml` (`scope.architecture`, `scope.system_lang`, `scope.test_lang`) and surfaces them to the agent as a textual line in the prompt:

```
# internal/atdd/runtime/agents/prompts/atdd-task.md (line 7)
Scope: Architecture=${architecture}, System Lang=${system_lang}, Test Lang=${test_lang}
```

The substitution is wired through `internal/atdd/runtime/clauderun/clauderun.go` (`renderPrompt`, lines 284–296). The prompt body then asks the agent to "Restrict ALL file edits, residual-reference greps, and per-language work to paths that match the in-scope architecture(s) and system language(s)" (line 28).

The 2026-05-05 atdd-rehearsal for `optivem/shop#61` ("Redesigning New Order UI", subtype `system-interface-redesign`) ran with `architecture=monolith system_lang=java test_lang=java`. Per the run log, the agent edited files across all four stacks anyway:

- `system/multitier/frontend-react/...` (out of scope)
- `system/monolith/typescript/...` (out of scope)
- `system/monolith/java/...` (in scope)
- `system/monolith/dotnet/...` (out of scope)
- `system-test/typescript/...` (out of scope)
- `system-test/java/...` (in scope)
- `system-test/dotnet/...` (out of scope)

The existing scope block is a soft constraint — the agent has to translate `monolith + java` into directory paths itself, and there is no preflight that fails if those directories are missing. The schema in `internal/projectconfig/config.go` carries no path information at all today.

## Motivation

Two distinct failure modes, one preflight:

1. **Cross-stack bleed.** The textual scope is too easy for the agent to ignore or misinterpret. Resolved, machine-readable paths injected into the prompt are harder to over-apply — and the prompt body can name them as the *only* allowed write roots.
2. **Silent path drift.** When the consumer repo's layout diverges from the gh-optivem scaffold convention (student moves `system/monolith/java/` to `app/`, splits into multiple folders, deletes a stack the scope still references, …), the agent today wanders into the wrong directory and the failure mode is "edits in the wrong place" rather than a clean error. A preflight that validates every path before any agent dispatch fails fast with a readable message.

Both problems are addressed by the same change: make paths first-class in `gh-optivem.yaml` (the *only* source of path truth — no runtime derivation), validate them on disk before dispatch, and inject them into the agent prompt as hard write roots.

The committed-file vs local-machine split must be respected: structural paths (relative to each repo root) belong in committed `gh-optivem.yaml`; the absolute location of each cloned repo on this machine must not.

## Decisions

1. **`paths:` block lives in committed `gh-optivem.yaml`, values are repo-relative.** A path like `paths.system_root: system/monolith/java` is meaningful on every machine. Absolute paths or workspace-relative paths in this file are rejected at validation time.
2. **Yaml is the sole source of path truth — no convention-based derivation at runtime.** The runtime never infers paths from `scope.architecture` + `scope.system_lang`. Required path fields must be present in `gh-optivem.yaml` or the config fails validation; the runtime reads them directly. The scaffold (`internal/steps/optivem_yaml.go`) knows the layout it creates and emits an explicit `paths:` block at scaffold time, so newly scaffolded repos still need zero hand-editing. Rationale: a single source of truth eliminates the "is this path coming from convention or yaml?" debugging question entirely, and means a student who renames a directory updates exactly one file (`gh-optivem.yaml`) — the runtime never silently picks a different path.
3. **Local clone resolution stays out of `gh-optivem.yaml`.** Three precedence rules, in order: (a) `--repo-dir` flag (per-repo, repeatable in multi-repo); (b) `GH_OPTIVEM_WORKSPACE` env var pointing at a directory whose immediate children are clones named after the repo slug's repo name; (c) sibling-dir convention — for mono-repo, CWD; for multi-repo, sibling directories of CWD whose names match the repo names in `project.repos`. No `gh-optivem.local.yaml` sidecar in this plan — adding a fourth mechanism is not justified by current need and can be added later as a fallback if real friction shows up.
4. **Preflight is a single function, called from one place.** `implement-ticket`'s entry point (the cobra `RunE` in `atdd_commands.go`) is the only caller. Preflight runs after config load and before any agent or board interaction. A failure prints one error block listing every missing repo and every missing path across all repos in scope, then exits non-zero. Do not interleave preflight failures with partial work.
5. **Paths injected into prompt as new placeholders, not by rewriting the Scope line.** Add `${system_root}`, `${frontend_root}`, and `${system_test_root}` to the substitution map in `clauderun.renderPrompt`. Update `atdd-task.md` to consume them as an explicit "Allowed write roots" block alongside the existing Scope line. Keeping the Scope line preserves the language/architecture context for the agent's own reasoning; the paths block is the hard constraint.
6. **Schema is three top-level tiers — `system`, `frontend`, `system_test` — each carrying `root` and an optional `repo` qualifier.** Mirrors the actual directory structure: `system/`, `system-test/`, and (in multitier) the frontend code are siblings, not nested. Tests do not belong under `system:` because the acceptance test code is its own body of code that drives the system, not part of it. There is one acceptance test root per scope (driven by `scope.test_lang`) — a multitier setup does not need separate backend-test and frontend-test roots because the same test suite drives both tiers. In mono-repo, `repo:` is omitted (implicit single repo). In multi-repo, `repo:` names which slug from `project.repos` the tier lives in. In monolith, the `frontend` block is omitted entirely. The flat 4-field design (`system_root`, `frontend_root`, …) was rejected because multi-repo with a flat schema cannot answer "which repo does each path live in?" without introducing implicit ordering rules that drift over time.

## Sample configs

The four combinations of `repo_strategy` × `architecture`. Each is a complete, copy-pasteable `gh-optivem.yaml` for the most common (java + java) language pairing; substitute `dotnet` or `typescript` as needed.

### mono-repo + monolith

Most common case. Single repo, single stack.

```yaml
project:
  url: https://github.com/orgs/optivem/projects/20
  repo_strategy: mono-repo
scope:
  architecture: monolith
  system_lang: java
  test_lang: java
paths:
  system:
    root: system/monolith/java
  system_test:
    root: system-test/java
```

### mono-repo + multitier

Single repo, two tiers (backend + frontend) living side-by-side, plus one acceptance test root that drives both.

```yaml
project:
  url: https://github.com/orgs/optivem/projects/20
  repo_strategy: mono-repo
scope:
  architecture: multitier
  system_lang: java
  test_lang: java
paths:
  system:
    root: system/multitier/backend-java
  frontend:
    root: system/multitier/frontend-react
  system_test:
    root: system-test/java
```

### multi-repo + monolith

Single stack, but the system code lives in its own repo (e.g. operator wants the system repo separate from the workspace repo where pipelines run). System tests live alongside the system code in the same repo.

```yaml
project:
  url: https://github.com/orgs/optivem/projects/20
  repo_strategy: multi-repo
  repos:
    - optivem/shop
scope:
  architecture: monolith
  system_lang: java
  test_lang: java
paths:
  system:
    repo: optivem/shop
    root: .
  system_test:
    repo: optivem/shop
    root: system-test
```

### multi-repo + multitier

Backend and frontend in separate repos. Each tier names its repo explicitly. System tests are placed in whichever repo the operator prefers — the example below puts them in the backend repo, but `repo: optivem/shop-frontend` would be equally valid.

```yaml
project:
  url: https://github.com/orgs/optivem/projects/20
  repo_strategy: multi-repo
  repos:
    - optivem/shop-backend
    - optivem/shop-frontend
scope:
  architecture: multitier
  system_lang: java
  test_lang: java
paths:
  system:
    repo: optivem/shop-backend
    root: .
  frontend:
    repo: optivem/shop-frontend
    root: .
  system_test:
    repo: optivem/shop-backend
    root: system-test
```

## Items

### 1. Extend the schema with a per-tier `paths:` block

**Files (edit):**

- `internal/projectconfig/config.go`
- `internal/projectconfig/config_test.go`

Add the new types on `Config`:

```go
type Config struct {
    Project Project `yaml:"project"`
    Scope   Scope   `yaml:"scope"`
    Paths   Paths   `yaml:"paths"`
}

// Paths holds the per-tier directory roots the ATDD pipeline writes into.
// Tiers are top-level siblings — `system_test` is NOT nested under `system`
// because the acceptance test code is its own body of code that drives
// the system, not part of it.
//
// `system` and `system_test` are required when scope.architecture is set.
// `frontend` is required when scope.architecture=multitier and forbidden
// when scope.architecture=monolith. There is no `frontend_test` tier —
// one acceptance test suite drives both tiers in multitier.
type Paths struct {
    System     PathSpec `yaml:"system,omitempty"`
    Frontend   PathSpec `yaml:"frontend,omitempty"`
    SystemTest PathSpec `yaml:"system_test,omitempty"`
}

// PathSpec describes one tier's location. `repo` is required when
// project.repo_strategy=multi-repo and forbidden when mono-repo. `root`
// is required when the enclosing tier is set, and is repo-relative
// (relative to the tier's repo root). Empty PathSpec means "tier omitted"
// — valid only for `frontend` under monolith.
type PathSpec struct {
    Repo string `yaml:"repo,omitempty"`
    Root string `yaml:"root,omitempty"`
}
```

Validation rules:

1. `Root`, when non-empty, must be relative (`!filepath.IsAbs`), must not start with `..`, and must not contain a `..` segment. (No `os.Stat` here — `Validate` stays string-shape only; FS checks live in the preflight, item 4.)
2. `scope.architecture=monolith` ⇒ `paths.system` non-empty AND `paths.system_test` non-empty; `paths.frontend` must be empty (operator typo guard).
3. `scope.architecture=multitier` ⇒ all three of `paths.system`, `paths.frontend`, `paths.system_test` non-empty.
4. `scope.architecture` empty ⇒ `paths` may be empty (config is partially populated, e.g. fresh).
5. Within a non-empty `PathSpec`, `Root` must be non-empty (a tier is "all or nothing" except for `Repo`).
6. `project.repo_strategy=mono-repo` ⇒ every non-empty `PathSpec.Repo` must be empty.
7. `project.repo_strategy=multi-repo` ⇒ every non-empty `PathSpec.Repo` must be non-empty AND must appear in `project.repos`.
8. `project.repo_strategy` empty ⇒ skip rules 6–7 (mirrors how the existing Validate skips repo-strategy-dependent rules when the strategy is empty).

Test cases in `config_test.go`:

- Round-trip: `Write` → `Load` preserves the `Paths` block for each of the four sample configs in this plan.
- Validate accepts each of the four samples.
- Validate rejects absolute paths (`/abs/path`, `C:\\...` on Windows) on `root`.
- Validate rejects `..` prefix and embedded `..` on `root`.
- Validate rejects `paths.system` empty when `scope.architecture=monolith`.
- Validate rejects `paths.system_test` empty when `scope.architecture=monolith`.
- Validate rejects `paths.frontend` non-empty when `scope.architecture=monolith`.
- Validate rejects `paths.frontend` empty when `scope.architecture=multitier`.
- Validate rejects a tier with `repo` set but `root` empty.
- Validate rejects `paths.system.repo` non-empty under `repo_strategy=mono-repo`.
- Validate rejects `paths.system.repo` empty under `repo_strategy=multi-repo`.
- Validate rejects `paths.system.repo` set to a slug that is not in `project.repos`.

### 2. Resolve local clone paths

**Files (new):**

- `internal/atdd/runtime/repolocator/repolocator.go`
- `internal/atdd/runtime/repolocator/repolocator_test.go`

A small package the implement-ticket entry point calls to turn `(project.repos, project.repo_strategy)` into a `map[slug]localPath`. Three resolution strategies, tried in order, first match wins per-repo (decision 3):

1. `--repo-dir slug=path` flag on `implement-ticket`. Repeatable. Empty when absent. The flag is added in item 4.
2. `$GH_OPTIVEM_WORKSPACE` env var. If set, the local path for repo `<owner>/<name>` is `$GH_OPTIVEM_WORKSPACE/<name>`. Skipped if the env var is unset.
3. Sibling-dir convention. For `mono-repo`, the sole repo's local path is the CWD (already where `implement-ticket` runs from). For `multi-repo`, each repo's local path is `<dirname(cwd)>/<repo-name>`.

The function returns `map[string]string` plus a slice of unresolved slugs. Unresolved slugs are not an error inside the locator — the preflight (item 4) decides how to surface them.

Tests:

- mono-repo with one repo, no flags, no env var → CWD wins.
- multi-repo, sibling convention → both repos resolve to siblings of CWD.
- multi-repo, env var set → both repos resolve under `$GH_OPTIVEM_WORKSPACE`.
- multi-repo, one `--repo-dir` flag, env var also set → flag wins for that repo, env wins for the other.
- multi-repo, sibling does not exist → still returned by the locator (existence check belongs to preflight); the test asserts the returned path matches the convention.

### 3. Inject resolved paths into the agent prompt

**Files (edit):**

- `internal/atdd/runtime/clauderun/clauderun.go`
- `internal/atdd/runtime/clauderun/clauderun_test.go`
- `internal/atdd/runtime/agents/prompts/atdd-task.md`
- `internal/atdd/runtime/driver/driver.go` (the seed-scope-params caller — extend to seed the resolved paths too)

In `clauderun.Options`, add three flat fields. The driver flattens the per-tier struct on the way in so the prompt-substitution layer stays simple:

```go
SystemRoot     string
FrontendRoot   string
SystemTestRoot string
```

In `renderPrompt`, extend the params map:

```go
"system_root":      opts.SystemRoot,
"frontend_root":    opts.FrontendRoot,
"system_test_root": opts.SystemTestRoot,
```

Empty values render as the empty string — no `scopeOrDefault`-style fallback, because "no path" is meaningful (monolith repos have no frontend root).

In `atdd-task.md`, add a new block right after the Scope line:

```
Allowed write roots:
- System: ${system_root}
- Frontend (multitier only): ${frontend_root}
- System tests: ${system_test_root}
```

And rewrite the existing Scope guidance section (lines 26–28) to make the paths the load-bearing constraint and the scope axes context:

> Edit ONLY files under the "Allowed write roots" listed above. Treat any other path as out-of-scope and do not modify it, even if a sibling implementation appears related to the ticket. The `Scope:` line is contextual: it tells you which language and architecture the allowed roots correspond to, so you choose appropriate file types (e.g. `.java` under a Java root, `.tsx` under a React root). Do not infer additional roots from the Scope line.

In `driver.go`, the function that seeds scope params from `projectconfig.Config` (search the file for the existing Architecture / SystemLang / TestLang assignment) gains a sibling block that flattens `cfg.Paths` onto the dispatch options:

```go
opts.SystemRoot     = cfg.Paths.System.Root
opts.FrontendRoot   = cfg.Paths.Frontend.Root
opts.SystemTestRoot = cfg.Paths.SystemTest.Root
```

`cfg.Paths.{System,Frontend,SystemTest}.Repo` is *not* flattened into the prompt — it is consumed by the preflight (item 4) for repo-existence checks and by the local-path resolver to know which clone each path lives in.

Tests in `clauderun_test.go`: `renderPrompt` produces the expected substituted text for monolith (frontend root empty) and multitier (all three populated). No "merge / derive" tests since neither exists.

### 4. Preflight in `implement-ticket`

**Files (edit/new):**

- `atdd_commands.go` (the `newAtddImplementTicketCmd` cobra builder)
- `internal/atdd/runtime/preflight/preflight.go` (new)
- `internal/atdd/runtime/preflight/preflight_test.go` (new)

Add a `--repo-dir` repeatable flag on `implement-ticket` with form `slug=path` (e.g. `--repo-dir optivem/shop=/abs/path/to/shop`). Empty by default — most operators rely on the convention.

Add a `Preflight(cfg *projectconfig.Config, repoDirs map[string]string) error` function. It:

1. Calls `repolocator.Resolve(cfg, repoDirs)` to get the per-slug local path map plus unresolved slugs.
2. For each slug listed in `cfg.Project.Repos` (or the implicit-self repo when `mono-repo` and `Repos` is empty):
   - Confirms the resolved path exists and is a directory.
   - Confirms it is a git repo (presence of a `.git` directory or file — `os.Stat`, no `git` invocation).
3. For each non-empty tier (`system`, `frontend`, `system_test`):
   - Determines the host repo: in mono-repo, the single repo; in multi-repo, the slug named in `cfg.Paths.<tier>.Repo` (validated by item 1 to be present in `project.repos`).
   - Confirms `<host repo's local path>/<tier.Root>` exists and is a directory.
4. Aggregates every failure into one error, formatted as a multi-line block listing each missing item (slug and/or path), and returns it. Does not return on the first failure — the operator should see the whole list.

Wire `Preflight` into `newAtddImplementTicketCmd`'s `RunE` immediately after `projectconfig.Load` and before any board, classify, or dispatch call. On error, print to stderr and exit non-zero — no partial work.

Tests in `preflight_test.go` use `t.TempDir` to fabricate a fake workspace, one per sample config in this plan:

- mono-repo + monolith, all paths exist → nil.
- mono-repo + monolith, `system.root` directory does not exist → error mentions `paths.system.root` and the absolute path checked.
- mono-repo + monolith, `system_test.root` does not exist → error mentions `paths.system_test.root`.
- mono-repo + multitier, `frontend.root` does not exist → error mentions `paths.frontend.root`.
- multi-repo + monolith, the single repo not cloned locally → error mentions slug + expected local path.
- multi-repo + multitier, both repos cloned but the frontend repo is not a git repo → error mentions "is not a git repository".
- multi-repo + multitier, paths exist under each correct repo → nil.
- multi-repo + multitier, `system.root` accidentally exists under the FRONTEND repo but not under the system repo → error (the host-repo mapping from item 4 step 3 must enforce per-tier scoping, not "exists somewhere").

### 5. Scaffold writes the `paths:` block (now mandatory)

**Files (edit):**

- `internal/steps/optivem_yaml.go`
- `internal/steps/optivem_yaml_test.go` (or wherever the existing translation tests live)

Decision 2 makes `paths:` mandatory at runtime, so the scaffold MUST emit it. `buildOptivemYAML` gains a `paths:` block populated from the scaffold's own knowledge of the directories it creates. The mapping is the scaffold's internal table, not a runtime convention — if the scaffold layout ever changes, this table changes alongside it without affecting the runtime contract. The `system_test.root` value is keyed on `scope.test_lang` (not `system_lang`) since the test suite directory is named after the test language:

| Architecture | system_lang  | system.root                       | frontend.root                     |
| ------------ | ------------ | --------------------------------- | --------------------------------- |
| `monolith`   | `java`       | `system/monolith/java`            | (omitted)                         |
| `monolith`   | `dotnet`     | `system/monolith/dotnet`          | (omitted)                         |
| `monolith`   | `typescript` | `system/monolith/typescript`      | (omitted)                         |
| `multitier`  | `java`       | `system/multitier/backend-java`   | `system/multitier/frontend-react` |
| `multitier`  | `dotnet`     | `system/multitier/backend-dotnet` | `system/multitier/frontend-react` |
| `multitier`  | `typescript` | `system/multitier/backend-node`   | `system/multitier/frontend-react` |

| test_lang    | system_test.root         |
| ------------ | ------------------------ |
| `java`       | `system-test/java`       |
| `dotnet`     | `system-test/dotnet`     |
| `typescript` | `system-test/typescript` |

For `multi-repo`, the scaffold also fills `paths.system.repo`, `paths.frontend.repo` (multitier), and `paths.system_test.repo` from `cfg.SystemFullRepo` / `cfg.BackendFullRepo` / `cfg.FrontendFullRepo`. By default `paths.system_test.repo` mirrors `paths.system.repo` (system tests live alongside system code) — adjust if the scaffolded multi-repo layout puts tests in a different repo. For `multi-repo` + `monolith`, the in-repo paths use whatever the scaffold actually creates inside the system repo (the sample config uses `.` for system and `system-test` for system_test; mirror the real scaffolded layout).

Cross-check both tables against `internal/steps/apply_template.go` and the scaffolded `shop` template before committing — the tables are the **claim** to verify, not the source of truth. If a cell is wrong relative to what gh-optivem actually creates, fix the table, not the scaffold.

Tests:

- For each (architecture, system_lang, test_lang, repo_strategy) combo the scaffold supports, `buildOptivemYAML` produces a config whose `Paths` block round-trips through `Validate` cleanly and matches the tables above.
- A scaffolded `multi-repo + multitier` config has all three of `paths.system.repo`, `paths.frontend.repo`, `paths.system_test.repo` populated and pointing at slugs in `project.repos`.
- A scaffolded `mono-repo + monolith` config has `paths.system.repo` and `paths.system_test.repo` empty (decision 6 / item 1 rule 6).

### 6. Documentation

**Files (edit):**

- `README.md` — single paragraph under the existing config section explaining the `paths:` block, that it is mandatory when scope is set, and pointing at the four sample configs in this plan as the canonical reference.
- `docs/atdd/process/system-interface-redesign.md` (in the `shop` repo, per the plan at `plans/20260504-180000-shop-system-interface-redesign-doc.md`) — add a one-line note that the agent now receives explicit write roots and won't bleed into out-of-scope stacks. Only if the doc has already landed in `shop` by the time this plan is executed; otherwise out of scope here and pick up in the existing shop-side phase doc plan.

No CLAUDE.md changes — the rule "don't bleed into out-of-scope stacks" is now enforced by the prompt + preflight, not by guideline.

## Verification

After all items land, re-run the rehearsal that exposed the original bug:

```bash
./scripts/atdd-rehearsal.sh --issue 61
```

Expected:

- The atdd-task agent's prompt contains `Allowed write roots: System: system/monolith/java` and `System tests: system-test/java`, with an empty `Frontend` root.
- The agent edits files only under `system/monolith/java/` and `system-test/java/`.
- `system/multitier/...`, `system/monolith/typescript/...`, `system/monolith/dotnet/...`, `system-test/typescript/...`, `system-test/dotnet/...` are untouched.
- A separately-rigged rehearsal where `system/monolith/java/` is renamed to `app/` and `paths.system.root` is updated to `app` succeeds; one where the rename is done WITHOUT updating `paths:` fails preflight with a readable error before any agent runs.
- Loading any of the four sample configs from this plan succeeds; deleting the `paths:` block from one of them and reloading fails validation with an error naming `paths.system` (or `paths.system_test`).

## Migration of existing repos

This is a **breaking change** to `gh-optivem.yaml`: any existing file with `scope.architecture` set but no `paths:` block will fail validation after item 1 lands. Two viable migration paths, both out of band of this plan but worth noting so reviewers expect the breakage:

1. **Re-scaffold.** Re-run `gh optivem init` against the existing repo (overwrites `gh-optivem.yaml` with the new shape, including `paths:`). Acceptable when the scaffolded layout still matches the on-disk layout — the common case.
2. **Hand-edit.** Copy the matching sample config from this plan into `gh-optivem.yaml`, adjust the `scope` axes to the repo's actual values, and adjust the path values if the layout has diverged from the scaffold. Acceptable for repos that have moved files around.

A `gh optivem config migrate` command is *not* in scope here — the population of repos using `gh-optivem.yaml` is small enough today (the `shop` template plus rehearsal scratch repos) that one of the two manual paths above is fine.

## Out of scope

- **`gh-optivem.local.yaml` sidecar for local clone paths.** Decision 3 is "three mechanisms is enough for now"; add a sidecar later if `--repo-dir` + env var + sibling convention turn out not to cover real workflows.
- **Validating that the resolved `paths.system.root` actually contains source files matching `system_lang`** (and equivalently for `system_test.root` / `test_lang`). Preflight stops at "directory exists and is git." A monolith repo with `paths.system.root` pointing at an empty directory is valid as far as preflight is concerned. The agent will fail informatively when it can't find files to edit, and adding source-presence checks makes preflight slow and brittle.
- **Extending `gh optivem config init`** to prompt for `paths:`. The scaffold writes the block automatically (item 5); operators only hand-edit when overriding the scaffolded layout, and that is rare enough not to warrant interactive prompting.
- **A `gh optivem config migrate` command.** See "Migration of existing repos" above — manual migration is acceptable at the current scale.
