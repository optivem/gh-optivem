# CLI owns commit, agent never touches git

## Plan revision (2026-05-02)

The plan was written 2026-04-30 against the world where leaf agent prompts lived in `shop/.claude/agents/atdd/*.md` and `gh-optivem` had a `clauderun/prompt.tmpl`. Two commits since then have moved that ground:

- `bea4977` (gh-optivem, 2026-05-01) — embedded the leaf prompts into `internal/atdd/runtime/agents/prompts/*.md` (one file per agent, with the `shared-commit-confirmation.md` content inlined as a `### Reference:` block in each). Deleted `prompt.tmpl`.
- `f1e30634` (shop, 2026-05-01) — deleted the consumer-side leaf prompts from `shop/.claude/agents/atdd/`. The `shop/docs/atdd/process/shared-commit-confirmation.md` source-of-truth is unchanged.

Net effect on this plan:

- **Item 4** target moves from `prompt.tmpl` to the preamble of each `internal/atdd/runtime/agents/prompts/atdd-*.md` (the "your COMMIT must land on HEAD" sentence now lives there).
- **Item 5** target moves from `shop/.claude/agents/atdd/*.md` to `internal/atdd/runtime/agents/prompts/atdd-*.md` in gh-optivem. Items 4 and 5 now both edit the same set of 11 files in one pass; they are merged into a single item below.
- **Item 6** (mirror in `rehearsal-atdd-cli`) is obsolete — there is no such sibling repo; rehearsal runs are throwaway worktrees that pick up whatever `gh-optivem` ships.
- **Item 7** (delete `shared-commit-confirmation.md` in shop, fix cross-refs) is unchanged.

Items renumbered below: 1, 2, 3 unchanged; old 4+5 → new 4 (combined); old 6 dropped; old 7 → new 5.

## Motivation

Today's design has a contradiction between the CLI side and the agent side of dispatch.

- **CLI side** — `internal/atdd/runtime/clauderun/clauderun.go` polls `git HEAD` to detect agent completion (lines 254–258, `errNoCommit` if HEAD unchanged), and each leaf prompt's preamble (e.g. `atdd-task.md:9`) instructs the agent: *"your COMMIT must land on HEAD before you exit."* The runtime is built on the assumption that the agent commits autonomously.
- **Agent side** — leaf agent definitions (e.g. `atdd-task.md` step 5) say *"After WRITE, STOP. Do NOT continue,"* and they embed the `shared-commit-confirmation.md` rule, whose text is *"No agent may run `git commit` … without first asking the user 'Can I commit?'"* The agent is built on the assumption that a human approves every commit interactively.

This contradiction surfaced during a v2b rehearsal on issue #61 (`rehearsal/atdd-cli` branch, 2026-04-30): the `atdd-task` agent finished WRITE, stopped at the commit gate, and asked the operator. The CLI's HEAD-poll never advanced because the agent (correctly per its own rules) never committed.

The clean resolution is to move the commit step out of the agent entirely:

1. The agent window is purely creative work — write, human reviews, agent reworks, loop until the human is satisfied.
2. The human exits the agent window when satisfied. Exit *is* the approval signal.
3. The CLI then stages the working-tree changes and commits with a templated message built from known context (phase, ticket, agent, diff stat).

Why this is the right split:

- **No agent intelligence is needed for the commit message.** Phase, ticket number/title, and agent name are already in `clauderun.Options`; `git diff --stat` supplies the file list. Asking the agent to compose a commit message is paying tokens for a mechanical step.
- **Single point of control.** Staging policy, message format, and (later) sign-off / hook handling live in one place instead of drifting across N leaf agents.
- **No `shared-commit-confirmation.md` rule to maintain.** That rule exists to gate a thing the agent shouldn't be doing in the first place.
- **Human gate stays where it belongs.** The WRITE-STOP inside the agent window already gives the human review/rework loop. We don't need a *second* "Can I commit?" gate after the human has already exited the window — exit is the gate.

## Items

- [ ] **5. Delete `shared-commit-confirmation.md` and rewrite cross-refs (was item 7)** — ⏳ Deferred: gated on default flipping to `--cli-commits=on` and rehearsals running green (steps 3 of "Order of operations"). Until then, `shop`'s shared doc is still imported by the legacy-mode prompt path that gh-optivem swaps back when `--cli-commits=off`. Re-open this item once the flag default flips.

**File:** `docs/atdd/process/shared-commit-confirmation.md` in `shop`. (rehearsal-atdd-cli is obsolete — see plan revision note.)

The file exists to enforce the "agent asks before committing" rule. Once the agent doesn't commit, the rule is gone — there's nothing left to confirm. Keeping a file by that name to describe "how the CLI commits" is a misleading filename, which is worse than no file.

Steps:

- Delete the file in `shop`.
- Grep `shop/docs/atdd/process/cycles.md`, `shared-ticket-status-in-acceptance.md`, `task-and-chore-cycles.md`, and any other process docs for references to `shared-commit-confirmation.md` and remove or rewrite them. The expected mentions are short pointers like "see shared-commit-confirmation.md" — these can be deleted outright since the new flow needs no equivalent gate doc.
- If a CLI commit policy doc proves valuable later (operator confusion, audit requirement), write it fresh under an accurate name — e.g. `cli-commit-policy.md` — or fold a short paragraph into `cycles.md`. Do not pre-emptively create one as part of this plan.

**Estimated effort:** 30 minutes including the grep-fix pass.

## Out of scope

- The WRITE-STOP gate inside the agent window. That stays; this plan is about who runs `git commit`, not whether the human reviews.
- Phase boundary gates between agents (e.g. "human confirms before the next agent starts"). If we want those, that's a separate plan in the CLI orchestrator, not here.
- Sign-off, GPG signing, hooks. The CLI commits without `--no-verify` by default; whatever pre-commit hooks the repo defines will run. Sign-off / GPG is a follow-up.

## Order of operations for landing this

1. ~~Land items 1–3 in `gh-optivem` behind a `--cli-commits` flag (default off) so existing rehearsals keep working.~~ **Done.**
2. ~~Land item 4 in `gh-optivem` (prompt edits) — gated to only apply when `--cli-commits` is on.~~ **Done (2026-05-02).** Prompts now ship in their CLI-commits target state; `clauderun.applyCommitGating` reverse-substitutes the legacy preamble + commit-confirmation reference block when `--cli-commits=off`. Scope extended to 11 files (added `atdd-bug.md` and `atdd-story.md` for preamble parity; the plan's strict 9-file list missed them — they had the same legacy preamble line).
3. ~~Flip the default in `gh-optivem` (`--cli-commits` becomes default-on, with `--agent-commits` as the legacy escape hatch).~~ **Done (2026-05-02).** `--cli-commits` now defaults to `true` and `--agent-commits` is the documented legacy escape hatch (errors if both are explicitly passed). Wiring lives in `atdd_commands.go:resolveCommitMode`.
4. Land item 5 in `shop` (delete shared doc, fix cross-refs) once the default has flipped and rehearsals have run green.
5. Remove `--agent-commits` after one full soak window. At that point also delete `internal/atdd/runtime/agents/shared/legacy-commit-confirmation.md` and the `applyCommitGating` legacy branch.
