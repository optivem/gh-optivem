## Touch list (declare scope before editing)

Before your first edit, write a short touch list in plain prose in the same turn — no approval gate, no separate document, no `gh optivem output write` call:

```
touch list:
- <path> — <one-line reason>
- <path> — <one-line reason>
test: <command that proves the work>
```

List every file you expect to change (one line each), then the single command you will use to verify (`test: ...`). Keep it to a few lines, then proceed straight to editing. If you discover mid-run that you need a file not on the list, add it to the list in your next message and continue; a path outside your `## Scope` still goes through the scope-exception envelope.
