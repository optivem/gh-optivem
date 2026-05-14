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

### 7. Verify end-to-end — ⏳ Deferred

Requires intentionally breaking docker on the test machine (uninstall,
rename binary out of PATH, or stop the docker daemon — whichever is
convenient on the user's platform) and re-running `scripts/manual-test.sh`
to confirm the new FATAL line carries the docker-compose stderr instead
of the bare "exit status 125". Then restoring docker and re-running to
confirm the success path still streams stdio live.

Deferred because it is platform-side manual verification — the regression
test (`TestRunComposeError_SurfacesStderr`) already proves the
runCompose-wrap path on the happy path. Whoever runs the next live
scaffold against this branch finishes verification.

```bash
bash scripts/manual-test.sh --owner valentinajemuovic --system-name "Page Turner" \
    --arch multitier --repo-strategy multirepo \
    --backend-lang dotnet --frontend-lang typescript --test-lang typescript \
    --shop-ref main
```

Expected (post-fix):

```
FATAL: Build system failed: docker compose build: exit status 125
stderr tail:
docker-compose: command not found
```

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
