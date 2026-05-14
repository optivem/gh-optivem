# Plan: preflight language-conditional compiler checks (npm / dotnet / java)

## Context

On 2026-05-14 the user ran `manual-test.sh` against a fresh scaffold and the
run failed 19 seconds in, at Phase 6/11 "Verify local":

```
FATAL: Compilation failed for system source in /tmp/scaffold-3276036955/repo/system:
  compile (typescript) "npm ci" in /tmp/scaffold-3276036955/repo/system:
  exec: "npm": executable file not found in $PATH
```

The TypeScript compile path in `internal/compiler/compiler.go:137` shells out to
`npm ci && npx tsc --noEmit`. When `npm` is missing the failure surfaces 19 s
into the run instead of in the Phase 2 preflight, which is where every other
"your environment is wrong" failure surfaces today via `VerifyEnvironment()`
(`internal/config/token_auth.go:254`).

`environment verify` (the user-facing CLI surface for this preflight,
`environment_commands.go:78`) currently checks only the language-agnostic
tools — `gh` CLI auth and `actionlint`
(`internal/config/tool_checks.go:26,46`). The per-language compilers
(`npm` for typescript, `dotnet` for dotnet, `java` for the JVM that
`gradlew.bat` invokes) are not checked at all.

This plan adds language-conditional compiler checks in two call sites:

1. **`init`'s Phase 2 preflight** — automatic; uses the langs resolved from
   `--monolith-lang` / `--backend-lang` / `--frontend-lang` / `--test-lang`.
   Fails in <1 s with a readable "npm not found, install: …" instead of 19 s
   into Phase 6 with a raw exec error.
2. **`environment verify --lang <list>`** — opt-in; accepts a comma-separated
   list of languages so a CI preflight job can validate one matrix combination
   without scaffolding anything.

Per `feedback_interactive_validation_parity.md` the two paths must share one
validator. Per `feedback_always_online_no_offline_flags.md` (the toolchain
analog: assume the operator has the compilers; fail fast and aggregate) all
missing tools should be reported in one pass, alongside missing env vars and
auth failures.

## Critical files

- `internal/config/tool_checks.go` — current home of `verifyGhAuth` and
  `verifyActionlint`. New `verifyNpm` / `verifyDotnet` / `verifyJava` helpers
  go here, plus a `compilerChecksFor(langs []string)` dispatcher.
- `internal/config/token_auth.go:254` — `VerifyEnvironment()` orchestrator.
  Gets a `langs []string` parameter; when non-nil/empty, appends the
  per-language compiler checks to the existing parallel check slice so they
  run with the same aggregation and timeout semantics.
- `internal/config/config.go:994` — `ParseAndValidate`'s Phase 2 call to
  `VerifyEnvironment()`. Becomes `VerifyEnvironment(collectLangs(lc))` where
  `lc` is the `langChoice` returned by `resolveLangs(f)` a few lines above.
- `environment_commands.go:78` — `newEnvironmentVerifyCmd`. Gains a
  `--lang` `StringSliceVar` and passes it through.
- `internal/compiler/compiler.go:130` — the source of truth for
  `(lang → command)`. The new compiler-check dispatcher should reference
  the same `projectconfig.Lang*` constants used there so adding a fourth
  language touches exactly one map.

## Reuse references

- `internal/config/tool_checks.go:26-39` — `verifyGhAuth` shape:
  `exec.LookPath` + actionable install hint. New compiler checks mirror this
  one-for-one.
- `internal/config/tool_checks.go:46-52` — `verifyActionlint` shape: the
  no-auth variant (just LookPath). `verifyNpm` / `verifyDotnet` / `verifyJava`
  are this shape — we don't `npm --version` etc. because the binary's
  presence on PATH is the only thing `compiler.go` cares about; running
  `--version` would add 50–200 ms per language for no extra signal.
- `internal/config/token_auth.go:274-310` — the `[]check` slice + parallel
  goroutine fan-out. The compiler checks slot into this slice; no new
  scheduling code.
- `internal/config/token_auth.go:325-345` — aggregated error builder.
  Reused as-is; compiler-check failures land in the same "Verification
  failed for N check(s)" block, distinguishable from token failures by
  their name (`npm`, `dotnet`, `java` vs. `DOCKERHUB_TOKEN`, etc.).
- `projectconfig.LangDotnet` / `LangJava` / `LangTypescript` constants
  (already referenced in `internal/compiler/compiler.go:73-75`) — use the
  same constants in the dispatcher.

## Out of scope

- **Version checks.** "Is `npm` ≥ 9?" is the next thing someone will ask
  for; this plan deliberately ships presence-only. The compile step itself
  will fail with a clear toolchain error if the version is too old, and
  the matrix of "what version works with what" is `package.json`-driven
  per-scaffold. Add later if there's evidence of a real failure mode.
- **Auto-install fallback.** The install hints are URLs/commands, not
  invocations. No `winget install nodejs` or `apt-get install`.
- **Reading langs from `gh-optivem.yaml` for `environment verify`.** The
  command stays explicit-args-only — the operator passes `--lang` or
  gets the language-agnostic check. Auto-reading the yaml would couple
  `environment verify` to the project-config schema and to cwd state,
  for marginal benefit (the CI preflight already knows its matrix combo).
