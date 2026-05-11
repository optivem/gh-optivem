# Collapse override flags into `gh-optivem.yaml`

## Motivation

`gh optivem` currently carries several per-command flags whose only job is
to shadow a value that already (or could) live in `gh-optivem.yaml`:

- `--system-config` shadows `system_config:`
- `--test-config` shadows `test_config:`
- `--project` shadows `project.url`
- `--yaml` shadows the embedded process-flow YAML
- `--agent-prompt name=path` shadows the embedded agent prompts
- `--extra NODE=text` and `--replace NODE=text` shape one process-flow
  node's prompt, but the text is invariably project-stable advice
  (e.g. `AT_RED_DSL_WRITE="prefer record types"` — true on every run
  of that project, not a per-invocation decision)

Plus one flag with overlapping semantics against an env var and a sibling-dir
convention:

- `--repo-dir slug=path` overlaps `$GH_OPTIVEM_WORKSPACE` and the
  `<dirname(cwd)>/<repo-name>` sibling convention.

The result is a precedence story that takes a paragraph to explain per knob
and an over-stuffed `--help` page for what is, in practice, a small set of
decisions.

Unifying rule: **anything the project says lives in `gh-optivem.yaml`**;
**anything this invocation says is a flag**. Apply that rule consistently
and the surface collapses.

## Scope

### A. Delete (true duplicates of YAML fields)

| Flag                | YAML field      | Where today                                            |
| ------------------- | --------------- | ------------------------------------------------------ |
| `--system-config`   | `system_config` | `build/run/stop/clean/test system` in `runner_commands.go` |
| `--test-config`     | `test_config`   | `test system` in `runner_commands.go`                  |
| `--project`         | `project.url`   | `atdd implement-ticket`, `atdd manage-project`, `atdd debug pick-top-ready` |

For each: drop the flag, drop the 3-tier `resolveSystemPath`/`resolveTestsPath`
helpers, fall back to the 2-tier rule "YAML field or default."
Operators with multiple variants per project use `--config ./alt.yaml`.

### B. Repurpose (collapse three workspace strategies into one)

| Old                                                  | New                              |
| ---------------------------------------------------- | -------------------------------- |
| `--repo-dir slug=path` (repeatable, per-slug)        | `--workspace path` (single path) |
| `$GH_OPTIVEM_WORKSPACE`                              | *(removed)*                      |
| Sibling-dir convention: `parent(CWD)/<repo-name>`    | Default value of `--workspace`   |

Resolution becomes:

```
workspace        = --workspace || parent(CWD)
clone_path(slug) = <workspace>/<repo-name(slug)>
```

One formula, no env var, no per-slug map. Operators with non-uniform layouts
symlink their outlier clones into the workspace dir.

