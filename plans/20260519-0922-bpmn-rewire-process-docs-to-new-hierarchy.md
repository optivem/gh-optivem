# Plan: rewire BPMN runtime + agent prompts to the new `process/` hierarchy

> ⚠️ **BEFORE RE-PICKUP — check prerequisites** (the next agent / future-you reads this first):
> 1. **Plan 1530 item 4 must have committed first.** It edits the YAML frontmatter (`scope:` block) of the same `internal/assets/runtime/prompts/atdd/*.md` files that this plan's Item 3 rewrites the body `Read` lines of. Two agents editing the same files concurrently risk silent collision per `[[feedback_concurrent_agent_collision]]`. Verify with `git log --oneline -10 | grep -i "phase-scope-ssot\|1530"` — there must be a commit landing item 4 (look for "item 4" or "scope frontmatter" in the message).
> 2. **Plan 0911 should have committed first** (authors `internal/assets/global/docs/atdd/process/change/structure/external-system-interface-redesign.md`). Verify with `git log --oneline -10 | grep -i "0911\|ESIR WRITE"`. If 0911 hasn't landed, Items 3 + 6 can still execute, but `process-flow.yaml:1108` (already committed) and the corresponding runtime prompt will forward-reference a missing file until 0911 lands.
> 3. Once prerequisites are clear: `/clear` then `/execute-plan plans/20260519-0922-bpmn-rewire-process-docs-to-new-hierarchy.md`. Auto-detect will see the `⏳ Deferred` markers and run in **Execute approved** mode for Items 3, 6, and Item 7's two remaining sub-bullets.

> ✅ **PARTIAL EXECUTE 2026-05-19** — Items 1, 2, 4, 5 landed in commit `470c67a` (process-flow.yaml + dispatch_spy / clauderun / driver / embedded_smoke test fixtures + comments in gates/bindings.go, actions/bindings.go, clauderun.go, clauderun_test.go). Build + scoped tests green. Items 3 + 6 deferred to avoid file collision with parallel agent on plan 1530 item 4; Item 7's smoke + rendered-prompt sub-bullets also deferred until Item 3 lands.

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

### 3. Rewrite `Read ${docs_root}/atdd/process/...` lines in runtime prompts

> ⏳ **Deferred 2026-05-19:** parallel agent on plan 1530 item 4 was editing the YAML frontmatter (`scope:` block) of these same prompt files at execute time. Wait for that commit to land, re-check `git log` per `[[feedback_concurrent_agent_collision]]`, then execute these edits in a follow-up session.


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

### 6. Final sweep + delete the archive

After Item 3 lands and tests pass, search the repo for any remaining `docs/atdd/process/<old-flat-name>.md` references and burn them down. Then `git rm -r internal/assets/global/docs/atdd/process/_ARCHIVED_PENDING_DELETE/` **in the same PR** — atomic migration, no transient old+new state. Coordinate ordering with sibling plan [20260519-0911](20260519-0911-author-esir-write-phase-doc.md): 0911 should land first (or simultaneously) so the ESIR sibling doc exists before this plan's archive deletion removes the last historical copy of the relevant ESIR prose.

> ⏳ **Deferred 2026-05-19:** blocked on Item 3 (must not delete archived files while the runtime prompts still `Read` from them). Pair with Item 3 in the follow-up session.

### 7. Verify build + tests (smoke + rendered prompt sub-bullets only)

> ⏳ **Deferred 2026-05-19:** `go build ./...` and `go test ./internal/atdd/... ./internal/assets/... -p 2` ran green this session covering Items 1/2/4/5. The two sub-bullets below need Item 3's prompt rewrites in place to be meaningful.

- `gh optivem` smoke: run `gh optivem sync` against a scaffolded test project and confirm `./.gh-optivem/docs/atdd/process/change/behavior/at-red-test.md` (etc.) appears with substituted `${name}` placeholders.
- Pick one phase, inspect the rendered prompt, and confirm the `Read ${docs_root}/atdd/process/change/...` line points at a file that exists in the materialized tree.

## Hand-off dependencies (remaining work)

- **Item 6 (final sweep + archive delete) must follow Item 3** — only safe once nothing live references the archive.
- **Wait for plan 1530 item 4 to commit** before re-picking Item 3 — both edit the same `internal/assets/runtime/prompts/atdd/*.md` files (different sections; the YAML frontmatter `scope:` block vs the body `Read` line). Per `[[feedback_concurrent_agent_collision]]`, re-check `git log` before staging.
- **Coordinate with [20260519-0704](20260519-0704-bpmn-external-system-naming-consistency.md)** before touching `process-flow.yaml` lines 1108 / surrounding ESIR section (Item 2 already landed; relevant only if 0704 introduces fresh edits to the same lines).
- **Coordinate with [20260519-0911](20260519-0911-author-esir-write-phase-doc.md)** — that plan authors `change/structure/external-system-interface-redesign.md`, which this plan's Item 3 forward-references. Land 0911 at or before Item 3's re-pickup; Item 2's already-landed `process-flow.yaml` line 1108 currently forward-references the missing file (acceptable transitional state until 0911 lands).

## What this plan does NOT do

- Does NOT modify the `//go:embed` wiring (`internal/assets/embed.go`) — same directive as [20260518-2236](20260518-2236-migrate-process-docs-hierarchy.md): the embed root stays at `internal/assets/global/`.
- Does NOT touch the `${name}` substitution mechanism.
- Does NOT add new content to phase docs — pure reference rewrite.
- Does NOT decide the fate of `cycles.md` (archived orphan, no live references). Out of scope; revisit if a future doc needs that content.
