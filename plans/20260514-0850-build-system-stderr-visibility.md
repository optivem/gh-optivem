# Plan: surface swallowed stderr/stdout in local-verify lifecycle failures

## Context

On 2026-05-14 a user (`jasonribble`, optivem/gh-optivem#55) ran
`manual-test.sh`, did not have `docker-compose` installed, and saw the
scaffolder die at Phase 6/11 "Build system" with the entire failure
explanation being:

```
> [11:34:00] Building system...
FATAL: Build system failed: build real: exit status 125
FAIL Step failed: Build system -- Build system failed: build real: exit status 125
```

Exit code 125 with zero context. They had no way to know docker-compose
was the problem from the FATAL line alone.

The 2026-05-14 audit (`audits/20260514-silent-external-call-failures.md`)
traced the bug to `internal/runner/system.go:257-264` (`runCompose`),
which streams docker-compose's stdio live to `os.Stdout/Stderr` but
returns the bare `cmd.Run()` exit error — so by the time the caller
chain (`runner.Build` → `verify.go:357 log.Fatalf`) prints the FATAL
line, the docker-compose output is gone (and may have scrolled off, or
in the user's case been redirected to a log file they never looked at).

The audit identified five high-severity sites with the same bug shape,
all in the local-verify lifecycle:

| # | Site | Surfaces |
|---|---|---|
| H1 | `internal/runner/system.go:257-264` `runCompose` | Build / Down / Clean (the seed bug) |
| H2 | `internal/runner/system.go:294-300` `runDocker` | `downOne`'s force-remove path |
| H3 | `internal/runner/tests.go:233-249` `runShell` | Setup tests / suite install / suite run |
| H4 | `internal/shell/github.go:145-160` `RunPassthrough` | `gh optivem compile` (Compile system / Compile tests) |
| H5 | `main.go:707-715` bug-report write | Truncated `gh issue create --body-file` payloads |

H1–H4 share one mechanical fix: tee stdio to a tail buffer, inline the
tail into the returned error. Working templates already exist in the
codebase — `runComposeCtx` (line 273) for H1/H2 and `realShell.Run`
(`internal/atdd/runtime/actions/bindings.go:2063`) for H3/H4. H5 is
orthogonal but in the same High tier.

This plan applies the fix to H1–H4 and the H5 file-write check. Medium
and Low audit findings are out of scope (documented as defensible or
not user-visible during a normal scaffold run).

## Critical files

- `internal/runner/system.go` — `runCompose` (H1, line 257), `runDocker`
  (H2, line 294), and the existing `tailWriter` (line 80) + `runComposeCtx`
  (line 273) template to copy from.
- `internal/runner/tests.go` — `runShell` (H3, line 233).
- `internal/shell/github.go` — `RunPassthrough` (H4, line 145).
- `main.go` — bug-report `bodyFile` write/close (H5, lines 707-715).
- `internal/runner/system_test.go` (or new file under `internal/runner/`)
  — regression test asserting that a failing `runCompose` returns an
  error whose `Error()` contains a recognizable substring of the child's
  stderr.

## Reuse references

- **`internal/runner/system.go:80-103`** — the existing `tailWriter`
  type. Bounded ring buffer, capped at N bytes, safe for concurrent
  writes from `Stdout` + `Stderr` MultiWriter targets. Reused as-is by
  H1/H2.
- **`internal/runner/system.go:273-291`** — `runComposeCtx`, the working
  template for H1. Direct line-for-line model: `io.MultiWriter(os.Stdout, tail)`
  + `fmt.Errorf("...: %w\nstderr tail:\n%s", ..., err, tail.String())`.
- **`internal/atdd/runtime/actions/bindings.go:2063-2095`** — `realShell.Run`,
  the working template for H3/H4. Same shape, uses `bytes.Buffer` instead
  of `tailWriter` (unbounded — appropriate where output is expected to be
  small; `tailWriter`'s 16 KB cap is appropriate where output is large,
  e.g. `npm ci`).
- **`internal/shell/github.go:40-67`** — `Run`, which already folds output
  into the error correctly via `CombinedOutput`. Confirms the codebase
  convention that errors from shell-outs include their output.

## Out of scope

- **Medium audit findings (M1-M6).** `EnsureWorkflowDir` is a one-line
  fix worth doing eventually, but folding it into this plan blurs the
  focus per `feedback_materialize_dont_expand.md` — this plan is
  "make swallowed stderr visible in the local-verify lifecycle", not
  "fix every dropped error in `main.go`".
- **Low audit findings (L1-L8).** All documented as intentional
  (404 suppression, best-effort teardowns, etc.).
- **Pre-emptive docker presence check.** Covered by the sibling plan
  `20260514-0850-preflight-docker-check.md` — that one prevents this
  failure mode entirely for the docker-missing case; this plan makes
  sure that when *any* exit-non-zero docker / shell / compile error
  fires, the cause is visible. The two plans are complementary: the
  preflight catches the common case fast; this plan covers the long
  tail (image-pull 403, container OOM, npm-registry ECONNRESET, etc.).
- **Live progress display changes.** The current behaviour — streaming
  child stdio to `os.Stdout/Stderr` — is correct and stays. The fix
  *adds* a tee into a buffer; it does not redirect the live stream.
- **Changing `runComposeCtx`.** It already does the right thing; do
  not touch.
- **Backporting to runtime-side packages.** Audit confirms
  `atdd/runtime/...` and the `realGh`/`realGit` runners already
  capture stderr correctly. No work needed there.

## Steps

### 1. Fix H1: `runCompose` (the seed bug)

In `internal/runner/system.go:257-264`, replace the body with the
`runComposeCtx`-shaped equivalent (line 273 is the template):

```go
// runCompose executes `docker compose <args...>` from cwd. stdout+stderr are
// streamed to os.Stdout/os.Stderr so the user sees live progress; the last
// 16KB are also mirrored into the returned error message so a failure's
// FATAL line is self-contained (the live stream may have scrolled off or
// been redirected to a log file the user does not look at).
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

Confirm `io`, `strings` imports are already present in the file (they are —
`runComposeCtx` uses both).

### 2. Fix H2: `runDocker`

In `internal/runner/system.go:294-300`, apply the same shape:

```go
// runDocker executes `docker <args...>` with output streamed to the user
// and the last 16KB mirrored into the returned error.
func runDocker(cwd string, args ...string) error {
    cmd := exec.Command("docker", args...)
    cmd.Dir = cwd
    tail := &tailWriter{cap: 16 * 1024}
    cmd.Stdout = io.MultiWriter(os.Stdout, tail)
    cmd.Stderr = io.MultiWriter(os.Stderr, tail)
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("docker %s: %w\nstderr tail:\n%s",
            strings.Join(args, " "), err, tail.String())
    }
    return nil
}
```

Note: `downOne` (line 251) calls `runDocker` as `_ = runDocker(...)` — a
deliberate best-effort discard for the force-remove cleanup. The fix
still applies; the discarder just continues to discard. The audit M4
finding asks for a `log.Warnf` there but it is **out of scope** for
this plan (see Out of scope).

### 3. Fix H3: `runShell`

In `internal/runner/tests.go:233-249`, apply the same shape. `runShell`
is the workhorse for the entire test lifecycle (setup, install, suite
run), so this single change covers three caller wraps at lines 98, 111,
153.

```go
func runShell(cwd, label string, line string) error {
    cmd := exec.Command("bash", "-lc", line)
    cmd.Dir = cwd
    tail := &tailWriter{cap: 16 * 1024}
    cmd.Stdout = io.MultiWriter(os.Stdout, tail)
    cmd.Stderr = io.MultiWriter(os.Stderr, tail)
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("%s: %w\nstderr tail:\n%s", label, err, tail.String())
    }
    return nil
}
```

(Adjust signature to match the existing one — the audit gave line 233
but the exact body should be read fresh during implementation. Keep the
existing label/cwd/cmd semantics; only the stdio+error path changes.)

If `tailWriter` is not exported from `internal/runner/system.go`, it is
already in the same package — `runShell` in `tests.go` can reference it
directly. Confirm by reading the file before editing.

### 4. Fix H4: `RunPassthrough`

In `internal/shell/github.go:145-160`, apply the same shape. `tailWriter`
is in `internal/runner`, not `internal/shell`, so this site uses
`bytes.Buffer` (the `realShell.Run` template, audit:152). Output from a
single `npm ci` is bounded enough that an unbounded buffer is acceptable
and matches the existing template; switching to `tailWriter` would
require either exporting it or duplicating the type, neither of which
buys anything here.

```go
func RunPassthrough(commandLine, cwd string) error {
    parts := strings.Fields(commandLine)
    if len(parts) == 0 {
        return fmt.Errorf("empty command line")
    }
    bin := pathx.NormalizeExe(parts[0])
    cmd := exec.Command(bin, parts[1:]...)
    cmd.Dir = cwd
    var buf bytes.Buffer
    cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
    cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("%s: %w\noutput:\n%s", commandLine, err, buf.String())
    }
    return nil
}
```

(As with Step 3, read the existing body fresh before editing to preserve
any imports, normalisation, or signature details the audit summary
glosses over.)

### 5. Fix H5: bug-report file write

In `main.go:707-715`, capture both errors and skip the `gh issue create`
on failure rather than posting a truncated body:

```go
if _, err := bodyFile.WriteString(body); err != nil {
    log.Warnf("Failed to write bug-report body to %s: %v — skipping issue creation.", bodyFile.Name(), err)
    return
}
if err := bodyFile.Close(); err != nil {
    log.Warnf("Failed to close bug-report body file %s: %v — skipping issue creation.", bodyFile.Name(), err)
    return
}
```

(Adapt the early-return path to match the surrounding control flow —
read the function before editing. If the call site is not in a
function with a clean return, log the warning and skip the
`gh issue create --body-file` call specifically.)

### 6. Regression test for H1

Add a test under `internal/runner/` that runs `runCompose` (or, if the
function is unexported and not callable cross-file, a thin
package-internal test) against a deliberately-failing invocation —
e.g. `docker compose --file /nonexistent.yml config`. Assert:

- `err != nil`
- `err.Error()` contains a substring of the child's stderr (e.g.
  `"open /nonexistent.yml"` or `"no such file"`).
- `err.Error()` contains the `"docker compose"` prefix from the wrap.

Skip the test if `docker` is not on PATH (use `t.Skip` after
`exec.LookPath`). Keep the test small — one positive case (real stderr
surfaces) is enough; do not test the tailWriter's 16 KB cap (that
belongs to `tailWriter`'s own tests if it has them).

This test is the canary for H1; H2/H3/H4 are the same change and do not
need their own end-to-end tests — adding one for each would be
boilerplate. Optionally add a unit test that injects a fake `cmd.Run`
via a small `exec.Command` seam if `internal/runner` has one; if not,
skip — the H1 integration test suffices.

### 7. Verify end-to-end

From `gh-optivem/`, intentionally break docker on the test machine
(uninstall, rename binary out of PATH, or stop the docker daemon —
whichever is convenient on the user's platform) and re-run:

```bash
bash scripts/manual-test.sh --owner valentinajemuovic --system-name "Page Turner" \
    --arch multitier --repo-strategy multirepo \
    --backend-lang dotnet --frontend-lang typescript --test-lang typescript \
    --shop-ref main
