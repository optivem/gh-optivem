# Stash `verify-results` and `changed-files` for the unexpected-tests fixers

## Context

`unexpected-failing-tests-fixer` and `unexpected-passing-tests-fixer` are
dispatched with two empty diagnostic blocks in their prompts:

```
### Verify results to address

${verify-results}

### Changed files from the WRITE phase

${changed-files}
```

Observed during the `gift-wrap an order` rehearsal
(`worktrees/rehearsal-20260527-135607`, dispatch
`010-unexpected-failing-tests-fixer.prompt.md`): both placeholders
substituted to "", which left the fixer with nothing to diagnose. The
fixer correctly noticed the absence and exited; on a *real* red verify
it would have done the same and the silent-fail would have been masked
by a green re-verify.

### Root causes

**Bug 1 — `${verify-results}` is read but never written.**

`internal/atdd/runtime/driver/driver.go:956` substitutes the placeholder
from `ctx.GetString("verify_results_text")`. A full-tree grep finds
**zero writes** to that ctx key — the only references are the one read
site in `driver.go` plus the two fixer prompt templates that consume the
placeholder.

The `runCommand` action
(`internal/atdd/runtime/actions/bindings.go:720-755`) — the one
binding that executes `gh optivem test run` — only stamps:

- `command-succeeded` (bool)
- `test-outcome` (`pass` | `fail`)
- on failure: `failure-kind`, `command-line`, `command-exit-code`,
  `command-stderr-tail`

It does not stamp `verify_results_text`. Result: the placeholder is
empty on every fix-unexpected-failing-tests / fix-unexpected-passing-tests
dispatch.

