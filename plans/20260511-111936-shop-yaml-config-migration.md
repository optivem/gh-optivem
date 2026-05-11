# Migrate shop's `systems.json` / `tests-*.json` to YAML

## Motivation

The companion plan in gh-optivem
(`20260511-170000-yaml-config-migration.md`, now landed) flipped:

- the scaffolder's defaults (`scaffoldedSystemConfigPath` /
  `scaffoldedTestConfigPath`) to `.yaml`,
- the runner's defaults (`./systems.yaml` / `./tests.yaml`) and its
  CLI flag descriptions,
- the ATDD path probe (`docker/systems.{yaml,yml,json}` plus the
  templated layout under `docker/<lang>/<arch>/`),
- the README, runner package comment, and `docs/gh-monitoring-process.md`.

All of these accept JSON for now via the phase-1 extension dispatch
(`.yaml`/`.yml` → YAML codec, anything else → JSON), so shop's
existing `.json` files keep loading without a code change. This plan
finishes the migration in shop itself: rewrite the 12 checked-in
config files as YAML and flip every reference (workflows, scripts,
docs, narrative comments) to the new extensions.

The motivation is the same as the gh-optivem plan: one config syntax
across `gh-optivem.yaml`, `systems.yaml`, and `tests-*.yaml`; inline
comments next to each suite / system entry; multi-line strings if
setup commands ever need them.

## Out of scope

- **Schema changes.** Field names and shape (`composeFile`,
  `containerName`, `testFilter`, `setupCommands`, etc.) stay
  byte-identical. The `yaml:"..."` tags on the runner structs already
  honour the camelCase JSON keys verbatim, so a `cp systems.json
  systems.yaml` round-trips. A future plan can rename keys to
  snake_case once nobody reads `.json` anywhere; that is *not* this
  plan.
- **Profiles or multi-environment configs.** Same as the gh-optivem
  plan — separate idea, not bundled.
- **`test-config-{env}-{mode}.json` files** under
  `system-test/<lang>/config/`. These are the **test-process** config
  files (URLs / endpoints consumed by Playwright / xUnit / JUnit at
  test time) — a different concern from the runner's
  `systems.<ext>` / `tests-*.<ext>`. The TS / .NET / Java loaders
  parse those with native JSON readers and would need YAML libraries
  before the same flip could happen. Leave them alone.
- **Removing the phase-1 JSON fallback in gh-optivem.** That's the
  third PR in the rollout (after this one lands and bakes), tracked
  separately.

## Decisions (resolved 2026-05-11)

1. **Same camelCase keys in YAML.** Identical to gh-optivem's phase-2
   decision — `composeFile`, `containerName`, `testFilter`,
   `testFilterJoin` etc. stay camelCase in the migrated `.yaml`
   files. Lint friction (camelCase is non-idiomatic YAML) accepted
   for one release; the JSON fallback in gh-optivem stays alive until
   every academy clone catches up, then a separate rename plan can
   take the keys to snake_case.
2. **Add header comments while rewriting.** Each migrated `.yaml`
   file gets a short comment block at the top:
   - For `systems.yaml`: one-liner per `label:` explaining what the
     stub vs real (and `-isolated` where present) compose pairs are
     for.
   - For `tests-latest.yaml` / `tests-legacy.yaml`: one-liner per
     suite explaining what cycle / mode it covers, and a callout on
     `sampleTest` (which is otherwise easy to mistake for "this is
     the test that runs").
   This is the *only* reason we're migrating — capture the
   institutional knowledge that the JSON couldn't carry.
3. **Atomic flip per language, not per file.** The 12 config files
   pair with workflow runs that pass both flags in lockstep
   (`--system-config docker/<lang>/<arch>/systems.<ext>
   --test-config system-test/<lang>/tests-latest.<ext>`). Migrating
   one file in a pair without the other would leave a window where
   only one half is `.yaml` and the workflow still uses `.json`. A
   single commit per language (3 files per lang: 2 systems + 2
   tests for monolith + multitier) keeps each step bisectable and
   keeps CI green at every commit.
4. **Workflow rewrites use `.yaml` end-to-end** — even though the
   runner accepts `.json` post-flip, leaving stale `.json` references
   in YAML workflow files is just noise. After this plan there
   should be zero `systems.json` / `tests-*.json` references in
   shop.
5. **Tab-vs-space callout in header comments.** YAML 1.2 rejects tab
   indentation. Add a one-line "use spaces, not tabs" reminder to
   each migrated file's header comment. (Cheap insurance — the next
   person editing one of these files has been writing JSON for
   months.)
