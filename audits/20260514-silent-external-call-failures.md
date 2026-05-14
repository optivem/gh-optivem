# Silent external-call failures — audit report

Date: 2026-05-14
Scope: `internal/**/*.go`, `main.go`, and the top-level `*_commands.go` files.
Read-only audit. No code changes made.

---

## TL;DR

The seed bug is **`internal/runner/system.go:257-264` — `runCompose`**. It
streams docker-compose's stdio to `os.Stdout/Stderr` but the returned error is
the bare `cmd.Run()` exit error with no captured output, so when the FATAL line
"Build system failed: build real: exit status 125" is finally printed
(`internal/steps/verify.go:357`) it carries zero context about *why* exit 125
happened. The sibling `runComposeCtx` already does this correctly — it tees to
a `tailWriter` and folds the last 16 KB into the returned error. `runCompose`
needs the same teeing.

The same class of bug exists in `internal/runner/system.go:runDocker` (line
294) and `internal/runner/tests.go:runShell` (line 233) and is propagated to
the user through `internal/compiler/compiler.go:passthroughShell.Run` (via
`shell.RunPassthrough`, line 145 in `internal/shell/github.go`). Together
these cover the entire local-verify lifecycle (Build, Start, Run tests, Stop,
Clean, Compile system, Compile tests, Setup tests).

For comparison, the runtime-side packages (`atdd/runtime/...`) and the
`*.run` helpers in `actions/bindings.go` all do this correctly — they
capture stderr into a `bytes.Buffer` and inline it into the returned
error message. The seed bug is concentrated in the local-verify lifecycle,
not the broader codebase.

---

## Seed bug — full trace

**Symptom (user-visible):**

```
> [11:34:00] Building system...
FATAL: Build system failed: build real: exit status 125
FAIL Step failed: Build system -- Build system failed: build real: exit status 125
```

**Root cause:**

The call chain is

1. `internal/steps/verify.go:356` — `runner.Build(loadSys(cfg), dockerDir(cfg), runner.BuildOptions{})`
2. `internal/runner/system.go:113` — `runCompose(cwd, args...)`
3. `internal/runner/system.go:257-264` — `runCompose`:

```go
func runCompose(cwd string, args ...string) error {
    full := append([]string{"compose"}, args...)
    cmd := exec.Command("docker", full...)
    cmd.Dir = cwd
    cmd.Stdout = os.Stdout
    cmd.Stderr = os.Stderr
    return cmd.Run()
}
```

`cmd.Run()` returns only the exit error (e.g. `*exec.ExitError` with
`"exit status 125"`). Output was streamed live to `os.Stdout/Stderr`, not
captured, so the returned error carries no copy of it. The caller wraps it
as `fmt.Errorf("build %s: %w", s.Label, err)` (line 114), `VerifyBuildSystem`
formats it as `"Build system failed: %v"` (line 357), and `runStep`'s
deferred recover surfaces it as the final FATAL line — none of these have
access to the docker-compose output, only the bare exit code.

In the user's environment, `manual-test.sh` redirected the run's stdout/stderr
to a log file the user did not see at exit. Even on an interactive terminal,
the live stream and the FATAL line are visually separated by a spinner clear
and an unknown amount of intervening output — the FATAL line should be
**self-contained** like `runComposeCtx`'s already is.

**Fix shape (do not apply — report-only):**

Mirror the `runComposeCtx` pattern (lines 273-291 of the same file):

```go
func runCompose(cwd string, args ...string) error {
    full := append([]string{"compose"}, args...)
    cmd := exec.Command("docker", full...)
    cmd.Dir = cwd
    tail := &tailWriter{cap: 16 * 1024}
    cmd.Stdout = io.MultiWriter(os.Stdout, tail)
    cmd.Stderr = io.MultiWriter(os.Stderr, tail)
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("docker compose %s: %w\nstderr tail:\n%s",
            strings.Join(args, " "), err, tail.String())
    }
    return nil
}
```

**Blast radius:**

`runCompose` is the workhorse for the local-verify lifecycle. It is invoked
by `Build` (the seed-bug site), `downOne`, `Clean`, and the post-health
log dump in `upOne` (line 180). Every "Build system / Stop system /
Clean system" FATAL line in a scaffolded run goes through this function;
patching it fixes the entire family at once.

---

## Findings — prioritized

### High (user-visible during normal scaffold runs)

