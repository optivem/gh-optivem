---
# Reads a story and rewrites use case markdown: judgement-heavy but small in volume, and the user approves the result. Sonnet at medium effort.
model: sonnet
effort: medium
---
Merge the story below into the project's use cases. The use cases are the single current spec (narrative plus `UC-NNN.M` acceptance criteria); the story is an increment that adds or modifies behaviour. The merge is one-way and one-time: you edit the use cases, never the story. After your edit the user reviews and approves the change, then the acceptance tests are written against the merged `UC-NNN.M` criteria.

## Inputs

### Story Acceptance Criteria (Gherkin)

${acceptance-criteria}

## Steps

1. Find the spec directory: `specDir` in `spec.yaml` at the repo root, else `docs/use-cases`. Read the use case files there (`UC-NNN-<slug>.md`). Read the format reference in `docs/spec-gate/convention.md` (front matter with `id` and `title`; criteria are list lines `- UC-NNN.M: <prose>`, optionally `- UC-NNN.M [flag: name]: <prose>`, grouped under headings). If the directory is empty or absent, there are no use cases yet: the first merge creates it.
2. Decide for each story scenario where it belongs:
   - it **adds** behaviour to an existing use case: add a step or an extension to the narrative, and a new criterion `UC-NNN.M` (next free number in that use case) that states the scenario as prose;
   - it **modifies** a rule an existing criterion states: edit that criterion's text (keep its id) and the narrative step it comes from;
   - it needs a **new use case**: create `UC-NNN-<slug>.md` (next free `UC-NNN`) with front matter, a short narrative (actor, goal, main scenario, extensions) and the criteria;
   - it is **already described** by an existing criterion: change nothing for it and map it to that id.
   A criterion states one path (the main scenario or one extension) as a plain-prose sentence derived from the Gherkin: no Given/When/Then keywords, no test detail.
3. **Contradictions are flagged, not overwritten.** If a scenario conflicts with what a use case already says (the same condition with a different outcome that is not an intended modification), do not edit that criterion. Discuss it with the user. If it cannot be resolved in this conversation, record it in `contradictions` (one entry per conflict, naming the scenario and the `UC-NNN.M` it conflicts with) and stop editing; the run then halts with your list.
4. Run `gh optivem spec validate` and fix every error it reports in the files you edited.
5. Show the user the proposed change (narrative edits and `UC-NNN.M` lines, or the new use case file) and let them adjust it. Do not edit any file outside the spec directory. The story ticket itself is never edited.
6. Emit `scenario-map`: one entry per story scenario, of the form `<scenario title> => UC-NNN.M` (several ids separated by commas when a scenario spans criteria). Use the scenario title exactly as written after `Scenario:`. Every scenario must appear. The orchestrator fails the run on any scenario without a mapped, existing criterion.

If the use cases already contain this story's behaviour (for example from an earlier run of the same story), make no edit: confirm that to the user and emit the `scenario-map` for the existing criteria.

## Outputs

${expected-outputs}

Notes:

- `scenario-map` is checked mechanically against the story's scenarios and the merged use cases.
- `contradictions` is emitted only when you found a conflict you could not resolve with the user.
