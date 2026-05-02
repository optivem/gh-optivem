# Embed orchestration YAML and agent prompts in gh-optivem; drop consumer-side copies and Claude Code subagent dependency

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-05-02T08:17:59Z`

## Motivation

Today the orchestration YAML and the per-agent prompts live in each consumer repo, not in `gh-optivem`:

- `docs/atdd/process/process-flow.yaml` is read from the consumer's CWD by the runtime (`driver.DefaultYAMLPath`).
- `.claude/agents/atdd/<name>.md` is resolved by Claude Code's subagent file convention when `clauderun.Dispatch` instructs the parent `claude` session to "Launch the `<name>` subagent".

`gh-optivem` itself carries only a `testdata/` snapshot of the YAML and zero agent prompts. The consumer-owned-content design was introduced as "repo-agnostic by design" but in practice every consumer carries byte-identical copies (`shop`, `rehearsal-atdd-cli`); the customization extension point is unused.

The drift surface this creates:

- **Schema changes are N-repo migrations.** Renaming `flows:` → `processes:` (the open BPMN-consolidation plan) requires identical edits to the loader, the testdata snapshot, *and* every consumer repo's YAML. The CLI breaks for any consumer not migrated in lock-step.
- **New consumer repos require copying ~12 files of orchestration scaffolding** (the YAML, eleven agent prompts, two skill shells) before the first ticket can ship.
- **Two-tier dispatch.** Every `claude` invocation pays for both a parent Claude Code harness AND a Task-spawned subagent that reads the prompt file from disk. The parent exists only to spawn the subagent.

v2 already moved orchestration from "Claude Code subagent decides what to do next" to "Go runtime orchestrates; subagent does narrow creative work". Inlining the prompts completes that arc — Claude becomes a stateless creative-work executor, gh-optivem owns the orchestration content end-to-end, and the consumer repo carries nothing related to ATDD orchestration by default.

The architectural shift, in one line: **gh-optivem stops treating Claude Code as an OS that resolves program names from the filesystem, and starts treating Claude as a function that takes a literal prompt string.** Coupling drops from "Task tool + subagent file convention + filesystem layout" to "just `claude -p`".

## Items

Sequence: YAML embed first (small, mechanical, unblocks the BPMN plan); then agent-prompt embed (the substantial change); then override flags, the diagram CLI replacement, and consumer cleanup. Each item is one PR.

### 7. Migrate slash-command logic into native gh-optivem CLI + config; delete the slash commands

**Motivation.** The slash commands (`shop/.claude/commands/atdd/atdd-implement-ticket.md` — 182 lines, `atdd-manage-project.md` — 31 lines) aren't thin wrappers; they encode features that today only work via the slash command (rehearsal worktrees, scope-axis flags, run-mode confirmation). After this plan they have nowhere to live unless we either reintroduce them as per-repo files or migrate the features into gh-optivem proper. We do the latter, then delete the slash commands.

**Files (gh-optivem):**
- `internal/atdd/runtime/config/` → move to `internal/config/` (the file holds project-level facts useful beyond ATDD; the package shouldn't sit under the ATDD subtree).
- `internal/config/config.go` — extend the `Config` schema and rename `Path`:
  - `Path` const: `docs/atdd/config.yaml` → `optivem.yaml` (project root).
  - Add `Project.Layout` (`single-repo` | `multi-repo`).
  - Add `Project.Repos []string` (required when `Layout: multi-repo`; optional / implicit-self when `single-repo`).
  - Add a top-level `Scope` group: `Architecture` (`monolith` | `multitier` | `both`), `SystemLang` (`java` | `dotnet` | `typescript` | `all`), `TestLang` (same enum).
  - Update package comment — drop "ATDD configuration"; describe as "project configuration".
- `internal/config/config_test.go` — coverage for the new fields, including absence-is-OK and validation on the multi-repo branch (empty `Repos` with `Layout: multi-repo` is an error).
- `internal/atdd/runtime/driver/driver.go` — update import path (`internal/atdd/runtime/config` → `internal/config`); read scope + layout, surface them on `Options` (or directly into `statemachine.Context.Params`), thread into agent prompt context as `${architecture}`, `${system_lang}`, `${test_lang}`, `${layout}`, `${repos}`.
- `internal/atdd/runtime/board/` — update import path.
- `atdd_commands.go` — no per-axis CLI override flags; deviation goes through `--config <path>` (defined in Item 3).
- Run-mode detection: `--issue` present → specific-issue mode; absent → board mode. Drop the slash-command's "ask the user to confirm" step (the flag presence already disambiguates).
- **Rehearsal-mode worktree handling is removed from gh-optivem entirely.** It was a dev-workflow concern, not a pipeline feature. Replaced by a separate maintenance script (see Item 8). gh-optivem itself stays focused on running the ATDD pipeline against the current repo / cwd.

**Files (shop, separate commit):**
- `.claude/commands/atdd/atdd-implement-ticket.md` — git rm
- `.claude/commands/atdd/atdd-manage-project.md` — git rm
- `docs/atdd/config.yaml` → `git mv` to `optivem.yaml` at the repo root, then add the new fields (`project.layout`, `project.repos` if multi-repo, `scope.architecture`, `scope.system_lang`, `scope.test_lang`).

**Files (rehearsal-atdd-cli, separate commit):**
- Same `git rm` for the slash-command files.
- Same config relocation: existing `docs/atdd/config.yaml` → `optivem.yaml` at root, populate the new fields.

**Resulting CLI surface:**
```
gh optivem atdd implement-ticket --issue 42
gh optivem atdd implement-ticket --issue 42 --config optivem-multitier.yaml
gh optivem atdd manage-project
gh optivem atdd manage-project --autonomous
```

**`optivem.yaml` after this item:**
```yaml
project:
  url: https://github.com/orgs/optivem/projects/3
  name: Shop Project
  layout: single-repo                  # single-repo | multi-repo
  repos:                                # required only when layout: multi-repo
    - shop                              # implicit-self when single-repo

