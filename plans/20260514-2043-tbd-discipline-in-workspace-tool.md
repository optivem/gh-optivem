# Build TBD discipline into `gh optivem workspace`

> 🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-05-14T19:55:37Z`

> ✅ **STATUS: READY TO EXECUTE.** This plan captures the gap between `docs/tbd.md` and the current behaviour of `gh optivem workspace commit` / `sync`, and proposes a layered set of changes. All open decisions have been resolved — see "Decisions resolved" at the bottom.

## Context

`docs/tbd.md` is the canonical statement of how this repo (and every repo scaffolded by `gh optivem init`) practises trunk-based development. The workspace tool — specifically `gh optivem workspace commit` and `gh optivem workspace sync` in `workspace_commands.go` — is the daily on-ramp to that discipline: it is what an operator runs to stage, commit, pull, and push across every academy repo.

Today the tool *implements* TBD shape only partially, and *teaches* it not at all. Most of the discipline lives in the operator's global git config and in the doc. If the config drifts or the operator is new to the machine, the tool will silently do the wrong thing.

The bot in `.github/workflows/gh-bump-patch-version.yml` lands `Bump VERSION ...` commits directly on `main`. `docs/tbd.md:151-169` documents the resulting race in detail. The tool does not handle that race today.

## Current behaviour, against `docs/tbd.md`

Per-repo loop in `workspace_commands.go:130-158` is `stage → confirm → commit → git pull → git push`. `workspace sync` (`:310-315`) is the same minus the commit.

| `docs/tbd.md` rule | Tool today | Gap |
|---|---|---|
| `pull --rebase` is the default | `runGit(repo, "pull")` — no explicit `--rebase` | **Yes** — config-dependent. Silent merge commits on `main` if a teammate hasn't run the one-time setup. |
| Race-retry loop ("pull + push again") | One-shot push; `error` on rejection (`:154`) | **Yes** — bot race is documented (`tbd.md:151-169`) but ignored. |
| Pull *before* editing, again *before* push | Only the second pull happens | Partial — operator may commit onto stale local trunk; conflicts hit a dirty working tree instead of a clean one. |
| Never force-push `main` | Nothing checks | No defense-in-depth. |
| Linear trunk history | Not verified | No lint anywhere. |
| Scaled TBD: branch + rebase + `--force-with-lease` + squash/rebase-merge | Not modelled | Tool only knows the plain-TBD shape. |
| One-time git setup (`pull.rebase`, `rebase.autoStash`, `rerere.enabled`) | No `doctor` command | Operator has to remember on every new machine. |

## Proposed work

Layered so each layer is shippable on its own. Don't pre-commit to layers 2 and 3 — the value of doing them depends on whether layer 1 actually changes operator behaviour.

### Layer 1 — Stop relying on operator config (small, near-zero risk)

The shortest path to making the tool's behaviour match the doc regardless of how the operator's machine is configured.

1. **Explicit `--rebase` on every `pull`.** Change `runGit(repo, "pull")` to `runGit(repo, "pull", "--rebase")` in both `runWorkspaceCommit` (`:150`) and `runWorkspaceSync` (`:310`). Two-line change.
2. **Push-rejected retry loop.** Wrap the push: on non-fast-forward rejection → `git pull --rebase` → retry (cap 3 attempts). Log a generic "racing origin/main, retrying…" so the operator sees what is happening. This is the literal loop on `tbd.md:46` and resolves the bot-race at `:160-165`. Message stays generic — no pattern-match on the bot's commit format, since the bot is one of several possible race sources (other operators, future bots).
3. **Pre-commit pull.** Insert `git pull --rebase` *before* `commitOneRepo` so commits land on current trunk. Matches the "pull → edit → commit → pull → push" shape in `tbd.md:38-44`. Avoids the "fresh commit then conflicting rebase" foot-gun. Because the working tree is dirty by definition at this point (operator has unstaged work), the tool explicitly stashes unstaged changes, rebases, then `stash pop` — emulating `rebase.autoStash` regardless of the operator's config.
4. **`gh optivem doctor`.** Verify `pull.rebase=true`, `rebase.autoStash=true`, `rerere.enabled=true`; `--fix` to set them. Replaces "copy these three commands out of the doc" with "run one command." New top-level command. Narrow scope: config only. Broader repo-health checks are out of scope here.

### Layer 2 — Make TBD visible in the UX

Operators learn the model by seeing the tool name it.

5. **Mode banner.** At the top of `commit` and `sync`, for each repo print "plain TBD (on `main`)" or "Scaled TBD (on `<branch>`, upstream `origin/<branch>`)". One line per repo. Costs nothing, surfaces the doc's framing where work happens.
6. **`main` force-push guard.** Before `push`, compare `HEAD` with `@{u}` via `git rev-list --left-right`; if local has rewritten history *and* current branch is `main`, abort with a pointer to `docs/tbd.md`. Defense-in-depth against the one rule that doesn't bend.
7. **Pre-push hook installer** (`gh optivem hooks install`). Drops a `.git/hooks/pre-push` that blocks `--force*` on `main`. Belt-and-suspenders the operator can't bypass with a global config tweak.

### Layer 3 — Scaled-TBD primitives

Today the tool knows nothing about feature branches. If `docs/tbd.md`'s Scaled-TBD section is actually used in practice (open question — see decisions below), these encapsulate the rituals:

8. **`gh optivem branch start <name>`** = `git checkout main && git pull --rebase && git checkout -b <name>`.
9. **`gh optivem branch refresh`** = `git fetch origin && git rebase origin/main && git push --force-with-lease` — the ritual on `tbd.md:75-81`. Hardcodes `--force-with-lease`; plain `--force` is not an option.
10. **`gh optivem pr merge`** defaults to `--squash` or `--rebase`; rejects `--merge`. Wrapper over `gh pr merge`.

### Layer 4 — Drift detection

Catch the case where the doc and the repo have drifted apart.

11. **`gh optivem workspace lint-history`** — for each repo, `git log --merges --first-parent main` over the last N commits; flag any hits. Ships with a paired GitHub Actions workflow that fails on any new merge commit on `main`. Both shipped together — local for ad-hoc, CI for guarantee.
12. **`gh optivem workspace stale-branches`** — list branches in each workspace repo older than ~24h (per `tbd.md:62`); helps Scaled-TBD teams notice when "hours, not days" has slipped.

## Affected commands

Single reference for every command this plan touches. Each entry says what the command is, whether it's existing or new, and which layer items modify it.

### Modified (existing)

**`gh optivem workspace commit`** — layers 1.1, 1.2, 1.3, 2.5, 2.6
Iterates every repo declared in the resolved `*.code-workspace` file, stages changes, prompts for confirmation, commits with a supplied message, then pulls and pushes. Today it does `git pull` (relying on the operator's global `pull.rebase`) followed by a one-shot `git push`. The plan tightens it: explicit `--rebase` on the pull, a retry loop when push is rejected, an additional pull *before* committing, a one-line "plain TBD / Scaled TBD" mode banner per repo, and an abort if the push would rewrite history on `main`.

**`gh optivem workspace sync`** — layers 1.1, 1.2, 2.5
Same iterate-every-repo loop as `commit`, but without the staging step — just pull and push. Used to bring every repo declared in the resolved `*.code-workspace` file up to date with its remote. The plan applies the same fixes that aren't commit-specific: explicit `--rebase`, push-retry loop, mode banner.

### New

**`gh optivem doctor`** — layer 1.4
A one-shot health check. Verifies the three git config keys that `docs/tbd.md` requires (`pull.rebase=true`, `rebase.autoStash=true`, `rerere.enabled=true`) and reports pass/fail. With `--fix`, sets the missing ones. Replaces "copy three commands out of the doc onto each new machine" with one command. Scope open per decision 3 (config-only, or broader repo-health checks?).

**`gh optivem hooks install`** — layer 2.7
Installs a `.git/hooks/pre-push` in the current repo that refuses `--force` / `--force-with-lease` against `main`. Belt-and-suspenders enforcement of the "never force-push main" rule. Unlike a global config setting, the operator can't bypass it accidentally. Idempotent; safe to re-run.

**`gh optivem branch start <name>`** — layer 3.8
Encapsulates the Scaled-TBD branch-start ritual: `git checkout main && git pull --rebase && git checkout -b <name>`. Prevents the common foot-gun of branching off a stale local `main`. One command instead of three.

**`gh optivem branch refresh`** — layer 3.9
Encapsulates the Scaled-TBD "main moved while my PR was open" ritual: `git fetch origin && git rebase origin/main && git push --force-with-lease`. The exact sequence in `docs/tbd.md:75-81`. Hardcodes `--force-with-lease`; plain `--force` is not exposed, so the operator can't pick the dangerous variant.

**`gh optivem pr merge`** — layer 3.10
Wrapper over `gh pr merge` that defaults to `--squash` or `--rebase` and outright rejects `--merge`. Stops "Create a merge commit" merges from sneaking onto `main`, which would break the linear-trunk invariant the rest of the tooling depends on.

**`gh optivem workspace lint-history`** — layer 4.11
Reports, for each repo in the workspace, any merge commit on `main` over the last N commits (`git log --merges --first-parent main`). A drift detector — catches the case where the doc says "linear trunk" but the actual history disagrees. Ships with a paired GitHub Actions workflow that fails on any new merge commit on `main`, so drift is caught at PR time rather than discovered later.

**`gh optivem workspace stale-branches`** — layer 4.12
Lists branches in each workspace repo that have lived longer than ~24h, per `docs/tbd.md:62`'s "hours, not days" rule. Helps Scaled-TBD teams notice when a branch has drifted from the TBD discipline.

## Suggested sequencing

Layer 1 ships as a single PR (all four items per decision 1), then re-evaluate before opening anything else. Layers 2–4 are each their own PRs and are additive. Layer 3 is in scope (decision 5: Scaled TBD is in use).

## Decisions resolved

1. **Layer 1 scope.** All four items (1.1–1.4) as one PR.
2. **Pre-commit pull behaviour.** Explicit autoStash: tool stashes unstaged changes, rebases, then `stash pop`. Config-independent — does not assume `rebase.autoStash` is set.
3. **`doctor` reach.** Narrow: config only (`pull.rebase`, `rebase.autoStash`, `rerere.enabled`). Broader repo-health checks are out of scope.
4. **Force-push guard placement.** Both: in-tool guard (item 6) *and* pre-push hook (item 7). Independent failure modes (tool flow vs. raw `git push --force`); blast radius of force-pushing `main` justifies belt-and-suspenders.
5. **Scaled TBD in use.** Yes — Layer 3 stays in scope.
6. **`lint-history` enforcement.** Both: local command + paired CI workflow that fails on any new merge commit on `main`. Shipped together — CI is the only thing that prevents drift over time; the local form is for ad-hoc checks.
7. **Bot-race log line.** Generic only: "racing origin/main, retrying". No pattern-match on the bot's commit format — the bot is one of several possible race sources, and coupling the retry loop to its message format creates hidden coupling someone touching the workflow would have to remember.
8. **Stale-branch command name.** Lock in as `gh optivem workspace stale-branches`.

## Out of scope

- Anything inside scaffolded repos beyond installing a pre-push hook. Scaffolded repos already inherit `docs/tbd.md` from the template; behaviour changes there are a separate plan.
- Replacing `gh pr merge` / `gh pr create` outright. Layer 3 wraps `gh`, it does not reimplement it.
- Touching `.github/workflows/gh-bump-patch-version.yml`. The bot is correct; the tool needs to handle the race the bot legitimately creates.
- Merge-queue / Bors-style serialisation. `docs/tbd.md:192-206` already names this as "tooling for very large engineering orgs"; not relevant at academy scale.

## References

- `docs/tbd.md` — canonical TBD doc; sections cited inline above.
- `workspace_commands.go:107-165` — `runWorkspaceCommit`.
- `workspace_commands.go:292-325` — `runWorkspaceSync`.
- `.github/workflows/gh-bump-patch-version.yml` — the bot that makes the race not hypothetical.
