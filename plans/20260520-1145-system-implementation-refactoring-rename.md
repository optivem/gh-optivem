# 2026-05-20 11:45 UTC — Rename `chore` → `system-implementation-refactoring` (+ drop `task-` prefix)

**Status:** READY (small dated plan; not yet picked up)

**Origin:** promoted from Items 9 + 4 of `plans/20260519-1537-post-meta-bpmn-topics.md` (both DECIDED, ready for promotion). Item 4's `task-` prefix removal piggybacks on this plan because the surfaces overlap (prompt filenames + `process-flow.yaml` agent references). The *why* (refactoring-not-change framing, symmetry with `system-interface-redesign` / `external-system-interface-redesign`) lives in the source plan; this plan captures the mechanics only.

---

## Decisions

### Decision A — canonical name

`chore` → `system-implementation-refactoring`. Symmetry triple becomes:

- `system-interface-redesign` — interface change, system-side
- `external-system-interface-redesign` — interface change, external-side
- `system-implementation-refactoring` — implementation change, system-side (was `chore`)

### Decision B — drop `task-` prefix from the two prompts that still carry it

- `task-system-interface-redesign.md` → `system-interface-redesign.md`
- `task-external-system-interface-redesign.md` → `external-system-interface-redesign.md`

All other prompts (`at-*`, `ct-*`, `fix-verify`, `disable-tests`, `enable-tests`) are already bare; this drop brings the two stragglers in line.

### Decision C — short form for commit-prefix / phase label

Pick **one** convention rather than perpetuate the `change_type:` overload at `process-flow.yaml:306` (routing name) vs `process-flow.yaml:1234` (commit-prefix label):

- Option 1 (recommended): use the long kebab `system-implementation-refactoring` everywhere; let the commit step shorten if it actually needs to.
- Option 2: keep the short form (e.g., `SYSTEM-IMPL-REFACTOR` or similar) only at the commit-prefix surface and document the mapping.

Decide inside this plan. Recommendation: option 1 — fewer translation surfaces, easier grep.

---

## Sequencing

**Independent.** No architectural dependencies on other in-flight plans:

- Orthogonal to the asset-tree rename (`plans/20260520-1145-runtime-references-tree-rename.md`) — touches prompts under `runtime/prompts/atdd/`, not under `global/docs/`.
- Orthogonal to the AT_GREEN collapse (drafted in parallel by another agent) — touches different `process-flow.yaml` regions.
- Orthogonal to the orphan-promotion plan (`plans/20260520-1109-ac-refinement-and-at-refactor-agent-steps.md`).

Land whenever; ideally before any new feature work that would refer to either name, so the new code uses the canonical form from day one.

---

## Surfaces to touch

### Prompt files

- `internal/assets/runtime/prompts/atdd/chore.md` → `system-implementation-refactoring.md`
- `internal/assets/runtime/prompts/atdd/task-system-interface-redesign.md` → `system-interface-redesign.md`
- `internal/assets/runtime/prompts/atdd/task-external-system-interface-redesign.md` → `external-system-interface-redesign.md`
- Prompt body wording inside `chore.md`: "Chore Agent" → "Implementation Refactoring Agent"; "CHORE - WRITE" phase label → "SYSTEM - IMPLEMENTATION - REFACTORING - WRITE" (or chosen short form per Decision C).

### `process-flow.yaml`

Verify exact line numbers against current HEAD before editing — source plan numbers may have drifted.

- Line ~1199 + ~1208: `agent: task-system-interface-redesign` → `agent: system-interface-redesign`; same for `task-external-…`.
- Line ~1235: `agent: chore` → `agent: system-implementation-refactoring`.
- Line ~1234: `change_type: CHORE` → `change_type: system-implementation-refactoring` (or chosen short form per Decision C).
- Line ~306: routing condition `change_type == system-implementation-change` → `change_type == system-implementation-refactoring`.
- Line ~273: comment / mapping line — update.
- Line ~1236: `phase_doc: docs/atdd/process/change/structure/system-implementation-change.md` — file is gone (Item 5 inlining). Drop the field or repoint per Item 5's prompt-sourcing model. Same review needed for any other `phase_doc:` field touching these renamed prompts.

Run `grep -n 'chore\|task-system-interface\|task-external-system-interface\|system-implementation-change' process-flow.yaml` once to catch anything the line-number list misses.

### Go code

- `internal/steps/github_setup.go:80`: `subtype:system-implementation-change` → `subtype:system-implementation-refactoring`; description `"Structural change to system internals (no test-stack artifact)"` → `"Refactoring of system internals (no boundary or behavioral change)"`.
- `grep -rn 'chore\|system-implementation-change\|task-system-interface\|task-external-system-interface' internal/` — sweep for anything else.

### Documentation

- `docs/process-diagram.md` — references to old names.
- `docs/images/process-diagram-5-run-cycle.svg` — re-export if the diagram source has the old label baked in.
- `plans/deferred/20260518-2236-migrate-process-docs-hierarchy.md` — references the old name; update or note as superseded.
- `grep -rni 'system-implementation-change\|chore agent' docs/` — sweep.

### GitHub labels (migration concern)

Existing GitHub issues are labelled `subtype:system-implementation-change`. Options:

1. Recognise both labels during a transition window (Go code accepts either; new issues only get the new label).
2. Bulk-migrate via `gh issue list --label subtype:system-implementation-change --json number | jq …` then `gh issue edit … --add-label … --remove-label …`.
3. Leave old issues untouched; only new issues get the new label.

Decide inside this plan. Recommendation: option 2 if the issue count is small (single digits); option 3 otherwise.

---

## Out of scope (explicitly)

- **CI consistency walk** (Item 4 residual — a test that walks `process-flow.yaml` and asserts every `agent:` / `phase_doc:` resolves to an existing file). Not promoted here. Pick up when someone hits another dangling-reference bug.
- Any rename of the *other* two siblings in the triple (`system-interface-redesign`, `external-system-interface-redesign`) beyond dropping the `task-` prefix.
- Restructuring `change_type:` semantics beyond Decision C.

---

## Done when

- All file renames landed; `grep -rn 'chore\.md\|task-system-interface\|task-external-system-interface\|system-implementation-change' .` returns no stale references (or only intentional historical references, e.g., changelog entries, this plan, the source plan).
- `go test ./...` (with `-p 2` per memory) passes.
- One acceptance run end-to-end through a `system-implementation-refactoring`-typed ticket confirms routing + commit prefix work.
- GitHub label migration executed (per Decision C choice) or explicitly deferred with a note.
- A short changelog entry about the renames if the project keeps one.