scope:
  architecture: monolith                # monolith | multitier | both
  system_lang: java                     # java | dotnet | typescript | all
  test_lang: java
```

**Three tiers, each with a clear ownership story:**
- **Embedded in gh-optivem** (canonical, identical for everyone): YAML, agent prompts, diagram.
- **Per-repo config** (`optivem.yaml`): repo-stable facts that legitimately differ — project URL, layout, scope axes.
- **Per-invocation CLI flags**: ticket- or session-specific — `--issue`, `--autonomous`. Document-level overrides (`--yaml`, `--agent-prompt`, `--config`) point at alternate files when you need to deviate from the embedded / repo defaults.

### 8. Author-side maintenance script for rehearsal worktrees

Rehearsal-mode worktree handling moves out of gh-optivem (per Item 7) and into a separate maintenance script for the plan author's personal dev workflow. It is *not* a CLI feature consumers need.

**Files (suggested location: `gh-optivem/scripts/atdd-rehearsal.sh`, or `github-utils/scripts/atdd-rehearsal.sh` if it should live workspace-wide):**

```bash
#!/usr/bin/env bash
# Wraps `gh optivem atdd implement-ticket` in a throwaway git worktree.
#
# Usage: atdd-rehearsal.sh <issue-num> [label]
#   issue-num: GitHub issue number to dispatch.
#   label:     optional [A-Za-z0-9_-]+ tacked onto the worktree id for sortability.
#
# Workflow:
#   1. Resolve <id> = <ts>[-<label>] where <ts>=date +%Y%m%d-%H%M%S.
#   2. Create sibling worktree at ../rehearsal-<id> on branch rehearsal/<id>.
#   3. Cd into it, run `gh optivem atdd implement-ticket --issue <issue-num>`.
#   4. On exit, prompt the user to delete the worktree (default: yes).
```

The script is the user's to author; this plan only reserves the location and codifies the contract (worktree-create → CLI invoke → worktree-cleanup prompt). It can live in `gh-optivem/scripts/` if it's gh-optivem-specific, or `github-utils/scripts/` if other workspace tooling needs the same lifecycle.

### 9. `gh optivem atdd init` — interactive consumer-repo setup

**Files (gh-optivem):**
- `init_commands.go` (or `atdd_commands.go` extension) — Cobra subcommand wired into `gh optivem atdd init`.
- Internal helper for the prompt loop (use a small dependency-free reader; no external prompt library).

**Behaviour:**
- Detect what's detectable: `git remote get-url origin` for the layout default (single-repo) and to seed the project-URL discovery (reuse existing `board.ResolveProjectURL` logic).
- Prompt for the rest interactively, with sensible defaults shown in parens:
  ```
  ? Layout: single-repo / multi-repo? [single-repo]
  ? Architecture: monolith / multitier / both? [monolith]
  ? System language: java / dotnet / typescript / all? [java]
  ? Test language: java / dotnet / typescript / all? [java]
  ? GitHub Project URL (Enter to discover from README/remote):
  ```
- Write the populated config to `<repo-root>/optivem.yaml`.
- **Idempotency**: if `optivem.yaml` already exists, exit non-zero with `init: optivem.yaml already exists at <path>; remove it or edit by hand`. No silent overwrites.
- Non-interactive mode: `--yes` accepts all defaults; flags `--layout`, `--architecture`, `--system-lang`, `--test-lang`, `--project-url` skip individual prompts. (These flags exist *only* for `init`; the running pipeline does not honour them — that's still `--config <path>`.)

**Why init is worth having now:**
With the schema growing (layout, repos, scope axes) and the file moving to project root, hand-authoring a fresh `optivem.yaml` is enough friction that "copy from shop's example" is a brittle answer. Init turns onboarding into one command and centralises the schema's authoring conventions in code (Go validates as it writes, vs. authoring-by-example which silently drifts).

`init` is *only* responsible for `optivem.yaml`. It does **not** write any `.claude/...` files, any per-phase docs, or anything in `docs/`. Those either don't exist (slash commands deleted) or are out of scope.

## Out of scope

- **Versioning of agent prompts.** No `--agent-prompt-version`; defer until anyone needs to pin a prior revision.
- **Migration to a non-Claude LLM runner.** The new architecture makes this possible (no Claude-Code-specific feature in the dispatch path), but actually wiring an alternative runner is its own piece of work.
- **BPMN node renames.** Tracked in `plans/20260501-155353-consolidate-process-flow-with-bpmn.md` and `plans/20260501-144322-process-flow-node-id-rename-open-questions.md`; this plan is purely about *where* the artifacts live, not what they're called.
- **Per-phase doc embedding.** The per-phase docs (`at-red-test.md`, `at-red-dsl.md`, …) referenced by agent prompts via `phase_doc:` may also belong in the engine binary, but the call is less obvious — humans read those docs directly today. Decide as a follow-up.

