# Process diagram cleanup — labels, layout, duplication, rendering budget

## Origin / intent

Conversation with user (2026-05-26 08:32) walking through observed issues in
`docs/process-diagram.md` (generated from
`internal/atdd/runtime/statemachine/process-flow.yaml` by
`internal/atdd/runtime/diagram/diagram.go`). Eleven distinct issues were
surfaced (six initial + five added during the refine walk: Item 10
`refine-backlog-item` rename, Item 11 ticket-kind gateway split, plus three
items already in the original draft).
This plan groups them and proposes a direction for each. The 2026-05-26
/refine-plan walk settled directions for Items 2, 3, 7, 8, 9, 10, 11
(in-scope) and deferred Items 1, 5, 6 (out-of-scope this pass). Item 4
merged into Item 2.

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

> ⏳ **Deferred** (2026-05-26): not worth the renderer churn right now — diagram is still readable once you know which sibling is which. Revisit if more cyclic processes show the same off-rank pattern.

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

## Item 2 — `call_activity` label inconsistency: require `documentation:` everywhere

**Observation**: Of ~90 `call_activity` nodes in `process-flow.yaml`, three
labelling styles coexist today:

| Style | Count | Example |
|---|---|---|
| ID fallback (no `documentation:`) | ~85 | `OPP_REFACTOR_SYSTEM_STRUCTURE — see § refactor-system-structure` |
| TDD-stage doc | 4 | `RED — write failing acceptance tests — see § write-and-verify-acceptance-tests-fail` |
| agent-action doc | 3 | `agent-action: implement-system — see § implement-and-verify-system` |

The renderer (`diagram.go:369-393`) is uniform: `label = documentation` if
set, else `label = ID`. The inconsistency is in the YAML — six nodes opted
into `documentation:`, the rest leak their screaming-snake ID.

**Direction (decided 2026-05-26): require `documentation:` on every
call_activity node**. Mass-edit the YAML to add a human-readable
`documentation:` line to all ~85 call_activity nodes that currently lack
one. Drop the ID fallback in the renderer entirely. The point: an operator
reading the diagram should not see screaming-snake YAML node IDs anywhere.

**Renderer change** (`diagram.go:383-390`, the `CallActivity` case in
`writeNode`):

- If `documentation:` is set and is different from the sub-process name →
  render `[doc — see § sub-process]`.
- If `documentation:` is set and is equal to the sub-process name → render
  `[doc]` (drop the redundant "see §" suffix).
- If `documentation:` is missing → the YAML is incomplete; emit a
  schema-validation error at parse time (catches new call_activity nodes
  that forget the doc field).

**Labelling convention to apply across the ~85 nodes** — TBD in Q2.1 below,
but candidates:

- **Verb-phrase action**: what the call site is *doing*. E.g.,
  `MARK_IN_REFINEMENT` → `"mark ticket IN REFINEMENT"`,
  `IMPLEMENT_TEST_LAYER` (called 3× for AT/CT/DSL) →
  `"implement acceptance-test layer"` / `"implement contract-test layer"` /
  `"implement DSL layer"`, `OPP_REFACTOR_SYSTEM_STRUCTURE` →
  `"opportunistic refactor: system structure"`.
- **Stage + role**: pairs nicely with the existing RED/GREEN/Cover labels.
  E.g., `OPP_*` → `"REFACTOR — system structure"`. Tighter but only fits
  where there's an established stage vocabulary.
- **Bare sub-process name**: e.g. `"refactor-system-structure"`. Mechanical,
  cheap, but adds no info beyond the `process:` field — defeats the point
  of requiring docs.

**Files**:

