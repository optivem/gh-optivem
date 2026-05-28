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

3. **PID marker file location: user-level state directory, NOT
   project-sidecar.** One file per dispatch under:
   - Windows: `%LOCALAPPDATA%\gh-optivem\runs\<ts>-<parent-pid>\<seq>-<agent>.pid`
   - Linux/Mac: `$XDG_STATE_HOME/gh-optivem/runs/<ts>-<parent-pid>/<seq>-<agent>.pid`
     (XDG default: `~/.local/state/gh-optivem/`)

   Rationale: PID files are OS-resource handles, not run history. The
   motivating bug is `rm -rf worktrees/rehearsal-XYZ/` failing because
   orphan claude.exe holds handles inside the worktree — sidecar
   markers would die with the worktree, leaving orphans untrackable.
   User-level state survives worktree deletion and lets one
   `gh optivem doctor --orphans` from any cwd find every orphan for the
   current user. The `<parent-pid>` suffix in the dir name disambiguates
   simultaneous starts (two gh-optivem processes for the same user can't
   share a PID). Per-dispatch granularity (one file per agent dispatch,
   not one per run) lets `doctor --orphans` show per-agent context.

   Note: this is a deliberate departure from the `events.jsonl`
   sidecar pattern. The events log IS a run artifact and stays at the
   project level; the PID marker is process-resource state and lives
   at the user level. Two different lifetimes, two different homes.

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

7. **PID marker file schema: JSON `{"child_pid", "parent_pid", "cwd"}`.**
   The file records:
   - `child_pid` — the spawned claude PID, used to kill/probe.
   - `parent_pid` — the gh-optivem process that spawned the child,
     used to distinguish force-cancelled dispatch (parent dead →
     orphan) from a live rehearsal (parent alive → skip).
   - `cwd` — the dispatch's working directory at write time, used by
     `doctor --orphans` to show "this orphan was for project X" since
     the file path itself no longer carries project context (user-level
     state dir is project-agnostic).

   Agent name + sequence stay in the filename (`<seq>-<agent>.pid`),
   timestamp + parent-pid stay in the dir name. Fields earn their JSON
   slot only when code branches on them ([[feedback_schema_fields_earn_slot]]).
   Bare-PID was the original draft; it broke once parent-PID and cwd
   joined the schema, so JSON wins on explicit field naming.
   Bare-int-per-line and key=value were considered and rejected.

## Open questions

(None. Schema, location, and process-alive shape are all settled above.
Items below carry the resolved choices.)

## Items

### A. PID marker file: binary side

1. **Add `PidFilePath` to `clauderun.Options`** (file:
   `internal/atdd/runtime/clauderun/clauderun.go` near the existing
   `EventsLogPath` at line 318-329). Doc-string: "When non-empty,
   `runHeadless` writes a JSON marker (`child_pid`, `parent_pid`,
   `cwd`) for the spawned claude process to this path immediately
   after `cmd.Start()` and removes the file on clean dispatch exit.
   Used by `gh optivem doctor --orphans` to identify children that
   survived a crash. Empty path → no marker file written (back-compat
   for non-dispatched callers, e.g. utility runs)."

   Also add to `RunOpts` (line 454-456) mirroring `EventsLogPath`.

2. **Switch `runHeadless` from `cmd.Run()` to `cmd.Start()` + `cmd.Wait()`**
   (file: `internal/atdd/runtime/clauderun/clauderun.go::runHeadless` at
   line 1715-1746). The PID is only available after `Start()` returns;
   `Run()` is `Start() + Wait()` fused so we must split them. Insert
   between them:
   ```go
   if opts.PidFilePath != "" {
       cwd := opts.Dir
       if cwd == "" {
           cwd, _ = os.Getwd() // best-effort; empty string acceptable
       }
       writePidFile(opts.PidFilePath, pidMarker{
           ChildPid:  cmd.Process.Pid,
           ParentPid: os.Getpid(),
           Cwd:       cwd,
       }, opts.Stderr)
       defer removePidFile(opts.PidFilePath, opts.Stderr)
   }
   ```
   `opts.Dir` already exists on `Options` (line 437) and is what
   `cmd.Dir` is set from at line 1728; reuse it. `writePidFile`
   marshals the `pidMarker` struct as JSON.
   `writePidFile` / `removePidFile` follow the same fail-soft policy as
   `openEventsLog` (line 1740): a missing-dir or unwritable-path failure
   downgrades to a non-fatal stderr warning so diagnostics never break
   the dispatch. `MkdirAll` the parent dir before writing. New helpers
   + `pidMarker` struct live next to `openEventsLog`.

3. **Also wire `runInteractive`** (line 1648-1666). Interactive
   dispatches can be Ctrl+C'd just as easily as headless. Same
   pattern: `cmd.Start()` + write PID + `cmd.Wait()` + remove PID on
   defer.

4. **Plumb `PidFilePath` through the driver** (file:
   `internal/atdd/runtime/driver/driver.go` near `EventsLogPath` at
   line 1044, and the path-building logic around line 1301). Resolve
   the user-level state root via a new helper
   `userStateDir() (string, error)`:
   - Windows: `filepath.Join(os.Getenv("LOCALAPPDATA"), "gh-optivem")`,
     fallback to `<userhome>/AppData/Local/gh-optivem` when
     `LOCALAPPDATA` is unset. **Not** `os.UserConfigDir` — that returns
     `%APPDATA%` (roaming), and PID files must stay machine-local.
   - Linux/Mac: `filepath.Join(os.Getenv("XDG_STATE_HOME"), "gh-optivem")`,
     fallback to `<userhome>/.local/state/gh-optivem` when
     `XDG_STATE_HOME` is unset.

   Construct path as
   `filepath.Join(stateDir, "runs", fmt.Sprintf("%s-%d", ts, os.Getpid()), fmt.Sprintf("%03d-%s.pid", seq, agentName))`.
   `os.MkdirAll` the parent before write. Empty `PidFilePath` when
   `eventsLog` would be empty (utility runs / non-dispatched calls).
   If `userStateDir` errors (e.g. no HOME on a stripped-down container),
   downgrade to a stderr warning and skip the marker — fail-soft, same
   policy as `openEventsLog`.

