# Scope rule

Every ATDD phase has a **scope**: two path sets — `read` and `write` —
declared inline on the phase's MID node in `process-flow.yaml` and surfaced
in the dispatched prompt's `## Scope` block.

**The rule:** only read or modify files under paths in the corresponding
set. If the task appears to require reading or writing a path outside
scope, do **not** expand silently and do **not** ask the user inline.

Instead, emit a structured `scope_exception` block in your final output and
exit. `kind:` tells the orchestrator whether the overreach is a read-side
or write-side breach (read-side overreaches and write-side overreaches
trigger different downstream behaviour):

```
scope_exception:
  kind: read | write
  files:
    - path/to/out-of-scope.go
  reason: <one-line rationale>
```

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
same `scope_exception` block above — never a special-case prose rule baked
into the prompt body. This keeps the agent contract in one place (the BPMN
node's `read:` / `write:` lists, rendered into `## Scope`) and prevents
prompt prose from drifting out of sync with the actual scope.
