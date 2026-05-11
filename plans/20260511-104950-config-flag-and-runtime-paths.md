# `--config` flag + `system_config:` / `test_config:` in `gh-optivem.yaml`

## Motivation

Today every command that reads `gh-optivem.yaml` (`compile`, the `atdd`
subtree, `config validate`) hardcodes the location to
`<cwd>/gh-optivem.yaml` via `projectconfig.Load(cwd)`
(`compile_commands.go:88-98`). Two friction points fall out of that:

1. **No way to use a non-default filename.** Users who maintain
   multiple gh-optivem configurations in the same checkout (the shop
   repo carries a monolith × multitier × three-language matrix) have
   no override flag — they have to swap files in place or `cd` into
   a subdirectory.
2. **Missing-file error is terse.** The current message reads
   `no gh-optivem.yaml in <dir>; run `gh optivem config init` first`
   (`compile_commands.go:94`). It tells the user *what* to do but
   doesn't offer to do it. On a TTY an interactive prompt would shave
   one round-trip.

The runner flags solve the analogous problem for `system.json` and
`tests.json` (`runner_commands.go:40-114`). `gh-optivem.yaml` deserves
the same treatment, and `system.json` / `tests.json` deserve a way to
opt out of being passed on every invocation when their paths are
fixed for the project.

## Design summary

Two phases, both small.

**Phase 1 — universal `--config` mechanism.** A persistent root flag
points at any `gh-optivem.yaml` file, with an env-var alias for
shell-session pinning. Missing-file error becomes an interactive
prompt on a TTY.

**Phase 2 — optional path fields in `gh-optivem.yaml`.** Users who
have stable `system.json` / `tests.json` paths declare them in their
`gh-optivem.yaml` once and drop the per-command flags. Existing
`--system-config` / `--test-config` flags stay as the explicit
override.

Explicitly **not** introducing:
- A `profiles:` block. Shop's lang × arch matrix is shop-specific and
  is satisfied by maintaining multiple `gh-optivem.*.yaml` files and
  selecting via `--config`. Don't impose that complexity on every
  scaffolded user.
- A `--suite` flag or `legacy_test_config:` field. Legacy isn't a
  first-class concept in the tool; it's just another file someone can
  point `--test-config` at. Removes vocabulary from the CLI.

## Phase 1 — `--config` / `-c` + `GH_OPTIVEM_CONFIG` + missing-file prompt

Steps 1, 2, 3, 5 landed 2026-05-11. Notable resolution decisions:

- atdd's local `--config` flag (`atdd_commands.go:172, :268`) was the
  same semantic as the proposed persistent root flag. To keep a single
  source of truth, both local declarations were removed and the
  persistent root `--config` / `-c` takes over for atdd too. (The
  plan's note that Cobra rejects child-shadows-ancestor was incorrect;
  pflag's `AddFlagSet` silently skips the parent flag in that case.
  Removal is for unification, not breakage.)
- `--dir` removed from `config validate`; kept on `config init` as a
  parent-directory selector. `config init` now also honors `--config`
  as an exact target path (overrides `--dir` when set).
- atdd preflight now hard-errors on missing `gh-optivem.yaml` (was
  silent no-op via `Load(cwd)` returning nil cfg). Consistent with
  compile + config validate.
- `driver.loadDriverConfig` retains a programmatic-caller affordance:
  empty `Options.ConfigPath` falls back to `Load(repoPath)` so the
  embedded smoke test (`embedded_smoke_test.go:207`) keeps passing.
  The cobra layer never passes empty after the resolver.

- [ ] Step 4: Missing-file interactive prompt. — ⏳ Deferred:
  Step 4 as written requires extracting `config init`'s body into a
  callable package AND adding interactive prompting for the required
  flags (`--owner`, `--repo`, `--arch`, language, paths). The current
  error wording (`no gh-optivem.yaml at <path>; run gh optivem config
  init first`) is preserved. Reopen when the interactive flag-
  collection design is settled.

## Phase 2 — `system_config:` / `test_config:` fields in `gh-optivem.yaml`

### Step 6 — Extend `projectconfig.Config`

In `internal/projectconfig/config.go`, add two optional string fields:

```go
type Config struct {
    // ... existing fields ...

    // SystemConfig is the path to system.json for `gh optivem run|build|test|stop|clean`.
    // Optional; if empty, commands fall back to ./system.json or the --system-config flag.
    SystemConfig string `yaml:"system_config,omitempty"`

    // TestConfig is the path to tests.json for `gh optivem test`.
    // Optional; if empty, commands fall back to ./tests.json or the --test-config flag.
    TestConfig string `yaml:"test_config,omitempty"`
}
```

Both optional. A `gh-optivem.yaml` without either field behaves
exactly like today (this is critical — every existing repo's file
keeps working).

`Validate` doesn't need to check these — they're free-form paths and
file-existence checks happen at load-time, not validate-time. Match
the existing convention.

Auto-populate split by caller (see resolved questions):

- `steps.WriteOptivemYAML` (scaffolder, `gh optivem init`):
  populates `system_config: docker/system.json` and
  `test_config: system-test/tests-latest.json` — the paths it just
  produced via `copySystemTests` and the flattening rule
  (`apply_template.go:558-562`). Without this, scaffolded repos
  don't work from repo root without `--system-config`.
- `steps.WriteOptivemYAMLToPath` (called from `gh optivem config
  init` in a hand-rolled repo): leaves both fields empty — opt-in.
  Document in `config init` help text.

