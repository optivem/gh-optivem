# Recover from force-cancelled `gh optivem implement` runs

## Background

A force-cancel of `gh optivem implement` (Ctrl+C in the parent terminal,
closed terminal, kernel kill, panic in a child) can leave debris behind
that the existing cleanup paths do not reach:

- **Orphan headless `claude.exe` subprocesses** — the BPMN runner spawns
  one per `RUN_AGENT` task (see `internal/atdd/runtime/clauderun/clauderun.go::runHeadless`
  at line 1715). Ctrl+C in the parent terminal often fails to propagate
  cleanly to the child on Windows, so the subprocess survives the parent
  exit. The same problem affects direct `gh optivem implement` invocations
  AND rehearsal-wrapped (`scripts/atdd-rehearsal.sh`) invocations.
- **Stuck worktree directories** — observed concretely: `rm -rf
  worktrees/rehearsal-20260528-{094106,110635,124904}/` fails with
  `Device or resource busy` because the orphan claude.exe processes
  still hold file handles inside their cwd. This is the user-visible
  symptom that motivated this plan.
- **Orphan `rehearsal/*` branches and `.git/worktrees/<id>` metadata** —
  produced ONLY by `scripts/atdd-rehearsal.sh` (since direct
  `gh optivem implement` does not create worktrees). The script's EXIT
  trap (`scripts/atdd-rehearsal.sh::cleanup` at line 217) handles this
  on normal exit but is skipped on hard force-cancel.

Windows complicates detection: `Win32_Process` does **not** expose a
process's CWD via WMI/CIM. A cwd-heuristic cleanup ("find claude.exe
whose cwd is under `worktrees/rehearsal-*/`") would require Sysinternals
`handle.exe` or native Win32 calls. The binary knows the PIDs it
spawned; a PID marker file written by the binary side-steps the CWD
detection problem entirely.

## Design decisions (settled, not open)

These were resolved during refinement on 2026-05-28. Execution should
**not** revisit them; only the items below in **Open questions** are open.

1. **Encapsulation: binary owns its own state.** The binary writes the
   PID marker file AND reads it back for cleanup. A shell script
   reading the marker would create a hidden contract between the
   binary's schema and the script's parser — see [[feedback_question_second_file_ssots]]
   on multi-file SSoTs as drift sources.

2. **Subcommand placement: `gh optivem doctor --orphans`.** Force-cancel
   recovery is exceptional + diagnostic, which is `doctor`'s shape. This
   broadens `doctor` from "git config invariants" to "local state
   invariants" — the help text needs updating accordingly. Alternatives
   considered and rejected:
   - `gh optivem implement clean` — couples the cleanup to a single
     command; awkward if future BPMN-driven commands also write PID
     markers.
   - `gh optivem cleanup orphans` — currently scoped to "destructive
     operations against remote systems," broadening it to local state
     conflicts with its current docstring.

