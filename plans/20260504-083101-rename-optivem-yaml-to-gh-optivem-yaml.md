# Rename `optivem.yaml` → `gh-optivem.yaml`

## Motivation

The per-project config file the ATDD pipeline reads is currently named
`optivem.yaml`. After consolidating `.system/config.json` into it
(2026-05-04), this is the **only** config the gh-optivem binary
consumes. The current name is slightly ambiguous — "optivem" is a
brand/org, not a tool — and asymmetric with the binary's actual name
(`gh-optivem`).

Conventional naming for a single-tool-owned config file is to match the
binary: `Cargo.toml` for cargo, `Dockerfile` for docker, `tsconfig.json`
for tsc, `.eslintrc` for eslint. Renaming to `gh-optivem.yaml` makes
ownership unambiguous, future-proofs against a hypothetical second
optivem tool with its own config schema, and aligns file ⇄ binary
naming.

This is a **convenience rename**, not a correctness fix. The current
name works; the new name is only better. The cost is a non-trivial
transition across already-scaffolded consumer repos. Defer until a
quiet window.

## Approach

Two layers, applied together so the rename never breaks an existing
scaffolded repo:

### 1. Code: dual-read with deprecation

`projectconfig.Load(repoPath)` looks for `gh-optivem.yaml` first; on
miss, falls back to `optivem.yaml` and prints a one-line deprecation
warning to stderr ("optivem.yaml is deprecated; rename to
gh-optivem.yaml"). After one soak window, drop the fallback.

The default `Path` constant flips to `gh-optivem.yaml`. Tests assert
both branches: canonical-name path returns config without warning;
legacy-name path returns config plus deprecation warning.

### 2. Scaffolder: write the new name

`steps.WriteOptivemYAML` (rename to `WriteGhOptivemYAML`) writes
`gh-optivem.yaml`. Newly scaffolded repos start clean; existing
scaffolds keep working under the dual-read shim until they regenerate
or commit a manual rename.

### 3. Docs + comments

Update every doc reference, code comment, error message, and CLI flag
help text. Most surfaces use the literal string `optivem.yaml`, so a
careful global rename is feasible — but every change must be reviewed
to catch the few sites that should keep saying `optivem.yaml` (e.g.
the deprecation warning itself, the dual-read fallback, the changelog
entry for the rename).

### 4. Migration command (optional)

A small `gh optivem atdd migrate-config` subcommand that detects
`optivem.yaml` and renames it to `gh-optivem.yaml` in-place (with
`git mv` if the consumer is in a git tree). Lets operators clean up
without hand-editing. Worth doing only if there are many consumer repos
to migrate.

## Affected surfaces (initial inventory)

Counted via `grep optivem.yaml` on 2026-05-04 — recheck before starting:

**gh-optivem (11 files):**
- `main.go` — scaffold step name
- `internal/steps/optivem_yaml.go` — function name + doc + write logic
- `internal/projectconfig/config.go` — `Path` constant + Load logic + doc
- `internal/atdd/runtime/board/board.go` — `ErrNoProjectURL` message
- `internal/atdd/runtime/board/board_test.go` — test fixtures
- `internal/atdd/runtime/driver/driver.go` — config-source label + Options doc
- `internal/atdd/runtime/clauderun/clauderun.go` — references in comments / messages
- `internal/atdd/runtime/clauderun/clauderun_test.go` — fixtures
- `internal/config/config.go` — doc comment on `ProjectURL`
- `atdd_commands.go` — `--config` flag help text + examples
- `docs/how-it-works.md` — scaffold-step row

**shop (2 files):**
- `docs/atdd/process/task-and-chore-cycles.md`
- `docs/atdd/process/cycles.md`

**Existing scaffolded consumer repos (out-of-tree):** every shop scaffold
in the wild has `optivem.yaml` on disk. The dual-read shim keeps them
working until they regenerate or `git mv` to the new name.

## Out of scope

- Renaming `gh-optivem` itself. The binary name is fine; only the file
  is being renamed.
- Renaming `WriteOptivemYAML` Go function (cosmetic; do or don't, no
  external impact).
- Touching `system.json` / `tests.json` (different files, different
  audience — runner subcommands).

## Order of operations

1. Land `projectconfig` dual-read shim with tests for both branches.
2. Flip `projectconfig.Path` to `gh-optivem.yaml`.
3. Update `steps.WriteOptivemYAML` to write the new name.
4. Sweep every other surface in the inventory above. Run `go test ./...`
   after each batch.
5. Sweep shop docs.
6. Manual rehearsal (`bash scripts/atdd-rehearsal.sh <issue>`) against a
   shop repo that still has `optivem.yaml` to confirm the deprecation
   warning fires and the run still succeeds.
7. Manual rehearsal against a shop repo that has been renamed to
   `gh-optivem.yaml` to confirm the canonical path is silent.
8. Optional: ship `migrate-config` subcommand and use it on the academy
   workspace shop scaffolds.
9. After one soak window (≥ a couple of weeks of regular use), drop the
   `optivem.yaml` fallback in `projectconfig.Load`.

## Risk

Low. The dual-read shim makes the rename non-breaking for in-flight
consumer repos. The only cost is the fan-out of the sweep itself —
which is mechanical. Worst case: a missed reference somewhere prints
the literal string `optivem.yaml` in a log line; cosmetic, fixable in
the next pass.
