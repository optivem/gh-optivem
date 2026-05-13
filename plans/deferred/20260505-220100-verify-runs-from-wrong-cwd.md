# `verify_run_tests_after_driver` runs `gh optivem test run` from the wrong cwd

## Symptom

Choosing `[t]racer` (or `[r]` / `[f]`) at the verify prompt produces:

```
$ gh optivem test run --suite acceptance-api
(test run failed: shell "gh optivem test run --suite acceptance-api":
  exit status 1
  (stderr: ERROR: read system config ./system.json: open ./system.json:
   The system cannot find the file specified.) — continuing)
```

Repeats for `acceptance-ui` and `contract-stub`. Verification is a no-op
on every cycle until this is fixed.

## Root cause

`runVerifyCommand` (`internal/atdd/runtime/actions/bindings.go:910`)
shells out via `a.deps.Shell.Run`. The production `realShell.Run`
(`bindings.go:1452`) builds an `exec.Command` without setting `cmd.Dir`,
so the command inherits the orchestrator's working directory — which is
the repo root, not the directory holding `system.json`.

`gh optivem test run` defaults to `--system-config ./system.json` and
`--test-config ./tests.json` (see `runner_commands.go:38`). For the shop
template these files live at:

- `docker/<lang>/<arch>/system.json` (e.g. `docker/typescript/monolith/system.json`)
- `system-test/<lang>/tests-latest.json`

After scaffolding's path-flattening (per `apply_template.go:90`) the
generated repos use:

- `docker/system.json`
- `system-test/tests-latest.json`

In neither layout is `system.json` at the repo root, so `./system.json`
fails 100% of the time.

## Two viable fixes

### Option A. Pass explicit `--system-config` / `--test-config` flags

Build the command line with the paths the orchestrator already knows from
the issue / intake (language, arch). The verify node would emit:

```
gh optivem test run --suite <s> --test <t> \
  --system-config <docker_dir>/system.json \
  --test-config <test_dir>/tests-<phase>.json
```

The `<phase>` selection (latest vs legacy) and the docker/test dirs are
already encoded in `internal/steps/replacements_test.go` and read by
the existing acceptance-stage workflow templates — there's an authoritative
mapping to reuse.

**Pros:**

- Self-documenting: the printed command is a copy-paste-runnable invocation.
- Doesn't change shell semantics; cwd is still the repo root for any other
  side-effect the command relies on (e.g. `git` lookups inside the runner).

**Cons:**

- Path inference must live in the verify node. Either duplicate the
  layout heuristic from `apply_template.go` (bad) or extract it to a
  shared helper (better, more code).
- The selector itself is language-aware (`testselect.layout`); the
  ChangedMethod's `Lang` field is already there. But arch
  (`monolith` vs `multitier`) is not, and one is needed when the path
  isn't flattened.

### Option B. Run the command with `cmd.Dir` set to the system-test working dir

Add a `WorkingDir` field to `ShellRunner.Run` (or a parallel `RunIn`
method) and have the verify node call `RunIn(systemTestDir, cmd)`.
`systemTestDir` is the directory containing `system.json`.

**Pros:**

- Smaller code surface: one path inference site, one new shell method.
- Matches how a human runs the tests today (cd into the directory first).
- Doesn't rely on the runner accepting flags that may change later.

**Cons:**

- Path inference still needed.
- Affects only verify; other shell users (`compile-all.sh`, `test-all.sh`)
  still inherit repo-root cwd, which is what they want — so the new
  method must be additive, not a replacement.

### Recommended

**Option A.** The runner already exposes the flags as the supported way
to point it at non-default config paths (see `README.md:146`). Using
flags keeps the verify command line printable and reproducible — the
user can copy what they see in the orchestrator output and run it in a
shell to debug. With Option B, the printed `$ gh optivem test run ...`
line is misleading because the cwd context is invisible.

Lean: ship Option A. If the path-inference helper grows hairy, fall back
to Option B for v1.

## Path inference — what to reuse

A scaffolded student repo always has:

- `docker/system.json` (path-flattened from `docker/<lang>/<arch>/`).
- `system-test/tests-latest.json` and `system-test/tests-legacy.json`
  (path-flattened from `system-test/<lang>/`).

The shop template (under `templates/shop/`) keeps the original layout:

- `docker/<lang>/<arch>/system.json`.
- `system-test/<lang>/tests-{latest,legacy}.json`.

The verify node needs to choose which it's looking at. Two signals:

1. If `docker/system.json` exists at `repoRoot` → flattened layout.
2. Otherwise expect the templated layout; the lang comes from the
   ChangedMethod, the arch comes from `system.json`'s presence under
   `docker/<lang>/monolith/` vs `docker/<lang>/multitier/` (probe both).

