# Abstract a Tracker port so the intake/board layer is not GitHub-only

**Status: ACTIVE — decisions captured.** Undeferred 2026-05-14 (was deferred 2026-05-04). This plan now consolidates both the original deferred analysis and the 2026-05-14 design walkthrough into a single document.

## Why now (undeferral triggers)

Two triggers, both real:

1. **GitHub Projects is acting as a near-term blocker.** Instability on the projectV2 complex-query path is already forcing an in-flight workaround in `internal/atdd/runtime/board/board.go` (migrate `gh project view` / `gh project item-list` to minimal direct GraphQL — currently staged in the working tree). A local, network-free fallback adapter is needed as an escape hatch when GitHub is unavailable, not as a hypothetical future feature.
2. **A real second adapter is in scope from day one.** This plan ships GitHub *and* markdown adapters together. Two concrete implementations is the minimum needed to find what is genuinely common vs. accidentally GitHub-shaped. The deferred plan's "wait for the second concrete consumer" rule is satisfied — markdown is that consumer.

The deferred plan's caution about "two touchpoints don't generalize" (Issue Forms, markdown checkboxes) was framed against Jira. For markdown specifically it turns out to be moot, because markdown files have the same content model as GitHub issue bodies — H2/H3 headings and `- [ ]` checkboxes parse the same way. See the per-method analysis below. The concern remains valid for a future Jira adapter, which is out of scope here.

## Cross-references

- Was `plans/deferred/20260504-170000-abstract-tracker-port-for-non-github-backends.md`. Renamed and updated in place.
- **`plans/20260514-2128-projectv2-graphql-remaining-callsites.md` must land before this plan executes.** That plan extends the same `gh project ...` → minimal GraphQL workaround from `board/` to the remaining call sites in `actions/bindings.go`, `gates/bindings.go`, etc. The abstraction plan (this one) then picks up the post-workaround github code wholesale when porting it into `tracker/github/`. Doing this work in the other order would force re-doing the GraphQL migration inside the new package.
- `plans/20260430-133420-config-driven-pipeline-labels.md` — related deferred plan that makes vocabulary (label tokens, ticket-type names) configurable. Independent of this work; a future Jira adapter would want both together.

## Design decisions (from 2026-05-14 walkthrough)

Settled in conversation and baked into the rest of the plan:

| Decision | Choice | Rationale |
|---|---|---|
| Seam | `Tracker` interface (7 methods) | Project board alone is too narrow — Classify, ReadSections, MarkChecklistComplete also live on the seam. |
| Adapters delivered | github + markdown | Two concrete implementations from day one; no Jira stub. |
| ID type | `string` | Fits GitHub stringified numbers, Jira keys (`SHOP-7`), markdown slugs. |
| ID field name | `IssueID` | "issue" is shared GitHub + Jira vocabulary; "ID" is generic where "Num" is GitHub-specific and "Key" is Jira-specific. |
| Struct name | `Issue` (not `Ticket`) | GitHub and Jira both say "issue." See memory: `feedback_naming_github_jira_first`. |
| CLI flag | `--issue` accepts ID *or* URL | Both backends have URL-addressable issues; adapter detects the shape. |
| Backend selection | Explicit required `project.provider` field (`github` \| `markdown`) | Self-documenting; trivial dispatch in `tracker.Open`; loud config-mismatch errors; forward-compatible for `jira`. |
| Older configs missing `provider` | Hard error + `gh optivem config migrate` auto-fix | Error message names the exact fix; the migrate command infers provider from URL shape and idempotently adds the field. |
| `Issue.Repo` field | Dropped from the interface entirely | Control-flow uses (`gh issue view --repo …`) migrate to `Tracker.Classify` / `ReadSections` / `MarkChecklistComplete`; subagent preamble drops the `(repo)` suffix. One fewer leak. |
| Markdown `IssueID` source | Full filename sans `.md` | One source of truth; rename = change ID. Lets `SHOP-7.md` mirror a Jira key. |
| Markdown `Title` source | First H1 in file; filename fallback | Closest to GitHub model. |
| Markdown layout | `board/<status>/<id>.md` | Folder per status; `git mv` performs the status change; ordering by filename ascending. |
| Markdown `MarkChecklistComplete` | Auto-commit after rewrite | `git add` + `git commit` so the working tree stays clean after the call. |
| Mixed-URL `--issue` handling | Active adapter rejects with clear error | No CLI-level adapter routing — predictable that `--issue` is interpreted by the configured backend. |
| Naming principle | GitHub + Jira first, not markdown | Markdown is the escape hatch; vocabulary follows the real backends. |

## The interface