| # | Site | Issue | Fix shape |
|---|---|---|---|
| H1 | `internal/runner/system.go:257-264` (`runCompose`) | Streams to stdio but returns bare `cmd.Run()` error — no `string(out)` in error message. **Seed bug.** | Tee to a `tailWriter` and inline `tail.String()` into the returned error, like `runComposeCtx` (line 273) already does. Fixes Build / Down / Clean / log-dump in one place. |
| H2 | `internal/runner/system.go:294-300` (`runDocker`) | Same shape as `runCompose`: streams to stdio, returns bare exit error. Used by `downOne` (line 251) to force-remove stray containers. | Same fix as H1. |
| H3 | `internal/runner/tests.go:233-249` (`runShell`) | Same shape: streams to stdio, returns bare `cmd.Run()` error. Surfaces as `"setup %q: %w"` (line 111), `"install %q: %w"` (line 153), and `"suite %s: %w"` (line 98) — every test setup / suite run failure has the same lossy wrap. | Same fix as H1. The runtime-side `realShell` in `internal/atdd/runtime/actions/bindings.go:2063` is a working template (uses `io.MultiWriter` + tee). |
| H4 | `internal/shell/github.go:145-160` (`RunPassthrough`) | Same shape: streams to stdio, returns bare `cmd.Run()` error. Used **only** by `internal/compiler/compiler.go:43` (`passthroughShell.Run`), which underpins **every** `gh optivem compile` invocation — the Phase 6 "Compile system" / "Compile tests" steps. Failures produce `"compile (typescript) "npx tsc --noEmit" in /path: exit status 1"` with no stderr context. | Same fix as H1. Captures the same value (the user can scroll up to see the stream, but the FATAL line is the load-bearing artifact in bug reports and CI logs). |
| H5 | `main.go:707-715` (bug-report file write) | `bodyFile.WriteString(body)` and `bodyFile.Close()` errors silently discarded. If the temp file fails mid-write the subsequent `gh issue create --body-file` will post a truncated bug report and the user will see "Bug report created: <url>" pointing at incomplete content. | Capture both errors; on write failure log a warn and skip the `gh issue create` step. |

### Medium (less-common code paths, still user-visible)