- `internal/atdd/runtime/statemachine/process-flow.yaml` — add
  `documentation:` to ~85 nodes; normalise the 6 existing (overlaps with
  Item 4's surface-level normalisation, which folds into Item 2 now).
- `internal/atdd/runtime/diagram/diagram.go:383-390` — drop the ID-fallback
  branch in the `CallActivity` case; add the "label == sub-process name →
  collapse the `see §`" rule.
- `internal/atdd/runtime/statemachine/load.go` (or wherever the schema
  validation lives) — require `documentation:` on `call_activity` nodes;
  emit a parse-time error otherwise.
- Tests under `internal/atdd/runtime/statemachine/*_test.go` that build
  call_activity nodes in fixtures.

**Open questions for /refine-plan**:

- Q2.1 — **Labelling convention**: verb-phrase, stage+role, or bare
  sub-process name (or a mix — verb-phrase by default, stage+role where a
  stage applies)? Settle this *before* the YAML mass edit so the wording is
  uniform.
- Q2.2 — **Schema validation**: hard-error at parse time on missing
  `documentation:`, or warn-and-fallback (e.g. render as just
  `[sub-process-name]`)? Hard-error is the only way to keep the invariant
  alive once Phase D and other plans land new nodes.
- Q2.3 — **Folding Item 4**: Item 4 (RED/GREEN/agent-action normalisation)
  becomes a subset of Item 2. Confirm Item 4 collapses into Item 2 (one
  YAML pass instead of two).
- Q2.4 — **Naming inside parameterised sub-processes**: the call site
  `CALL_AGENT_ACTION` inside `implement-and-verify-system` currently has
  doc `"agent-action: ${agent-action}"` — a template literal that
  resolves at call time. Keep the template form, or change to something
  like `"run the configured agent action"`?

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

## Item 4 — RED vs GREEN label asymmetry — *merged into Item 2 on 2026-05-26*

The six pre-existing `documentation:` strings (RED, GREEN, Cover, two
`agent-action:` variants, one template) get normalised in the same YAML
pass as the other ~85 nodes, under whatever convention Q2.1 settles on.
No standalone work item.

Open questions transferred to Item 2:

- Q4.1 (verb-phrase vs noun-phrase vs verb-particle) → folded into Q2.1
  (labelling convention).
- Q4.2 (structural fix via wrapper sub-processes around
  `implement-and-verify-system`) — **dropped**. We accept the
  wrapper/template asymmetry between the two sub-process patterns;
  surface-level label normalisation is enough.

## Item 5 — Top-level processes and sub-processes use the same heading depth

> ⏳ **Deferred** (2026-05-26): blocked on Item 6's file-shape decision. If
> Item 6 splits per BPMN level, the level prefix is partly redundant (the
> filename carries it) but still useful for cross-file anchor links; if
> Item 6 keeps one file (via SVG pre-render or otherwise), the prefix is
> the primary distinguisher. Settle Item 6 first, revisit.

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

## Item 6 — `docs/process-diagram.md` exceeds GitHub's per-page Mermaid render budget — *design space captured, deferred*

> ⏳ **Deferred** (2026-05-26): brainstorm captured below. Decision postponed
> until the audience question and tour-vs-reference question are settled.
> The render-budget bug remains real (blocks past ~40 fail to render on
> github.com) but is not blocking other items in this plan.

**Observation**: The file has **51 Mermaid blocks in 23KB**. GitHub starts
showing "Unable to render rich display" at `compile-system` (around the
40th block) and every block below it. Mermaid syntax in those blocks is
valid — it's GitHub's per-page rendering quota.

### Design space (brainstorm)

Two orthogonal questions sit behind this decision: **(A) what's the file
shape** (one file vs many; how to split) and **(B) what's the reading
purpose** (reference lookup vs teaching tour vs both).

**(A) — File shape candidates**

- **(a) One big file with level prefixes** — the lightest-touch option;
  fails today because GitHub's per-page budget caps render at ~40
  diagrams. Only viable in combination with SVG pre-render (Option 6D
  below).

- **(b) Five files by BPMN level** — `process-top.md`, `process-cycle.md`,
  `process-high.md`, `process-mid.md`, `process-low.md` + an `index.md`.
  ~10 diagrams per file, all under the budget. Reader pattern: *"I want
  to see all the CYCLEs"* → open one file. Generator-friendly. Heading
  prefix from Item 5 becomes partly redundant (filename carries the
  level) but stays useful for cross-file anchor links.

- **(c) Per top-level entry walkthroughs** — `process-implement-story.md`,
  `process-refactor.md`, `process-refine-ticket.md`, etc. Each file
  walks one TOP cycle end-to-end. Reader pattern: *"what happens when a
  student picks up a story?"* Trade-off: shared HIGH/MID/LOW
  sub-processes either get duplicated across files or live in a separate
  reference file and get linked. Usually ends up as (c) + (b) combined.

- **(d) One file per process (~50 files)** — `process/<name>.md` plus
  an index. Maximum granularity. Cleanest separation; biggest file-count
  growth; index becomes critical.

- **(e) Hybrid: walkthrough + reference** — one or two "tour" files
  (c-style narrative) + a reference shaped as (b) or (d). Highest
  editorial quality, biggest generator/maintenance burden.

**(B) — Audience and reading purpose**

The audience for this repo is **mixed**: students learning ATDD top-down
*and* operators/maintainers looking specific processes up.

- **Reference half** (lookup-friendly) is generator-friendly. Whatever
  file shape from (A) is chosen above, the generator emits it from the
  YAML; drift-free.

- **Tour half** (teaching narrative) is editorial. Two sub-options:
  - **Hand-curated tour files** — best narrative quality; drift risk when
    YAML changes; someone has to remember to update them.
  - **Generated walkthroughs** — DFS through the call graph, one heading
    per process visited; always in sync; reads mechanically (a flat dump
    of the call graph, not a tour).
  - **Defer the tour entirely** — ship the reference first; figure out
    the tour later once there's student feedback on what's confusing.

### Independent fix path: pre-rendered SVG

- **Option 6D — Pre-render to SVG, embed instead of Mermaid source**:
  generate SVG per process via `mmdc` (the Mermaid CLI) at generation
  time, embed `<img>` tags in the markdown. The
  `docs/images/process-diagram-*.svg` files already in the repo suggest
  someone tried this for a subset. Removes the GitHub render budget
  entirely; works with any file shape from (A) above. Cost: adds an
  `mmdc` tooling dependency and a build step.

### Files (will be revisited after deferral lifts)

- `internal/atdd/runtime/diagram/diagram.go` — output emission (single vs
  multi-file; mermaid vs SVG embed).
- `process_commands.go` — CLI surface, possibly a `--shape` or `--split`
  flag.
- `docs/process-diagram.md` and any new sibling files.
- The existing `docs/images/process-diagram-*.svg` — confirm current
  purpose or remove as stale.
- README + ATDD process docs that link to `docs/process-diagram.md` —
  update references in lockstep.

### Open questions still on the table

- Q6.1 — Pick a file shape from (a)–(e) above.
- Q6.2 — Tour: defer / hand-curated / generated walkthroughs?
- Q6.3 — Pre-render to SVG (6D) or keep Mermaid source?
- Q6.4 — If multi-file: does `gh optivem process show` always emit
  multiple files, or keep a single-file mode behind a flag?
- Q6.5 — Existing `docs/images/process-diagram-*.svg` — current purpose
  or stale?
- Q6.6 — README and other docs that link to `docs/process-diagram.md` —
  inventory before split.

## Item 7 — Legend wording: executor labels for service / user-task nodes

**Observation**: The legend at the top of `docs/process-diagram.md` (emitted
by `diagram.go::writeLegend`, lines 147-172) currently uses three executor
labels that the user wants revised:

- `[[Service task — Go runtime]]` — "Go runtime" is implementation detail
  leaking into a vocabulary diagram. We want a vocabulary-level word for
  "the engine that runs mechanical, non-LLM, non-human steps".
- `[Agent task — LLM]` — rename to **`User Task (LLM Agent)`**. Aligns
  with BPMN: in the YAML these are `user_task` nodes; the executor is the
  LLM agent, which is a *kind* of user task.
- `[Human STOP]` — rename to **`User Task (Human)`**. Same reasoning:
  YAML `user_task` with `agent: human`; "STOP" connotes a halt signal,
  which isn't what every human task is.

The accompanying bullet text in the legend needs to match:

```
- `[[subroutine]]` — service task — mechanical step run by the Go runtime (white)
- `[rectangle]`  — user task — LLM agent (dark blue) or human STOP (yellow); `call_activity` rectangles are unfilled and link to a sub-process heading
```

**Direction (decided 2026-05-26)**: apply the three renames in lockstep
across the legend's Mermaid sample labels, the bullet description text,
and the `writeExecutorStyling` doc-comment block (`diagram.go:406-416`).

Final wording:

| Mermaid sample (was) | Mermaid sample (becomes) |
|---|---|
| `SVC[[Service task — Go runtime]]` | `SVC[[Service Task (Automated)]]` |
| `AGT[Agent task — LLM]` | `AGT[User Task (LLM Agent)]` |
| `HUM[Human STOP]` | `HUM[User Task (Human)]` |

Bullet description text (`diagram.go:147-172`) updates in lockstep:

- `[[subroutine]]` — service task — mechanical, automated step (white)
- `[rectangle]` — user task — LLM agent (dark blue) or human (yellow);
  `call_activity` rectangles are unfilled and link to a sub-process heading

**Files**:
- `internal/atdd/runtime/diagram/diagram.go::writeLegend` (lines 147-172) —
  three Mermaid sample labels + the bullet text.
- `internal/atdd/runtime/diagram/diagram.go::writeExecutorStyling` doc
  comment (lines 406-416) — "(Go runtime)" → "(Automated)" in lockstep.

No YAML changes — the YAML uses `service_task` / `user_task` /
`agent: human` already, which match the proposed legend vocabulary.

**Remaining check at execution time**:

- Q7.3 — Does the term "Go runtime" or "Agent task" appear in the ATDD
  process docs (`docs/atdd/process/*.md`)? Grep and update in lockstep.

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

**Direction (decided 2026-05-26): Option 8A — drop the alias entirely.**
Delete the `processAlias` entry for `main`. Heading becomes `## main`. The
Phase-D-collapse note already lives in the YAML comment block above the
`main` definition (process-flow.yaml:122-132) and doesn't need to be
duplicated in the rendered heading.

Ships independently of Item 5 (which is deferred). If Item 5 ever lands,
the level prefix gets added automatically.

**Files**:
- `internal/atdd/runtime/diagram/diagram.go:39-41` — delete the
  `processAlias["main"]` entry. The map can stay (it might gain other
  entries later) or be removed entirely if `main` is the only consumer —
  decide at execution time.

**Remaining check at execution time**:

- Q8.3 — Is Phase D documented elsewhere (e.g. an existing
  `plans/*.md`) that should be cross-referenced from this plan? Quick
  grep during execution.

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

**Direction (decided 2026-05-26): Option 9A — rename to `mark-ticket`.**
Aligns the sub-process name with the `MARK_*` call-site IDs already in the
YAML; reading is uniform ("MARK_IN_REFINEMENT calls mark-ticket with state
IN REFINEMENT").

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

- Q9.1 — *Decided 2026-05-26*: Option 9A (`mark-ticket`).
- Q9.2 — *Decided 2026-05-26*: rename the agent task-name in lockstep.
  Change the hardcoded `task-name: update-ticket` (line 1397) to
  `task-name: mark-ticket`, rename the agent prompt file (likely
  `.claude/agents/.../update-ticket.md` → `mark-ticket.md`), and update
  any prompt-routing config referencing it. Inventory at execution time.
- Q9.3 — *Decided 2026-05-26*: defer. Other generic MID names (`commit`,
  `compile`, `run-tests`, `approve`) get their own naming pass in a
  separate plan — each needs its own analysis.

## Item 10 — `refine-backlog` sub-process: rename to `refine-backlog-item`

**Observation**: The `refine-backlog` sub-process (process-flow.yaml:343-354)
runs `refine-acceptance-criteria` against **one ticket**, not the whole
backlog. Its only call site (`REFINE_BACKLOG` in `refine-ticket`, lines
176-178) sits between `MARK_IN_REFINEMENT` and `MARK_READY` for that single
ticket. The name is misleading — "refine the backlog" reads as a
batch/grooming activity over the entire queue, when in fact it's refining
one backlog item.

**Direction (decided 2026-05-26)**: rename to **`refine-backlog-item`**
(sub-process) and **`REFINE_BACKLOG_ITEM`** (call site).

**Files**:
- `internal/atdd/runtime/statemachine/process-flow.yaml`:
  - Line 343: process definition rename.
  - Line 341: YAML section comment.
  - Line 176, 190, 191: call-site rename in `refine-ticket`.
  - Line 350, 354: `REFINE_BACKLOG_END` → `REFINE_BACKLOG_ITEM_END` (for
    consistency with the process rename).
- `internal/atdd/runtime/diagram/diagram.go` — `processOrder` list (one
  entry to rename).
- Test fixtures referencing `refine-backlog` / `REFINE_BACKLOG`.

**Open questions for /refine-plan**:

- Q10.1 — *Decided 2026-05-26*: `refine-backlog-item`.
- Q10.2 — *Decided 2026-05-26*: defer. Other CYCLE-level names get their
  own naming pass in a separate plan (consistent with Q9.3 doctrine).

## Item 11 — `implement-ticket`: split flat ticket-kind gateway into hierarchical type → subtype

**Observation**: `implement-ticket` (process-flow.yaml:232-294) has a single
`GATE_TICKET_KIND` gateway with seven outgoing edges, one per ticket-kind
value:

```
story                                  → change-system-behavior
bug                                    → change-system-behavior
task/cover-legacy                      → cover-system-behavior
task/redesign-system                   → redesign-system-structure
task/refactor-system                   → refactor-system-structure
task/refactor-tests                    → refactor-test-structure
task/onboard-external-system           → onboard-external-system
```

The compound `task/<subtype>` slash already encodes a hierarchy — "task" is
a parent category with five subtypes; "story" and "bug" are flat siblings of
"task". The current flat gateway flattens this hierarchy at the gateway
level, losing structure.

**Direction (decided 2026-05-26)**: split into two gateways in series.

- `GATE_TICKET_KIND` (binding: `ticket_kind`, values: `story`, `bug`, `task`)
- `GATE_TASK_SUBTYPE` (binding: `task_subtype`, values: `cover-legacy`,
  `redesign-system`, `refactor-system`, `refactor-tests`,
  `onboard-external-system`)

```
MARK_IN_PROGRESS
  → GATE_TICKET_KIND  (binding: ticket_kind)
      story → CALL_CHANGE_SYSTEM_BEHAVIOR
      bug   → CALL_CHANGE_SYSTEM_BEHAVIOR
      task  → GATE_TASK_SUBTYPE  (binding: task_subtype)
          cover-legacy             → CALL_COVER_SYSTEM_BEHAVIOR
          redesign-system          → CALL_REDESIGN_SYSTEM_STRUCTURE
          refactor-system          → CALL_REFACTOR_SYSTEM_STRUCTURE
          refactor-tests           → CALL_REFACTOR_TEST_STRUCTURE
          onboard-external-system  → CALL_ONBOARD_EXTERNAL_SYSTEM
  → (all CALLs converge) → MARK_IN_ACCEPTANCE
```

Keeping the existing `ticket_kind` binding name (just shrinks its value
set from 7 to 3) means Phase D bindings see less churn — a new
`task_subtype` binding is added.

Benefits:

- Matches how operators conceptually classify ("first: what is this — story,
  bug, or task? then: if task, what kind?").
- The two-gateway structure mirrors the slash-delimited shape in the
  underlying value space.
- Story and bug both route to `change-system-behavior` — currently two
  separate edges with identical targets; one edge from the type-level
  gateway is enough.

Trade-offs:

- One extra gateway node in the diagram (modest cost; readable).
- `refine-ticket` (or whatever upstream sets the ticket-kind) now produces
  two classifications (type + subtype) instead of one slash-delimited
  string. The Phase-D bindings for `ticket_type` and `task_subtype` would
  be the canonical place to do that split.
- The lookup table comment in process-flow.yaml:215-225 needs rewriting
  into a two-level form.

**Files**:
- `internal/atdd/runtime/statemachine/process-flow.yaml`:
  - Replace `GATE_TICKET_KIND` (line 241-244) with `GATE_TICKET_TYPE` and
    `GATE_TASK_SUBTYPE`.
  - Rewrite the sequence_flows in `implement-ticket`.
  - Update the lookup-table comment (lines 215-225) into a two-level
    table.
- Statemachine binding code (Phase D scope) — the `ticket_kind` binding
  splits into `ticket_type` + `task_subtype`.
- Any test fixtures referencing `ticket_kind` / `GATE_TICKET_KIND`.

**Open questions for /refine-plan**:

- Q11.1 — *Decided 2026-05-26*: `GATE_TICKET_KIND` + `GATE_TASK_SUBTYPE`.
  Keeps the existing `ticket_kind` binding name; new `task_subtype`
  binding for the second level.
- Q11.2 — *Decided 2026-05-26*: this plan ships YAML structure + stub
  binding; Phase D wires the real `task_subtype` binding alongside its
  other binding work. Cross-reference the Phase D plan from this plan's
  Item 11 execution notes.
- Q11.3 — `refine-ticket` and any other upstream code that *writes*
  `ticket_kind` needs to be located and updated. Inventory at execution
  time. Most likely lands with Phase D too.
- Q11.4 — Validation: how does the gateway handle "task" type with no
  subtype set, or a subtype set when type is story/bug? Document the
  invariant in the YAML comment block. Likely: the runtime's
  no-edge-matched error fires (consistent with how unrecognised
  ticket-kinds are handled today, per the comment at
  process-flow.yaml:227-230).

## Execution order

Items in execution order after the 2026-05-26 /refine-plan walk. Deferred
items (1, 5, 6) and merged items (4) are excluded — they don't ship in
this plan's scope.

1. **Item 7** (legend wording) — pure renderer change, isolated. Updates
   the three Mermaid sample labels + bullet text + `writeExecutorStyling`
   doc comment to `Service Task (Automated)` / `User Task (Human)` /
   `User Task (LLM Agent)`.
2. **Item 8** (drop the `main` legacy-alias) — single deletion in the
   `processAlias` map; heading becomes `## main`.
3. **Item 2** (require `documentation:` everywhere) — mass YAML edit:
   ~85 nodes gain a `documentation:` line under whatever labelling
   convention Q2.1 settles on at execution time; renderer drops the
   ID-fallback branch and adds the schema-validation requirement. Subsumes
   the six pre-existing docs from old Item 4.
4. **Item 3** (de-duplicate opportunistic-refactor block) — YAML change:
   collapses 6 nodes + 7 edges into one `call_activity` → `refactor`.
   Updates statemachine tests that reference the removed `OPP_*` IDs.
5. **Item 9** (rename `update-ticket` → `mark-ticket`) — YAML rename across
   4 call sites + process definition + section comment + diagram.go
   `processOrder`; rename the agent prompt file and any prompt-routing
   config in lockstep (Q9.2 decided).
6. **Item 10** (rename `refine-backlog` → `refine-backlog-item`) — YAML
   rename: process def + call site + section comment + diagram.go
   `processOrder` + `REFINE_BACKLOG_END` → `REFINE_BACKLOG_ITEM_END`.
7. **Item 11** (split ticket-kind gateway into hierarchical type → subtype)
   — YAML structural change: `GATE_TICKET_KIND` keeps name but its value
   set shrinks to story/bug/task; new `GATE_TASK_SUBTYPE` gateway routes
   the five task subtypes. Stub binding for `task_subtype`; real wiring
   lands with Phase D.

Regenerate `docs/process-diagram*.md` after each item; commit per item.
The GitHub render-budget bug (deferred Item 6) is unrelated — readers will
still hit "Unable to render rich display" past ~40 diagrams until Item 6
ships in a follow-up plan.

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