3. **PID marker file location: same sidecar pattern as `events.jsonl`.**
   One file per dispatch under `.gh-optivem/runs/<ts>/<seq>-<agent>.pid`
   (see `internal/atdd/runtime/driver/driver.go:1301` for the existing
   `events.jsonl` precedent). NOT a single `.gh-optivem/pids.txt` per
   run — per-dispatch granularity lets `doctor --orphans` show
   per-agent context in its list ("acceptance-test-writer for issue
   #71, started 12:49:04") instead of a bare PID.

4. **Lifecycle: delete marker on clean exit, persist on crash.** The
   presence of a leftover `.pid` file IS the crash signature — no
   separate "marked dirty" flag needed. Clean exit of the dispatch
   removes the file in a `defer`; crash leaves the file in place for
   `doctor --orphans` to find on the next run.

5. **`scripts/atdd-rehearsal-cleanup.sh` is rehearsal-only and chains
   to `doctor`.** Worktree dirs, branches, and `.git/worktrees`
   metadata are concepts only `atdd-rehearsal.sh` creates — they
   belong in the script. Process cleanup is broader (direct-implement
   users get orphans too) and belongs in the binary. The script
   invokes `gh optivem doctor --orphans` at the end so the rehearsal
   author gets one-stop recovery.

6. **No process auto-kill.** `doctor --orphans` lists candidates with
   PID, agent name, start time, age, and prompts y/n per process. Same
   UX as `atdd-rehearsal.sh`'s exit trap. Matches the safety stance in
   `scripts/cleanup-orphans.sh::--tmp` (lines 367-401).

## Open questions

1. **PID marker file schema.** Two reasonable shapes:
   - Bare PID, one int per line (simplest). Agent name + start time
     are derivable from the filename + file mtime.
   - JSON object `{"pid": 12345, "agent": "acceptance-test-writer",
     "issue": 71, "spawned_at": "..."}`. Self-describing but
     duplicates info already encoded in path + mtime.
   Items below assume the bare-PID shape; revisit during execution if
   the doctor formatter wants more context than the filename carries.

2. **Cross-platform process-alive check.** `os.FindProcess` on Linux/Mac
   always succeeds even for dead PIDs; `process.Signal(syscall.Signal(0))`
   is the conventional probe. On Windows, `os.FindProcess` actually opens
   a handle and returns an error for dead PIDs. The doctor implementation
   needs both paths. Likely a small helper in `internal/atdd/runtime/procs/`
   (new package) or inlined into the doctor command.

## Items

### A. PID marker file: binary side

1. **Add `PidFilePath` to `clauderun.Options`** (file:
   `internal/atdd/runtime/clauderun/clauderun.go` near the existing
   `EventsLogPath` at line 318-329). Doc-string: "When non-empty,
   `runHeadless` writes the spawned claude PID to this path immediately
   after `cmd.Start()` and removes the file on clean dispatch exit. Used
   by `gh optivem doctor --orphans` to identify children that survived a
   crash. Empty path → no marker file written (back-compat for
   non-dispatched callers, e.g. utility runs)."

   Also add to `RunOpts` (line 454-456) mirroring `EventsLogPath`.

2. **Switch `runHeadless` from `cmd.Run()` to `cmd.Start()` + `cmd.Wait()`**
   (file: `internal/atdd/runtime/clauderun/clauderun.go::runHeadless` at
   line 1715-1746). The PID is only available after `Start()` returns;
   `Run()` is `Start() + Wait()` fused so we must split them. Insert
   between them:
   ```go
   if opts.PidFilePath != "" {
       writePidFile(opts.PidFilePath, cmd.Process.Pid, opts.Stderr)
       defer removePidFile(opts.PidFilePath, opts.Stderr)
   }
   ```
   `writePidFile` / `removePidFile` follow the same fail-soft policy as
   `openEventsLog` (line 1740): a missing-dir or unwritable-path failure
   downgrades to a non-fatal stderr warning so diagnostics never break
   the dispatch. New helpers live next to `openEventsLog`.

3. **Also wire `runInteractive`** (line 1648-1666). Interactive
   dispatches can be Ctrl+C'd just as easily as headless. Same
   pattern: `cmd.Start()` + write PID + `cmd.Wait()` + remove PID on
   defer.

4. **Plumb `PidFilePath` through the driver** (file:
   `internal/atdd/runtime/driver/driver.go` near `EventsLogPath` at
   line 1044, and the path-building logic around line 1301). Construct
   as `filepath.Join(dir, fmt.Sprintf("%03d-%s.pid", seq, agentName))`,
   sibling to the events.jsonl path. Empty when `eventsLog` would be
   empty (utility runs / non-dispatched calls).

5. **Unit tests** (file:
   `internal/atdd/runtime/clauderun/clauderun_test.go`):
   - File is created with the spawned PID before `cmd.Wait()` returns
     (mock the claude CLI with a script that signals readiness then
     sleeps; assert the file appears, then signal exit).
   - File is removed on clean dispatch exit.
   - File is **not** removed if `cmd.Wait()` returns a non-nil error
     (use a script that exits non-zero) — the file IS the crash
     signature; preserving it on dispatch failure is the point.
   - Empty `PidFilePath` skips both write and defer-remove (no file
     created, no error logged).
   - Unwritable path (parent dir is a regular file) logs a stderr
     warning but does not fail the dispatch — mirroring the
     `openEventsLog` test at line 1031.

### B. `gh optivem doctor --orphans`

6. **Broaden `doctor`'s scope: add the `--orphans` flag** (file:
   `internal/<doctor command file>.go` — find via
   `grep -r 'Use:.*"doctor"' internal/`). Adjacent to the existing
   `--fix` flag. When `--orphans` is set, the doctor command:
   - Scans `.gh-optivem/runs/*/[0-9][0-9][0-9]-*.pid` from the cwd-rooted
     state directory (same root that the runs/ subdir lives under).
   - For each file, reads the PID, checks process-alive (see Open
     question 2), parses `<seq>-<agent>` from the filename, derives
     `<ts>` from the parent dir name (matches the events.jsonl naming
     scheme).
   - Prints a list with PID, agent, run timestamp, age (now minus file
     mtime). Skip files whose PID is no longer alive — those are stale
     markers from a crash where the OS reaped the process but the file
     persisted; just `os.Remove` them silently.
   - For each alive PID: prompt y/n via `internal/promptio.ConfirmYN`
     (the same prompter `atdd-rehearsal.sh` mimics — see comment at
     `scripts/atdd-rehearsal.sh:88`). On `y`: `process.Kill()`, then
     `os.Remove` the marker file. On `n`: leave both alone.
   - Exits 0 when all orphans were either dead-and-cleaned or
     interactively-resolved; non-zero if any kill failed.

