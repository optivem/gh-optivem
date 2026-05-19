# Plan: rewire BPMN runtime + agent prompts to the new `process/` hierarchy

> 🤖 **Picked up by agent (refine)** — `Valentina_Desk` at `2026-05-19T07:38:25Z`

> ⚠️ **NOT YET REFINED** — run `/refine-plan` on this file before `/execute-plan`. Five orphan / dangling-reference rows (see **Open questions**) need decisions before any edits land.

**Date:** 2026-05-19

## Context

The new hierarchical `internal/assets/global/docs/atdd/process/` tree is in place (`analysis/`, `change/behavior/`, `change/structure/`, `shared/`). The old flat files have been moved into `_ARCHIVED_PENDING_DELETE/`. Everything that drives BPMN-style orchestration — `internal/atdd/runtime/statemachine/process-flow.yaml`, the runtime prompts under `internal/assets/runtime/prompts/atdd/*.md`, the dispatch-spy / clauderun / driver tests, and a few code comments — still names the old flat paths. After-archive, those references now resolve to either `_ARCHIVED_PENDING_DELETE/*` (if a reader walks the archive) or simply nothing.

This plan rewires the BPMN side to the new locations and surfaces the orphan files for which there is no obvious new home.

**Sibling / coordinated plans:**

- [Migrate process docs hierarchy (20260518-2236)](20260518-2236-migrate-process-docs-hierarchy.md) — earlier plan that assumed the new layout would use numbered prefixes (`1.1-at-red-test.md` etc.) and lived under a *cut-paste from `docs/atdd/process/`* model. **Superseded in two ways by this plan:** (a) the actual new files have no numeric prefixes; (b) the move has already happened and the old files are in `_ARCHIVED_PENDING_DELETE/` rather than deleted, so the "delete old files" step is already done up to a final rm. This plan replaces the *reference-rewrite* portion (its Items 4–7) with the correct destination filenames.
- [ATDD BPMN orchestration (20260518-1144)](20260518-1144-atdd-bpmn-orchestration.md) — defines the BPMN runtime layout; only relevant here as a pointer to where the references live (process-flow.yaml + prompts).
- [BPMN external-system naming consistency (20260519-0704)](20260519-0704-bpmn-external-system-naming-consistency.md) — touches the same `process-flow.yaml` file; coordinate ordering at execute time so we don't both rewrite the `external-system-interface-redesign.md` line.
- [Author ESIR WRITE phase doc (20260519-0911)](20260519-0911-author-esir-write-phase-doc.md) — spun out of Q1's resolution; authors `change/structure/external-system-interface-redesign.md`, which this plan's Items 2 & 3 forward-reference. **Must land at or before this plan's execute** so the references resolve to an existing file.

## Proposed mapping (verify before `/execute-plan`)

### A. Clean 1:1 mappings (9 files)

Every reference below points at a file that already exists in the new tree at exactly the path shown.

| Old path (now archived)                               | New path (verified to exist)                              |
|-------------------------------------------------------|-----------------------------------------------------------|
| `docs/atdd/process/at-red-test.md`                    | `docs/atdd/process/change/behavior/at-red-test.md`        |
| `docs/atdd/process/at-red-dsl.md`                     | `docs/atdd/process/change/behavior/at-red-dsl.md`         |
| `docs/atdd/process/at-red-system-driver.md`           | `docs/atdd/process/change/behavior/at-red-system-driver.md` |
| `docs/atdd/process/at-green-system.md`                | `docs/atdd/process/change/behavior/at-green-system.md`    |
| `docs/atdd/process/ct-red-test.md`                    | `docs/atdd/process/change/behavior/ct-red-test.md`        |
| `docs/atdd/process/ct-red-dsl.md`                     | `docs/atdd/process/change/behavior/ct-red-dsl.md`         |
| `docs/atdd/process/ct-red-external-driver.md`         | `docs/atdd/process/change/behavior/ct-red-external-driver.md` |
| `docs/atdd/process/ct-green-stubs.md`                 | `docs/atdd/process/change/behavior/ct-green-stubs.md`     |
| `docs/atdd/process/system-interface-redesign.md`      | `docs/atdd/process/change/structure/system-interface-redesign.md` |