Both paths use `yaml:",omitempty"` so empty fields don't appear in
the written file.

### Step 7 — Three-tier resolution in `loadSystem` / `loadTests`

`runner_commands.go:72-81`:

```go
func loadSystem(path string) (*runner.SystemConfig, error) {
    sys, err := runner.LoadSystem(path)
    return sys, hintIfMissing(err, "--system-config", defaultSystemConfig)
}
```

Change to:

1. If `--system-config` was passed explicitly (non-default), use it
   (today's behaviour).
2. Else, read `gh-optivem.yaml` (via the Phase 1 resolver). If
   `SystemConfig` is non-empty, use it.
3. Else, fall back to `defaultSystemConfig` (`./system.json`).

Same shape for `--test-config` ↔ `TestConfig`.

Subtle: every runner subcommand currently declares `--system-config`
with `defaultSystemConfig` as the literal default
(`runner_commands.go:114`, `:149`, `:180`, `:211`, `:289`, etc.).
Cobra doesn't expose "did the user pass this flag?" by default —
need either `cmd.Flags().Changed("system-config")` or change the
default to `""` and resolve manually in each Run. The latter is
cleaner. Pre-flight: confirm no script or doc relies on the
documented default `./system.json` appearing in `--help`.

`hintIfMissing` (`runner_commands.go:60-68`) error message stays
inline (don't defer to `--help`) and lists the three knobs in
resolution-precedence order (flag → YAML → default), so the hint
also documents lookup order:

```
hint: pass --system-config <path>, set system_config: in gh-optivem.yaml,
      or create ./system.json
```

Same shape for the `tests.json` variant.

### Step 8 — Tests

Extend `runner_commands_test.go`:

- `gh-optivem.yaml` with `system_config:` set, no flag passed,
  custom path is used.
- Flag overrides YAML field.
- Empty YAML field falls back to `./system.json`.
- Same matrix for `test_config:`.

`config_commands_test.go`: round-trip Write→Load preserves the new
fields when set, omits them when empty.

### Step 9 — Docs

`README.md` and `docs/` (if present) get a short section:
"Pointing at non-default configs." Show the three knobs in
ascending order of permanence:

```bash
# One-shot
gh optivem -c ./gh-optivem.shop-monolith.yaml test system

# Shell session
export GH_OPTIVEM_CONFIG=./gh-optivem.shop-monolith.yaml
gh optivem test system

# Per-project default in gh-optivem.yaml itself
system_config: docker/java/monolith/system.json
test_config:   system-test/java/tests-latest.json
```

Shop's `CLAUDE.md` (in the consuming repo) can also be updated to
show the new ergonomic, but that's a separate commit in `shop/`.

## Resolved questions

- **Resolver lives in `internal/projectconfig`.** New helper
  `ResolvePath(flagVal string) (path string, explicit bool)` sits
  alongside `Load` / `LoadFromPath` — the `LoadFromPath` doc already
  names `--config` as the future caller, so the seam is coherent.
  Callers under `internal/atdd/runtime/driver/` (which already
  imports `projectconfig`) can use it without round-tripping through
  cobra to read the persistent flag.
- **`--dir` is hard-dropped from `config validate`, kept on `config
  init`.** `--config` fully subsumes `--dir` for validate, and the
  two-ways-to-say-the-same-thing carries no back-compat value worth
  preserving here. `config init` keeps `--dir` because "where to
  scaffold a fresh file" is a genuinely different semantic from
  "which existing file to read." Update `config_commands.go:137`
  (remove the `--dir` flag on validate) and the help text on
  `:130` (drop the `--dir ./some-other-repo` example).
- **Auto-populate split by caller.** `steps.WriteOptivemYAML` (the
  scaffolder) writes `system_config: docker/system.json` and
  `test_config: system-test/tests-latest.json` because those are
  literally the paths it just produced via `copySystemTests`
  (`apply_template.go:89-110`) and the flattening rule
  (`apply_template.go:558-562`). Neither matches the runner default
  `./system.json` / `./tests.json`, so without auto-populate a
  freshly-scaffolded repo doesn't work without an explicit flag —
  defeating Phase 2. `steps.WriteOptivemYAMLToPath` (called from
  `gh optivem config init` against a hand-rolled repo) leaves both
  fields empty — it doesn't know the layout, and opt-in is correct
  there.

## Out of scope

- **Profiles in `gh-optivem.yaml`.** Shop's multi-combination case is
  satisfied by maintaining multiple `gh-optivem.*.yaml` files and
  selecting via `--config`. No `profiles:` block, no `--profile` /
  `-P` flag. Discussed and ruled out 2026-05-11.
- **`--suite latest|legacy` flag and `legacy_test_config:` field.**
  Legacy is not a first-class concept in the tool. Users wanting
  the legacy suite pass `--test-config ./tests-legacy.json`
  explicitly. Discussed and ruled out 2026-05-11.
- **Auto-detect / walk-up search for `gh-optivem.yaml`** (cargo /
  git-style upward traversal). Not requested; current explicit-cwd
  behaviour is fine and the new flag/env covers the same need.
- **Consolidating `system.json` / `tests.json` schemas into
  `gh-optivem.yaml`** outright. Different lifecycles, different
  audiences. Path pointers are the right level of consolidation;
  schema merge is a separate, much bigger conversation.
