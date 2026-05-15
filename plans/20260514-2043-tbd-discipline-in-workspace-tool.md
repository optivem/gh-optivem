# Build TBD discipline into `gh optivem workspace`

> 🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-05-15T06:23:54Z`

> ✅ **STATUS: LAYERS 1 + 2 + 3 SHIPPED.** Layer 1 (config-independent `--rebase`, push-retry loop, pre-commit pull with auto-stash, `gh optivem doctor`), Layer 2 (mode banner, in-tool main force-push guard, `gh optivem hooks install`), and Layer 3 (`gh optivem branch start`, `gh optivem branch refresh`, `gh optivem pr merge`) have shipped. Layer 4 remains. All open decisions have been resolved — see "Decisions resolved" at the bottom.

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

Layered so each layer is shippable on its own. Layers 1, 2, and 3 have shipped. Layer 4 is its own PR and is additive.

### Layer 4 — Drift detection

Catch the case where the doc and the repo have drifted apart.

11. **`gh optivem workspace lint-history`** — for each repo, `git log --merges --first-parent main` over the last N commits; flag any hits. Ships with a paired GitHub Actions workflow that fails on any new merge commit on `main`. Both shipped together — local for ad-hoc, CI for guarantee.
12. **`gh optivem workspace stale-branches`** — list branches in each workspace repo older than ~24h (per `tbd.md:62`); helps Scaled-TBD teams notice when "hours, not days" has slipped.

## Affected commands

Single reference for every new command this plan still proposes. Layers 1, 2, and 3 surfaces (workspace commit, workspace sync, doctor, hooks install, branch start, branch refresh, pr merge) have shipped and are documented in the code itself.

### New

**`gh optivem workspace lint-history`** — layer 4.11
Reports, for each repo in the workspace, any merge commit on `main` over the last N commits (`git log --merges --first-parent main`). A drift detector — catches the case where the doc says "linear trunk" but the actual history disagrees. Ships with a paired GitHub Actions workflow that fails on any new merge commit on `main`, so drift is caught at PR time rather than discovered later.

**`gh optivem workspace stale-branches`** — layer 4.12
Lists branches in each workspace repo that have lived longer than ~24h, per `docs/tbd.md:62`'s "hours, not days" rule. Helps Scaled-TBD teams notice when a branch has drifted from the TBD discipline.

## Suggested sequencing

Layers 1, 2, and 3 have shipped. Layer 4 is its own PR and is additive.

## Decisions resolved

(Decisions originally numbered 1, 2, 3, 7 were applied to Layer 1 and have shipped; the Layer 2 force-push placement decision has also shipped. They remain as the committed shape if anything in that surface is later revisited.)

1. **Scaled TBD in use.** Yes — Layer 3 stays in scope.
2. **`lint-history` enforcement.** Both: local command + paired CI workflow that fails on any new merge commit on `main`. Ship together — CI is the only thing that prevents drift over time; the local form is for ad-hoc checks.
3. **Stale-branch command name.** Lock in as `gh optivem workspace stale-branches`.

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