### B. Pre-existing dangling references (already broken; not caused by the archive move)

These references point at files that **were never present** under the OLD flat layout either — they are stale references that survived earlier renames or were stubbed in anticipation of files that never landed.

| Live reference                                            | File never existed at OLD path? | Candidate NEW path                                                       |
|-----------------------------------------------------------|---------------------------------|--------------------------------------------------------------------------|
| `process-flow.yaml:1108` → `docs/atdd/process/external-system-interface-redesign.md` | yes (no archived copy) | `docs/atdd/process/change/structure/external-system-interface-redesign.md` — file authored by sibling plan [20260519-0911](20260519-0911-author-esir-write-phase-doc.md) |
| `process-flow.yaml:1135` → `docs/atdd/process/chore.md`   | yes (no archived copy)          | `docs/atdd/process/change/structure/system-implementation-change.md` (confirmed — new doc is titled `# CHORE - WRITE` and carries the 3-step chore contract) |

### C. References to files that only ever existed as archived orphans

| Live reference                                            | Old archived file        | Has a new counterpart? |
|-----------------------------------------------------------|--------------------------|------------------------|
| `internal/assets/runtime/prompts/atdd/chore.md:21` → `${docs_root}/atdd/process/task-and-chore-cycles.md` | `_ARCHIVED_PENDING_DELETE/task-and-chore-cycles.md` | **Yes** — drop-in replace with `change/structure/system-implementation-change.md` (the new CHORE - WRITE contract). No task-half is needed in this prompt — it's the chore-specific agent, not a shared task-and-chore prompt. |
| `internal/atdd/runtime/clauderun/clauderun_test.go:347,402` → `${docs_root}/atdd/process/glossary.md` | `_ARCHIVED_PENDING_DELETE/glossary.md` | **Yes** — rewrite both `PromptOverride` strings to read `shared/conventions.md` (a doc that exists in the new tree). The tests assert on the rendered prompt string only, not on reading any file — any real path works. |
| `internal/projectconfig/paths_defaults.go:7` comment → `internal/assets/global/docs/atdd/process/placeholders.md` | `_ARCHIVED_PENDING_DELETE/placeholders.md` | **Yes** — rewrite the comment to cite `internal/atdd/phase-scopes.yaml` as the canonical-keys doctrine source. Inline seven-key list stays. |

## Open questions (must resolve at `/refine-plan` time, one at a time per `[[feedback_open_questions_one_at_a_time]]`)

1. **`external-system-interface-redesign.md`** (process-flow.yaml:1108). The new `change/structure/system-interface-redesign.md` is a single doc; the BPMN flow has two distinct call_activity nodes (SIR + ESIR) that historically pointed at two separate phase docs (only one of which was ever authored). Pick: (a) point both at `change/structure/system-interface-redesign.md` and let the prompt context (`change_type: "EXTERNAL SYSTEM INTERFACE REDESIGN"` vs `"SYSTEM INTERFACE REDESIGN"`) carry the variant; (b) author an `external-system-interface-redesign.md` sibling under `change/structure/`; (c) coordinate with [20260519-0704](20260519-0704-bpmn-external-system-naming-consistency.md) which already covers ESIR naming.

   > **Refined 2026-05-19:** Resolved as (b), spun into sibling plan [20260519-0911](20260519-0911-author-esir-write-phase-doc.md). **Why:** SIR and ESIR are genuinely different processes — same WRITE *shape* (driver interface + adapter) but different *targets* (`${driver_*}` vs `${external_driver_*}`), different contract-test fallout (none vs CT updates required), and different artifact patterns (channel-shaped DTOs vs Real/Stub Driver + BaseXyzClient + `Ext*` DTOs). The archived `cycles.md:50, 204-208, 258` and `task-and-chore-cycles.md:17-21` claim ESIR has *no standalone WRITE* — that claim is stale and contradicted by the BPMN (`process-flow.yaml:1102-1118` runs ESIR through `structural_cycle` WRITE → REVIEW → TEST → COMMIT → DA_END, not through CT). The BPMN is the live source of truth; the archived doctrine self-resolves when the archive is deleted (Item 6). Authoring the new doc broadens scope past *pure reference rewrite* and per `[[feedback_new_plan_not_extend]]` belongs in a fresh plan — hence sibling plan 0911. This plan's Items 2 & 3 forward-reference the destination path; the sibling plan creates the file.