- **Changing `compiler.go`'s command tables.** The compile sequences are
  fine as-is; this plan only adds upstream checks.
- **Per-component checks for `actionlint` / `gh`.** Those remain
  unconditional — every scaffold path needs them.

## Steps

### 1. Add compiler-presence helpers in `internal/config/tool_checks.go`

Three new functions, each a near-copy of `verifyActionlint`. Order
matters only for readability; the parallel runner doesn't care.

```go
// verifyNpm checks that the npm binary is on PATH. Required for the
// TypeScript compile sequence (internal/compiler/compiler.go), which runs
// `npm ci && npx tsc --noEmit` against the tier cwd.
func verifyNpm() error {
    if _, err := exec.LookPath("npm"); err != nil {
        return errors.New("npm not found on PATH.\n    " +
            "Install Node.js (bundles npm): https://nodejs.org/")
    }
    return nil
}

// verifyDotnet checks that the dotnet binary is on PATH. Required for the
// .NET compile sequence (`dotnet build`).
func verifyDotnet() error {
    if _, err := exec.LookPath("dotnet"); err != nil {
        return errors.New("dotnet not found on PATH.\n    " +
            "Install the .NET SDK: https://dotnet.microsoft.com/download")
    }
    return nil
}

// verifyJava checks that the java binary is on PATH. Required for the Java
// compile sequence — gradlew.bat (in-repo) shells out to whatever java is
// resolved from PATH / JAVA_HOME.
func verifyJava() error {
    if _, err := exec.LookPath("java"); err != nil {
        return errors.New("java not found on PATH.\n    " +
            "Install a JDK (Temurin recommended): https://adoptium.net/")
    }
    return nil
}
```

Then a single dispatcher that maps the resolved-langs list to a slice of
`(name, fn)` entries — same shape as the `check` struct in
`token_auth.go:274` so we can append directly:

```go
// compilerChecksFor returns the local-tool checks required for the given
// set of languages. Duplicates in langs are deduped — passing
// ["typescript", "typescript", "java"] returns one npm check and one java
// check. Unknown language strings are silently skipped (the language-flag
// validators run earlier and reject anything outside the known set).
func compilerChecksFor(langs []string) []struct {
    Name string
    Fn   func() error
} {
    seen := map[string]bool{}
    var out []struct {
        Name string
        Fn   func() error
    }
    for _, l := range langs {
        if seen[l] {
            continue
        }
        seen[l] = true
        switch l {
        case projectconfig.LangTypescript:
            out = append(out, struct{Name string; Fn func() error}{"npm", verifyNpm})
        case projectconfig.LangDotnet:
            out = append(out, struct{Name string; Fn func() error}{"dotnet", verifyDotnet})
        case projectconfig.LangJava:
            out = append(out, struct{Name string; Fn func() error}{"java", verifyJava})
        }
    }
    return out
}
```

(If the anonymous struct literal proves ugly, lift it to a named
`compilerCheck` type alongside `check` in `token_auth.go` and share. Up
to taste — the parallel runner only reads the two fields.)

Add a `tool_checks_test.go` if one doesn't exist with one table-driven test
per dispatcher path: empty list → empty result, all three langs → three
results, duplicates deduped, unknown lang ignored. Skip testing the actual
`exec.LookPath` calls — that's a Go stdlib smoke test, not our logic.

### 2. Wire compiler checks into `VerifyEnvironment`

Change the signature in `internal/config/token_auth.go:254` from

```go
func VerifyEnvironment() error {
```

to

```go
// VerifyEnvironment runs every preflight check the gh-acceptance pipeline
// depends on: token presence, live token auth against each provider, and
// local tools (gh CLI, actionlint, plus the per-language compilers in
// langs). langs may be nil/empty — in that case only the
// language-agnostic checks run (gh, actionlint, tokens), which is what
// `gh optivem environment verify` (no --lang) does.
func VerifyEnvironment(langs []string) error {
```

In the body, after the existing `checks := []check{...}` for gh + actionlint
(line ~279), append the compiler checks:

```go
for _, cc := range compilerChecksFor(langs) {
    checks = append(checks, check{cc.Name, cc.Fn})
}
```

No change needed to the goroutine fan-out, result aggregation, or success
logging — the new entries flow through the same path. On success the user
sees `  npm: valid`, `  dotnet: valid`, etc. alongside `  gh CLI auth: valid`.

### 3. Pass resolved langs from `init`'s Phase 2 preflight

In `internal/config/config.go:994`, replace

```go
if err := VerifyEnvironment(); err != nil {
    log.FatalExit(err.Error())
}
```

with

```go
if err := VerifyEnvironment(collectLangs(lc)); err != nil {
    log.FatalExit(err.Error())
}
```

Add `collectLangs` as a small helper in the same file (near `resolveLangs`),
private to the package:

