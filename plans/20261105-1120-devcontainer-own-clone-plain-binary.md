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

Start with Step 1: restore Windows. In Git Bash on Windows, in the gh-optivem checkout, run `cp gh-optivem.exe gh-optivem` and then `gh optivem --version`. Gate: the version prints.

## Steps

- [ ] **Step 1: Restore Windows now (human, Windows).** In Git Bash, in `/c/Users/valen/Documents/GitHub/optivem/academy/gh-optivem`, run `cp gh-optivem.exe gh-optivem`. This temporarily breaks `gh optivem` in the current (bind-mounted) container until Step 5. Done when: `gh optivem --version` prints on Windows.
- [ ] **Step 2: Plain binary name everywhere.** Build only `gh-optivem`, on Windows too: no `gh-optivem.exe`, since nothing runs it. `.gitignore` keeps `*.exe` so leftovers stay ignored. Revert the GOEXE naming from `052f3cbb` and build and call plain `gh-optivem` in `scripts/install.sh`, `scripts/atdd-clean.sh`, `scripts/atdd-rehearsal.sh`, `scripts/manual-test-runner-shop.sh`, `scripts/manual-test.sh` and `scripts/atdd-rehearsal-cleanup.sh` (comment). Add a one-line comment in `install.sh` explaining why the name has no `.exe`: `gh` runs `<repo>/gh-optivem` on every OS. Done when: `grep -rn 'GOEXE\|gh-optivem\.exe' scripts/` returns nothing and `bash -n` passes on each edited script.
- [ ] **Step 3: Docs for the binary name.** In `CONTRIBUTING.md`, replace the `BIN="gh-optivem$(go env GOEXE)"` snippets and `install.sh` mentions with plain `gh-optivem` (e.g. `go -C ../gh-optivem build -o gh-optivem .` and `../../academy/gh-optivem/gh-optivem implement ...`). Done when: `grep -n 'GOEXE\|gh-optivem\.exe' CONTRIBUTING.md` returns nothing. Can run in parallel with Step 2.
- [ ] **Step 4: Devcontainer on its own volume.** In `.devcontainer/devcontainer.json`, set `workspaceMount` to a named volume (`source=gh-optivem-workspace,target=/workspaces,type=volume`) and `workspaceFolder` to `/workspaces/gh-optivem`. VS Code still reads `devcontainer.json` from the Windows folder; the container just doesn't mount it. Because `postCreateCommand` runs inside the empty workspace folder, use `initializeCommand`/`onCreateCommand` or a path-independent `post-create.sh` invocation so that the first create clones `optivem/gh-optivem` into `/workspaces/gh-optivem` if it's missing (with `gh repo clone`, using the existing `gh-optivem-gh-config` volume for auth, with a clear message if gh isn't logged in yet), then runs the existing post-create steps. Remove the `powershell` feature (unused on Linux; the container stays bash-only). Keep the gh, Claude and credential-helper mounts and hooks unchanged. Done when: `devcontainer.json` is valid JSON and `bash -n` passes on the edited scripts.
- [ ] **Step 5: Verify the container (human starts the rebuild).** Rebuild the container (VS Code: "Dev Containers: Rebuild Container"). Prerequisite: everything from the current bind-mounted container is committed and pushed, because the new container starts clean from GitHub. Inside it, check that `pwd` = `/workspaces/gh-optivem`, that `/workspaces/gh-optivem` is not a 9p mount (`grep ' /workspaces' /proc/self/mountinfo` doesn't show `9p`), and run `bash scripts/install.sh`. Done when: `gh optivem --version` prints `dev-<sha>`, the first 4 bytes of `gh-optivem` are `\x7fELF`, `git status` shows no `safe.directory` warning, and `bash -n scripts/test-system.sh` passes.
- [ ] **Step 6: Verify Windows (human, Windows).** In Git Bash on Windows, `git pull` the gh-optivem checkout, then run `bash scripts/install.sh` and `gh optivem --version`. Done when: the version prints and the build log says `go build -o gh-optivem`.
- [ ] **Step 7: Document the two-clone workflow.** In `.devcontainer/README.md`, plus a short pointer in `CONTRIBUTING.md`: the container has its own clone in the `gh-optivem-workspace` volume; changes move between Windows and the container only via commit, push and pull; how to reset the volume (delete the volume, rebuild). Done when: both files describe it and no doc still tells you to edit the Windows checkout from inside the container.
- [ ] **Step 8: Retire the old plan.** Delete `plans/20261105-1049-os-correct-binary-names.md`, since its remaining Step 8 is superseded. Done when: the file is gone.

## Non-goals and invariants

- Don't change release packaging: the precompile workflow's `gh-optivem-<os>-<arch>[.exe]` assets are untouched.
- The binary stays at the repo root, named `gh-optivem`, because `gh extension install .` requires that.
- The Windows clone stays where it is; this plan doesn't move or re-clone it.
- Keep the merge-hooks-only-on-Windows fix (`6883e285`).
- Out of scope: `scripts/atdd-rehearsal.sh` hard-wiring `../shop` (tracked separately).
