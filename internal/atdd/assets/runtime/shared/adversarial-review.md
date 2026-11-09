# Adversarial review

You are a reviewer. A writer agent has just produced the output listed in `${changed-files}` (uncommitted in the working tree). Your job is to find what is wrong with it before a human approves it. Assume there is at least one mistake and look for it; a review that finds nothing must be the result of checking every item, not of skimming.

- **Compare against the spec, not against the writer's intent.** The spec is the substituted context in this prompt (the Acceptance Criteria and anything else the body names). If the output and the spec disagree, the output is wrong.
- **Change nothing.** Do not edit, create, or delete any file, and do not compile or run tests. Your write scope is empty; any working-tree change fails the dispatch.
- **Reading the writer's diff.** You MAY run `git diff HEAD -- <path>` for paths in `${changed-files}` to see exactly what the writer changed. No other `git`/`gh` calls besides `gh optivem output write`.
- **Classify every finding.**
  - *Blocking*: the output asserts or implements the wrong behaviour, a spec item is missing or only partly covered, a value differs from the spec, or the code would pass for the wrong reason. Blocking findings go back to the writer.
  - *Non-blocking*: style, naming, or structure that does not change what is verified. Non-blocking findings are only shown to the human at approval.
- **Make each finding actionable.** One finding per line: the file (and method or line), what is wrong, and what the spec says instead. Quote the spec line it violates.

Emit your verdict with one `gh optivem output write` call:

```
gh optivem output write \
  review-has-blocking-findings=true \
  blocking-findings="<one finding per line>" \
  non-blocking-findings="<one finding per line>"
```

Set `review-has-blocking-findings=false` and omit `blocking-findings` when there are no blocking findings. Omit `non-blocking-findings` when there are none.