7. **Update `doctor`'s help text** (`Long` field of the cobra Command).
   Broaden from "Verify the three global git config keys docs/tbd.md
   requires for trunk-based development" to something like:

   > Verify local state invariants:
   >   - global git config keys docs/tbd.md requires for trunk-based development
   >   - orphan child processes from crashed `implement` runs (with --orphans)
   >
   > With --fix, sets any missing/wrong git config keys at the global level.
   > With --orphans, lists orphan claude.exe children from crashed runs and
   > prompts to kill them.

   Also update the `Short` and the `Examples` block.

8. **Add a recovery hint to `implement`'s `Long`** (file: search for
   `Use:.*"implement"` then look for the cobra `Long` field). Append a
   short paragraph: "If a run crashes mid-dispatch (Ctrl+C, terminal
   closed, panic), orphan headless `claude` subprocesses may survive.
   Run `gh optivem doctor --orphans` to list and clean them up."

9. **Integration test for `--orphans`** (place alongside the existing
   doctor tests). Use a real `sleep`-like subprocess as a stand-in for
   claude.exe (the cleanup logic does not depend on it being claude
   specifically — it kills whatever PID the marker file names):
   - Pre-seed a marker file with a known live PID; run
     `doctor --orphans` with stdin scripted to answer `y`; assert the
     process is gone and the file is removed.
   - Pre-seed a marker file with a known **dead** PID; assert the file
     is silently removed and exit code is 0.
   - Pre-seed a marker file with a known live PID; run with stdin
     scripted to answer `n`; assert the process is alive and the file
     persists.

### C. Rehearsal-specific script

