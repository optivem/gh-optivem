---
# Reads a story and rewrites use case markdown: judgement-heavy but small in volume, and the result is reviewed (approved by the user unless the run is `--auto`). Sonnet at medium effort.
model: sonnet
effort: medium
---
Merge the story below into the project's use cases. The use cases are the single current spec (narrative, business rules and `UC-NNN.M` acceptance criteria); the story is an increment that adds or modifies behaviour. The merge is one-way and one-time: you edit the use cases, never the story. After your edit the change is reviewed (by the user, unless the run is `--auto`), then the acceptance tests are written against the merged `UC-NNN.M` criteria.

## Inputs

### Story Acceptance Criteria (Gherkin)

${acceptance-criteria}

## Steps

1. Find the spec directory: `specDir` in `spec.yaml` at the repo root, else `docs/use-cases`. Read the use cases there and the format reference `docs/spec-gate/convention.md` if the repo has it. If the directory is empty or absent, there are no use cases yet: the first merge creates it. A use case is in one of two layouts; follow the one the repo already uses, and use the folder layout when there are no use cases yet:
   - **Folder per use case:** `UC-NNN-<slug>/` holding `use-case-narrative.md` (the narrative, no front matter), `business-rules.md` (the business rules, no front matter) and `acceptance-criteria.md` (front matter with `id` and `title`, then the criteria). Only `acceptance-criteria.md` is parsed by `gh optivem spec`.
   - **Flat:** one `UC-NNN-<slug>.md` holding the front matter, the narrative, a `## Business rules` section and the criteria.
2. Keep the three parts of a use case apart:
   - **Narrative:** primary actor, a numbered main success scenario and extensions (`- 4a. <condition>:` with the outcome indented under it). It says what happens and contains no formulas: a step that calculates something references its rules, e.g. `([BR-1 to BR-4](business-rules.md))`.
   - **Business rules:** one line per rule, `- BR-n. <rule or formula>` (for example `- BR-1. Base price = unit price x quantity.`). Every formula lives here. Numbers run per use case; a new rule takes the next free number, and a number is never renumbered or reused.
   - **Acceptance criteria:** Gherkin scenarios in a fenced ```` ```gherkin ```` block under a `Feature: <use case title>` line. Each scenario carries exactly one `@UC-NNN.M` tag; several scenarios may share an id (together they are one criterion). A scenario that exercises an extension also carries `@narrative-<extension>` (e.g. `@narrative-4a`); a main-scenario one may carry `@narrative-<step>` (e.g. `@narrative-7`). Every `@narrative-…` tag must name a step or extension the narrative has, and every `BR-n` you mention must be defined in the business rules. A criterion id is never renumbered or reused: a new criterion takes one above the highest id in the use case, and a retired criterion's id stays unused. Existing criteria written as bullet lines (`- UC-NNN.M: <text>`) stay as they are unless the story changes them.
3. Decide for each story scenario where it belongs:
   - it **adds** behaviour to an existing use case: add a step or an extension to the narrative, any new formula as a business rule, and a new tagged scenario (with a new `UC-NNN.M`) adapted from the story's scenario;
   - it **modifies** a rule an existing criterion states: edit that criterion's scenario (keep its `@UC-NNN.M` tag), the narrative step it comes from and, when a formula changes, the business rule (keep its `BR-n`);
   - it needs a **new use case**: create it (next free `UC-NNN`) with the narrative, the business rules (if it has any) and the criteria;
   - it is **already described** by an existing criterion: change nothing for it and map it to that id.
   A criterion scenario describes the behaviour at use-case level: keep its Given/When/Then in the domain's language and leave out test detail (channels, selectors, test data setup mechanics).
4. **Contradictions are flagged, not overwritten.** If a scenario conflicts with what a use case already says (the same condition with a different outcome that is not an intended modification), do not edit that criterion. Discuss it with the user. If it cannot be resolved in this conversation, record it in `contradictions` (one entry per conflict, naming the scenario and the `UC-NNN.M` it conflicts with) and stop editing; the run then halts with your list.
5. Run `gh optivem spec validate` and fix every error it reports in the files you edited.
6. Show the user the proposed change (narrative, business rule and criteria edits, or the new use case) and let them adjust it. Do not edit any file outside the spec directory. The story ticket itself is never edited.
7. Emit `scenario-map`: one entry per story scenario, of the form `<scenario title> => UC-NNN.M` (several ids separated by commas when a scenario spans criteria). Use the scenario title exactly as written after `Scenario:`. Every scenario must appear. The orchestrator fails the run on any scenario without a mapped, existing criterion.

If the use cases already contain this story's behaviour (for example from an earlier run of the same story), make no edit: confirm that to the user and emit the `scenario-map` for the existing criteria.

## Outputs

${expected-outputs}

Notes:

- `scenario-map` is checked mechanically against the story's scenarios and the merged use cases, and so are the `@narrative-…` tags and `BR-n` references in the use cases you touched.
- `contradictions` is emitted only when you found a conflict you could not resolve with the user.
