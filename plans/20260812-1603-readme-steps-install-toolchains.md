# 2026-08-12 16:03:00 CEST — Install Go, Java, .NET and Node in readme-steps.sh instead of stopping on them

## TL;DR

**Why:** The clean-room walkthrough stops dead at `[Local environment setup]` on every fresh guest, because `readme-steps.sh` *demands* Go, Java, .NET and Node but installs none of them. The script's own comment has recorded this as known-unfinished since it was written: "Still NOT covered here: Go, Docker Desktop, Java, .NET, and Node ... a clean guest stops at `go version`."

**End result:** A run on a restored `clean-baseline` checkpoint installs those four toolchains itself, at the versions the repo's own CI pins, and carries on. The only tool left needing a human is Docker Desktop — deliberately, and with a comment saying why.

## Outcomes

What we get out of this — the goals and deliverables:

- The clean-room loop reaches Docker instead of Go: four of the five manual-install stops are gone, and the walkthrough gets four sections further per operator intervention.
- The guest's Java, .NET and Node match `.github/actions/acceptance-test/action.yml` exactly, so a scaffolded project that builds in CI builds in the guest — no "works on the runner" version drift.
- Docker Desktop stays the one documented manual step, and the script says out loud why it is different rather than looking like an oversight.
- A toolchain that is installed but PATH-invisible fails loudly at the setup step that owns it, instead of surfacing as a baffling Gradle/dotnet/npm error twenty minutes later.
- The file's prose stops contradicting its code: four comment blocks that still describe Go/Java/.NET/Node as un-installable are corrected.

## Evidence

Clean-room Hyper-V guest run, `.vm-logs/20260812-155808-readme-steps.log:101`:

```
machine setup: Go NOT installed
machine setup: Go is required and this script does not install it. Install it from https://go.dev/dl/ and re-run.
readme-steps: FAILED (exit 1) during [Local environment setup].
```

Root cause, pinned:

- `scripts/readme-steps.sh:554-556` — `ensure_go_installed` calls `require_tool 'Go' 'https://go.dev/dl/' go version`, and `require_tool` (`scripts/readme-steps.sh:287-295`) prints the README link and `exit 1` by design.
- `scripts/readme-steps.sh:579-583` — `ensure_language_toolchains` does the same for Java, .NET and Node.

The six missing env vars and the `actionlint`/`docker` verify failures earlier in that log are **expected**, not part of this defect: the `gh optivem environment verify` call at `scripts/readme-steps.sh:539` is `|| true`-gated precisely so a brand-new system reports everything missing in one pass. Go is the first genuine error.

Classification: incomplete clean-room harness — not a product bug, not a flake. It cannot reproduce on the host (all six tools are present there); the failure is clean-guest-specific by construction and the log is the evidence. No parallel implementation to fix: `scripts/readme-setup.ps1:27` explicitly delegates every other install to `readme-steps.sh`.

## ▶ Next executable step (resume here)

Do Step 1 — swap `require_tool` for `winget_install` in `ensure_go_installed` (`scripts/readme-steps.sh:554-556`) and in `ensure_language_toolchains` (`scripts/readme-steps.sh:579-583`), using the four winget ids in the table below. `winget_install` (`scripts/readme-steps.sh:242-278`) already carries the right semantics — skip-if-present, tolerate winget's non-zero-but-installed exit codes, `refresh_path`, then assert the tool runs or `exit 1` — so this is a call-site swap, not a new install path. Leave `ensure_docker_installed` alone. Steps 2–4 are comment and hardening work on the same file; Step 5 is the syntax gate. Everything lives in one file, `scripts/readme-steps.sh`; no README change is needed, because `machine_setup` and the `ensure_*` functions already own the Windows-guest commands the README only links to.

## Steps