Mono-repo still works because the user runs from inside the clone:
`parent(CWD)/<repo-name>` == CWD when CWD is `<workspace>/<repo-name>`.
The "clone dir name must match repo-name(slug)" implicit constraint is now
explicit (it was already true under the sibling convention for multi-repo;
it's now true for mono-repo too).

### C. Move to YAML (project-stable overrides of embedded defaults)

| Flag                       | New YAML field        | Shape                                                                 |
| -------------------------- | --------------------- | --------------------------------------------------------------------- |
| `--yaml`                   | `process_flow:`       | scalar path, optional, falls back to embedded                         |
| `--agent-prompt name=path` | `agent_prompts:`      | map of name→path, optional, partial overrides allowed                 |
| `--extra NODE=text`        | `node_extras:`        | map of node-ID→literal text, appended to that node's prompt            |
| `--replace NODE=text`      | `node_replacements:`  | map of node-ID→path, replaces that node's prompt verbatim with file body |

The flags go away. Operators commit the override file(s) and point
`gh-optivem.yaml` at them; experimentation without committing routes through
`--config ./gh-optivem.experimental.yaml`.

Shape note: `node_extras` uses inline text (values are short — e.g.
`"prefer record types"`); `node_replacements` uses paths (values are
whole prompt bodies, identical lifetime/granularity to `agent_prompts`).
Mirrors the agent_prompts convention so contributors see one pattern,
not two.

### D. Keep (genuinely per-invocation)

After this cleanup, **zero override flags survive**. What remains:

- `--workspace` — workspace-root selector introduced in Step 4. Per-machine,
  flag-only by design.
- Persistent: `--config` / `-c` (and `$GH_OPTIVEM_CONFIG`) — selects which
  `gh-optivem.yaml` to read. By definition can't live inside one.
- Imperatives (tell *this run* what to do — no project-stable value to
  commit): `--rebuild`, `--restart`, `--suite`, `--test`, `--sample`,
  `--no-build`, `--no-start`, `--no-setup`, `--list`, `--log-lines`,
  `--up-timeout`, `--autonomous`, `--manual-agents`, `--log-file`,
  `--keep-runs`, `--show-prompt`, `--issue`, `--force`, `--dir`,
  `--message`, `--no-close`, `--dry-run`, `--root`.

## Implementation

### Step 5. Move `--yaml` to `process_flow:`

`atdd_commands.go`:

- Drop `--yaml` flag from `newAtddImplementTicketCmd`, `newAtddManageProjectCmd`,
  `newAtddDebugNextPhaseCmd`.
- Read `cfg.ProcessFlow` after loading the config; thread to
  `driver.Options.YAMLPath` and `statemachine.LoadFile` accordingly.
- For `atdd debug next-phase`: same — load the config, read the field.
  Document that the command operates on the configured process flow, not
  an arbitrary one.

The `driver.Options.YAMLPath` field stays as the internal carrier; the only
change is who fills it (config loader, not flag binding).

### Step 6. Move `--agent-prompt` to `agent_prompts:`

`atdd_commands.go`:

- Drop `--agent-prompt` `StringSliceVar` from `newAtddImplementTicketCmd`,
  `newAtddManageProjectCmd`.
- Drop `parseAgentPromptPairs` (or refactor to consume the map directly
  from `cfg.AgentPrompts`).
- Read `cfg.AgentPrompts`; for each entry, read the file at startup
  (so a missing path surfaces at config-load, not deep inside the pipeline);
  thread into `driver.Options.AgentPromptOverrides`.

`driver.Options.AgentPromptOverrides` stays as the internal carrier.

### Step 7. Move `--extra` / `--replace` to YAML and delete `--interactive`

`atdd_commands.go`:

- Drop `--extra`, `--replace`, and `--interactive` flags from
  `newAtddImplementTicketCmd`, `newAtddManageProjectCmd`.
- Drop `parseNodeKVPairs` entirely. Rename `buildOverrideHooks` to
  something cfg-flavoured (e.g. `overrideHooksFromConfig`) since the
  flag-parsing layer is gone. New job: build `*override.Hooks` purely
  from `cfg.NodeExtras` and `cfg.NodeReplacements`.
- New body:
  - Read `cfg.NodeExtras` directly into `Hooks.Extra` (literal text map).
  - Read `cfg.NodeReplacements`; for each entry, read the file at startup;
    populate `Hooks.Replace` with the file body. Missing path surfaces at
    config-load.
  - Return nil when both maps are empty (driver's `wrapOverride` already
    handles nil hooks as a no-op).

`internal/atdd/runtime/override/middleware.go`:

- Drop `Interactive bool` from `Hooks`.
- Drop `KeyInteractive` constant and all `ctx.Set(KeyInteractive, …)` calls.
- Update the doc comments accordingly. Delete the stale "Empty in v1;
  populated in v2 by the --extra / --replace / --interactive flags"
  paragraph — the wiring is live (tested in driver_test.go:260,
  clauderun_test.go:795) and the source is now config, not flags.

`internal/atdd/runtime/driver/driver.go`:

- Delete `promptForInteractiveExtra` (currently lines 859-879).
- Delete the `interactive, _ := ctx.Get(override.KeyInteractive).(bool)`
  read and the `if interactive { … }` branch in `agentDispatcher`
  (currently lines 730, 773-…).
- Remove the now-unused `bufio` / `io` imports if nothing else uses them.
- Update the comment in `wrapOverride` (currently line 884) to drop the
  `--interactive` reference.

`override.Hooks` shape change: shrinks to `{Extra, Replace}` — same
in-memory carrier minus the dead Interactive field.

### Step 8. Delete `--repo` on `atdd debug classify`

`atdd_commands.go`:

- Drop the `repo` var and `cmd.Flags().StringVar(&repo, "repo", …)` line
  in `newAtddDebugClassifyCmd` (currently line 435).
- The `classify.Classify` call becomes `classify.Classify(context.Background(), issue, classify.Options{})`.
  `classify.Options.Repo` stays as a field on the package API (still used
  by `classify_test.go:243`'s argv assertion); the debug command just no
  longer fills it.

Behaviour change: with no `--repo`, classify falls back to gh's CWD
git-remote inference. Operators debugging classify must `cd` into a
clone of the target repo first. The cross-repo shortcut goes away.

### Step 9. Tests

- `internal/projectconfig/config_test.go` — schema additions, validation
  of new fields (including the "same key in both node_extras and
  node_replacements is rejected" rule and the deferred node-ID check).
- `runner_commands_test.go` — flag deletions.
- `atdd_commands_test.go` (if it exists; otherwise the integration tests
  that exercise the atdd commands) — flag deletions for `--system-config`,
  `--test-config`, `--project`, `--yaml`, `--agent-prompt`, `--extra`,
  `--replace`; new cases driving `Hooks.Extra` / `Hooks.Replace` from
  `cfg.NodeExtras` / `cfg.NodeReplacements`.
- `internal/atdd/runtime/driver/driver_test.go` — keep the
  `--replace`-swap assertion (line 260) but rename / re-wire it as
  "node_replacements swap"; the behaviour under test is unchanged, only
  the fill site differs.
- `internal/atdd/runtime/clauderun/clauderun_test.go` — same rename for
  the equivalent assertion at line 795.
- `repolocator_test.go` — new formula.
- `classify_test.go` — `TestClassify_GhArgs` (line 243+) keeps the
  "--repo passthrough when Options.Repo is set" assertion since the
  package field stays; just the debug command stops filling it.

Per CLAUDE.md / memory: never run `go test ./...` unbounded on Windows.
Use `scripts/test.sh` or `-p 2`, or scope to one package at a time during
development.

### Step 10. Docs

- `README.md` — strip references to deleted flags; describe the new
  `gh-optivem.yaml` fields and `--workspace` flag.
- Search for `--system-config` / `--test-config` / `--project` /
  `--repo-dir` / `--yaml` / `--agent-prompt` / `--extra` / `--replace` /
  `--interactive` / `--repo` mentions in any markdown, scripts
  (`scripts/manual-test-runner-shop.sh`), and templates.
- `CONTRIBUTING.md` lines 127-131, 130 (`--interactive`), 183, 202-205 —
  these document the flag-aware rehearsal path and the
  `--extra AT_RED_DSL_WRITE=...` example. Rewrite to point at
  `node_extras:` / `node_replacements:` in `gh-optivem.yaml`. Drop the
  `--interactive` bullet entirely.
- `process-flow.yaml` / `process-diagram.md` — unaffected (those are
  the *embedded* defaults the new fields override).

## Order of operations

1. Schema additions (Step 1) — non-breaking, lands first.
2. Flag deletions (Steps 2, 3, 5, 6, 7, 8) and `--repo-dir`→`--workspace`
   swap (Step 4) — breaking; can land in one commit or split per flag.
3. Tests (Step 9).
4. Docs (Step 10).
5. Bump VERSION minor (breaking CLI surface change — semver minor since
   pre-1.0 conventions don't apply here; this is a v1.x removal).

## Resolved decisions

- **`workspace:` not in YAML.** Per-operator/per-machine, stays flag-only.
  Operators wanting persistence alias the command.
- **No migration messages.** Just delete the flags. Cobra's "unknown flag"
  is sufficient. Academy-student audience has no built-up muscle memory.
- **`atdd debug next-phase --yaml` goes away.** Same as the public
  commands: read `process_flow` from `gh-optivem.yaml`. To poke alternate
  flows, use `--config ./alt.yaml`.
- **`--repo` on `atdd debug classify` deleted.** Operators `cd` into the
  target clone first; gh's CWD git-remote inference handles the normal
  debug case. Cross-repo shortcut goes away.
- **`--interactive` deleted.** Zero behavioural tests, one-line docs,
  overlaps with `--show-prompt` / `--manual-agents` / `node_extras:`.
  Untested feature with marginal coverage; revisit if a concrete need
  surfaces.
