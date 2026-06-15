# Move claude-settings sync into a native `gh optivem` command

**Date:** 2026-06-15 (local)
**Status:** Proposed — design plan, key decisions flagged for `/refine-plan`.

---

## Problem (why this plan exists)

Claude settings (`.claude/settings.json` permissions/hooks + the `.claude/commands/*.md`
skills) are kept in one source-of-truth repo and **distributed** to every workspace repo
and to the user's global `~/.claude/`. Today that distribution is:

- a Node script — `academy/claude/scripts/sync-claude-settings.js` (the real logic), wrapped by
  `academy/claude/scripts/sync-all-claude-settings.sh`;
- invoked by the `/sync-claude` skill, whose body runs
  `bash "$(git rev-parse --show-toplevel)/scripts/sync-all-claude-settings.sh"`.

`git rev-parse --show-toplevel` resolves to **whatever repo you happen to be in**. Run from
`gh-optivem` (or any repo that isn't `claude`) it points at a non-existent path and the skill
fails with exit 127 — exactly what happened on 2026-06-15. The path fragility is the symptom;
the root cause is that a workspace-wide operation is bootstrapped through a repo-relative shell
path.

`gh optivem` is already on `PATH` everywhere and already owns the cross-repo workspace verbs
(`commit`, `sync`, `actions`, …). Folding the settings sync into a native subcommand removes
the path bootstrap entirely.

## Decisions (locked — from the 2026-06-15 design Q&A)

1. **Home: a native `gh optivem` subcommand.** Not a skill-path patch, not a shell-out.
2. **Implementation: native Go port** of the merge+distribute logic. Drops the runtime
   dependency on `node` and on the `claude` repo's script files.
3. **Source repo: detected, never hardcoded.** The command must not bake the literal repo
   name `claude` into the (student-facing) CLI — it finds the canonical `.claude/` source by a
   marker/convention. This honours the no-scaffold-coupling rule
   (`feedback_no_scaffold_repo_coupling.md`): gh-optivem distributed code uses generic
   discovery, never `claude`/`shop`/`greeter-*` literals.

## Recommended approach

A new command reusing the existing scope machinery:

- **Reuse `internal/workspace.Resolve`.** It already finds the `*.code-workspace` file and
  returns `Scope{ Root, Folders[], SourceFile, Mode }` — the absolute repo paths the JS
  currently rediscovers by hand (`WORKSPACE_ROOT` + parsing `academy.code-workspace`). The
  command operates on `Scope.Folders` in `ModeWorkspace`; outside a workspace it no-ops with a
  clear message (a student running it sees "no workspace settings source found", nothing
  breaks).
- **Detect the source repo, don't name it.** Scan `Scope.Folders` for the one folder that
  carries a settings-source marker (exact mechanism is decision (A) below) instead of
  `folderPath === "claude"`. That folder's `.claude/settings.json` + `.claude/commands/` are
  the SSoT.
- **Port the merge/distribute logic verbatim in behaviour** (this is a materialisation, not a
  redesign — `feedback_materialize_dont_expand.md`; do not "improve" the merge semantics):
  1. Merge `permissions.allow` as a **name-keyed union** (strip the `(...)` arg suffix when
     deduping, keep the fuller entry) of global + source settings.
  2. Write the merged result back to **global** (`~/.claude/settings.json`) and to the
     **source** repo, only-if-changed.
  3. Source repo is authoritative for `permissions.defaultMode` and for `hooks` — push those
     into global.
  4. Build the `repoSettings` payload (`permissions.allow`, `defaultMode`, optional `hooks`,
     optional `skipDangerousModePermissionPrompt` carried from global) and write it to every
     **other** workspace folder's `.claude/settings.json`, only-if-changed.
  5. Copy every `.claude/commands/*.md` from the source repo to `~/.claude/commands/`,
     only-if-changed.
  6. Print the same per-file `Updated:` lines + the `(N repo(s), M skill(s))` /
     `already in sync` summary so the UX is unchanged.
- **Wire it into the cross-repo group** in `main.go` next to `newSyncCmd()` /
  `newCommitCmd()` (group `groupCrossRepoOps`).
- **Repoint the `/sync-claude` skill** at the new binary (`gh optivem claude sync`). This
  supersedes the earlier quick path-patch — that patch becomes moot once the binary owns the
  operation.

### Why not the alternatives (record, don't reopen)

- *Just fix the skill's `git rev-parse` path.* Smallest change, but leaves the operation
  bootstrapped through a shell script in one specific repo; still breaks if that repo is moved
  or absent. The user chose the binary.
- *Shell out from Go to the existing `.js`.* Keeps two SSoTs and a `node` + `claude`-repo
  runtime dependency. Rejected in favour of the native port.
- *Hardcode the `claude` repo as source.* Simplest, but couples the distributed student CLI to
  this workspace's layout. Rejected per decision (3).

---

## Open decisions to resolve in `/refine-plan`

A. **Source-repo detection mechanism.** Pick one and pin it:
   - a sentinel file in the source repo (e.g. `.claude/.settings-source`), scanned for across
     `Scope.Folders`; **or**
   - a key in the workspace/`gh-optivem` config naming the source folder; **or**
   - "the folder whose `.claude/` has a non-empty `commands/` dir" (weakest — distribution
     targets may also grow a `commands/`).
   Sentinel file is the leading candidate (explicit, zero new config schema, no hardcoded name).
B. **Command name / shape.** `gh optivem claude sync` (a `claude` parent group with a `sync`
   verb) vs a flat `gh optivem sync-claude`. Note the existing flat `sync` already means git
   pull/push, so the name must not collide. Decide whether an `install` alias is wanted.
C. **Source repo in `ModeSingleRepo` / no workspace.** Confirm the no-op message + exit code
   when run outside a resolvable workspace (don't error hard; a clean "nothing to do").
D. **Global `~/.claude` writes under `--workspace` scoping.** The global settings + global
   commands writes are machine-level, not repo-scoped. Confirm they still happen regardless of
   `Scope.Mode` (as the JS does today), and that this is desirable.

## Items (agent work)

- [ ] **1. New `internal/workspace`-backed source-repo detector.** Add the detection function
  (per decision A) that, given a resolved `Scope`, returns the source folder + its `.claude/`
  paths, or a "not found" sentinel. Unit-tested with table cases (found / not-found /
  multiple-markers-error).
- [ ] **2. New command file `claude_commands.go`** implementing `newClaudeSyncCmd()` (name per
  decision B): resolve scope, detect source, run the merge/distribute steps 1–6 above, print
  the existing UX. Register it in `main.go` under `groupCrossRepoOps`.
- [ ] **3. Port the merge helpers to Go** — name-keyed `union` of `permissions.allow`,
  only-if-changed JSON writer (2-space indent + trailing newline, matching the JS byte output
  so the first run after migration is a no-op, not a churn), settings payload builder. Keep
  behaviour identical to `sync-claude-settings.js`.
- [ ] **4. `--help` text** for the new command (Use/Short/Long/Example), matching the
  house style of the other cross-repo verbs.
- [ ] **5. Repoint the `/sync-claude` skill** (`academy/claude/.claude/commands/sync-claude.md`)
  to run `gh optivem claude sync` instead of the bash/`git rev-parse` line. (This file is itself
  distributed by the sync, so the new body propagates on the next run.)
- [ ] **6. Unit tests** (`claude_commands_test.go`): union/dedup semantics; only-if-changed
  writer is a true no-op on identical input; source detection; distribution writes the
  `repoSettings` payload to non-source folders and skips the source; commands copied to a temp
  global dir. Use temp dirs / fakes — **no writes to the real `~/.claude`** in tests.

## Verification (user-driven — not agent Items)

- [ ] `go build ./...` and `scripts/test.sh` (or scoped `go test -p 2 ./...` for the touched
  packages — **never** unbounded `go test ./...` on Windows, `feedback_go_test_windows.md`).
- [ ] Run `gh optivem claude sync` from **gh-optivem** (the repo that broke the old skill) and
  confirm it resolves the workspace, distributes settings, and prints the same summary the JS
  produced.
- [ ] Run it a second time → `already in sync` (no churn), confirming byte-identical output to
  the JS migration baseline.
- [ ] Run `/sync-claude` and confirm the skill now drives the binary.

## Risks / notes

- **Byte-for-byte JSON parity matters.** If the Go writer's formatting differs from the JS
  (indent, key order, trailing newline), the first run rewrites every `settings.json` and looks
  like a mass change. Pin the writer against the JS output in a test before rollout.
- **Decommissioning the JS is out of scope here.** Leave
  `sync-claude-settings.js` / `sync-all-claude-settings.sh` in place until the binary is
  verified across the workspace; removing them is a separate follow-up.
- **Scope discipline.** Port behaviour only. Do not reshape the merge semantics, add new synced
  artefacts, or "fix" unrelated settings drift in the same pass.
- **No diagram regeneration step** — not applicable here, but noted for the
  house plan-format (`feedback_plans_no_diagram_regen.md`).
