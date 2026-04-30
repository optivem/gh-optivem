# v2b: auto-dispatch ATDD agents via the `claude` CLI

**Status: Deferred.** Open this plan after the v1 soak (parent plan `shop/plans/20260429-211522-script-vs-agent-atdd-orchestration.md` items 1–2) confirms the human-in-the-loop driver is stable and tokens drop ≥30%.

## Motivation

v1 (shipped) is *human in the loop*: the Go driver pauses at every `user_task` node, the operator switches to a Claude Code window, asks Claude to launch the named agent, the agent commits, the operator returns to the terminal and presses Enter. Two windows, lots of context-switching per ticket.

v2b collapses Window 2 into the driver. When the driver hits a `user_task`, instead of pausing for stdin, it shells out to the `claude` CLI with a constructed prompt that names the agent and the phase context. Claude Code runs the agent in its existing tool runtime (Read/Edit/Write/Bash, MCP servers, subagent isolation, model defaults), produces a commit, exits. The driver detects the new HEAD and advances.

**Why CLI shell-out, not the Anthropic Go SDK?** Three reasons:

1. **Architecture** — the SDK approach (call `anthropic-sdk-go` directly) duplicates Claude Code: tool-use loop in Go, transcript management, MCP plumbing, model selection. ~half a week of work and ongoing maintenance. CLI shell-out reuses Claude Code's runtime as the agent execution backend; ~150 lines of Go.
2. **Cost predictability** — `claude` CLI authenticates with the operator's existing Pro/Max subscription. Usage counts toward the weekly cap; no separate API-key billing. SDK hits `api.anthropic.com` and bills against API credits, separately metered (~$2.50–$5.50/ticket on Sonnet, ~$13–$26/ticket on Opus).
3. **Auth simplicity** — no API-key management in the Go binary, no CI secret to provision, no "where is `ANTHROPIC_API_KEY` set?" gotchas.

This is the natural progression after v1 stabilises, not a parallel track.

## Design

### Subprocess invocation

For each `user_task` node whose `agent:` is something other than `human`, the driver runs:

```
claude -p "<constructed-prompt>" --no-update-check
```

`-p` is non-interactive print mode. The prompt is constructed from:

- Agent name (e.g. `atdd-test`).
- Phase doc path (e.g. `docs/atdd/process/at-red-test.md`).
- Ticket context — issue number, title, repo, project URL — pulled from `Context` keys already populated by `preResolveIssue`.
- Per-node override text from the override hook (`--extra <NODE>="..."`), already plumbed in v1 as a no-op decorator; v2b is what actually consumes it.

Prompt template (sketch):

```
Launch the {{agent}} subagent for the current ATDD phase.

Ticket: #{{issue_num}} "{{issue_title}}" ({{issue_repo}})
Project: {{project_title}} ({{project_url}})
Phase: {{node_description}}
Phase doc: {{phase_doc}}

{{override_text}}

When the agent finishes, do not summarise — exit cleanly. The Go driver
detects completion by polling HEAD; your COMMIT must land on HEAD before
you exit.
```

Claude Code is expected to interpret "Launch the X subagent" as a `Task` tool call with `subagent_type: X`, since that's how it currently routes when a user types the same phrase interactively.

### Commit detection

The driver doesn't need the agent's output text — it needs to know that a commit landed. Approach:

1. Before invocation: capture `git rev-parse HEAD` → `headBefore`.
2. Run the subprocess. Block until exit.
3. After exit: capture `git rev-parse HEAD` → `headAfter`.
4. If `headBefore != headAfter`: success, advance the engine. Read `git log -1 --format=%s` to surface the commit subject in the driver's stdout.
5. If `headBefore == headAfter` and exit status is 0: agent ran clean but produced no commit. Treat as failure (`Outcome.Err`), surface the agent's stdout for diagnosis, halt — same shape as v1's "abort" path.
6. If exit status is non-zero: surface stderr, halt.

This works because the v1 process-flow YAML already pairs every `user_task` with a downstream `commit_phase` service task (or, at WRITE phases, expects the agent to have committed). The convention is "agent commits, driver verifies" — v2b just makes the verification programmatic.

### Override hooks

v1 ships `internal/atdd/runtime/override/` with `--extra <NODE>="..."`, `--replace <NODE>="..."`, `--interactive` already wired as decorators. v1 has no CLI surface for them; v2b adds:

- `--extra AT_RED_DSL_WRITE="prefer record types"` → `override_text` interpolated into the prompt template above.
- `--replace AT_RED_DSL_WRITE="<full prompt>"` → swaps the entire constructed prompt for the user-supplied one (escape hatch).
- `--interactive` → before each dispatch, print the constructed prompt and read stdin to allow last-minute additions; useful for debugging.

The decorator wrapping is already in place; v2b just fills in the bodies.

### Autonomous mode

v1's `--autonomous` flag is mostly stubbed. v2b makes it meaningful:

- **Without `--autonomous`** — between every two `user_task` dispatches, the driver pauses for human "Continue?" approval. Default. Matches today's v1 behaviour but condensed (one pause per agent run instead of two — "before launch" and "after commit" — collapsed into a single post-run gate).
- **With `--autonomous`** — no human gates between user_tasks. The driver runs the entire flow start to end, dispatching agents back-to-back. Service-task gates (e.g. "Run the smoke test now") still pause because they're not agent-driven.