10. **Create `scripts/atdd-rehearsal-cleanup.sh`.** Idempotent and safe
    to run with a live rehearsal in progress (uses `git worktree list`
    and `git branch --list 'rehearsal/*'` to distinguish registered
    from orphan artifacts — see [[feedback_check_concurrent_agents]]).
    Sections, in order:

    a. **Discover orphans:**
       - List `worktrees/rehearsal-*/` directories (find under
         `<consumer-repo>/../worktrees/`, mirroring `atdd-rehearsal.sh`'s
         `WORKTREES_DIR` derivation at line 207).
       - Cross-reference with `git -C <consumer-repo> worktree list
         --porcelain`. Any directory NOT in the list is an orphan.
       - List `rehearsal/*` branches via `git branch --list
         'rehearsal/*'`. Any branch NOT checked out by a registered
         worktree is an orphan.

    b. **Print summary** (orphan dirs, orphan branches, count of each).
       Refuse to delete branches with commits ahead of `main` —
       print them as "skipped (has commits)" so the operator can
       investigate manually. Mirror the safety stance in
       `scripts/cleanup-orphans.sh:382-399`.

    c. **Prompt y/n** ("Delete N orphan worktree dirs + M orphan
       branches?"). Use the same `prompt_yn` shape as
       `atdd-rehearsal.sh:88` for consistency.

    d. **Execute:** `rm -rf` each orphan dir, `git branch -D` each
       orphan branch, then `git worktree prune` to drop stale
       `.git/worktrees/<id>` metadata.

    e. **Chain to the binary:** at the end, exec
       `gh optivem doctor --orphans` (resolved from PATH, NOT the
       just-built `gh-optivem.exe` — the rehearsal script's
       freshly-built binary is scoped to the rehearsal worktree; the
       cleanup script should use whatever the operator's PATH points
       at, which is the installed binary). The operator gets the
       process-cleanup prompt as a continuation of the same UX.

11. **Add `--dry-run` support** to the script (matches
    `cleanup-orphans.sh:67`). Default to dry-run; require `--delete`
    for real deletion. Same pattern: prints what WOULD be deleted
    without doing it, so the operator can sanity-check before
    committing.

12. **`--help` text** at the top of the script (between the shebang
    and `set -euo pipefail`), following the same format as
    `atdd-rehearsal.sh:12-69`. Document the safety stance ("safe to
    run with a live rehearsal in progress"), the chain-to-doctor
    behaviour, and `--dry-run` vs `--delete`.

## Verification

- Force-cancel a `gh optivem implement` run via `kill -9` on the
  parent gh-optivem process (NOT Ctrl+C, which might still let
  defers fire). Confirm a `.pid` file remains under
  `.gh-optivem/runs/<ts>/` AND a `claude.exe` with that PID is
  still alive.
- Run `gh optivem doctor --orphans`. Confirm the orphan is listed
  with agent name and age; answer `y`; confirm the process is gone
  and the file is removed.
- Run `gh optivem doctor --orphans` again on a clean state. Confirm
  it exits 0 with "no orphans" output.
- Run `bash scripts/atdd-rehearsal-cleanup.sh --delete` against a
  state with both orphan worktree dirs and a live rehearsal in
  progress (start one via `atdd-rehearsal.sh` in another terminal).
  Confirm the live rehearsal's worktree and branch are untouched;
  orphans are removed; the doctor chain runs and lists the live
  rehearsal's claude.exe but does NOT prompt to kill it (it isn't
  orphaned — the parent `gh optivem implement` is still alive).

  Subtlety to verify: a live rehearsal's pid file IS present under
  `.gh-optivem/runs/<ts>/`. The doctor logic must distinguish
  "process alive AND parent gh-optivem alive" (skip — not orphan)
  from "process alive AND parent dead" (prompt — orphan). The
  cleanest signal is the parent PID at write time:
  `writePidFile` should also record the parent gh-optivem PID, and
  doctor skips entries whose parent is still alive. **This is a
  design refinement uncovered during verification authoring** —
  fold into Item 2's `writePidFile` shape and Open question 1's
  schema discussion before executing.

## Cross-references

- Existing related plans:
  - `plans/20260528-1302-suppress-subprocess-stderr-non-verbose.md` —
    open question, unrelated to this plan; no overlap with this work.
  - `plans/20260528-1235-tighten-test-writer-and-dsl-implementer-prompts.md` —
    in-flight, touches `internal/assets/runtime/agents/atdd/*.md`;
    no file overlap with this plan.
- Related script: `scripts/atdd-rehearsal.sh` (creates the worktrees
  this plan recovers from), `scripts/cleanup-orphans.sh` (remote-side
  cleanup, NOT to be conflated — different scope).
- Memories: [[feedback_check_concurrent_agents]] (concurrent-rehearsal
  safety), [[feedback_question_second_file_ssots]] (why the binary
  owns its own state), [[reference_plan_filename_timestamp]] (filename
  uses local time, not UTC).
