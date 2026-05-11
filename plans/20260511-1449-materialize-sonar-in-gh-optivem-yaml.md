# Materialize SonarCloud org + project keys in `gh-optivem.yaml`

> 🤖 **Picked up by agent** — `Valentina_Desk` at `2026-05-11T15:34:08Z`

> ⚠️ **Needs explicit human approval before implementation. Discuss first.**
> This plan is a draft. Do not execute any step until the author signs off on
> the overall shape (and the open questions in the final section).

## Context

SonarCloud identifiers are currently **derived** at multiple call sites
from `owner` + `repo` + `arch` + `repo_strategy`, never persisted:

- **Org** = `cfg.OwnerLower` — built inline in `main.go:208`
  (`shell.NewSonarCloud(cfg.SonarToken, cfg.OwnerLower)`).
- **Project keys** = `internal/steps/cleanup.go:14` `GetSonarProjectKeys(cfg)`:
  - monolith + monorepo: `<owner>_<repo>-system`
  - monolith + multirepo: `<owner>_<repo>-system` (via `SystemRepo`)
  - multitier + monorepo: `<owner>_<repo>-backend`, `<owner>_<repo>-frontend`
  - multitier + multirepo: `<owner>_<repo>-backend`, `<owner>_<repo>-frontend`
- **In scaffolded workflows**: burned in at template apply time via
  `monolithSonarKeyReplacements` / `multitierSonarKeyReplacements`
  (`internal/steps/apply_template.go:739, 748`) and the
  `sonar.organization` rewrite in `internal/steps/replacements.go:183, 185`.

### Why materialize at all

**Inconsistency with the schema's existing convention.** Repo names and
tier paths are equally derivable (multirepo names from
`<repo>-{frontend,backend,system}`, paths from `resolveScaffoldPaths`
defaults), yet **both are materialized into `gh-optivem.yaml` by
`config init`** so every load-bearing identity is visible in one place.
SonarCloud identities are the one external identity class still missing
from this pattern.

### The system-test inconsistency this plan also fixes

Verified 2026-05-11 against `optivem/shop@main`. The shop template's
system-test build files carry **language-specific** Sonar keys:
- `system-test/java/build.gradle` → `sonar.projectKey = optivem_shop-tests-java`
- `system-test/dotnet/Run-Sonar.ps1` → `$projectKey = "optivem_shop-tests-dotnet"`
- `system-test/typescript/Run-Sonar.ps1` → `-Dsonar.projectKey=optivem_shop-tests-typescript`

This is correct for the shop template's own CI (the template hosts all
three test suites side-by-side and needs separate Sonar projects to keep
analyses distinct), but it's wrong for scaffolded repos: the existing
scaffold convention is to **strip language out** of suffix identifiers
(`-monolith-<lang>` → `-system`, `-multitier-backend-<lang>` →
`-backend`, etc. — see `apply_template.go:739-753`). The
`-tests-<lang>` suffix slipped through because the replacement table
doesn't cover it.

The result: scaffolded repos today carry `<owner>_<repo>-tests-<testLang>`
in their system-test build files, SonarCloud auto-provisions that
project on first push, and gh-optivem never names it. This plan fixes
the missing replacement and brings system-test in line with the
language-agnostic suffix convention. After the fix, scaffolded repos
carry the uniform `<owner>_<repo>-system-test`, the derivation produces
the same value, and `finalize.go` creates it explicitly.

### Scope

Two changes that fit together:

1. **Scaffold fix.** Add the missing `-tests-<lang>` → `-system-test`
   replacement (mirroring the existing per-tier suffix replacements)
   plus a forbidden-ref check. Scaffolded repos now get uniform
   language-stripped suffixes across all tiers.
2. **Materialize the derivation output into `gh-optivem.yaml`.** The
   derivation gains a system-test output (uniform suffix, no testLang
   input) and the YAML carries `sonar.organization` plus
   `sonar_project` on every code tier (system or backend+frontend, plus
   system_test).