```go
package tracker

type Issue struct {
    ID     string // "42" (GitHub), "SHOP-7" (Jira), "001-add-cart" (markdown)
    Title  string
    URL    string // GitHub/Jira always populate; markdown empty (unless we generate file:// links)
    Handle string // opaque per-backend payload; github encodes "projectID:itemID"
}

type Tracker interface {
    PickReady(ctx context.Context) (Issue, error)
    FindIssue(ctx context.Context, idOrURL string) (Issue, error)
    SetStatus(ctx context.Context, handle, status string) error
    Verify(ctx context.Context) error
    Classify(ctx context.Context, i Issue) (kind string, confident bool, err error)
    ReadSections(ctx context.Context, i Issue, headings []string) (map[string]string, error)
    MarkChecklistComplete(ctx context.Context, i Issue) error
}

// Open dispatches on cfg.Provider, validates cfg.URL against the chosen
// adapter's expected shape, and returns the adapter. Unknown provider
// values, or provider/URL mismatches, return an error naming both fields.
func Open(ctx context.Context, cfg projectconfig.Project) (Tracker, error)
```

Notes:
- `SetStatus` replaces `MoveToInProgress` and `MoveToInAcceptance`. One verb covers every status change; the adapter maps `"In progress"` / `"In acceptance"` / `"Done"` to backend mechanics.
- `Handle` is the only adapter-internal escape hatch the driver carries. The state machine `Context` shuttles a single `issue_handle` string instead of today's `project_id` + `item_id` + `project_url` triple.
- `Issue.Repo` is **not** on the struct. The github adapter's internal repo state lives in `Handle`; the seven `gh issue view --repo …` call sites in `actions`/`gates`/`classify` migrate to `Tracker.Classify` / `Tracker.ReadSections` / `Tracker.MarkChecklistComplete`. The agent preamble template drops the `(repo)` suffix — subagents have the issue URL, which is enough.

## Adapter map per method

| Method | GitHub adapter | Markdown adapter |
|---|---|---|
| `PickReady` | `gh api graphql` against projectV2 (post-workaround). | Glob `<project.url>/ready/*.md` sorted ascending by filename; return first. |
| `FindIssue(idOrURL)` | Parse `42` *or* `https://github.com/.../issues/42`; resolve project item via GraphQL. | Parse `001-add-cart` *or* `board/ready/001-add-cart.md`; locate file across all status dirs. |
| `SetStatus` | `gh project item-edit --field-id … --single-select-option-id …` | `git mv <project.url>/<from>/<id>.md <project.url>/<to>/<id>.md` (mkdir target if missing). |
| `Verify` | Minimal projectV2 GraphQL lookup (id, title). | Stat `<project.url>/ready/`, `/in-progress/`, `/done/`. |
| `Classify` | Projects v2 `Type` field + label-token table. | Frontmatter `type:` field; fall back to filename heuristic if absent. |
| `ReadSections` | Parse H2/H3 from issue body (current Issue Forms behavior). | Same parser, against the local file body — markdown content model is shared. |
| `MarkChecklistComplete` | Rewrite `- [ ]` → `- [x]` in issue body via `gh issue edit`. | Same rewrite in the file, then `git add <file> && git commit -m "checklist: tick item N for <id>"`. Working tree stays clean. |

## Package layout

```
internal/atdd/runtime/tracker/
  tracker.go          # Issue struct, Tracker interface, Open() factory
  github/
    github.go         # implements Tracker; absorbs current board/ + github halves of classify/intake
    github_test.go
  markdown/
    markdown.go       # implements Tracker; new
    markdown_test.go
  internal/parse/     # shared markdown H2/H3 + checklist parser, reused by both adapters
    parse.go
```

The existing `internal/atdd/runtime/board/` is decomposed:
- URL/path parsing + status name normalization → `tracker.go`
- All `gh` CLI / GraphQL logic → `tracker/github/`
- Package deleted at the end.

## Migration steps

Each step is a single commit (or small commit pair) with passing tests. The github GraphQL migration currently in the working tree (`board.go` modifications) lands first as a self-contained github-adapter-internal change; step 2 picks it up wholesale.

> Steps 1–7 completed 2026-05-14. The new package lives at `internal/atdd/runtime/tracker/`. Shared parsing primitives live in `tracker/internal/parse/` (ExtractSection, TickCheckboxes, HasUnchecked, FirstH1); both adapters consume them. The github adapter satisfies all seven Tracker methods against projectV2 + REST. The markdown adapter satisfies all seven against a `<boardDir>/<status>/<id>.md` layout — git operations go through an injectable `GitRunner` (note: the plan claimed `configinit_test.go` runs real git; it doesn't, so the markdown adapter mirrors github's `GhRunner` abstraction instead of shelling out in tests). Consumers (`actions/bindings.go`, `gates/bindings.go`, `intake/parse.go`) route through the Tracker interface. The `classify/` package is deleted. The old `internal/atdd/runtime/board/` package is still imported by `driver/driver.go` and `preflight_helpers.go` — step 13 deletes it once those switch over.

