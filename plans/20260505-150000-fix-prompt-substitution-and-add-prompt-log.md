# Fix prompt substitution and add per-dispatch prompt log

## Motivation

A rehearsal run of `atdd implement-ticket --issue 61` (system-interface-redesign,
monolith/typescript scope) produced a misbehaviour the inner `atdd-task` agent
flagged itself: its prompt arrived with `Architecture:` blank and the
`Allowed write roots:` block empty. With no scope anchor, the agent fell back to
the embedded "parallel implementations" doctrine in
`docs/atdd/architecture/system.md` and edited all four trees
(`system/monolith/{java,dotnet,typescript}`, `system/multitier/frontend-react`)
plus the matching `system-test/{java,dotnet,typescript}` driver adapters,
instead of just the typescript pair the YAML scope intended.

Root cause is a wires-crossed plumbing bug between two halves of the
state-machine context:

- `seedScopeParams` (`internal/atdd/runtime/driver/driver.go:398-413`) writes
  `architecture` and `allowed_roots` into `sCtx.Params`.
- `newClaudeRunDispatcher` (`driver.go:628-629`) reads them back via
  `ctx.GetString(...)`, which `internal/atdd/runtime/statemachine/context.go:48`
  defines as a `c.State` lookup — `Params` is never consulted.

So the values are computed correctly from `gh-optivem.yaml`, written to one
map, and read from the other map. The dispatcher always gets `""`, the prompt
template's `${architecture}` and `${allowed_roots}` always expand to empty.

The other ticket fields (`issue_title`, `project_title`, `ticket_checklist`, …)
flow correctly because `preResolveIssue` writes them via `sCtx.Set(...)` →
`State`, lining up with what `GetString` reads. Only the two scope params
(written by `seedScopeParams`) hit the wrong map.

The bug went undetected because nothing in the dispatch path makes the rendered
prompt visible. `materializePrompt` only spills to a tempfile when the prompt
exceeds 8 KB (and `cleanup()` deletes that tempfile on exit anyway), so the
operator's only window into the rendered text was the inner agent happening to
echo a substring back. A persistent prompt log per dispatch would have made
this a 10-second diagnosis.

## Items

Ordered by dependency. Item 1 is the actual fix; items 2-5 are guardrails so
the next substitution bug surfaces in seconds, not in a multi-tree edit.

### 1. Move scope params from `Context.Params` to `Context.State`

**Files:**
- `internal/atdd/runtime/driver/driver.go` (`seedScopeParams`, ~10 lines)
- `internal/atdd/runtime/driver/driver_test.go` (any test that asserts on
  `sCtx.Params["architecture"]`)

**Change:** flip the four writes in `seedScopeParams` from
`sCtx.Params[k] = v` to `sCtx.Set(k, v)` (i.e. `repo_strategy`, `repos`,
`architecture`, `allowed_roots`). Rename the function to `seedScopeState` and
update its doc comment — the current docstring mis-describes the destination
(it says "Context.Params so agent prompts can substitute …" but the
substitution path actually reads from `State`).

**Why this is the right map:** `Context.Params` is documented as
"parameter substitutions for the current call_activity scope" — local YAML
flow variables like `${change_type}` or `${agent}` that swap on
call_activity entry/exit. The four scope facts are not call-scoped; they are
project-scoped and stable for the entire run. They belong next to
`issue_title`, `project_title`, `ticket_checklist`, etc., which the
dispatcher already reads via `GetString`.

**Risk:** if any predicate evaluation today inspects `architecture` or
`repo_strategy` via the `Params` map, that read would silently return empty
after this change. A grep for `Params["architecture"]` and
`Params["repo_strategy"]` across `internal/atdd/` should be near-zero today
(those names were introduced for the prompt-substitution path); confirm
during implementation and adjust if any callsite turns up.

**Effort:** ~15 minutes implementation, ~30 minutes test.

### 2. Persist the rendered prompt to a per-dispatch run log

**Files:**
- `internal/atdd/runtime/clauderun/clauderun.go` (`Options`, `Dispatch`)
- `internal/atdd/runtime/driver/driver.go` (per-run timestamp + per-dispatch
  sequence, threaded into `cOpts.PromptLogPath`)

**Change:**

1. Add `Options.PromptLogPath string` to `clauderun.Options`. Empty → no log
   (preserves test-side ergonomics).
