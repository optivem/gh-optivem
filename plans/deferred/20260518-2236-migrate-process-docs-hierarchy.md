# Plan: migrate process docs into the new hierarchy (replace `internal/assets/global/docs/atdd/process/` contents)

> **Note (2026-05-20):** Superseded re: `chore` naming by `plans/20260520-1145-system-implementation-refactoring-rename.md`. References to `chore.md`, `subtype:system-implementation-change`, and `CHORE_CYCLE` below are stale; map them to `task-system-implementation-refactoring.md`, `subtype:system-implementation-refactoring`, and `SYSTEM_IMPLEMENTATION_REFACTORING_CYCLE` (see that plan for Decisions A/B) when/if this plan is reactivated.

> ⏳ **DEFERRED PERMANENTLY 2026-05-19** — superseded by `20260519-0922-bpmn-rewire-process-docs-to-new-hierarchy.md` (now fully landed: see commits `470c67a`, `be12d7b`, `46ef833`). 0922 owns the reference-rewrite work against the actual post-archive filesystem (no numeric prefixes; old files in `_ARCHIVED_PENDING_DELETE/` rather than to-be-deleted). Per Needs-decision §1 of `plans/20260519-0929-meta-bpmn-ssot-coordination.md` (resolved 2026-05-19, meta-plan since discharged). Kept here for archival reference only; do not execute.

> ⚠️ **NOT YET REFINED** — run `/refine-plan` on this file before `/execute-plan`. Several items below have orphan-file decisions and an in-flight-plan reconciliation that must be walked through interactively.

**Date:** 2026-05-18

## Decisions log

Recorded during interactive refinement. Each entry locks a decision and SUPERSEDES the matching row in the **Open questions** / **Items** sections below.

| # | Date       | Topic                              | Decision                                                                                                                                  | Status      |
|---|------------|------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------|-------------|
| D0 | 2026-05-18 | Migration shape                    | **Cut-paste, not re-point.** Embed wiring (`//go:embed runtime global`) stays unchanged; new hierarchy moves INTO `internal/assets/global/docs/atdd/process/`. No `MaterializeProject` / sync changes. | ✅ Locked   |
| D1 | 2026-05-18 | `glossary.md` destination *(revised under D2)* | **Split by agent-runtime relevance.** Agent-relevant definitions → `docs/atdd/process/shared/glossary.md`. Human-only design-context definitions → `docs/archived/glossary.md`. Concretely: "Interface Change" → `shared/`; "Behavioral Change" / "Structural Change" / "Legacy Coverage" → `archived/`. **Why:** the three classification terms are intake-time concepts handled by the Go runtime in `internal/atdd/runtime/classify/` — by dispatch time, phase agents are already in the right cycle and don't need to re-classify. Only "Interface Change" is evaluated at runtime by phase agents (at the AT-RED-DSL / CT-RED-DSL gates). | ✅ Locked   |
| D2 | 2026-05-18 | Archive pattern (meta) | **`docs/archived/` (top-level) for human-only content.** Universal rule for this plan: if a fragment is needed by agents at runtime, it goes into `docs/atdd/process/`. If it's for humans only (design rationale, BPMN-enforced behavior, historical context), it goes to `docs/archived/`. Archive content is **link-OUT-only** — archived docs may link to live docs; live docs never link into `archived/`. **Why:** keeps the agent reading surface lean without permanently losing design rationale. Top-level `docs/archived/` (not scoped under `docs/atdd/`) is the user's chosen location — keeps the pattern reusable for non-ATDD content that may later need the same treatment. | ✅ Locked   |

## Content-loss budget (computed during refinement)

A pure file move would drop ~700 lines of canonical content not covered by either the new tree OR `process-flow.yaml`. Breakdown:

**~400 lines in three orphan files (no replacement anywhere):**
- `cycles.md` (268 lines) — master decision-flow prose. Items 1–8 below walk through which fragments survive and where.
- `glossary.md` (69 lines) — concept definitions. See D1.
- `placeholders.md` (65 lines) — `${name}` substitution mechanism doc.

**~340 lines of per-phase deltas:**
- OLD per-phase docs total ~495 lines; NEW counterparts total ~155 lines.
- Tracked by [Part 2 — per-phase content (20260518-1116)](20260518-1116-atdd-at-cycle-part2-per-phase-content.md). Not re-litigated here.

