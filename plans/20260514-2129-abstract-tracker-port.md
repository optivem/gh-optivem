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
| Backend selection | Discriminated by `project.url` shape | `https://github.com/...` → github; filesystem path → markdown; future `https://*.atlassian.net/...` → jira. No separate `tracker:` config field. |
| Markdown layout | `board/<status>/<id>.md` | Folder per status; `git mv` performs the status change; ordering by filename ascending. |
| Naming principle | GitHub + Jira first, not markdown | Markdown is the escape hatch; vocabulary follows the real backends. |

## The interface

```go
package tracker

type Issue struct {
    ID     string // "42" (GitHub), "SHOP-7" (Jira), "001-add-cart" (markdown)
    Title  string
    URL    string // GitHub/Jira always populate; markdown empty (unless we generate file:// links)
    Repo   string // GitHub-only: owner/repo. Jira/markdown leave empty.
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

// Open inspects projectURL and returns the matching adapter.
//   https://github.com/(orgs|users)/<x>/projects/<n>  → github adapter
//   <existing filesystem path>                        → markdown adapter
//   anything else                                     → error naming both expected shapes
func Open(ctx context.Context, projectURL string) (Tracker, error)
```

Notes:
- `SetStatus` replaces `MoveToInProgress` and `MoveToInAcceptance`. One verb covers every status change; the adapter maps `"In progress"` / `"In acceptance"` / `"Done"` to backend mechanics.
- `Handle` is the only adapter-internal escape hatch the driver carries. The state machine `Context` shuttles a single `issue_handle` string instead of today's `project_id` + `item_id` + `project_url` triple.
- `Issue.Repo` keeps the GitHub-shaped name; Jira/markdown leave it empty. We do not invent a markdown-fitting alternative — per the naming rule.

## Adapter map per method

| Method | GitHub adapter | Markdown adapter |
|---|---|---|
| `PickReady` | `gh api graphql` against projectV2 (post-workaround). | Glob `<project.url>/ready/*.md` sorted ascending by filename; return first. |
| `FindIssue(idOrURL)` | Parse `42` *or* `https://github.com/.../issues/42`; resolve project item via GraphQL. | Parse `001-add-cart` *or* `board/ready/001-add-cart.md`; locate file across all status dirs. |
| `SetStatus` | `gh project item-edit --field-id … --single-select-option-id …` | `git mv <project.url>/<from>/<id>.md <project.url>/<to>/<id>.md` (mkdir target if missing). |
| `Verify` | Minimal projectV2 GraphQL lookup (id, title). | Stat `<project.url>/ready/`, `/in-progress/`, `/done/`. |
| `Classify` | Projects v2 `Type` field + label-token table. | Frontmatter `type:` field; fall back to filename heuristic if absent. |
| `ReadSections` | Parse H2/H3 from issue body (current Issue Forms behavior). | Same parser, against the local file body — markdown content model is shared. |
| `MarkChecklistComplete` | Rewrite `- [ ]` → `- [x]` in issue body via `gh issue edit`. | Same rewrite in the file; `git add` + `git commit` the change. |

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

1. **Scaffolding.** Create `internal/atdd/runtime/tracker/` with `Issue`, `Tracker`, and a stub `Open()` that returns `ErrNotImplemented`. Tests assert the interface contract.
2. **Move github adapter.** Port `internal/atdd/runtime/board/*` → `internal/atdd/runtime/tracker/github/`. Map: `PickTopReady` → `PickReady`; `MoveToInProgress` → `SetStatus("In progress")`; `FindIssue` → `FindIssue` (newly accepting URL form too); `VerifyProjectURL` → `Verify`. `Pick` becomes `Issue` with `IssueNum int` → `ID string`.
3. **Migrate `actions/bindings.go`.** Replace direct `board.*` calls with `Tracker` calls. The `pickTopReady`/`moveToInProgress`/`moveToInAcceptance` actions shrink to ~5 lines each. `Context` shuttles a single `issue_handle` string.
4. **Migrate `classify/classify.go`.** GitHub-specific classification moves into `tracker/github/`. Runtime calls `Tracker.Classify(issue)`. Label-token table moves into the github adapter package.
5. **Migrate `intake/sections.go`.** Replace the hardcoded Issue Forms heading walker with `Tracker.ReadSections(issue, headings)`. The github adapter's `ReadSections` does today's body parse; markdown adapter's reuses the shared `tracker/internal/parse`.
6. **Migrate `gates/bindings.go`.** Same pattern — `gh` calls replaced with `Tracker.ReadSections` / `Tracker.Classify`.
7. **Build markdown adapter.** Implement all seven `Tracker` methods in `tracker/markdown/`. See the adapter-map table above for behavior per method.
8. **Adapter selection factory.** `tracker.Open(ctx, projectURL)` inspects the URL/path and returns the right adapter; unknown shapes error with both expected forms named.
9. **Drop `project_id` / `item_id` / `project_url` from `Context`.** Replaced by `issue_handle` (opaque string). The github adapter encodes its triple into `Issue.Handle` internally.
10. **Delete `internal/atdd/runtime/board/`.** All consumers migrated.
11. **Update preflight.** `preflight_helpers.go`'s `BoardURLOK` calls `tracker.Open` + `Verify`. New failure mode for markdown: "directory not found at <path>."
12. **CLI accepts `--issue <URL>`.** Update `implement_commands.go` parser (currently rejects non-numeric `--issue` values per lines 261-277). The github adapter parses both shapes; markdown adapter parses ID or file path.