5. **Unit tests** (file:
   `internal/atdd/runtime/clauderun/clauderun_test.go`):
   - File is created before `cmd.Wait()` returns, contains parseable
     JSON, and the JSON's `child_pid` matches the spawned process,
     `parent_pid` matches `os.Getpid()`, and `cwd` matches `opts.Dir`
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
   - Resolves the user-level state dir via the same `userStateDir`
     helper Item 4 introduces, then scans
     `runs/*/[0-9][0-9][0-9]-*.pid` underneath.
   - For each file, reads the JSON (`child_pid`, `parent_pid`, `cwd`),
     parses `<seq>-<agent>` from the filename, parses `<ts>` and
     `<parent-pid>` from the parent dir name.
   - Classifies each marker:
     - `child_pid` dead → stale, `os.Remove` silently.
     - `child_pid` alive AND `parent_pid` alive → live dispatch in
       progress, skip (do NOT prompt).
     - `child_pid` alive AND `parent_pid` dead → orphan, list it.
   - Prints orphan list with child PID, agent, cwd, run timestamp,
     age (now minus file mtime).
   - For each orphan: prompt y/n via `internal/promptio.ConfirmYN`
     (the same prompter `atdd-rehearsal.sh` mimics — see comment at
     `scripts/atdd-rehearsal.sh:88`). On `y`: `process.Kill()`, then
     `os.Remove` the marker file. On `n`: leave both alone.
   - Exits 0 when all orphans were either dead-and-cleaned or
     interactively-resolved; non-zero if any kill failed.

   Cross-platform process-alive: small private helper next to the
   doctor logic with build-tagged variants. Linux/Mac:
   `process.Signal(syscall.Signal(0))` (errno ESRCH = dead). Windows:
   `os.FindProcess` returning a non-nil error = dead. Inline rather
   than a new package — single caller, ~10 lines per variant.

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
   doctor tests). Use real `sleep`-like subprocesses as stand-ins
   for claude.exe and for the parent gh-optivem (the cleanup logic
   does not depend on either being a specific binary — it kills /
   probes whatever PID the marker JSON names). Override the user-level
   state root via env var (e.g. `XDG_STATE_HOME` on Linux/Mac,
   `LOCALAPPDATA` on Windows) so the test writes into a `t.TempDir()`
   instead of polluting the real user dir.

   Scenarios:
   - **Orphan, killed:** marker JSON with live child PID, dead parent
     PID (e.g. spawn a child then kill an unrelated short-lived
     parent stand-in before the test runs). Run `doctor --orphans`
     with stdin scripted to answer `y`; assert the child is gone and
     the file is removed.
   - **Orphan, declined:** same setup as above; stdin answers `n`;
     assert the child is alive and the file persists.
   - **Stale (child already dead):** marker JSON with a known dead
     child PID and dead parent PID; assert the file is silently
     removed and exit code is 0, no prompt issued.
   - **Live dispatch (parent alive):** marker JSON with live child
     PID AND live parent PID; assert doctor does NOT prompt, file is
     left in place, exit code 0. This is the live-rehearsal-safety
     scenario.

### C. Rehearsal-specific script

10. **Create `scripts/atdd-rehearsal-cleanup.sh`.** Idempotent and safe
    to run with a live rehearsal in progress (uses `git worktree list`
    and `git branch --list 'rehearsal/*'` to distinguish registered
    from orphan artifacts — see [[feedback_check_concurrent_agents]]).
    Sections, in order:

    a. **Discover orphans:**
       - Derive `WORKTREES_DIR` exactly as `atdd-rehearsal.sh` does at
         line 207 (`<consumer-repo>/../worktrees`); reuse the same
         expression verbatim so the two scripts stay in lockstep.
       - List `<WORKTREES_DIR>/rehearsal-*/` directories.
       - Cross-reference with `git -C <consumer-repo> worktree list
         --porcelain`. Any directory NOT in the list is an orphan.
       - List `rehearsal/*` branches via `git -C <consumer-repo>
         branch --list 'rehearsal/*'`. Any branch NOT checked out by a
         registered worktree is an orphan.

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
  `%LOCALAPPDATA%\gh-optivem\runs\<ts>-<parent-pid>\` (Windows) or
  `$XDG_STATE_HOME/gh-optivem/runs/<ts>-<parent-pid>/` (Linux/Mac)
  AND a `claude.exe` with the JSON's `child_pid` is still alive.
- Run `gh optivem doctor --orphans`. Confirm the orphan is listed
  with agent name, cwd, and age; answer `y`; confirm the process
  is gone and the file is removed.
- Run `gh optivem doctor --orphans` again on a clean state. Confirm
  it exits 0 with "no orphans" output.
- Run `bash scripts/atdd-rehearsal-cleanup.sh --delete` against a
  state with both orphan worktree dirs and a live rehearsal in
  progress (start one via `atdd-rehearsal.sh` in another terminal).
  Confirm the live rehearsal's worktree and branch are untouched;
  orphans are removed; the doctor chain runs and silently classifies
  the live rehearsal's claude.exe as "parent alive, skip" (NOT
  prompted to kill — the parent `gh optivem implement` is still
  alive, so it's a live dispatch, not an orphan).

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
