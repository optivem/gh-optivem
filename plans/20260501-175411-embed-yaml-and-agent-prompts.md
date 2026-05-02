# Embed orchestration YAML and agent prompts in gh-optivem; drop consumer-side copies and Claude Code subagent dependency

> 🤖 **Picked up by agent** — `Valentina_Desk` at `2026-05-02T11:38:56Z`

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

