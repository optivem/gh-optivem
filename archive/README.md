# archive/

Retired, unused source kept in-tree for reference only. Nothing here is
embedded, shipped, built, or synced by the `gh-optivem` binary.

## `references/`

The former `internal/assets/runtime/references/` tree — the ATDD architecture
doctrine (`references/atdd/`) and the per-language equivalents + testkit
reference docs (`references/code/`).

It was once synced to `~/.gh-optivem/references/` and materialized per-project
to `<repo>/.gh-optivem/references/` by the `internal/assets/sync` package, and
agent prompts were meant to read it via the `${references-root}` placeholder.
By the time it was archived (2026-05-29), **zero** prompts or configs referenced
`${references-root}`: the consumer side was dead, so the whole sync/materialize
subsystem and this doc tree were retired together. Moved out of
`internal/assets/` so `//go:embed runtime` no longer ships it.

Kept here rather than deleted because the doctrine prose is still a useful
historical reference. If a future feature needs these docs live again, wire them
back through a fresh delivery mechanism rather than resurrecting the sync
package.
