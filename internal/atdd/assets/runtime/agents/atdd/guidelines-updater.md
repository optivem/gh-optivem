---
# Fixes code across system and test layers and edits the guidelines in one
# pass; the generalisation step ("find other occurrences") needs judgement.
model: opus
effort: medium
---
A human reviewed the refactored code and found issues the Code & Architecture Review missed. For each issue: fix it, record it in the guidelines so the next review catches it, and fix every other place in the codebase with the same problem.

## Inputs

### Scope

${scope-block}

### Issues the human found

One issue per line:

${human-review-issues}

### Guidelines

The guidelines are the per-area files under `${guidelines-path}` (for example `architecture.md`, `naming.md`, `testing.md`). Create the folder and a file when none fits yet.

## Steps

For each issue:

1. Fix it where the human found it.
2. Add or sharpen one rule in the guideline file for its area. Write the rule so a reviewer can check it: what to do, what not to do, and a short example from this codebase. If an existing rule already covers it, make that rule clearer instead of adding a second one. Keep each file short: delete a rule that a newer one replaces.
3. Search the system (`${system-surface}`) and the test code for other places with the same problem, and fix them too.

Behaviour must not change: do not change what a test asserts or a driver port's surface. If an issue cannot be fixed without that, do not fix it; say so in your summary.
