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

Step 1: inventory every hard-coded binary name. Run `grep -rn 'gh-optivem\.exe' scripts/ docs/ .github/ internal/ CONTRIBUTING.md README.md .devcontainer/ main.go`, excluding `plans/`. Classify each hit as a **build** (`go build -o ...`), a **call** (runs the binary by path) or **prose**. Record the list under Step 2/3 in this plan. Gate: the list is in the plan; no code changes yet.

## Steps

- [ ] **Step 1: Inventory.** Grep as in the ▶ block; record each hit as build, call or prose. Known so far: builds at `scripts/install.sh:44-45`, `scripts/atdd-clean.sh:108`, `scripts/manual-test-runner-shop.sh:38`, `CONTRIBUTING.md:487`, `CONTRIBUTING.md:510`; call in `scripts/atdd-rehearsal.sh` (runs `gh-optivem.exe`). Done when: the full list is in this plan.
- [ ] **Step 2: Builds use GOEXE.** Change each build to `go build ... -o "gh-optivem$(go env GOEXE)" .`, keeping existing `-ldflags`. For scripts that also call the binary, compute the name once at the top (e.g. `BIN="gh-optivem$(go env GOEXE)"`) and use `$BIN` for both the build and the calls. Depends on Step 1. Done when: `grep -rn 'build .*-o [^ ]*gh-optivem\.exe' scripts/` returns nothing and `bash -n` passes on every edited script.
- [ ] **Step 3: Callers use the same name.** Update every call site from Step 1 (starting with `scripts/atdd-rehearsal.sh`) to use the GOEXE-derived name. Depends on Step 1 (can run in parallel with Step 2 when the files don't overlap). Done when: no script outside `plans/` invokes a literal `gh-optivem.exe`.
- [ ] **Step 4: Docs.** Update `CONTRIBUTING.md:487` and `:510` (and any prose hits from Step 1) to the OS-neutral command, with one line explaining GOEXE (`.exe` on Windows, empty elsewhere). Done when: `CONTRIBUTING.md` has no hard-coded `-o gh-optivem.exe`.
- [ ] **Step 5: `.gitignore`.** Confirm `/gh-optivem` and `gh-optivem.exe` (or `*.exe`) are both ignored; add whichever is missing. Done when: `git check-ignore gh-optivem gh-optivem.exe` prints both.
- [ ] **Step 6: Remove the stray Windows binary.** Delete `/workspaces/gh-optivem/gh-optivem` (an extensionless Windows PE build, root-owned, gitignored, written by no script). It's root-owned, so this may need `sudo`; if that's not possible, record it for the user. Done when: the file is gone, or replaced by a Linux ELF from Step 7. Its producer is unknown (built 2026-11-05 10:17, same size as `gh-optivem.exe`). Decision (user, 2026-11-05): delete it in this plan and don't investigate; only if a Windows PE `gh-optivem` reappears after this plan, find what builds it (e.g. a VS Code task or launch config) and switch it to `.exe`.
- [ ] **Step 7: Verify on Linux.** Run `scripts/install.sh`, then `gh optivem --version`. Done when: the version prints `dev-<sha>` and `file gh-optivem` reports an ELF. Note: `install.sh` also runs `gh optivem claude setup`, which merges `~/.claude/settings.json`. Check that the permissions allowlist added on 2026-11-05 survives the merge.
- [ ] **Step 8: Windows check (human).** On the Windows host, run `scripts/install.sh` in Git Bash and confirm `gh optivem --version` works and `gh-optivem.exe` is rebuilt; the Linux `gh-optivem` next to it is untouched. This needs the user's Windows machine.

## Non-goals and invariants

- Don't change release packaging (`gh-optivem-<os>-<arch>[.exe]` assets via the precompile workflow); only local dev builds are in scope.
- Don't move the build output out of the repo root: `gh extension install .` requires the binary at the root, named after the repo.
- Out of scope: `scripts/atdd-rehearsal.sh` hard-wiring `../shop` (decision, user 2026-11-05: tracked separately; this plan touches only its binary name).
- Out of scope: giving the devcontainer its own clone instead of the shared Windows bind mount (the deeper fix; separate decision).
