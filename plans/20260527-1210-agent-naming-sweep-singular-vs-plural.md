# Sweep agent naming for singular-vs-plural consistency

## Why

The `dsl-implementer` agent emitted two boolean output flags as
plurals (`system-driver-ports-changed`, `external-driver-ports-changed`)
while the sibling boolean flag `dsl-port-changed` was already
singular. Renamed in this session to align both to singular. The
inconsistency went unnoticed for the lifetime of those flags, which
suggests other naming drift may exist across the rest of the agent
prompts and the process-flow YAML.

This plan defines a one-shot sweep that audits every name an ATDD
agent reads or writes, checks it against the documented convention,
and renames the outliers.

## Convention to enforce

Inferred from the current shape of `process-flow.yaml`'s `outputs:`
blocks (after the dsl-implementer rename):

- **Singular for scalar-shaped names** — booleans, single strings,
  single integers. Example: `dsl-port-changed: bool`,
  `system-driver-port-changed: bool`, `scope-exception-reason: string`,
  `command-exit-code: int`.
- **Plural for list-shaped names** — string-lists, comma-separated
  payloads, file lists. Example: `test-names: string-list`,
  `scope-exception-files: string-list`, `missing-outputs: string-list`,
  `violating-paths: string-list`.

The convention is: name follows shape. If the value type is
`string-list`, the name is plural; otherwise singular. A name that
violates that rule is an audit hit.

## Scope

In scope (read AND propose-rename):

- `internal/assets/runtime/agents/atdd/*.md` — every agent prompt's
  Inputs, Parameters, Outputs sections (the names the agent reads
  via `${name}` and writes via `gh optivem output write KEY=VAL`).
- `internal/atdd/runtime/statemachine/process-flow.yaml` — every
  `outputs:` block (the canonical declaration of the names), every
  `params:` block (call-site bindings), every `binding:` on gateway
  nodes, every `${name}` reference in `when:` predicates.
- `internal/atdd/runtime/gates/bindings.go` —
  `r.Register("kebab-name", ...)` strings; the Go function names
  next to them should mirror the kebab form.
- `internal/atdd/runtime/actions/bindings.go` — `ctx.Set("name", …)`
  / `ctx.Get("name")` strings.
- `internal/atdd/runtime/clauderun/clauderun.go` — the substitution
  map (`params["name"]`) in `renderPromptWithReferencesRoot`.
- Test files alongside the above (`*_test.go`).

Out of scope (do NOT edit):

- `docs/process-diagram.md`, `docs/architecture-diagram.md`,
  `docs/images/process-diagram-*.svg` — auto-generated. The CI
  workflow `.github/workflows/regenerate-diagram.yml` regenerates
  them on push; any rename in the YAML/agents propagates
  automatically. See memory `feedback_never_edit_generated_diagrams`.
- `plans/archived/**` — historical record of past plans. Renames
  there would falsify the historical context.
- Memory files under `~/.claude/projects/.../memory/` — per-project
  convention is to update memory only when the user-facing fact
  changes; a naming rename doesn't.

## Audit checklist

For each name found in scope, classify it into exactly one bucket:

1. **Scalar with singular name** → ✓ pass.
2. **List with plural name** → ✓ pass.
3. **Scalar with plural name** → ✗ rename to singular. Example
   shape: `*-ports-changed` (now `*-port-changed`).
4. **List with singular name** → ✗ rename to plural. Example shape
   (hypothetical): a `string-list` declared as `path-violation`
   should be `path-violations`.
5. **Ambiguous shape** (e.g. a string that holds a comma-separated
   list as a string, not a list type) → record but do not rename
   without operator decision; the storage type may be the issue,
   not the name.

For each rename:

1. Edit the canonical declaration in `process-flow.yaml`
   (`outputs:` / `binding:` / `params:`).
2. Sweep every `${name}` reference in agent prompt bodies.
3. Sweep every `r.Register(...)` and `boolStateGate(ctx, "...")` /
   `ctx.Set("...")` / `ctx.Get("...")` site in Go.
4. Sweep test files for the kebab string AND the camelCase
   identifier (e.g. rename `systemDriverPortsChanged` →
   `systemDriverPortChanged` alongside the kebab key).
5. Update any plan file under `plans/` (not `plans/archived/`) that
   references the old name.
6. Do NOT regenerate diagrams locally — CI handles it.

## Method

1. **Inventory pass** — for every agent prompt under
   `internal/assets/runtime/agents/atdd/*.md`, extract:
   - All `${name}` placeholders (inputs the agent reads).
   - All output keys (under "## Outputs" + the
     `gh optivem output write KEY=VAL` examples).
   - All gateway binding references in `process-flow.yaml` that
     ride alongside those outputs.

   Build a flat table: `agent | name | role (input/output/binding)
   | value-type-from-yaml | naming-convention-fit`.

2. **Classify pass** — walk the table and tag each row with one of
   the five buckets in the audit checklist. Anything not in buckets
   1 or 2 is a hit.

3. **Decision pass** — for each hit:
   - Bucket 3/4: stage a kebab rename.
   - Bucket 5: surface to the operator with a one-line description;
     the storage-type fix is a separate plan.

4. **Apply pass** — do the rename across the six sweep sites
   (declaration + agent prompts + gates + actions + clauderun +
   tests). One commit per name (operator preference: small, focused
   diffs; easier to revert if a rename trips a regression that the
   tests didn't catch).

5. **Verify pass** — `go build ./...` + `go test ./internal/atdd/...`
   after each commit. Then run an ATDD rehearsal in a sandbox repo
   to confirm no prompt has an unfilled placeholder (the failure
   mode that surfaced the original `touches-system-driver` bug).

## Known seeds for the inventory pass

Already-confirmed singular boolean flags (just renamed — pass):

- `dsl-port-changed`
- `system-driver-port-changed`
- `external-driver-port-changed`

Already-confirmed plural lists (pass):

- `test-names`
- `scope-exception-files`
- `missing-outputs`
- `violating-paths`

Already-confirmed singular scalars (pass):

- `scope-exception-reason`
- `command-exit-code`
- `command-stderr-tail`
- `failing-task-name`
- `expected-test-result`
- `subtype`
- `task-name`
- `ticket-kind`
- `task-subtype`
- `refactor-type-choice`
- `approval-outcome`
- `outputs-and-scopes-valid`
- `command-succeeded`
- `test-outcome`
- `fix-on-failure-enabled`

These are the rows the inventory pass starts from; the goal is to
find names *not* on this list and run them through the audit
checklist.

## Out-of-band

Naming consistency is a code-style concern, not a behavioural one —
the rehearsal that prompted this plan was unblocked by the
dsl-implementer fix. Schedule the sweep when there's a quiet window;
no urgency.

## References

- `internal/assets/runtime/agents/atdd/dsl-implementer.md` — the
  origin of the inconsistency.
- `internal/atdd/runtime/statemachine/process-flow.yaml` — canonical
  `outputs:` declarations.
- Memory `feedback_never_edit_generated_diagrams` — do not touch
  `docs/process-diagram.md` or the SVGs during this sweep.
- Memory `feedback_schema_fields_earn_slot` — the rationale that
  names must match their storage shape.
- Memory `project_retry_consolidation_priority` — sequencing rule
  for cross-cutting consolidations (one rename per commit, not a
  big-bang rename).
