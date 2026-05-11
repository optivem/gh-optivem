# Migrate `system.json` / `tests.json` to YAML

## Motivation

The tool already uses YAML for its own project config (`gh-optivem.yaml`).
The two runner configs scaffolded into every student repo —
`system.json` and `tests-latest.json` / `tests-legacy.json` — are JSON.
Students therefore juggle two syntaxes for files that sit next to each
other and are conceptually the same kind of artifact (human-edited,
declarative, comment-friendly).

YAML wins here for three reasons:
1. **Comments.** The current JSON suites can't explain *why* a
   `testFilter` looks the way it does, or what a `sampleTest` is for.
   YAML lets us drop one-line hints next to each field.
2. **Consistency.** One format across `gh-optivem.yaml`, `system.yaml`,
   and `tests-latest.yaml` means one syntax for students to learn.
3. **Multi-line strings.** Setup commands and suite commands today are
   one-liners; YAML's `|` block lets future setup steps span lines
   without backslash hell.

Modern convention for *new* CLI dev tools would be TOML
(`Cargo.toml`, `pyproject.toml`, `ruff.toml`), but this repo has
standardised on YAML for its own config and the YAML library is
already a dependency. Picking TOML now would re-introduce the
two-format problem in a different shape.

## Out of scope

- **TOML.** Discussed and ruled out 2026-05-11 — would re-create the
  mismatch with `gh-optivem.yaml`.
- **Renaming `gh-optivem.yaml`.** That config keeps its current name
  and structure; this plan only touches the two runner configs.
- **Restructuring the schema.** Field names, nesting, validation
  rules stay byte-identical to today's JSON. This is a syntax
  migration, not a redesign.
- **Profiles or multi-environment YAML.** A separate idea; not
  bundled in.

## Open questions (resolve before phase 1)

1. **Field-name case.** JSON tags are camelCase (`composeFile`,
   `containerName`, `testFilter`, `testFilterJoin`). `gopkg.in/yaml.v3`
   maps Go struct fields to lowercase by default but `yaml:"..."`
   tags can hold any case. Two choices:
   - **(A) Keep camelCase** in YAML (`composeFile:`). Zero schema
     change; lets us read both JSON and YAML with one struct during
     the transition (JSON's `composeFile` and YAML's `composeFile`
     parse identically). **Recommended.**
   - **(B) Switch to snake_case** (`compose_file:`). More idiomatic
     YAML but forces a clean cutover and breaks shop's existing
     files in the same commit.
2. **File extension.** `.yaml` matches `gh-optivem.yaml` and
   `process-flow.yaml` already in the repo. **Recommended: `.yaml`.**
3. **Migration strategy for shop and any other already-scaffolded
   repos.** Three options:
   - **(A) Hard cutover** — bump tool version, students re-scaffold.
     Painful for shop which has hand-edited test suites.
   - **(B) Dual-read, single-write** — loaders accept either
     extension; scaffolder writes only YAML going forward.
     Old `.json` files keep working until the student deletes them.
     **Recommended.**
   - **(C) Auto-migrate** — first run of the new tool against a
     `.json` config rewrites it as `.yaml`. Too magical for a
     student-facing tool.

## Design summary

Two phases.

**Phase 1 — make the loaders YAML-first.** `LoadSystem` /
`LoadTests` (`internal/runner/config.go:87,124`) try `.yaml` first,
then fall back to `.json` for the same basename. Struct tags
gain `yaml:"..."` alongside the existing `json:"..."` (option A
above keeps the keys identical). Error messages stop asserting
"expected JSON file format".

**Phase 2 — make the scaffolder and the rest of the codebase
YAML-native.** Update the template applier, the `optivem_yaml.go`
defaults, the path resolver in `verify_paths.go`, and the docs.
Existing `.json` files keep working via the phase-1 fallback.

## Phase 1 — YAML-aware loaders (no scaffolder changes yet)

### Files to change

1. `internal/runner/config.go`
   - Add `yaml:"..."` tags mirroring every `json:"..."` tag on
     `SystemConfig`, `SystemEntry`, `Component`, `TestsConfig`,
     `SetupCommand`, `Suite`. Keys identical to JSON
     (`composeFile`, `containerName`, `testFilter`, …).
   - `LoadSystem(path string)` becomes a thin shim that picks the
     unmarshaller by extension:
     - `.yaml` / `.yml` → `yaml.Unmarshal`
     - `.json` (or anything else) → `json.Unmarshal`
   - Update both error messages (`expected JSON file format` →
     `expected JSON or YAML; <yaml error>`).
   - Same treatment for `LoadTests`.
2. `internal/runner/config_test.go`
   - Add YAML round-trip tests covering: happy path, every
     validation error (empty `systems[]`, missing `label`,
     missing `composeFile`, …), and the
     "wrong extension / wrong content" cross-check (e.g.
     a `.yaml` file with valid JSON content should still parse,
     since YAML is a JSON superset).
   - Keep all existing JSON tests untouched — they prove the
     fallback still works.
3. `go.mod` / `go.sum` — no change, `gopkg.in/yaml.v3` is already
   pulled in by other code paths.

