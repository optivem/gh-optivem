# Promote `workspace` verbs to root, retire the `workspace` noun

> 🤖 **Picked up by agent** — `ValentinaLaptop` at `2026-05-15T07:01:16Z`

> 📜 **History note:** this file went through several discarded designs
> before landing here. See "Rejected alternatives" below — that section
> exists so we don't re-litigate. The current shape: promote the
> cross-repo verbs to root, make scope environment-derived, drop the
> idea of a workspace-level `gh-optivem.yaml`.

## Final state (what the plan produces)

After all phases land, the CLI surface looks like this:

```
gh optivem commit                ← NEW (root). scope: workspace if *.code-workspace found, else cwd repo.
gh optivem sync                  ← NEW (root). same scope rules.
gh optivem actions status        ← NEW (noun-verb). renames `check-actions`. same scope rules.
gh optivem rate-limit            ← NEW (root). no scope (single API call).

gh optivem workspace …           ← REMOVED. hard rename, no alias.

gh optivem compile               ← unchanged
gh optivem test ...              ← unchanged
gh optivem system ...            ← unchanged
gh optivem init / doctor / config / ... ← unchanged
```

**Naming rationale:**
- `commit`, `sync`, `rate-limit` — short, modifying or single-call ops → bare verbs at root.
- `actions status` — query op (reads state, presents summary) → noun-verb pattern, matches `gh run list` / `gh workflow list` style. The `actions` noun can grow naturally (`actions list`, `actions rerun`) if useful later.
- `workspace` noun — hard removed; you control all callers, no need to keep aliases.

**Scope cascade** for `commit` / `sync` / `actions status`:

| cwd is inside… | gh-optivem operates on |
|---|---|
| a workspace (`*.code-workspace` found via flag / env / walk-up) | all workspace folders |
| a project with `repos:` in `gh-optivem.yaml` | the listed repos (project iteration) |
| a project without `repos:` (or empty) | the cwd repo only |
| any git repo (no project either) | the cwd repo only |
| no git repo | error |

The project-iteration row is added by **phase 3** (new `repos:` field
in `gh-optivem.yaml` + `gh optivem config migrate` back-fill). Phases
1–2 ship the rename with the two-row cascade; phase 3 inserts the
project-iteration row.

**Banner** prints the resolved scope on every run:
- `Mode: workspace (5 repos from page-turner.code-workspace)`
- `Mode: single repo (shop, no workspace file found)`

**What does NOT change:**
- Per-project `gh-optivem.yaml` semantics — still the one and only config for a scaffolded project.
- No new config file, no new flag, no new verb.
- TBD discipline (pull --rebase, push-retry, auto-stash, co-author trailer, hasUpstream skip) — applied identically in workspace and single-repo modes.
- `test`, `system`, `compile`, `init`, `doctor`, `config` — untouched.
- ATDD pipeline's `release.Commit` — untouched.

## Context

