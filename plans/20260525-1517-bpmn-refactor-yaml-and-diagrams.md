# BPMN refactor — encode YAML + render diagrams

Phase C + Phase D of the BPMN five-level refactor (TOP / CYCLE / HIGH / MID / LOW). Encodes the new structure into `internal/atdd/runtime/statemachine/process-flow.yaml`, regenerates `docs/process-diagram.md` via `gh optivem process show`, and writes the downstream-alignment plan as a Phase D handoff.

> **Working style: token-efficient.** Execute in the cheapest form that still produces a quality result. If a workflow burns tokens unnecessarily, surface the cheaper alternative and let the user choose. (Memory: `feedback_flag_non_token_efficient`.)

## Inputs (read these first)

This plan does NOT re-host the design rationale — it executes a design that has already been settled.

- **Design archive:** `plans/20260525-1057-bpmn-refactor-design.md` — full Q&A history (Q1–Q34 + Q-new-1/2/3 + Q-ext), cross-check inventory of the 21 existing diagrams, prompt-rename table. Use this when you need the *why* behind any decision below.
- **Brainstorm sources (authoritative inputs):** `plans/ideas/1-5-bpmn-refactor-*-level.md` — LOW / MID / HIGH / CYCLE / TOP. These are the design surface this plan encodes into YAML. They must be Q-tag-stripped first (parent plan's Item 12) — verify before starting Item 1.

## Scope

- Full replacement of the existing BPMN (Q17 = A). All 21 existing process diagrams either **absorbed** into the new five-level structure or **dropped** with explicit rationale (cross-check inventory in the archive).
- The existing `gh optivem process show` rendering pipeline stays (Q23 = A). No new renderer, no per-level Mermaid split (Q14 = A).
- `docs/process-diagram.md` regenerated from the new YAML at each item that touches `process-flow.yaml`. **No hand-drawn Mermaid.**
- Phase D writes a separate downstream-alignment plan (Q21 = A); this plan does NOT execute the downstream work.

## Decisions ledger (compact)

Single-line summaries. For options considered + rationale, read the archive plan's `## Decisions` section.

### Structure (Q17, Q18, Q26)
- Five-level: **TOP / CYCLE / HIGH / MID / LOW.** Brainstorms exhaustive + final.

### Naming (Q15, Q27, Q28.a, Q29)
- Kebab-case lowercase **everywhere** — YAML keys, doc headings, prompt filenames, in-prose refs, anchor slugs, Go struct tags.
- Verb-based identifiers; "Write" for tests, "Implement" for code.
- HIGH composites use full-scope names: `implement-and-verify-system`, `refactor-and-verify-tests`, `write-and-verify-acceptance-tests`.
- **`agent-name:` field DROPPED from `process-flow.yaml`.** Runtime derives `prompt_path(task_name) = task_name + ".md"`; errors at startup if prompt file missing.

### LOW primitives
- 4 primitives: `approve`, `execute-agent`, `execute-command`, **`fix`** (new — single attempt, PRE-approval only, no own validation; caller re-validates).
- `approve` NO-branch: exit-only (caller owns NO). Add `TODO` comment in YAML for possible revisit (parameterized NO-action OR split into two primitives). **Confirmed during refine — not blocking; the TODO is a future-work marker, not a Phase-C dependency.**
- `execute-command` is PRE-only (asymmetric with agent's PRE+POST — intentional, machine-checkable success).
- Terminology: `calls` (BPMN call-activity).

### MID tasks
- `run-tests` — single task with structured filter params (Q5.a resolved → structured discriminated union):
    - `filter-type:` enum — `test-type` | `test-name`
    - `filter-value:` — depends on `filter-type` (single tag string for `test-type`; list of strings for `test-name`)
    - Both params absent ⇒ run all tests.
    - Both params kebab-case per the plan's kebab-everywhere decision (Q15/Q27). Existing snake_case keys in `process-flow.yaml` (`phase_id`, `compile_action`, …) are a separate open question; do not retro-rename in this plan.
- `write-contract-tests` added (symmetric with `write-acceptance-tests`).
- Vocabulary unified: `-driver-adapters` (not `-drivers`).
- Tasks added by Item 11's connectedness pass (already in brainstorms): `implement-system`, `implement-external-system-stubs`, `refactor-tests`, `refactor-system`, `refine-acceptance-criteria`, `update-ticket`, `build-system`, `start-system`.

### HIGH orchestrations (Q31 = D; names refined per Q-new-6)
- Parameterized core `write-and-verify-acceptance-tests` (`<Expected Test Result>` threaded through step 1 + `implement-test-layer`).
- Two thin wrappers: `write-and-verify-acceptance-tests-fail` (Failure) + `write-and-verify-acceptance-tests-pass` (Success).
- CYCLEs call **wrappers**, not the core — call sites are parameter-free and self-documenting.
- No `(BIG)` suffix; no `red`/`green` in HIGH names — parameterized via `implement-test-layer` (Q-new-1).
- `refactor-and-verify-tests` is a distinct HIGH (refactor-tests → compile-tests → verify-tests-pass → commit). `refactor-system-structure` reuses `implement-and-verify-system`.

### CYCLE composition
Seven cycles:

- `change-system-behavior` — step 3 is loopable opportunistic refactor menu (calls existing CYCLEs in *no-checklist / opportunistic mode*; Q33).
- `cover-system-behavior` — legacy-coverage cycle, expected=Success (Q16 = B, parameterized expectation instead of separate legacy cycles). **Q31.a resolved → Option A (nested):** outer cycle is acceptance-driven (operator specifies acceptance criteria only); CT is a nested sub-process called when the AT layer needs a stub/driver to pass — structurally mirrors `change-system-behavior`. Gateway stays at one row: `task/cover-legacy` → `cover-system-behavior`. **Not** split into `task/cover-legacy-acceptance` + `task/cover-legacy-contract` (operator doesn't think in CT subtypes — CT is an implementation detail of making AT pass).
- `redesign-system-structure` — step 1 splits into `1a implement-system-driver-adapters` + `1b implement-external-system-driver-adapters` (MID-direct, no umbrella; Q-new-2).
- `refactor-system-structure` — step 1 calls HIGH `implement-and-verify-system`.
- `refactor-test-structure` — step 1 calls HIGH `refactor-and-verify-tests`.
- `refine-backlog` — standalone sibling (Q9 = A).
- `onboard-external-system` — standalone (Q-ext = b); `redesign-system-structure` may call it as a sub-process.

### TOP processes
Three:

- `refine-ticket` — backlog grooming; steps include `update-ticket` calls for state transitions (IN REFINEMENT / READY).
- `implement-ticket` — Mark IN PROGRESS → mechanical-lookup gateway by ticket-type + optional `task` subtype → call chosen CYCLE → Mark IN ACCEPTANCE. Unknown subtypes **hard-exit at gateway**. Single-cycle tickets only (multi-cycle work splits during refinement; Q30.b = A).
- `refactor` — ad-hoc, no ticket overhead (Q34). Loopable refactor-type chooser; no Mark-Ticket bookends.

### Gateway lookup table (TOP `implement-ticket`)
Already lives in `plans/ideas/5-bpmn-refactor-top-level.md`. Encode as the YAML gateway:

| Ticket type / subtype | CYCLE |
|---|---|
| `story` | `change-system-behavior` |
| `bug` | `change-system-behavior` |
| `task/cover-legacy` | `cover-system-behavior` |
| `task/redesign-system` | `redesign-system-structure` |
| `task/refactor-system` | `refactor-system-structure` |
| `task/refactor-tests` | `refactor-test-structure` |
| `task/onboard-external-system` | `onboard-external-system` |

### Contract blocks (Q13 = A)
- Live in `process-flow.yaml` as `user_task` metadata: `scopes:` + `outputs:`.
- Both consumers read the same YAML — (1) agent invocation uses them for prompt context + permitted file scope; (2) post-execute BPMN verify reads `outputs:` to validate "required output variables present?" and `scopes:` to validate "scope constraints satisfied? (diff)".
- Schema/generator extension (Item 2) likely needed to add these fields.

### Cross-check resolution highlights
- Diagram #13 (`at-refactor-system`) → `change-system-behavior` step 3 opportunistic loop (Q32). `at-refactor-system.md` prompt: DROP.
- Diagram #4 (`run-legacy-cycle`) → DROP (no separate legacy test-run operation; `run-tests` runs whatever's in the suite).
- Diagrams #12, #18, #19 → CYCLE `cover-system-behavior` (Q16 = B parameterized expectation).
- All 15 standard absorption rows walked and confirmed (Item 6, parent plan).
- Full inventory + rationale per row: archive plan's "Cross-check vs existing BPMN" section.

### Refined decisions (added during refine walk)
- **Kebab-vs-snake scope.** Q15/Q27 (kebab everywhere) applies **only to new keys** added in this refactor (e.g. `filter-type`, `filter-value`, `scopes`, `outputs`). Existing snake_case keys in `process-flow.yaml` (`phase_id`, `compile_action`, `change_type`, …) **stay snake_case**. A retroactive rename is a separable cleanup, not part of Phase C — open a follow-up plan if/when consistency becomes worth the churn.

### Exploration backlog (not blocking; confirmed during refine — no Phase-C work)
- `spike` ticket type — currently no entry in the gateway table. Future: cycle? Or own TOP process? Defer until a real `spike` workflow exists.
- Multi-cycle ticket model — currently rejected (Q30.b = A); revisit if real workflow demands it.

---

## Items

No execution items remain. Phase C is fully landed (Items 1–3 encoded the YAML); Phase D's downstream-alignment plan was written as **`plans/20260525-1841-bpmn-refactor-downstream.md`** (Item 4). Invoke `/execute-plan` on that file from here on.

## Re-running `/execute-plan`

No more invocations on this file. Continue with `/execute-plan plans/20260525-1841-bpmn-refactor-downstream.md`.

## Standing constraints (from user memory)

- **Reuse existing Go code — don't reinvent.** Before writing new Go (schema fields, generators, validators, runtime helpers), grep `internal/atdd/**` and `internal/projectconfig/**` for existing types/functions/parsers that already do the job. The repo already encodes a lot of the BPMN runtime (process loader, gateway dispatcher, agent runner, diagram emitter, scope validator, phase-scopes lookup, prompt-path resolver) — most of this plan's Items should be *re-wiring + YAML*, not *new code*. If you can't find an existing primitive for something you need, **stop and ask the user** — name what you searched for and what was missing — instead of writing a fresh implementation. (Memory: bias toward reuse; the user has frequently noted "we already have code for this.")
- **Token-efficient by default** — flag any user-proposed workflow that burns tokens unnecessarily (`feedback_flag_non_token_efficient`).
- **Session-handoff cadence: auto-commit, then surface `/clear` + `/execute-plan`.** End-of-item: auto-commit with a surgical message via raw `git` (no `/commit` per `feedback_use_commit_skill`); then surface the literal next-session commands in a Next-steps block (`feedback_offer_clear_then_execute_plan`, `feedback_execute_plan_always_next_steps`).
- **Concurrent-agent collision risk** — re-inspect `git log` before staging if mid-session new commits appear (`feedback_concurrent_agent_collision`).
- **Legacy test artifacts** are indistinguishable from AT/CT artifacts on disk — no folder, no annotation, no filename suffix (`feedback_legacy_tests_no_marker`). Authoring cycles collapse per Q16 = B in the archive.
