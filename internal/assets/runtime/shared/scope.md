# Scope rule

Every ATDD phase has a **scope**: two path sets — `read` and `write` —
declared inline on the phase's MID node in `process-flow.yaml` and surfaced
in the dispatched prompt's `## Scope` block.

**The rule:** only read or modify files under paths in the corresponding
set. If the task appears to require reading or writing a path outside
scope, do **not** expand silently and do **not** ask the user inline.

Instead, emit the scope-exception envelope via the structured output
channel — call `gh optivem output write` from your `Bash` tool with
the `scope-exception-*` keys declared on the MID:

```
gh optivem output write \
  scope-exception-files=path/to/out-of-scope.go \
  scope-exception-reason="<one-line rationale>"
```

`scope-exception-files` is a comma-separated list (the CLI splits it
into a list at the dispatcher boundary). The downstream
`scope_exception_requested` gate reads `scope-exception-files` from
`ctx.State` and routes to the appropriate handler.

## `scope: none`

If your phase's MID node declares `scope: none`, modify NO file in the repo
working tree — no config, no docs, no scripts. Mutate only the inter-phase
artifact or external system (e.g. the GitHub / Jira tracker) that your
phase targets.

## Scope is the complete contract

The `## Scope` block in your dispatched prompt is the *complete* read/write
contract. The prompt body does **not** enumerate forbidden layers in prose:
no "do not modify acceptance tests / DSL / driver port", no "frozen layer"
lists, no inline "stop and ask the user" guardrails for specific paths.

If a layer is not in `## Scope` `read`, you cannot read it. If it is not in
`## Scope` `write`, you cannot write to it. The escape hatch for both is the
same `gh optivem output write scope-exception-*` call above — never a
special-case prose rule baked into the prompt body. This keeps the agent
contract in one place (the BPMN node's `read:` / `write:` lists, rendered
into `## Scope`) and prevents prompt prose from drifting out of sync with
the actual scope.
