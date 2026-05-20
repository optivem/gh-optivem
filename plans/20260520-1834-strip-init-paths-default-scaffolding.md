# Strip `gh optivem init`'s paths-default scaffolding

**Cross-references**: extends the "no defaults anywhere" doctrine landed in
the validation work (Rule 22a in `internal/projectconfig/config.go::Validate`
+ removal of migrate's paths back-fill from `config_commands.go`). That
change made `paths:` operator-owned at validate-time and at migrate-time;
this plan is the last hop — `gh optivem init`'s scaffolder also seeds
`paths:` from `projectconfig.DefaultPaths`, which is the remaining place
the binary writes default-shaped values the operator never asked for.

## Context

`internal/steps/optivem_yaml.go:195` (inside `BuildOptivemYAML`):

```go
pc.Paths = projectconfig.DefaultPaths(cfg.TestLang, cfg.SystemTestPath, sutNamespace)
```

When the operator runs `gh optivem init`, this line fills the generated
`gh-optivem.yaml` with the eight canonical Family B path values from
`DefaultPaths` (e.g. `Testkit.Driver.Adapter/<sut_namespace>` for dotnet).
Those values match the **shop template's** layout but do not necessarily
match the operator's actual scaffold tree — and once Rule 22a is in
place, they are also the only `paths:` block any new project ever sees
unless the operator hand-edits afterwards.

This is the same "no defaults" tension we resolved for `migrate` — the
question is whether `init` deserves the same treatment.

## Tension

| Argument                                       | Direction        |
|------------------------------------------------|------------------|
| "No defaults anywhere" — doctrine consistency. | Strip            |
| Fresh `init` must produce a working config.    | Keep             |
| Scaffolder ALSO creates the directory tree it points at (shop template), so DefaultPaths matches the freshly-scaffolded layout BY CONSTRUCTION. | Keep (semantically not "defaults") |
| If the operator later renames `Testkit.Driver.Adapter` → `Driver.Adapter` (the actual shop did this), the seeded paths drift silently. Today's behaviour. | Strip |
| Operator running `init` has no context to fill `paths:` by hand (just picked Java/monolith). | Keep, or replace with prompts |

The earlier migrate-strip decision was clean because migrate writes
into an **existing** operator-curated YAML — defaults are clearly an
imposition. `init` is different: the binary owns the entire first
draft (template + YAML), so "default" vs "scaffold-owned authoritative
value" is a definitional question, not just a workflow one.

## Options

### Option A — Strip + hand-edit workflow

Remove the line at `optivem_yaml.go:195`. `init` emits the YAML
without `paths:`. The first `gh optivem` command after init fails
Rule 22a with the canonical-key list; operator hand-edits
`gh-optivem.yaml` and re-runs.

- **Pro**: cleanest possible doctrine — binary writes zero defaults.
- **Con**: every fresh project gets a one-time friction wall between
  `init` and first useful command. Workflow doesn't match what most
  scaffolders do.
- **Con**: the eight values are deterministic given (test_lang,
  system_test_path, sut_namespace) — making the operator type them
  out from scratch when the binary already knows them is friction
  for friction's sake.

### Option B — Strip + interactive prompts

Strip the line; add a `paths:` prompting step to `init` that asks
the operator to confirm/edit each of the eight canonical keys, with
the DefaultPaths value as the displayed default. Same prompt
machinery the existing `--repo-strategy` / `--arch` flags use.

- **Pro**: matches "explicit configuration" without sacrificing the
  init UX.
- **Pro**: surfaces the path doctrine to the operator at the moment
  they could most usefully push back on it (e.g. "actually I want
  `Driver.Adapter` not `Testkit.Driver.Adapter`").
- **Con**: eight prompts is a lot. Could collapse to one "confirm
  layout" yes/no + an editor-style "edit if you say no" flow.
- **Con**: the prompts have to be wired to non-interactive
  invocations too (CI, `--yes`) — likely just "accept the displayed
  defaults silently when stdin isn't a TTY", which is functionally
  Option C.

### Option C — Keep, semantically reframe (no code change)

Leave `optivem_yaml.go:195` as-is. Update the docstring + the
project-config doctrine doc to call out the asymmetry explicitly:
"the scaffolder seeds paths from the shop template's layout; this
is not a runtime default but the authoritative initial value
matching the directory tree just scaffolded. Subsequent edits are
operator-owned." Update CLAUDE.md or the path-keys.md so future
agents don't try to "fix" the apparent inconsistency.

- **Pro**: no behaviour change, no workflow regression.
- **Pro**: the semantic distinction is real — the scaffold writes
  both the YAML AND the directory tree; the paths reflect the tree.
- **Con**: leaves one place in the binary still writing
  `DefaultPaths` values, which (today's shop case) drift the moment
  the operator renames a scaffolded directory.

### Option D — Strip + `--paths-from-template` flag

`init` emits no `paths:` block by default. A `--paths-from-template`
(or `--include-default-paths`) flag opts the operator into the
DefaultPaths values explicitly. The flag is documented as "seed
paths matching the shop template's layout; subsequent edits are on
you."

- **Pro**: defaults are opt-in, not opt-out. The flag's presence
  in the operator's shell history is the audit trail.
- **Pro**: keeps the cheap-UX path available for operators who
  know they want the template layout.
- **Con**: extra flag surface for what is, frequently, "yes, give
  me the obvious thing".

## Files touched (Option A or D — the strip branches)

- `internal/steps/optivem_yaml.go` — remove (or guard) line 195. Drop the
  comment block at 185–190 explaining the SSoT join — no longer accurate.
- `internal/steps/optivem_yaml_test.go` — every test that checks `got.Paths`
  shape needs updating:
  - `TestBuildOptivemYAML_NonScaffoldPaths` (line 152) — currently asserts
    `got.Paths[tc.wantKey] != tc.wantPath`; needs to assert `got.Paths == nil`.
  - The `Paths` map check at line 236 (`if len(got.Paths) == 0`) becomes a
    "Paths must be nil/empty" assertion.
- `internal/configinit/configinit_test.go` — currently has no Paths checks,
  but any "fresh init validates" assertion will start failing once Rule 22a
  fires on the init output. Tests that load the emitted YAML need to either
  (a) accept the validation error explicitly or (b) post-process to inject
  paths before loading.
- `internal/projectconfig/paths_defaults.go` — `DefaultPaths` becomes
  unreferenced by production code (only tests use it). Decide whether to
  delete it, move it to `paths_defaults_test.go`'s package, or keep it as
  a documentation artifact for the canonical key vocabulary.
- `internal/projectconfig/path-keys.md` — update the doctrine note: paths
  are operator-owned at every layer (scaffold, migrate, runtime).
- `CLAUDE.md` (this repo) — consider adding a doctrine line so future
  agents don't reintroduce a default-paths writer somewhere.

## Knock-on (any option)

- The shop repo's 12 `gh-optivem-*.yaml` configs currently have no
  `paths:` block. Independent of which option lands here, those configs
  still need `paths:` added before `gh optivem implement` works against
  the shop — Rule 22a will catch them at the next `gh optivem` invocation.
  That's a separate one-shot data fix, not part of this plan.
- The Java testkit middle-segment package convention (sutNamespace inside
  `src/test/java/<ns>/latest/acceptance` rather than appended) is encoded
  in `DefaultPaths`. If `DefaultPaths` is deleted as part of this strip,
  that convention needs to live somewhere — likely in `path-keys.md` as
  documentation, since it's no longer enforced by any code.

## Open questions

1. **Which option?** Recommend Option C (keep + semantically reframe) on
   the grounds that the scaffolder genuinely owns both the YAML AND the
   directory tree, so the values are not "defaults the operator
   didn't ask for" but "authoritative initial values matching what was
   just scaffolded". Open to A/B/D if the doctrine matters more than the
   UX.
2. If Option A/D: should `gh optivem init` print a "next step: edit
   gh-optivem.yaml to add the paths: block before running implement"
   banner so the Rule 22a failure isn't a surprise?
3. If Option B: how many prompts? Single confirm-or-edit flow vs eight
   individual prompts vs grouped (testkit / tests) prompts.
4. Should `DefaultPaths` itself survive in production code if no caller
   uses it? Tests still need it as a fixture builder — could be
   relocated to a `testpaths` test helper package.
