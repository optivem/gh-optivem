# Plan: decide final placement for `lint-history` and `stale-branches`

> ⚠️ **Status: deferred — decision pending operator use.**
> No work to execute yet. Revisit after a few weeks of using the new
> root-level CLI surface (post the `workspace`-noun retirement).

> **Type: decision pending.** The verbs already work — they were promoted
> to root as **hidden** during the `workspace`-noun retirement (see commit
> history; originally tracked in the now-deleted plan
> `20260515-0736-extend-test-system-with-universal-git-verbs.md`). This
> plan parks the open question of where they should ultimately live.

## Background

When the `workspace` noun was hard-removed, four cross-repo verbs were
promoted to root with environment-derived scope (`commit`, `sync`,
`actions status`, `rate-limit`). Two more workspace-iterating verbs —
`lint-history` and `stale-branches` — also needed a destination, so
they were promoted to root **as hidden commands** (`Hidden: true`) with
the same scope cascade.

Hidden = they work, but don't show in `--help`. The TODO comments in
the code mark them for follow-up placement.

## Question

Where should `lint-history` and `stale-branches` ultimately live?

## Options

| Option | Shape | Notes |
|---|---|---|
| A | Keep at root + unhide | Matches `commit` / `sync` precedent. Risk: `--help` clutter as more workspace-y verbs accumulate. |
| B | Move under a new noun (`tbd`, `audit`, `inspect`?) | Groups history-hygiene + branch-hygiene under one banner. Needs a name that doesn't bikeshed forever. |
| C | Merge into `actions` | Only if `actions`'s scope broadens beyond CI status. Today this would be a stretch. |
| D | Leave hidden indefinitely | Acceptable if no operator pain emerges; revisit only if someone asks. |

## Decision criteria (revisit triggers)

- Operator (plan author) reaches for either verb often enough to feel
  the friction of it being undocumented in `--help`.
- A third workspace-iterating "audit"-style verb appears, making a
  noun grouping pull its weight.
- `actions` grows a verb beyond `status` that suggests a broader
  inspection surface.

If none of those happen within a few weeks, **Option D (leave hidden)**
is the default — silence is consent.

## What NOT to do

- Don't bikeshed the noun name in isolation. Only pick a noun if
  Option B is actually chosen on its merits.
- Don't unhide without picking a placement — hidden-but-at-root is a
  parking spot, not a long-term home.
