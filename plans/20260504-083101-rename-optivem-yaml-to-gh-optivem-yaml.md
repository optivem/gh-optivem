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
name works; the new name is only better.

## Approach

Hard rename. There are no consumer repos in the wild yet — the shop
template will be regenerated, and no third-party scaffolds exist —
so a dual-read shim with deprecation warning would be dead weight.
Flip the constant, write the new name, sweep references in one pass.

### 1. Code: flip the constant

`projectconfig.Path` becomes `gh-optivem.yaml`. `Load` still does the
single canonical lookup; no fallback. Any consumer repo with an old
`optivem.yaml` on disk will surface `ErrNoProjectURL` until it's
renamed by hand — acceptable because there are no such repos right
now.

### 2. Scaffolder: write the new name

`steps.WriteOptivemYAML` writes `gh-optivem.yaml`. Optionally rename
the Go function to `WriteGhOptivemYAML` for symmetry; cosmetic.

### 3. Docs + comments + tests

Sweep every doc reference, code comment, error message, CLI flag help
text, and test fixture. The string `optivem.yaml` should not appear
anywhere after the sweep.

## Affected surfaces (initial inventory)

Counted via `grep optivem.yaml` on 2026-05-04 — recheck before starting:

**gh-optivem (11 files):**
- `main.go` — scaffold step name
- `internal/steps/optivem_yaml.go` — function name + doc + write logic
- `internal/projectconfig/config.go` — `Path` constant + doc
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

## Out of scope

- Renaming `gh-optivem` itself. The binary name is fine; only the file
  is being renamed.
- Touching `system.json` / `tests.json` (different files, different
  audience — runner subcommands).

## Order of operations

1. Flip `projectconfig.Path` to `gh-optivem.yaml`.
2. Update `steps.WriteOptivemYAML` to write the new name.
3. Sweep every other surface in the inventory above. Run `go test ./...`
   after each batch.
4. Sweep shop docs.
5. Manual rehearsal (`bash scripts/atdd-rehearsal.sh <issue>`) against
   a freshly regenerated shop scaffold to confirm the canonical path
   works end-to-end.

## Risk

Low. No live consumers means no migration concern. The only cost is
the fan-out of the mechanical sweep itself. Worst case: a missed
reference somewhere prints the literal string `optivem.yaml` in a log
line; cosmetic, fixable in the next pass.
