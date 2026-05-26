# Process diagram cleanup — labels, layout, duplication, rendering budget

## Origin / intent

Conversation with user (2026-05-26 08:32) walking through observed issues in
`docs/process-diagram.md` (generated from
`internal/atdd/runtime/statemachine/process-flow.yaml` by
`internal/atdd/runtime/diagram/diagram.go`). Six distinct issues were surfaced.
This plan groups them and proposes a direction for each — every item has open
questions for /refine-plan to settle before execution.

## Scope

- Renderer: `internal/atdd/runtime/diagram/diagram.go` — node ordering, label
  format, heading format.
- YAML: `internal/atdd/runtime/statemachine/process-flow.yaml` — structural
  de-duplication of one block, normalisation of six existing `documentation:`
  labels.
- Output artefact: `docs/process-diagram.md` — file layout (single vs split).

Out of scope: the underlying ATDD semantics encoded in the YAML (no process
behaviour changes, just naming + structure where it's strictly duplicate).

## Item 1 — `refactor` process: `CALL_REDESIGN_SYSTEM_STRUCTURE` renders above the gateway

**Observation**: In the rendered SVG (`docs/images/process-diagram-5-refactor.svg`),
the three CALL siblings sit at three different y-coordinates: REDESIGN at y=47
(above the gateway at y=299), the other two at y=551 (below). Expected: all
three siblings at the same rank below the gateway.

**Cause**: Mermaid (Dagre) breaks cycles by reversing edges for layout. The
`refactor` process has three cycles (gateway → CALL → gateway). For two of
them Dagre reverses the back-edge (correct — CALL ends up below). For
REDESIGN it reverses the forward edge (wrong — CALL ends up above). The
choice is heuristic and biased by node declaration order; `diagram.go:213`
sorts nodes alphabetically, putting `CALL_REDESIGN_SYSTEM_STRUCTURE` first,
which pulls it toward rank 0. The comment at `diagram.go:207` ("node
rendering order does not affect Mermaid layout") is wrong in cyclic graphs.

**Direction (recommended)**: change node emission order from alphabetical to
**topological from `process.start`** (BFS), with cycle back-edges visited last.
This biases Dagre to keep loopable siblings at the rank below the gateway and
generalises beyond this one process.

**Files**: `internal/atdd/runtime/diagram/diagram.go` (the `sort.Strings(ids)`
at line 213; the comment at line 204-208).

**Open questions for /refine-plan**:

- Q1.1 — BFS-from-start is the recommendation, but Dagre is heuristic. Are
  we OK if a future YAML graph still produces an off-rank sibling, treating
  this as best-effort? Or do we want a stronger lever (e.g. invisible
  same-rank links between siblings, or wrap loopable siblings in a `subgraph`
  with `direction LR`)?
- Q1.2 — Ungrouped vs grouped nodes: BFS interacts with the existing
  `group:` annotation (slash-delimited subgraphs). Should grouped nodes be
  visited in BFS order *within their subgraph block*, or BFS across the
  whole process ignoring grouping, then partitioned at render time? (Likely
  the latter — the current renderer partitions ungrouped vs grouped after
  collecting; we keep that.)

## Item 2 — `call_activity` label inconsistency (three styles in the YAML)

**Observation**: Of ~90 `call_activity` nodes in `process-flow.yaml`, three
labelling styles coexist:

| Style | Count | Example |
|---|---|---|
| ID fallback (no `documentation:`) | ~85 | `OPP_REFACTOR_SYSTEM_STRUCTURE — see § refactor-system-structure` |
| TDD-stage doc | 4 | `RED — write failing acceptance tests — see § write-and-verify-acceptance-tests-fail` |
| agent-action doc | 3 | `agent-action: implement-system — see § implement-and-verify-system` |

The renderer (`diagram.go:369-393`) is uniform: `label = documentation` if
set, else `label = ID`. The inconsistency is in the YAML — six nodes opted
into `documentation:`, the rest leak their screaming-snake ID.

**Direction (recommended) — minimal renderer change**: drop the ID-fallback
prefix entirely. When no `documentation:` is set, render
`[see § sub-process]`. When `documentation:` is set, render
`[doc — see § sub-process]`. No mass YAML edit — the 6 existing docs stay
where they are (Item 4 below normalises their wording).

**Files**: `internal/atdd/runtime/diagram/diagram.go:383-390` (the
`CallActivity` case in `writeNode`).

**Open questions for /refine-plan**:

- Q2.1 — Is the "drop ID, keep just `[see § sub-process]`" rendering OK
  visually? Alternative: render the bare sub-process name, e.g. just
  `[refactor-system-structure]` (no "see §" prefix), since the heading link
  is the redundancy.
- Q2.2 — Do we want to require `documentation:` on every call_activity
  instead (mass YAML edit, no fallback at all)? Recommendation: no — that's
  ~85 lines of editorial busywork for marginal label gain over what
  Item 2's renderer change already buys.

## Item 3 — Duplicate refactor menu in `change-system-behavior` step 3

**Observation**: The "opportunistic refactor" block in `change-system-behavior`
(lines 423-452) is structurally identical to the `refactor` TOP process
(lines 308-338): same three CALL options (refactor-system-structure /
refactor-test-structure / redesign-system-structure), same gateway binding
(`refactor_type_choice`), same loopback, same `none` exit. Only differences
are cosmetic (`CALL_` vs `OPP_` prefix, slightly different gateway wording)
and the exit target (own end vs change-system-behavior's end). The inline
comment at lines 297-307 claims "Three surfaces, three ceremony levels" but
no actual ceremony difference exists in the YAML.

**Direction (recommended)**: replace lines 423-452 with a single
`call_activity` pointing at the `refactor` process:

```yaml
- id: OPPORTUNISTIC_REFACTOR
  type: call_activity
  process: refactor
  documentation: "Opportunistic refactor (loopable; none = end cycle)"
```

Net: removes one gateway + three CALL nodes + their loopback edges; the
"opportunistic" framing survives as a call-site label. One canonical
refactor menu reused from two callers (TOP entry + `change-system-behavior`).

**Files**: `internal/atdd/runtime/statemachine/process-flow.yaml` (lines
423-452 replaced; comment at 297-307 reworded; statemachine tests if they
reference the removed node IDs).

**Open questions for /refine-plan**:

- Q3.1 — `IMPLEMENT_AND_VERIFY_SYSTEM` (the GREEN step) currently flows
  directly into `GATE_OPPORTUNISTIC_REFACTOR`. With Item 3, it flows into
  `OPPORTUNISTIC_REFACTOR` (the call_activity). Confirm this is fine — no
  param needs to thread through to the menu.
- Q3.2 — `implement-ticket` (the other caller of `refactor`'s sibling
  cycles) currently routes via `GATE_TICKET_KIND` directly to
  `CALL_REFACTOR_SYSTEM_STRUCTURE` etc. (lines 281-294). It does NOT go
  through the `refactor` menu — the kind is already known. After Item 3 we
  have two patterns: ticket-driven routes directly to the chosen cycle;
  opportunistic routes through the menu. Confirm this asymmetry is
  intentional (it reflects different call semantics — the ticket carries the
  kind, the opportunistic site doesn't).
- Q3.3 — Are there tests that exercise the `OPP_*` node IDs that need
  updating in lockstep? (Likely `internal/atdd/runtime/statemachine/*_test.go`
  — check during execution.)

## Item 4 — RED vs GREEN label asymmetry (verb-phrase vs param-dump)

**Observation**:

- `WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL` doc: `"RED — write failing acceptance tests"` (verb-phrase, no agent).
- `IMPLEMENT_AND_VERIFY_SYSTEM` (GREEN call site) doc: `"GREEN — agent-action: implement-system"` (param-dump).

**Cause** — structural asymmetry between the two sub-processes:

| Sub-process | Pattern | Call site needs `params:`? |
|---|---|---|
| `write-and-verify-acceptance-tests-fail` | Wrapper that hard-codes `expected-test-result: failure` | No |
| `implement-and-verify-system` | Template-parameterised on `${agent-action}` | Yes — that's the only thing distinguishing GREEN from REFACTOR call sites |

So RED has nothing to expose at the call site (sub-process is fixed-purpose),
while GREEN must expose `agent-action` to disambiguate from the REFACTOR
call site of the same sub-process.

**Direction (recommended) — surface fix, accept the structural difference**:
normalise the six `documentation:` strings to verb-phrase form. The renderer
already prints `— see § sub-process`, so the wiring is visible without
duplicating the sub-process name in the doc. Param details stay in the YAML
`params:` block but don't appear in the label.

Proposed labels (before → after):

| Where | Current | Proposed |
|---|---|---|
| change-system-behavior RED | `RED — write failing acceptance tests` | unchanged |
| change-system-behavior GREEN | `GREEN — agent-action: implement-system` | `GREEN — implement system` |
| cover-system-behavior Cover | `Cover — write passing acceptance tests` | unchanged |
| (elsewhere, implement-system) | `agent-action: implement-system` | `implement system` |
| (elsewhere, refactor-system) | `agent-action: refactor-system` | `refactor system` |
| inside `implement-and-verify-system` | `agent-action: ${agent-action}` | unchanged (template, not a call-site label) |

**Files**: `internal/atdd/runtime/statemachine/process-flow.yaml` (six
`documentation:` lines).

**Open questions for /refine-plan**:

- Q4.1 — Verb-phrase ("implement system") vs noun-phrase ("system
  implementation") vs verb-particle ("implement-system" matching the
  agent name): pick one and apply uniformly across the six docs.
- Q4.2 — Alternative direction (structural fix, Option 2 from the
  conversation): introduce thin wrappers around `implement-and-verify-system`
  per agent (e.g. `implement-and-verify-system-fresh`,
  `implement-and-verify-system-refactor`), mirroring how
  `write-and-verify-acceptance-tests-fail` wraps `write-and-verify-acceptance-tests`.
  Eliminates the asymmetry at the cost of two new sub-process definitions.
  Recommended only if more `agent-action` variants are anticipated.

## Item 5 — Top-level processes and sub-processes use the same heading depth

**Observation**: Every process renders as `## <name>`. A reader can't tell
which are TOP entry points (`refine-ticket`, `implement-ticket`, `refactor`)
vs CYCLEs (`change-system-behavior`, …) vs HIGH (`write-and-verify-…`) vs
MID (`compile`, `write-acceptance-tests`, …) vs LOW (`approve`,
`execute-agent`). The renderer already knows the level — `diagram.go:63-121`
has `processOrder` grouped by `// TOP`, `// CYCLE`, `// HIGH`, `// MID`,
`// LOW` comments — but it drops this information at render time.

**Direction (recommended) — level prefix in the heading**: same H2 for all,
level shown before the name, format matching the YAML section comments:

```
## TOP — refine-ticket
## CYCLE — change-system-behavior
## HIGH — write-and-verify-acceptance-tests-pass
## MID — write-acceptance-tests
## MID — compile
## LOW — approve
```

Implementation: replace `processOrder []string` with `[]processEntry{name, level}`
so ordering and level come from one source, then format the heading as
`## %s — %s`. The `main` process keeps its `processAlias` ("Runtime
Bootstrap (legacy entry — collapses in Phase D)").

**Files**: `internal/atdd/runtime/diagram/diagram.go` (the `processOrder` var
and `writeProcessSection`).

**Open questions for /refine-plan**:

- Q5.1 — Alternative direction: heading depth by level (H2 TOP, H3 CYCLE, …
  H6 LOW). GitHub TOC auto-indents, but H5/H6 render small and shared
  sub-processes have multiple "parents" so the hierarchy is misleading.
  Recommendation: stick with H2 + prefix.
- Q5.2 — Should the legend at the top of the document also be re-titled
  (e.g. `## Legend` → `## Legend — node shapes and executor colours`)?
  Cosmetic, optional.
- Q5.3 — Sub-MID classification: the YAML splits MID into "agent tasks" and
  "command tasks" (comment in `processOrder`). Do we surface that distinction
  too (`## MID-agent — write-acceptance-tests` / `## MID-command — compile`),
  or stop at one level of "MID"?

## Item 6 — `docs/process-diagram.md` exceeds GitHub's per-page Mermaid render budget

**Observation**: The file has **51 Mermaid blocks in 23KB**. GitHub starts
showing "Unable to render rich display" at `compile-system` (around the
40th block) and every block below it. Mermaid syntax in those blocks is
valid — it's GitHub's per-page rendering quota.

**Direction options**:

- **Option 6A — Split by BPMN level**: emit five (or six) sibling files,
  e.g. `docs/process-diagram-top.md`, `…-cycle.md`, `…-high.md`,
  `…-mid.md`, `…-low.md`, with `docs/process-diagram.md` becoming an index
  page that links to each. Each file's diagram count stays under the
  budget. Generator changes: emit multiple files, link from the index.

- **Option 6B — Split into per-process pages**: every process gets its own
  `docs/process/<name>.md`. Index page lists them. Most pages contain one
  diagram. Maximum granularity; biggest file-count growth.

- **Option 6C — Drop the long-tail processes from the main page**: keep
  TOP + CYCLE + HIGH in `process-diagram.md` (~20 blocks), spill MID + LOW
  to `process-diagram-tasks.md` (~30 blocks). Index links from one to the
  other. Smallest change.

- **Option 6D — Pre-render to SVG, embed instead of Mermaid source**:
  generate SVG per process via `mmdc` (the Mermaid CLI) at generation
  time, embed `<img>` tags in the markdown. The `docs/images/process-diagram-*.svg`
  files already in the repo suggest someone tried this for a subset.
  Removes the GitHub render budget entirely but adds a tooling dependency
  (`mmdc`) and a build step.

**Files**: `internal/atdd/runtime/diagram/diagram.go` (output emission),
`process_commands.go` (CLI flag if we add one to pick output mode), and
the `docs/` files themselves.

**Open questions for /refine-plan**:

- Q6.1 — Pick one of 6A / 6B / 6C / 6D. Recommendation: **6A** (split by
  level) — aligns with Item 5's heading prefix, keeps each file
  conceptually coherent, no new tooling.
- Q6.2 — If 6A: does `gh optivem process show` still emit one combined
  markdown by default and add a `--split` flag, or does it always emit
  multiple files? (Recommendation: always emit multiple — the combined
  output doesn't render on GitHub, so it's a footgun to keep as the
  default.)
- Q6.3 — Do the existing `docs/images/process-diagram-*.svg` files have a
  current purpose, or are they stale from an earlier attempt? Check
  before removing.
- Q6.4 — `docs/process-diagram.md` is referenced from other docs (README?
  ATDD process docs?). Find and update those references after the split.

## Item 7 — Legend wording: executor labels for service / user-task nodes

**Observation**: The legend at the top of `docs/process-diagram.md` (emitted
by `diagram.go::writeLegend`, lines 147-172) currently uses three executor
labels that the user wants revised:

- `[[Service task — Go runtime]]` — "Go runtime" is implementation detail
  leaking into a vocabulary diagram. We want a vocabulary-level word for
  "the engine that runs mechanical, non-LLM, non-human steps".
- `[Agent task — LLM]` — rename to **`User task (LLM agent)`**. Aligns
  with BPMN: in the YAML these are `user_task` nodes; the executor is the
  LLM agent, which is a *kind* of user task.
- `[Human STOP]` — rename to **`User task (Human)`**. Same reasoning:
  YAML `user_task` with `agent: human`; "STOP" connotes a halt signal,
  which isn't what every human task is.

The accompanying bullet text in the legend needs to match:

```
- `[[subroutine]]` — service task — mechanical step run by the Go runtime (white)
- `[rectangle]`  — user task — LLM agent (dark blue) or human STOP (yellow); `call_activity` rectangles are unfilled and link to a sub-process heading
```

**Direction (recommended)**: apply the two confirmed renames in lockstep
(Mermaid sample label + bullet text), and pick one of the candidates below
for the "Go runtime" replacement in /refine-plan.

**Candidates for "Go runtime"** — pick one:

- **`Service task (engine)`** — most BPMN-flavoured; "engine" is the
  generic term for the deterministic executor.
- **`Service task (runtime)`** — keeps "runtime" but drops the language
  specifier. Matches existing internal package name (`internal/atdd/runtime`).
- **`Service task (automated)`** — emphasises that no human or LLM
  judgement is involved; consistent with how BPMN diagrams typically
  distinguish "automated service task" from "manual task".
- **`Service task (deterministic)`** — emphasises the contract: same
  input ⇒ same output, no model variance. Most precise but jargon-heavy.

Bullet text would update to something like (depends on choice above):

```
- `[[subroutine]]` — service task — mechanical step run by the engine (white)
- `[rectangle]`  — user task — LLM agent (dark blue) or human (yellow);
  `call_activity` rectangles are unfilled and link to a sub-process heading
```

**Files**: `internal/atdd/runtime/diagram/diagram.go::writeLegend` (lines
147-172). No YAML changes — the YAML uses `service_task` / `user_task` /
`agent: human` already, which match the proposed legend vocabulary.

**Open questions for /refine-plan**:

- Q7.1 — Pick the "Go runtime" replacement from the four candidates above.
- Q7.2 — `writeExecutorStyling` comment block (lines 406-416) mentions
  "(Go runtime)" alongside the `serviceNode` classDef. Re-word in lockstep
  with Q7.1.
- Q7.3 — Does the term used in the legend also need to flow into the
  ATDD process docs (`docs/atdd/process/*.md`)? Check for "Go runtime"
  references during execution.

## Item 8 — `main` process heading: "Runtime Bootstrap (legacy entry — collapses in Phase D)"

**Observation**: `diagram.go:39-41` defines a single `processAlias` entry:

```go
var processAlias = map[string]string{
    "main": "Runtime Bootstrap (legacy entry — collapses in Phase D)",
}
```

The `main` process is the YAML entry point invoked by `gh optivem implement` —
it picks a top-READY ticket (board mode) or accepts a pre-picked one
(specific_issue mode) and delegates to `implement-ticket`. The Phase D plan
(see process-flow.yaml lines 14-18, 131) **removes `main` from the YAML
entirely** by moving the picker into Go driver code.

So "legacy" here doesn't mean "old/deprecated tech" — it means "transitional
plumbing on its way out". The parenthetical is essentially a TODO in the
heading.

**Tension with existing memory rules**:

- `feedback_legacy_tests_no_marker.md`: "Legacy tests must look identical to
  AT/CT tests — no `@LegacyCoverage` annotation, no `*_LegacyTest` suffix".
- `feedback_teaching_repo_no_legacy.md`: "Teaching repo — no legacy-alias
  machinery for schema moves".

Both rules push back on **permanent** "legacy" markers in user-facing
artefacts. The current heading is the same shape: an inline "going-away"
warning that stays in the diagram until Phase D actually ships.

**Direction options**:

- **Option 8A — Drop the alias entirely.** `main` renders as `## RUNTIME — main`
  (or `## main` with Item 5's prefix). The Phase-D-collapse note moves to the
  YAML comment block above the `main` definition (where it already lives, lines
  122-132) and out of the rendered heading. Cleanest; consistent with the two
  memory rules. After Phase D ships, no follow-up needed — the YAML simply
  loses the `main` block.

- **Option 8B — Keep an alias but drop "(legacy entry — collapses in Phase D)".**
  Render as `## Runtime Bootstrap` (no parenthetical). Reader still sees a
  human-readable name; the going-away framing is silent. Cosmetic but keeps the
  one alias in `processAlias`.

- **Option 8C — Restructure to introduce a "RUNTIME" level.** Treat `main` as a
  sixth BPMN-shaped level (RUNTIME, above TOP). Item 5's prefix would render
  `## RUNTIME — main`. Aligns with the existing YAML comment vocabulary
  ("Runtime bootstrap"); no special-case alias. Phase D removes the only
  RUNTIME-level entry; the level disappears with it.

- **Option 8D — Status quo.** Keep "Runtime Bootstrap (legacy entry —
  collapses in Phase D)" until Phase D ships, then delete. Conflicts with the
  memory rules above.

**Recommended**: **Option 8C** (RUNTIME level). The `main` process is genuinely
a different level — it's the CLI bridge, not a domain BPMN unit. Naming the
level surfaces that distinction in vocabulary that survives Phase D's
restructuring (the level just empties out).

**Files**: `internal/atdd/runtime/diagram/diagram.go` (`processAlias` map,
the `processOrder` from Item 5 if 8C is picked — `main` gets level
"RUNTIME"). No YAML changes (the comment block already explains the role).

**Open questions for /refine-plan**:

- Q8.1 — Pick from 8A / 8B / 8C / 8D.
- Q8.2 — If 8C: do we want the RUNTIME level shown in the legend / level
  enumeration anywhere else (e.g. process-flow.yaml comments at lines 7-100)?
- Q8.3 — Is Phase D documented elsewhere (e.g. an existing
  `plans/*.md`) that should be cross-referenced from this plan? Quick search
  during execution.

## Item 9 — `update-ticket` sub-process name is too generic

**Observation**: `update-ticket` (process-flow.yaml:1390-1403) does exactly
one thing — change a ticket's lifecycle state via a single agent call:

```yaml
update-ticket:
  start: EXECUTE_AGENT
  nodes:
    - id: EXECUTE_AGENT
      type: call_activity
      process: execute-agent
      params:
        task-name: update-ticket           # ← hardcoded literal
  …
```

It is called 4 times, every call site named `MARK_*`:

| Call site | `target-state:` |
|---|---|
| MARK_IN_REFINEMENT | "IN REFINEMENT" |
| MARK_READY | "READY" |
| MARK_IN_PROGRESS | "IN PROGRESS" |
| MARK_IN_ACCEPTANCE | "IN ACCEPTANCE" |

The `target-state` param is the only input. "Update" is generic — it could
mean editing the body, labels, assignee, etc. The operator's mental model
(per the `MARK_*` node IDs) is **state transition**, not "update".

**Direction options** — pick one:

- **Option 9A — `mark-ticket`**: matches the operator vocabulary already
  encoded in the `MARK_*` call-site IDs. Most natural reading: "MARK_IN_REFINEMENT
  calls mark-ticket with state IN REFINEMENT". Slight informality.

- **Option 9B — `update-ticket-status`**: most explicit; pairs with the
  `target-state` param semantically (state = status). Verbose.

- **Option 9C — `move-ticket`**: Kanban / board vocabulary. Strong domain
  match if the team thinks in terms of board columns ("move ticket to READY").

- **Option 9D — `transition-ticket`**: state-machine vocabulary. Pairs with
  the fact that states are an enumerated lifecycle.

- **Option 9E — `set-ticket-state`**: declarative; matches the param name
  `target-state` literally. The `set-X` verb is unambiguous.

- **Option 9F — keep `update-ticket`**: status quo. Generic but unblocked.

**Recommended**: **Option 9A (`mark-ticket`)** — aligns the sub-process name
with the call-site IDs that already exist, so reading the YAML is uniform
("MARK_IN_REFINEMENT calls mark-ticket"). The other options are formally
correct but disconnect from how callers already refer to it.

**Files**:
- `internal/atdd/runtime/statemachine/process-flow.yaml`: rename the process
  definition (line 1390), all 4 call sites (lines 172, 182, 237, 272), and
  the YAML section comment (line 1388).
- The agent's hardcoded `task-name: update-ticket` (line 1397) — rename in
  lockstep? See Q9.2 below.
- `internal/atdd/runtime/diagram/diagram.go` `processOrder` list (one entry,
  currently `"update-ticket"`).
- Any test fixtures referencing `update-ticket`.

**Open questions for /refine-plan**:

- Q9.1 — Pick from 9A–9F.
- Q9.2 — The underlying agent task-name (line 1397, hardcoded literal
  `update-ticket`) — rename in lockstep with the sub-process? That implies
  renaming the agent's prompt file too (one extra moving part). Or keep
  the agent's task-name decoupled and leave it as `update-ticket`?
- Q9.3 — Are there other generic-named MID processes worth re-examining
  in the same pass (e.g. `commit`, `compile`, `run-tests`, `approve`)? Or
  is this strictly about `update-ticket` and we treat the others as
  already-fine?

## Execution order

Items in execution order, picking the recommended direction at each step
(may change after /refine-plan):

1. **Item 5** (heading prefix) — pure renderer change, isolated, easy to
   review. Bakes in the level vocabulary used by other items.
2. **Item 7** (legend wording) — pure renderer change, isolated. Bakes in
   the executor vocabulary used in the legend.
3. **Item 2** (drop ID-fallback in call_activity label) — pure renderer
   change.
4. **Item 1** (BFS node emission order) — pure renderer change.
5. **Item 8** (drop the `main` legacy-alias, optionally introduce RUNTIME
   level) — `processAlias` map + maybe `processOrder` level vocabulary.
6. **Item 4** (normalise 6 documentation strings) — pure YAML change.
7. **Item 9** (rename `update-ticket`) — YAML rename across 4 call sites
   + process definition + section comment + diagram.go `processOrder`
   + possibly the agent task-name + agent prompt file.
8. **Item 3** (de-duplicate opportunistic-refactor block) — YAML change
   + test updates if any reference the removed node IDs.
9. **Item 6** (split output) — biggest change, depends on Item 5's level
   vocabulary; do last so the level-prefix pattern is settled first.

Regenerate `docs/process-diagram.md` (and any new sibling files) after each
item; commit per item; verify on github.com that the rendering issue is
gone after Item 6.

## Tests / verification

- `internal/atdd/runtime/diagram/diagram_test.go` (if it exists) — likely
  to need updates for Items 1, 2, 5, 6.
- `internal/atdd/runtime/statemachine/*_test.go` — node-ID references for
  Item 3.
- Manual: after regenerating, scroll through the new `docs/process-diagram*.md`
  files on github.com and confirm every Mermaid block renders.

## Out of scope (explicit non-goals)

- Changing any process semantics (which agent runs, what gateways branch on,
  which outputs a process produces). The de-duplication in Item 3 preserves
  semantics — same gateway binding, same options.
- Renaming the BPMN level labels (TOP / CYCLE / HIGH / MID / LOW). The
  vocabulary is already in the YAML comments; this plan surfaces it.
- The `processAlias` map (the only entry, `main`, is left alone).
- The legend block at the top of `process-diagram.md`.
