# Plan: BPMN commit subprocess must pass a commit message to `gh optivem commit`

## Context

`gh optivem implement` reached its first BPMN-driven commit step during a rehearsal of issue 69 (`bash ../gh-optivem/scripts/atdd-rehearsal.sh 69 --config gh-optivem-monolith-typescript.yaml`) and failed:

```
[trace 10:43:33] > RUN_COMMAND  kind=service-task action=run-command
$ gh optivem commit
ERROR: commit message is required (no default).
       Pass it as the last positional argument, e.g.:
         gh optivem commit "<message>"
         gh optivem commit --repo myrepo "<message>"
run-command: shell "gh optivem commit": exit status 1
[trace 10:43:34] OK RUN_COMMAND -> command-exit-code=1, …, failure-kind=command-failed
```

This is the first time the BPMN-driven `commit` subprocess has been exercised since the CLI was changed to require a positional message.

## Root cause

The `commit` subprocess in `internal/atdd/runtime/statemachine/process-flow.yaml:1815-1831` hardcodes the command line with no positional argument:

```yaml
commit:
  name: "Commit"
  start: EXECUTE_COMMAND
  nodes:
    - id: EXECUTE_COMMAND
      type: call-activity
      process: execute-command
      name: "Dispatch the Command"
      params:
        command: "gh optivem commit"        # ← no message, no message param
```

All four BPMN call sites that invoke this subprocess (`COMMIT_TEST_CODE`, `COMMIT_SYSTEM`, `COMMIT_TESTS`, `COMMIT_LAYER`) likewise pass no params. `execute-command` → `runCommand` then dispatches the bare string verbatim (`internal/atdd/runtime/actions/bindings.go:722-734`).

This worked under earlier `gh optivem commit` behaviour, which had a default message. Commit `a9347a3` ("workspace: port commit, sync, check-actions, rate-limit subcommands") changed the CLI to make the positional message mandatory (see `errMissingMsg` in `cross_repo_commands.go:322-324` and the help text at `:94`: "A commit message is required when any iterated repo has dirty changes"). The BPMN side was not updated.

The `runCommand` action already has a precedent for splicing typed params into a command line: lines 727-733 detect `gh optivem test run` and append `--suite=` / `--test=` via `shellEscape`. We extend that precedent to `gh optivem commit`.

## Decisions resolved (best long-term, autonomous)

Four design decisions resolved upfront so executors aren't stalled mid-implementation.

### 1. Message is spliced by `runCommand`, not by string-templating in YAML.

The `commit` subprocess gains a `message:` input param. `runCommand` detects a leading `gh optivem commit` and, when `ctx.Params["message"]` is non-empty, appends ` "<escaped-message>"` using the existing `shellEscape` helper.

**Why action-side splicing, not `command: "gh optivem commit \"${message}\""`:** string interpolation in YAML cannot safely handle messages containing `"`, `$`, backticks, or backslashes; the `shellEscape` helper exists precisely to centralise that escaping inside the action. This also matches the `test run` precedent at `bindings.go:727-733` — no new mechanism.

**Why not introduce a generic `args:` list for `execute-command`:** would force a wider change across every existing call site that uses `command: …` as a bare string today, for one extra caller. The `test run` precedent shows command-specific splicing is the accepted shape; we follow it.

### 2. All four call sites supply the same literal `message:` shape.

| Call site                    | Subprocess containing it                       | `message:` literal                          | Example                                       |
|------------------------------|------------------------------------------------|---------------------------------------------|-----------------------------------------------|
| `COMMIT_TEST_CODE`           | `write-and-verify-acceptance-tests`            | `"[${ticket_id}] ${issue_title}"`           | `[69] Add product search`                     |
| `COMMIT_SYSTEM`              | `implement-and-verify-system`                  | `"[${ticket_id}] ${issue_title}"`           | `[69] Add product search`                     |
| `COMMIT_TESTS`               | `refactor-and-verify-tests`                    | `"[${ticket_id}] ${issue_title}"`           | `[69] Add product search`                     |
| `COMMIT_LAYER`               | `implement-test-layer` (parameterised)         | `"[${ticket_id}] ${issue_title}"`           | `[69] Add product search`                     |

