# Replace remaining `gh project ...` calls with minimal GraphQL

> Status: ready for execution; one open decision per site (see "Decisions" below). Plan does not execute by itself.

## Context

GitHub's GraphQL resolver for Projects v2 currently regresses on the heavy
expansion queries that `gh` CLI's `project view` / `project item-list` /
`project field-list` subcommands send (full cartesian expansion of every
`ProjectV2ItemFieldValue` type variant × every field-type variant per item).
The same call shape was tested today against project `optivem/#20`:

- `gh project view 20 --owner optivem` → fails with GraphQL correlation-ID errors
- `gh project item-list 20 …` → fails
- `gh project field-list 20 …` → fails
- Minimal hand-written GraphQL queries against the same project, asking for
  only the scalars the callers consume → succeed

The minimal-query workaround was already applied inside `internal/atdd/runtime/board`
in the same session that produced this plan (commit not yet made at write
time — see `board.go` / `board_test.go` working-tree diff). Three new helpers
live in board:

- `projectMetaQuery` + `fetchProjectMetadata` (replaces `gh project view`)
- `projectItemsQuery` + `fetchProjectItems` (replaces `gh project item-list`,
  with `pageInfo`-driven pagination at `items(first:100)`)
- `projectFieldsQuery` + a rewritten `lookupStatusField` (replaces
  `gh project field-list`)

`parseProjectURL` was extended to return `ownerKind` (`"organization"`|`"user"`)
so each query dispatches to the right top-level GraphQL field — querying both
in one request produces a partial `NOT_FOUND` for the wrong type, which
`gh api graphql` treats as fatal.

Two `gh project ...` callsites remain in the codebase and will hit the same
upstream bug. This plan covers both.

## Vulnerable callsites that remain

| # | File | Function | Subcommand | On rehearsal path? |
|---|---|---|---|---|
| 1 | `internal/atdd/runtime/actions/bindings.go:1766` | `lookupStatusOption` | `gh project field-list` | **Yes** — state machine hits this on every status transition (In acceptance, In QA, …). |
| 2 | `internal/config/config.go:865` | `realCheckProjectExists` | `gh project view` | No — only invoked from `gh optivem init` config validation. Same bug class. |

---

## Site #1 — `actions/bindings.go::lookupStatusOption`

### Current shape

```go
// :1761
func lookupStatusOption(ctx context.Context, gh GhRunner, sCtx *statemachine.Context, optionName string) (fieldID, optionID string, err error) {
    owner, number, err := projectOwnerAndNumber(sCtx)
    if err != nil { return "", "", err }
    out, err := gh.Run(ctx, "project", "field-list", strconv.Itoa(number), "--owner", owner, "--format", "json")
    if err != nil { return "", "", fmt.Errorf("gh project field-list: %w", err) }
    field, optionID, err := findStatusOption(out, optionName)
    if err != nil { return "", "", err }
    return field, optionID, nil
}
```

`findStatusOption` calls `findFieldBlock` (a permissive hand-rolled JSON
scanner) followed by `jsonFieldString` / `jsonFieldRaw` / `splitJSONArray`.
The hand-rolled parser was deliberately introduced "to avoid importing
encoding/json into every action — keeps the file small. The shape is
well-known" (`:1779`). The argument for staying with the hand-rolled parser
is weaker now: the response shape changes from the gh-flat-JSON to a deeply
nested GraphQL response, and the comment "the shape is well-known" no longer
holds because the shape is whatever we tell GraphQL to return.

`lookupStatusOption` is called from at least one binding (`move_to_in_acceptance`,
`bindings.go:268`) — likely more once the rehearsal progresses through QA
and Done transitions. Run `grep -n lookupStatusOption internal/atdd/runtime/actions/`
to find them all at execution time.

### Approach options

**Approach A — Promote board's helper, call into it from actions.**

Generalize `board.lookupStatusField` (currently hard-coded to "In progress")
to take an option name, and expose it as a public function:

```go
// in board.go
func LookupStatusOption(ctx context.Context, gh GhRunner, projectURL, optionName string) (fieldID, optionID string, err error)
```

Then `actions.lookupStatusOption` becomes a one-line passthrough to
`board.LookupStatusOption`. `bindings.go` already imports
`internal/atdd/runtime/board` (`:30`), so this is a zero-dependency-graph-change move.

Removes:
- ~25 lines of `lookupStatusOption` body
- ~24 lines of `findStatusOption` (becomes unreachable; keep if any other caller)
- `findFieldBlock` (also becomes unreachable if no other caller; verify)
- All four hand-rolled JSON helpers (`jsonFieldRaw`, `jsonFieldString`,
  `splitJSONArray`, `findFieldBlock`) — only if zero remaining callers

Adds:
- ~3 lines exposed-helper wrapper in board
- New optional 4th parameter on board's existing `MoveToInProgress` path
  (or keep `lookupStatusField` as a thin wrapper that calls the generalised
  version with `"In progress"`)

Trade-off: it changes board's public API surface. We accept that — the
package already exports the picker/move functions, and a generalised
status-option lookup is a coherent member of the same surface.

**Approach B — Duplicate the GraphQL helper in bindings.go.**

Mirror what board does (query string + parsing) inside bindings.go.
Either add `encoding/json` (drops the hand-rolled JSON helpers) or keep
the hand-rolled parser and adapt it to navigate `data.<kind>.projectV2.fields.nodes`.