These two changes are bundled because (1) makes (2) clean — without
the replacement fix, the system-test key would have to depend on
testLang, splitting it from the language-agnostic pattern every other
tier follows.

## Design

### Schema addition

Org goes at the root (singleton — one SonarCloud account per scaffold);
the project key goes on each **code tier** alongside its `path` /
`repo` / `lang`, since project key is a property of "the code being
analyzed", 1:1 with the tier itself. Every code tier — system (monolith)
or backend+frontend (multitier), plus system_test — gets one.

```yaml
sonar:
  organization: optivem

system:
  architecture: multitier
  backend:
    path: backend
    repo: optivem/page-turner-backend
    lang: java
    sonar_project: optivem_page-turner-backend
  frontend:
    path: frontend
    repo: optivem/page-turner-frontend
    lang: typescript
    sonar_project: optivem_page-turner-frontend

system_test:
  path: system-test
  repo: optivem/page-turner
  lang: java
  sonar_project: optivem_page-turner-system-test
```

Monolith case:

```yaml
sonar:
  organization: optivem

system:
  architecture: monolith
  path: system
  repo: optivem/page-turner
  lang: java
  sonar_project: optivem_page-turner-system

system_test:
  path: system-test
  repo: optivem/page-turner
  lang: java
  sonar_project: optivem_page-turner-system-test
```

Why per-tier and not a centralized list:

- Matches the existing schema pattern — `path`, `repo`, `lang` already
  live on the tier. `sonar_project` is just another property of that tier.
- Monolith stays clean — one `sonar_project:` on `system:`, no awkward
  centralized `components: { system: <key> }` wrapper for a one-entry list.
- No ordering ambiguity — each key sits next to its tier; no positional
  vs role-keyed dilemma.
- ExternalSystems (stubs, simulators) naturally have no `sonar_project:`
  — those are JSON configs / ad-hoc simulators, not Sonar-scanned.

### Field shape

- `sonar.organization` — root-level singleton. Today always equals
  `strings.ToLower(owner)`; future-proofed as its own field so an
  operator whose SonarCloud org differs from their GitHub owner can
  override it without a schema migration.
