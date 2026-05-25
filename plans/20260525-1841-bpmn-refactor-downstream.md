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

1. - [ ] **Item 1 — Prompt file renames.** `[autonomous]` `[sequential]`
    Apply the renames from the Q28 table (archive's "Q28 prompt rename table") via `git mv`. Body content unchanged in this item (body alignment is Item 4).

    ```
    at-red-test.md                            → write-acceptance-tests.md
    ct-red-test.md                            → write-contract-tests.md
    at-red-system-driver.md                   → implement-system-driver-adapters.md
    ct-red-external-system-driver.md          → implement-external-system-driver-adapters.md
    at-green-system.md                        → implement-system.md
    ct-green-external-system-stub.md          → implement-external-system-stubs.md
    refine-acc.md                             → refine-acceptance-criteria.md
    task-system-implementation-refactoring.md → refactor-system.md
    ```

    Special cases (NOT in this item):
    - `at-red-dsl.md` + `ct-red-dsl.md` collapse to one `implement-dsl.md` — needs content merge, handled in Item 3.
    - `disable-tests.md`, `enable-tests.md`, `update-ticket.md` — no rename per Q28 table.

    All renames under `internal/assets/runtime/prompts/atdd/`. Commit message: `prompts/atdd: rename per Q28 table (BPMN downstream item 1)`.
    **Done when:** all 8 `git mv` operations committed; `git status` clean; no body content changed.

2. - [ ] **Item 2 — Prompt file deletes.** `[autonomous]` `[sequential]`
    Delete the 8 retired prompts:

    ```
    at-refactor-system.md                       (Q32 — folded into CYCLE opportunistic mode)
    legacy-at-test.md                           (Q16=B collapse)
    legacy-at-dsl.md                            (Q16=B collapse)
    legacy-at-system-driver.md                  (Q16=B collapse)
    legacy-ct-test.md                           (Q16=B collapse)
    legacy-ct-dsl.md                            (Q16=B collapse)
    legacy-ct-external-system-driver.md         (Q16=B collapse)
    legacy-ct-external-system-stub.md           (Q16=B collapse)
    ```

    All under `internal/assets/runtime/prompts/atdd/`. Commit message: `prompts/atdd: delete retired prompts (BPMN downstream item 2)`.
    **Done when:** all 8 files deleted; `git status` clean; no other file modified.

3. - [ ] **Item 3 — Prompt splits + content merges (creates new files).** `[gated]` `[parallel-safe]`
    Three independent content operations. Dispatch as parallel subagents — each produces new prompt body content for review.

    - **3a. Split `fix-verify.md`** (Q28.b) → `fix-unexpected-passing-tests.md` + `fix-unexpected-failing-tests.md`. The current `fix-verify.md` switches on `${failure_type}` (compile vs test); the split produces two single-purpose prompts. Both new files need bodies authored from scratch using the relevant subset of the original.
    - **3b. Collapse `at-red-dsl.md` + `ct-red-dsl.md` → `implement-dsl.md`** (Q28 table — collapse row). Merge the two existing bodies into one parameterized prompt that handles both AT-side and CT-side DSL implementation.
    - **3c. Split `task-system-interface-redesign.md` + `task-external-system-interface-redesign.md`** (Q28.c recommended resolution): each composite prompt splits into a system-surface portion (folds into `implement-system.md`) and a driver-adapter absorption portion (folds into `implement-system-driver-adapters.md` / `implement-external-system-driver-adapters.md`). After split, the two source files are deleted.

    **Coordination with Item 4 (body alignment):** 3b and 3c merge content INTO files that Item 4 will also rewrite. Two safe orderings:
    - Run Item 3 first, then Item 4 sweeps the merged content during body alignment.
    - Or run Item 4 first on the rename-only files, and let Item 3's merge subagents produce already-aligned bodies.
    Pick whichever surfaces first in `/execute-plan`; the gate catches misalignment either way.

    **Done when:** for each sub-item (3a/3b/3c), new file bodies presented for review, user approves diffs, files committed under `internal/assets/runtime/prompts/atdd/`, source files for 3b/3c deleted.

4. - [ ] **Item 4 — Prompt body alignment.** `[gated]` `[parallel-safe]`
    For each renamed prompt from Item 1, sweep the body to use the new MID task name + new five-level vocabulary. The current bodies reference the old noun-based identifiers (`at-red-test`, `at-green-system`, etc.) and old framing ("AT cycle", "RED phase", etc.); rewrite to use the verb-based MID task names (`write-acceptance-tests`, `implement-system`, etc.) and the new CYCLE/HIGH/MID terminology.

    File list (parallel-safe, one subagent per file):

    ```
    write-acceptance-tests.md
    write-contract-tests.md
    implement-system-driver-adapters.md
    implement-external-system-driver-adapters.md
    implement-system.md
    implement-external-system-stubs.md
    refine-acceptance-criteria.md
    refactor-system.md
    ```

    Plus the files written in Item 3 if the merged bodies were not pre-aligned: `fix-unexpected-passing-tests.md`, `fix-unexpected-failing-tests.md`, `implement-dsl.md`.

    Also covers the **Q4 terminology sweep** (replace "inherits" / "instantiates" → "calls" wherever the BPMN call-activity is referenced in prompt bodies). Q4 is a cross-prompt concern; fold into each per-file pass rather than running as its own surface.

    **Done when:** each file diff presented for review, user approves, all committed under `internal/assets/runtime/prompts/atdd/`. Commit message per subagent: `prompts/atdd: align <file> body to new MID task vocabulary`.

5. - [ ] **Item 5 — Q1 + Q5 prompt content updates.** `[gated]` `[parallel-safe]`
    Two independent content concerns; dispatch as two parallel subagents.

    - **5a. Q1 — FIX as separate primitive.** Sweep all prompt bodies that currently describe FIX as embedded inside EXECUTE AGENT (or as a recursive call). Reshape to reflect the resolved Q1=A doctrine: FIX is a distinct LOW primitive with its own contract, single-attempt termination, PRE-approval only, no own validation. Affected files likely include any fix-* prompt body and any prompt that describes the FIX call site. Specific file inventory: run `grep -ln 'fix' internal/assets/runtime/prompts/atdd/*.md` and triage.
    - **5b. Q5 — run-tests filter parameter shape.** Sweep all prompt bodies that reference the run-tests task. The structured shape per this plan's parent (Decisions ledger):
      - `filter-type:` enum — `test-type` | `test-name`
      - `filter-value:` — single tag string for `test-type`; list of strings for `test-name`
      - Both absent ⇒ run all tests
      Replace any older filter-parameter wording (single-string-with-prefix, single `filter:` field, etc.) with the structured shape. Affected files: any prompt that documents the run-tests invocation. Triage with `grep -ln 'run.tests\|RunTests\|run-tests' internal/assets/runtime/prompts/atdd/*.md`.

    **Done when:** each subagent's diffs presented for review, user approves, files committed.

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
