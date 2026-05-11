# `--config` flag + `system_config:` / `test_config:` in `gh-optivem.yaml`

## Motivation

Today every command that reads `gh-optivem.yaml` (`compile`, the `atdd`
subtree, `config validate`) hardcodes the location to
`<cwd>/gh-optivem.yaml` via `projectconfig.Load(cwd)`
(`compile_commands.go:88-98`). Two friction points fall out of that:

1. **No way to use a non-default filename.** Users who maintain
   multiple gh-optivem configurations in the same checkout (the shop
   repo carries a monolith × multitier × three-language matrix) have
   no override flag — they have to swap files in place or `cd` into
   a subdirectory.
2. **Missing-file error is terse.** The current message reads
   `no gh-optivem.yaml in <dir>; run `gh optivem config init` first`
   (`compile_commands.go:94`). It tells the user *what* to do but
   doesn't offer to do it. On a TTY an interactive prompt would shave
   one round-trip.

The runner flags solve the analogous problem for `system.json` and
`tests.json` (`runner_commands.go:40-114`). `gh-optivem.yaml` deserves
the same treatment, and `system.json` / `tests.json` deserve a way to
opt out of being passed on every invocation when their paths are
fixed for the project.

## Design summary

Two phases, both small.

**Phase 1 — universal `--config` mechanism.** A persistent root flag
points at any `gh-optivem.yaml` file, with an env-var alias for
shell-session pinning. Missing-file error becomes an interactive
prompt on a TTY.

**Phase 2 — optional path fields in `gh-optivem.yaml`.** Users who
have stable `system.json` / `tests.json` paths declare them in their
`gh-optivem.yaml` once and drop the per-command flags. Existing
`--system-config` / `--test-config` flags stay as the explicit
override.

Explicitly **not** introducing:
- A `profiles:` block. Shop's lang × arch matrix is shop-specific and
  is satisfied by maintaining multiple `gh-optivem.*.yaml` files and
  selecting via `--config`. Don't impose that complexity on every
  scaffolded user.
- A `--suite` flag or `legacy_test_config:` field. Legacy isn't a
  first-class concept in the tool; it's just another file someone can
  point `--test-config` at. Removes vocabulary from the CLI.

## Deferred

- [ ] Step 4: Missing-file interactive prompt. — ⏳ Deferred:
  Step 4 as written requires extracting `config init`'s body into a
  callable package AND adding interactive prompting for the required
  flags (`--owner`, `--repo`, `--arch`, language, paths). The current
  error wording (`no gh-optivem.yaml at <path>; run gh optivem config
  init first`) is preserved. Reopen when the interactive flag-
  collection design is settled.

## Out of scope

- **Profiles in `gh-optivem.yaml`.** Shop's multi-combination case is
  satisfied by maintaining multiple `gh-optivem.*.yaml` files and
  selecting via `--config`. No `profiles:` block, no `--profile` /
  `-P` flag. Discussed and ruled out 2026-05-11.
- **`--suite latest|legacy` flag and `legacy_test_config:` field.**
  Legacy is not a first-class concept in the tool. Users wanting
  the legacy suite pass `--test-config ./tests-legacy.json`
  explicitly. Discussed and ruled out 2026-05-11.
- **Auto-detect / walk-up search for `gh-optivem.yaml`** (cargo /
  git-style upward traversal). Not requested; current explicit-cwd
  behaviour is fine and the new flag/env covers the same need.
- **Consolidating `system.json` / `tests.json` schemas into
  `gh-optivem.yaml`** outright. Different lifecycles, different
  audiences. Path pointers are the right level of consolidation;
  schema merge is a separate, much bigger conversation.