- [ ] **Step 1: Install the four toolchains via `winget_install`.**

  `scripts/readme-steps.sh:554-556` — `ensure_go_installed`:

  ```bash
  winget_install GoLang.Go 'Go' go version
  ```

  `scripts/readme-steps.sh:579-583` — `ensure_language_toolchains`:

  ```bash
  winget_install EclipseAdoptium.Temurin.21.JDK 'Java'    java   -version
  winget_install Microsoft.DotNet.SDK.8         '.NET'    dotnet --version
  winget_install OpenJS.NodeJS.22               'Node.js' npm    --version
  ```

  All four ids were verified present on `--source winget` before this plan was written. Version rationale, to be captured as a comment above `ensure_language_toolchains`:

  | Tool | Winget id | Pinned to |
  |---|---|---|
  | Go | `GoLang.Go` | unpinned — it exists only to `go install` actionlint |
  | Java | `EclipseAdoptium.Temurin.21.JDK` | `.github/actions/acceptance-test/action.yml:72` (`java-version: 21`, `distribution: temurin`) |
  | .NET | `Microsoft.DotNet.SDK.8` | `.github/actions/acceptance-test/action.yml:79` (`dotnet-version: 8.0.x`) |
  | Node.js | `OpenJS.NodeJS.22` | `.github/actions/acceptance-test/action.yml:85` (`node-version: 22.x`) |

  Use `OpenJS.NodeJS.22`, **not** `OpenJS.NodeJS.LTS` — LTS currently resolves to 24.19.0 and would silently drift off the CI pin the moment upstream promotes a new LTS.

  Keep the existing note that all three languages are installed because `readme_generate_your_project` (`scripts/readme-steps.sh:608-611`) scaffolds with all three.

- [ ] **Step 2: Leave Docker manual, and say why.**

  `scripts/readme-steps.sh:573-575` — `ensure_docker_installed` stays a `require_tool`. Add a short comment: Docker Desktop needs a reboot, WSL2, and an interactive first launch, so installing it would convert a clear "install Docker" stop into a confusing "cannot connect to the Docker daemon" failure several steps later. Without this the asymmetry with the four tools above reads as an oversight.

- [ ] **Step 3: Close the "installed but not on PATH" hole in `winget_install`.**

  `scripts/readme-steps.sh:257-260` — the branch that prints *"is installed (`$id` is in the winget package database) but not on this shell's PATH - skipping the install"* returns **without** verifying the tool is runnable. That was written to stop a working `gh` from being reinstalled, but applied to Java/.NET/Node it means a toolchain whose installer skipped the PATH feature sails past this section and detonates later inside a Gradle, `dotnet` or `npm` invocation.

  Fix: before conceding, call `refresh_path` and re-probe. Return only if the probe now passes; otherwise fall through to the install, or exit loud naming both the tool and the package id. This keeps the `gh` case working (a stale shell PATH is exactly what `refresh_path` repairs) while honouring the file's own rule at `scripts/readme-steps.sh:57-64`: *Success is ASSERTED, never inferred.*

- [ ] **Step 4: Correct the comments this invalidates.** All in `scripts/readme-steps.sh`:

  - `:280-284` — `require_tool` doc-comment: "Bash, Go, Docker, and the language toolchains" → Bash and Docker only.
  - `:354-357` — inventory doc-comment: "require_tool exits on a missing Go, Docker, Java, .NET or Node" → Docker only. The paragraph's conclusion (that a run stopping there never reaches the closing pass) still holds for Docker, so keep it.
  - `:409-411` — `machine_setup` section header: "Still NOT covered here: Go, Docker Desktop, Java, .NET, and Node ... Install those by hand first." → Docker Desktop only.
  - `:552-553` — the comment above `ensure_go_installed` ("The README's own note: Go only matters because actionlint below is installed with it"). Keep the rationale; adjust the wording only if the new body makes it read wrong.

- [ ] **Step 5: Syntax gate.** Run `bash -n scripts/readme-steps.sh`, and `shellcheck scripts/readme-steps.sh` if shellcheck is on the host. Then re-read the "Run" block at `scripts/readme-steps.sh:733-759` and confirm the call list still matches `README.md`'s headings one-for-one — the file's own stated contract at `scripts/readme-steps.sh:19-33`.

## Verification

- `bash -n scripts/readme-steps.sh` exits clean (and shellcheck, if available).
- The `## Run` call list at `scripts/readme-steps.sh:733-759` still mirrors `README.md`'s headings — no heading in one and not the other.
- **Operator, in the guest:** restore the `clean-baseline` Hyper-V checkpoint, re-run `bash scripts/readme-steps.sh`, and confirm:
  1. the run advances past `[Local environment setup]`;
  2. the `inventory: after` block shows real versions next to Go, actionlint, Java, .NET and Node.js where `inventory: before` said NOT installed;
  3. the only remaining stop in that section is Docker.

## Notes

**Known next blocker, out of scope for this plan.** The same log shows six unset tokens — `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN`, `SONAR_TOKEN`, `GHCR_TOKEN`, `WORKFLOW_TOKEN`, `REPO_TOKEN`. Once Go through Node stop the run no longer, `readme_environment_variables` at `scripts/readme-steps.sh:594` becomes the next stop, and unlike the earlier call that one is **not** `|| true`-gated. That is guest provisioning, not a script defect — no items here address it.
