# 2026-11-05 10:49:26 UTC — OS-correct gh-optivem binary names (GOEXE)

## TL;DR

**Why:** every build command hard-codes `-o gh-optivem.exe`, whatever the OS. In the Linux devcontainer that writes a Linux binary named `gh-optivem.exe`. `gh` on Linux never runs that file; it runs the extensionless `gh-optivem`, which in the shared Windows/Linux checkout is currently a stray Windows build. So `gh optivem` is broken in the container, and rehearsals had to work around it with scratch builds and symlink swaps.
**End result:** every build writes `gh-optivem$(go env GOEXE)`: `gh-optivem.exe` on Windows, `gh-optivem` on Linux/macOS. Every caller refers to the binary the same way, so both binaries coexist in one checkout and `gh optivem --version` works on both OSes after `scripts/install.sh`.

## Outcomes

- `scripts/install.sh` on Linux produces a working `gh optivem`, with no scratch builds and no symlink swaps.
- The same scripts still produce and run `gh-optivem.exe` on Windows (Git Bash): no regression for the Windows workflow.
- A Windows build and a Linux build can sit side by side in the shared checkout without clobbering each other.
- `.gitignore` covers both names, so neither binary gets committed.
- `CONTRIBUTING.md` documents the OS-neutral build command.

## ▶ Next executable step (resume here)

Step 8 (human, Windows host): in Git Bash on Windows, run `bash scripts/install.sh`. Confirm that `gh optivem --version` prints `dev-<sha>`, that `gh-optivem.exe` gets rebuilt (new mtime), and that the Linux `gh-optivem` next to it is untouched. When that passes, delete this plan. Steps 1–7 are done and verified on Linux.

## Steps

- [ ] **Step 8: Windows check (human).** On the Windows host, run `scripts/install.sh` in Git Bash and confirm `gh optivem --version` works and `gh-optivem.exe` is rebuilt; the Linux `gh-optivem` next to it is untouched. This needs the user's Windows machine.

## Non-goals and invariants

- Don't change release packaging (`gh-optivem-<os>-<arch>[.exe]` assets via the precompile workflow); only local dev builds are in scope.
- Don't move the build output out of the repo root: `gh extension install .` requires the binary at the root, named after the repo.
- Out of scope: `scripts/atdd-rehearsal.sh` hard-wiring `../shop` (decision, user 2026-11-05: tracked separately; this plan touches only its binary name).
- Out of scope: giving the devcontainer its own clone instead of the shared Windows bind mount (the deeper fix; separate decision).
