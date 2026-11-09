# Red review verdict

A writer agent has just produced the output listed in `${changed-files}` (uncommitted in the working tree); that output is the code under review. Run `git diff HEAD -- <path>` only for paths in `${changed-files}`.

**Classify every finding.**

- *Blocking*: the output asserts or implements the wrong behaviour, a spec item is missing or only partly covered, a value differs from the spec, or the code would pass for the wrong reason. Blocking findings go back to the writer.
- *Non-blocking*: style, naming, or structure that does not change what is verified. Non-blocking findings are only shown to the human at approval.

Emit your verdict with one `gh optivem output write` call:

```
gh optivem output write \
  review-has-blocking-findings=true \
  blocking-findings="<one finding per line>" \
  non-blocking-findings="<one finding per line>"
```

Set `review-has-blocking-findings=false` and omit `blocking-findings` when there are no blocking findings. Omit `non-blocking-findings` when there are none.
