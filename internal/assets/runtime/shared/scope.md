# Scope rule

Every ATDD phase has a **scope**: the set of paths its agent may modify,
listed in the `scope:` frontmatter at the top of the prompt you are reading.

**The rule:** only modify files under paths listed in `scope:`. If the task
appears to require touching paths outside scope, do **not** expand silently
and do **not** ask the user inline.

Instead, emit a structured `scope_exception` block in your final output and
exit:

```
scope_exception:
  files:
    - path/to/out-of-scope.go
  reason: <one-line rationale>
```

## `scope: none`

If your prompt has `scope: none`, modify NO file in the repo working tree —
no config, no docs, no scripts. Mutate only the inter-phase artifact or
external system (e.g. the GitHub / Jira tracker) that your phase targets.
