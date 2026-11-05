# 2026-11-05 11:20:03 UTC — Devcontainer gets its own gh-optivem clone; build plain `gh-optivem` everywhere

## TL;DR

**Why:** the devcontainer bind-mounts the Windows checkout (`C:\Users\valen\Documents\GitHub\optivem\academy\gh-optivem` → `/workspaces/gh-optivem`, over 9p), so Windows and Linux share one working tree. `gh` runs a locally installed extension as `<repo>/gh-optivem` on every OS; on Windows it runs it through Git Bash's `sh` and never uses `gh-optivem.exe`. A shared tree therefore can't hold a working binary for both OSes. The previous plan's GOEXE naming (commit `052f3cbb`) broke Windows: `cannot execute binary file: Exec format error`. The shared mount also causes Windows line endings (CRLF) in scripts (`scripts/test-system.sh` fails `bash -n`), root-owned `777` files, `safe.directory` warnings and slow I/O.
**End result:** the container works in its own clone of gh-optivem, kept in a Docker volume and synced with Windows only through GitHub. Every OS builds the plain extensionless `gh-optivem`, the file `gh` actually runs, so `gh optivem` works on Windows and in the container after `scripts/install.sh`, and neither side can break the other.

## Outcomes

- Opening the devcontainer gives a Linux-native clone of gh-optivem at `/workspaces/gh-optivem` (same path as today), in a named volume mounted at `/workspaces`. It's cloned automatically on first create. Only gh-optivem is in the container, as today; a sibling (e.g. `shop`) can be cloned into `/workspaces/` later without a rebuild.
- No bind mount of Windows source into the container, so no CRLF, ownership or `safe.directory` problems, and native file-system speed.
- `scripts/install.sh` builds `gh-optivem` (no extension) on every OS, and `gh optivem --version` works on Windows (Git Bash) and in the container.
- Scripts and `CONTRIBUTING.md` refer to the binary by one plain name; the GOEXE indirection from `052f3cbb` is gone.
- `CONTRIBUTING.md` and `.devcontainer/README.md` document the two-clone workflow: Windows clone and container clone, synced through GitHub (commit and push on one side, pull on the other).
- The old plan `plans/20261105-1049-os-correct-binary-names.md` is deleted; its Step 8 is superseded by this plan.

## ▶ Next executable step (resume here)

Only Step 5 remains (Windows verified 2026-11-05: `install.sh` built plain `gh-optivem`, `gh optivem --version` printed `dev-81fc9b7f`). Rebuild the container (VS Code: "Dev Containers: Rebuild Container"); the devcontainer now uses `.devcontainer/Dockerfile` + `bootstrap.sh` (`gh-optivem-bootstrap`) to clone into the `gh-optivem-workspace` volume. Then run the Step 5 checks; when they pass, delete this plan.

## Steps

- [ ] **Step 5: Verify the container (human starts the rebuild).** Rebuild the container (VS Code: "Dev Containers: Rebuild Container"). Prerequisite: everything from the current bind-mounted container is committed and pushed, because the new container starts clean from GitHub. Inside it, check that `pwd` = `/workspaces/gh-optivem`, that `/workspaces/gh-optivem` is not a 9p mount (`grep ' /workspaces' /proc/self/mountinfo` doesn't show `9p`), and run `bash scripts/install.sh`. Done when: `gh optivem --version` prints `dev-<sha>`, the first 4 bytes of `gh-optivem` are `\x7fELF`, `git status` shows no `safe.directory` warning, and `bash -n scripts/test-system.sh` passes.

## Non-goals and invariants

- Don't change release packaging: the precompile workflow's `gh-optivem-<os>-<arch>[.exe]` assets are untouched.
- The binary stays at the repo root, named `gh-optivem`, because `gh extension install .` requires that.
- The Windows clone stays where it is; this plan doesn't move or re-clone it.
- Keep the merge-hooks-only-on-Windows fix (`6883e285`).
- Out of scope: `scripts/atdd-rehearsal.sh` hard-wiring `../shop` (tracked separately).
