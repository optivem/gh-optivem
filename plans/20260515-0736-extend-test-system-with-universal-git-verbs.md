# Promote `workspace` verbs to root, retire the `workspace` noun

> ⚠️ **Draft — needs explicit human approval before implementation.**
> The shape is settled; phase ordering and the "retire vs alias" question
> in the open questions section gate the start. Do not execute until the
> author signs off.

> 📜 **History note:** this file previously held a plan titled
> "Extend `test` and `system` with universal git verbs". That direction
> was discarded after we established that (a) git is repo-scoped and
> tiers cross-cut repos, and (b) the ATDD pipeline already enforces
> whole-repo commits via `git add -A` at
> [release.go:350-352](../internal/atdd/runtime/release/release.go#L350).
> See "Rejected alternatives" below for the full audit.

## Context

`gh optivem` follows the same hybrid pattern `gh` itself uses: **bare
verbs at root for cross-cutting operations, nouns for scoped operations**.
This is established by the existing `compile` command
([compile_commands.go:32-58](../compile_commands.go#L32)):

- `gh optivem compile` — cross-tier shortcut at root
- `gh optivem system compile`, `gh optivem test compile` — scoped forms

The `workspace` noun ([workspace_commands.go:42-56](../workspace_commands.go#L42))
parks four verbs that are also cross-cutting (cross-repo, not cross-tier),
but they live under a noun because they were ported wholesale from bash
scripts (`commit.sh`, `sync.sh`, `check-actions-all.sh`,
`gh-rate-limit.sh`). The noun was a convenient port destination, not a
deliberate design choice. This creates an inconsistency:

| Verb | Cross-cutting in nature | Lives at |
|---|---|---|
| `compile` | spans tiers | **root** ✓ |
| `commit` | spans repos | under `workspace` ✗ |
| `sync` | spans repos | under `workspace` ✗ |
| `check-actions` | spans repos | under `workspace` ✗ |
| `rate-limit` | single API call (no scope) | under `workspace` ✗ |

## Design decision

**Promote the four workspace verbs to root. The data source stays the
same (`*.code-workspace` file). The `workspace` noun retires (or becomes
a deprecation alias — see open question #1).**

End state:

```
gh optivem commit                ← was `workspace commit`
gh optivem sync                  ← was `workspace sync`
gh optivem check-actions         ← was `workspace check-actions`
gh optivem rate-limit            ← was `workspace rate-limit`
gh optivem compile               ← unchanged
gh optivem test ...              ← unchanged
gh optivem system ...            ← unchanged
gh optivem doctor / init / config / ... ← unchanged
```

Two important non-changes:

1. **`*.code-workspace` stays the source of truth** for "which repos do
   the cross-repo verbs iterate". No new `gh-optivem.yaml`-driven
   filtering. The verbs are *HOW-opinionated* (TBD discipline,
   push-retry, auto-stash, co-author trailer, etc.) but *WHICH-generic*
   (any folder in `*.code-workspace`). That mix is the right answer:
   the discipline benefits every repo, including dotfiles and the
   gh-optivem tool repo itself, so restricting WHICH would be a
   downgrade.
2. **No flag changes.** `--repo`, `--paths`, `--yes`,
   `--include-untracked`, `--workspace`, `--component`, etc. stay
   identical. This plan is a pure rename + retirement of one noun.

## Rejected alternatives

So we don't re-litigate these in three months:

| Alternative | Rejected because |
|---|---|
| Add `gh optivem test commit` / `gh optivem system commit` | Git is repo-scoped, tiers cross-cut repos. In a monolith, `test commit` and `system commit` would target the same repo, with the second no-oping. ATDD already enforces whole-repo (`release.go:350-352`). |
| Add `gh optivem workspace commit --kind test --component frontend` | Makes `workspace` read `gh-optivem.yaml`, destroying its WHICH-generic stance. |
| Add a `gh optivem repos` noun with `--kind`/`--component` | New noun whose only job is to host filters; would duplicate `workspace`-style iteration without buying functional capability (clean non-project repos no-op anyway). |
| Force every verb under a noun (strict noun-first like AWS) | Would require inventing an awkward `all` / `every` / `project` noun. `compile` precedent already says cross-cutting verbs go at root. |
| Leave the inconsistency alone | Fine if you can ignore it. We chose not to. |

## Phases

Each phase is shippable on its own. Do not pre-commit to phase 3 — its
shape depends on whether real usage of the new names is uneventful.

### Phase 1 — Register the new root-level commands (no behavior change)

Goal: `gh optivem commit / sync / check-actions / rate-limit` work at
root; `gh optivem workspace <verb>` continues to work unchanged. Two
surfaces, one implementation.

1. **Refactor** `workspace_commands.go`. Each `newWorkspace*Cmd()`
   constructor currently returns a `*cobra.Command` that knows its
   parent's persistent `--workspace` flag. Split into:
   - A package-level builder function per verb (e.g.,
     `newCommitCmd()`) that constructs the command body — flags, args,
     `Run`. No assumption about parent.
   - The existing `newWorkspaceCmd()` keeps adding the verbs as
     children, **and** registers them by alias under `workspace` (for
     back-compat).
   - `main.go` registers the same builders directly at root.

2. **Move the `--workspace` flag** ([workspace_commands.go:47](../workspace_commands.go#L47))
   from a `workspace`-noun persistent flag to a **root-level**
   persistent flag (on the root cobra command). Every cross-repo verb
   uses it identically.

3. **Tests.** Add cobra-level smoke tests asserting both call paths
   resolve to the same `Run`:
   - `gh optivem commit "msg"` → handled
   - `gh optivem workspace commit "msg"` → handled (alias)
   - Same for sync / check-actions / rate-limit.
   - All existing workspace-verb tests still pass unchanged.

4. **Help text.** The root `gh optivem --help` lists the new verbs
   under a "Cross-repo operations" group (cobra supports command
   grouping via `GroupID`). Keeps the help readable as the root surface
   grows.

**Acceptance:** every call path works; no test regressions; both `gh
optivem commit` and `gh optivem workspace commit` produce identical
behavior.

### Phase 2 — Migrate internal callers

Goal: every caller inside this repo uses the new names. The
back-compat alias keeps anything we miss working.

5. **Skills** (.claude/skills/ or wherever they live — locate first):
   - `/commit` — currently runs `gh optivem workspace commit`
   - `/sync` — currently runs `gh optivem workspace sync`
   - `/github-commit-push-all` — same
   - `/github-sync-all` — same
   - `/check-actions` (if it exists) — same
   - Update each to call the bare verb.

6. **Documentation:**
   - `README.md` if it references workspace commands
   - `CLAUDE.md` (project) — the "Always use commit/push/sync skills"
     rule mentions `gh optivem workspace commit`; update.
   - `docs/tbd.md` if it references workspace commands.
   - Other plans in `plans/` that reference the old names — light
     touch, don't churn historical plan files unless they're active.

7. **Agents** — grep `.claude/agents/` for `workspace commit` /
   `workspace sync` and update active agents only.

**Acceptance:** `grep -r "workspace commit\|workspace sync\|workspace
check-actions\|workspace rate-limit"` in this repo returns only
historical references (plans/deferred, git history) — nothing in
active code, skills, agents, or docs.

### Phase 3 — Decide on the `workspace` noun's fate

Goal: resolve open question #1 once phase 2 has soaked.

8. **If retire**: remove `newWorkspaceCmd()`, remove the alias
   wiring. Print a tombstone error for any caller still using the old
   form ("`gh optivem workspace commit` has been removed; use `gh
   optivem commit`"). Update CHANGELOG.

9. **If alias permanently**: leave the alias wiring in place. Add a
   single deprecation note in `--help` for the `workspace` noun
   ("Deprecated alias for root-level cross-repo verbs. Will be removed
   in a future major release.").

**Acceptance:** decision recorded; CHANGELOG entry; CLAUDE.md updated
if relevant.

## Affected commands

| Command | Status | Phase |
|---|---|---|
| `gh optivem commit` | NEW (root-level) | 1 |
| `gh optivem sync` | NEW (root-level) | 1 |
| `gh optivem check-actions` | NEW (root-level) | 1 |
| `gh optivem rate-limit` | NEW (root-level) | 1 |
| `gh optivem workspace commit` | alias → `commit` | 1 |
| `gh optivem workspace sync` | alias → `sync` | 1 |
| `gh optivem workspace check-actions` | alias → `check-actions` | 1 |
| `gh optivem workspace rate-limit` | alias → `rate-limit` | 1 |
| `gh optivem workspace` (noun itself) | deprecated or retired | 3 |
| All other commands | unchanged | — |

## Open questions

1. **Retire the `workspace` noun, or keep it as a permanent alias?**
   Retiring is cleaner. Keeping is friendlier to muscle memory and to
   any external script that might call the long form. **Recommended:
   keep as alias permanently** — the maintenance cost is one line of
   cobra wiring and a tiny help-text note. Cleaner CLI is not worth
   breaking anyone's existing automation.

2. **What about the existing `--workspace` flag?** Currently scoped to
   the `workspace` noun. Phase 1 promotes it to a root-level persistent
   flag (since multiple cross-repo verbs use it). Anything else that
   needs that flag (e.g. future `check-actions` extensions) gets it
   for free. **No question, just naming it.**

3. **Help-text grouping?** With four new top-level verbs, the root
   `--help` becomes denser. `cobra` supports command groups (`GroupID`).
   Worth using — e.g., group as "Project ops" (init, compile, test,
   system, doctor, config), "Cross-repo ops" (commit, sync,
   check-actions, rate-limit), "Other" (browse, etc.). **Recommended:
   yes, add groupings in phase 1.**

4. **Does `gh optivem compile` need any adjustments?** Today it's at
   root with no parent noun, exactly the pattern we're migrating
   toward. It's the *model*, not the thing being changed. **No
   question, just confirming.**

## Non-goals

- Adding new verbs or new flags.
- Changing the `*.code-workspace` data source.
- Adding `gh-optivem.yaml`-driven repo filtering.
- Touching `test`, `system`, `compile`, `init`, `doctor`, `config`, or
  any other tier-scoped or project-scoped tool.
- Touching the ATDD pipeline's `release.Commit` path.

## Decisions log

Append decisions here as they're made.

- (none yet — awaiting author input on phase 3 shape)