## Tests

- Each adapter has a table-driven test per `Tracker` method.
- The github adapter inherits the current `board_test.go` set (fake `gh` runner), ported and adjusted for the verb-based interface.
- The markdown adapter tests use `t.TempDir()` + real filesystem + real `git init` (the codebase already runs real git in tests — see `internal/configinit/configinit_test.go`).
- `tracker.Open` factory tests: github URLs route github, paths route markdown, ambiguous strings error clearly.
- `actions/bindings_test.go`, `gates/bindings_test.go`, `intake/parse_test.go` switch from a fake `gh` runner to a fake `Tracker` (much smaller surface — seven methods).
- Windows note: per memory `feedback_go_test_windows`, never `go test ./...` unbounded. Use `scripts/test.sh` or `-p 2`, or scope to one package at a time.

## Backwards compatibility

- Existing `gh-optivem.yaml` with `project.url: https://github.com/orgs/X/projects/N` keeps working — routes to the github adapter via `tracker.Open`.
- CLI `--issue 42` keeps working — github adapter accepts numeric strings.
- CLI `--issue https://github.com/.../issues/42` is now valid (currently a parse error in `implement_commands.go`).
- No config schema changes required for github users. Markdown users set `project.url: ./board` (or absolute path).

## Decisions still needed

Not nailed down in the design conversation — call before execution:

1. **Markdown `IssueID` source.** Full filename sans `.md` (e.g. `001-add-cart`), or optional frontmatter `id:` with filename fallback? Frontmatter lets human-friendly filenames coexist with stable external IDs (e.g. mirror a Jira key); filename-only is simpler.
2. **Markdown `Title` source.** First H1 in the file, frontmatter `title:`, or filename slug? First-H1 matches the GitHub model most closely.
3. **Mixed-shape URL handling.** When `project.url` is `https://github.com/...` and the user passes `--issue https://example.atlassian.net/browse/SHOP-7`, the github adapter rejects with "not a GitHub URL." Confirm this vs. CLI-level routing.
4. **Project URL config field name.** Keep `project.url` (slight legacy when value is a folder path) or rename to `project.board` with a deprecation alias? Cleanest is rename, but breaks existing configs.
5. **`Issue.Repo` for markdown.** Empty (current proposal), or populate with the git remote of the repo containing `board/`?
6. **`MarkChecklistComplete` for markdown.** Auto-commit the rewrite (`git add` + `git commit`), or just rewrite the file and leave staging to the user? Auto-commit matches GitHub's atomic mutation; user-staged is friendlier in local-iteration mode.

## Out of scope

- A Jira adapter. The interface is shaped to accommodate one but the trigger condition (real Jira consumer) isn't met. Do not stub.
- Config-driven label tokens (covered by `plans/20260430-133420-config-driven-pipeline-labels.md`).
- The `gh project ...` → minimal GraphQL migration. That work is owned by `plans/20260514-2128-projectv2-graphql-remaining-callsites.md` and must complete first; this plan picks up its output unchanged when porting the github code into `tracker/github/`.

## Estimated effort

- Steps 1–6 + 9–12: 2–3 days (mechanical extraction; matches the prior deferred-plan estimate).
- Step 7 (markdown adapter): 1–2 days.
- Steps 1 and 7 are independent enough to run in parallel by different sessions if desired.

## Touchpoints catalog (from the original deferred analysis)

GitHub vocabulary currently lives in:

- `internal/atdd/runtime/actions/bindings.go` — `pickTopReady`, `moveToInProgress`, `moveToInAcceptance`, `tickChecklist`, `classifyTicket`, `printClassifiedSections`. Uses `gh project item-edit --field-id X --single-select-option-id Y`, parses `gh project field-list` JSON, reads `.issueType.name` from `gh issue view`.
- `internal/atdd/runtime/board/board.go` — `PickTopReady`, `MoveToInProgress`, `FindIssue`, `VerifyProjectURL`, project URL parsing.
- `internal/atdd/runtime/classify/classify.go` — reads Projects v2 `Type` field + label-token table.
- `internal/atdd/runtime/intake/sections.go` — section heading constants tied to GitHub Issue Forms.
- `internal/atdd/runtime/gates/bindings.go` — calls `gh` for body-shape inspection.

All five packages are touched by the migration steps above.
