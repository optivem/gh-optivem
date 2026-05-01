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

### 3. Override flags — `--yaml`, `--agent-prompt`

**Files (gh-optivem):**
- `internal/atdd/runtime/driver/driver.go` (add `Options.AgentPromptOverrides map[string]string`)
- `atdd_commands.go` (wire `--yaml` and `--agent-prompt` into `implement-ticket` and `manage-project`)

`--yaml <path>` overrides the embedded process-flow YAML for one invocation. `--agent-prompt <name>=<path>` (repeatable) overrides one named agent prompt. Both default to embedded.

This makes the override story a CLI flag, not a filesystem layout. `--help` documents what's overridable; consumers don't need to know that `.claude/agents/atdd/` is a magic directory.

### 4. Replace shop's `diagram-generator` agent with `gh optivem atdd diagram`

**Files (gh-optivem):**
- `internal/atdd/runtime/diagram/` (NEW; emits Mermaid from `statemachine.Engine`)
- `atdd_commands.go` (`gh optivem atdd diagram` subcommand)

**Files (shop):**
- `.claude/agents/atdd/meta/diagram-generator.md` (delete or thin to: "run `gh optivem atdd diagram > docs/atdd/process/diagram-process.md`")
- `docs/atdd/process/diagram-process.md` (regenerated by the new CLI command in CI / on demand)

The diagram-generator agent currently reads the consumer YAML to produce Mermaid. After items 1–2 the consumer YAML disappears, so the agent has nothing to read. Replace with a CLI command — `gh optivem atdd diagram` walks the embedded YAML's `Engine` directly and emits Mermaid. Diagram regeneration becomes deterministic, runs in CI, and stops costing agent tokens.

Prose-doc cross-references that today point at `docs/atdd/process/process-flow.yaml` (e.g. `task-and-chore-cycles.md`) are updated to point at `gh optivem atdd diagram` or removed.

### 5. Delete consumer-side copies

**Files (shop, separate commit):**
- `docs/atdd/process/process-flow.yaml` — git rm
- `.claude/agents/atdd/<name>.md` — git rm (~11 files)
- `.claude/agents/atdd/meta/diagram-generator.md` — git rm
- `docs/atdd/process/diagram-process.md` — keep, but regenerated by the new CLI
- Skill shells under `.claude/commands/atdd/` — KEEP (they save typing; they're 5-line wrappers and don't drift)
- Any prose-doc cross-reference touching the deleted files

**Files (rehearsal-atdd-cli, separate commit):**
- Same `git rm` cleanup as shop.

This is the payoff: every consumer repo carries zero ATDD orchestration scaffolding. New repos opt in by installing `gh-optivem` and (optionally) running an `init` to drop the slash-command shells.

### 6. Smoke-test the consumer-empty path

**Files (gh-optivem):**
- A new integration test that constructs a temp repo with no `.claude/` and no `docs/atdd/process/` and runs `gh optivem atdd implement-ticket --issue 42` (mocked clauderun + git) end-to-end. Asserts the dispatch completes against embedded artifacts only.

This locks the property that future schema changes don't accidentally reintroduce a consumer-side dependency.

## Out of scope

- **Versioning of agent prompts.** No `--agent-prompt-version`; defer until anyone needs to pin a prior revision.
- **Migration to a non-Claude LLM runner.** The new architecture makes this possible (no Claude-Code-specific feature in the dispatch path), but actually wiring an alternative runner is its own piece of work.
- **Removing slash-command shells.** Keep `.claude/commands/atdd/atdd-implement-ticket.md` and `atdd-manage-project.md` in shop — they're typing convenience, rarely change, and removing them is a UX regression with no architectural benefit.
- **BPMN node renames.** Tracked in `plans/20260501-155353-consolidate-process-flow-with-bpmn.md` and `plans/20260501-144322-process-flow-node-id-rename-open-questions.md`; this plan is purely about *where* the artifacts live, not what they're called.
- **Per-phase doc embedding.** The per-phase docs (`at-red-test.md`, `at-red-dsl.md`, …) referenced by agent prompts via `phase_doc:` may also belong in the engine binary, but the call is less obvious — humans read those docs directly today. Decide as a follow-up.

## Open questions

1. **Prompt template syntax**: `${name}` (matches the YAML's existing param substitution) vs. `text/template` (`{{.IssueNum}}`). Recommendation: `${name}` for one-syntax consistency. Confirm.
2. **Slash-command shells in shop**: keep them, or also delete and have users type `gh optivem atdd …` directly? Recommendation: keep — typing convenience, no drift cost. Confirm.
3. **`gh optivem atdd init`**: do we want a one-shot command that drops the slash-command shells into `.claude/commands/atdd/` for new repos? Adjacent to scope; could land later.
4. **Sequencing with the BPMN plan**: this plan should land first; afterwards `plans/20260501-155353-consolidate-process-flow-with-bpmn.md` Item 1 reduces to a 1-repo change in gh-optivem. Confirm sequencing.
5. **Diagram CLI format**: emit straight Mermaid to stdout, or emit a full markdown wrapper (header, generated-warning, sources block) to match today's `diagram-process.md` shape? Recommendation: full wrapper — drop-in replacement for the agent's output.