`gh optivem` follows the same hybrid pattern `gh` itself uses: **bare
verbs at root for cross-cutting operations, nouns for scoped
operations**. This is established by the existing `compile` command
([compile_commands.go:32-58](../compile_commands.go#L32)):

- `gh optivem compile` — cross-tier shortcut at root
- `gh optivem system compile`, `gh optivem test compile` — scoped forms

The `workspace` noun ([workspace_commands.go:42-56](../workspace_commands.go#L42))
parks four cross-cutting verbs (commit / sync / check-actions /
rate-limit), but only because they were ported wholesale from bash
scripts. The noun was a convenient port destination, not a deliberate
design choice. This creates an inconsistency:

| Verb | Cross-cutting in nature | Lives at |
|---|---|---|
| `compile` | spans tiers | **root** ✓ |
| `commit` | spans repos | under `workspace` ✗ |
| `sync` | spans repos | under `workspace` ✗ |
| `check-actions` | spans repos | under `workspace` ✗ |
| `rate-limit` | single API call | under `workspace` ✗ |

## Core model

**A scaffolded project has exactly one config: its own
`gh-optivem.yaml`. The workspace is optional infrastructure that only
exists when there are multiple repos to coordinate ACROSS projects.**

After phase 3, the project's `gh-optivem.yaml` can also declare its
constituent local repos (for multitier projects with separate
frontend/backend/test repos). Those repos are *project-internal* —
not a workspace concept.

Four apparent scopes, one tool:

| cwd is inside… | gh-optivem operates on |
|---|---|
| a workspace (`*.code-workspace` found) | all workspace folders (broadest) |
| a project with `repos:` listed in `gh-optivem.yaml` | the listed repos (project tiers) |
| a project without `repos:` (or empty) | the cwd repo only |
| any git repo (no project either) | the cwd repo only |
| no git repo | error |

This means `gh optivem commit` Just Does The Right Thing at each
scope. No flag needed. No two-config-files. No workspace-level
`gh-optivem.yaml`. The same TBD discipline (pull --rebase, push-retry,
auto-stash, co-author trailer) applies in every case — only the *set*
of repos iterated differs.

## Design decision

**Promote the four cross-repo verbs to root, hard-remove the
`workspace` noun (no aliases), rename `check-actions` to the
noun-verb form `actions status`. Scope is environment-derived
(cascade in the Core model section).**

End state:

```
gh optivem commit                ← was `workspace commit`; scope env-derived
gh optivem sync                  ← was `workspace sync`
gh optivem actions status        ← was `workspace check-actions`; noun-verb form
gh optivem rate-limit            ← was `workspace rate-limit`
gh optivem workspace …           ← REMOVED entirely (no alias)
gh optivem compile               ← unchanged
gh optivem test ...              ← unchanged
gh optivem system ...            ← unchanged
gh optivem doctor / init / ...   ← unchanged
```

**Naming pattern rule:** modifying / action verbs go bare at root
(`commit`, `sync`, `compile`). Query verbs that read state and report
use the noun-verb form (`actions status`). `rate-limit` is a single
API call (neither iteration nor a query about your repos) — kept bare
for brevity. This split matches `gh`'s own conventions (`gh pr create`
modifying vs `gh run list` query).

Two important non-changes:

1. **No new config file.** No workspace-level `gh-optivem.yaml`. The
   existing per-project `gh-optivem.yaml` and the optional
   `*.code-workspace` are sufficient; scope is determined by which one
   the cascade finds.
2. **No flag additions.** `--repo`, `--paths`, `--yes`,
   `--include-untracked`, `--workspace` stay identical. The behavior
   change is purely "scope is now inferred when no workspace is
   present".

## Rejected alternatives

So we don't re-litigate:

| Alternative | Rejected because |
|---|---|
| Add `gh optivem test commit` / `gh optivem system commit` | Git is repo-scoped, tiers cross-cut repos. In a monolith, both would target the same repo with the second no-oping. ATDD already enforces whole-repo (`release.go:350-352`). |
| Add `--kind` / `--component` filters to `workspace commit` | Makes `workspace` read `gh-optivem.yaml`, destroying its WHICH-generic stance. |
| Introduce a `gh optivem repos` noun with filter flags | Adds a noun whose only job is to host filters; functional output unchanged because clean non-project repos no-op anyway. |
| Workspace-level `gh-optivem.yaml` listing all repos | A scaffolded project already has its own `gh-optivem.yaml`; a second one at workspace level is redundant for the single-project case. Per-project standalone-ness is a property worth preserving. |
| Sibling-folder scan when no workspace file | Filesystem layout becomes the source of truth; non-deterministic, includes random repos. |
| Hard-error when no workspace file | Today's behavior; too rigid once the verb lives at root. Inferring scope from environment is friendlier without being magical. |
| Force every verb under a noun (strict noun-first) | Would require inventing an awkward `all` / `every` noun. `compile` precedent already says cross-cutting goes at root. |

## Phases

**Delivery: one PR landing all phases together.** Phases are labelled
for organisational clarity (what work belongs to what concern), not as
separate merge boundaries. Phase 1+2 are tightly coupled (no aliases
means callers must update in lockstep); phase 3 is logically separable
but bundled with the rest by author preference for a single migration
moment over two staged ones.

### Phase 1 — Register the new root-level commands + scope cascade + remove `workspace` noun

*(Completed in code — items deleted. The CLI surface is live: `gh
optivem commit`, `gh optivem sync`, `gh optivem actions status`, `gh
optivem rate-limit` ship at root with environment-derived scope. `gh
optivem workspace …` is removed; cobra emits its standard unknown-
command error. lint-history + stale-branches were promoted to root as
hidden verbs — see "Deferred follow-ups" below.)*

### Phase 2 — Migrate internal callers (must land with phase 1)

Goal: every caller inside this repo uses the new names. **This phase
must ship in the same PR as phase 1** because there's no alias to
catch missed callers — the old commands hard-error.

7. **Skills** (locate first — likely under `.claude/`):
   - `/commit` → `gh optivem commit`
   - `/sync` → `gh optivem sync`
   - `/github-commit-push-all` → `gh optivem commit` (still iterates
     workspace because that's the calling context)
   - `/github-sync-all` → `gh optivem sync`
   - any skill calling `workspace check-actions` → `gh optivem actions status`
   - any skill calling `workspace rate-limit` → `gh optivem rate-limit`

8. **Documentation:**
   - `README.md` references (including the `workspace rate-limit` line
     at README.md:256)
   - `CLAUDE.md` "Always use commit/push/sync skills" rule
   - `docs/tbd.md` references
   - Active plans in `plans/` (light touch — don't churn history)

9. **Agents** — grep `.claude/agents/` for old names; update active
   agents only.

10. **CI workflow `.github/workflows/`** — only if any workflow shells
    out to `gh optivem workspace …` (the `gh-rate-limit.sh` script
    does *not* — it's self-contained bash, not a wrapper).

**Acceptance:** `grep -rn "gh optivem workspace"` returns only
historical references (deferred plans, git history, archived docs) —
nothing in active code, skills, agents, docs, or workflows.

### Phase 2.5 — Confirm ATDD pipeline is already aligned (no work)

The state machine has a single `COMMIT` activity
([process-flow.yaml:1014-1037](../internal/atdd/runtime/statemachine/process-flow.yaml#L1014))
shared by every cycle. The `commit_phase` action runs `git add -A`
then commits (whole-repo, no path slicing) — confirmed at
[release.go:350-352](../internal/atdd/runtime/release/release.go#L350).
There is **no** `COMMIT_TEST` or `COMMIT_SYSTEM` state. The phase
suffix that appears in commit messages (`AT - GREEN - SYSTEM`,
`AT - RED - TEST`) is a *message convention*, not a state name, and
stays as-is.

**Acceptance:** explicit "no change needed" confirmed; this phase
exists only so future readers don't ask the same question and end up
re-investigating.

### Phase 3 — Local repo paths in `gh-optivem.yaml` + migration

Goal: a multitier project declares its constituent local repos in its
own `gh-optivem.yaml`. The scope cascade gains a project-aware layer
between "workspace file" and "cwd repo only". `gh optivem config
migrate` back-fills the field for existing projects.

**Why now:** without this, a multitier project (frontend + backend +
test repos) requires a `*.code-workspace` file to commit across its
own tiers via one command. That's redundant — the project's own
config should know about its own repos. With this, `gh optivem
commit` inside a multitier project commits the project's repos
without needing a separate workspace file.

11. **Schema** — add `repos:` to `gh-optivem.yaml`:

    ```yaml
    project:
      name: page-turner
      arch: multitier

    # NEW: local repo paths for THIS project's tiers
    repos:
      - path: ./system-frontend
      - path: ./system-backend
      - path: ./system-tests
    ```

    Empty / missing `repos:` = single-repo project (cwd repo). No new
    fields are mandatory; existing configs keep working.

12. **Extend the scope cascade** (modifies phase 1 step 3):

    | cwd context | scope |
    |---|---|
    | `*.code-workspace` found via flag/env/walk-up | workspace iteration (broadest) |
    | `gh-optivem.yaml` with non-empty `repos:` found | project iteration (the listed repos) |
    | `gh-optivem.yaml` with no/empty `repos:` found | cwd repo only |
    | any git repo (no project) | cwd repo only |
    | nothing | error |

    Banner gains a third mode: `Mode: project (3 repos from gh-optivem.yaml)`.

13. **`gh optivem config migrate`** — idempotent back-fill of `repos:`:
    - Monolith projects: writes `repos: []` (or omits — they don't need it; cwd repo behavior already works).
    - Multitier projects: infers from `system.config` and `system_test.config` — reads each, extracts repo paths, writes them to `repos:`. If inference fails (config malformed, paths ambiguous), errors with a clear pointer to "edit `repos:` by hand".
    - Preserves comments and key ordering (existing config_migrate already does this via yaml.v3 node-level edits).

14. **Tests:**
    - Cascade picks the right scope for each config combination.
    - Migrate is idempotent (running twice = no-op).
    - Migrate correctly infers from real multitier sample configs.
    - Backwards-compat: existing configs without `repos:` still load
      and produce single-repo or workspace-mode behavior.

**Acceptance:** new schema field accepted; cascade behaves per table;
`gh optivem config migrate` adds the field idempotently for older
configs; existing tests still pass.

## Affected commands

| Command | Status | Phase |
|---|---|---|
| `gh optivem commit` | NEW (root, env-derived scope) | 1 |
| `gh optivem sync` | NEW (root, env-derived scope) | 1 |
| `gh optivem actions status` | NEW (noun-verb, env-derived scope) | 1 |
| `gh optivem rate-limit` | NEW (root, no scope) | 1 |
| `gh optivem workspace commit` | REMOVED | 1 |
| `gh optivem workspace sync` | REMOVED | 1 |
| `gh optivem workspace check-actions` | REMOVED (also renamed verb) | 1 |
| `gh optivem workspace rate-limit` | REMOVED | 1 |
| `gh optivem workspace` (noun itself) | REMOVED | 1 |
| `gh optivem config migrate` | extended (back-fills `repos:`) | 3 |
| `gh-optivem.yaml` schema | new optional `repos:` field | 3 |
| All other commands | unchanged | — |

## Non-goals

- Adding new verbs or new flags.
- Introducing a workspace-level `gh-optivem.yaml`.
- Adding `gh-optivem.yaml`-driven repo filtering (`--kind` / `--component`).
- Touching `test`, `system`, `compile`, `init`, `doctor`, `config`.
- Touching the ATDD pipeline's `release.Commit` path.
- Sibling-folder repo discovery.

## Decisions log

Append decisions here as they're made.

- 2026-05-15: rename direction confirmed (`workspace` verbs → root).
- 2026-05-15: scope cascade confirmed (workspace file → cwd repo → error).
- 2026-05-15: no workspace-level `gh-optivem.yaml` — project's own config is sufficient; workspace file (when present) handles multi-repo enumeration.
- 2026-05-15: **hard remove `workspace` noun** — no alias. Author controls all callers; aliases would just be two-ways-to-say-the-same-thing in `--help`. Phases 1 and 2 must ship together.
- 2026-05-15: **`check-actions` → `actions status`** — noun-verb pattern for query ops; matches `gh run list` / `gh workflow list` convention; `actions` noun is extensible (`actions list`, `actions rerun` if useful later).
- 2026-05-15: **`rate-limit` stays bare at root** — short, self-evident, no scope to cascade. Not placed under `api` noun because there's no other `api` verb planned right now.
- 2026-05-15: script-integration enhancements (`--json`, `--wait` for `rate-limit`) deferred — additive, not blocking the rename. Track separately if pursued.
- 2026-05-15: ATDD process flow is **already aligned** — no `COMMIT_TEST` / `COMMIT_SYSTEM` states exist; the single shared `COMMIT` activity + `commit_phase` action already does whole-repo (`git add -A`) commits. Commit-message phase suffixes (`AT - GREEN - SYSTEM`, etc.) stay; they're a message convention, not a state name.
- 2026-05-15: **add `repos:` field to `gh-optivem.yaml`** (phase 3) — declares a project's own constituent local repos for multitier; back-filled by `gh optivem config migrate`. Inserts a "project iteration" row in the scope cascade between workspace and cwd-repo modes.
- 2026-05-15: **one big PR for all phases** — phases 1, 2, 2.5, and 3 ship together. Single migration moment for the operator instead of two staged ones.
- 2026-05-15: **single-repo banner wording** — `Mode: single repo (<basename>)`. Matches the workspace banner pattern (`Mode: <scope-name> (<details>)`); drops the "no workspace file found" trailer to avoid noise once operators internalize the cascade.
- 2026-05-15: **`--yes` still required in single-repo mode** — same opt-in-to-scripted-commits semantics as workspace mode. Existing skill callers (`/commit`, `/github-commit-push-all`) already pass `--yes`, so no caller churn. Keeps one flag with one meaning across scopes.
- 2026-05-15: **`rate-limit` has no scope cascade** — single API call to GitHub's rate-limit endpoint; nothing to iterate. Promotion to root is purely a rename.
- 2026-05-15: **cobra `GroupID` grouping added in phase 1** — groups: "Project ops" (compile, test, system, doctor, config, init), "Cross-repo ops" (commit, sync, actions, rate-limit), "Other". Without grouping the new root verbs would interleave alphabetically with project verbs in `--help`, defeating the visual distinction the rename creates.
- 2026-05-15: **`workspace lint-history` and `workspace stale-branches` are also moved out of the `workspace` noun** — they iterate workspace repos too, so they need a destination when the noun is removed. Promoted to root but registered with `Hidden: true` so they don't show in `--help` while the operator (the plan author) decides their final placement. TODO comments mark them. Same scope cascade as the other cross-repo verbs (single-repo mode is meaningful for both — lint-history can scan one repo's main; stale-branches can list one repo's branches). The Affected commands table is implicitly extended to include `gh optivem lint-history` and `gh optivem stale-branches` as NEW (hidden, root).

## Deferred follow-ups

- [ ] Decide final placement for `lint-history` and `stale-branches` (currently hidden at root). Options: keep at root + unhide, move under a new noun (`tbd`, `audit`, `inspect`?), or merge into `actions` if its scope broadens. Revisit after a few weeks of operator use.