2. **`chore.md`** (process-flow.yaml:1135 `phase_doc`). The new `change/structure/system-implementation-change.md` looks like the right destination — confirm it covers the chore agent's WRITE-phase contract before pointing the YAML at it.

   > **Refined 2026-05-19:** Confirmed. Point `process-flow.yaml:1135` at `docs/atdd/process/change/structure/system-implementation-change.md`. **Why:** the new doc is titled `# CHORE - WRITE` and carries the 3-step chore contract verbatim (implement the change inside `system/`; drivers untouched; tests/DSL/Gherkin untouched, STOP-and-reclassify if not). It IS the chore phase doc, just renamed for the new tree.
3. **`task-and-chore-cycles.md`** (prompts/atdd/chore.md:21 `Read` line). The new tree has no `task-and-chore-cycles.md` equivalent. Pick: (a) point the prompt at `change/structure/system-implementation-change.md` (assumes chore-half coverage); (b) split into two reads (chore-half → `change/structure/...`, task-half → wherever task content landed); (c) drop the `Read` line if the new structure doc is now self-sufficient.

   > **Refined 2026-05-19:** Resolved as (a) — drop-in replace with `change/structure/system-implementation-change.md`. **Why:** the chore prompt's preamble says *"Follow the CHORE - WRITE phase referenced below"* and the new doc is literally titled `# CHORE - WRITE` with the 3-step contract. (b) doesn't apply (this is the chore-specific agent, not a shared task-and-chore prompt — no task-half to split out); (c) would break the prompt's design by removing the phase-mechanics contract.
4. **`glossary.md`** (clauderun_test.go:347,402 `PromptOverride`). These two test cases inject a synthetic prompt to exercise the materialize-and-read pipeline; they don't validate glossary content. Pick: (a) rewrite both to read `shared/conventions.md` (a doc that exists in the new tree); (b) replace the prompt with one that references a guaranteed-present synthetic fixture file checked in alongside the test; (c) delete these test cases if a sibling test already covers the same code path.

   > **Refined 2026-05-19:** Resolved as (a) — rewrite both `PromptOverride` strings to `"Read ${docs_root}/atdd/process/shared/conventions.md."`. **Why:** the tests assert on the rendered prompt string only (substitution behavior), not on reading any file. Pointing at an existing doc costs nothing and gives a real reference any future reader can `cat`. (b) adds a fixture file for no gain; (c) is risky without verifying equivalent coverage exists.
5. **`placeholders.md`** comment (paths_defaults.go:7). The seven-key doctrine is no longer documented in any markdown — it now lives in `internal/atdd/phase-scopes.yaml` + `CanonicalPathKeys`. Pick: (a) point the comment at `phase-scopes.yaml`; (b) drop the citation and let the inline list speak for itself; (c) restore a slim `placeholders.md` (or equivalent) under `shared/` to host the doctrine prose.

   > **Refined 2026-05-19:** Resolved as (a) — rewrite the comment to cite `internal/atdd/phase-scopes.yaml` as the canonical-keys doctrine source. Inline seven-key list stays. **Why:** the doctrine question is *which keys are canonical* — settled by `phase-scopes.yaml`. `CanonicalPathKeys` is in the same package so pointing at it is redundant. (b) loses the cross-link to the authoritative source; (c) reintroduces markdown duplication of YAML.

## Items

### 1. Resolve open questions 1–5

Walk through each at `/refine-plan` time and pin a destination per row. The mapping tables above are updated in place to remove the "candidate / see Open question N" footnotes, after which the **Items** below can execute mechanically.

### 2. Rewrite `phase_doc:` entries in `process-flow.yaml`

`internal/atdd/runtime/statemachine/process-flow.yaml` — 12 lines to rewrite (per **Mapping A** + Open questions 1–2 outcomes):