- `sonar_project` — added to `System` (monolith path) and `TierSpec`
  (used by `system_test`, `system.backend`, `system.frontend`).
  Required on every code tier when architecture is set; rejected on
  `ExternalSpec` (Sonar isn't applicable to stubs/simulators).

### Derivation

Lift the derivation out of `internal/steps/cleanup.go` and into
`internal/projectconfig/sonar.go`. **Extend the output** to include a
system-test key (made possible by the scaffold fix below — without
that fix, the system-test key would have to depend on testLang).
Inputs unchanged: 4-tuple of `owner`, `repo`, `arch`, `repo_strategy`.

```go
// DeriveSonarProjects returns the SonarCloud project keys for each
// code tier given a system's identity. system-test is always present;
// the others depend on architecture.
type DerivedSonar struct {
    System     string // monolith only
    Backend    string // multitier only
    Frontend   string // multitier only
    SystemTest string // always
}

func DeriveSonarProjects(owner, repo, arch, repoStrategy string) DerivedSonar
```

Concrete values by quadrant (the scaffold fix below ensures these match
what scaffolded build files actually carry):

|                          | System              | Backend                | Frontend                | SystemTest                  |
|--------------------------|---------------------|------------------------|-------------------------|-----------------------------|
| monolith + monorepo      | `<o>_<r>-system`    | —                      | —                       | `<o>_<r>-system-test`       |
| monolith + multirepo     | `<o>_<r>-system`    | —                      | —                       | `<o>_<r>-system-test`       |
| multitier + monorepo     | —                   | `<o>_<r>-backend`      | `<o>_<r>-frontend`      | `<o>_<r>-system-test`       |
| multitier + multirepo    | —                   | `<o>_<r>-backend`      | `<o>_<r>-frontend`      | `<o>_<r>-system-test`       |

Notes:
- The multitier+multirepo case uses the **base** repo name, not the
  suffixed component repo names — matches existing behaviour
  (`GetSonarProjectKeys` uses `cfg.Repo`, not `cfg.BackendRepo`). The
  monolith+multirepo case uses `cfg.SystemRepo` (= `<base>-system`),
  producing the same single-`-system` suffix as monolith+monorepo;
  verified against `TestSonarProjectKeys` (`config_test.go:271-278`).
- system-test key is identical across all four quadrants (uniform
  suffix, no testLang). system-test always lives in the base repo
  regardless of `repo_strategy`.

### Scaffold-time fix: language stripping for the system-test suffix

The existing per-arch suffix-replacement helpers (`apply_template.go:
739-753`) strip language out of the system/backend/frontend tiers but
miss system-test. Add a new helper that handles system-test in both
architectures:

```go
// systemTestSonarKeyReplacements returns the language-stripping
// replacement for the system-test Sonar key suffix. The shop template
// carries -tests-{java,dotnet,typescript} per its multi-suite CI;
// scaffolded repos host only one suite and use the language-agnostic
// -system-test suffix, matching the existing -system / -backend /
// -frontend pattern.
func systemTestSonarKeyReplacements() [][2]string {
    return [][2]string{
        {"-tests-java",       "-system-test"},
        {"-tests-dotnet",     "-system-test"},
        {"-tests-typescript", "-system-test"},
    }
}
```

Call site: both `applyMonolithTemplate` and `applyMultitierTemplate`
paths in `apply_template.go` already invoke `monolithSonarKeyReplacements`
/ `multitierSonarKeyReplacements` against the scaffolded text files via
`templates.FixupAllTextFiles`. Add an additional call against
`systemTestSonarKeyReplacements()` in the same place.

Update `forbiddenTemplateRefs` (`apply_template.go:812+`) so a missed
case can't silently survive — append `-tests-java`, `-tests-dotnet`,
`-tests-typescript` to the `monolithForbiddenRefs` /
`multitierForbiddenRefs` returns. After
`ValidateNoLeftoverTemplateRefs` runs, no scaffolded repo can ship
with the language-suffixed system-test key.

Verification: snapshot a scaffold's `system-test/<lang>/build.gradle`
(or `Run-Sonar.ps1`) before and after this step to confirm the
`-tests-<lang>` → `-system-test` rewrite lands. Update the existing
`TestRewritePublisherRefsSonar` fixture (`replacements_test.go:114-148`)
to include a `system-test/java/build.gradle` line with
`optivem_shop-tests-java` and assert it survives as
`<owner>_<repo>-system-test` post-rewrite.

### `config init` materialization

`gh optivem config init` already derives values it then materializes
(repo names, paths). Add the same treatment:

1. Compute `organization = strings.ToLower(f.Owner)`.
2. Compute the four-field tuple via
   `DeriveSonarProjects(f.Owner, f.Repo, f.Arch, f.RepoStrategy)`.
3. Set `cfg.Sonar.Organization`, `cfg.System.SonarProject` (monolith)
   or `cfg.System.Backend.SonarProject` +
   `cfg.System.Frontend.SonarProject` (multitier), and
   `cfg.SystemTest.SonarProject` on the `projectconfig.Config`
   returned by `ValidateAndDeriveForYAML`.

No new CLI flags — the values are pure functions of inputs already on
`RawFlags`.

### Validation

In `projectconfig.Validate`, the new rules:

- **Rule 17.** `sonar.organization` is required when
  `system.architecture` is set; same gate as the existing tier-path rules.
- **Rule 18.** Each code tier with arch set requires its
  `sonar_project`:
  - monolith → `system.sonar_project`
  - multitier → `system.backend.sonar_project` +
    `system.frontend.sonar_project`
  - always when arch set → `system_test.sonar_project`
- **Rule 19.** Consistency: org + each key match the canonical
  derivation from `(owner, repo, arch, repo_strategy)`. `owner` and
  base-`repo` are parsed from `system_test.repo` (always
  `<owner>/<base>`, set when arch is set, always the base repo
  regardless of strategy). Stale hand-edits of owner/repo elsewhere in
  the YAML therefore fail this check.

All three rules live **inside** `projectconfig.Validate`, so they run
for free through every load path — including `gh optivem config
validate`, which today goes
`config_commands.go:113 → projectconfig.LoadFromPath → parse → Validate`.

`sonar_project` is added to the existing shared `TierSpec` (the same
struct backing both `system_test` and `system.backend` / `system.frontend`).
Validate rejects it being set on `ExternalSpec` (Sonar isn't applicable
there) — enforced by not adding the field to `ExternalSpec` at all.

### Runtime consumption

Two call sites switch from re-derivation to YAML reads:

1. **`finalize.go:CreateSonarCloudProjects`** — replace the
   `GetSonarProjectKeys(cfg)` iteration with a walk over the loaded
   `projectconfig.Config`'s tier-level `sonar_project` fields (system
   or backend+frontend, plus system_test always). **Behaviour change:**
   `CreateProject` now runs for the system-test key too (3 projects
   for monolith, 4 for multitier — up from 1/2), dropping the
   dependency on SonarCloud's auto-provision on first scan.