### `cycles.md` fragments (Items 1–8) — content NOT in `process-flow.yaml`

| Item | Fragment                                        | Status        |
|------|-------------------------------------------------|---------------|
| 1    | Concept definitions (Behavioral / Structural / Legacy Coverage / Interface Change) — live in `glossary.md` | ✅ Locked per D1 (revised under D2) — split: Interface Change → `process/shared/glossary.md`; other 3 → `archive/glossary.md` |
| 2    | Agent contracts / rules (unit-of-work = ticket; `[^green]`; agents CI-unaware; TEST gated upfront; structural-change checklist in ticket body) | ⏳ Pending |
| 3    | Decision-criteria definitions (what "DSL Interface Changed?" etc. actually evaluate) | ⏳ Pending |
| 4    | Phase-to-Agent table "Notes" column (per-phase WRITE definitions) | ⏳ Pending |
| 5    | External System Onboarding agent guidance (json-server pattern; minimal-interface rule; STOP-and-present) | ⏳ Pending |
| 6    | Scope concept (Architecture / System Lang / Test Lang axes; `${...}` propagation; `--config <path>`; "sub-agents restrict ALL file edits to in-scope paths") | ⏳ Pending |
| 7    | Naming-convention mappings (`subtype:system-implementation-change` → COMMIT suffix `CHORE`; `shop/` vs `shop`) | ⏳ Pending |
| 8    | Driver Adapter Cycle commentary ("driver interfaces may grow, existing AC must keep passing"; system-side path is channel-agnostic — WRITE agent reads Checklist + system tree) | ⏳ Pending |

**Context.** Today there are two parallel process-doc trees in this repo:

- **OLD (embedded):** `internal/assets/global/docs/atdd/process/` — 13 flat files. Embedded via `//go:embed runtime global` (`internal/assets/embed.go:18`). Materialized into `~/.gh-optivem/docs/` (per-user, unsubstituted) and `./.gh-optivem/docs/` (per-project, `${name}`-substituted) by `internal/assets/sync/`. This is what runtime prompts currently `Read ${docs_root}/atdd/process/<name>.md` from.
- **NEW (authoring, top-level):** `docs/atdd/process/` — hierarchical tree (`analysis/`, `change/behavior/`, `change/behavior/ct/`, `change/behavior/shared/`, `change/structure/`, `shared/`). 15 files. Not embedded, not materialized.

Goal: cut-paste the NEW hierarchy into the OLD location so the new tree becomes the embedded SSoT, retire the old flat filenames everywhere they're referenced, and remove the top-level `docs/atdd/process/`. **Embed wiring stays as-is** (per user directive: "I don't want to repoint") — `internal/assets/global/` keeps being the embed root, just with a different file tree under `process/`.

**Sibling / coordinated plans:**

- [ATDD phase-scope SSoT (20260518-1530)](20260518-1530-atdd-phase-scope-ssot.md) — that plan treats `docs/atdd/process/` as the canonical phase-doc location in the tool source. **CONFLICT to reconcile at refinement:** this plan moves the same files *under* `internal/assets/global/`, which contradicts the SSoT plan's "Tool source" / "Scaffolded project" split. Decide before `/execute-plan` whether (a) this plan supersedes the SSoT plan's location decision, (b) the SSoT plan is revised in lockstep, or (c) this plan is itself wrong and the embed should re-point instead. See **Open question 1** below.
- [AT-cycle absorb internal assets (20260516-1701)](20260516-1701-atdd-at-cycle-absorb-internal-assets.md) and [Part 2 — per-phase content (20260518-1116)](20260518-1116-atdd-at-cycle-part2-per-phase-content.md) — predecessors that authored the new hierarchy under `docs/atdd/process/`. This plan is the file-relocation follow-up they implicitly assumed.
- [Deferred — Structure-cycle SSoT alignment](deferred/20260518-1530-structure-cycle-ssot-alignment.md) — references `docs/atdd/process/change/structure/*.md` paths; those references will need rewriting if this plan moves the files.

## File mapping

OLD flat name → NEW hierarchical path (within the NEW tree):