2. In `Dispatch`, after `renderPrompt` returns and before invoking the runner,
   write the rendered prompt to `PromptLogPath` (creating parent dirs as
   needed). I/O failure here is a non-fatal warning to `opts.Stderr`, not a
   halt — diagnostics shouldn't break the dispatch.
3. In the driver, compute the path per-dispatch:
   `<repoPath>/.gh-optivem/runs/<run-ts>/<seq>-<agent>.prompt.md`
   - `<run-ts>` is set once at `driver.Run` start (e.g.
     `time.Now().UTC().Format("20060102-150405")`).
   - `<seq>` is a 3-digit zero-padded counter incremented per dispatch
     (`001-atdd-task.prompt.md`, `002-atdd-test.prompt.md`, …) so log files
     sort in dispatch order regardless of clock granularity.
   - The counter lives on a struct shared by every dispatcher closure created
     in `wrapAgentDispatchers` (currently nothing of the sort exists; smallest
     home is a `*atomic.Int64` captured by the closure factory).

**Lifetime:** these files are persistent diagnostics, not runtime artifacts.
The `materializePrompt` tempfile (8 KB-spillover, deleted on dispatch exit)
keeps its current lifetime — different concern, different lifecycle. The two
files share the same rendered text but only when the prompt is large enough
to spill.

**Prune-on-start (default 10 most recent runs):** at the start of
`driver.Run`, list `<repoPath>/.gh-optivem/runs/<run-ts>/` entries, sort by
mtime descending, delete all but the most recent N-1 (leaving room for the
run we're about to create). N defaults to 10, override via a new
`--keep-runs N` flag on `gh optivem atdd implement-ticket`. N=0 means "never
prune"; negative values are rejected. Pruning failures (e.g. permission
errors) are warnings to stderr, not halts.

**Effort:** ~half a day, including the counter plumbing and prune-on-start.

### 3. Halt before claude launch on unfilled `${…}` placeholders

**Files:**
- `internal/atdd/runtime/clauderun/clauderun.go` (after `renderPrompt`)
- `internal/atdd/runtime/clauderun/clauderun_test.go` (new cases)

**Change:** after `renderPrompt` returns the rendered string and before
`Dispatch` invokes the runner, scan for leftover `\$\{[a-zA-Z_][a-zA-Z0-9_]*\}`
matches. If any match, return an error of the form:

```
clauderun: prompt has unfilled placeholders after substitution: ${architecture}, ${allowed_roots}
  this usually means the field was not seeded into Context.State before dispatch — check seedScopeState and preResolveIssue
```

**Skip the check when `opts.RawPrompt != ""`** — the operator's `--replace`
override is allowed to contain literal `${…}` if the operator wants. This
matches the documented "RawPrompt wins" rule the rest of `Dispatch` already
honours.

**Why a leftover-marker check rather than a per-field schema:** "no leftover
markers" is the smallest correct guardrail and would have caught this exact
bug. A per-field schema ("architecture is required when …") is more work and
duplicates information already encoded in the prompt template (the template
is the source of truth for which placeholders matter).

**Effort:** ~1 hour.

### 4. `.gitignore` ensure-line for `.gh-optivem/`

**Files:**
- `internal/atdd/runtime/driver/driver.go` (idempotent ensure-line at
  `driver.Run` start, alongside the prune-on-start step)
- `internal/steps/optivem_yaml.go` (or a sibling `ensure_gitignore.go`) for
  the `gh optivem config init` path
- shop's scaffolder template (one-line addition to the committed `.gitignore`)

**Change:** ensure `.gh-optivem/` is on a line of its own in
`<repoPath>/.gitignore`. Idempotent: if the line already exists (with or
without trailing slash), no-op; otherwise append. Create the file if missing.

**Why two ensure-line callsites:** `config init` covers the happy path
(fresh consumer repo, scaffolder runs once); the at-Run check covers the
upgrade path (existing consumer repo never re-scaffolded after this change
landed). Both paths converge on the same helper. The shop template update is
belt-and-braces — fresh shops will scaffolded with the line baked in.

**Effort:** ~1 hour, including a small unit test for the idempotent helper.

### 5. Pre-dispatch "Prepared Prompt" summary banner (+ `--show-prompt` for full text)

**Files:**
- `internal/atdd/runtime/clauderun/clauderun.go` (new banner before
  `writeEnterBanner`; `Options.ShowPrompt bool`)
- `internal/atdd/runtime/driver/driver.go` (wire `--show-prompt` into
  `cOpts.ShowPrompt` for every dispatch)
