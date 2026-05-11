# Drop verb-first deprecation aliases — v1.6

> ⚠️ **Needs explicit human approval before implementation. Discuss first.**
> This plan is a draft. Do not execute any step until the author signs off.

> **Paired plan.** This plan deletes the hidden aliases registered by
> [`20260511-2000-noun-first-cli-rename.md`](20260511-2000-noun-first-cli-rename.md).
> Do not execute this plan until the rename plan has shipped in a tagged
> gh-optivem release **and** the shop template + scaffolded student repos
> have been migrated to the new forms.

## Context

The noun-first rename ships in v1.5 (gh-optivem) with hidden verb-first
aliases for backwards compatibility:

```
gh optivem build system     → hidden alias of `gh optivem system build`
gh optivem run system       → hidden alias of `gh optivem system start`
gh optivem stop system      → hidden alias of `gh optivem system stop`
gh optivem clean system     → hidden alias of `gh optivem system clean`
gh optivem compile system   → hidden alias of `gh optivem system compile`
gh optivem compile system-tests → hidden alias of `gh optivem test compile`
gh optivem test system      → hidden alias of `gh optivem test run`
gh optivem verify tokens    → hidden alias of `gh optivem token verify`
```

Each alias prints a stderr deprecation warning before delegating to the new
handler. This keeps the old forms working in:

- Scaffolded student repos in the wild (their workflows reference the old forms).
- The shop template, until a coordinated PR updates it.
- Documentation, blog posts, and any external references.

In v1.6 we drop the aliases entirely. After that, the old forms return
Cobra's standard "unknown subcommand" error.

## Preconditions

Before executing this plan:

1. **gh-optivem v1.5 shipped** with the rename and the hidden aliases.
2. **Shop template updated** to the new forms (separate shop PR per
   rename plan Step 10).
3. **Soak window elapsed.** At least one minor-release cycle (~weeks)
   has passed and the deprecation warning has been visible to anyone
   still on the old forms.
4. **No first-party caller still on the old forms.** Confirm with:
   ```bash
   grep -rEn 'gh optivem (build|run|stop|clean) system\b|gh optivem (test|compile) (system|system-tests)\b|gh optivem verify tokens\b' \
       /c/GitHub/optivem/academy/gh-optivem
   ```
   Expected: only matches inside historical plan files in `plans/`
   (annotated per rename plan Step 9), and inside this very plan.
5. **No scaffolded-repo references in the latest shop release.** Verify
   in the most recent `meta-v*` shop tag that no `gh optivem (build|run|stop|clean) system` /
   `gh optivem (test|compile) (system|system-tests)` / `gh optivem verify tokens`
   invocation survives in any workflow, README, or helper script.

If any precondition fails, do not proceed — fix that first and re-check.

## Steps

### Step 1 — Delete the alias wiring

In `main.go`, remove the four `newDeprecated*Cmd()` entries from the root
`AddCommand` block:

```diff
 cmd.AddCommand(
     newInitCmd(),
     newConfigCmd(),
     newSystemCmd(),
     newTestCmd(),
     newCompileCmd(),
     newAtddCmd(),
     newTokenCmd(),
     newUpgradeCmd(),
-
-    // Hidden verb-first aliases for one release. Drop in v1.6 per
-    // plans/20260511-2010-drop-verb-first-aliases.md.
-    newDeprecatedBuildCmd(),
-    newDeprecatedRunCmd(),
-    newDeprecatedStopCmd(),
-    newDeprecatedCleanCmd(),
-    newDeprecatedVerifyCmd(),
 )
```

Delete `deprecated_commands.go` in its entirety (the file added by Step 5 of
the rename plan).

In `compile_commands.go`, remove the hidden `system` / `system-tests`
children registered on the bare `compile` parent.

In the `test` subcommand wiring, remove `newDeprecatedTestSystemCmd()` from
the `test` parent's `AddCommand` block.

### Step 2 — Remove the `child.PreRun` deprecation hook

The `newDeprecatedAlias` helper attached a `PreRun` to each new handler that
prints the warning. With Step 1 deleting the file, the `PreRun` assignments
disappear too. Verify by grepping for `DEPRECATED:` in the codebase — the
only remaining matches should be inside historical plan files.

### Step 3 — Update the version bump

`VERSION` file: bump to `1.6.0`. Confirm `internal/version` picks it up via
the standard goreleaser flow.

### Step 4 — Update CHANGELOG / release notes

In the v1.6 release notes:

> **Breaking:** removed deprecated verb-first command aliases. If you are
> still on `gh optivem build system` etc., upgrade to the noun-first form
> (`gh optivem system build`, …). See the v1.5 release notes for the full
> rename matrix.

### Step 5 — Verify

- `go build ./...` clean.
- `scripts/test.sh` clean.
- `gh optivem build system` now returns Cobra's standard
  `Error: unknown command "system" for "gh-optivem build"`, exit code != 0.
- `gh optivem verify tokens` now returns Cobra's standard unknown-subcommand
  error, exit code != 0.
- `gh optivem system build` and `gh optivem token verify` continue to work.
- `gh-acceptance-stage` matrix green.

## Out of scope

- **The renames themselves.** Already shipped in v1.5; this plan only
  removes the bridge.
- **Renaming or removing the `system` / `test` / `compile` commands.**
  Stable surface; not touched.
- **Updating CLAUDE.md or other long-form docs.** They should already be
  on the new forms after v1.5 ships.

## Open questions

1. **Hard break vs. friendlier error.** Cobra's default "unknown
   subcommand" error is terse. Consider a custom error message on the
   `system` / `test` parents that detects the specific deprecated patterns
   (`build system`, `run system`, etc.) and prints a one-liner pointing at
   the new form. Recommendation: don't bother — the v1.5 deprecation
   warning has been visible for a full release; the v1.6 hard break can be
   Cobra-default-terse.
2. **Drop timing.** v1.6 (one minor after rename) is the default sequence
   per the rename plan. Slip to a later release if shop or scaffolded repos
   aren't fully migrated when v1.6 is cut.