Encapsulate this in a small helper in
`internal/atdd/runtime/actions/` (or move to `internal/runner/` if it
grows beyond 30 lines), exposing:

```go
func ResolveSystemTestPaths(repoRoot, lang string) (systemConfig, testConfig string, err error)
```

and call it from `verifyRunTestsAfterDriver` once per run, then thread
both paths into every `runVerifyCommand` call.

## Phase-aware test config

`tests-latest.json` is what runs in CI's GREEN gate. `tests-legacy.json`
is the warm-rerun shortcut. The verify node post-WRITE wants `latest`
because WRITE phases are by definition not yet in the legacy set. No
phase-aware switching needed for v1; pin to `tests-latest.json`.

## Items

### 1. Path-inference helper

**File (new):** `internal/atdd/runtime/actions/verify_paths.go`

`ResolveSystemTestPaths(repoRoot, lang string) (systemConfig,
testConfig string, err error)`. Probe order:

- `docker/system.json` + `system-test/tests-latest.json` (flat).
- `docker/<lang>/monolith/system.json` + `system-test/<lang>/tests-latest.json`.
- `docker/<lang>/multitier/system.json` + `system-test/<lang>/tests-latest.json`.

Returns the first pair where both files exist. Returns an error when
none match — the caller logs the error and skips the suite, rather than
running with broken paths.

### 2. Wire flags into verify commands

**File:** `internal/atdd/runtime/actions/bindings.go`

In `verifyRunTestsAfterDriver`, after `Select`, resolve the paths once
per language present in `res.Changed`. Pass them into:

- `runTracerVerify` via a new struct param (avoid a long signature).
- `runAffectedSetVerify` via the same struct.

Both functions append `--system-config <path> --test-config <path>` to
each `gh optivem test run ...` command they build. `shellEscape`
already handles spaces.

### 3. Surface a clear error when paths can't be resolved

**File:** same as Item 2.

When `ResolveSystemTestPaths` fails, print:

```
verify_run_tests_after_driver: could not locate system.json/tests-latest.json
for lang=<lang> under <repoRoot> — skipping verification.
Tried: docker/system.json, docker/<lang>/monolith/system.json,
docker/<lang>/multitier/system.json
```

And return `Outcome{}` (no test run, but don't halt the state machine —
verification is feedback, not a gate).

### 4. Tests

**File:** `internal/atdd/runtime/actions/verify_paths_test.go`

Table-driven tests for `ResolveSystemTestPaths`:

- Flat layout (only `docker/system.json` exists) → returns flat paths.
- Templated monolith → returns `docker/<lang>/monolith/...`.
- Templated multitier → returns `docker/<lang>/multitier/...`.
- Neither → returns error.

**File:** `internal/atdd/runtime/actions/bindings_test.go`

Extend the existing `verifyRunTestsAfterDriver` tests to assert that the
shell command captured by `fakeShell` contains the expected
`--system-config` and `--test-config` flags.

### 5. Documentation

**File:** `internal/atdd/runtime/agents/prompts/atdd-driver.md` (and any
other prompt that surfaces the verify behaviour) — note that the verify
node now self-resolves config paths, so students don't need to set
anything.

## Out of scope

- **Tracer staging fix.** The `inputSku` change is unmapped because of a
  bridge bug in `testselect`, not because of the cwd issue. Sibling
  plan: `20260505-220000-tracer-bridge-page-object-helpers.md`.
- **In-process test-runner integration.** The original plan
  (`20260504-130000-...md`) flagged shelling out to `gh optivem test
  system` as wasteful and asked whether an in-process entry point
  exists. That tradeoff is unchanged; the cwd fix doesn't depend on
  resolving it.
- **Phase-aware `tests-legacy.json` vs `tests-latest.json` selection.**
  Always use `tests-latest.json` for verify. Legacy is only meaningful
  inside the warm rerun loop, not the WRITE-phase gate.
- **Repos with non-standard layouts.** If a project drifts from the
  scaffolded conventions, `ResolveSystemTestPaths` returns an error and
  verify is a no-op for that run. A configurable override is a v2
  feature.

## Order of operations

1. Land Item 1 (path helper) + Item 4 (helper tests) together. Pure
   function, no external dependencies, easy to land in isolation.
2. Land Item 2 (wire flags) + Item 4 (binding tests) + Item 3 (error
   surface) in the same PR.
3. Land Item 5 (docs) in the same PR as Item 2.
4. **Manual rehearsal:** rerun the failing cycle from this morning. Edit
   one Page Object method, choose `[t]racer`, observe `gh optivem test
   system --system-config docker/typescript/monolith/system.json
   --test-config system-test/typescript/tests-latest.json --suite ...
   --test ...` actually executes and reports pass/fail.
