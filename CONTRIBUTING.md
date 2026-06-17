# Contributing

## Most-used commands

Rebuild + link `gh optivem` after editing CLI source (see [Install from source](#install-from-source)):

```bash
bash scripts/install.sh   # rebuilds gh-optivem.exe and links it as `gh optivem`
```

End-to-end manual test (see [Quick smoke test](#quick-smoke-test-no-install) for details):

```bash
bash scripts/manual-test.sh --owner valentinajemuovic --system-name "Page Turner" \
    --arch multitier --repo-strategy multirepo \
    --backend-lang dotnet --frontend-lang typescript --test-lang typescript \
    --shop-ref main
```

Dev-loop ATDD rehearsal (see [Part 1](#part-1--dev-loop-local-gh-optivem-against-existing-shop) for details):

Rehearse several tickets back-to-back, unattended:

```bash
cd ../shop

# Full corpus (61 65 68 69 70 71 72 76 78 79 80 81), default java config:
bash ../gh-optivem/scripts/atdd-rehearsal-loop.sh

# A subset, in the given order:
bash ../gh-optivem/scripts/atdd-rehearsal-loop.sh 72 71 69 76 61

# Delete each worktree after its run instead of keeping them:
bash ../gh-optivem/scripts/atdd-rehearsal-loop.sh --keep never
```

`atdd-rehearsal-loop.sh` runs `atdd-rehearsal.sh` over each ticket serially (`--auto --headless`), keeping each worktree+branch on exit by default (`--keep always`; the per-run `.log` is kept regardless) and **stopping on the first failure**. Pass `--keep never` to delete every worktree or `--keep on-failure` to keep only the broken one; `--config <yaml>` to switch stacks for the whole run; run with `--help` for all options. Stale worktrees accumulate under `../worktrees/` — clear them with `atdd-rehearsal-cleanup.sh`.

**Choosing the stack / language.** There is no `--language` flag — language is one axis of the `--config` value. Each config is a `gh-optivem.yaml` variant living in `../shop/`, named `gh-optivem-<arch>-<lang>[-legacy].yaml`, where `<arch>` is `monolith` | `multitier` and `<lang>` is `java` | `dotnet` | `typescript` (the `-legacy` suffix selects the legacy stack). The loop's `--config` applies to the **whole run** — to exercise more than one stack, invoke it once per config. Available variants:

| | `java` | `dotnet` | `typescript` |
| --- | --- | --- | --- |
| **monolith** | `gh-optivem-monolith-java.yaml` | `gh-optivem-monolith-dotnet.yaml` | `gh-optivem-monolith-typescript.yaml` |
| **multitier** | `gh-optivem-multitier-java.yaml` | `gh-optivem-multitier-dotnet.yaml` | `gh-optivem-multitier-typescript.yaml` |

Each also has a `-legacy` companion (e.g. `gh-optivem-monolith-java-legacy.yaml`). The loop defaults to `gh-optivem-monolith-java.yaml`. For example, to run the full corpus against the TypeScript monolith:

```bash
bash ../gh-optivem/scripts/atdd-rehearsal-loop.sh --config gh-optivem-monolith-typescript.yaml
```

> **Windows one-time setup:** enable long paths so the deep rehearsal worktrees can be deleted (otherwise removal fails with `Filename too long`, orphaning the directory):
>
> ```bash
> git config --global core.longpaths true
> ```

> **One-time setup — rebase on pull:** set pull to rebase rather than merge:
>
> ```bash
> git config --global pull.rebase true
> ```
>
> This academy workspace is ~18 sibling repos, and rehearsal loops plus multiple sessions/machines push to the same branch, so your local `main` frequently diverges from `origin/main`. With the default merge-pull, a divergent push is **rejected** (`! [rejected] … (fetch first)`) — and in VS Code's Source Control that surfaces as an **endless progress spinner** rather than a clear error, because it can't complete the push. Rebasing on pull replays your local commits on top of the remote ones, keeping history linear and letting a Sync resolve the divergence cleanly instead of stalling. When the spinner does appear, the reliable unstick is a terminal in that repo: `git pull` (now rebases) then `git push`.

To rehearse a single ticket, use `atdd-rehearsal.sh` directly:

For structural change - UI Redesign

**Issue [#61 — Redesigning New Order UI](https://github.com/optivem/shop/issues/61)**:

```bash
cd ../shop

bash ../gh-optivem/scripts/atdd-rehearsal.sh 61 --config gh-optivem-monolith-java.yaml --auto --headless
```

For behavioral change - user story

**Issue [#65 — View product list](https://github.com/optivem/shop/issues/65)** (read-only flow):

```bash
cd ../shop

bash ../gh-optivem/scripts/atdd-rehearsal.sh 65 --config gh-optivem-monolith-java.yaml --auto --headless
```

**Issue [#68 — Apply automatic quantity discount on cart lines](https://github.com/optivem/shop/issues/68)** (write flow with calculation rule):

```bash
cd ../shop

bash ../gh-optivem/scripts/atdd-rehearsal.sh 68 --config gh-optivem-monolith-java.yaml --auto --headless
```

**Issue [#69 — Reject order with line quantity of 100](https://github.com/optivem/shop/issues/69)** (write flow with validation rule):

```bash
cd ../shop

bash ../gh-optivem/scripts/atdd-rehearsal.sh 69 --config gh-optivem-monolith-java.yaml --auto --headless
```

**Issue [#70 — Return a delivered order](https://github.com/optivem/shop/issues/70)** (write flow extending the DSL + driver surface):

```bash
cd ../shop

bash ../gh-optivem/scripts/atdd-rehearsal.sh 70 --config gh-optivem-monolith-java.yaml --auto --headless
```

**Issue [#71 — Gift-wrap an order](https://github.com/optivem/shop/issues/71)** (write flow adding a new field to the existing DSL):

```bash
cd ../shop

bash ../gh-optivem/scripts/atdd-rehearsal.sh 71 --config gh-optivem-monolith-java.yaml --auto --headless
```

**Issue [#72 — Charge shipping based on product weight from ERP](https://github.com/optivem/shop/issues/72)** (write flow adding a new field to the external-system ERP DSL):

```bash
cd ../shop

bash ../gh-optivem/scripts/atdd-rehearsal.sh 72 --config gh-optivem-monolith-java.yaml --auto --headless
```

> **#72 is the full-BPMN rehearsal story.** It is the only story in this corpus that trips all three change-detection gates on the `change-system-behavior` path TRUE, so it walks the entire flow end-to-end:
> - `at-dsl-port-changed` — new `weighing …` / `shipping fee …` DSL steps + the `hasWeight` contract accessor.
> - `at-external-driver-port-changed` — ERP `GetProductResponse` gains `weight` (only `sku`+`price` today), driving the contract-test cascade (external driver port + adapters, real-sim, stub).
> - `at-system-driver-port-changed` — `ViewOrderResponse` has no `shippingFee` field, so asserting "shipping fee is 0.60" forces a new system driver-port field, driving the per-channel system driver adapter tail.
>
> Because `erp` is `real-kind: simulator` in every shop config, it also takes the longest external branch (verify-fail real → author real simulator → verify-pass real → stub red→green). By contrast #65/#69/#70/#71 have no external system, and #68's discount fields already exist on `ViewOrderResponse`.
>
> **Known coverage gap:** with `erp`/`clock` pinned to `real-kind: simulator`, the `real-kind == test-instance` branch of the contract-test flow (collapse to a single contract-real pass-verify, no real-sim authoring) is never exercised by any shop rehearsal. The simulator path is the superset, so this is fine functionally — noted as a gap until some external system is declared `test-instance`.

For bug fix - reproduce then fix

**Issue [#76 — Order cancellation blackout on Dec 31 ends at 22:30 instead of 23:00](https://github.com/optivem/shop/issues/76)** (bug-fix flow: a failing acceptance test reproduces the defect, then the fix turns it green):

```bash
cd ../shop

bash ../gh-optivem/scripts/atdd-rehearsal.sh 76 --config gh-optivem-monolith-java.yaml --auto --headless
```

> **#76 is the only bug-fix rehearsal in this corpus.** Every other story (#61, #65, #68–#72) adds new behavior, so its acceptance test starts red because the feature is absent. #76 starts red against a defect that already exists in the system: `cancelOrder` blocks the Dec 31 cancellation blackout only for 22:00–22:30, while its own validation message states the policy is 22:00–23:00. The fix is a pure behavioral write-flow change (extend the window end to 23:00) — no DSL or driver-port change, reusing the existing cancel + clock-control DSL steps.

For structural refactor — system internals

**Issue [#78 — Extract order pricing math into an OrderPricing component](https://github.com/optivem/shop/issues/78)** (`task / system-refactor` → `refactor-system-structure`):

```bash
cd ../shop

bash ../gh-optivem/scripts/atdd-rehearsal.sh 78 --config gh-optivem-monolith-java.yaml --auto --headless
```

> **#78 is a structural refactor with no behavior change.** Task-kind ticket (Checklist body, not Gherkin AC) labelled `subtype:system-refactor`, so the `implement-ticket` gateway routes `ticket-kind == task` → `task-subtype == system-refactor` → the `refactor-system-structure` CYCLE. The cart-line discount + shipping-fee math is extracted into an internal `OrderPricing` component; the full regression runs GREEN with no AT-red phase and no DSL/driver/port change.

For structural refactor — test structure

**Issue [#79 — Extract a shared cart/order builder in the acceptance DSL](https://github.com/optivem/shop/issues/79)** (`task / test-refactor` → `refactor-test-structure`):

```bash
cd ../shop

bash ../gh-optivem/scripts/atdd-rehearsal.sh 79 --config gh-optivem-monolith-java.yaml --auto --headless
```

> **#79 is a test-only structural refactor.** Labelled `subtype:test-refactor`, it walks `refactor-test-structure` → `refactor-and-verify-tests`: the repeated cart/order-building setup in the acceptance DSL is extracted into a shared builder/helper, with no production change and tests staying GREEN throughout.

For legacy coverage — write passing ATs for existing behavior

**Issue [#80 — Coupon validity-window enforcement during PlaceOrder](https://github.com/optivem/shop/issues/80)** (`task / legacy-coverage` → `cover-system-behavior`):

```bash
cd ../shop

bash ../gh-optivem/scripts/atdd-rehearsal.sh 80 --config gh-optivem-monolith-java.yaml --auto --headless
```

> **#80 is the green-from-start coverage rehearsal.** Labelled `subtype:legacy-coverage`, it walks `cover-system-behavior` → `write-and-verify-acceptance-tests-pass` (`verify-mode: green-when-complete`). It adds a *passing* acceptance test for an existing-but-uncovered rule — placing an order with an *expired* coupon (`validTo` in the past) is rejected with `couponCode: "Coupon code <X> has expired"`, enforced today in `CouponService.getDiscount` but covered by no AT. The test is GREEN from the first verify, with no system-implementation phase — the opposite of #76, whose AT starts red against a defect.

For structural redesign — external system boundary

**Issue [#81 — Reshape ERP GetProductResponse structure (no behavior change)](https://github.com/optivem/shop/issues/81)** (`task / external-system-redesign` → `redesign-external-system-structure`):

```bash
cd ../shop

bash ../gh-optivem/scripts/atdd-rehearsal.sh 81 --config gh-optivem-monolith-java.yaml --auto --headless
```

> **#81 redesigns the external-system boundary.** Labelled `subtype:external-system-redesign`, it walks `redesign-external-system-structure` → `update-external-system-driver-adapters`: the ERP `GetProductResponse` is structurally reshaped (e.g. nesting `price` under a `pricing` object) with no behavior change and full-regression GREEN. It is the external-side sibling of #61, which only covers the system-side `redesign-system-structure`. ERP is declared in `gh-optivem-monolith-java.yaml`, so the boundary exists to reshape.

## Contents

- [Prerequisites](#prerequisites)
- [Run locally](#run-locally)
- [Init flags for development and CI](#init-flags-for-development-and-ci)
  - [Local cleanup](#local-cleanup)
  - [Unattended runs (CI)](#unattended-runs-ci)
  - [Deployment target](#deployment-target)
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
- [Project config (`gh-optivem.yaml`)](#project-config-gh-optivemyaml)
  - [Pointing at non-default configs](#pointing-at-non-default-configs)
    - [ATDD-specific overrides](#atdd-specific-overrides)
- [How it works](#how-it-works)
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
# multitier + multirepo, .NET backend, TypeScript frontend + TypeScript system tests
bash scripts/manual-test.sh --owner valentinajemuovic --system-name "Page Turner" \
    --arch multitier --repo-strategy multirepo \
    --backend-lang dotnet --frontend-lang typescript --test-lang typescript \
    --shop-ref main

# multitier + multirepo, Java backend, TypeScript frontend + TypeScript system tests
bash scripts/manual-test.sh --owner valentinajemuovic --system-name "Page Turner" \
    --arch multitier --repo-strategy multirepo \
    --backend-lang java --frontend-lang typescript --test-lang typescript \
    --shop-ref main
```

Skip slow steps with `--no-local-tests --no-local-sonar --no-legacy`. Keep the scaffold dir on success with `--no-cleanup` / `--keep-local`. See [README.md](README.md#usage) for the full flag set.

## Init flags for development and CI

### Local cleanup

On a successful run the local scaffold dir is deleted — the end result is just the created GitHub repo(s) + SonarCloud project(s), which you can clone later. Pass `--keep-local` to keep the dir (e.g. for inspection). On failure the dir is always kept so the broken scaffold can be debugged.

### Unattended runs (CI)

Pass `--yes` (or `-y`) to skip all interactive confirmations — the existing-repo prompt and the `--report-bug` confirmation. This is the expected pattern for CI/automation:

```bash
gh optivem init ... --yes
```

### Deployment target

Only `--deploy docker` is currently supported (the default). `--deploy cloud-run` is in development and may be available in a future release.

### Auto-filed bug report (opt-in)

If you want the failure auto-filed to `optivem/gh-optivem` as an issue — including scaffold config — opt in with `--report-bug`:

```bash
gh optivem init ... --report-bug
```

Off by default. Filing a quick issue yourself is usually clearer and keeps the scaffold config private unless you decide to share it.

## Install from source

```bash
bash scripts/install.sh   # rebuilds gh-optivem.exe and links it as `gh optivem`
```

Run this any time you edit CLI source (e.g. `implement_commands.go`, anything under `internal/diagrams/diagram/`, etc.). Without rebuilding, `gh optivem …` keeps running the previously built binary and silently masks your changes — cobra falls through to help text for subcommands the old binary doesn't know about, and `>` redirects then clobber whatever file you piped into.

`--shop-ref` resolution for local builds: explicit flag wins; otherwise the latest `meta-v*` release of `optivem/shop`. Released binaries (`gh extension install optivem/gh-optivem`) pin the shop SHA baked in at release time and do **not** auto-upgrade. Pass `--shop-ref main` to test against unreleased shop changes.

## Tests

```bash
go test -p 2 ./...                            # unit; -p caps parallel package builds (see Windows tip below)
go test -tags=system ./...                    # all system tests
bash scripts/test-system.sh                   # quick subset
bash scripts/test.sh ./internal/atdd/...      # wrapper: caps -p (default 2), refuses ./... without --all
bash scripts/test.sh --all ./...              # opt in to a repo-wide run (still capped)
```

While you're iterating in one package, run just that package: `go test ./internal/atdd/process/clauderun`. Save `./...` for pre-push and CI.

A single system test (requires `TEST_OWNER`, `DOCKERHUB_USERNAME`, `DOCKERHUB_TOKEN`, `SONAR_TOKEN`, `GHCR_TOKEN`, `WORKFLOW_TOKEN` in env). Export them in your shell, or — for the credential vars the scaffolder binary reads — drop them in the user-level `.env` (see [Environment Variables](README.md#environment-variables)); the built binary the harness runs loads that file at startup, so no terminal restart is needed when you rotate a token:

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
gh optivem process show > docs/process-diagram.md
```

The `regenerate-diagram` workflow watches `internal/atdd/process/process-flow.yaml` and `internal/diagrams/diagram/**`, but it behaves differently depending on the event:

- **Pull requests** — renders and **fails the PR** if the committed diagram is stale. It does *not* auto-fix the PR branch. So when you edit the YAML in a feature branch, run the command above and commit the result alongside your YAML change before opening the PR.
- **Push to `main`** — regenerates and commits the updated diagram back to main as `github-actions[bot]`. Direct main pushes are the only path where you don't have to regenerate yourself.

### implement — what it does

`gh optivem implement --issue <n>` moves the issue to **In Progress**, then walks the embedded process-flow node-by-node, launching the matching Claude Code subagent in your terminal at each user-task node. When the subagent commits and exits, the engine advances. Omit `--issue` and the driver picks the top Ready item from the project board instead, then walks the same pipeline from START.

Useful flags:

- `--auto` (root flag, before `implement`) + `--headless` — fully autonomous mode: auto-approve everything except commit/fix, run each subagent as `claude -p` instead of an interactive session. Supersedes the deprecated `--autonomous` alias (which still works but warns and rewrites itself to `--auto --headless`).
- `--manual-agents` — v1 two-window dispatch (driver pauses, human launches the agent in a separate Claude Code session, presses Enter to advance). Right tool when bisecting "did v2 misroute?" vs. "did v1 see the commit?".

Per-node prompt shaping (`extra` text, full `replace`, alternate `process_flow`, or `task_prompts` swaps) is configured via fields in `gh-optivem.yaml`, not flags — see the [pipeline overrides](README.md#pipeline-overrides) section in the README.

The two rehearsal flows below show how to actually invoke it.

### Two ways to rehearse the full flow

Both end with `implement` walking a real GitHub issue. Pick based on what you're testing.

#### Part 1 — dev loop: local gh-optivem against existing shop

Fast iteration on the driver. **Local working copy of gh-optivem** + **existing shop repo** (no scaffolding). A throwaway git worktree on a `rehearsal/<timestamp>[-<label>]` branch keeps the rehearsal off shop's main; the worktree is the right model here precisely because shop is a long-lived repo you don't want to dirty.

##### Quick path

`scripts/atdd-rehearsal.sh` does **everything** end-to-end: runs `scripts/install.sh` (which `go build`s `gh-optivem.exe` from your working copy and re-installs the `gh optivem` extension), creates the worktree, runs `implement` inside it, prompts to delete the worktree + branch on exit. The script relies on the ambient shell environment for credentials; since the binary now loads the user-level `.env` at startup (see [Environment Variables](README.md#environment-variables)), the same portable file covers rehearsals too — no need to re-export tokens per shell. Docker state cleanup is no longer part of this script — if you want a fresh state (volumes + locally-built images dropped, per the current config's `systems.yaml`), run `bash ../gh-optivem/scripts/atdd-clean.sh [--config <yaml>]` first.

```bash
# Step 1 — go to shop
cd ../shop

# Step 2 — run the rehearsal (pick one form)
bash ../gh-optivem/scripts/atdd-rehearsal.sh 61 --config gh-optivem-monolith-java.yaml
bash ../gh-optivem/scripts/atdd-rehearsal.sh 61 ticket-cli                       # optional sortable label
bash ../gh-optivem/scripts/atdd-rehearsal.sh https://github.com/optivem/shop/issues/61
bash ../gh-optivem/scripts/atdd-rehearsal.sh 61 --auto --headless                # fully autonomous mode

# Step 3 — answer the cleanup prompt (default Y deletes worktree + branch; n keeps for inspection)
```

`--auto` and `--headless` are forwarded to the binary in the right positions (root vs. subcommand). For any other implement flag — `--manual-agents`, `--show-prompt`, `--log-file`, `--keep-runs`, `--workspace` — use the flag-aware path below.

The worktree lands at `../../worktrees/rehearsal-<id>` (under a `worktrees/` folder beside academy). The script auto-creates `worktrees/` if it's not there.

##### Iterating on the same worktree

The rehearsal script is one-shot — it runs `implement` once, then exits with the cleanup prompt. If you answered `n` to keep the worktree (e.g. the run failed and you want to retry with a fixed driver, or you want to extend the same branch), iterate by hand. The worktree path was logged at the start of the script; assume it's `../../worktrees/rehearsal-<id>`.

```bash
# Step 1 — edit gh-optivem code in the gh-optivem repo (not in the worktree)

# Step 2 — rebuild gh-optivem.exe from shop
cd ../shop
go -C ../gh-optivem build -o gh-optivem.exe .

# Step 3 — cd into the kept worktree and re-run implement
cd ../../worktrees/rehearsal-<id>   # tab-complete <id> from the script's log line
../../academy/gh-optivem/gh-optivem.exe implement --issue 61

# Step 4 — when truly done, clean up from shop
cd ../../academy/shop
git worktree remove --force ../../worktrees/rehearsal-<id>
git branch -D rehearsal/<id>
```

Re-running on the same worktree means subsequent commits land on the same `rehearsal/<id>` branch, so the diff accumulates. If you want a clean slate, exit, choose to delete the worktree, and run the rehearsal script again — that creates a fresh `rehearsal/<new-ts>` branch.

##### Flag-aware path (when you need `--manual-agents`, alt config, …)

The rehearsal script forwards `--auto` and `--headless` but nothing else. For anything else (`--manual-agents`, `--show-prompt`, alt `-c` config, etc.), drive the worktree by hand:

```bash
# Step 1 — go to shop
cd ../shop

# Step 2 — build gh-optivem from your local working copy
go -C ../gh-optivem build -o gh-optivem.exe .

# Step 3 — create a throwaway worktree on a rehearsal branch, under a
#          `worktrees/` folder beside academy (outside academy on purpose
#          — see scripts/atdd-rehearsal.sh for the repo-locator reason).
TS=$(date +%Y%m%d-%H%M%S)
mkdir -p ../../worktrees
git worktree add -b "rehearsal/${TS}" "../../worktrees/rehearsal-${TS}"
cd "../../worktrees/rehearsal-${TS}"

# Step 4 — run implement with whatever flags you need (pick one)
../../academy/gh-optivem/gh-optivem.exe implement --issue 42
../../academy/gh-optivem/gh-optivem.exe --auto implement --issue 42 --headless                  # fully autonomous (script also supports this)
../../academy/gh-optivem/gh-optivem.exe implement --issue 42 --manual-agents
../../academy/gh-optivem/gh-optivem.exe -c ./gh-optivem.experimental.yaml implement --issue 42  # swap node_extras / task_prompts / process_flow via an alternate config

# Step 5 — clean up the worktree + branch when done
cd ../../academy/shop
git worktree remove --force "../../worktrees/rehearsal-${TS}"
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
    --backend-lang dotnet --frontend-lang typescript --test-lang typescript \
    --project-url https://github.com/orgs/valentinajemuovic/projects/N \
    --backend-path backend --frontend-path frontend \
    --system-test-path system-test

# Step 4 — scaffold a fresh project (no --shop-ref → uses the baked-in SHA)
gh optivem init

# Step 5 — walk an issue on the new repo
cd page-turner
gh optivem implement --issue 1

# Step 5 — (optional) remove the extension
gh extension remove optivem
```

Use this to reproduce a user-reported issue against the same binary they're running, or to smoke-test what an external user gets right after a release.

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

## Project config (`gh-optivem.yaml`)

Every scaffolded repo gets a `gh-optivem.yaml` at its root. The file declares five top-level keys:

- `project:` — the GitHub Projects board URL.
- `repo-strategy:` — `mono-repo` or `multi-repo`.
- `system:` — the system being built. Polymorphic by architecture: under `monolith`, `system:` carries flat `path:` / `repo:` / `lang:` directly; under `multitier`, it nests `backend:` and `frontend:` blocks (each with its own per-component language).
- `system-test:` — the acceptance-test suite that drives the system. Top-level (not nested under `system:`) because tests aren't part of the system; they drive it.
- `external-systems:` (optional) — a name-keyed map of stand-ins for third-party dependencies, one entry per external system. Each entry declares `real-kind:` (`test-instance` | `simulator` — what backs its contract-real suite), an always-present `stub:` block (the cycle-2 WireMock-style stand-in), and — iff `real-kind: simulator` — a `simulator:` block (the cycle-3 real-sim we author). `gh optivem init` does not scaffold this map; operators add entries by hand.

Every populated tier carries the same `path:` (repo-relative) and `repo:` (slug from the participating repos) pair; system-tier blocks additionally carry `lang:`. The runtime preflight on `gh optivem implement` validates that every declared path exists on disk before any agent runs, so a config / layout mismatch fails fast with a readable error rather than mid-pipeline.

For the canonical schema, see [`internal/projectconfig/config.go`](internal/projectconfig/config.go) — every YAML field is declared on the `Config` struct with its `yaml:` tag, and the `Validate` method spells out the cross-field rules (architecture exclusivity, repo-strategy consistency, per-tier completeness, SonarCloud presence).

### Pointing at non-default configs

`gh-optivem.yaml` is the single entry point for every `gh optivem` command — there is no default-name fallback for `systems.yaml` / `tests.yaml`. Three knobs decide *which* `gh-optivem.yaml` the tool reads, in ascending order of precedence — each overrides the one below:

```bash
# 1. One-shot flag (highest precedence) — selects which gh-optivem.yaml to read
gh optivem -c ./gh-optivem.shop-monolith.yaml test run

# 2. Shell-session env var (same role as --config)
export GH_OPTIVEM_CONFIG=./gh-optivem.shop-monolith.yaml
gh optivem test run

# 3. Default location: ./gh-optivem.yaml in the current working directory
gh optivem test run
```

Inside the selected `gh-optivem.yaml`, `system.config:` / `system-test.config:` point at the actual systems/tests config files:

```yaml
system:
  config: docker/systems.yaml
system-test:
  config: system-test/tests.yaml
```

Legacy `.json` files still work — the loader picks the parser from the file extension, and any in-flight repo carrying `systems.json` / `tests.json` keeps loading without changes.

`gh optivem init` auto-populates `system.config:` / `system-test.config:` to the paths it produces, so freshly scaffolded repos work without any flags. `gh optivem config init` (hand-rolled repos) leaves both fields empty — add them before invoking the runner commands.

If no `gh-optivem.yaml` is found, the runner commands hard-error with a hint pointing at `gh optivem config init` (to create one in place) and at `--config <path>` (to use one that lives elsewhere). If `gh-optivem.yaml` is present but `system.config:` / `system-test.config:` is unset, the runner commands hard-error pointing at the missing field plus the same `--config` escape hatch.

#### Pipeline overrides

The implementation pipeline (`gh optivem implement`, with or without `--issue`) reads four optional override fields from the same `gh-optivem.yaml`:

```yaml
process_flow: config/process-flow.yaml         # alternate process-flow YAML (default: embedded)
task_prompts:                                   # swap one or more embedded MID task prompts
  write-acceptance-tests: config/prompts/write-acceptance-tests.md
node_extras:                                    # appended to a node's prompt at dispatch
  AT_RED_DSL_WRITE: prefer record types
node_replacements:                              # replaces a node's prompt verbatim with this file body
  AT_RED_TEST_WRITE: config/prompts/at-red-test-write.md
```

All four fields are optional; absent means "use the embedded default." To experiment without committing a change to the project's `gh-optivem.yaml`, copy it to a side file and pass `--config ./gh-optivem.experimental.yaml`. There is no per-invocation flag for any of these — they are project-stable values by design.

## How it works

See [docs/how-it-works.md](docs/how-it-works.md) for a detailed walkthrough of the `main.go` logic, setup steps, and verification levels.

For the ATDD pipeline orchestration view, see the rendered [process diagram](docs/process-diagram.md). It is regenerated automatically whenever the canonical YAML at `internal/atdd/process/process-flow.yaml` changes; do not edit the diagram by hand.

### Adding a y/n confirmation prompt

Every new yes/no confirmation must go through `internal/kernel/approval` with a category tag, not through `promptio.ConfirmYN` directly:

```go
ok, err := approval.Confirm(cmdctx.Approval(cmd), approval.CategoryPrompt, os.Stdin, os.Stdout, "Proceed?")
```

This routes the prompt through the `--auto` / `--confirm` policy resolved once in `PersistentPreRunE` (see [Auto-approve](README.md#auto-approve) for the user-facing contract). Pick the category that matches what the prompt gates: `CategoryCommit`, `CategoryFix`, `CategoryRelease`, `CategoryHuman`, or `CategoryPrompt` (the low-stakes catch-all). New `promptio.ConfirmYN` / `ConfirmYNVia` call sites that bypass `approval.Confirm` are a review-block — they read like unattended-mode bugs the next time someone runs `--auto`.

## Releasing

This project uses [semantic versioning](https://semver.org/).

```bash
git tag v1.2.3
git push origin v1.2.3
```

Triggers the Release workflow (GoReleaser builds binaries for all platforms and publishes a GitHub Release). Users on `gh extension install optivem/gh-optivem` get the new version on their next `gh extension upgrade`.