6. **"Norway problem" callout.** Same rationale as the gh-optivem
   plan: `yaml.v3` is 1.2 by default, but a header-comment reminder
   prevents anyone copying values like `containerName: NO` from
   another file and getting a boolean back.

## Files to change

### Config files (12) — rewrite JSON → YAML

| File | Note |
|---|---|
| `docker/dotnet/monolith/systems.json` | → `systems.yaml` |
| `docker/dotnet/multitier/systems.json` | → `systems.yaml` |
| `docker/java/monolith/systems.json` | → `systems.yaml` |
| `docker/java/multitier/systems.json` | → `systems.yaml` |
| `docker/typescript/monolith/systems.json` | → `systems.yaml` |
| `docker/typescript/multitier/systems.json` | → `systems.yaml` |
| `system-test/dotnet/tests-latest.json` | → `tests-latest.yaml` |
| `system-test/dotnet/tests-legacy.json` | → `tests-legacy.yaml` |
| `system-test/java/tests-latest.json` | → `tests-latest.yaml` |
| `system-test/java/tests-legacy.json` | → `tests-legacy.yaml` |
| `system-test/typescript/tests-latest.json` | → `tests-latest.yaml` |
| `system-test/typescript/tests-legacy.json` | → `tests-legacy.yaml` |

Each rewrite is mechanical (JSON → YAML with `gopkg.in/yaml.v3`'s
canonical form): same keys, same values, same nesting, plus the
header-comment block from Decision 2. After writing the `.yaml` file,
`git rm` the original `.json` (clean cutover — no dual residency,
per Decision 4).

### Workflows (12) — flip extensions in runner invocations

`.github/workflows/`:

- `monolith-typescript-acceptance-stage.yml`
- `monolith-typescript-acceptance-stage-legacy.yml`
- `monolith-java-acceptance-stage.yml`
- `monolith-java-acceptance-stage-legacy.yml`
- `monolith-dotnet-acceptance-stage.yml`
- `monolith-dotnet-acceptance-stage-legacy.yml`
- `multitier-typescript-acceptance-stage.yml`
- `multitier-typescript-acceptance-stage-legacy.yml`
- `multitier-java-acceptance-stage.yml`
- `multitier-java-acceptance-stage-legacy.yml`
- `multitier-dotnet-acceptance-stage.yml`
- `multitier-dotnet-acceptance-stage-legacy.yml`

Each carries ~10 `run: gh optivem test system ... --system-config
docker/<lang>/<arch>/systems.json --test-config
system-test/<lang>/tests-{latest,legacy}.json` lines (one per suite).
A sed-style sweep handles this safely: the substrings are unique to
runner invocations and don't appear in any other shape in these
files.

### Cross-cutting workflows (3)

- `.github/workflows/_meta-prerelease-pipeline.yml`
- `.github/workflows/_prerelease-pipeline.yml`
- `.github/workflows/cross-lang-system-verification.yml`

Same treatment — flip `.json` → `.yaml` in any `--system-config` /
`--test-config` paths and in any path-construction shell snippets.

### Shell scripts (1)

- `test-all.sh` — lines 52–53 build a path with `systems.json` and
  pass `tests_file` (caller passes `tests-latest.json` /
  `tests-legacy.json` on lines 73–74). Flip both call-site and
  builder.

### Configuration loader comments (3)

These loaders parse `test-config-{env}-{mode}.json`, **not**
`tests-latest.json` — but their narrative comments mention
`tests-latest.json` (and one even talks about "the YAML files"
already, in the Java loader). Update for consistency:

- `system-test/typescript/config/configuration-loader.ts:46` —
  `tests-latest.json` → `tests-latest.yaml`.
- `system-test/dotnet/SystemTests/TestInfrastructure/Configuration/SystemConfigurationLoader.cs:18` —
  same.
- `system-test/java/src/main/java/com/mycompany/myshop/systemtest/configuration/ConfigurationLoader.java:22` —
  same.

Do **not** touch the actual `JSON.parse` / `JsonSerializer` calls.
The `test-config-*.json` files are out of scope.

### Docs (4)