Why not: produces a second copy of `projectFieldsQuery` to maintain.
Drift between board and actions is exactly what bit us last time someone
changed a related shape.

### Recommended approach: **A**

Cleaner, single source of truth, removes hand-rolled JSON parsing. The
original "avoid encoding/json" justification was about avoiding heavy
imports per-action; promoting the helper to board sidesteps that
entirely.

### Test impact

- `internal/atdd/runtime/actions/bindings_test.go` (or wherever
  `lookupStatusOption` is exercised): canned-response fixtures keyed on
  `["project","field-list", …]` argv must move to
  `projectFieldsArgs("organization", …)` (or whichever helper board
  exposes alongside) and the response body must move from gh-flat-JSON
  to the GraphQL shape already used in `board_test.go::fieldListJSON`.
- Any test that uses `findStatusOption` / `findFieldBlock` directly:
  delete if those helpers become unreachable.

Grep `internal/atdd/runtime/actions/*_test.go` for these symbols before
deleting anything.

---

## Site #2 — `config/config.go::realCheckProjectExists`

### Current shape

```go
// :860
func realCheckProjectExists(url string) error {
    owner, number, err := parseProjectURL(url)
    if err != nil { return fmt.Errorf("project URL %q: %w", url, err) }
    cmd := exec.Command("gh", "project", "view", strconv.Itoa(number), "--owner", owner, "--format", "json")
    cmd.Stderr = nil
    if err := cmd.Run(); err != nil {
        return fmt.Errorf("project %s/%d not found or not accessible", owner, number)
    }
    return nil
}
```

`config/config.go:877-879` documents the architectural rule that prevents
importing board here: *"Duplicated from internal/steps/project.go and
internal/atdd/runtime/board rather than imported to keep internal/config
dependency-free of the runtime-side packages it underpins."*

So Approach A (call into board) is **off the table** for site #2. The fix
duplicates the minimal query inline.

### Approach

Replace the `exec.Command("gh", "project", "view", …)` invocation with
`exec.Command("gh", "api", "graphql", "-F", "login=…", "-F", "number=…", "-f", "query=…")`.

The query string is the same shape as board's `projectMetaQuery` but
inlined here to honour the package-isolation rule. Adapt
`parseProjectURL` in config/config.go to return `ownerKind` the same way
board's was extended — config/config.go has its own copy of
`parseProjectURL` (the duplication is intentional, per the same comment).

Response decoding: only need exit-status success, like the current code.
Don't parse the body unless we want to surface a clearer error message.
(Current behaviour is to lump "doesn't exist" and "exists but caller
can't read it" — keep that.)

### Test impact

- `internal/config/config_test.go` (or wherever `realCheckProjectExists`
  is tested): no fakes today for this function — it's wired into a
  `CheckProjectExistsFn` pluggable from the `Validate` path. Check for
  any test that does a real or stubbed shell-out on `project view`. None
  expected; verify at execution time.

---

## Shared work

- Both sites need their local `parseProjectURL` to return `ownerKind`
  the same way board's did. Site #1 imports board's already and gets it
  for free (Approach A). Site #2's standalone copy needs the same
  extension.
- Confirm no other `gh project (view|item-list|field-list)` callsites
  remain after these two — re-run the grep that found these.

## Acceptance criteria

1. `grep -rn '"project", "view"' --include='*.go'` returns zero hits.
2. `grep -rn '"project", "item-list"' --include='*.go'` returns zero hits.
3. `grep -rn '"project", "field-list"' --include='*.go'` returns zero hits.
4. `go build ./...` clean.
5. `go test -p 2 ./internal/atdd/... ./internal/steps/... ./internal/config/...` all green.
6. `bash scripts/atdd-rehearsal.sh 61 --config gh-optivem-monolith-typescript.yaml`
   — picker and at least one status transition succeed. (Cannot verify
   the *full* rehearsal in unit tests; this is the smoke gate.)

## Decisions to confirm before execution

These are *not* pre-asked here — per the workspace convention, walk them
one at a time at execution time.

1. **Site #1 approach.** Plan recommends A (promote helper to board's
   public API). Confirm or pick B (duplicate in bindings.go).
2. **Generalisation shape for board.** If A: should
   `board.lookupStatusField` be **renamed** to `LookupStatusOption` with
   the option name as a parameter (and a deprecated thin wrapper for
   the "In progress" case), or should we keep both side by side? Plan
   leans rename + wrapper.
3. **Hand-rolled JSON helpers' fate.** If A: confirm we may delete
   `findStatusOption`, `findFieldBlock`, `jsonFieldRaw`,
   `jsonFieldString`, `splitJSONArray` when unreferenced. Grep first.

## Cross-references

- Working-tree changes from the in-progress board-package fix (same
  session, not committed at write time): `internal/atdd/runtime/board/board.go`
  + `internal/atdd/runtime/board/board_test.go`.
- Upstream symptom: GitHub GraphQL correlation IDs in the form
  `D4xx:xxxxxx:xxxxxxx:xxxxxxx:6A06xxxx` on `2026-05-14T18:5x-19:1xZ`.
  Status page (githubstatus.com) was green throughout — Projects v2 is
  not a separately-tracked component and partial GraphQL errors don't
  trip the page's thresholds.
