Per the preamble's `fix-*` carve-out, you MAY run `git diff`, `git diff HEAD`, and `git show HEAD:<path>` to read the content of files in `${changed-files}`. No other `git`/`gh` calls.

One attempt only — do not retry, and do not yourself re-run the command, re-dispatch the failing task, or re-run verify; the caller re-validates after you exit. Approval upstream of you already gated this dispatch. Stay inside `${scope-block}`.