| # | Site | Issue | Fix shape |
|---|---|---|---|
| M1 | `internal/steps/github_setup.go:148` (`EnsureWorkflowDir`) | `os.MkdirAll(filepath.Join(repoDir, ".github", "workflows"), 0755)` — error dropped. If the parent is read-only or the path is a file, every subsequent workflow-file write produces a confusing "no such file or directory" rather than a clear "could not create workflows dir". | Capture err; `log.Fatalf` on failure. |
| M2 | `main.go:735-743` (`checkForUpdate`) | `cmd.Output()` error dropped — the comment "fail silently — don't block usage if offline or rate-limited" is correct intent, but a 4xx (e.g. a renamed release endpoint) would also be swallowed and the user never gets a "you're 6 versions behind" notice. | Defensible as-is given the comment; consider logging the err under `log.Debugf` so `--verbose` users can diagnose missed update notices. **Low-priority.** |
| M3 | `main.go:893` (`ghCLIVersion`) | `cmd.Output()` error returns "unknown" — defensible per the doc comment, but pre-validation should already have caught "gh missing". Leaving "unknown" in the banner is a small but real diagnostic loss. | Defensible as-is. |
| M4 | `internal/runner/system.go:230-253` (`downOne`) | `_ = runDocker(cwd, args...)` (line 251) — explicit blank-discard with no comment. `runDocker`'s output (if H2 is fixed) would carry useful info on a partial cleanup. | Add a comment explaining the intent (best-effort container cleanup, mirrors PS1's `2>$null`) **and** at minimum log via `log.Warnf` so a stuck container surfaces a hint. |
| M5 | `internal/runner/system.go:242-245` (`downOne`'s `dockerCapture`) | `out, err := dockerCapture(...); if err != nil { return nil }` — `// probe-only; don't fail Down on a missing daemon`. Comment present; intent clear; defensible. | None — the comment is the documentation the M4 site is missing. |
| M6 | `internal/steps/cleanup.go:25` (`deleteLocalDirs`) | `os.RemoveAll(dir)` — error dropped, post-success best-effort cleanup. The `cfg.KeepLocal` branch already covers the failure case. | Defensible as-is. Consider a `log.Debugf` on err. |

### Low (defensive / internal)

| # | Site | Issue |
|---|---|---|
| L1 | `internal/config/config.go:817,822,840,894` — `userCmd.Stderr = nil` / `cmd.Stderr = nil` on `gh api` probes. | Intentional 404-suppression for owner-or-org / project-exists / repo-exists probes. Documented at line 812 ("Stderr is suppressed so the first 404 doesn't leak when we fall back"). **Legitimate.** |
| L2 | `internal/atdd/runtime/driver/driver.go:804-810` (`fixVerifyChangedFiles`) | `git status --porcelain` failure returns empty string. Documented at lines 796-799 ("dispatch is feedback, not load-bearing"). **Legitimate.** |
| L3 | `internal/shell/github.go:64,100` — `check=false` in `Run`/`RunStdin` logs via `log.Warnf` rather than swallowing. The package-level contract on line 36-39 documents this. **Legitimate.** |
| L4 | `internal/shell/github.go:536-541` (`GitHub.Delete`) | `Run(...)` error logged via `log.Warnf` (best-effort teardown, comment at line 534). **Legitimate.** |
| L5 | `internal/shell/sonarcloud.go:104,124,143,200` — every `s.api(...)` error is `log.Warnf`'d not returned. SonarCloud project creation is documented as best-effort and the runtime preflight has a separate enforcement path. **Legitimate.** |
| L6 | `internal/shell/sonarcloud.go:61` — `defer resp.Body.Close()` error dropped. **Standard idiom; legitimate.** |
| L7 | `main.go:713-714` — `defer os.Remove(bodyFile.Name())` dropped. **Best-effort temp cleanup; legitimate.** |
| L8 | `internal/steps/finalize.go:54` — `os.WriteFile` for LICENSE wrapped in `log.Warnf` ("continuing without LICENSE"). Documented best-effort. **Legitimate.** |

---

## Notable healthy patterns (kept for contrast)

The following sites get it right and serve as the working templates the H1-H4
sites should adopt:

- `internal/runner/system.go:273-291` (`runComposeCtx`) — tees stdio to a `tailWriter`, inlines the tail into the error. This is the direct fix template for H1.
- `internal/atdd/runtime/actions/bindings.go:2063-2095` (`realShell.Run`) — tees stdio to both `os.Stdout/Stderr` and a `bytes.Buffer`, inlines the buffer into the error. Direct template for H3 and H4.
- `internal/atdd/runtime/actions/bindings.go:2027-2061` (`realGh`, `realGit`), and the matching pairs in `board/board.go`, `classify/classify.go`, `gates/bindings.go`, `release/release.go`, `verify/bindings.go`, `trace/trace.go`, `clauderun/clauderun.go` — all use `cmd.Output()` + captured `stderr` buffer + `fmt.Errorf("...: %w (stderr: %s)", ...)`. Uniform across the runtime-side codebase.
- `internal/shell/github.go:40-67` (`Run`) — uses `cmd.CombinedOutput()` and folds `output` into the returned error on `check=true`. Every `shell.Run`/`MustRun*` call site in the scaffold path is therefore self-contained.
- `internal/shell/github.go:118-142` (`RunCapture`) — captures stderr separately and folds it into the returned error.
- `internal/config/config.go:1147-1149,1153-1155,1166-1168` — `cloneShop` and `latestMetaRelease` use `CombinedOutput` and explicitly include `string(out)` in the returned error.
- `internal/steps/verify.go:478-483` (`mklinkJ`) — `CombinedOutput` + output-in-error.
- `internal/steps/verify.go:516-520` (`sonarComponent`) — `shell.Run` + explicit `\n%s` of `out` in `log.Fatalf`.
- `internal/steps/finalize.go:163-165` — `git push` failure includes `out` in the FATAL line.

---

## Recommended order of fixes

1. **H1** (`runCompose`) — single function, fixes the seed bug, plus Down / Clean / log-dump in one shot.
2. **H3** (`runShell`) — fixes the entire "Setup tests" / "Run tests" failure family. Same pattern as H1.
3. **H4** (`RunPassthrough` / `passthroughShell`) — fixes both `gh optivem compile` and `compile_all` structural-cycle action.
4. **H2** (`runDocker`) — small surface (only `downOne`'s force-remove path), but trivially the same change as H1.
5. **H5** (bug-report write) — orthogonal to the docker family, but a real silent corruption risk on bug-report submission. Three-line fix.
6. **M1** (`EnsureWorkflowDir`) — one-line fix, prevents a confusing failure mode.
7. **M4** (commented `_ = runDocker`) — documentation + log.Warnf.

Items M2-M3, M5-M6, and L1-L8 are documented as intentional; no action recommended.

---

## Counts

- High-severity findings: **5**
- Medium-severity findings: **6** (2 actionable, 4 defensible)
- Low-severity findings: **8** (all defensible / documented)
- **Total findings: 19** (well under the 20-cap; no overflow tail).

---

## 2026-05-14: H1–H5 fixed

Per `plans/20260514-0850-build-system-stderr-visibility.md`:

- **H1** `runCompose` (`internal/runner/system.go`) — tees stdio to a 16 KB `tailWriter`, folds tail into returned error.
- **H2** `runDocker` (same file) — same fix.
- **H3** `runShell` (`internal/runner/tests.go`) — same fix.
- **H4** `RunPassthrough` (`internal/shell/github.go`) — tees stdio to a `bytes.Buffer`, folds output into returned error.
- **H5** bug-report `bodyFile` write/close (`main.go` `createBugReport`) — write/close errors now log and abort the `gh issue create` call instead of posting a truncated body.

Regression: `TestRunComposeError_SurfacesStderr` in `internal/runner/system_test.go`.

Medium and Low findings remain out of scope.