| OLD                                | NEW                                                       |
|---|---|
| `at-red-test.md`                   | `change/behavior/1.1-at-red-test.md`                      |
| `at-red-dsl.md`                    | `change/behavior/1.2-at-red-dsl.md`                       |
| `at-red-system-driver.md`          | `change/behavior/1.3-at-red-system-driver.md`             |
| `at-green-system.md`               | `change/behavior/2-at-green-system.md`                    |
| —                                  | `change/behavior/3-at-refactor.md` *(new addition)*       |
| `ct-red-test.md`                   | `change/behavior/ct/1.1-ct-red-test.md`                   |
| `ct-red-dsl.md`                    | `change/behavior/ct/1.2-ct-red-dsl.md`                    |
| `ct-red-external-driver.md`        | `change/behavior/ct/1.3-ct-red-external-driver.md`        |
| `ct-green-stubs.md`                | `change/behavior/ct/2-ct-green-stubs.md`                  |
| `system-interface-redesign.md`     | `change/structure/1-sir-write.md` *(name change — verify equivalence)* |
| `task-and-chore-cycles.md`         | `change/structure/2-chore-write.md` *(name change — chore-half only; task-half coverage TBD)* |
| —                                  | `analysis/acceptance-criteria-analysis.md` *(new)*        |
| —                                  | `change/behavior/shared/disable-tests.md` *(new)*         |
| —                                  | `change/behavior/shared/enable-tests.md` *(new)*          |
| —                                  | `shared/conventions.md` *(new)*                           |
| `cycles.md`                        | **ORPHAN — no NEW counterpart** (see Open question 2)     |
| `glossary.md`                      | **ORPHAN — no NEW counterpart** (see Open question 3)     |
| `placeholders.md`                  | **ORPHAN — no NEW counterpart** (see Open question 4)     |

## Open questions (must resolve before `/execute-plan`)

