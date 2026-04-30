# Minimize tokens and latency in clauderun agent dispatch

## Motivation

Concrete waste observed during a v2b rehearsal run on issue #61 (`atdd-task` agent, interactive mode). The host `claude` session — whose only instruction was the templated *"Launch the atdd-task subagent for the current ATDD phase"* — performed six sequential tool calls before invoking the requested subagent:

1. `git status && git log --oneline -10`
2. `gh optivem atdd state` *(errored — command doesn't exist; the host invented it)*
3. `gh optivem --help`
4. `gh issue view 61`
5. `git diff HEAD~5..HEAD --stat`
6. `gh issue view 61` *(duplicate of #4)*

…then finally launched `atdd-task` via the Agent tool.

This pre-investigation is doubly wasteful:

- **Tokens:** each tool result is materialised in the host's conversation, paid for on every subsequent turn until session end.
- **Latency:** six sequential round-trips (~5–10 seconds wall-clock) before the actual subagent spawn.
- **Functionally useless:** subagent context is isolated. Nothing the host fetched is visible to the subagent — the subagent will redo the work via its own onboarding (e.g. `atdd-task.md` step 1 mandates `gh issue view`).

The dispatch design intent — "host is a thin shim, subagent does the work" — is correct. The implementation just doesn't enforce it. This plan tightens the dispatch path.

## Items

Ordered by ROI. Items 1–3 are pure gh-optivem changes; items 4–7 require coordination with shop.

### 1. Tighten the prompt template to forbid pre-investigation

**File:** `internal/atdd/runtime/clauderun/prompt.tmpl`

Current template is permissive — it tells the host *what* to do (launch the subagent) but not *what not to do*. Append an explicit dispatch-only section:

```
This is a dispatch-only session. Your job is to invoke the
{{.Agent}} subagent via the Agent tool and exit. Do NOT:

- Run git status, git log, git diff, gh issue view, gh optivem,
  or any other tool before dispatching.
- Read files, search the repo, or inspect ticket state.
- Summarise after the subagent returns — exit cleanly.

The subagent has its own onboarding and runs in isolated context —
anything you fetch here is wasted tokens and latency. Pass the
issue number and phase context through to the subagent and let it
do its own onboarding.
```

Place this BEFORE the existing "When the agent finishes, do not summarise" sentence (which already handles the post-dispatch yapping problem; the new section handles the pre-dispatch yapping problem).

**Estimated effort:** 30 minutes including a unit test that asserts the new clause is rendered.

**Risk:** none. Worst case the host still pre-investigates (model doesn't honour the prompt strictly) — same state as today.

### 2. Restrict the host's tool surface in autonomous mode

**File:** `internal/atdd/runtime/clauderun/clauderun.go` → `execClaude.Run`

In **autonomous** mode (`claude -p`), the host has no legitimate reason to call any tool other than `Agent` (to dispatch the subagent). Pass `--allowed-tools=Task` to make pre-investigation literally impossible — any `Bash`/`Read`/`Grep` call by the host fails fast and the model is forced to dispatch.

```go
if opts.Autonomous {
    args = append(args, "-p", opts.Prompt, "--allowed-tools", "Task")
}
```

In **interactive** mode this is the wrong restriction (the operator may want to chat with the host after dispatch, inspect repo state, etc.) — leave unrestricted.

**Estimated effort:** 1 hour including verification that the `--allowed-tools` flag exists in the supported `claude` CLI version. If the flag is named differently, adjust.

**Risk:** if the host genuinely needs to read one file before dispatching (unlikely given the design), this hard-blocks it. Mitigation: monitor the first few autonomous runs after rollout for unexpected dispatch failures.

### 3. Disable update check on every dispatch

**File:** `internal/atdd/runtime/clauderun/clauderun.go` → `execClaude.Run`

Pass `--no-update-check` (or whatever the equivalent flag is in the current `claude` CLI) so each dispatch doesn't pay the cost of checking for a new CLI version. Per ATDD pipeline run there are 4–8 dispatches; even 100ms per dispatch adds up.

**Estimated effort:** 15 minutes.

**Risk:** tiny — operator may run a stale CLI longer. Solo runs of `claude` outside the orchestrator will still update-check, so this only suppresses inside the dispatch path.

### 4. Pass the resolved issue body into the subagent prompt (eliminates per-subagent `gh issue view`)

**Files:**
- `internal/atdd/runtime/clauderun/clauderun.go` → `Options` (add `IssueBody`, `IssueLabels`, `IssueProjectItems` fields)
- `internal/atdd/runtime/clauderun/prompt.tmpl` (interpolate the new fields)
- `internal/atdd/runtime/driver/driver.go` → `preResolveIssue` (fetch body + labels + projectItems alongside title)
- shop: `.claude/agents/atdd/atdd-{task,test,driver,dsl,backend,frontend,chore,bug,story,release}.md` — remove the "Fetch the issue with `gh` before proceeding" mandate; replace with "issue context is already pasted into your prompt".

The orchestrator already calls `gh issue view` once during pre-resolve to get the title. Extending that single call to also fetch body + labels + projectItems is free. Pasting the result into every subsequent subagent prompt eliminates 4–8 redundant `gh issue view` calls per ticket cycle (one per phase).

**Coordination:** this is a coupled change across both repos. Land gh-optivem side first (additive — agents that still call `gh issue view` keep working), then update each agent in shop.

**Estimated effort:** half a day on gh-optivem side; one or two days for the shop-agent updates.

**Risk:** issue state may change mid-cycle (e.g. operator edits labels while ATDD is running). Mitigation: the orchestrator re-fetches on each dispatch, not just at session start, so each subagent prompt has a fresh snapshot.

### 5. Audit per-agent model assignments in shop

Not a gh-optivem change but listed here for completeness because it directly impacts every dispatch. The `claude` CLI honours the `model:` line in the agent's frontmatter. Today every ATDD agent sets `model: opus` (verified for `atdd-task`; assumed for the rest until audited).

Mechanical agents could drop to a smaller/faster model with no quality loss:

| Agent | Current | Proposed | Rationale |
|---|---|---|---|
| `atdd-dispatcher` | opus | haiku | Classification is mechanical |
| `atdd-release` | opus | haiku | Removing `@Disabled` + committing is mechanical |
| `atdd-bug` | opus | sonnet | Gherkin authoring from STR — pattern-following |
| `atdd-story` | opus | sonnet | Same — Gherkin from user story |
| `atdd-test`, `atdd-driver`, `atdd-dsl`, `atdd-task`, `atdd-backend`, `atdd-frontend`, `atdd-chore` | opus | opus | Heavy implementation work — keep |

Each tier-down on a hot-path agent is roughly: cost ÷ 5–10×, latency ÷ 2–3× per turn.

**Coordination:** purely shop-side. Land after item 1 so the dispatch is well-behaved before changing model assignments.

**Estimated effort:** 2 hours including a soak run on each retiered agent.

### 6. Add token/latency observability to the exit banner

**File:** `internal/atdd/runtime/clauderun/clauderun.go` → `writeExitBanner`

`claude -p --output-format json` emits a final JSON object with `total_cost_usd` and `usage.{input,output,cache_creation,cache_read}_tokens`. Capture it and surface in the green "EXITED AGENT" banner alongside the elapsed time and commit SHA. Helps the operator (and future-you) spot the agents bleeding the most tokens.

```
✅ EXITED AGENT: committed abc1234  (47s, 12.4k in / 1.8k out, $0.18)
   "atdd: red phase — failing test"
```

For interactive mode the JSON output isn't available — gracefully degrade to today's elapsed-time-only banner.

**Estimated effort:** 3–4 hours including JSON parse + per-agent total in the final session summary.

**Risk:** the JSON envelope shape is `claude` CLI-specific; pin it behind a feature gate so a CLI version bump can't break the dispatch path.

### 7. Background long-running shell commands in heavy agents

Not a gh-optivem change. Some ATDD agents shell out to `./compile-all.sh`, `./gradlew build`, or `./test-all.sh`, which can run 30–120 seconds. If invoked with default (foreground) `Bash`, the model sits in context the entire time.

Add a guideline in `docs/atdd/process/*` and reinforce in each implementation agent's body: any shell command expected to take > 10 seconds should use `run_in_background: true` and poll for completion via `BashOutput`.

**Coordination:** purely shop-side.

**Estimated effort:** 2 hours.

## Tradeoff

The aggressive items (1, 2, 4) all narrow the host's discretion. If a real ATDD scenario emerges where the host genuinely needs to investigate before dispatching (some kind of conditional routing the prompt template can't express), these items would block it. Mitigation: items 1–2 can be loosened independently if soak surfaces such a case; item 4 is purely additive (worst case the agent does its own `gh issue view` anyway and the prompt-pasted body is wasted bytes).

## Phased rollout

- **Phase 1** (this plan): items 1, 2, 3, 6 — pure gh-optivem changes. Land in one PR.
- **Phase 2** (sister plan in shop): item 5 (model retiering) + item 7 (background commands). Tracked separately because it's shop-side.
- **Phase 3** (coordinated): item 4 (issue-body pass-through). gh-optivem side ships additively first; shop-agent updates follow agent-by-agent.

Each phase ends with a soak run of `gh optivem atdd implement-ticket` against a real shop ticket to verify the dispatch path still produces a green commit.

## See also

- v2b implementation: `gh-optivem fcf4d0c` — established the auto-dispatch path this plan optimises.
- Sister plan: `plans/20260430-144514-v2b-operational-hardening.md` — orthogonal operational-hardening items (rate limits, branch detection, CI auth). Either can be picked up first.
- Source observation: rehearsal run on `optivem/shop` issue #61 (`Redesigning New Order UI`) on 2026-04-30.
