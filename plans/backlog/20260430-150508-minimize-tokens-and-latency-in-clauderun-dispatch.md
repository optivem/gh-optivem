# Minimize tokens and latency in clauderun agent dispatch

> 📋 **Deferred-plan review (2026-06-04, re-verified against code): KEEP — two items actionable.** Phase 1 (Items 1, 2, 6) shipped.
> - **Item 3 is no longer deferred.** The old blocker has cleared: `--no-update-check` now exists and is already used in `preflight/tools.go:25`. The dispatch path (`claudeTuningArgs`, `clauderun.go:1669`) does NOT yet pass it. → ~1-line pickup, see Item 3 below.
> - **Item 4 is still genuinely pending, untouched.** Verified: `tracker.Issue` (`tracker.go:43`) carries only ID/Title/URL/Handle — no Body/Labels/projectItems field; `writeResolvedIssue` (`driver.go:787`) seeds only num/url/title/handle/id; `clauderun.Options`/`RunOpts` carry `IssueTitle` but no `IssueBody`; the shared preamble (`preamble.md:3`) interpolates only `${issue-num}` + `${issue-title}`. So each subagent still re-fetches the body itself.
> - Items 5/7 are shop-side. Trim the shipped Phase 1 items if revisited.
>
> Note: the orthogonal prose-trimming work (`runtime-prompts-audit` / `token-usage-audit`) shrinks static prompt bodies — a different axis. It does not touch this plan's two mechanisms (host pre-dispatch investigation; redundant per-subagent `gh issue view`), so the two stack rather than supersede.

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

Ordered by ROI. Items 4–7 require coordination with shop.

### 3. Disable update check on every dispatch — ✅ Actionable (blocker cleared)

**History:** originally deferred because the `claude` CLI exposed no `--no-update-check` flag as of 2026-04-30 (commit `aeeff82` had removed an earlier attempt for that reason). That blocker is now gone — the flag exists and is already used in `preflight/tools.go:25` (`claude --no-update-check --version`).

**Files:**
- `internal/atdd/runtime/clauderun/clauderun.go` → `claudeTuningArgs` (`clauderun.go:1669`)

**Fix:** prepend `"--no-update-check"` to the args slice in `claudeTuningArgs`. That function is the single chokepoint feeding both the interactive path (`execClaude.Run`) and the headless path (`runHeadless`), so one edit covers every dispatch. Each dispatch then skips the per-invocation update probe — a small but unconditional latency saving on the hot path.

**Estimated effort:** minutes; verify with one dispatch that the flag is accepted by the pinned `claude` version.

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

### 7. Background long-running shell commands in heavy agents

Not a gh-optivem change. Some ATDD agents shell out to `./compile-all.sh`, `./gradlew build`, or `./test-all.sh`, which can run 30–120 seconds. If invoked with default (foreground) `Bash`, the model sits in context the entire time.

Add a guideline in `docs/atdd/process/*` and reinforce in each implementation agent's body: any shell command expected to take > 10 seconds should use `run_in_background: true` and poll for completion via `BashOutput`.

**Coordination:** purely shop-side.

**Estimated effort:** 2 hours.

## Tradeoff

The aggressive items (1, 2, 4) all narrow the host's discretion. If a real ATDD scenario emerges where the host genuinely needs to investigate before dispatching (some kind of conditional routing the prompt template can't express), these items would block it. Mitigation: items 1–2 can be loosened independently if soak surfaces such a case; item 4 is purely additive (worst case the agent does its own `gh issue view` anyway and the prompt-pasted body is wasted bytes).

## Phased rollout

- **Phase 1** ✅ shipped: items 1, 2, 6 — pure gh-optivem changes.
- **Phase 1b** (pure gh-optivem, now unblocked): item 3 (`--no-update-check` on dispatch). ~1-line change in `claudeTuningArgs`; can land independently of everything below.
- **Phase 2** (sister plan in shop): item 5 (model retiering) + item 7 (background commands). Tracked separately because it's shop-side.
- **Phase 3** (coordinated): item 4 (issue-body pass-through). gh-optivem side ships additively first; shop-agent updates follow agent-by-agent.

Each phase ends with a soak run of `gh optivem atdd implement-ticket` against a real shop ticket to verify the dispatch path still produces a green commit.

## See also

- v2b implementation: `gh-optivem fcf4d0c` — established the auto-dispatch path this plan optimises.
- Sister plan: `plans/20260430-144514-v2b-operational-hardening.md` — orthogonal operational-hardening items (rate limits, branch detection, CI auth). Either can be picked up first.
- Source observation: rehearsal run on `optivem/shop` issue #61 (`Redesigning New Order UI`) on 2026-04-30.