1. **SSoT plan reconciliation.** The in-flight [20260518-1530 SSoT plan](20260518-1530-atdd-phase-scope-ssot.md) describes the tool source as `docs/atdd/process/**.md` (top-level, not embedded) and the scaffolded-project surface as `docs/atdd/process/**.md` (materialized into the project's working tree, not `./.gh-optivem/docs/`). This plan instead keeps everything under `internal/assets/global/` and the existing `./.gh-optivem/docs/` materialization mechanism. **Pick one:** (a) this plan wins, SSoT plan revised; (b) SSoT plan wins, this plan needs to retire materialize instead of moving files into it; (c) hybrid (e.g. tool source under `internal/assets/global/`, scaffolded-project surface lands at `docs/atdd/process/` per SSoT). The choice changes the shape of every item below.
2. **`cycles.md` fate.** The current `cycles.md` is a 22k-line master decision flow. The new hierarchy has no top-level overview doc. Options: (a) cut-paste under a new name (e.g. `cycles.md` at the root of the new tree, or `change/cycles.md`); (b) absorb into BPMN / process-flow.yaml and drop; (c) split into `analysis/`, `change/`, `coverage/` overviews. Discuss at refinement.
3. **`glossary.md` fate.** Shared definitions doc, referenced by `clauderun_test.go` test prompts and Part 2 plan item 17 ("Decide migration targets for those supporting docs"). Options: (a) move to `shared/glossary.md`; (b) defer to Part 2 item 17. Discuss at refinement.
4. **`placeholders.md` fate.** Meta doc describing the `${name}` substitution mechanism itself. Currently lives next to the docs it describes. References will need updating if moved. Options: (a) move to `shared/placeholders.md`; (b) move out of `process/` entirely (it's tool meta, not process content) — e.g. to `docs/atdd/architecture/` or a new `docs/atdd/meta/`; (c) absorb into a CLAUDE.md / authoring-guide doc. Discuss at refinement.
5. **`system-interface-redesign.md` ↔ `change/structure/1-sir-write.md` equivalence.** Verify these cover the same material before deletion — title change suggests reframe (SIR as a write-cycle phase, not a standalone redesign cycle).
6. **`task-and-chore-cycles.md` ↔ `change/structure/2-chore-write.md`.** Old doc covered both task and chore cycles; new doc covers chore only. Where does task-cycle content land? May need a new `change/structure/N-task-write.md`.

## Items

### 1. Resolve orphan-file fates (Open questions 2–4)

Walk through `cycles.md`, `glossary.md`, `placeholders.md` at `/refine-plan` time. Pin the destination path for each in this plan before any move happens. After this item, the **File mapping** table above is updated in place to remove the **ORPHAN** rows.

### 2. Verify equivalence for renamed docs (Open questions 5–6)

For `system-interface-redesign.md` → `change/structure/1-sir-write.md` and `task-and-chore-cycles.md` → `change/structure/2-chore-write.md`: read both, confirm the new docs subsume the old content (or note the residual gap and file a follow-up before deletion). Pin a one-line equivalence note in this plan per pair.

### 3. Cut-paste files

Physically move the contents of `docs/atdd/process/**` into `internal/assets/global/docs/atdd/process/**`, preserving the hierarchy. Also delete the OLD flat files that are now superseded (per the **File mapping** table). End state:

- `internal/assets/global/docs/atdd/process/` — contains the new hierarchical tree only (with whatever destination paths Items 1–2 settled for orphans).
- `docs/atdd/process/` — removed entirely (do not leave an empty directory or a `.gitkeep`).

Use `git mv` so history is preserved per-file where rename detection lands.

### 4. Update `process-flow.yaml` phase doc paths

`internal/atdd/runtime/statemachine/process-flow.yaml` has 11 `phase_doc:` entries pointing at OLD flat names (lines 324, 340, 365, 419, 431, 483, 500, 519, 538, 1099 — and any later additions). Rewrite each to its NEW hierarchical path per the **File mapping** table.

These paths are the source for the runtime `phase_doc` parameter that gets injected into prompts — they must match the NEW hierarchy exactly or the `Read ${docs_root}/atdd/process/...` lines in prompts will reference non-existent files post-materialize.

### 5. Update runtime prompt `Read` lines

Each file under `internal/assets/runtime/prompts/atdd/*.md` has a `Read ${docs_root}/atdd/process/<old-name>.md` line near the top. Rewrite each to the NEW path. Files to edit:

- `at-red-test.md:20` → `change/behavior/1.1-at-red-test.md`
- `at-red-dsl.md:13` → `change/behavior/1.2-at-red-dsl.md`
- `at-red-system-driver.md:12` → `change/behavior/1.3-at-red-system-driver.md`
- `at-green-system-backend.md:11` → `change/behavior/2-at-green-system.md`
- `at-green-system-frontend.md:10` → `change/behavior/2-at-green-system.md`
- `ct-red-test.md:12` → `change/behavior/ct/1.1-ct-red-test.md`
- `ct-red-dsl.md:12` → `change/behavior/ct/1.2-ct-red-dsl.md`
- `ct-red-external-driver.md:12` → `change/behavior/ct/1.3-ct-red-external-driver.md`
- `ct-green-stubs.md:8` → `change/behavior/ct/2-ct-green-stubs.md`
- `chore.md:21` → `change/structure/2-chore-write.md`
- `task-system-interface-redesign.md:19` → `change/structure/1-sir-write.md`
- `task-external-system-interface-redesign.md:21` → `change/structure/1-sir-write.md`

### 6. Update tests

Tests carry the OLD paths as string literals; update each to the NEW path:

- `internal/atdd/runtime/clauderun/clauderun_test.go:121,157` — `at-red-test.md` references.
- `internal/atdd/runtime/clauderun/clauderun_test.go:347,402` — `glossary.md` references *(depends on Open question 3 resolution)*.
- `internal/atdd/runtime/statemachine/dispatch_spy_test.go:243–331` — 8 phase_doc references covering AT and CT phases plus SIR.
- `internal/atdd/runtime/driver/driver_test.go:89,208` — `at-red-test.md` references.
- `internal/atdd/runtime/driver/embedded_smoke_test.go:152` — `system-interface-redesign.md` reference.
- `internal/assets/sync/sync_test.go` — check for fixtures naming the OLD flat files (per `Grep` hit at line 103: `~/.gh-optivem/docs/atdd/`).

Run `go test ./internal/assets/sync/... ./internal/atdd/...` (with `-p 2` per `[[feedback_go_test_windows]]`) to confirm.

### 7. Update code comments referencing old paths

- `internal/projectconfig/paths_defaults.go:7` — comment `// in internal/assets/global/docs/atdd/process/placeholders.md` → update to the destination Open question 4 picks.
- `internal/atdd/runtime/clauderun/clauderun.go:54` — comment example `(e.g. "docs/atdd/process/at-red-test.md")` → use the NEW path as the example.

### 8. Update agent docs that glob/reference process docs

- `.claude/agents/atdd/meta/process-audit.md:14–18,107` — currently lists OLD flat filenames (`cycles.md`, `shared-phase-progression.md`, `at-cycle-conventions.md`, `at-*.md`, `ct-cycle-conventions.md`, `ct-*.md`, `glossary.md`). Rewrite to reference the NEW hierarchy (likely via a glob like `internal/assets/global/docs/atdd/process/**/*.md` with a one-paragraph summary of the new shape).
- `.claude/agents/atdd/meta/token-usage-audit.md` — similar; audit and rewrite any explicit OLD-path references.

### 9. Update placeholders.md (if it's the chosen destination per Open question 4)

The "Editing phase docs" section of `placeholders.md` (line 59 in the OLD location) explicitly says "When editing a doc under `internal/assets/global/docs/atdd/process/`". The path is still correct after this plan (embed location unchanged) but the listed editing conventions may need updating to reflect the new hierarchy (e.g. naming conventions for numbered phase files, cross-references between `change/behavior/at` and `change/behavior/ct`).

### 10. Sweep plans / reports for OLD-path references

Living plans and reports that reference OLD flat paths need updating; historical plans (already-executed or deferred-and-frozen) do not. Targets to audit:

- `plans/20260518-1530-atdd-phase-scope-ssot.md` — heavy `docs/atdd/process/...` references; may need rewriting depending on Open question 1 resolution.
- `plans/20260518-1116-atdd-at-cycle-part2-per-phase-content.md` — Phase 5 cross-references reference `docs/atdd/process/` paths; update or note "will be reconciled at execute time."
- `plans/20260518-1742-family-b-stems-and-ct-vocab.md` — `docs/atdd/process/` references.
- `reports/atdd-at-cycle-gap-analysis.md` — gap analysis sources OLD paths; if still active, update.
- `plans/deferred/20260518-1530-structure-cycle-ssot-alignment.md` — references `docs/atdd/process/change/structure/*.md`; rewrite to the NEW (post-move) location.

Items 1–9 above are blocking; this item 10 is mostly bookkeeping but should be done in the same PR so the repo doesn't ship in a half-renamed state.

### 11. Verify build + tests

- `go build ./...`
- `go test ./... -p 2` (per `[[feedback_go_test_windows]]`)
- `gh optivem` smoke: run `gh optivem sync` against a scaffolded test project and confirm `./.gh-optivem/docs/atdd/process/change/behavior/1.1-at-red-test.md` (etc.) appears with substituted `${name}` placeholders.
- Inspect a materialized prompt for one phase and confirm the `Read ${docs_root}/atdd/process/...` line resolves to a file that exists in the materialized tree.

## Hand-off dependencies

- **Item 3 (cut-paste) must follow Items 1–2** so destination paths for orphans and renamed docs are pinned before files move.
- **Items 4–10 must follow Item 3** because they depend on the NEW paths actually existing.
- **Open question 1 (SSoT plan reconciliation) gates everything** — if the answer is "(b) SSoT plan wins," this plan rewrites top-to-bottom or gets scrapped in favor of a retire-materialize plan.

## What this plan does NOT do

- Does NOT change `internal/assets/embed.go` (`//go:embed` directive) — explicit user directive: "I don't want to repoint."
- Does NOT touch `internal/assets/global/docs/atdd/architecture/` — the 6 architecture files stay where they are.
- Does NOT modify the `${name}` substitution mechanism or the `~/.gh-optivem/docs/` / `./.gh-optivem/docs/` materialization contract.
- Does NOT add new content to phase docs. Content changes belong in [Part 2 (20260518-1116)](20260518-1116-atdd-at-cycle-part2-per-phase-content.md). This plan is a pure file relocation + reference rewrite.
