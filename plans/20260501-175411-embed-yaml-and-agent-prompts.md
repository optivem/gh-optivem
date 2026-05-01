# Embed orchestration YAML and agent prompts in gh-optivem; drop consumer-side copies and Claude Code subagent dependency

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

### 1. Move YAML out of `testdata/`; embed canonically; default driver to it

**Files (gh-optivem):**
- `internal/atdd/runtime/statemachine/testdata/process-flow.yaml` → move to `internal/atdd/runtime/statemachine/process-flow.yaml`
- `internal/atdd/runtime/statemachine/embed.go` (NEW)
- `internal/atdd/runtime/statemachine/transitions_test.go` (use `LoadDefault`)
- `internal/atdd/runtime/driver/driver.go` (drop `DefaultYAMLPath`; branch on empty `YAMLPath`)
- `atdd_commands.go` (update `next-phase` debug helper)

The YAML stops being a test fixture and becomes the canonical source. Both production and tests load via `//go:embed`.

```go
// internal/atdd/runtime/statemachine/embed.go
package statemachine

import _ "embed"

//go:embed process-flow.yaml
var DefaultYAML []byte

// LoadDefault loads the canonical embedded process-flow document.
// Equivalent to LoadBytes(DefaultYAML).
func LoadDefault() (*Engine, error) {
    return LoadBytes(DefaultYAML)
}
```

`driver.Run` chooses embedded vs. file:

```go
var eng *statemachine.Engine
if opts.YAMLPath == "" {
    eng, err = statemachine.LoadDefault()
} else {
    eng, err = statemachine.LoadFile(opts.YAMLPath)
}
```

`DefaultYAMLPath` and the YAML default in `withDefaults` go away. Update the package comment that calls gh-optivem "repo-agnostic by design" — that framing is obsolete after this item.

### 2. Embed agent prompts; rewrite `clauderun.Dispatch` to inline them; drop the Task-subagent indirection

**Files (gh-optivem):**
- `internal/atdd/runtime/agents/prompts/<name>.md` (NEW; ~11 files migrated from `shop/.claude/agents/atdd/`)
- `internal/atdd/runtime/agents/embed.go` (NEW)
- `internal/atdd/runtime/clauderun/clauderun.go` (rewrite render path)
- `internal/atdd/runtime/clauderun/clauderun_test.go` (update assertions)
- `internal/atdd/runtime/driver/driver.go` (update dispatcher wiring; `agentNames` becomes a filesystem walk of embedded prompts)
- `internal/atdd/runtime/driver/driver_test.go`

