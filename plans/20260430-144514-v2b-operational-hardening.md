# v2b: operational hardening for clauderun auto-dispatch

## Motivation

v2b shipped on 2026-04-30 (gh-optivem `fcf4d0c`): `gh optivem atdd …` now auto-dispatches each ATDD agent through the `claude` CLI by default, and the v1 two-window workflow is reachable via `--manual-agents`. Implementation, unit tests, and prompt template are in place — but four operational concerns the v2b design plan called out remain unaddressed because they only surface during real-world runs:

1. **Subscription rate limits.** A fully autonomous run on a complex ticket can burn through a Pro plan's weekly cap. Today the driver halts on whatever non-zero exit code `claude -p` returns when the cap is hit, but the message bubbled up is generic ("clauderun: atdd-X exited non-zero: exit status 1") — operator has to read the subprocess stderr to find the actual cause.
2. **Repo-state assumptions.** The driver assumes the working tree is clean before each user_task and that the agent commits on the same branch. If the agent leaves untracked files, the HEAD-diff detection succeeds but those files are stranded outside the commit. If the agent switches branches mid-run, HEAD on the original branch is unchanged → driver halts with "no commit produced" even though the agent did work.
3. **Auth surface on CI.** The `claude` CLI looks up auth from `~/.claude/`. In a CI runner or any non-interactive context, that directory is empty unless someone has run `claude /login` as the executing user. There's no documented bootstrap path, and the failure surfaces as a confusing "claude: command not found" or "auth required" deep in the subprocess output.
4. **Interactive-mode commit detection.** The dispatcher waits for the subprocess to exit (via `cmd.Run`) and only then diffs HEAD. The intent — "let the operator keep chatting after a commit lands" — is correctly implemented (we don't advance until `/exit`), but it's unverified against an actual interactive session. There's a subtle failure mode where the operator commits, then `git reset`s back, then `/exit`s: the driver advances on stale HEAD info.

These concerns are unblockers for production-grade soak, not enhancements; until they're addressed v2b is "works on my machine" software.

## Items

- [ ] **Item 4: Soak interactive mode against real ATDD runs** — ⏳ Deferred: requires real-ticket runs over ~1–2 weeks of ATDD work, not implementable from a code-edit session. Goals:
  - Run `gh optivem atdd implement-ticket --issue <N>` against ~5 real shop tickets.
  - Track per agent name: misroute count (Claude Code interprets "Launch the X subagent" as something other than a `Task` call with `subagent_type: X`), no-commit-but-clean-exit count, branch-switch incidents (item 2's check now logs these explicitly).
  - Capture the failure modes (and operator workarounds) in a soak log.
  - After ~10 tickets, decide whether the prompt template needs tuning and whether `--manual-agents` is being used as a workaround for any specific class of failure.

## Trigger to undefer

Items 1–3 shipped on 2026-05-01. Item 4 (the soak) is deferred until the operator drives ~5–10 real tickets through `gh optivem atdd implement-ticket` and can report the per-agent misroute / no-commit / branch-switch counts.

## See also

- v2b implementation commit: `gh-optivem fcf4d0c` ("atdd(driver): auto-dispatch agents via clauderun…").
- Parent design plan (deleted but recoverable from git history): the file `plans/20260430-134908-v2b-auto-dispatch-via-claude-cli.md` carried the original risks list under "Risks / open questions" — this plan inherits items 4–7 from that section.
- Sister plan: `plans/20260430-133420-config-driven-pipeline-labels.md` — orthogonal to this one; either can be picked up first.
- v2b code:
  - `internal/atdd/runtime/clauderun/` — subprocess engine, prompt template, banners.
  - `internal/atdd/runtime/driver/driver.go` — `newClaudeRunDispatcher`, `newManualAgentDispatcher`.
  - `internal/atdd/runtime/override/middleware.go` — override hook publication into Context.