- `docs/operations/running-system-tests.md`
- `system-test/typescript/README.md`
- `system-test/dotnet/README.md`
- `system-test/java/README.md`
- `CLAUDE.md`
- `.claude/agents/test-comparator.md`

Search-and-replace `systems.json` → `systems.yaml`, `tests-latest.json`
→ `tests-latest.yaml`, `tests-legacy.json` → `tests-legacy.yaml` in
each. Skim each after the sweep to make sure no doc reads as
nonsense post-replacement (e.g., a sentence like "the JSON file
declares..." should become "the YAML file declares...").

### Plans (do not touch)

- `plans/20260427-130100-cross-language-verification-workflow.md`
- `plans/20260430-1837-workflow-comparator.md`
- `plans/20260430-160000-consolidate-stage-workflows.md`

Plans are historical artefacts. Leave them with `.json` references
intact — they describe what the world looked like at the time they
were written, not what's true now.

## Acceptance for this plan

- `grep -r "systems\.json\|tests-latest\.json\|tests-legacy\.json"
  shop/` returns matches **only** in `plans/*.md` (historical).
- Every `.yaml` file under `docker/*/*/` and `system-test/*/` is
  parsed cleanly by `gh optivem build|run|test|stop|clean system`
  pointed at it directly (sanity-check one per language locally
  before pushing).
- Every acceptance-stage workflow run on the next push succeeds —
  this is the critical signal, because each workflow now hands the
  runner `.yaml` paths and the runner has to dispatch them through
  the YAML codec.
- Cross-lang verification workflow run succeeds (it composes paths
  from multiple languages, so any half-migrated language would
  surface here).

## Risks

1. **CI run cost on cutover.** Every acceptance-stage workflow has
   to pass post-cutover or the rollout is broken. Mitigation: bundle
   the migration per language (Decision 3) and watch CI per push.
   Per-language scope keeps the blast radius to one flavor at a
   time.
2. **In-flight shop branches.** Anyone with a feature branch off
   shop's `main` will hit merge conflicts on the workflow files
   after the cutover. Coordinate the merge order: drain open shop
   PRs first, then run this plan.
3. **YAML serialization quirks.** The hand-rewrite is exact-for-exact,
   but `yaml.v3` will emit `containerName: my-shop-real` (no
   quotes) where the JSON had `"my-shop-real"`. A value like
   `containerName: yes` *would* round-trip differently — check
   every value that could parse as a YAML scalar quirk (booleans,
   nulls, numbers with leading zeros) and quote it explicitly.
   Practical rule: when in doubt, quote.
4. **Stale references in not-yet-touched repos.** Once shop migrates,
   any academy student repo still scaffolded from a pre-phase-2
   build of gh-optivem will have `.json` files. The dual-codec
   fallback in gh-optivem handles that — but the assumption breaks
   if someone copies a workflow snippet out of shop and pastes it
   into one of those repos. Mitigation: README callout in shop's
   `running-system-tests.md` that "shop and freshly scaffolded
   academy repos use `.yaml`; older student repos may still have
   `.json` until they re-scaffold."

## Rollout

1. **One PR in shop, one commit per language.** Three commits in
   order: `dotnet`, `java`, `typescript` (or whichever order matches
   recent CI health — start with the language with the most stable
   pipeline so a green run validates the migration mechanic before
   the riskier languages get touched). Each commit migrates that
   language's 4 config files + flips the matching 4 workflow files
   (`{monolith,multitier}-<lang>-acceptance-stage[-legacy].yml`).
   The cross-cutting workflows, `test-all.sh`, and docs go in a
   fourth commit at the end.
2. **CI watchpoint.** After each per-language commit, watch the
   matching acceptance-stage runs. A failure at this stage almost
   certainly means a YAML serialization quirk (Risk 3) or a missed
   reference in a workflow — both bisect cleanly to the just-pushed
   commit.
3. **Bake.** Let the migrated shop run through one full pipeline
   cycle (acceptance → QA → prod) before touching gh-optivem's
   phase-1 fallback removal. This is the proof that the dual-codec
   dispatch isn't load-bearing anywhere we missed.
4. **Future cleanup PR in gh-optivem (no date).** Once every
   academy repo is on YAML (shop + every active student repo),
   delete the `.json` branches from `LoadSystem` / `LoadTests` and
   the legacy-extension probes from `verify_paths.go`. Track via a
   TODO with a date in `internal/runner/config.go` and
   `internal/atdd/runtime/actions/verify_paths.go`.
