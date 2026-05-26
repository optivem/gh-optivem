# Fix parameter-definition substitution dragging

## Origin

Surfaced during execution of plan `20260526-1448-agent-prompt-fixes.md`
Session 3 (Item 4). When a prompt defines a node-param in `## Inputs` /
`### Parameters` and then consumes it in `## Steps`, the same `${name}`
placeholder is used twice — once to *name* the parameter (the definition
line), once to *consume* its value (in steps). Substitution doesn't
distinguish, so the definition line drags too.

## Concrete example — `implement-dsl.md`

Source on disk:
```markdown
- `${touches-system-driver}` — boolean, `true` | `false`. Controls whether...
```

Rendered (with `touches-system-driver: true` from the call-activity params):
```markdown
- `true` — boolean, `true` | `false`. Controls whether...
```

Reads oddly — the bullet's "definition" form has lost the parameter
name and now shows only its value. Same shape appears in any prompt
whose `## Inputs` / `### Parameters` block names a parameter using the
exact same `${...}` placeholder that downstream consumption sites use.

## Pre-existing — not introduced by plan 1448

The dragging existed before Item 4; it just became more visible after
Item 4 made every prompt body explicitly list its inputs under
`## Inputs` / `### Parameters` (Item 6's four-heading skeleton). The
fix is independent of Item 4's scope-block work.

## Candidate fixes

### 1. Drop `${...}` from the definition line, keep at consumption

Definition: `` `touches-system-driver` `` (backticked literal name, no
substitution); consumption: `${touches-system-driver}` (substituted
value, unchanged). The cheapest mechanical fix — definition lines just
name the parameter, downstream steps still substitute live values.

### 2. Add a "current value" annotation

Definition line: `` `touches-system-driver` (current value: `${touches-system-driver}`) — boolean, ... ``
Combines (1) with explicit live-value visibility on the definition line.
The agent sees the literal parameter name AND its current value in one
place.

### 3. Generalise the existing `<!-- if:NAME=VALUE -->` gating mechanism

Already used for `${subtype}` in `clauderun.go`'s Subtype field comment.
Extend the `if:` evaluator to all node-params. Rewrite consumption
sites like `(only when \`${touches-system-driver}\`=true; otherwise skip)`
as `<!-- if:touches-system-driver=true -->...<!-- end-if -->` gates.
Biggest sweep but eliminates the awkward `true=true` strings at the
root.

### 4. Introduce `$${name}` as literal escape in ExpandParams

Industry-standard templating-escape mechanism. Source: `$${name}`
renders to literal `${name}`. Single-dollar form substitutes as today.
Adds one parser branch in `statemachine.ExpandParams`. Invents new
syntax — but it's the standard fix in every other templating system.

## Recommended

Combine **(1) + (2)**. Audit pass:

- Drop `${name}` from definition lines.
- Add `(current value: \`${name}\`)` annotations where the live value
  matters (e.g. `implement-dsl.md`'s `${touches-system-driver}`).

This keeps the substitution machinery flat — no new escape syntax, no
new directive — and the change is mechanical.

## Scope

Audit every prompt under `internal/assets/runtime/prompts/atdd/*.md`
that:
1. Defines a parameter in `## Inputs` / `### Parameters` using
   `\`${name}\``, AND
2. Consumes that same `${name}` downstream (in `## Steps`, `### Phase-
   output flags` table, etc.).

Currently known cases (from plan 1448's audit):
- `implement-dsl.md` — `${touches-system-driver}` (5 occurrences).

Other prompts may carry similar shapes — audit when this plan is
picked up.

## Acceptance

- Every definition line names its parameter literally (backticked, no
  `${...}` wrap).
- Every consumption site continues to use `${name}` for the substituted
  value.
- Rendered prompts no longer carry awkward `\`true\` — boolean, ...`
  lines.
- One end-to-end rehearsal of `implement-dsl` shows the dispatched
  prompt with the literal `touches-system-driver` name visible on the
  definition line.
