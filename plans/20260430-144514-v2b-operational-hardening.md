# v2b: operational hardening for clauderun auto-dispatch

## Motivation

v2b shipped on 2026-04-30 (gh-optivem `fcf4d0c`): `gh optivem atdd …` now auto-dispatches each ATDD agent through the `claude` CLI by default, and the v1 two-window workflow is reachable via `--manual-agents`. Implementation, unit tests, and prompt template are in place — but four operational concerns the v2b design plan called out remain unaddressed because they only surface during real-world runs:

1. **Subscription rate limits.** A fully autonomous run on a complex ticket can burn through a Pro plan's weekly cap. Today the driver halts on whatever non-zero exit code `claude -p` returns when the cap is hit, but the message bubbled up is generic ("clauderun: atdd-X exited non-zero: exit status 1") — operator has to read the subprocess stderr to find the actual cause.
2. **Repo-state assumptions.** The driver assumes the working tree is clean before each user_task and that the agent commits on the same branch. If the agent leaves untracked files, the HEAD-diff detection succeeds but those files are stranded outside the commit. If the agent switches branches mid-run, HEAD on the original branch is unchanged → driver halts with "no commit produced" even though the agent did work.
3. **Auth surface on CI.** The `claude` CLI looks up auth from `~/.claude/`. In a CI runner or any non-interactive context, that directory is empty unless someone has run `claude /login` as the executing user. There's no documented bootstrap path, and the failure surfaces as a confusing "claude: command not found" or "auth required" deep in the subprocess output.
4. **Interactive-mode commit detection.** The dispatcher waits for the subprocess to exit (via `cmd.Run`) and only then diffs HEAD. The intent — "let the operator keep chatting after a commit lands" — is correctly implemented (we don't advance until `/exit`), but it's unverified against an actual interactive session. There's a subtle failure mode where the operator commits, then `git reset`s back, then `/exit`s: the driver advances on stale HEAD info.

These concerns are unblockers for production-grade soak, not enhancements; until they're addressed v2b is "works on my machine" software.

## Items

### 1. Surface rate-limit hits clearly

When `claude -p` exits non-zero, capture the last ~20 lines of stderr and grep for known rate-limit / auth signatures. When matched, produce a dedicated error message: "Rate limit hit on Pro plan; weekly cap exhausted. Re-run after the next reset window or upgrade to Max." Otherwise fall through to today's generic message.

Implementation surface: `internal/atdd/runtime/clauderun/clauderun.go`'s `Dispatch` already wraps the runner error with `"clauderun: %s exited non-zero: %w"`. Extend the wrap with a small classifier that inspects the captured stderr (already streamed to opts.Stderr in autonomous mode — capture into a `bytes.Buffer` tee rather than discarding). Add unit tests with sample stderr from a real cap-hit run.

Estimated effort: 3–4 hours.

### 2. Detect leftover untracked files / wrong-branch commits

Before each user_task dispatch:
- Snapshot `git status --porcelain` → record initial untracked-file set + current branch name.
- After dispatch (subprocess exit, before HEAD diff): re-run `git status --porcelain` → diff against snapshot. Surface any new untracked files in the dispatch banner. Verify branch is unchanged; halt with a clear message if the agent switched branches.

This is a verify-decorator-shaped concern but lives more naturally inside the clauderun dispatcher because it needs to fire *between* subprocess exit and HEAD diff. Add it as a pre/post step in `clauderun.Dispatch`. The "warn about untracked" path is non-fatal (the operator may have intended them); the "branch switched" path halts.

Estimated effort: half a day including tests.

### 3. Document and test the CI auth bootstrap

Two pieces:
- **Doc:** add a "Running ATDD on CI" section to `CONTRIBUTING.md` covering: the `claude /login` requirement, where credentials live (`~/.claude/`), the option to bake credentials into a CI image vs mounting them at job start, and the failure mode when missing (with the actual error text).
- **Pre-flight check:** at driver startup (before walking the flow), if `Options.ManualAgents` is false, run `claude --no-update-check --version` once and surface a helpful error if it fails. This catches "no claude binary on PATH" and "not authenticated" before the engine begins, rather than after the operator has watched several service-task spinners scroll by.

Estimated effort: 2–3 hours.

### 4. Soak interactive mode against real ATDD runs

The interactive-mode flow (default) is unit-tested with fakes but unverified end-to-end. Goals:
- Run `gh optivem atdd implement-ticket --issue <N>` against ~5 real shop tickets.
- Track per agent name: misroute count (Claude Code interprets "Launch the X subagent" as something other than a `Task` call with `subagent_type: X`), no-commit-but-clean-exit count, branch-switch incidents.
- Capture the failure modes (and operator workarounds) in a soak log.
- After ~10 tickets, decide whether the prompt template needs tuning, whether item #2's branch-switch detection is worth shipping, and whether `--manual-agents` is being used as a workaround for any specific class of failure.

Estimated effort: scattered across one to two weeks of normal ATDD work; not a focused-implementation block.

## Trigger to undefer

This plan is **active**. Items 1–3 are pure code/doc work and can be picked up immediately. Item 4 needs operator availability to drive real tickets — it can run in parallel with 1–3 once at least one v2b run has happened.

## See also

- v2b implementation commit: `gh-optivem fcf4d0c` ("atdd(driver): auto-dispatch agents via clauderun…").
- Parent design plan (deleted but recoverable from git history): the file `plans/20260430-134908-v2b-auto-dispatch-via-claude-cli.md` carried the original risks list under "Risks / open questions" — this plan inherits items 4–7 from that section.
- Sister plan: `plans/20260430-133420-config-driven-pipeline-labels.md` — orthogonal to this one; either can be picked up first.
- v2b code:
  - `internal/atdd/runtime/clauderun/` — subprocess engine, prompt template, banners.
  - `internal/atdd/runtime/driver/driver.go` — `newClaudeRunDispatcher`, `newManualAgentDispatcher`.
  - `internal/atdd/runtime/override/middleware.go` — override hook publication into Context.
