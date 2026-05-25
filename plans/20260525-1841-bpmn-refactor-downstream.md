# BPMN refactor — downstream alignment

Phase D handoff from `plans/20260525-1517-bpmn-refactor-yaml-and-diagrams.md` (Item 4). After Phase C landed the new five-level structure (TOP / CYCLE / HIGH / MID / LOW) in `internal/atdd/runtime/statemachine/process-flow.yaml`, this plan aligns the downstream surfaces — prompts, Go runtime comments, ATDD docs, retired SVGs — with the new vocabulary.

## Inputs (read these first)

- **Design archive:** `plans/20260525-1057-bpmn-refactor-design.md` — full Q&A history, Decisions ledger, Q28 prompt-rename table (under "Naming doctrine"), Q28.c resolutions, Q-late-* refinements.
- **YAML source of truth:** `internal/atdd/runtime/statemachine/process-flow.yaml` — already encoded with the new five-level structure. **This plan does NOT touch the YAML.**

## Working style

Per memory `feedback_renames_autonomous_content_gated`:

- `[autonomous]` — pure `git mv` / `rm` only, no body change. Batch + commit without per-file review.
- `[gated]` — any change to file *contents* (splits, body rewrites, new file authoring, YAML/Go edits, prose updates). List files touched, present diffs for review, gate before commit.

Per memory `feedback_prefer_parallel_subagents`:

- `[parallel-safe]` — items annotated with this dispatch one subagent per file or per logical chunk; subagents work concurrently. Sequential items run in the main session.

## Out of scope (already applied — do NOT re-do)

- **Q28.a YAML `agent-name:` removal** — applied during Item 3 of the YAML-and-diagrams plan. `process-flow.yaml` already uses `task-name:` throughout. No YAML edit needed.

## Items

Each item is sized for one `/execute-plan` invocation. Re-running `/execute-plan plans/20260525-1841-bpmn-refactor-downstream.md` picks up the next unchecked item. Resolved items are deleted, not checked.