**Why `${ticket_id}` and `${issue_title}` are safe to splice:** both are stamped into the BPMN context by `writeResolvedIssue` at `internal/atdd/runtime/driver/driver.go:565-571`, which runs in the dispatcher's pickup path *before* any cycle subprocess starts. They are therefore in scope at every `commit` call site without any new state plumbing. (An earlier version of this plan claimed the BPMN had no named access to ticket-id at commit time; that was incorrect — see driver.go:565-571.)

**Why the same message for all four sites (not phase-qualified):** keeps this plan minimal — the rehearsal-unblocking change is just "give `commit` a message at all". Phase enrichment (suffixing `acceptance test code` / `system implementation` / `test refactor` / `${cycle_phase} layer`) is a follow-up tracked separately at `plans/upcoming/20260527-1108-bpmn-commit-phase-suffix.md`, since it needs its own decision on whether `cycle_phase` is the right granularity (vs. `${suite} ${cycle_phase}`).

**Why no operator-overridable detail:** writing agents do not currently emit a `commit-message` output. Adding one is a separate change — see "Out of scope" below.

### 3. `commit` subprocess body declares `message:` as a required input param.

```yaml
commit:
  name: "Commit"
  start: EXECUTE_COMMAND
  nodes:
    - id: EXECUTE_COMMAND
      type: call-activity
      process: execute-command
      name: "Dispatch the Command"
      params:
        command: "gh optivem commit"
        message: ${message}
```

Callers MUST bind `message:` explicitly. Mirrors the "strict-mode rule" already established for `verify-tests-pass` (`process-flow.yaml:1841-1846`): omitting a required input is a wiring bug, not a permissive default.

**Why strict over default-empty:** an unset `message:` propagates as the literal string `${message}` through `coerceStateValue`, which `runCommand` would then splice into the command line as `gh optivem commit "${message}"`. That's a worse failure mode than today (silent miscommit instead of loud crash). Strict-mode dispatch (refuse to expand unbound `${message}`) is the existing convention for `suite:` / `test-names:`; we follow it.

### 4. `runCommand`'s commit splice is gated on the command prefix, like `test run`.

```go
isCommit := strings.HasPrefix(cmd, "gh optivem commit")
if isCommit {
    if msg := strings.TrimSpace(ctx.Params["message"]); msg != "" {
        cmd += " " + shellEscape(msg)
    }
}
```

The `isCommit` prefix guard means `ctx.Params["message"]` is ignored for any other command — a stray `message:` binding on, say, an `EXECUTE_COMMAND` whose `command:` is `gh optivem test run` is silently a no-op rather than a corruption.

**Why no error on `message:` set for a non-commit command:** matches today's behaviour for `suite:` / `test-names:` on commands other than `test run` (also silently ignored at `bindings.go:728-733`). Tightening to error would be a separate hygiene change orthogonal to this fix.

## Items

