# Ticket-type classification: rename `story`, configurability, label fallback

> ⏳ **Deferred (2026-05-15): quick-fix alias is sufficient for now.**
> The `ticketTypeAliases` map in `internal/atdd/runtime/actions/bindings.go`
> normalizes GitHub's current type name (`"feature"`) → internal `"story"`
> at the boundary. Since GitHub's native type names change rarely
> (last rename: Story → Feature in 2026), the alias is good enough —
> when GitHub renames again, we add another entry. None of the three
> strategic questions below (rename end-to-end, configurability, label
> fallback) are blocking any current work; revisit if/when a second
> GitHub rename, a non-GitHub backend, or external adopters create
> real pressure.

> ⚠️ **Draft — discussion needed before implementation.**
> Three intertwined questions are on the table (rename, configurability,
> label fallback). The "final state" section is a strawman, not a
> decision. Sign-off required on each open question below before any
> code lands.

> 📜 **History note:** GitHub renamed the native "Story" issue type to
> "Feature" in 2026. We hit the conflict on optivem/shop#65 — the
> orchestrator's `read_ticket_type` action rejected `"feature"` as an
> unsupported issue type. A narrow alias was shipped (see "Quick-fix
> shipped" below) so the pipeline accepts Feature today; this plan is
> the proper follow-up.

## Quick-fix shipped (context, not part of this plan)

`internal/atdd/runtime/actions/bindings.go` now contains a
`ticketTypeAliases` map that normalizes `"feature"` → `"story"` at the
boundary inside `readTicketType`. The rest of the pipeline
(`parse.go`, `deriveChangeType`, `process-flow.yaml`, gates, tests)
continues to speak `"story"` unchanged. Error messages and prompts now
read "Feature / Bug / Task". One new test case covers the alias.

This is deliberately tactical — it unblocks `shop#65` without rippling
through ~15 files. The work below is the strategic follow-up.

## The three questions

The questions are sequenced because the answers depend on each other:

1. **Rename `story` → `feature` end-to-end?** If yes, the alias goes
   away and the pipeline's vocabulary matches GitHub's. If no, the
   alias is permanent.
2. **Should ticket-type recognition be user-configurable?** If yes,
   what's the config shape (per-project YAML? a `gh-optivem.yaml`
   field? environment variables?), and who's the audience (academy
   students, or external adopters)?
3. **Should we use labels instead of (or as a fallback for) native
   issue type?** GitHub's native types are a moving target;
   `subtype:*` labels are already part of our intake protocol. Should
   ticket-type classification take the same shape?

Each question has its own section below.

## Question 1 — Rename `story` → `feature`

### Strawman: yes, rename

Replace `"story"` with `"feature"` in:

- `internal/atdd/runtime/actions/bindings.go` — `supportedTicketTypes`,
  `deriveChangeType` case, doc comments. Delete `ticketTypeAliases`.
- `internal/atdd/runtime/gates/bindings.go` — prompt text, switch case,
  doc comment.
- `internal/atdd/runtime/intake/parse.go` — `case "story"` and the
  unsupported-type error message.
- `internal/atdd/runtime/statemachine/process-flow.yaml` — the
  `ticket_type == story` gate predicate.
- Tests: `actions/bindings_test.go`, `gates/bindings_test.go`,
  `intake/parse_test.go`, `statemachine/{behavioral_cycle,
  structural_cycle, transitions}_test.go`, `trace/trace_test.go`,
  `diagram/diagram_test.go`, `tracker/github/github_test.go`.
- Process docs: `internal/assets/global/docs/atdd/process/cycles.md`,
  `glossary.md`, the process diagram.
- The `atdd-story` agent name (or its descendant) wherever it survives
  in `internal/assets/`.

Estimated scope: ~15-20 files, mostly mechanical, but every test
fixture moves.

### Trade-offs

| | Rename | Keep alias |
|---|---|---|
| Vocabulary matches GitHub | ✅ | ❌ |
| Survives a future GitHub rename ("Feature" → "Capability"?) | ❌ (rename again) | ❌ (need new alias) |
| Touches one file | ❌ | ✅ |
| Process docs / diagrams stay current | ✅ if updated, ❌ otherwise | ❌ (docs say "story" but UI says "Feature") |
| Vulnerable to vocab drift between code and GitHub | ❌ | ✅ |