`--autonomous` is what makes v2b *fully* automated. Without it, v2b is "Window 2 collapsed" but the operator still confirms each phase. Both modes ship.

## Implementation outline

1. **New package `internal/atdd/runtime/clauderun/`** — owns subprocess invocation. Single exported function `Dispatch(ctx, opts) (CommitInfo, error)` that:
   - Builds the prompt from a template + opts.
   - Runs `claude -p <prompt> --no-update-check`.
   - Captures HEAD before/after.
   - Returns `CommitInfo{SHA, Subject}` or an error with surfaced stderr.
   - Has a `ClaudeRunner` interface so tests can inject fakes (mirrors `gates.GhRunner` / `actions.realShell`).

2. **Driver wiring** (`internal/atdd/runtime/driver/driver.go`):
   - `wrapAgentDispatchers` currently registers a "pause-and-prompt" wrapper; v2b registers a "dispatch via clauderun" wrapper instead.
   - Mode selection: stdin pause vs auto-dispatch is a driver `Options` field (default = auto-dispatch in v2b; v1 behaviour reachable via `--manual-agents` for fallback).
   - Read overrides from the new CLI flags into `override.Hooks` and pass into `clauderun.Dispatch`.

3. **CLI flags** (`atdd_commands.go`):
   - `--extra NODE=text` (repeatable; parsed into a map).
   - `--replace NODE=text` (repeatable).
   - `--interactive` (bool).
   - `--manual-agents` (bool; v1 stdin-pause behaviour for diagnosis).

4. **Prompt template** — short Markdown / plain-text in `clauderun/prompt.tmpl` (Go `text/template`), parameterised on the fields above. Versioned in source so prompt drift is reviewable.

5. **Tests:**
   - `clauderun_test.go` — fake `ClaudeRunner` returning canned exit codes + canned HEAD-changes; assert prompt construction, commit detection, error paths.
   - Integration test in `internal/atdd/runtime/driver/driver_test.go` (new file) wiring a fake clauderun + a tiny in-memory git repo, verifying the engine advances on commit and halts on no-commit.

6. **Documentation:**
   - `CONTRIBUTING.md` "Testing the ATDD driver" section gains a v2 paragraph: same workflow, now without Window 2 unless using `--manual-agents`.
   - One-liner in shop's `docs/atdd/config.yaml` example pointing at the v2 mode.

Estimated total effort: 2–3 days of focused work plus a soak.

## Risks / open questions

1. **"Launch the X subagent" prompt phrasing.** Whether Claude Code reliably routes that to a `Task` tool call with `subagent_type: X` — or whether we need a more explicit instruction — must be verified by hand before committing to the prompt template. Test against all 9 agent names (atdd-{story,bug,task,chore,test,dsl,driver,backend,frontend}).
2. **`claude -p` behaviour during long agent runs.** A WRITE phase that takes 5–10 minutes — does `-p` mode handle that gracefully, or does it have an internal timeout we need to flag in `--max-turns`? Verify before shipping.
3. **Streaming output.** v1 watched the agent in Window 2 in real time. v2b's `claude -p` runs to completion before returning, so the driver sees nothing until exit. We should at minimum tail the agent's stderr to the driver's stdout during the run (subprocess plumbing) so the operator can see progress; "live" tool-use streaming would need `claude --stream` (does that exist?) or polling a sidecar log. v1 stdout-only is acceptable for v2b's MVP.
4. **Subscription rate limits.** A fully autonomous run might burn through a Pro plan's weekly cap on a single complex ticket. Document the failure mode (rate-limit error mid-run); halt with a clear message rather than a partial state.
5. **Repo-state assumptions.** The driver assumes the working tree is clean before each user_task and that the agent commits on the same branch. If the agent leaves untracked files, the diff detection above misses them. Consider adding an explicit "clean state" pre-condition (verify decorator) before each dispatch.
6. **Auth surface.** `claude` CLI looks up auth from the user's home directory (`~/.claude/`). In CI / non-interactive runners, this needs `claude /login` to have been run as the executing user. Document the bootstrap.

## Trigger to undefer

Any of:

- v1 soak passes (parent plan items 1–2 marked done) — token reduction confirmed, no human-in-the-loop regressions.
- Operator fatigue from the two-window workflow becomes the primary friction.
- A teammate / second consumer wants to run the pipeline non-interactively (e.g. nightly batch tickets in CI).

Until then: v1's manual dispatch is fine and v2b stays parked here.

## See also

- Parent design plan: `shop/plans/20260429-211522-script-vs-agent-atdd-orchestration.md` — motivation, v1 architecture, and the sessions that delivered v1.
- Sister plan: `gh-optivem/plans/20260430-133420-config-driven-pipeline-labels.md` — also deferred; orthogonal to v2b. Either can be picked up first.
- v1 driver code: `gh-optivem/internal/atdd/runtime/driver/driver.go` — the `wrapAgentDispatchers` and `promptForAgent` functions are what v2b replaces.
- v1 override hook: `gh-optivem/internal/atdd/runtime/override/` — already-shipped no-op decorators that v2b lights up.
