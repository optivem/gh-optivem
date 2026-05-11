# Contributing

## Contents

- [Prerequisites](#prerequisites)
- [Run locally](#run-locally)
- [Install from source](#install-from-source)
- [Tests](#tests)
  - [Windows: keep `go test ./...` fast](#windows-keep-go-test--fast)
- [ATDD process](#atdd-process)
  - [View the diagram](#view-the-diagram)
  - [Render the diagram](#render-the-diagram)
  - [implement-ticket — what it does](#implement-ticket--what-it-does)
  - [Two ways to rehearse the full flow](#two-ways-to-rehearse-the-full-flow)
    - [Part 1 — dev loop: local gh-optivem against existing shop](#part-1--dev-loop-local-gh-optivem-against-existing-shop)
    - [Part 2 — external-user flow: released extension + brand-new scaffolded repo](#part-2--external-user-flow-released-extension--brand-new-scaffolded-repo)
  - [Debug a single phase](#debug-a-single-phase)
  - [Running on CI](#running-on-ci)
- [Releasing](#releasing)

## Prerequisites

- [Go 1.22+](https://go.dev/dl/)
- [GitHub CLI](https://cli.github.com/) (`gh auth login`)
- A C compiler on `PATH` (`gh-optivem` builds with `CGO_ENABLED=1` because the tree-sitter language bindings require CGo). Check first with `gcc --version` — if it prints a version, you're done. Otherwise install: Windows: `scoop install gcc` (MinGW) or `choco install mingw` (admin shell, then restart your terminal so `PATH` picks it up). macOS: `xcode-select --install`. Linux: `apt install gcc` (or your distro's equivalent). End users on `gh extension install optivem/gh-optivem` don't need this — they download the prebuilt binary.
- `CGO_ENABLED=1` in your Go env. Check with `go env CGO_ENABLED` — if it prints `0`, run `go env -w CGO_ENABLED=1`. Without this, Go silently excludes the tree-sitter binding files via build constraints and `go build` fails with `build constraints exclude all Go files in …/tree-sitter-typescript@…/bindings/go`.

## Quick smoke test (no install)

Sanity-check that the code compiles and the CLI runs without touching your `gh` extension state:

```bash
go run . --version
```

For invocations beyond `--version`, see the README for usage examples — once you want to iterate, `bash scripts/install.sh` (below) gives you `gh optivem` for the natural invocation form.

End-to-end manual test (creates real GitHub repos + SonarCloud projects; cleaned up by `scripts/cleanup-orphans.sh` on success, kept on failure for debugging):

```bash
bash scripts/manual-test.sh --owner valentinajemuovic --system-name "Page Turner" \
    --arch multitier --repo-strategy multirepo \
    --backend-lang dotnet --frontend-lang react --test-lang typescript \
    --shop-ref main
```

Skip slow steps with `--no-local-tests --no-local-sonar --no-legacy`. Keep the scaffold dir on success with `--no-cleanup` / `--keep-local`. See [README.md](README.md#usage) for the full flag set.

## Install from source

```bash
bash scripts/install.sh   # rebuilds gh-optivem.exe and links it as `gh optivem`
```

Run this any time you edit CLI source (e.g. `atdd_commands.go`, anything under `internal/atdd/runtime/diagram/`, etc.). Without rebuilding, `gh optivem …` keeps running the previously built binary and silently masks your changes — cobra falls through to help text for subcommands the old binary doesn't know about, and `>` redirects then clobber whatever file you piped into.

`--shop-ref` resolution for local builds: explicit flag wins; otherwise the latest `meta-v*` release of `optivem/shop`. Released binaries (`gh extension install optivem/gh-optivem`) pin the shop SHA baked in at release time and do **not** auto-upgrade. Pass `--shop-ref main` to test against unreleased shop changes.

## Tests

```bash
go test -p 2 ./...                            # unit; -p caps parallel package builds (see Windows tip below)
go test -tags=system ./...                    # all system tests
bash scripts/test-system.sh                   # quick subset
bash scripts/test.sh ./internal/atdd/...      # wrapper: caps -p (default 2), refuses ./... without --all
bash scripts/test.sh --all ./...              # opt in to a repo-wide run (still capped)
```

While you're iterating in one package, run just that package: `go test ./internal/atdd/runtime/clauderun`. Save `./...` for pre-push and CI.

A single system test (requires `TEST_OWNER`, `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN`, `SONAR_TOKEN`, `GHCR_TOKEN`, `WORKFLOW_TOKEN` in env):

```bash
go test -tags=system ./internal/config/ -v -timeout 2h \
    -run "TestValidMonolithConfigurations/monolith_monorepo_java_dotnet"
```

### Windows: keep `go test ./...` fast

`go test ./...` builds a separate test binary per package. On Windows the linker is the slow phase and Defender real-time-scans every fresh `.exe` Go writes — the 13+ packages under `internal/atdd/` link-storm the box for several minutes at peak RAM.

- Cap parallel package builds: `go test -p 2 ./...` (Go's default is `NumCPU`).
- Add Defender exclusions — Settings → Virus & threat protection → Exclusions:
  - Folders: `%LocalAppData%\go-build`, `%USERPROFILE%\go`, the repo root.
  - Processes: `go.exe`, `link.exe`, `compile.exe`, `gofmt.exe`.
- Confirm `go env GOCACHE GOMODCACHE` aren't under OneDrive / Dropbox / a network drive — sync clients re-read every cache file Go writes. If they are: `setx GOCACHE C:\go-cache` and `setx GOMODCACHE C:\go-mod`.

CI runs Linux and isn't affected; CI parallelism is unchanged.

## ATDD process

The ATDD driver walks the embedded process-flow YAML against a real GitHub issue, dispatching agent prompts to the `claude` CLI at each user-task node. The YAML is read from the **caller's working directory**, so smoke-tests run from inside a consumer repo (typically `shop`).

### View the diagram

The canonical rendered diagram is [docs/process-diagram.md](docs/process-diagram.md). GitHub renders Mermaid natively — just open it on github.com.

Standalone SVGs of each Mermaid chart are committed under `docs/images/` for tools that don't render Mermaid (slides, external docs). They're regenerated automatically by the `regenerate-diagram` workflow on push to main, so you normally don't need to render them yourself.

If you do want to render locally (requires `npx`):

```bash
bash scripts/render-svgs.sh
```

The script pins `@mermaid-js/mermaid-cli` to match the version the workflow uses.

### Regenerate the diagram

Do not edit `docs/process-diagram.md` by hand — it is generated from the YAML. To regenerate it locally:

```bash
gh optivem atdd show diagram > docs/process-diagram.md
```

The `regenerate-diagram` workflow watches `internal/atdd/runtime/statemachine/process-flow.yaml` and `internal/atdd/runtime/diagram/**`, but it behaves differently depending on the event:

- **Pull requests** — renders and **fails the PR** if the committed diagram is stale. It does *not* auto-fix the PR branch. So when you edit the YAML in a feature branch, run the command above and commit the result alongside your YAML change before opening the PR.
- **Push to `main`** — regenerates and commits the updated diagram back to main as `github-actions[bot]`. Direct main pushes are the only path where you don't have to regenerate yourself.

### implement-ticket — what it does

`gh optivem atdd implement-ticket --issue <n>` moves the issue to **In Progress**, then walks the embedded process-flow node-by-node, launching the matching Claude Code subagent in your terminal at each user-task node. When the subagent commits and exits, the engine advances.

Useful flags:

- `--autonomous` — headless agents (`claude -p`)
- `--manual-agents` — v1 two-window dispatch (driver pauses, human launches the agent in a separate Claude Code session, presses Enter to advance). Right tool when bisecting "did v2 misroute?" vs. "did v1 see the commit?".

Per-node prompt shaping (`extra` text, full `replace`, alternate `process_flow`, or `agent_prompts` swaps) is configured via fields in `gh-optivem.yaml`, not flags — see the [ATDD-specific overrides](README.md#atdd-specific-overrides) section in the README.

The two rehearsal flows below show how to actually invoke it.

### Two ways to rehearse the full flow

Both end with `implement-ticket` walking a real GitHub issue. Pick based on what you're testing.

#### Part 1 — dev loop: local gh-optivem against existing shop

Fast iteration on the driver. **Local working copy of gh-optivem** + **existing shop repo** (no scaffolding). A throwaway git worktree on a `rehearsal/<timestamp>[-<label>]` branch keeps the rehearsal off shop's main; the worktree is the right model here precisely because shop is a long-lived repo you don't want to dirty.

##### Quick path (no extra flags)

`scripts/atdd-rehearsal.sh` does **everything** end-to-end: `go build`s `gh-optivem.exe` from your working copy, creates the worktree, runs `implement-ticket` inside it, prompts to delete the worktree + branch on exit.

```bash
# Step 1 — go to shop
cd ../shop

# Step 2 — run the rehearsal (pick one form)
bash ../gh-optivem/scripts/atdd-rehearsal.sh 61
bash ../gh-optivem/scripts/atdd-rehearsal.sh 61 ticket-cli                       # optional sortable label
bash ../gh-optivem/scripts/atdd-rehearsal.sh https://github.com/optivem/shop/issues/61

# Step 3 — answer the cleanup prompt (default Y deletes worktree + branch; n keeps for inspection)
```

The worktree lands at `../rehearsal-<id>` (sibling of shop).

##### Iterating on the same worktree

The rehearsal script is one-shot — it runs `implement-ticket` once, then exits with the cleanup prompt. If you answered `n` to keep the worktree (e.g. the run failed and you want to retry with a fixed driver, or you want to extend the same branch), iterate by hand. The worktree path was logged at the start of the script; assume it's `../rehearsal-<id>`.

```bash
# Step 1 — edit gh-optivem code in the gh-optivem repo (not in the worktree)

# Step 2 — rebuild gh-optivem.exe from shop
cd ../shop
go -C ../gh-optivem build -o gh-optivem.exe .

# Step 3 — cd into the kept worktree and re-run implement-ticket
cd ../rehearsal-<id>           # tab-complete <id> from the script's log line
../gh-optivem/gh-optivem.exe atdd implement-ticket --issue 61

# Step 4 — when truly done, clean up from shop
cd ../shop
git worktree remove --force ../rehearsal-<id>
git branch -D rehearsal/<id>
```

Re-running on the same worktree means subsequent commits land on the same `rehearsal/<id>` branch, so the diff accumulates. If you want a clean slate, exit, choose to delete the worktree, and run the rehearsal script again — that creates a fresh `rehearsal/<new-ts>` branch.

##### Flag-aware path (when you need `--autonomous`, `--manual-agents`, …)

The rehearsal script doesn't accept extra flags, so do it by hand:

```bash
# Step 1 — go to shop
cd ../shop

# Step 2 — build gh-optivem from your local working copy
go -C ../gh-optivem build -o gh-optivem.exe .

# Step 3 — create a throwaway worktree on a rehearsal branch
TS=$(date +%Y%m%d-%H%M%S)
git worktree add -b "rehearsal/${TS}" "../rehearsal-${TS}"
cd "../rehearsal-${TS}"

# Step 4 — run implement-ticket with whatever flags you need (pick one)
../gh-optivem/gh-optivem.exe atdd implement-ticket --issue 42
../gh-optivem/gh-optivem.exe atdd implement-ticket --issue 42 --autonomous
../gh-optivem/gh-optivem.exe atdd implement-ticket --issue 42 --manual-agents
../gh-optivem/gh-optivem.exe -c ./gh-optivem.experimental.yaml atdd implement-ticket --issue 42  # swap node_extras / agent_prompts / process_flow via an alternate config
../gh-optivem/gh-optivem.exe atdd manage-project                                  # pick top Ready item from project board

# Step 5 — clean up the worktree + branch when done
cd ../shop
git worktree remove --force "../rehearsal-${TS}"
git branch -D "rehearsal/${TS}"
```

#### Part 2 — external-user flow: released extension + brand-new scaffolded repo

What a real user actually does. **Released `gh-optivem` extension** (pins the shop SHA baked in at release time — does not auto-upgrade) + a **completely fresh repo** scaffolded by `gh optivem init`. No worktree — the scaffold is a brand new repo, you just work on main.

```bash
# Step 1 — install (or upgrade) the published extension
gh extension install optivem/gh-optivem
# gh extension upgrade optivem        # if already installed

# Step 2 — confirm you're on the latest release
gh optivem --version

# Step 3 — generate gh-optivem.yaml for the project (interactive or via flags)
gh optivem config init --owner valentinajemuovic --repo page-turner \
    --system-name "Page Turner" --arch multitier --repo-strategy multirepo \
    --backend-lang dotnet --frontend-lang react --test-lang typescript \
    --project-url https://github.com/orgs/valentinajemuovic/projects/N \
    --backend-path backend --frontend-path frontend \
    --system-test-path system-test \
    --stubs-path external-systems/external-stub \
    --simulators-path external-systems/external-real-sim

# Step 4 — scaffold a fresh project (no --shop-ref → uses the baked-in SHA)
gh optivem init

# Step 5 — walk an issue on the new repo
cd page-turner
gh optivem atdd implement-ticket --issue 1

# Step 5 — (optional) remove the extension
gh extension remove optivem
```

Use this to reproduce a user-reported issue against the same binary they're running, or to smoke-test what an external user gets right after a release.

### Debug a single phase

`gh optivem atdd debug …` exercises individual runtime packages standalone. Flag shapes here are not part of the stable API.

```bash
gh optivem atdd debug pick-top-ready                              # what would manage-project pick?
gh optivem atdd debug classify --issue 42                         # deterministic fast path
gh optivem atdd debug next-phase --node GATE_DSL --state dsl_interface_changed=true
gh optivem atdd debug gate dsl_changed                            # one gateway binding
gh optivem atdd debug release --issue 42 --dry-run                # release primitives
```

Run `gh optivem atdd debug --help` for the full list.

### Running on CI

The driver shells out to `claude`, which needs `~/.claude/credentials.json`. A pre-flight `claude --no-update-check --version` runs at startup (skipped under `--manual-agents`) so a missing/unauthenticated CLI surfaces before any flow-walking. Bootstrap options:

- **Bake credentials into the runner image** — run `claude /login` once locally as the executing user, copy `~/.claude/` into the image.
- **Mount at job start** from an encrypted secret:
  ```bash
  mkdir -p ~/.claude
  printf '%s' "$CLAUDE_CREDENTIALS" > ~/.claude/credentials.json
  chmod 600 ~/.claude/credentials.json
  ```
- **Fall back to `--manual-agents`** — driver pauses at each user-task; a human dispatches the agent and presses Enter to advance. Right choice when CI should walk the gates / actions but not the agent dispatches.

Rate-limit failures surface as `rate limit hit on Claude subscription; weekly cap likely exhausted …`; mid-run credential expiry surfaces as `claude CLI is not authenticated — run \`claude /login\` …`. Both are detected from the runner's stderr signature.

## Releasing

This project uses [semantic versioning](https://semver.org/).

```bash
git tag v1.2.3
git push origin v1.2.3
```

Triggers the Release workflow (GoReleaser builds binaries for all platforms and publishes a GitHub Release). Users on `gh extension install optivem/gh-optivem` get the new version on their next `gh extension upgrade`.