```

Expected FATAL line (compared with the pre-fix one):

**Before:**
```
FATAL: Build system failed: build real: exit status 125
```

**After:**
```
FATAL: Build system failed: docker compose build: exit status 125
stderr tail:
docker-compose: command not found
```

(Exact stderr depends on the docker setup — Docker Desktop missing,
Compose v1 missing, daemon stopped, etc. The point is that whatever
docker said is now in the FATAL line itself.)

Then restore docker and run the same command to confirm the success
path still streams stdio live to the terminal (the tail is only
folded into the error on failure).

### 8. (Optional) Update CONTRIBUTING.md or the audit file

If the audit file is going to live on as a reference, append a
`## 2026-05-14: H1–H5 fixed in <PR#>` line at the bottom so a future
reader knows which findings are stale.

## Verification

The plan is complete when:

1. Running `gh optivem init` with docker stopped (or docker-compose
   missing on a Compose-v1 setup) fails with a FATAL line that
   contains the docker-compose error message, not just "exit status 125".
2. The same is true for `gh optivem compile` when a compile tool emits
   a useful stderr (e.g. typescript type-error output, .NET build
   errors) — the error is in the returned error, not only in scrollback.
3. The regression test added in Step 6 passes when run against a real
   docker installation and fails (with a clear assertion message) if H1
   is reverted.
4. The bug-report write failure case (H5) no longer produces a
   truncated GitHub issue — file-write failures abort the issue
   creation with a `log.Warnf`.