The `classifyShellErr` infra/red classifier
(`internal/atdd/runtime/actions/verify_classify.go`) exists but is
**dormant** — only its own unit test references it. It was scaffolded
for the same plan that intended to populate `verify_results_text` (see
its package comment: "the gateway and fix-agent dispatch land in later
items of the same plan"), and the later items never landed.

**Bug 2 — `${changed-files}` is shelled out live at dispatch time and
returns "" when the working tree is clean for any reason.**

`fixChangedFiles` (`driver.go:1050-1074`) runs
`git status --porcelain` against `opts.RepoPath` at dispatch time for
every fix-* agent except `fix-scope-diff` (which reads pre-captured
`phase-changed-files` from ctx.State). When that shell-out returns
empty bytes — for any reason — the placeholder substitutes to "" and
the fixer loses sight of "what the WRITE phase just edited."

Compare to `fix-scope-diff`, which reads the pre-captured snapshot
delta `validateOutputsAndScopes` stashes at `phase-changed-files`. That
path is robust against working-tree state at dispatch time because the
delta is computed against the pre-WRITE baseline
(`snapshot-working-tree` ⇒ `CtxKeyPreAgentFingerprint`).

### Why both bugs surface together

The two fix-unexpected-tests fixers carry both fields, and both fields
arrive empty. The agent body explicitly tells the fixer "Read these
first; they are the entire signal" — so when both arrive empty, the
fixer has *nothing* to work from. The agent's own diagnosis in the
rehearsal was correct: "no failing-verify input to diagnose."

### Existing load-bearing pattern is the precedent

Other diagnostic placeholders already use the "register only when
non-empty so `findUnfilledPlaceholders` fails the dispatch fast"
pattern — see `CommandLine` / `CommandExitCode` / `CommandStderrTail`
for `fix-command-failed` (clauderun.go:163-187) and `FailingTaskName`
/ `MissingOutputs` / `ViolatingPaths` for fix-missing-output /
fix-scope-diff (clauderun.go:189-210). `VerifyResults` and
`ChangedFiles` are currently registered unconditionally
(clauderun.go:665-676), so an empty value substitutes silently and
the operator sees the symptom only by reading the rendered prompt
log.

### Open questions resolved up-front

1. **What should `verify_results_text` contain — stdout, stderr, or
   both?** Both, combined as `stdout\n---STDERR---\nstderr` when both
   are non-empty (mirroring how test runners surface failure: the
   test name + assertion message often lands on stdout, the stack
   trace on stderr). Cap at `commandStderrTailLines` (20 lines) per
   stream so a chatty runner does not blow the prompt size, matching
   the existing `command-stderr-tail` discipline. Reuse the
   `lastNLines` helper.
2. **Should we revive `classifyShellErr` and route infra-vs-red here?**
   No — out of scope. This plan stays focused on the empty-placeholder
   bug. The infra/red classification is a separate dormant feature
   (gateway routing, halt vs dispatch) that the original plan punted
   and this one does not pick up. Add a TODO comment at the new
   stash site so a future revival knows where to hook in.
3. **Should `${changed-files}` switch from live `git status` to a
   pre-captured snapshot delta for the unexpected-tests fixers?** Yes
   — same mechanism `fix-scope-diff` already uses. Capture once,
   reuse for both diagnostic fixers, eliminate the live-shell timing
   dependency. The live `git status` path stays as fallback for
   `fix-command-failed` (where no pre-snapshot exists because
   `runCommand` is the executor itself).
4. **Should we make `${verify-results}` and `${changed-files}`
   load-bearing (registered only when non-empty)?** Yes for
   `${verify-results}` — silent empty substitution is exactly the
   class of bug this plan exists to fix, and the load-bearing
   pattern catches the regression class at dispatch time rather than
   in the rendered prompt. No for `${changed-files}` — `fix-command-failed`
   legitimately dispatches when the failing command produced no
   working-tree changes (e.g. a syntax error in the runner command
   itself), so the empty case is real for at least one consumer and
   load-bearing would false-alarm.
5. **Should we capture `phase-changed-files` on the success path of
   `validateOutputsAndScopes` too, so it is always available for the
   downstream fix-unexpected-tests dispatch?** Yes. The existing
   action only stashes it on scope-diff failure; this plan flips
   that to "always stash, regardless of validation outcome." The
   downstream consumer set grows from {fix-scope-diff} to
   {fix-scope-diff, fix-unexpected-failing-tests,
   fix-unexpected-passing-tests}. The trace state-delta stays
   readable because the stash happens at validate time, not at
   dispatch.
6. **Anything for `unexpected-passing-tests-fixer`?** Yes — it
   consumes the same two placeholders and has the same bug. Every
   item below covers both fixers symmetrically; the wiring fixes
   are in the dispatcher and action layer, not per-agent.

## Items

### 1. Stash `verify_results_text` from `runCommand` on test-run failure

**Files touched:**

- `internal/atdd/runtime/actions/bindings.go`

**Change:** in `runCommand`, on the `isTestRun && !succeeded` branch,
stash a formatted block to `ctx.State["verify_results_text"]`. Use a
helper that returns:

- `stdout` alone if `stderr` is empty,
- `stderr` alone if `stdout` is empty,
- `<stdout-tail>\n--- stderr ---\n<stderr-tail>` if both are non-empty,

with both streams individually capped via `lastNLines(s,
commandStderrTailLines)`. Stash unconditionally on the test-run
failure path (the placeholder is fixer-only; other consumers ignore
it).

Add a TODO comment immediately above the stash referencing
`verify_classify.go` so a future revival of the infra/red classifier
knows this is the canonical hook point.

The stash MUST happen on the `isTestRun && !succeeded` branch
specifically — not on every `runCommand` failure. The
`fix-command-failed` placeholder set (`command-line` /
`command-exit-code` / `command-stderr-tail`) already covers
non-test-run failures; adding `verify_results_text` to those would
register a placeholder the prompt body does not reference.

### 2. Make `${verify-results}` load-bearing in `clauderun.renderPrompt`

**Files touched:**

- `internal/atdd/runtime/clauderun/clauderun.go`

**Change:** in the unconditional placeholder map (clauderun.go:665-676),
move `"verify-results": opts.VerifyResults` out of the map and into a
conditional block immediately after — register the placeholder ONLY
when `opts.VerifyResults != ""`. Pattern matches the existing
`Language` / `Checklist` / `AcceptanceCriteria` / `CommandLine`
blocks at clauderun.go:689-755.

Keep `"changed-files": opts.ChangedFiles` unconditional — per
question 4, `fix-command-failed` legitimately dispatches with an
empty working tree.

Update the `VerifyResults` field comment (clauderun.go:146-153) to
match the actual wiring: "Stamped by `runCommand` on the
`isTestRun && !succeeded` branch; flows through
`verify_results_text` in `ctx.State` to the driver's
`cOpts.VerifyResults`. Registered only when non-empty so an absent
value surfaces via `findUnfilledPlaceholders` rather than silently
substituting "" — same rationale as `CommandLine`." Drop the old
`verifyCommandResult` reference (that type does not exist in the
current codebase).

### 3. Always stash `phase-changed-files` in `validateOutputsAndScopes`

**Files touched:**

- `internal/atdd/runtime/actions/bindings.go` (the
  `validateOutputsAndScopes` action)

**Change:** the action currently stashes `phase-changed-files` ONLY
on the scope-diff failure branch (per the comment at the action's
`Writes:` section: "Not stashed on success: no downstream consumer
reads it then"). Lift that stash out of the failure branch so it
runs unconditionally after the snapshot-delta computation, regardless
of whether scope validation passes or fails.

Update the `Writes:` block comment to reflect the new contract:

> - `ctx.State["phase-changed-files"]` — set on every dispatch
>   (success and failure). Newline-joined sorted list of every path
>   in the snapshot delta. Consumed by `fix-scope-diff` (this MID's
>   own failure-kind), `fix-unexpected-failing-tests`, and
>   `fix-unexpected-passing-tests` to scope their reasoning to "what
>   the WRITE phase just edited." Replaces the previous live
>   `git status --porcelain` shell-out at dispatch time, which was
>   fragile against working-tree state (clean tree at dispatch ≠ no
>   WRITE-phase changes).

The trace state-delta sort by value-length (per `f35afc0`) handles
the now-always-present key gracefully — the empty-delta case
serialises as `""`, which sorts to the top and stays readable.

### 4. Switch `fixChangedFiles` to prefer `phase-changed-files` for the unexpected-tests fixers

**Files touched:**

- `internal/atdd/runtime/driver/driver.go` (the `fixChangedFiles`
  helper at lines 1050-1074)

**Change:** extend the case label that already short-circuits
`fix-scope-diff` to its pre-captured stash so it ALSO covers
`fix-unexpected-failing-tests` and `fix-unexpected-passing-tests`:

```go
switch agent {
case "fix-scope-diff",
    "fix-unexpected-failing-tests",
    "fix-unexpected-passing-tests":
    if v := ctx.GetString("phase-changed-files"); v != "" {
        return v
    }
    // Fall through to the live git-status fallback. The stash should
    // always be present after Item 3 lands; the fallback exists for
    // defence-in-depth (e.g. tests that bypass validateOutputsAndScopes).
case "fix-command-failed", "fix-missing-output":
    // Live git-status only — no pre-snapshot exists for these dispatches.
default:
    return ""
}
```

Drop the existing `if agent == "fix-scope-diff"` block (its work is
now in the switch case). Keep the live `git status --porcelain`
shell-out as the fallback tail for both branches so the existing
behaviour holds when the snapshot stash is absent (defensive — Item 3
guarantees it).

Update the `fixChangedFiles` function comment (lines 1030-1049) to
state the new precedence: snapshot-stash for the three fixers that
have one, live shell-out for the two that don't.

### 5. Update `ChangedFiles` field comment in `clauderun.go`

**Files touched:**

- `internal/atdd/runtime/clauderun/clauderun.go` (the `ChangedFiles`
  field comment at lines 155-161)

**Change:** rewrite to match the new wiring:

> ChangedFiles carries the working-tree path list the WRITE phase
> produced, for substitution into fix-unexpected-passing-tests' /
> fix-unexpected-failing-tests' / fix-scope-diff's / fix-command-failed's
> / fix-missing-output's `${changed-files}` placeholder. For the
> three diagnostic fixers with a pre-WRITE snapshot
> (`unexpected-passing-tests`, `unexpected-failing-tests`,
> `scope-diff`), the driver reads the snapshot delta from
> `ctx.State["phase-changed-files"]` (always stashed by
> `validateOutputsAndScopes` per plan 20260527-1536); the live
> `git status --porcelain` fallback runs only when the stash is
> absent. For `fix-command-failed` and `fix-missing-output` (no
> pre-WRITE snapshot), the driver always uses the live shell-out.
> Registered unconditionally — `fix-command-failed` may legitimately
> dispatch with an empty working tree.

### 6. Unit tests

**Files touched:**

- `internal/atdd/runtime/actions/bindings_test.go`

**Change:** add or extend tests covering:

- `runCommand` on `isTestRun && !succeeded` stamps
  `verify_results_text` to a non-empty value containing both stdout
  and stderr tails (a chatty stdout case to confirm `lastNLines`
  caps both streams).
- `runCommand` on `isTestRun && succeeded` does NOT stamp
  `verify_results_text` (so a later fixer dispatch via an unrelated
  failure-kind does not inherit a stale value).
- `runCommand` on a non-test-run failure (e.g. `gh optivem commit`)
  does NOT stamp `verify_results_text` (per Item 1's
  test-run-only-branch constraint).
- `validateOutputsAndScopes` stashes `phase-changed-files` on the
  validation-success path (the inverse of the existing
  scope-diff-failure test).

Reuse the existing `fakeShell` / `ShellResult` test scaffolding at
`bindings_test.go:91`. No new helper types needed.

### 7. Driver unit test for the new `fixChangedFiles` precedence

**Files touched:**

- `internal/atdd/runtime/driver/driver_test.go`

**Change:** add a test covering the new switch case in
`fixChangedFiles`:

- For `fix-unexpected-failing-tests` and
  `fix-unexpected-passing-tests`, with `phase-changed-files` stashed
  in ctx, the function returns the stash content without shelling
  out.
- For the same two agents, with no stash, the function falls back
  to the live `git status` path (mock the shell-out or assert the
  function returns "" in a non-git directory).
- For `fix-command-failed` and `fix-missing-output`, the stash is
  IGNORED — the live shell-out path is always taken.

If existing `fixChangedFiles` tests in `driver_test.go` already
cover the `fix-scope-diff` case, extend them to also assert the
new fixer names rather than duplicating the harness.

## Verification

Out-of-scope for agent execution; for the operator after Items 1-7 land:

- Re-run the `gift-wrap an order` rehearsal (or any rehearsal whose
  change-cycle reaches a verify-tests-fail branch). Inspect the
  next `*-unexpected-failing-tests-fixer.prompt.md` log under
  `.gh-optivem/runs/<ts>/`. Confirm:
  - The `### Verify results to address` block contains the test
    runner's captured stdout/stderr tail, not an empty line.
  - The `### Changed files from the WRITE phase` block lists the
    files the preceding WRITE phase modified, not an empty line.
- Trigger a synthetic test-runner failure by tampering with a known
  test (e.g. invert an assertion in a contract test the WRITE phase
  is expected to satisfy) and re-run the slice. Confirm the
  unexpected-failing-tests-fixer now receives non-empty inputs and
  diagnoses against the actual stderr text.
- Confirm `findUnfilledPlaceholders` fires when an upstream regression
  silently stops stamping `verify_results_text` (e.g. comment out
  the new stash temporarily, re-run, see the dispatch fail fast
  rather than silently render an empty placeholder).

## Non-goals

- Reviving `classifyShellErr` and routing infra-vs-red verify
  failures. That is the deferred half of the original plan and stays
  deferred — this plan only fixes the empty-placeholder bug. A TODO
  comment at the new stash site marks the hook point for the future
  revival.
- Changing the fixer agents' prompt bodies. The prompts already
  describe the inputs correctly (`### Verify results to address`,
  `### Changed files from the WRITE phase`); only the wiring is
  broken.
- Making `${changed-files}` load-bearing. `fix-command-failed`
  legitimately dispatches with an empty working tree (syntax error
  in the runner command, etc.), so a non-empty assertion would false-
  alarm.
- Replacing the live `git status --porcelain` shell-out for
  `fix-command-failed` or `fix-missing-output`. Those dispatches do
  not have a pre-WRITE snapshot to read from — `runCommand` IS the
  executor for command-failed, and `validateOutputsAndScopes` may
  fire on missing-output before any working-tree change exists.
- Backporting the snapshot-delta to `fix-command-failed` by having
  `runCommand` take its own pre/post snapshot. The command-failed
  dispatch is not behaviour-preserving the way the unexpected-tests
  fixers are — the changed-files set is less load-bearing for its
  diagnosis, and the live shell-out is good enough.

## Cross-references

- Symptom report and root-cause walk: conversation
  `C:\GitHub\optivem\academy\worktrees\rehearsal-20260527-135607` at
  `.gh-optivem/runs/20260527-115613/010-unexpected-failing-tests-fixer.prompt.md`.
- Dormant infra/red classifier (Non-goal #1):
  `internal/atdd/runtime/actions/verify_classify.go`.
- Load-bearing placeholder precedent (Item 2):
  `internal/atdd/runtime/clauderun/clauderun.go:163-210` (CommandLine,
  FailingTaskName, MissingOutputs, ViolatingPaths).
- Snapshot-delta precedent (Items 3-4):
  `internal/atdd/runtime/actions/bindings.go` — `snapshot-working-tree`
  + `validateOutputsAndScopes` scope-diff branch.
- Fixer prompts that consume the placeholders:
  `internal/assets/runtime/agents/atdd/unexpected-failing-tests-fixer.md`,
  `internal/assets/runtime/agents/atdd/unexpected-passing-tests-fixer.md`.
