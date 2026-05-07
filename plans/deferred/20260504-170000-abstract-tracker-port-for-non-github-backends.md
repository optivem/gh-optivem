# Abstract a Tracker port so the intake/board layer is not GitHub-only

**Status: Deferred.** Not actively planned for execution. Open this plan when a second tracker (JIRA, Linear, Azure Boards, …) is on a real near-term roadmap with a concrete consumer, not before. Until then the GitHub-coupled implementation is the right call — see "Why deferred" below.

## Motivation

Today every intake / board / classification action is hardwired to GitHub. The runtime calls `gh project ...` / `gh issue ...` directly, parses Projects v2 JSON, reads the native `issueType` field, and ticks markdown `- [ ]` checkboxes in issue bodies. If a future consumer needs to drive the same ATDD pipeline against JIRA (or Linear, Azure Boards, …) every one of those touchpoints needs to change.

Concretely, GitHub vocabulary lives in:

- `internal/atdd/runtime/actions/bindings.go` — `pickTopReady`, `moveToInProgress`, `moveToInAcceptance`, `tickChecklist`, `classifyTicket`, `printClassifiedSections`. Uses `gh project item-edit --field-id X --single-select-option-id Y`, parses `gh project field-list` JSON, and reads `.issueType.name` from `gh issue view --json issueType`.
- `internal/atdd/runtime/board/board.go` — `PickTopReady`, `MoveToInProgress`, project URL parsing (`/orgs/X/projects/N` and `/users/X/projects/N`).
- `internal/atdd/runtime/classify/classify.go` — reads Projects v2 `Type` field plus a label-token table.
- `internal/atdd/runtime/intake/sections.go` — section heading constants tied to GitHub Issue Forms (Issue Forms render each `label:` as an H2/H3 in the issue body; that is how the parser finds them).
- `internal/atdd/runtime/gates/bindings.go` — also calls `gh` for body-shape inspection.

JIRA (and most non-GitHub trackers) do not map cleanly onto several of these:

- **Status changes** are workflow transitions, not "edit a single-select field option ID".
- **Native issue type** is `issueTypeId` in JIRA; mapping to `Story / Bug / Task / Chore` is per-project.
- **Issue Forms** have no equivalent — JIRA's analogue is custom fields per section, with values stored in ADF (Atlassian Document Format) or wiki markup, not raw markdown. The "section headings as parser anchors" model from `intake/sections.go` does not survive the move.
- **Markdown checkboxes** are GitHub-native; JIRA needs the Checklist plugin (paid) or a different content model.
- **Project URL parsing** is pure Projects v2.

## Why deferred

Three reasons we should not build this now.

1. **One tracker, no concrete second consumer.** Premature ports-and-adapters tend to ossify around the wrong seam — the right interface only becomes obvious after the second backend forces it. The cost of guessing today (every action becomes stringly-typed via a generic `Tracker`, GitHub-specific affordances like Issue Forms get hidden behind generic verbs) is real; the benefit is hypothetical.
2. **Two of the touchpoints don't generalise.** Issue Forms and markdown checkboxes are not "GitHub doing the same thing differently" — they are content-model choices that JIRA does not have. Designing those out of the runtime requires either dropping them from the GitHub adapter (loss of capability) or accepting that the JIRA adapter will work very differently (the abstraction does not buy uniformity in the place that matters most).
3. **The minimal discipline below already buys ~90% of the future-proofing**, at zero cost.

## Minimal discipline to keep now (zero-cost, do as we touch the code)

Keep GitHub vocabulary contained inside the `actions` / `board` / `classify` packages. Only put domain words into `statemachine.Context`:

- ✅ Already domain-shaped: `issue_num`, `issue_repo`, `issue_url`, `issue_title`, `ticket_type`, `classify_confident`, `external_system_name`, `smoke_test_passes`.
- ⚠️ Leaky GitHub-isms in Context: `project_id`, `item_id`, `project_url`. These are Projects v2 primary keys. They flow between `pick_top_ready`, `move_to_in_progress`, and `move_to_in_acceptance` because resolving them once is cheaper than re-resolving per call.

