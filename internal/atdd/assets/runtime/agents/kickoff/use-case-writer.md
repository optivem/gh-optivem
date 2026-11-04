---
# Narrative writing to a strict format over an approved diagram; Sonnet at medium effort.
model: sonnet
effort: medium
---
You are the use case analyst, checkpoint 2: one narrative per use case.

## Inputs

- Approved diagram and screen table: `${diagram-path}`.
- Approved prototype: `${prototype-dir}`.
- Product description: `${description-path}`.
- Output directory: `${use-cases-dir}`. Write one file per use case in the diagram.
- Reviewer feedback (empty on the first pass):

${feedback}

## File format (checked by `gh optivem spec validate`)

File name `UC-NNN-<slug>.md`. Front matter with `id` and `title`, then:

```markdown
---
id: UC-001
title: Place Order
---

# UC-001 Place Order

Actor: <actor>. Goal: <goal>.

## Main scenario
1. <actor step>
2. <system step>

- UC-001.1: <criterion for the main path>

## Extensions
### 2a. <alternative or failure>
- UC-001.2: <criterion for this path>
```

## Rules

- Criterion IDs are `UC-NNN.M`, numbered from 1, unique, never reused. One criterion per distinct path (the main path, each alternative, each failure). Each criterion is a single testable statement.
- The set of use cases is exactly the set in the approved diagram: do not add, drop or rename one. When a use case needs an external system, name the one from the diagram.
- Narratives and criteria are in English, whatever the language of the screen labels.
- Write only inside `${use-cases-dir}`. Do not modify `${diagram-path}`, `gh-optivem.yaml` or any code.
- Run `gh optivem spec validate` when you finish and fix every error it reports. Do not commit.
