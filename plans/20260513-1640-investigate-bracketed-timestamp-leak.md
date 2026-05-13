# Plan: identify the source of the stray `[HH:MM:SS]` file in `gh-optivem/`

## Context

On 2026-05-13 the user noticed an empty (0-byte) file literally named `[16:20:51]` in the `gh-optivem/` repo root. Git Bash on Windows surfaced it as `"[16\357\200\27220\357\200\27251]"` (U+F03A standing in for the disallowed `:` characters on NTFS), but it is a single innocuous filename.

The filename format `[HH:MM:SS]` exactly matches the prefix `internal/log/log.go:111,159` prepends to `Info()` / `StepDone()` lines via `time.Now().Format("15:04:05")`. The file appeared after a `bash scripts/manual-test.sh ...` invocation.

A static-analysis sweep on 2026-05-13 ruled out every in-repo path that could plausibly create such a file:

- All `os.Create` / `os.OpenFile` / `os.WriteFile` call sites in `*.go` — none construct a path from a log timestamp.
- `--log-file` default resolution (`internal/config/config.go:941-947` → `resolveLogFilePath`) — defaults to `$TEMP/gh-optivem-YYYYMMDD-HHMMSS.log`, never bracketed.
- All redirect operators (`>`, `tee`, `2>`) in `scripts/*.sh` — none target a bracketed path.
- User's `~/.bashrc`, `~/.bash_profile`, and `gh-optivem/.claude/settings.json` — no shell wrapper or hook that captures output to a file.

Leading hypothesis: a copy/paste accident in the user's terminal where a log line `[16:20:51] something` ended up on a command line with a `>` redirect (a class of mistake `scripts/install.sh:8-9` explicitly warns about for stale-binary help-text clobbers). The 0-byte content fits — the redirect target gets created via `open(..., O_WRONLY|O_CREAT|O_TRUNC)` even if the producing process dies before writing.

This plan runs a controlled re-run of `manual-test.sh` to confirm or refute that hypothesis. If the file reappears, the bug is reproducible and a real fix is needed; if it does not, the paste-accident theory is confirmed and no code change is warranted.

## Critical files

No edits planned. This is an investigative plan only. Files that would be edited **only if** Step 3 reproduces the bug:

- `scripts/manual-test.sh` — candidate for adding a post-run guard that fails loudly if a bracketed filename appears in the repo root.
- `internal/log/log.go` — candidate if a code path is found that uses `Sprintf("[%s]", ts)` for anything other than terminal/log-file output.

## Reuse references

- `scripts/install.sh:8-9` — pre-existing warning comment about cobra-help-clobbers-`>`-redirect, the closest documented analog to the suspected failure mode.
- `internal/log/log.go:108-118` — `Info()` body that owns the `[HH:MM:SS]` prefix format.
- `internal/log/log.go:154-167` — `StepDone()` body, same prefix format.
- `internal/config/config.go:941-947` — `resolveLogFilePath` (the legitimate consumer of timestamps in filenames; control case for what a real log path looks like).

## Steps

### 1. Baseline: confirm a clean repo root

From `gh-optivem/`:

```bash
ls -la | grep -E '\[' || echo "clean"
```

Expected: `clean`. If any bracketed file is already present, delete it before proceeding so Step 3's check is unambiguous:

```bash
rm -- "[...]"   # use literal filename from the ls output
```

### 2. Snapshot the repo root before the run

Capture a directory listing so any new file in Step 3 can be diffed cleanly:

```bash
ls -1 . > /tmp/gh-optivem-root-before.txt
```

### 3. Re-run `manual-test.sh` exactly as documented

Use the same invocation the user ran on 2026-05-13 (from `CONTRIBUTING.md:80-83`). From `gh-optivem/`:

```bash
bash scripts/manual-test.sh --owner valentinajemuovic --system-name "Page Turner" \
    --arch multitier --repo-strategy multirepo \
    --backend-lang dotnet --frontend-lang typescript --test-lang typescript \
    --shop-ref main
```

Let it run to completion (or to natural failure). Do not redirect its stdout/stderr — the leak hypothesis is about the user's shell, not the script's own output handling, so keep the run as faithful to the original as possible.

### 4. Check for a reappearance

From `gh-optivem/`:

```bash
ls -1 . > /tmp/gh-optivem-root-after.txt
diff /tmp/gh-optivem-root-before.txt /tmp/gh-optivem-root-after.txt
ls -la | grep -E '\[..[:.]..[:.]..\]' || echo "no bracketed file"
```

Decision tree:

- **"no bracketed file"** → paste-accident theory confirmed. No fix needed in this repo. Stop here and mark this plan resolved with the conclusion. Optionally add a one-line note to `scripts/install.sh`'s existing warning comment block making the symptom explicit (empty file named after the log prefix).
- **A `[HH:MM:SS]`-shaped file appeared** → reproducible bug. Continue to Step 5.

### 5. Narrow the producing step (only if Step 4 reproduces)

Re-run `manual-test.sh` with `--no-cleanup` so the scaffold dir survives, and instrument to find which step opens the offending path:

- Option A (low-friction): re-run with `--verbose --no-cleanup` and `script` capturing the full transcript:
  ```bash
  script -q -c 'bash scripts/manual-test.sh --no-cleanup --verbose <same flags as Step 3>' /tmp/manual-test-transcript.log
  ```
  Then `grep -n '\[..:..:..\]' /tmp/manual-test-transcript.log` to find which step emitted the timestamp closest to the offending file's mtime.

- Option B (more invasive, only if A is inconclusive): add a `trap` in `manual-test.sh` between phases that runs the bracketed-file check after each phase, prints the phase name, and aborts on first hit. That isolates the culprit to a single phase (install / config init / init).

- Option C (last resort): run under Windows Sysinternals Process Monitor with a filter on Path contains `[`. This catches the exact PID/binary that calls `CreateFile` on the bracketed name.

### 6. Decide on the real fix (only if Step 5 identifies a code path)

- If a Go call site is found constructing a filename from a log-prefix timestamp → fix that call site directly and add a regression test that builds a config / runs a step and asserts no bracketed files end up in the working dir.
- If a shell call site is found (a `>` or `tee` with a variable that picks up timestamped output) → fix the redirect and add a `shellcheck` rule or a script-test that grep-asserts the file is absent after a smoke run.

Write the fix as a separate follow-up plan; this plan's job ends at "root cause identified."

## Out of scope

- Editing `internal/log/log.go`'s prefix format. The `[HH:MM:SS]` shape is the right human-readable log prefix; the bug (if any) is downstream consumption, not the prefix itself.
- Adding a global filesystem watcher to `manual-test.sh`. If Step 5's Options A/B isolate the producer, that is sufficient — a watcher adds complexity for a one-off investigation.
- Investigating the file's reappearance on machines other than the user's Windows / Git Bash setup. The U+F03A `:`-substitution is MSYS-specific; any in-repo code path that produces the bug would manifest with the literal `:` on Linux/macOS, but that environment is not currently in scope for `manual-test`.

## Verification

Plan is complete when one of the two terminal states is reached:

1. **Paste-accident confirmed** — Step 4 produced "no bracketed file", and a brief note is appended to `scripts/install.sh`'s redirect-clobber warning (or this plan is closed with an explicit "no code change needed" decision recorded in the commit message that removes the plan).
2. **Code-path identified** — Step 5 names the exact file, line, and conditions that trigger the leak, and a follow-up plan is created for the fix. The follow-up plan replaces this one in `plans/`.
