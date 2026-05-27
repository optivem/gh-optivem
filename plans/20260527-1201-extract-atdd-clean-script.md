# Plan: Extract `system clean` out of `atdd-rehearsal.sh` into a separate script

## Context

`scripts/atdd-rehearsal.sh:245-251` currently runs `gh optivem system clean` against the consumer repo (default `../shop`) before creating the rehearsal worktree. The intent is to drop volumes + locally-built images from prior rehearsals so the next `implement` run starts from a clean docker state.

Coupling `clean` to the rehearsal entry point has two downsides:

1. **Every rehearsal pays the clean cost**, even when the operator knows the previous rehearsal already cleaned up (e.g. retrying immediately after a worktree-removal `yes`) or when they explicitly want to keep state from the prior run for debugging.
2. **There's no way to run clean on its own** without going through the rehearsal flow (which builds the binary, creates a worktree, etc.).

The fix: move the clean step into its own script (`scripts/atdd-clean.sh`) that the operator invokes when they want a clean docker state, and remove it from `atdd-rehearsal.sh`. The rehearsal becomes "worktree + implement", nothing more.

## Items

### Item 1 — Create `scripts/atdd-clean.sh`

**File:** `scripts/atdd-clean.sh` (new)

Standalone wrapper around `gh optivem system clean` against the consumer repo. Mirror the rehearsal script's surface area for the bits it shares:

- `REHEARSAL_REPO` and `REHEARSAL_DEFAULT_CONFIG` constants at the top (same defaults: `shop`, `gh-optivem-monolith-typescript.yaml`).
- `--config <yaml>` / `-c <yaml>` / `--config=<yaml>` flag handling (copy the loop from `atdd-rehearsal.sh:103-149`, but with no positional `<issue-num>` or `[label]`).
- Resolve `CONSUMER_ROOT` as a sibling of `gh-optivem` (same logic as `atdd-rehearsal.sh:161-166`).
- Build `gh-optivem.exe` from source (same `go build` block as `atdd-rehearsal.sh:220-243`) so clean exercises the local working tree's `system clean` implementation, not the installed `gh optivem`. This matters because `system clean`'s project-scoped behaviour reads `systems.yaml` via the same path resolution as the rest of the binary; running an outdated installed copy would silently scope to the wrong systems.
- Invoke `gh-optivem.exe system clean` against `CONSUMER_ROOT` with `GH_OPTIVEM_CONFIG="$CONSUMER_ROOT/$CONFIG"` (same as `atdd-rehearsal.sh:251`).
- Exit with the clean step's own RC (do not swallow failures — operator ran clean explicitly and wants to know if it failed).

Header doc-comment block (lines 11-56 in `atdd-rehearsal.sh`) gets trimmed to describe this script only: usage, what gets cleaned, what is preserved (registry-pulled images), project-scope caveat (per current config's `systems.yaml`).

`--help` / `-h` prints the header block (same `sed -n` trick as `atdd-rehearsal.sh:106`).

### Item 2 — Remove the clean call from `atdd-rehearsal.sh`

**File:** `scripts/atdd-rehearsal.sh`

Delete lines 245-251 (the `log "Cleaning local docker state…"` line and the `( cd … system clean )` invocation).

Also remove the now-stale step 2 in the workflow header comment (lines 31-37): the numbered list reduces from 6 steps to 5. Renumber the remaining steps accordingly.

Add a one-line note at the top of the header (just under the `Wraps gh optivem implement…` paragraph) pointing operators at the new clean script:

```
# Note: this script no longer cleans docker state. If you want a fresh
# state, run `bash scripts/atdd-clean.sh [--config <yaml>]` first.
```

### Item 3 — `chmod +x` the new script

Match the other `scripts/*.sh` files (they're all executable per `ls -l scripts/`). Either `chmod +x scripts/atdd-clean.sh` and commit the mode bit, or rely on the operator to invoke via `bash scripts/atdd-clean.sh`. Default to setting the executable bit for parity with siblings.

### Item 4 — Cross-reference from `CONTRIBUTING.md` / `README.md`

Grep for existing mentions of `atdd-rehearsal.sh` in `CONTRIBUTING.md` and `README.md`. Where the rehearsal flow is described, add a sentence noting that clean is now a separate script and pointing at it. Do not add a freestanding "atdd-clean" section if there isn't already one for rehearsal — just augment the existing prose.

If neither file mentions `atdd-rehearsal.sh` (the rehearsal is described as personal dev workflow in the script header itself), skip this item.

## Out of scope

- **Generalising `system clean` itself.** The binary command is unchanged; this plan only relocates the wrapper script.
- **Adding a `--no-clean` flag to `atdd-rehearsal.sh`** as an alternative to extraction. The whole point is to make clean explicit and on-demand, not gate it behind a flag in the rehearsal script.
- **Sharing helpers between the two scripts** (e.g. a common `_atdd-build.sh`). Premature DRY — both scripts are short and the duplication is the build block plus arg parsing. Revisit only if a third script joins.
- **CI changes.** Neither script runs in CI; both are local dev tools.

## Verification

- `bash scripts/atdd-clean.sh --config gh-optivem-monolith-typescript.yaml` builds the binary, runs `system clean` against `../shop`, exits 0 on a clean docker daemon.
- `bash scripts/atdd-rehearsal.sh 69 --config gh-optivem-monolith-typescript.yaml` no longer prints the "Cleaning local docker state…" line, and the rehearsal worktree is created without `system clean` running first.
- `bash scripts/atdd-clean.sh --help` prints the trimmed header block.
- `bash scripts/atdd-rehearsal.sh --help` no longer mentions step 2 (clean); the note at the top references `atdd-clean.sh`.