6. - [ ] **Item 6 — Go-side stale vocabulary audit.** `[gated]` `[sequential]`
    Two distinct concerns; handle in one pass since both touch `*_commands.go`:

    - **6a. Stale "agent-name" wording in comments.** `implement_commands.go:236-240` and `process_commands.go:135` reference "agent-name" in doc comments. Verify whether each reference is stale (BPMN-runtime concept that's now `task-name`) or live (operator-facing `cfg.AgentPrompts` config field — see 6b). Update stale wording to "task-name"; leave live `AgentPrompts` references alone.
    - **6b. `cfg.AgentPrompts` rename decision.** The config field `AgentPrompts` (operator-facing YAML field for prompt-path overrides) keeps the "agent" framing. Decide whether to rename to `TaskPrompts` for vocabulary consistency, or leave as-is (operator-facing config naming is independent of internal BPMN runtime vocabulary). If renamed, audit all touch-points (`projectconfig.Config`, `projectconfig.Validate`, optivem.yaml schema, docs).

    **Done when:** comments updated; rename decision documented in the commit message; tests pass.

7. - [ ] **Item 7 — ATDD docs vocabulary updates.** `[gated]` `[parallel-safe]`
    Item 4 of the parent plan originally pointed at `docs/atdd/process/*.md` + `docs/atdd/architecture/*.md` — those paths do NOT exist. Actual hand-authored docs in `docs/`:

    ```
    docs/how-it-works.md
    docs/architecture-diagram.md
    docs/tbd.md
    docs/gh-monitoring-process.md
    ```

    (Excluded: `docs/process-diagram.md` — regenerated; covered by Item 9.)

    Per-doc triage: does the doc reference BPMN process vocabulary (cycles, phases, RED/GREEN, AT/CT cycles, peak/high/mid/low terminology, agent task names)? If yes, sweep to use the new five-level vocabulary (TOP / CYCLE / HIGH / MID / LOW) + new MID task names. If no BPMN vocabulary present, drop the doc from the file list and note in the commit message.

    Dispatch as parallel subagents (one per doc). Each subagent reads its doc, decides relevance, produces a diff or a "no changes needed" report. Aggregate gate at the end.

    **Done when:** per-doc diffs reviewed, user approves, all committed.

8. - [ ] **Item 8 — Retired SVG cleanup.** `[autonomous]` `[sequential]`
    Delete SVGs under `docs/images/process-diagram-*.svg` that no longer correspond to any node in the regenerated `docs/process-diagram.md` (after Item 9 regenerates it — so this item runs AFTER Item 9, OR uses the design archive's Cross-check inventory as the authoritative retention list).

    **Retention list** (from design archive Cross-check inventory — absorbed-into-new-structure rows):
    - Likely retained (regen produces equivalents): `process-diagram-1-legend.svg`, plus whichever SVGs the regenerated diagram references.
    - Likely retired: all `process-diagram-*-legacy-*.svg`, `process-diagram-13-at-refactor-system.svg` (Q32 absorbed), `process-diagram-4-run-legacy-cycle.svg` (Q16=B no separate legacy run), and any SVG whose node is absorbed under a different name.

    **Execution order:** run Item 9 first to produce the regenerated `docs/process-diagram.md`; diff its `<img src="...">` / referenced SVG list against the existing SVG file list; `rm` the unreferenced ones in this item.

    **Done when:** retired SVGs deleted; `git status` shows only deletions; commit message lists the deleted files.

9. - [ ] **Item 9 — Regenerated diagrams review.** `[gated]` `[sequential]`
    Run `gh optivem process show` to regenerate `docs/process-diagram.md` from the new YAML. Review the regenerated doc for:
    - Correct rendering of all 5 levels (TOP, CYCLE, HIGH, MID, LOW)
    - All CYCLEs and TOP processes present
    - Gateway lookup table in `implement-ticket` renders correctly
    - No orphan / dangling diagram references
    - Mermaid syntax valid (no rendering errors on github.com preview)

    Present the regenerated diff for user review. If acceptable, commit. If not, file issues against the encoding (Items in the parent plan's YAML migration may need follow-up).

    Note: this item runs BEFORE Item 8 (Item 8 needs Item 9's output to determine retired SVGs).

    **Done when:** `docs/process-diagram.md` regenerated, reviewed, committed.

---

## Re-running `/execute-plan`

Invoke `/execute-plan plans/20260525-1841-bpmn-refactor-downstream.md` repeatedly. Each invocation reads this file, finds the next unchecked Item, executes it (with parallel-subagent fan-out where `[parallel-safe]`), deletes the resolved item, commits.

Suggested order for max parallelism (some items can run concurrently in separate sessions if you launch multiple terminals):

1. Item 1 (renames) — must run first; everything else depends on the renamed filenames existing.
2. Item 2 (deletes) — independent of Item 1; can run in parallel session.
3. Items 3, 4, 5 — depend on Item 1; among themselves use parallel subagents per file.
4. Item 6 (Go audit) — independent of prompt items; can run any time after Item 1.
5. Item 9 → Item 8 → Item 7 — regenerate first, then SVG cleanup using regen output, then doc updates (which may reference the regenerated diagram).

## Standing constraints (from user memory)

- **Renames autonomous, content changes gated** (`feedback_renames_autonomous_content_gated`) — the structural rule for every Item in this plan.
- **Prefer parallel subagents for independent work** (`feedback_prefer_parallel_subagents`) — items tagged `[parallel-safe]` use subagent fan-out.
- **Reuse existing Go code — don't reinvent** — before adding Go-side changes (Item 6), grep for existing types/functions/parsers that already do the job.
- **Token-efficient by default** (`feedback_flag_non_token_efficient`) — flag any costly workflow; offer cheaper alternative.
- **Session-handoff cadence: auto-commit, then surface `/clear` + `/execute-plan`** (`feedback_offer_clear_then_execute_plan`, `feedback_execute_plan_always_next_steps`).
- **Concurrent-agent collision risk** (`feedback_concurrent_agent_collision`) — re-inspect `git log` before staging if mid-session new commits appear.
- **Legacy test artifacts indistinguishable from AT/CT artifacts** (`feedback_legacy_tests_no_marker`) — Item 2's `legacy-*.md` deletes are correct; do not preserve any legacy marker on disk.
- **No layer-coding in architectural names** (`feedback_no_layer_coding_in_names`) — when authoring split / merged prompt bodies in Item 3, names describe scope, not layer.