2. **`main.go:208`** — replace
   `shell.NewSonarCloud(cfg.SonarToken, cfg.OwnerLower)` with the org
   read from the YAML.

`cleanup-orphans.sh` is **out of scope** — it keeps its prefix-scan
behaviour. The new YAML fields are a parallel record it could read in
a follow-up if needed.

### Scaffolded workflow files

The `sonar.projectKey=…` / `sonar.organization=…` strings in
scaffolded workflow files stay frozen at scaffold time (current
behavior, now corrected for the system-test case by the scaffold fix
above). No runtime YAML read from inside scaffolded workflows — those
files live in a downstream repo that doesn't necessarily have
gh-optivem installed.

## Steps

### Step 1 — Fix the missing system-test suffix replacement

This is a prerequisite for the materialization: until scaffolded
build files carry the uniform `<owner>_<repo>-system-test` key, the
derivation can't predict what the runtime will actually push to.

In `internal/steps/apply_template.go`:

1. Add `systemTestSonarKeyReplacements()` returning
   `{-tests-java, -tests-dotnet, -tests-typescript}` → `-system-test`.
2. Invoke it via `templates.FixupAllTextFiles` in both
   `applyMonolithTemplate` and `applyMultitierTemplate` paths, next to
   the existing `monolithSonarKeyReplacements` /
   `multitierSonarKeyReplacements` calls.
3. Append `-tests-java`, `-tests-dotnet`, `-tests-typescript` to
   `monolithForbiddenRefs` and `multitierForbiddenRefs` so
   `ValidateNoLeftoverTemplateRefs` flags any leftover.

Update `internal/steps/replacements_test.go:TestRewritePublisherRefsSonar`
fixture to include a `system-test/java/build.gradle` snippet carrying
`optivem_shop-tests-java`, and assert it lands as
`<owner>_<repo>-system-test` post-rewrite. Add equivalents for the
.NET and TypeScript fixtures.

### Step 2 — Lift derivation to `internal/projectconfig`

Create `internal/projectconfig/sonar.go` with `DeriveSonarProjects`
returning the four-field `DerivedSonar` struct. system/backend/frontend
values mirror today's `GetSonarProjectKeys`; the new `SystemTest` field
returns `<owner>_<repo>-system-test` (uniform, no testLang input).

Old `GetSonarProjectKeys(*config.Config)` (in
`internal/steps/cleanup.go`) becomes a thin adapter that calls
`DeriveSonarProjects` and returns the non-empty values as a slice — so
existing call sites keep compiling unchanged. The adapter is removed
in Step 5 once `finalize.go` reads from the YAML.