Per-agent prompt body is migrated in self-contained form: any `@includes` pointing at per-phase docs (`at-red-test.md`, etc.) are resolved at copy-in time so the embedded prompt is one file with no Claude-Code-specific resolution. Substitution placeholders use `${name}` (matching the YAML's existing param syntax — see `ExpandParams` in `run.go`) so authors only learn one substitution dialect.

```go
// internal/atdd/runtime/agents/embed.go
package agents

import (
    "embed"
    "fmt"
)

//go:embed prompts/*.md
var promptFS embed.FS

// Prompt returns the embedded prompt template for the given agent name.
// Returns an error if no prompt is embedded under that name.
func Prompt(name string) ([]byte, error) {
    data, err := promptFS.ReadFile("prompts/" + name + ".md")
    if err != nil {
        return nil, fmt.Errorf("agents: no embedded prompt for %q", name)
    }
    return data, nil
}

// Names returns every embedded agent name (filesystem-walked once at startup).
func Names() []string { … }
```

`clauderun.Dispatch` reads the embedded template, runs `${name}` substitution against ticket context (`${issue_num}`, `${issue_title}`, `${issue_repo}`, `${project_title}`, `${project_url}`, `${phase_doc}`, `${phase}`, …), and passes the rendered string as the sole input to `claude -p`. The "Launch the `<name>` subagent" templating is removed. The `--manual-agents` v1 fallback continues to work (it prints a banner and blocks on stdin — no `claude` invocation either way).

### 3. Override flags — `--yaml`, `--agent-prompt`, `--config`

**Files (gh-optivem):**
- `internal/atdd/runtime/driver/driver.go` (add `Options.AgentPromptOverrides map[string]string`, `Options.ConfigPath string`)
- `atdd_commands.go` (wire `--yaml`, `--agent-prompt`, `--config` into `implement-ticket` and `manage-project`)

| Flag | Overrides | Default |
|---|---|---|
| `--yaml <path>` | embedded process-flow YAML | embedded canonical |
| `--agent-prompt <name>=<path>` (repeatable) | one named agent prompt | embedded canonical |
| `--config <path>` | path to project config | `<repo-root>/optivem.yaml` |

All three follow the same shape: override-by-pointing-at-a-file, not override-by-individual-value. Per-axis overrides (`--architecture`, `--system-lang`) are deliberately *not* added — if you need to deviate from the repo's `config.yaml`, you point at a different config file (itself a stable artifact you can check in or keep around). Same discipline as `--yaml`: scope changes are document-level, not flag-level.

This makes the override story a uniform CLI surface, not a filesystem layout. `--help` documents what's overridable; consumers don't need to know that `.claude/agents/atdd/` or `docs/atdd/` are magic directories.

### 4. Author-side rendered diagram in gh-optivem; consumers view, not generate

**Files (gh-optivem):**
- `internal/atdd/runtime/diagram/` (NEW; emits the Mermaid markdown wrapper from `statemachine.Engine`)
- `docs/process-flow-diagram.md` (NEW; rendered output, committed to gh-optivem)
- `atdd_commands.go` (`gh optivem atdd show diagram` subcommand — prints the embedded markdown to stdout)
- `.github/workflows/regenerate-diagram.yml` (NEW; on push that touches `internal/atdd/runtime/statemachine/process-flow.yaml`, regenerates `docs/process-flow-diagram.md` and commits if changed) — OR a pre-commit hook in `.git/hooks/` shipped via the contributor-setup script
- `README.md` link to the rendered diagram

**Files (shop):**
- `.claude/agents/atdd/meta/diagram-generator.md` — delete (no longer needed; gh-optivem owns the diagram)
- `docs/atdd/process/diagram-process.md` — delete; replace any prose-doc cross-reference with a link to `https://github.com/optivem/gh-optivem/blob/main/docs/process-flow-diagram.md`

**Why author-side, not user-side:**

- gh-optivem ships exactly one rendered diagram, regenerated automatically whenever the YAML changes. github.com renders Mermaid natively, so anyone browsing the gh-optivem repo sees the diagram with zero tooling.
- No per-consumer-repo `diagram-process.md` — same consolidation principle as the YAML and prompts.
- Consumer prose docs link to a stable github.com URL on gh-optivem; the link survives consumer-side moves.
- `gh optivem atdd show diagram` covers the offline / pipeline-it-into-something-else case (prints the embedded markdown to stdout). Useful but rarely needed.
- Aligns with `CLAUDE.md`'s "docs as plain markdown under `docs/`, linked via relative paths from the README" pattern.

Generation is part of gh-optivem's own dev workflow (CI hook on YAML change), not the consumer's. The user does not generate the diagram; the user views it.

Prose-doc cross-references in shop / rehearsal-atdd-cli that today point at `docs/atdd/process/process-flow.yaml` are rewritten to point at the gh-optivem-hosted diagram or removed.

### 5. Delete consumer-side copies

**Files (shop, separate commit):**
- `docs/atdd/process/process-flow.yaml` — git rm
- `.claude/agents/atdd/<name>.md` — git rm (~11 files)
- `.claude/agents/atdd/meta/diagram-generator.md` — git rm
- `docs/atdd/process/diagram-process.md` — git rm (gh-optivem now hosts the canonical rendered diagram)
- Skill shells under `.claude/commands/atdd/` — KEEP (they save typing; they're 5-line wrappers and don't drift)
- Any prose-doc cross-reference touching the deleted files

**Files (rehearsal-atdd-cli, separate commit):**
- Same `git rm` cleanup as shop.

This is the payoff: every consumer repo carries zero ATDD orchestration scaffolding. New repos opt in by installing `gh-optivem` and (optionally) running an `init` to drop the slash-command shells.

### 6. Smoke-test the consumer-empty path

**Files (gh-optivem):**
- A new integration test that constructs a temp repo with no `.claude/` and no `docs/atdd/process/` and runs `gh optivem atdd implement-ticket --issue 42` (mocked clauderun + git) end-to-end. Asserts the dispatch completes against embedded artifacts only.

This locks the property that future schema changes don't accidentally reintroduce a consumer-side dependency.

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

