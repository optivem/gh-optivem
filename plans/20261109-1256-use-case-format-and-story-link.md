# 2026-11-09 11:56 UTC — Use case format: business rules file, Gherkin criteria, story ↔ criterion link

## TL;DR

**Why:** The articles describe each use case in three parts:
- **Narrative:** what happens, no formulas.
- **Business rules:** the formulas, in a separate file.
- **Gherkin acceptance criteria:** each tagged `@UC-NNN.M @narrative-Xy` so it links back to its narrative step.

Today `use-case-merger.md` writes each criterion "as prose" in the use case. It has no business-rules file and no narrative tags, and nothing records which criterion IDs a story introduced or changed.
**End result:** Merging a story produces three things:
- an updated narrative
- an updated business-rules file
- Gherkin criteria tagged with their ID and narrative step

The ticket records the criterion IDs the story touched.

Parent: `plans/20261109-1219-improved-atdd-process-flow.md` ("Merge Story Into Use Cases" diagram). Coordinate with shop `plans/20261106-1533-use-cases-match-substack-articles.md`, which converts the shop's own use cases to this format.

## Outcomes

- `use-case-merger` writes formulas as BR-n business rules beside the use case (`<specDir>/UC-NNN-<slug>/business-rules.md`), and the narrative references them. ✅
- Criteria are Gherkin scenarios tagged `@UC-NNN.M` and `@narrative-Xy`. IDs are never renumbered or reused.
- A new `LINK_STORY_CRITERIA` service task records the story's touched criterion IDs on the ticket. It reuses the `merged-criteria` that `VERIFY_STORY_MERGE` already stamps.
- `VERIFY_STORY_MERGE` validates the new format (tags present, business-rules references resolve). ✅

## ▶ Next executable step (resume here)

Only Step 3 (`LINK_STORY_CRITERIA`) is left. Steps 1, 2 and 4 were done on 2026-11-09. Step 3 waited because another session had uncommitted edits in `internal/atdd/runtime/tracker/github/github.go`; check `git status` and start once that file is clean. It needs:
- a body-writing method on `tracker.Tracker` (github: `gh issue edit --body-file`; markdown: rewrite the file), plus every fake that implements the interface;
- the new section heading added to `intake.CanonicalHeadings`, so `parse-ticket` still accepts the ticket on a re-run;
- the service task after `VERIFY_STORY_MERGE`, writing `merged-criteria` into that section (replace it if present, so re-runs stay idempotent).

Shop already uses the format this plan teaches (`docs/shared/specs/use-cases/UC-001-place-order/`), so nothing waits on shop plan 20261106-1533.

## Steps

- [ ] Step 3 — ⏳ Deferred: `internal/atdd/runtime/tracker/github/github.go` has another session's uncommitted edits (2026-11-09); do after they land. Add the `LINK_STORY_CRITERIA` service task after `VERIFY_STORY_MERGE` in `merge-story-into-use-cases`. Write the IDs to the ticket through the tracker abstraction, naming it for GitHub + Jira.

## Decisions (accepted by the user 2026-11-09)

- **Criterion IDs go in a ticket body section** (not comments or labels): readable and parseable on both GitHub and Jira, and the body is what `parse-ticket` already reads.
- **Ordering: this plan ships before `20261109-1258-ac-delete-and-generate.md`.** The stable `@UC-NNN.M` IDs this plan introduces are the key the AC-delete plan needs: deleting a green criterion is only safe when its test still carries the ID, so a later "modify" story can re-add a criterion with the same ID and the AT writer edits the tagged test in place (parent Q8).

## Decisions made while executing (2026-11-09)

- **Business rules live with the use case, not in `docs/business-rules/`.** The merger follows the layout shop already uses and the spec parser already reads (commit 7399d6f2): `<specDir>/UC-NNN-<slug>/{use-case-narrative.md, business-rules.md, acceptance-criteria.md}`, or one flat `UC-NNN-<slug>.md` with a `## Business rules` section. So the merger still edits only the spec directory; `merge-use-cases` stays `scope: none` and no scope or placeholder was added (there is no `phase-scopes.yaml`).
- **Step 4: the criterion id format did not change** (`UC-NNN.M`). The acceptance-test writer's traceability section had a wrong tag syntax, fixed to match `docs/spec-gate/convention.md`: .NET `[Trait("tag", …)]` (it said `"Criterion"`, which `internal/spec/junit.go` ignores) and the TypeScript `tag` annotation; it also says not to copy `@narrative-…` tags onto tests.
- **Bullet criteria stay valid.** Existing `- UC-NNN.M: <text>` criteria are left alone; new and edited criteria are Gherkin. `VERIFY_STORY_MERGE` checks narrative tags and `BR-n` references only in the use cases the story touched (`internal/atdd/process/actions/story_merge_format.go`); the spec parser itself was not changed.
