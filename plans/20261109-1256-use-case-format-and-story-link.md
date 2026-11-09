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

- `use-case-merger` writes formulas to `docs/business-rules/UC-NNN-<slug>.md` (BR-n), and the narrative references them.
- Criteria are Gherkin scenarios tagged `@UC-NNN.M` and `@narrative-Xy`. IDs are never renumbered or reused.
- A new `LINK_STORY_CRITERIA` service task records the story's touched criterion IDs on the ticket. It reuses the `merged-criteria` that `VERIFY_STORY_MERGE` already stamps.
- `VERIFY_STORY_MERGE` validates the new format (tags present, business-rules references resolve).

## ▶ Next executable step (resume here)

Review this plan with the user. Check first whether the shop plan 20261106-1533 has landed, because the merger's reference examples come from shop.

## Steps

- [ ] Step 1: Update `use-case-merger.md` (and `ac-format.md` if it defines the criterion shape) for:
  - the business-rules file
  - Gherkin criteria
  - `@narrative-Xy` tags

  Widen its write scope in `phase-scopes.yaml` to `docs/business-rules/`, using a `${...}` placeholder from gh-optivem.yaml (no hardcoded path).
- [ ] Step 2: Update `VERIFY_STORY_MERGE` / the spec parser so it accepts and validates the Gherkin + tag format.
- [ ] Step 3: Add the `LINK_STORY_CRITERIA` service task after `VERIFY_STORY_MERGE` in `merge-story-into-use-cases`. Write the IDs to the ticket through the tracker abstraction, naming it for GitHub + Jira.
- [ ] Step 4: Update the acceptance-test writer's tag guidance if the criterion ID format changes. Update tests.

## Decisions (proposed 2026-11-09, pending user OK — override any line)

- **Criterion IDs go in a ticket body section** (not comments or labels): readable and parseable on both GitHub and Jira, and the body is what `parse-ticket` already reads.
- **Ordering: this plan ships before `20261109-1258-ac-delete-and-generate.md`.** The stable `@UC-NNN.M` IDs this plan introduces are the key the AC-delete plan needs: deleting a green criterion is only safe when its test still carries the ID, so a later "modify" story can re-add a criterion with the same ID and the AT writer edits the tagged test in place (parent Q8).