Unit tests in `internal/projectconfig/sonar_test.go` covering the four
arch×strategy quadrants — parallels the existing
`TestSonarProjectKeys` at `internal/config/config_test.go:256-306`.
Asserts the system-test key is identical across all four quadrants.

### Step 3 — Add Sonar fields to the schema

In `internal/projectconfig/config.go`:

```go
type Config struct {
    // … existing fields …
    Sonar Sonar `yaml:"sonar,omitempty"`
}

type Sonar struct {
    Organization string `yaml:"organization,omitempty"`
}

type System struct {
    // … existing fields …
    SonarProject string `yaml:"sonar_project,omitempty"` // monolith only
}

type TierSpec struct {
    // … existing fields …
    SonarProject string `yaml:"sonar_project,omitempty"`
}
```

`ExternalSpec` deliberately stays without `SonarProject` (Sonar isn't
applicable to stubs/simulators, and the absent field is the documentation).

In `Validate`, add Rules 17, 18, 19. Helper inside Validate:

```go
// parseOwnerRepo splits "<owner>/<repo>" into its parts; returns an
// error if the input lacks the slash. Caller has already verified
// system_test.repo is non-empty (via the presence rules).
func parseOwnerRepo(systemTestRepo string) (owner, repo string, err error)
```

Test fixtures in `internal/projectconfig/config_test.go`:
- monolith + monorepo: `system.sonar_project` + `system_test.sonar_project`.
- multitier + monorepo: backend + frontend + system_test `sonar_project`s.
- multitier + multirepo: same shape.
- Negative: missing `sonar.organization` when arch is set → reject.
- Negative: missing `sonar_project` on any required code tier → reject.
- Negative: stale `sonar.organization` not matching
  `strings.ToLower(owner)` → reject (consistency).
- Negative: hand-edited mismatched key → reject (consistency).
- Negative: `system_test.repo` missing or malformed (`foo` with no
  slash) → reject with the existing tier-completeness error before
  the Sonar rule fires.
- Positive: no arch set, sonar block absent → accepted.

End-to-end test for `gh optivem config validate` (in
`config_commands_test.go`):
- Hand-mutate a known-good fixture's `system.backend.sonar_project` to
  a stale value → `runConfigValidate` returns the consistency error.
- Same for `sonar.organization` and `system_test.sonar_project`.

### Step 4 — `config init` writes the block

In `internal/config/config.go:ValidateAndDeriveForYAML`, after the
`deriveMultirepoNames` call, populate the new fields on the returned
`*projectconfig.Config`:

```go
derived := projectconfig.DeriveSonarProjects(f.Owner, f.Repo, f.Arch, f.RepoStrategy)
cfg.Sonar.Organization = strings.ToLower(f.Owner)
switch f.Arch {
case "monolith":
    cfg.System.SonarProject = derived.System
case "multitier":
    cfg.System.Backend.SonarProject = derived.Backend
    cfg.System.Frontend.SonarProject = derived.Frontend
}
cfg.SystemTest.SonarProject = derived.SystemTest
```

Locate where `steps.WriteOptivemYAMLToPath` builds the
`projectconfig.Config` value (likely `internal/steps/registration.go`
or a YAML emitter helper); thread the new fields through.

Update the YAML emission golden tests (in
`internal/steps/replacements_test.go` or wherever the round-trip is
covered) to assert the new fields are written.

### Step 5 — Runtime call sites read from YAML

Two changes:

- **`internal/steps/finalize.go:CreateSonarCloudProjects`** — replace
  the `GetSonarProjectKeys(cfg)` iteration with a walk over the loaded
  `projectconfig.Config`'s tier-level `sonar_project` fields. Thread
  the loaded projectconfig through to this function. **Behaviour
  change:** the system-test project is now created explicitly (3
  projects for monolith, 4 for multitier — up from 1/2), no longer
  relying on SonarCloud's auto-provision.