### Open

- **OQ1.1:** Rename, or keep the alias permanent? Recommendation:
  rename, because the docs/diagrams already confuse new contributors
  (process diagram says "story", GitHub UI says "Feature").
- **OQ1.2:** If we rename, do we also rename `ticket_type` context key
  values stored in trace files? That's a state-file format change —
  old traces with `ticket_type=story` would mismatch. Probably fine
  (traces are ephemeral), but worth confirming.

## Question 2 — Configurability

### The risk being managed

GitHub has now renamed `Story` → `Feature` once. They could rename
again, add new types ("Spike", "Epic", "Initiative"), or remove types.
Today, every such change is a code change to gh-optivem.

### Strawman: per-project `ticket_types` config in `gh-optivem.yaml`

```yaml
# gh-optivem.yaml (per project)
ticket_types:
  feature: behavioral    # GitHub type name → internal change-type contribution
  bug: behavioral
  task: structural       # actual change_type derived from subtype:* label
```

With this:

- The hard-coded `supportedTicketTypes` map becomes config-driven.
- `deriveChangeType`'s `case "story", "bug"` becomes a config lookup
  for the `behavioral` mapping.
- When GitHub renames again, the operator edits `gh-optivem.yaml` and
  re-runs — no gh-optivem release needed.
- The shipped default config covers `feature / bug / task`, matching
  GitHub's current names.

### Trade-offs

| | Configurable | Hard-coded |
|---|---|---|
| Survives GitHub renames without a gh-optivem release | ✅ | ❌ |
| Academy students can adapt to their org's flavor | ✅ | ❌ |
| Risk of mis-config breaking the pipeline at runtime | ⚠️ (validate in `doctor`) | ✅ |
| Complexity to test (every combination valid) | ❌ | ✅ |
| Encourages forks where students invent their own taxonomy | ⚠️ | ✅ (uniformity) |

### Open

- **OQ2.1:** Is the audience external adopters, or academy students
  inside optivem? If only optivem-internal, hard-coded + occasional
  release may be cheaper than config + validation. If we expect
  external forks, config matters more.
- **OQ2.2:** Should the config live in `gh-optivem.yaml` (per-project,
  matches the existing pattern) or in a global `~/.config/gh-optivem/`
  file? Probably per-project, but worth confirming — students cloning
  the scaffolded shop repo would otherwise inherit whatever ships in
  the template.
- **OQ2.3:** What happens if a ticket has a type *not* in the config?
  STOP_CLASSIFY_CONFLICT with a "add it to gh-optivem.yaml or change
  the type" message, presumably.
- **OQ2.4:** Does `gh optivem doctor` validate the `ticket_types` map
  against the live GitHub repo's available native types? (Requires a
  GraphQL call against `repository.issueTypes`.)

## Question 3 — Labels instead of (or as a fallback for) native type

### The user's prompt

> "or should we be using labels instead — add that as discussion to the
> plan — and having fallbacks, e.g. it looks at ticket type, or label"

### Why this is interesting

- **Native types are GitHub-only.** A future markdown-tracker user (the
  `internal/atdd/runtime/tracker/markdown/` backend) has no native
  type — it already falls back to filename heuristics + frontmatter.
- **Native types are non-portable.** A migration to GitLab/Jira would
  re-open this question. Labels are universal.
- **Native types are unstable.** As 2026's rename demonstrated.
- **`subtype:*` labels are already in our protocol** — we already read
  labels for task subtype disambiguation in `readSubtype`. The
  infrastructure exists.

### Strawman: `kind:*` label as primary, native type as fallback

```
Classification priority:
  1. `kind:<value>` label on the issue (e.g. `kind:feature`)
  2. Native GitHub issue type (resolved via current GraphQL path)
  3. STOP_CLASSIFY_CONFLICT (operator must declare)
```

Or inverted:

```
  1. Native GitHub issue type (if set)
  2. `kind:<value>` label as fallback
  3. STOP_CLASSIFY_CONFLICT
```

Either way: labels are recognized, but neither replaces the other.
Conflict (both present and disagree) routes to STOP with an explicit
"reconcile" message.