| Line | Old value | New value (post-refinement) |
|---|---|---|
| 324  | `docs/atdd/process/at-red-test.md`                    | `docs/atdd/process/change/behavior/at-red-test.md` |
| 340  | `docs/atdd/process/at-red-dsl.md`                     | `docs/atdd/process/change/behavior/at-red-dsl.md` |
| 365  | `docs/atdd/process/at-red-system-driver.md`           | `docs/atdd/process/change/behavior/at-red-system-driver.md` |
| 419  | `docs/atdd/process/at-green-system.md`                | `docs/atdd/process/change/behavior/at-green-system.md` |
| 431  | `docs/atdd/process/at-green-system.md`                | `docs/atdd/process/change/behavior/at-green-system.md` |
| 483  | `docs/atdd/process/ct-red-test.md`                    | `docs/atdd/process/change/behavior/ct-red-test.md` |
| 500  | `docs/atdd/process/ct-red-dsl.md`                     | `docs/atdd/process/change/behavior/ct-red-dsl.md` |
| 519  | `docs/atdd/process/ct-red-external-driver.md`         | `docs/atdd/process/change/behavior/ct-red-external-driver.md` |
| 538  | `docs/atdd/process/ct-green-stubs.md`                 | `docs/atdd/process/change/behavior/ct-green-stubs.md` |
| 1099 | `docs/atdd/process/system-interface-redesign.md`      | `docs/atdd/process/change/structure/system-interface-redesign.md` |
| 1108 | `docs/atdd/process/external-system-interface-redesign.md` | `docs/atdd/process/change/structure/external-system-interface-redesign.md` — forward-reference; file authored by sibling plan [20260519-0911](20260519-0911-author-esir-write-phase-doc.md) |
| 1135 | `docs/atdd/process/chore.md`                          | `docs/atdd/process/change/structure/system-implementation-change.md` |

### 3. Rewrite `Read ${docs_root}/atdd/process/...` lines in runtime prompts

`internal/assets/runtime/prompts/atdd/*.md` — 12 lines to rewrite:

| File:line | Old `Read` target | New target |
|---|---|---|
| `at-red-test.md:20`                       | `at-red-test.md`                  | `change/behavior/at-red-test.md` |
| `at-red-dsl.md:13`                        | `at-red-dsl.md`                   | `change/behavior/at-red-dsl.md` |
| `at-red-system-driver.md:12`              | `at-red-system-driver.md`         | `change/behavior/at-red-system-driver.md` |
| `at-green-system-backend.md:11`           | `at-green-system.md`              | `change/behavior/at-green-system.md` |
| `at-green-system-frontend.md:10`          | `at-green-system.md`              | `change/behavior/at-green-system.md` |
| `ct-red-test.md:12`                       | `ct-red-test.md`                  | `change/behavior/ct-red-test.md` |
| `ct-red-dsl.md:12`                        | `ct-red-dsl.md`                   | `change/behavior/ct-red-dsl.md` |
| `ct-red-external-driver.md:12`            | `ct-red-external-driver.md`       | `change/behavior/ct-red-external-driver.md` |
| `ct-green-stubs.md:8`                     | `ct-green-stubs.md`               | `change/behavior/ct-green-stubs.md` |
| `task-system-interface-redesign.md:19`    | `system-interface-redesign.md`    | `change/structure/system-interface-redesign.md` |
| `task-external-system-interface-redesign.md:21` | `system-interface-redesign.md` | `change/structure/external-system-interface-redesign.md` — forward-reference; file authored by sibling plan [20260519-0911](20260519-0911-author-esir-write-phase-doc.md) |
| `chore.md:21`                             | `task-and-chore-cycles.md`        | `change/structure/system-implementation-change.md` |

### 4. Update test fixtures carrying old paths as string literals

