# Plan: preflight docker presence check in `environment verify` and `init`

## Context

On 2026-05-14 a user (`jasonribble`, optivem/gh-optivem#55) ran
`manual-test.sh` without `docker-compose` installed and the scaffolder
died 53 seconds in, at Phase 6/11 "Build system":

```
FATAL: Build system failed: build real: exit status 125
```

Same failure mode as the parallel compiler-check work
(`plans/20260514-0825-preflight-compiler-checks.md`) — a missing local
tool surfaces deep into the run instead of in Phase 2 preflight.

The fix shape is identical to the compiler-check plan: add a
presence-only check to `internal/config/tool_checks.go`, wire it into
`VerifyEnvironment()`, expose it via `gh optivem environment verify`.
Docker differs from the compilers only in its gating predicate:

- npm/dotnet/java are **language-conditional** — needed only if the
  scaffold compiles in that language.
- docker is **deploy-conditional** — needed whenever the scaffold's
  Build / Run / Stop / Clean steps run, which is every scaffold that
  uses `--deploy docker` (the default and currently the only working
  value; `--deploy cloud-run` is gated as in-development at
  `internal/config/config.go:595-598`).

So in practice **docker is unconditional today**. The plan still
parameterises the check by deploy target so the in-development cloud-run
path doesn't fail-fast on missing docker if/when it ships.

This plan is intentionally short — it extends the compiler-check work
pattern-wise. Read that plan first for the architectural reasoning; the
docker piece is mostly a copy of the same shape into one extra slot.

## Critical files

- `internal/config/tool_checks.go` — new `verifyDocker` helper next to
  `verifyGhAuth` / `verifyActionlint` (and, post-compiler-check-plan,
  next to `verifyNpm` / `verifyDotnet` / `verifyJava`).
- `internal/config/token_auth.go:254` — `VerifyEnvironment`. The
  compiler-check plan changes its signature to
  `VerifyEnvironment(langs []string)`. This plan piggybacks: extend to
  `VerifyEnvironment(langs []string, deploy string)` (or pass a small
  options struct — see Step 1) so the gating is symmetric.
- `internal/config/config.go:994` — `ParseAndValidate`'s Phase 2
  preflight. Pass `f.Deploy` alongside the langs.
- `environment_commands.go:78` — `newEnvironmentVerifyCmd`. Add a
  `--deploy` flag mirroring the `--lang` flag the compiler-check plan
  adds.
- `internal/compiler/compiler.go` is **not** touched. The compile path
  doesn't need docker; the Build path does.

## Reuse references

- `internal/runner/system.go:255-264` (`runCompose`) — the canonical
  invocation of `docker compose`. Confirms that `docker` is the binary
  on PATH the check should target (Compose v2 lives as a docker
  sub-command, not a separate `docker-compose` binary on modern setups).
- `internal/runner/system.go:294-300` (`runDocker`) — also targets
  `docker`; same binary name.
- `projectconfig.DeployDocker` constant (referenced from
  `internal/config/config.go:1232`) — the canonical "docker deploy"
  enum value to compare against.
- The compiler-check plan's Step 1 dispatcher
  (`compilerChecksFor(langs []string)`) — direct shape template for
  the deploy gating helper.

## Out of scope

- **Compose v1 detection.** Modern docker bundles Compose v2 as
  `docker compose`. If an operator is on Compose v1 (`docker-compose`),
  the `docker` binary check still passes but `runCompose` will fail
  at runtime with the stderr now-visible from the sibling
  visibility-fix plan. A separate compose-version check would buy
  little and add a noisy install-hint matrix.
- **`docker info` / daemon-running check.** The audit's M5 site
  (`dockerCapture` in `downOne`) shows the codebase already tolerates
  a missing daemon at runtime. Adding an `info` probe to preflight
  would add ~200 ms and could false-positive on a Docker-Desktop-in-Linux-mode
  transition, for low marginal value over "docker on PATH".
  Re-evaluate if a user reports a "docker installed but daemon
  stopped" failure that the visibility-fix plan doesn't sufficiently
  diagnose.
- **`--deploy cloud-run`.** That deploy target is gated as
  in-development. When it ships, this plan's gating already supports
  swapping docker for gcloud/equivalent.
- **Auto-install.** Hint only.
- **Version checks.** Presence only.

## Steps

### 1. Add `verifyDocker` in `internal/config/tool_checks.go`

Mirrors `verifyActionlint` one-for-one. Place adjacent to the new
`verifyNpm` / `verifyDotnet` / `verifyJava` from the compiler-check
plan (if that has landed) or at the bottom of the file (if not).

```go
// verifyDocker checks that the docker binary is on PATH. Required for
// every scaffold using --deploy docker (the default) — the local-verify
// lifecycle (Build / Up / Run tests / Down / Clean in internal/runner)
// shells out to `docker compose`. Compose v2 is a docker sub-command,
// so the `docker` binary alone is sufficient; legacy Compose v1
// (`docker-compose`) installs are not checked separately.
func verifyDocker() error {
    if _, err := exec.LookPath("docker"); err != nil {
        return errors.New("docker not found on PATH.\n    " +
            "Install Docker Desktop (macOS/Windows): https://www.docker.com/products/docker-desktop\n    " +
            "Install Docker Engine (Linux): https://docs.docker.com/engine/install/")
    }
    return nil
}
```

### 2. Add a deploy-gating helper next to `compilerChecksFor`

In `internal/config/tool_checks.go` (or wherever the compiler-check
plan landed `compilerChecksFor`):

```go
// deployChecksFor returns the local-tool checks required for the given
// deploy target. Empty deploy string returns nil — callers that don't
// know the deploy target skip the docker check (same idiom as
// compilerChecksFor with an empty langs slice).
func deployChecksFor(deploy string) []struct {
    Name string
    Fn   func() error
} {
    switch deploy {
    case projectconfig.DeployDocker:
        return []struct{Name string; Fn func() error}{
            {"docker", verifyDocker},
        }
    }
    return nil
}
```

(If `compilerChecksFor` landed with a named `compilerCheck` type, reuse
it here verbatim. Same shape, same slot.)

### 3. Wire into `VerifyEnvironment`

Two options for the signature, depending on what the compiler-check plan
landed:

**Option A — extend the slice + scalar pattern:**

```go
func VerifyEnvironment(langs []string, deploy string) error
```

**Option B — collect into a struct:**

```go
type EnvironmentChecks struct {
    Langs  []string
    Deploy string
}

func VerifyEnvironment(opts EnvironmentChecks) error
```

Option B scales better if a third axis appears (e.g. arch-conditional
toolchains); Option A is the smaller diff today. Pick whichever the
compiler-check plan picked — keep the two plans in step rather than
ping-ponging the signature twice. Default recommendation: **Option B**
if both plans land in the same release; Option A if the compiler plan
has already landed with the `[]string` signature.

In the body, after the compiler checks are appended, append the deploy
checks the same way:

```go
for _, dc := range deployChecksFor(opts.Deploy) {
    checks = append(checks, check{dc.Name, dc.Fn})
}
```

The aggregated-failure output path is reused as-is — a missing docker
now appears in the same "Verification failed for N check(s):" block as
a missing npm.

### 4. Pass deploy from `init`'s Phase 2 preflight

In `internal/config/config.go:994`, expand the call to include
`f.Deploy` (passed via whichever signature Step 3 picked). `f.Deploy`
is already populated and validated by `validateCommonFlags` upstream
of this point, so by the time the preflight runs, the value is one
of `"docker"` or `"cloud-run"`.

### 5. Add `--deploy` to `gh optivem environment verify`

Mirror the compiler-check plan's `--lang` flag:

```go
var deploy string
cmd.Flags().StringVar(&deploy, "deploy", "",
    "Deploy target to check tools for: docker. Omit to skip the deploy-conditional check.")
```

Validate the value before passing to `VerifyEnvironment`: must be
empty or one of `projectconfig.DeployDocker` / `projectconfig.DeployCloudRun`.
Reuse `projectconfig.IsValidDeploy` (already exists per
`internal/config/config.go:1234`).

Example usages to add to `Example:`:

```
  gh optivem environment verify
  gh optivem environment verify --lang typescript --deploy docker
  gh optivem environment verify --deploy docker
```

### 6. Update the long-form help text

In `environment_commands.go`'s `Long:` field, add a `docker` row to the
table the compiler-check plan introduced:

```
  docker              — required when --deploy is docker
```

### 7. Verify end-to-end

From `gh-optivem/`, with docker removed from PATH (or daemon stopped is
**not** sufficient — the check is presence-only; the visibility-fix
plan handles "installed but not running"):

```bash
gh optivem environment verify --deploy docker
```

Expected: <1 s exit non-zero, install hint visible, no scaffolding
attempted.

Then run the original failing scenario with docker missing:

```bash
bash scripts/manual-test.sh --owner valentinajemuovic --system-name "Page Turner" \
    --arch multitier --repo-strategy multirepo \
    --backend-lang dotnet --frontend-lang typescript --test-lang typescript \
    --shop-ref main
```

Expected: fail in Phase 2 ("Verifying environment…"), never reach
Phase 6. Restore docker and re-run to confirm the full flow proceeds.

### 8. Add unit test for `deployChecksFor`

Same shape as the compiler-check plan's `compilerChecksFor` test:
empty deploy → nil, `"docker"` → one entry named `"docker"`,
`"cloud-run"` → nil (today), unknown string → nil.

## Verification

The plan is complete when:

1. `gh optivem init --deploy docker …` (the default) with docker
   missing from PATH fails in <1 s during Phase 2 with the install-hint
   error, alongside any other missing tools / env vars in one
   aggregated report.
2. `gh optivem environment verify` (no flags) still passes when only
   `gh` and `actionlint` are installed — docker check remains
   opt-in for the standalone surface.
3. `gh optivem environment verify --deploy docker` runs the docker
   presence check alongside the existing gh/actionlint/token checks
   and aggregates failures into a single block.
4. `deployChecksFor` is the single source of truth for "what tools
   does deploy X need", same as `compilerChecksFor` is for languages —
   adding cloud-run later means adding one case to the dispatcher,
   nothing else.