### Item 1 — Add `message` param to the `commit` subprocess

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml` (around line 1815-1831).

Insert `message: ${message}` into the `params:` block of `commit`'s `EXECUTE_COMMAND` node. Leave the `command:` literal as `"gh optivem commit"` — message splicing happens in `runCommand`, not via interpolation.

Add a comment above the subprocess explaining the strict-mode requirement (mirrors the `suite:` comment at `:1043-1049`).

### Item 2 — Bind `message:` at the four call sites

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

For each of the four `process: commit` call-activity nodes, add a `params: { message: "[${ticket_id}] ${issue_title}" }` block:

- `COMMIT_TEST_CODE` (~`:789-792`)
- `COMMIT_SYSTEM` (~`:1053-1056`)
- `COMMIT_TESTS` (~`:1100-1103`)
- `COMMIT_LAYER` (~`:1182-1185`)

All four sites use the identical literal — `${ticket_id}` and `${issue_title}` are stamped by `writeResolvedIssue` (`driver/driver.go:565-571`) before any cycle subprocess starts, so they're in scope at every commit site without new state plumbing.

### Item 3 — Extend `runCommand` to splice the commit message

**File:** `internal/atdd/runtime/actions/bindings.go` (around line 727-733).

Add the `isCommit` branch alongside `isTestRun`:

```go
isCommit := strings.HasPrefix(cmd, "gh optivem commit")
if isCommit {
    if msg := strings.TrimSpace(ctx.Params["message"]); msg != "" {
        cmd += " " + shellEscape(msg)
    }
}
```

Update the doc comment on `runCommand` to mention the new param alongside the existing `suite` / `test-names` enumeration.

### Item 4 — Tests

**File:** `internal/atdd/runtime/actions/bindings_test.go`

Add three cases:

1. `command: "gh optivem commit"` + `message: "[69] Add product search"` → shell line equals `gh optivem commit '[69] Add product search'`.
2. `command: "gh optivem commit"` + empty `message` → command params binding is a wiring bug; assert via the strict-mode path (`commit` subprocess refuses to expand). Cover at the YAML-level test if `bindings_test.go` is not the right layer — see Item 5.
3. `command: "gh optivem commit"` + `message: "msg with 'quote' and $var"` → shell line round-trips correctly via `shellEscape`.

**File:** `internal/atdd/runtime/statemachine/run_test.go`

If any existing fixture invokes the `commit` subprocess and asserts on the dispatched command line, update it to expect the new ` "<message>"` suffix. (Verify during execute: grep `run_test.go` for `gh optivem commit` and update matchers.)

### Item 5 — Strict-mode regression test for unbound `${message}`

**File:** `internal/atdd/runtime/statemachine/run_test.go` (new test case)

Assert that invoking the `commit` subprocess without a `message:` binding from the caller produces a clear wiring-bug error (not a silent dispatch with literal `${message}` in the command). This catches future call sites that forget to bind. Pattern: same shape as the existing `suite:` strict-mode regression if one exists; otherwise add a fresh one.

### Item 6 — Re-run the rehearsal end-to-end

After Items 1-5 land, re-run `bash ../gh-optivem/scripts/atdd-rehearsal.sh 69 --config gh-optivem-monolith-typescript.yaml` and walk it past the first commit gate. Confirms the four call sites all reach `gh optivem commit "<message>"` successfully and that the BPMN does not regress on any other action.

## Out of scope

- **Writing-agent-authored commit messages.** Letting the `write-acceptance-tests` / `implement-system` / `refactor-tests` agents emit a `commit-message` output (e.g. via `gh optivem output write commit-message=…`) for `runCommand` to consume in place of the literal is a richer change with its own design questions (allowlist of agents permitted to author messages, default fallback if the agent skips, prompt-body update for each agent). Tracked as a follow-up; not gating the rehearsal unblock.
- **Ticket-ID / sequence-number injection.** Threading the picked-ticket identifier (currently held only in the dispatcher's local scope) through to commit time is a wider state-stash change. The git history is recoverable without it; revisit only if operators ask.
- **Other CLI-shape drifts.** `gh optivem sync`, `gh optivem check-actions`, `gh optivem rate-limit` were ported in the same `a9347a3` change. None are dispatched from process-flow.yaml today (grep confirms), but if/when they are, they may have analogous CLI-vs-BPMN gaps. Audit as a separate hygiene pass.
- **Diagram regeneration.** The `regenerate-diagram` GH Actions workflow regenerates `docs/process-diagram.md` and `docs/images/*.svg` on push to main; do not include a local regen step here.

## Open questions

None — all resolved upfront:

1. ~~Phase grain for `COMMIT_LAYER`~~ — deferred to `plans/upcoming/20260527-1108-bpmn-commit-phase-suffix.md`; this plan keeps all four sites on the same `[${ticket_id}] ${issue_title}` shape.
2. Strict-mode regression home → **`run_test.go`**: the existing `${suite}` strict-mode regression lives there (`run_test.go:886` onwards), so Item 5 follows that precedent.
3. Existing fixtures pinning `gh optivem commit` → **`bindings_test.go:379` and `:544`** pin the bare `gh optivem commit` command line for failure-routing and empty-flag-leak coverage respectively. Both continue to pass after the change because they leave `message:` unbound (which produces no splice); leave them as-is — they pin a different invariant than Item 4's new cases.

## Verification

- `go test ./internal/atdd/...` passes (use `-p 2` per the Windows-test memory).
- `bash scripts/atdd-rehearsal.sh 69 --config gh-optivem-monolith-typescript.yaml` walks past `COMMIT_TEST_CODE` (and ideally further) without `commit message is required` errors.
- `git log` on the rehearsal branch shows the per-site message literals from Decision 2.