- **`main.go:208`** —
  `shell.NewSonarCloud(cfg.SonarToken, projectCfg.Sonar.Organization)`
  instead of `cfg.OwnerLower`.

Remove `internal/steps/cleanup.go:GetSonarProjectKeys`. Confirm no
other callers via `grep -r GetSonarProjectKeys`. Drop the
`computeSonarKeysLocal` shim in `internal/config/config_test.go:308`
along with its enclosing `TestSonarProjectKeys` — the equivalent test
now lives in `internal/projectconfig/sonar_test.go` (Step 2).

### Step 6 — `gh optivem config validate` surfaces Sonar drift

No code change needed — `runConfigValidate` (`config_commands.go:127`)
already routes through `projectconfig.LoadFromPath → parse → Validate`,
so Rules 17/18/19 fire from inside it automatically. The work here is:

- **Help text refresh.** Update the `Long:` block in
  `newConfigValidateCmd` (`config_commands.go:101`) to mention that
  the validation now covers the Sonar block — both presence (org +
  per-tier project keys on all code tiers) and consistency (each key
  matches the canonical derivation).
- **Example refresh.** Add an `Example:` showing a hand-edit drift
  case being caught.

### Step 7 — Verify

- `go test ./internal/projectconfig/... ./internal/config/... ./internal/steps/...`
  passes (per [Windows go-test memory note](../../../Users/valen_4rjvn9e/.claude/projects/C--GitHub-optivem-academy-gh-optivem/memory/feedback_go_test_windows.md),
  scope to specific packages — never `go test ./...` unbounded).
- End-to-end rehearsal: `gh optivem config init` produces a YAML
  containing `sonar.organization` plus `sonar_project` on each code
  tier (2 for monolith — `system` + `system_test`; 3 for multitier —
  `backend` + `frontend` + `system_test`).
- `gh optivem init` with that YAML creates **3** SonarCloud projects
  for monolith / **4** for multitier (vs 1/2 today). All 4 carry
  language-stripped suffixes; no `-tests-<lang>` survives anywhere in
  the scaffolded repo.
- Hand-mutate the YAML's `system.backend.sonar_project` →
  `gh optivem config validate` rejects with the consistency error.

## Open questions for discussion

1. **Where does the derivation consistency check fire?** Decided:
   inside `projectconfig.Validate`, deriving owner/base-repo from
   `system_test.repo`. Rationale: this makes `gh optivem config
   validate` surface stale Sonar hand-edits for free.
2. **Should `CreateSonarCloudProjects` now explicitly create the
   system-test project?** Decided: **yes**. Once the scaffold fix
   (Step 1) lands and the derivation includes system-test, the runtime
   should iterate all 3/4 keys. Drops the SonarCloud auto-provision
   dependency.
3. **Do we materialize the namespaces / package names too?** Decided:
   **skip**. No external consumer needs them at runtime.
4. **Override of `sonar.organization`.** Decided: **no flag now**.
   YAGNI; the field exists in the YAML so a hand-edit works if needed.
5. **`gh optivem config refresh`.** Decided: **out of scope**.

All open questions resolved.

## Out of scope

- Tracking the SonarCloud projects of pre-existing scaffolds that
  carry `<owner>_<repo>-tests-<testLang>` (created by `gh optivem init`
  before Step 1 lands). Those projects stay orphaned on SonarCloud;
  cleanup is operator-driven via `cleanup-orphans.sh` or the SonarCloud
  UI. No migration script.
- Adding language namespace / package name materialization (open question 3).
- A `config refresh` command (open question 5).
- Changing the scaffolded repo's workflow files to read from
  `gh-optivem.yaml` at CI time (the scaffolded repos don't have
  gh-optivem installed; template-time substitution stays the contract).
- Any changes to `cleanup-orphans.sh`. The script keeps its
  prefix-scan behaviour; the new YAML fields are a parallel record it
  could read in a follow-up if the prefix scan ever proves problematic.