### Acceptance for phase 1

- `go test ./internal/runner/...` green on both JSON and YAML
  fixtures.
- A hand-written `system.yaml` next to shop's `docker/system.json`
  is detected and loaded; the JSON file's data is ignored.
- The existing shop run (`gh optivem test system` against the
  current `.json` files) still passes — proves the fallback path
  doesn't regress.

## Phase 2 — scaffold YAML, migrate docs and probes

### Files to change

1. `internal/steps/optivem_yaml.go:17` (`scaffoldedSystemConfigPath`)
   and the matching `scaffoldedTestConfigPath` constant — flip
   `docker/system.json` → `docker/system.yaml`, `system-test/
   tests-latest.json` → `system-test/tests-latest.yaml`.
2. `internal/steps/apply_template.go` — wherever the template
   applier renames `docker/<testLang>/<arch>/...` paths
   (lines 560, 678, 90 — search for `system.json`), update to
   `.yaml`. Same for `tests-latest.json`.
3. The shop template repo itself (`shop` is a separate repo).
   **Out of scope for this plan** — a follow-up commit in `shop`
   replaces the JSON files with YAML equivalents. Until then the
   phase-1 fallback carries us.
4. `internal/atdd/runtime/actions/verify_paths.go` — the layout
   probe glob (`docker/*/<arch>/system.json` on line 56, and the
   flat `docker/system.json` on line 46) needs to probe both
   extensions during the transition. Suggested form:
   - Try `.yaml`, then `.yml`, then `.json`. Return the first hit.
   - Update the error message (line 70) to mention all three.
   - Same treatment for `tests-latest.{yaml,yml,json}` (lines
     47, 63).
5. `internal/atdd/runtime/actions/bindings.go:809,1015,1152` —
   user-facing error strings hardcoding `system.json /
   tests-latest.json`. Generalise to `system.{yaml,json} /
   tests-latest.{yaml,json}`.
6. `runner_commands.go:7,26,27,29,41,44,76,167` — comments and
   the `defaultSystemConfig` / `defaultTestConfig` constants.
   - `flagSystemUsage` mentions `system.json`; update wording.
   - `defaultSystemConfig` flips to `./system.yaml`; loader's
     extension switch handles the legacy `.json` case.
7. Tests under `internal/atdd/runtime/actions/`
   (`verify_paths_test.go`, `bindings_test.go`,
   `verify_classify_test.go`) — these create fixture
   `system.json` files with `{}` content. The phase-1
   loader treats `{}` as valid YAML too, so most fixtures
   keep working unchanged; add new test cases that scaffold
   `.yaml` and confirm the probe finds them.
8. `internal/projectconfig/config.go:79-81` — comment talks
   about `./system.json`; update.
9. Docs:
   - `README.md` (lines 137, 182, 187, 195, 196) — examples
     use `.json`; flip to `.yaml`.
   - `docs/gh-monitoring-process.md:52`
   - `docs/how-it-works.md`
   - `scripts/manual-test-runner-shop.sh` (lines 10-13, 28-30,
     39) — once shop has YAML versions checked in.
10. `internal/runner/config.go:2` — opening package comment
    states "JSON config files: a system.json …"; rewrite as
    "YAML config files (with legacy JSON fallback)".

### Acceptance for phase 2

- A freshly scaffolded student repo contains `docker/system.yaml`
  + `system-test/tests-latest.yaml`, no `.json` runner configs.
- The ATDD process-flow tests still find these configs (probe
  hits `.yaml` first).
- shop's existing `.json` files still load correctly (phase-1
  fallback proves itself in CI).
- README/docs walkthrough uses `.yaml` filenames end to end.

## Risks

1. **shop and any other in-flight student repos still have
   `.json`.** Mitigated by phase 1's dual-read. Owner of each
   repo can migrate at their own pace.
2. **YAML tab characters.** `gopkg.in/yaml.v3` rejects tabs for
   indentation, which is correct but unfamiliar to JSON-reared
   students. Mitigation: the scaffolded files are spaces-only;
   docs add a one-line "use spaces, not tabs" callout.
3. **CamelCase YAML looks non-idiomatic.** Real cost, but the
   alternative (snake_case) costs us the painless dual-read
   transition. Accept the lint friction for one release; a
   future plan can rename keys once nobody's reading `.json`
   anymore.
4. **The Norway problem.** `country: NO` parses as `false` in
   YAML 1.1. `yaml.v3` defaults to YAML 1.2 where this is fixed,
   but worth a one-line callout in the scaffolded template's
   header comment.

## Rollout

1. Land phase 1 in a single PR. No behavior change for any
   existing user; pure additive.
2. Wait for one shop CI run to prove the fallback works in
   anger.
3. Land phase 2 in a second PR. Bumps the scaffolder default
   and rewrites docs. Coordinated commit in `shop` to flip
   its checked-in `.json` files to `.yaml` (separate repo,
   separate PR).
4. Future cleanup PR (no date): once every academy repo is on
   YAML, remove the `.json` fallback from `LoadSystem` /
   `LoadTests` and drop the legacy-extension probes from
   `verify_paths.go`. Track via a TODO with a date.