```go
// collectLangs flattens a langChoice into the unique non-empty languages
// the scaffold will compile. The slice is the input to
// compilerChecksFor — order doesn't matter (the dispatcher dedupes
// internally).
func collectLangs(lc langChoice) []string {
    var out []string
    for _, l := range []string{lc.lang, lc.backendLang, lc.frontendLang, lc.testLang} {
        if l != "" {
            out = append(out, l)
        }
    }
    return out
}
```

`resolveLangs` already validates the values, so by the time we get here every
non-empty entry is one of `java`/`dotnet`/`typescript`.

Add a single `config_test.go` test covering `collectLangs`: monolith-typescript,
multitier-dotnet-typescript-typescript (dedupe check), empty langChoice → nil.

### 4. Update the `environment verify` no-arg call site

In `environment_commands.go:110` change

```go
if err := config.VerifyEnvironment(); err != nil {
```

to

```go
if err := config.VerifyEnvironment(langs); err != nil {
```

where `langs` is the value of the new `--lang` flag (Step 5). For the
no-flag case this is `nil`, which is exactly the existing behaviour.

### 5. Add `--lang` to `gh optivem environment verify`

In `newEnvironmentVerifyCmd()` (`environment_commands.go:78`):

```go
var langs []string
cmd := &cobra.Command{
    Use:   "verify",
    Short: "Verify the local environment is ready to run the gh-acceptance pipeline",
    Long:  `…  --lang java,typescript  also checks per-language compilers (npm/dotnet/java) …`,
    Example: `  gh optivem environment verify
  gh optivem environment verify --lang typescript
  gh optivem environment verify --lang typescript,dotnet,java`,
    Args: cobra.NoArgs,
    Run: func(cmd *cobra.Command, args []string) {
        // … existing log.Init …
        if err := config.VerifyEnvironment(langs); err != nil {
            …
        }
    },
}
cmd.Flags().StringSliceVar(&langs, "lang", nil,
    "Languages to check compilers for: java, dotnet, typescript "+
        "(comma-separated or repeated). Omit to check only language-agnostic tools.")
return cmd
```

`StringSliceVar` accepts both `--lang typescript,java` and `--lang typescript --lang java`
out of the box — no custom parser needed.

Add a sanity validator that runs before `VerifyEnvironment`: reject any
value not in `{java, dotnet, typescript}` with a clear message. Mirror
`ValidateBackendLang` in `internal/config/config.go:436` rather than
duplicating the set — extract a tiny `IsValidLang(string) bool` in
`config.go` and call it from both `ValidateBackendLang` and the new
flag-validation loop. Per `feedback_interactive_validation_parity.md`,
one source of truth.

### 6. Update the long-form help text on `environment verify`

In `environment_commands.go:82-97` (the `Long:` field), add lines for the
new compiler-check section under the existing table. Match the surrounding
two-column shape exactly (column-aligned with the token entries):

```
  npm                 — required when --lang includes typescript
  dotnet              — required when --lang includes dotnet
  java                — required when --lang includes java
```

Also update the closing paragraph to mention that the compiler checks only
run when `--lang` is passed.

### 7. Verify end-to-end on Windows (the user's platform)

From `gh-optivem/` on the user's Windows / Git Bash setup, with `npm`
temporarily renamed out of PATH (e.g. `mv $(which npm) $(which npm).bak`
on WSL or `Rename-Item` on the Node install dir):

```bash
gh optivem environment verify --lang typescript
```

Expected: exits non-zero in <1 s with

```
Verification failed for 1 check(s):

  npm: npm not found on PATH.
    Install Node.js (bundles npm): https://nodejs.org/
```

Restore `npm` and re-run:

```bash
gh optivem environment verify --lang typescript,dotnet,java
```

Expected: every check line prints, exit 0.

Then re-run the original failing command:

```bash
bash scripts/manual-test.sh --owner valentinajemuovic --system-name "Page Turner" \
    --arch multitier --repo-strategy multirepo \
    --backend-lang dotnet --frontend-lang typescript --test-lang typescript \
    --shop-ref main
```

With `npm` removed, this must now fail in Phase 2 ("Verifying environment…"),
not Phase 6 ("Verify local"). Restore `npm` and confirm the full flow runs
to completion.

### 8. Update CLAUDE.md / README only if a user-facing surface changed

The new `--lang` flag is the only user-facing addition. If `README.md`
documents the `environment verify` flag set, add the new flag there; if
not, the Cobra help text in Step 6 is sufficient.

## Verification

The plan is complete when:

1. Running `gh optivem init --backend-lang typescript …` with `npm`
   missing from PATH fails in <1 s during Phase 2, with the install-hint
   error, and never proceeds to clone the shop template.
2. `gh optivem environment verify` (no flag) still passes on a machine that
   has only `gh` + `actionlint`, no language compilers — i.e. the new
   checks remain strictly opt-in for the standalone surface.
3. `gh optivem environment verify --lang typescript,dotnet,java` runs all
   three compiler checks in parallel with the existing ones and aggregates
   any failures into the single "Verification failed for N check(s):" block.
4. Removing or renaming the `compilerChecksFor` dispatcher's case for an
   existing language causes the unit test added in Step 1 to fail —
   i.e. the dispatcher is the single source of truth that `init` and
   `environment verify --lang` both rely on.
