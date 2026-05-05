# Per-component schema in `gh-optivem.yaml` + `implement-ticket` preflight

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

Two distinct gaps surfaced:

1. The textual scope is too easy for the agent to ignore — paths aren't named explicitly, the agent has to translate `monolith + java` into directories itself, and there's no preflight that fails if those directories are missing.
2. The current schema can't express what real systems look like. A multitier project with a Java backend and a TypeScript frontend uses *two* languages, not one — but `scope.system_lang` is a single value.

## Motivation

This plan addresses both gaps with a schema rework:

1. **Cross-stack bleed.** Resolved, machine-readable paths injected into the prompt — and the prompt body names them as the *only* allowed write roots. Validated on disk by a preflight before any agent dispatch.
2. **Per-component language.** The system schema becomes architecture-aware: a monolith has one body of code with one language; a multitier has a backend and a frontend, each with its own language. Tests are their own tier with their own language.
3. **Silent path drift.** When the consumer repo's layout diverges from the gh-optivem scaffold convention (student moves `system/monolith/java/` to `app/`, splits into multiple folders, deletes a stack), the agent today wanders into the wrong directory. A preflight that validates every path before any agent dispatch fails fast with a readable message.

The committed-file vs local-machine split is preserved: structural paths (relative to each repo root) belong in committed `gh-optivem.yaml`; the absolute location of each cloned repo on this machine does not.

## Decisions

1. **Schema is reshaped around top-level keys: `project:` + `repo_strategy:` + `system:` + `system_test:` + optional `external_systems:`.** The previous `scope:` and `paths:` blocks fold away. `project:` carries the GitHub Projects board URL. `repo_strategy:` (`mono-repo` or `multi-repo`) is a top-level scalar — it's not a property of the project board, and it affects every tier's `repo:` field across all the other top-level blocks, so it sits at the top alongside them rather than nested under one of them. `system:` describes the system being built — its architecture, and either flat per-monolith fields or nested backend/frontend components. `system_test:` is the test suite that drives the system, its own top-level block (not nested under `system:`). Tests aren't part of the system; they drive it. `external_systems:` is **optional** and declares vendored stand-ins for third-party dependencies (`simulators` for the ATDD cycle-3 real-sim pattern; `stubs` for the cycle-2 WireMock-style pattern). When present, the agent gets a separate "external-system roots" prompt block clarifying that these are write-eligible only when the ticket calls for stub/sim changes. When absent, the project has no declared externals and the prompt block is silently omitted.

2. **`system:` is polymorphic by architecture, by design.** Under `monolith`, `system` carries `path`/`repo`/`lang` directly — the system *is* the code. Under `multitier`, `system` carries nested `backend:` and `frontend:` blocks — the system *has* components. Forcing uniformity (e.g. always nesting under a single `monolith:` component name even in monolith) was rejected because `system.monolith.path` reads as "the monolith component of the system," which is conceptually wrong: the system *is* the monolith. The polymorphism costs one architecture switch in `Validate`; consumers (preflight, prompt rendering, scaffold) already branch on architecture so this is zero extra cost.