Acceptable for now (they're a perf optimisation, not a leak of API shape into business logic) — but flag in code review if any new Context key adopts a GitHub-specific name.

A small concrete audit task that's worth doing today, separate from this plan: grep Context keys set/read across `actions`, `gates`, and `driver`, and confirm no new GitHub-shaped keys are creeping in. If we keep that line, the future extraction is mechanical.

## The seam, when we do extract it

A `Tracker` port with **verbs**, not field names. Sketch:

```go
package tracker

type Ticket struct {
    Num   int
    Title string
    URL   string
    Repo  string // free-form scope identifier — owner/repo for GitHub, projectKey for JIRA
}

type Tracker interface {
    PickReady(ctx context.Context) (Ticket, error)
    SetStatus(ctx context.Context, t Ticket, status string) error // status names: "In progress", "In acceptance", …
    Classify(ctx context.Context, t Ticket) (kind string, confident bool, err error)
    ReadSections(ctx context.Context, t Ticket, headings []string) (map[string]string, error)
    MarkChecklistComplete(ctx context.Context, t Ticket) error
}
```

Notes on the shape:

- **Verbs hide the mechanism.** `SetStatus(t, "In acceptance")` hides "look up Status field ID, look up option ID, call `project item-edit`" on GitHub vs. "POST /rest/api/3/issue/{key}/transitions" on JIRA. The runtime never sees the difference.
- **Status is a string, not an enum.** Different trackers have different workflow states; the runtime already speaks in "In progress"/"In acceptance" strings, so let the adapter map.
- **`ReadSections` returns a map, not a parser.** The GitHub adapter does the markdown-heading parse; the JIRA adapter reads the matching custom fields. The runtime asks for `["Description", "Acceptance Criteria", "Checklist"]` and gets a `map[string]string` back.
- **`MarkChecklistComplete` is an opaque side-effect.** GitHub adapter rewrites `- [ ]` → `- [x]`; JIRA adapter no-ops (or calls the Checklist plugin REST endpoint); the runtime does not care.
- **`Classify` stays in the port.** It's the cleanest place to hide "Projects v2 Type field + labels" vs. "JIRA `issueTypeId` + project-specific mapping". The label-token table moves into the GitHub adapter.

What stays out of the port:

- `project_id` / `item_id` / `project_url` — adapter-internal state, not Context state.
- The intake-section heading constants — they become input parameters to `ReadSections`, not constants the runtime hardcodes.
- The Issue Forms YAML schema — that lives only in the GitHub-targeted `init` flow.

## Implementation outline (when undeferred)

1. Define `internal/atdd/runtime/tracker` with the `Tracker` interface and `Ticket` type.
2. Move `internal/atdd/runtime/board/*`, the GitHub-specific halves of `actions/bindings.go`, the GitHub-specific bits of `classify/classify.go`, and the markdown-section parser from `actions` into `internal/atdd/runtime/tracker/github/`. The package implements `Tracker`.
3. Rewrite `actions/bindings.go` to call `Tracker` instead of `gh`/`board`/`classify` directly. Each action becomes ~5 lines.
4. Drop `project_id` / `item_id` / `project_url` from `Context`. The GitHub `Tracker` implementation memoises them internally per run.
5. Replace the Issue Forms section-headings parse in `intake/sections.go` with a `Tracker.ReadSections(headings)` call. The `intake` package shrinks to "the canonical heading list" (one slice of strings).
6. Add `internal/atdd/runtime/tracker/jira/` (or whichever the triggering consumer needs). This package exists only when the second consumer is being built — do not stub it ahead of time.
7. Wire selection: a small factory keyed on a config field (`tracker: github` / `tracker: jira`) constructs the right implementation at startup. Default `github` keeps every existing consumer untouched.
8. Update transitions tests + structural-cycle tests — they currently use a fake `gh` runner; switch them to a fake `Tracker` (much smaller surface).

Estimated effort when triggered: 2–3 days for the GitHub adapter extraction (mostly mechanical), plus however long the second adapter takes (depends entirely on the target tracker — JIRA Cloud REST is well-documented but `MarkChecklistComplete` and `ReadSections` are non-trivial because of the content-model mismatch).

## When to undefer

Trigger conditions, any of:

- A real consumer wants to drive the ATDD pipeline against a non-GitHub tracker, with a concrete repo and a deadline.
- We discover the GitHub-specific concepts are leaking into `statemachine` itself (engine-level coupling) — at that point the abstraction is forced, even without a second consumer.
- A third place inside `actions` / `gates` starts re-implementing the same Projects v2 lookup (Status field ID, option ID). That repetition is the signal that the seam wants to be a port.

Until then: the GitHub-direct implementation is fine, and the "Minimal discipline to keep now" section above is the only ongoing work.

## See also

- `plans/20260430-133420-config-driven-pipeline-labels.md` — closely related deferred plan. That one makes the *vocabulary* (label tokens, ticket-type names) configurable per consumer; this one would make the *backend* (GitHub vs. JIRA) configurable per consumer. They are independent — vocab can be configurable without abstracting the tracker, and vice versa — but a JIRA adapter would almost certainly want config-driven labels too, so they'd likely land together.
- `internal/atdd/runtime/actions/bindings.go` line 220 (`classifyTicket`) — clearest single example of GitHub-shaped logic the port would hide.
- `internal/atdd/runtime/intake/sections.go` — note the comment "Issue Forms (.github/ISSUE_TEMPLATE/*.yml) enforce the canonical heading shape". That assumption is exactly what does not survive the JIRA move.