- `internal/atdd/runtime/statemachine/dispatch_spy_test.go:243,254,266,278,289,300,312,331` — 8 `phase_doc` literals; rewrite each per Item 2's mapping.
- `internal/atdd/runtime/clauderun/clauderun_test.go:121,157` — `at-red-test.md` references; rewrite to `change/behavior/at-red-test.md`.
- `internal/atdd/runtime/clauderun/clauderun_test.go:347,402` — `glossary.md` references; rewrite both `PromptOverride` strings to `"Read ${docs_root}/atdd/process/shared/conventions.md."` (per Q4 resolution).
- `internal/atdd/runtime/driver/driver_test.go:89,208` — `at-red-test.md` references; rewrite to `change/behavior/at-red-test.md`.
- `internal/atdd/runtime/driver/embedded_smoke_test.go:152` — `system-interface-redesign.md` reference; rewrite to `change/structure/system-interface-redesign.md`.
- `internal/atdd/runtime/driver/driver_test.go:428,445,470,486` — `sysui-redesign.md` (already a synthetic string, not a real doc); verify whether it should be touched. Likely **no change** since this looks like a deliberate synthetic fixture, but flag for confirmation.

### 5. Update code comments referencing old paths

- `internal/atdd/runtime/gates/bindings.go:2` — `docs/atdd/process/process-flow.yaml` → this is an *internal* yaml path (`internal/atdd/runtime/statemachine/process-flow.yaml`), not a process doc. **Confirm whether to leave alone or correct in passing.**
- `internal/atdd/runtime/actions/bindings.go:2` — same as above.
- `internal/atdd/runtime/clauderun/clauderun.go:54` — example `"docs/atdd/process/at-red-test.md"` → update to `"docs/atdd/process/change/behavior/at-red-test.md"`.
- `internal/atdd/runtime/driver/embedded_smoke_test.go:3,33` — comments that name `docs/atdd/process/process-flow.yaml`; same yaml-vs-doc confusion as `bindings.go` above.
- `internal/atdd/runtime/clauderun/clauderun_test.go:313` — comment "docs/atdd/process/*.md corpus"; update glob to reflect new tree shape.
- `internal/projectconfig/paths_defaults.go:7` — `placeholders.md` reference; rewrite the comment to cite `internal/atdd/phase-scopes.yaml` as the canonical-keys doctrine source (per Q5 resolution). Inline list of seven keys stays.

### 6. Final sweep + delete the archive

After Items 2–5 land and tests pass, search the repo for any remaining `docs/atdd/process/<old-flat-name>.md` references and burn them down. Then `git rm -r internal/assets/global/docs/atdd/process/_ARCHIVED_PENDING_DELETE/` in the same PR (or a follow-up — user's call at refine time).

### 7. Verify build + tests

- `go build ./...`
- `go test ./internal/atdd/... ./internal/assets/... -p 2` (per `[[feedback_go_test_windows]]`)
- `gh optivem` smoke: run `gh optivem sync` against a scaffolded test project and confirm `./.gh-optivem/docs/atdd/process/change/behavior/at-red-test.md` (etc.) appears with substituted `${name}` placeholders.
- Pick one phase, inspect the rendered prompt, and confirm the `Read ${docs_root}/atdd/process/change/...` line points at a file that exists in the materialized tree.

## Hand-off dependencies

- **Item 1 gates everything** — the five open questions choose the destination paths Items 2–5 write.
- **Items 2–5 can run in parallel** once Item 1 is resolved (independent files).
- **Item 6 (final sweep + archive delete) must follow Items 2–5** — only safe once nothing live references the archive.
- **Coordinate with [20260519-0704](20260519-0704-bpmn-external-system-naming-consistency.md)** before touching `process-flow.yaml` lines 1108 / surrounding ESIR section.
- **Coordinate with [20260519-0911](20260519-0911-author-esir-write-phase-doc.md)** — that plan authors `change/structure/external-system-interface-redesign.md`, which this plan's Items 2 & 3 forward-reference. Land 0911 at or before 0922's execute; if 0922 lands first, ESIR runtime references resolve to a missing file until 0911 lands (acceptable transitional state, ideally avoided).

## What this plan does NOT do

- Does NOT modify the `//go:embed` wiring (`internal/assets/embed.go`) — same directive as [20260518-2236](20260518-2236-migrate-process-docs-hierarchy.md): the embed root stays at `internal/assets/global/`.
- Does NOT touch the `${name}` substitution mechanism.
- Does NOT add new content to phase docs — pure reference rewrite.
- Does NOT decide the fate of `cycles.md` (archived orphan, no live references). Out of scope; revisit if a future doc needs that content.