- `cmd/.../implement-ticket` flag definition (`--show-prompt`)

**Change:** add a structured summary banner that prints between the trace
entry and the `ENTERING AGENT` banner on every dispatch. Always-on. Format:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
📋 PREPARED PROMPT for atdd-task
   size:           2.3 KB
   architecture:   monolith
   allowed roots:  system/monolith/typescript, system-test/typescript, +2 external
   checklist:      2 items (2 already [x])
   override text:  (none)
   log:            .gh-optivem/runs/20260505-150000/001-atdd-task.prompt.md
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

Every field falls directly out of the existing `Options` struct
(`Architecture`, `AllowedRoots`, `Checklist`, `OverrideText`) plus
`len(prompt)` for size — nothing new to plumb.

**Why a summary, not the full prompt by default:** prompts are 2-3 KB and a
full ticket cycle dispatches 6-10 agents. Printing every byte to stdout
drowns the trace in scrollback, especially in interactive rehearsal flows.
The summary catches the bug class we just hit (substitution failure: the
`architecture: ` line would be visibly empty) without the noise.

**`--show-prompt` flag:** when set, dump the full rendered prompt to stdout
between the summary banner and the `ENTERING AGENT` banner. Off by default.
Useful for debugging template edits or auditing a new agent.

**Naming choice:** `--show-prompt` over `--print-prompt` because POSIX
tradition reads `print` as "emit and exit" (`find -print`, `rustc --print=…`).
Our flag still dispatches afterwards. Reserve `--print` semantics for the
deferred `gh optivem atdd render-prompt` subcommand (Out of scope, below) if
it ever lands.

**Skip the banner when `opts.RawPrompt != ""`:** the summary's substitution
fields (`architecture`, `allowed roots`, etc.) come from `Options` fields
that the operator's `--replace` override deliberately bypasses. Show a
shorter banner in that case (`override mode — N bytes`) so the operator
still sees that a dispatch is happening, but don't pretend to introspect
fields that weren't used.

**Effort:** ~1-2 hours including a unit test that the banner reflects what
`Options` actually carries, and a second test for the `--show-prompt` flag
path.

### 6. Regression test: end-to-end prompt substitution through the driver

**Files:**
- `internal/atdd/runtime/driver/driver_test.go` (new test) or
  `internal/atdd/runtime/clauderun/clauderun_test.go` if a driver-level
  fixture is too heavy.

**Change:** drive a fake `clauderun.Options` build through the same
`seedScopeState` + `newClaudeRunDispatcher` path as production, with a fake
runner that captures the prompt argument it receives. Assert the captured
prompt contains:

- `Architecture: monolith` (substituted, not the literal `${architecture}`).
- `- System: system/monolith/typescript (lang: typescript)` line.
- `- System tests: system-test/typescript (lang: typescript)` line.
- The external-systems block with both `Stubs:` and `Simulators:` lines.

The existing tests at `clauderun_test.go:201` and `:241` only check that the
phrase `Allowed write roots:` appears (they pass with the value missing
after the colon, which is exactly today's bug). The new test must assert on
the *substituted values*.

**Bonus assertion:** after the dispatch, the file at
`<tmpRepoPath>/.gh-optivem/runs/<run-ts>/001-atdd-task.prompt.md` exists
and matches the captured prompt byte-for-byte. This pins the log
write down end-to-end alongside the substitution.

**Effort:** ~1-2 hours including fake-runner plumbing if not already present.

## Out of scope

- Per-field validation ("architecture is required when …"): see item 3
  rationale. The leftover-marker check is the smallest correct guardrail.
- Auto-cleanup at session end: explicitly rejected during design — would
  defeat the purpose for the most common diagnostic case (post-run
  "what did we send the agent?"). Prune-on-start (item 2) gives bounded disk
  use without losing yesterday's run.
- A `gh optivem atdd render-prompt --issue X --node Y` dry-run subcommand:
  nice-to-have for prompt-template work but not load-bearing for this fix.
  Defer to its own plan.
- Auditing whether `seedScopeState` should also seed other fields (`repos`
  is currently set but no consumer reads it; `repo_strategy` likewise). Not
  on the path of this bug; defer.

## Total effort

~1-1.5 days for items 1-6, including tests. Items can land in a single PR —
they are mutually reinforcing (the fix enables the test; the log, summary
banner, and validator together make the next bug of this class a
10-second diagnosis instead of a multi-tree edit blowout).
