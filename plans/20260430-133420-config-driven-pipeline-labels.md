# Make ATDD pipeline labels and ticket-type vocabulary config-driven

**Status: Deferred.** Not actively planned for execution. Open this plan when a second consumer repo materialises with a label vocabulary that diverges from `shop`'s.

## Motivation

The ATDD runtime pipeline (`internal/atdd/runtime/{classify,actions,gates}/` + `docs/atdd/process/process-flow.yaml` shipped per consumer) currently hardcodes both:

1. **Label tokens** that drive classification (`bug`, `feature`, `chore`, `system-api-redesign`, `system-ui-redesign`, `external-system-api-change`, `refactor`, `task`, `story`) — see `internal/atdd/runtime/classify/classify.go` `classifyLabels`.
2. **Ticket-type literals** consumed by YAML `when:` clauses and the manual prompts in `actions.classifyTicket` / `gates.ticketTypeFallback` (`system-api-task`, `system-ui-task`, `external-api-task`, plus `story`/`bug`/`chore`).

This means every consumer repo (currently only `shop`) must adopt those exact labels and types. As of `2026-04-30`, the `shop` repo's labels were renamed via `gh label edit` to match the hardcoded vocabulary — that fix is fine for one consumer, but doesn't scale.

When the second consumer (e.g. `eshop`, `courses`, an external partner repo) needs a different vocabulary — `defect` instead of `bug`, `improvement` instead of `feature`, sub-types named after their domain — the right move is to make this configurable rather than to mass-rename their labels.

## Bounded scope (the design we agreed on, before deferring)

The four classification *slots* (`story`, `bug`, `chore`, `task`) stay framework-level — they pair 1:1 with the `atdd-{story,bug,chore,task}` intake agents and the fixed routing in `process-flow.yaml`. Letting consumers redefine the slots themselves means also letting them rewire intake agents and `when:` clauses, which is an order of magnitude more complexity for marginal benefit.

Only the *labels that resolve into each slot* are repo-customisable. For the `task` slot, sub-type labels also map to structural-cycle subtypes.

Schema shape (mirrors what we sketched on `2026-04-30` and partially landed before reverting):

```yaml
project:
  url: https://github.com/orgs/optivem/projects/20
  name: Shop Project

classifications:
  story:    { labels: [story, feature] }
  bug:      { labels: [bug, ui-bug, api-bug] }
  chore:    { labels: [chore, refactor] }
  task:
    subtypes:
      system-api-redesign:           { labels: [system-api-redesign-task] }
      system-ui-redesign:            { labels: [system-ui-redesign-task] }
      external-system-api-redesign:  { labels: [external-system-api-redesign-task] }
```

When the file is absent or the `classifications:` block is missing, fall back to today's hardcoded defaults — no consumer is forced to adopt the schema until they need to diverge.

## Implementation outline

1. **Extend `internal/atdd/runtime/config/config.go`** — restore the `Classifications`, `TicketType`, `TaskClassification` structs (they were added and reverted on `2026-04-30`; commit `4a91f1e`'s parent has the version that landed). Add a sibling test for parse round-trip.
2. **Thread config into `classify`** — add `LabelTokens map[string][]string` and `TaskSubtypes map[string][]string` to `classify.Options`. When set, replace the hardcoded `tokens` table in `classifyLabels`. When nil, retain today's defaults so existing consumers don't break.
3. **Extend `classify.Result`** with a `Subtype string` field, populated when a Task classification's label matches one of the configured subtype label sets.
4. **Update `actions.classifyTicket`** to load config (single `config.Load(repoPath)` call), pass `LabelTokens` / `TaskSubtypes` into `classify.Classify`, skip the manual subtype prompt when `Result.Subtype` is already set, and enumerate available subtypes from config when prompting.
5. **Update `gates.ticketTypeFallback`** similarly — list available ticket types in the prompt by reading config; accept any configured slot key.
6. **Update consumer YAML** (`docs/atdd/process/process-flow.yaml` in shop / wherever the second consumer ships theirs) — replace literal `ticket_type == system-api-task` predicates with `structural_subtype == system-api-redesign`, splitting the routing key from the display key. The `classify_ticket` action then writes both `ticket_type` (display, slot-level) and `structural_subtype` (routing, sub-cycle level) into Context.
7. **Update transitions tests** — mirror the YAML literal changes in `internal/atdd/runtime/statemachine/transitions_test.go`.
8. **Document** the schema in `CONTRIBUTING.md` under "Testing the ATDD driver", and add a brief note in the consumer's `docs/atdd/config.yaml` example.

Estimated effort: half a day, ~200 LOC change + tests, plus a coordination commit on the consumer side to land the YAML.

## When to undefer

Trigger conditions, any of:

- A second consumer repo is being scaffolded and its team wants different labels.
- The label rename in `shop` causes friction (e.g. existing tooling, queries, or training material referenced the longer names).
- Someone wants to introduce a new sub-type to the structural cycle without editing `gh-optivem` source.

Until then: the renamed labels in `shop` plus today's hardcoded vocabulary are fine.

## See also

- Parent design plan in the shop repo: `plans/20260429-211522-script-vs-agent-atdd-orchestration.md` (motivation §6 calls out per-consumer customisation as a v2 concern).
- Commits where the partial implementation landed and was reverted on `2026-04-30` — recoverable from git history if you want to start from that point rather than a clean slate.
