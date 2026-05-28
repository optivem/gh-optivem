# Scope rule

Every ATDD phase declares `read:` and `write:` path sets (on the BPMN
MID, surfaced in the prompt's `## Scope` block). Only read or modify
files under those paths.

If you need to read or write outside scope, **do not** expand silently
and **do not** ask the user inline. Emit the scope-exception envelope:

```
gh optivem output write \
  scope-exception-files=path/to/out-of-scope.go \
  scope-exception-reason="<one-line rationale>"
```

`scope-exception-files` is comma-separated; the downstream
`scope_exception_requested` gate routes accordingly.

## Scope is the complete contract

The `## Scope` block is the complete read/write contract — anything not in `read:` you cannot read, anything not in `write:` you cannot write to. Both escape via the envelope above.