3. **Architecture is explicit, not derived.** `system.architecture` is an enum field, not inferred from the presence of `backend:`/`frontend:`. Reasons: a single source of truth simplifies reading (operators don't scan children to learn the arch); typos are caught (e.g. `backendd:` would otherwise validate as monolith silently); a single field is cheaper to read than a structural inspection.

4. **Per-component `lang:` replaces `scope.system_lang` and `scope.test_lang`.** Each `path/repo/lang` triple is co-located. A multitier config can have `backend.lang=java` and `frontend.lang=typescript` — the previous schema literally couldn't express this. `system_test` carries its own `lang`, since the test language is independent of the system language (e.g. Java tests driving a TypeScript system). All three of `path`, `repo`, `lang` are mandatory whenever a tier is present — no defaults, no derivation.

5. **`project.repos` list is dropped — repos are derived from tier `repo:` fields.** The set of participating repos is the union of every tier's `repo:` value. A separate `repos:` list would be a second source of truth and a drift risk. `repolocator` (item 2) iterates over the derived set.

6. **Local clone resolution stays out of `gh-optivem.yaml`.** Three precedence rules, in order: (a) `--repo-dir` flag (per-repo, repeatable); (b) `GH_OPTIVEM_WORKSPACE` env var pointing at a directory whose immediate children are clones named after the repo slug's repo name; (c) sibling-dir convention — for mono-repo, CWD; for multi-repo, sibling directories of CWD whose names match the repo names. No `gh-optivem.local.yaml` sidecar in this plan — adding a fourth mechanism is not justified by current need and can be added later as a fallback if real friction shows up.

7. **Preflight is a single function, called from one place.** `implement-ticket`'s entry point (the cobra `RunE` in `atdd_commands.go`) is the only caller. Preflight runs after config load and before any agent or board interaction. A failure prints one error block listing every missing repo and every missing path across all tiers in scope, then exits non-zero. Do not interleave preflight failures with partial work.

8. **Paths injected into prompt as a single rendered block, not as raw placeholders.** The driver computes a multi-line "Allowed write roots" string from `cfg` — handling architecture-specific shape — and substitutes it into the template via a single `${allowed_roots}` placeholder. The template stays a flat substitution; the runtime owns the rendering logic. Per-component lang is rendered inline (e.g., `Backend: <path> (lang: java)`) so the agent can pick file types correctly without re-reading a separate Scope line.

## Sample configs

The four combinations of `repo_strategy` × `architecture`. Each is a complete, copy-pasteable `gh-optivem.yaml`. Substitute `dotnet`/`typescript` for `java` as needed.

### mono-repo + monolith

Most common case. Single repo, single body of code, single language.

```yaml
project:
  url: https://github.com/orgs/optivem/projects/20

repo_strategy: mono-repo

system:
  architecture: monolith
  path: system/monolith/java
  repo: optivem/shop
  lang: java

system_test:
  path: system-test/java
  repo: optivem/shop
  lang: java

external_systems:
  stubs:
    path: external-stub
    repo: optivem/shop
  simulators:
    path: external-real-sim
    repo: optivem/shop
```

### mono-repo + multitier

Single repo, two components living side-by-side. Backend and frontend can be in different languages — captured per component.

```yaml
project:
  url: https://github.com/orgs/optivem/projects/20

repo_strategy: mono-repo

system:
  architecture: multitier
  backend:
    path: system/multitier/backend-java
    repo: optivem/shop
    lang: java
  frontend:
    path: system/multitier/frontend-react
    repo: optivem/shop
    lang: typescript

system_test:
  path: system-test/java
  repo: optivem/shop
  lang: java

external_systems:
  stubs:
    path: external-stub
    repo: optivem/shop
  simulators:
    path: external-real-sim
    repo: optivem/shop
```

### multi-repo + monolith

Single body of code, but it lives in its own repo separate from where pipelines run. System tests live alongside the system code.

```yaml
project:
  url: https://github.com/orgs/optivem/projects/20

repo_strategy: multi-repo

system:
  architecture: monolith
  path: .
  repo: optivem/shop
  lang: java

system_test:
  path: system-test
  repo: optivem/shop
  lang: java

external_systems:
  stubs:
    path: external-stub
    repo: optivem/shop
  simulators:
    path: external-real-sim
    repo: optivem/shop
```

### multi-repo + multitier

Backend and frontend in separate repos. System tests in whichever repo the operator prefers — this example uses a third "main" repo, but `optivem/shop-backend` would be equally valid.

```yaml
project:
  url: https://github.com/orgs/optivem/projects/20

repo_strategy: multi-repo

system:
  architecture: multitier
  backend:
    path: .
    repo: optivem/shop-backend
    lang: java
  frontend:
    path: .
    repo: optivem/shop-frontend
    lang: typescript

system_test:
  path: system-test
  repo: optivem/shop-main
  lang: java

external_systems:
  stubs:
    path: external-stub
    repo: optivem/shop-main
  simulators:
    path: external-real-sim
    repo: optivem/shop-main
```

## Items

### 1. Reshape the schema

**Files (edit):**

- `internal/projectconfig/config.go`
- `internal/projectconfig/config_test.go`

The new `Config` shape:

```go
type Config struct {
    Project         Project         `yaml:"project"`
    RepoStrategy    string          `yaml:"repo_strategy,omitempty"`
    System          System          `yaml:"system"`
    SystemTest      TierSpec        `yaml:"system_test"`
    ExternalSystems ExternalSystems `yaml:"external_systems,omitempty"`
}

type Project struct {
    URL string `yaml:"url,omitempty"`
}

// ExternalSystems declares vendored stand-ins for third-party dependencies
// the system talks to during ATDD cycles. Both sub-fields are optional and
// independent — a project might use only stubs, only simulators, both, or
// neither. When a sub-field is non-empty, all of its inner fields (`Path`,
// `Repo`) must be set.
//
// Field order matches the ATDD cycle progression: `Stubs` (cycle 2,
// WireMock-style no-logic stand-in driven by JSON mappings) comes before
// `Simulators` (cycle 3, e.g. a node + json-server simulator with
// controlled state).
type ExternalSystems struct {
    Stubs      ExternalSpec `yaml:"stubs,omitempty"`
    Simulators ExternalSpec `yaml:"simulators,omitempty"`
}

// ExternalSpec describes one external-system tier. Two fields, both
// mandatory when the tier is set. No `lang:` — externals are config and
// scaffolding (WireMock JSON, ad-hoc node simulators), not source code in
// the language enum sense; the agent picks file types from the directory
// contents the same way it does anywhere else.
type ExternalSpec struct {
    Path string `yaml:"path,omitempty"`
    Repo string `yaml:"repo,omitempty"`
}

// System describes the system being built. Polymorphic by architecture:
//   - monolith:  Path/Repo/Lang are populated; Backend/Frontend are empty.
//   - multitier: Backend/Frontend are populated; Path/Repo/Lang are empty.
//
// Validate enforces exclusivity. Operators reading the file should see
// exactly one shape per architecture, never both.
type System struct {
    Architecture string `yaml:"architecture,omitempty"`

    // Monolith-only.
    Path string `yaml:"path,omitempty"`
    Repo string `yaml:"repo,omitempty"`
    Lang string `yaml:"lang,omitempty"`

    // Multitier-only.
    Backend  TierSpec `yaml:"backend,omitempty"`
    Frontend TierSpec `yaml:"frontend,omitempty"`
}

// TierSpec describes one body of code: where it lives, in which repo,
// and what language it is written in. Used for backend, frontend, and
// system_test. All three fields are mandatory whenever the tier is set.
type TierSpec struct {
    Path string `yaml:"path,omitempty"`
    Repo string `yaml:"repo,omitempty"`
    Lang string `yaml:"lang,omitempty"`
}
```

Plus a helper:

```go
// Repos returns the union of every tier's Repo field, sorted. Used by
// repolocator and validation to know which repos participate, without
// requiring an explicit project.repos list.
func (c *Config) Repos() []string { ... }
```

Validation rules:

1. `RepoStrategy`, when non-empty, must be `mono-repo` or `multi-repo`.
2. `System.Architecture`, when non-empty, must be `monolith` or `multitier`.
3. `TierSpec.Lang`, when non-empty, must be `java`, `dotnet`, or `typescript`. (`react` is a framework, not a language — rejected.)
4. `TierSpec.Path`, when non-empty, must be relative (`!filepath.IsAbs`), must not start with `..`, and must not contain a `..` segment. (No `os.Stat` here — `Validate` is string-shape only; FS checks live in preflight, item 4.)
5. **Architecture exclusivity.**
   - `System.Architecture=monolith` ⇒ `System.{Path,Repo,Lang}` all non-empty; `System.Backend` and `System.Frontend` must both be zero-value `TierSpec{}`.
   - `System.Architecture=multitier` ⇒ `System.Backend` and `System.Frontend` both non-empty (each with `Path`, `Repo`, `Lang` all set); `System.{Path,Repo,Lang}` all empty.
   - `System.Architecture` empty ⇒ all of `System.{Path,Repo,Lang,Backend,Frontend}` empty (config is partially populated, e.g. fresh).
6. **Tier completeness.** Within any non-empty `TierSpec` (`SystemTest`, `Backend`, or `Frontend`), all three of `Path`, `Repo`, `Lang` must be non-empty — never two of three.
7. **`SystemTest` presence.** When `System.Architecture` is set, `SystemTest.{Path,Repo,Lang}` must all be set.
8. **Repo-strategy consistency** (only when `RepoStrategy` is non-empty):
   - `mono-repo` ⇒ `c.Repos()` has at most one element. When `System.Architecture` is set, exactly one (every tier's `repo:` must agree, including external-system repos).
   - `multi-repo` ⇒ `c.Repos()` has at least one element when `System.Architecture` is set.
9. **External systems (optional).** When `ExternalSystems.Simulators` is non-empty (any inner field set), both `Path` and `Repo` must be non-empty. Same for `ExternalSystems.Stubs`. The two sub-blocks are independent — declaring one without the other is valid. `c.Repos()` includes external-system `Repo` values; under `multi-repo`, those repos must point at slugs that also appear elsewhere in `c.Repos()` only if you want them on the existing repo set — there's no rule that externals share a repo with system or system_test.

Test cases in `config_test.go`:

- Round-trip: `Write` → `Load` preserves each of the four sample configs.
- Validate accepts each of the four samples.
- Validate rejects absolute paths (`/abs/path`, `C:\\...` on Windows).
- Validate rejects `..` prefix and embedded `..` on `path`.
- Validate rejects monolith config with `backend:` or `frontend:` present.
- Validate rejects multitier config with `system.path`/`system.repo`/`system.lang` present.
- Validate rejects multitier config with `backend:` set but `frontend:` empty (and vice versa).
- Validate rejects a tier with `path` set but `repo` or `lang` empty (and the other two-of-three permutations).
- Validate rejects `lang: react` (not in language enum).
- Validate rejects `system_test:` empty when `system.architecture` is set.
- Validate rejects `mono-repo` config where two tiers point at different `repo:` values (including across `external_systems`).
- Validate rejects `multi-repo` config where every tier's `repo:` is empty (`Repos()` returns empty).
- Validate accepts a config with `external_systems:` block omitted entirely.
- Validate accepts `external_systems:` with only `stubs:` populated and `simulators:` omitted (and vice versa).
- Validate rejects `external_systems.simulators.path` non-empty when `external_systems.simulators.repo` is empty (and the `path`-set-but-`repo`-empty mirror).
- Validate accepts `external_systems.simulators.repo` set to a slug that does not appear under `system:` or `system_test:` (externals can live in their own repo).

### 2. Resolve local clone paths

**Files (new):**

- `internal/atdd/runtime/repolocator/repolocator.go`
- `internal/atdd/runtime/repolocator/repolocator_test.go`

A small package the implement-ticket entry point calls to turn `cfg.Repos()` into a `map[slug]localPath`. Three resolution strategies, tried in order, first match wins per-repo (decision 6):

1. `--repo-dir slug=path` flag on `implement-ticket`. Repeatable. Empty when absent. The flag is added in item 4.
2. `$GH_OPTIVEM_WORKSPACE` env var. If set, the local path for repo `<owner>/<name>` is `$GH_OPTIVEM_WORKSPACE/<name>`. Skipped if the env var is unset.
3. Sibling-dir convention. For `mono-repo`, the sole repo's local path is the CWD (already where `implement-ticket` runs from). For `multi-repo`, each repo's local path is `<dirname(cwd)>/<repo-name>`.

The function returns `map[string]string` plus a slice of unresolved slugs. Unresolved slugs are not an error inside the locator — the preflight (item 4) decides how to surface them.

Tests:

- mono-repo with one repo (derived from the one tier set), no flags, no env var → CWD wins.
- multi-repo, sibling convention → all derived repos resolve to siblings of CWD.
- multi-repo, env var set → all derived repos resolve under `$GH_OPTIVEM_WORKSPACE`.
- multi-repo, one `--repo-dir` flag, env var also set → flag wins for that repo, env wins for the others.
- multi-repo, sibling does not exist → still returned by the locator (existence check belongs to preflight); the test asserts the returned path matches the convention.

### 3. Inject resolved paths into the agent prompt

**Files (edit):**

- `internal/atdd/runtime/clauderun/clauderun.go`
- `internal/atdd/runtime/clauderun/clauderun_test.go`
- `internal/atdd/runtime/agents/prompts/atdd-task.md`
- `internal/atdd/runtime/driver/driver.go` (the seed-scope-params caller)

In `clauderun.Options`, replace the existing `Architecture`/`SystemLang`/`TestLang` fields with:

```go
Architecture  string  // "monolith" or "multitier"
AllowedRoots  string  // pre-rendered multi-line block (see below)
```

In `renderPrompt`, replace `${architecture}`/`${system_lang}`/`${test_lang}` substitutions with:

```go
"architecture":  opts.Architecture,
"allowed_roots": opts.AllowedRoots,
```

The previous `${system_lang}` and `${test_lang}` placeholders go away — language is now per-component and surfaces inline in `${allowed_roots}`.

In `driver.go`, the function that seeds dispatch params from `projectconfig.Config` gains a renderer:

```go
func renderAllowedRoots(cfg *projectconfig.Config) string {
    var b strings.Builder

    // System + tests block.
    switch cfg.System.Architecture {
    case "monolith":
        fmt.Fprintf(&b, "- System: %s (lang: %s)\n",
            cfg.System.Path, cfg.System.Lang)
    case "multitier":
        fmt.Fprintf(&b, "- Backend: %s (lang: %s)\n",
            cfg.System.Backend.Path, cfg.System.Backend.Lang)
        fmt.Fprintf(&b, "- Frontend: %s (lang: %s)\n",
            cfg.System.Frontend.Path, cfg.System.Frontend.Lang)
    }
    fmt.Fprintf(&b, "- System tests: %s (lang: %s)\n",
        cfg.SystemTest.Path, cfg.SystemTest.Lang)

    // External-systems block — only if any external is declared.
    // Stubs first (cycle 2), then simulators (cycle 3).
    ext := cfg.ExternalSystems
    if ext.Stubs != (ExternalSpec{}) || ext.Simulators != (ExternalSpec{}) {
        b.WriteString("\nExternal-system roots (modify only when the ticket calls for stub/sim changes):\n")
        if ext.Stubs != (ExternalSpec{}) {
            fmt.Fprintf(&b, "- Stubs: %s\n", ext.Stubs.Path)
        }
        if ext.Simulators != (ExternalSpec{}) {
            fmt.Fprintf(&b, "- Simulators: %s\n", ext.Simulators.Path)
        }
    }

    return b.String()
}
```

Driver wires it: `opts.Architecture = cfg.System.Architecture; opts.AllowedRoots = renderAllowedRoots(cfg)`.

In `atdd-task.md`, replace the existing Scope line and the "Restrict ALL file edits…" paragraph with:

```
Architecture: ${architecture}

Allowed write roots:
${allowed_roots}
Edit ONLY files under the listed write roots. Treat any other path as out-of-scope and do not modify it, even if a sibling implementation appears related to the ticket. The lang annotation on each system root tells you which file types belong there (e.g. `.java` under a Java root, `.tsx` under a TypeScript+React frontend). External-system roots, when listed, are write-eligible only when the ticket explicitly calls for stub or simulator changes — otherwise treat them as read-only context.
```

Tests in `clauderun_test.go`: render the substituted prompt for the four sample configs; assert the expected `Allowed write roots:` block (monolith has one System row; multitier has Backend + Frontend rows; both have a System tests row; all four sample configs include the External-system roots block listing stubs + simulators). Plus a fifth case: a config with `external_systems:` omitted should render no External-system roots block at all.

### 4. Preflight in `implement-ticket`

**Files (edit/new):**

- `atdd_commands.go` (the `newAtddImplementTicketCmd` cobra builder)
- `internal/atdd/runtime/preflight/preflight.go` (new)
- `internal/atdd/runtime/preflight/preflight_test.go` (new)

Add a `--repo-dir` repeatable flag on `implement-ticket` with form `slug=path` (e.g. `--repo-dir optivem/shop=/abs/path/to/shop`). Empty by default — most operators rely on the convention.

`Preflight(cfg *projectconfig.Config, repoDirs map[string]string) error`:

1. Calls `repolocator.Resolve(cfg, repoDirs)` — returns per-slug local path map plus unresolved slugs.
2. For each slug in `cfg.Repos()`:
   - Confirms the resolved path exists and is a directory.
   - Confirms it is a git repo (presence of a `.git` directory or file — `os.Stat`, no `git` invocation).
3. For each present tier (`system` if monolith; `backend` + `frontend` if multitier; `system_test` always; plus `external_systems.stubs` and `external_systems.simulators` when declared):
   - Determines the host repo from the tier's `repo:` field.
   - Confirms `<host repo's local path>/<tier.path>` exists and is a directory.
4. Aggregates every failure into one error, formatted as a multi-line block listing each missing item (slug and/or path), and returns it. Does not return on first failure — the operator should see the whole list.

Wire `Preflight` into `newAtddImplementTicketCmd`'s `RunE` immediately after `projectconfig.Load` and before any board, classify, or dispatch call. On error, print to stderr and exit non-zero — no partial work.

Tests in `preflight_test.go` use `t.TempDir` to fabricate fake workspaces, one per sample config:

- mono-repo + monolith, all paths exist → nil.
- mono-repo + monolith, `system.path` directory does not exist → error mentions `system.path` and the absolute path checked.
- mono-repo + monolith, `system_test.path` does not exist → error mentions `system_test.path`.
- mono-repo + multitier, `system.frontend.path` does not exist → error mentions `system.frontend.path`.
- multi-repo + monolith, the single repo not cloned locally → error mentions slug + expected local path.
- multi-repo + multitier, both repos cloned but the frontend repo is not a git repo → error mentions "is not a git repository".
- multi-repo + multitier, paths exist under each correct repo → nil.
- multi-repo + multitier, `system.backend.path` accidentally exists under the FRONTEND repo but not under the backend repo → error (the host-repo mapping must enforce per-tier scoping, not "exists somewhere").
- mono-repo + monolith with `external_systems` declared, all paths exist → nil.
- mono-repo + monolith with `external_systems.simulators.path` declared but the directory missing → error mentions `external_systems.simulators.path`.
- mono-repo + monolith with `external_systems` omitted entirely → preflight skips the externals check and passes (no spurious failure).

### 5. Scaffold writes the new schema

**Files (edit):**

- `internal/steps/optivem_yaml.go`
- `internal/steps/optivem_yaml_test.go` (or wherever the existing translation tests live)

Decision 1 makes the schema mandatory at runtime when `system.architecture` is set, so the scaffold MUST emit it. `buildOptivemYAML` writes the new shape populated from the scaffold's own knowledge of the directories and languages it creates. The mapping below is the scaffold's internal table — if the scaffold layout ever changes, the table changes alongside it without affecting the runtime contract.

**System paths and languages.** For `monolith`, `system.path` and `system.lang` are populated directly. For `multitier`, `backend` and `frontend` are populated as `TierSpec`s.

| Architecture | system_lang  | system / backend.path             | backend.lang | frontend.path                     | frontend.lang |
| ------------ | ------------ | --------------------------------- | ------------ | --------------------------------- | ------------- |
| `monolith`   | `java`       | `system/monolith/java`            | `java`       | (omitted)                         | (omitted)     |
| `monolith`   | `dotnet`     | `system/monolith/dotnet`          | `dotnet`     | (omitted)                         | (omitted)     |
| `monolith`   | `typescript` | `system/monolith/typescript`      | `typescript` | (omitted)                         | (omitted)     |
| `multitier`  | `java`       | `system/multitier/backend-java`   | `java`       | `system/multitier/frontend-react` | `typescript`  |
| `multitier`  | `dotnet`     | `system/multitier/backend-dotnet` | `dotnet`     | `system/multitier/frontend-react` | `typescript`  |
| `multitier`  | `typescript` | `system/multitier/backend-node`   | `typescript` | `system/multitier/frontend-react` | `typescript`  |

Note: `frontend.lang` is `typescript` for every multitier variant because the only frontend the scaffold currently emits is React on TypeScript. Adding additional frontend frameworks later is out of scope (see "Out of scope").

**System-test path and language**, keyed on `scope.test_lang` (the existing scaffold input):

| test_lang    | system_test.path         | system_test.lang |
| ------------ | ------------------------ | ---------------- |
| `java`       | `system-test/java`       | `java`           |
| `dotnet`     | `system-test/dotnet`     | `dotnet`         |
| `typescript` | `system-test/typescript` | `typescript`     |

**Repo qualifiers.**

- For `mono-repo`, every tier's `repo:` is the workspace repo's slug (e.g. `optivem/shop`).
- For `multi-repo`, the scaffold's existing `cfg.SystemFullRepo` / `cfg.BackendFullRepo` / `cfg.FrontendFullRepo` map to the corresponding tier's `repo:`. By default `system_test.repo` mirrors `backend.repo` (or `system.repo` for monolith) — the operator can override post-scaffold by editing `gh-optivem.yaml`.

**External systems.** Per the related plan `shop/plans/20260505-move-external-systems-out-of-system.md`, scaffolded repos receive `external-real-sim/` and `external-stub/` at the workspace root (regardless of architecture or language). The scaffold emits the `external_systems:` block populated with both:

| repo_strategy | stubs.path      | stubs.repo                                                                                                              | simulators.path     | simulators.repo                  |
| ------------- | --------------- | ----------------------------------------------------------------------------------------------------------------------- | ------------------- | -------------------------------- |
| `mono-repo`   | `external-stub` | (workspace slug)                                                                                                        | `external-real-sim` | (workspace slug)                 |
| `multi-repo`  | `external-stub` | (by default `cfg.BackendFullRepo` for multitier or `cfg.SystemFullRepo` for monolith — operator can override post-scaffold) | `external-real-sim` | (same default as `stubs.repo`)   |

Even though `external_systems:` is optional in the schema, the scaffold always emits it for shop-derived projects since the related plan guarantees both directories exist post-scaffold. Operators who don't want externals declared can delete the block by hand.

Cross-check both tables against `internal/steps/apply_template.go` and the scaffolded `shop` template before committing — the tables are the **claim** to verify, not the source of truth. If a cell is wrong relative to what gh-optivem actually creates, fix the table, not the scaffold.

Tests:

- For each (architecture, system_lang, test_lang, repo_strategy) combo the scaffold supports, `buildOptivemYAML` produces a config whose values match the tables above and round-trip through `Validate` cleanly.
- A scaffolded `multi-repo + multitier` config has `system.backend.repo`, `system.frontend.repo`, `system_test.repo` all populated and pointing at distinct slugs (or matching, in `system_test`'s default).
- A scaffolded `mono-repo + monolith` config has all `repo:` fields populated with the same workspace slug.

### 6. Documentation

**Files (edit):**

- `README.md` — paragraph under the existing config section explaining the three top-level blocks (`project:`, `system:`, `system_test:`), the architecture polymorphism on `system:`, and pointing at the four sample configs in this plan as the canonical reference.
- `docs/atdd/process/system-interface-redesign.md` (in the `shop` repo, per the plan at `plans/20260504-180000-shop-system-interface-redesign-doc.md`) — add a one-line note that the agent now receives explicit write roots and won't bleed into out-of-scope stacks. Only if the doc has already landed in `shop` by the time this plan is executed; otherwise out of scope here and pick up in the existing shop-side phase doc plan.

No CLAUDE.md changes — the rule "don't bleed into out-of-scope stacks" is now enforced by the prompt + preflight, not by guideline.

## Verification

After all items land, re-run the rehearsal that exposed the original bug:

```bash
./scripts/atdd-rehearsal.sh --issue 61
```

Expected:

- The atdd-task agent's prompt contains:
  ```
  Architecture: monolith

  Allowed write roots:
  - System: system/monolith/java (lang: java)
  - System tests: system-test/java (lang: java)

  External-system roots (modify only when the ticket calls for stub/sim changes):
  - Stubs: external-stub
  - Simulators: external-real-sim
  ```
- The agent edits files only under `system/monolith/java/` and `system-test/java/`.
- `system/multitier/...`, `system/monolith/typescript/...`, `system/monolith/dotnet/...`, `system-test/typescript/...`, `system-test/dotnet/...` are untouched.
- A separately-rigged rehearsal where `system/monolith/java/` is renamed to `app/` and `system.path` is updated to `app` succeeds; one where the rename is done WITHOUT updating the config fails preflight with a readable error before any agent runs.
- Loading any of the four sample configs from this plan succeeds; deleting `system_test.path` from one of them and reloading fails validation with an error naming the missing field.
- A multitier rehearsal where `backend.lang=java` and `frontend.lang=typescript` produces a prompt that lists both languages distinctly — a regression test for the per-component-lang motivation (point 2 above).

## Migration of existing repos

This is a **breaking schema change** — every existing `gh-optivem.yaml` with `scope:` or `paths:` blocks fails validation after item 1 lands. Two viable migration paths, both out of band:

1. **Re-scaffold.** Re-run `gh optivem init` against the existing repo (overwrites `gh-optivem.yaml` with the new shape). Acceptable when the scaffolded layout still matches the on-disk layout — the common case.
2. **Hand-edit.** Copy the matching sample config from this plan into `gh-optivem.yaml`, adjust the values to the repo's actual layout and per-component languages. Acceptable for repos that have moved files around.

A `gh optivem config migrate` command is *not* in scope here — the population of repos using `gh-optivem.yaml` is small enough today (the `shop` template plus rehearsal scratch repos) that one of the two manual paths above is fine.

## Out of scope

- **`gh-optivem.local.yaml` sidecar for local clone paths.** Decision 6 is "three mechanisms is enough for now"; add a sidecar later if `--repo-dir` + env var + sibling convention turn out not to cover real workflows.
- **Validating that paths actually contain source files matching the declared `lang`.** Preflight stops at "directory exists and is git." A repo with `system.path` pointing at an empty directory passes preflight; the agent will fail informatively when it can't find files. Source-presence checks make preflight slow and brittle.
- **Multiple frontend frameworks.** The current `system/multitier/frontend-react/` is the only frontend variant the scaffold emits. If a Vue/Svelte/etc. frontend is added later, decide then whether `frontend.lang` covers the new case (e.g. `frontend.lang=typescript` regardless of framework) or a `frontend.framework:` field is needed; not in this plan.
- **Extending `gh optivem config init`** to prompt for the new fields interactively. The scaffold writes the block automatically (item 5); operators only hand-edit when overriding the scaffolded layout, and that is rare enough not to warrant interactive prompting.
- **A `gh optivem config migrate` command.** See "Migration of existing repos" above — manual migration is acceptable at the current scale.