> Steps 8, 9, 10, 14 completed 2026-05-15. `project.provider` is now a required field on `projectconfig.Project` (Validate Rule 19 + 20: presence + provider/url shape consistency). Adapter dispatch lives in a sibling package `internal/atdd/runtime/tracker/factory` (the planned `tracker.Open` location would have created an import cycle — both adapters import `tracker` for the interface + sentinel types). `gh optivem config migrate` idempotently back-fills `provider` from URL shape via yaml.v3 Node round-trip (preserves comments + key ordering). `preflight_helpers.go` now opens via `factory.Open` + `Tracker.Verify` instead of `board.VerifyProjectURL`. Step 15 was already done in earlier work — `parseIssueArg` in `implement_commands.go:260` accepts both numeric and URL forms.

11. **Drop `project_id` / `item_id` / `project_url` from `Context`.** Replaced by `issue_handle` (opaque string). The github adapter encodes its triple into `Issue.Handle` internally.
12. **Drop `Issue.Repo` / `issue_repo` from runtime.** Remove the field from the seven `gh issue view --repo …` call sites (subsumed by `Tracker` methods), from `clauderun.Options.IssueRepo`, from the agent preamble template (`internal/assets/runtime/shared/preamble.md`), and from `Context` (`issue_repo` key).
13. **Delete `internal/atdd/runtime/board/`.** All consumers migrated.

## Tests

- Each adapter has a table-driven test per `Tracker` method.
- The github adapter inherits the current `board_test.go` set (fake `gh` runner), ported and adjusted for the verb-based interface.
- The markdown adapter tests use `t.TempDir()` + real filesystem + real `git init` (the codebase already runs real git in tests — see `internal/configinit/configinit_test.go`).
- `tracker.Open` factory tests: github URLs route github, paths route markdown, ambiguous strings error clearly.
- `actions/bindings_test.go`, `gates/bindings_test.go`, `intake/parse_test.go` switch from a fake `gh` runner to a fake `Tracker` (much smaller surface — seven methods).
- Windows note: per memory `feedback_go_test_windows`, never `go test ./...` unbounded. Use `scripts/test.sh` or `-p 2`, or scope to one package at a time.

## Backwards compatibility

- **Required new field.** `project.provider` is mandatory after this plan lands. Existing configs without it fail to load with a clear error pointing at `gh optivem config migrate`.
- **Migrate path** (one-shot): `gh optivem config migrate` reads `gh-optivem.yaml`, infers `provider` from the existing `url` shape (https github URL → `github`; resolvable directory → `markdown`), and writes the field back idempotently. Existing comments and ordering preserved.
- **CLI continuity.** `--issue 42` keeps working — github adapter accepts numeric strings. `--issue https://github.com/.../issues/42` is now valid (was a parse error in `implement_commands.go` lines 261-277).
- **Markdown setup.** Markdown users set `project.provider: markdown` + `project.url: ./board` (or absolute path), and create `board/{ready,in-progress,done}/` directories.

## Out of scope

- A Jira adapter. The interface is shaped to accommodate one but the trigger condition (real Jira consumer) isn't met. Do not stub.
- Config-driven label tokens (covered by `plans/20260430-133420-config-driven-pipeline-labels.md`).
- The `gh project ...` → minimal GraphQL migration. That work is owned by `plans/20260514-2128-projectv2-graphql-remaining-callsites.md` and must complete first; this plan picks up its output unchanged when porting the github code into `tracker/github/`.

## Estimated effort

- Steps 1–6 + 11–15: ~2–3 days (mechanical extraction; matches the prior deferred-plan estimate).
- Step 7 (markdown adapter): ~1–2 days.
- Steps 8–10 (config field + migrate command): ~half a day.
- Steps 1 and 7 are independent enough to run in parallel by different sessions if desired.

## Touchpoints catalog (from the original deferred analysis)

GitHub vocabulary currently lives in:

- `internal/atdd/runtime/actions/bindings.go` — `pickTopReady`, `moveToInProgress`, `moveToInAcceptance`, `tickChecklist`, `classifyTicket`, `printClassifiedSections`. Uses `gh project item-edit --field-id X --single-select-option-id Y`, parses `gh project field-list` JSON, reads `.issueType.name` from `gh issue view`.
- `internal/atdd/runtime/board/board.go` — `PickTopReady`, `MoveToInProgress`, `FindIssue`, `VerifyProjectURL`, project URL parsing.
- `internal/atdd/runtime/classify/classify.go` — reads Projects v2 `Type` field + label-token table.
- `internal/atdd/runtime/intake/sections.go` — section heading constants tied to GitHub Issue Forms.
- `internal/atdd/runtime/gates/bindings.go` — calls `gh` for body-shape inspection.

All five packages are touched by the migration steps above.