### Trade-offs

| | Labels primary | Native-type primary | Both (with priority) |
|---|---|---|---|
| Portable across trackers | ✅ | ❌ | ✅ (labels exist everywhere) |
| Survives GitHub renames | ✅ | ❌ | ✅ (labels are operator-controlled) |
| Matches GitHub's "first-class" UX (type picker in Issue Form) | ❌ | ✅ | ✅ |
| Discoverable for students (one obvious place to set type) | ❌ (label syntax) | ✅ (dropdown in UI) | ⚠️ (two places to look) |
| Implementation cost | low (extends `readSubtype` pattern) | already done | medium (two paths + conflict handling) |
| Risk of label/type drift on the same issue | n/a | n/a | ⚠️ (must STOP on conflict) |

### Open

- **OQ3.1:** Labels primary, native-type primary, or both with a
  priority rule? Recommendation: native-type primary, label as
  fallback, STOP on conflict. Rationale: GitHub's type picker is the
  most-discoverable place for an academy student; labels are the
  escape hatch for non-GitHub backends and for orgs that disable type
  setting.
- **OQ3.2:** What label namespace? `kind:*` parallels `subtype:*`.
  `type:*` is more natural but collides with the Issue Form's `type:`
  field name in our heads — avoid. **Recommendation: `kind:*`.**
- **OQ3.3:** Does the markdown tracker (`tracker/markdown/markdown.go`,
  already handles frontmatter `type:` + filename heuristic) need to
  align? Today it has a richer set (`feature, bug, story, task, chore,
  techdebt`) than the GitHub tracker. The mapping should be unified.
- **OQ3.4:** Interaction with Question 2 — if labels become primary,
  does `ticket_types` config become "label → internal kind" instead of
  "native type → internal kind"? Probably yes, and the config example
  in Question 2 would change shape.

## Proposed phases (depends on the answers above)

Sequence depends on which questions get a "yes":

**Phase A — Rename (if OQ1.1 = rename):**
Mechanical replacement of `story` → `feature` across the ~15-20 files
listed in Question 1. Delete the `ticketTypeAliases` map. Single PR,
all-or-nothing.

**Phase B — Label fallback (if OQ3.1 != native-type-only):**
Extend `Tracker.Classify` interface to optionally read a `kind:*`
label; or add a second action `read_ticket_type_label` and route via
a gate. Update both `github` and `markdown` tracker backends. STOP on
conflict between label and native type.

**Phase C — Configurable mapping (if OQ2.1 = yes):**
Add `ticket_types:` to `gh-optivem.yaml` schema. Replace hard-coded
`supportedTicketTypes` + `deriveChangeType` switch with config
lookup. `gh optivem doctor` validates against the live GitHub repo's
available types. Ship a default that covers `feature / bug / task`.

Phases A and B are independent — either order works. Phase C depends
on whichever of A/B lands last (the config schema needs to know
whether it's keyed on native type names or label values).

## Rejected alternatives (so we don't re-litigate)

- **LLM-based classification fallback.** The pipeline deliberately
  removed LLM intake agents (`atdd-story / atdd-bug / atdd-task /
  atdd-chore`) in favor of deterministic markdown parsing. Bringing an
  LLM back to guess ticket type would undo that work. Native type +
  labels are both deterministic and operator-controlled.
- **Silent aliasing without operator action.** Accepting any unknown
  type as "story" or "task" would mask GitHub schema changes instead
  of surfacing them. The STOP-on-unknown contract is correct.
- **Per-issue override flag in the ticket body.** Adds a third source
  of truth and a parse step. The Issue Form's type picker + a `kind:*`
  label already covers this.

## Cross-references

- Quick-fix commit / lines: `internal/atdd/runtime/actions/bindings.go`
  `ticketTypeAliases` map.
- Related plan: none currently. If "configurable" wins, this will
  intersect with whatever ships per-project `gh-optivem.yaml` schema
  extensions next.
- Related memory:
  `[[feedback_naming_github_jira_first]]` — name multi-backend
  abstractions for the real backends (GitHub + Jira), not for
  markdown. The label-vs-type discussion above honors this — the
  label fallback exists because markdown/non-GitHub backends need it,
  not because markdown should dictate vocabulary.
