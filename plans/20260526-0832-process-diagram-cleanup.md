# Process diagram cleanup — labels, layout, duplication, rendering budget

> ⚠️ **Blocked on `plans/20260526-1220-fix-mark-ticket-state-transition-routing.md`.**
> That plan eliminates the `update-ticket` wrapper subprocess and converts
> MARK_* call_activities into direct service_tasks. Execute 1220 first
> (and its own blocker, 0907 legacy-bindings-audit), then this plan.
> Running in parallel risks merge conflicts in `process-flow.yaml` at the
> MARK_* nodes (lines 172, 182, 237, 272) and around `implement-ticket`'s
> gateway section (lines 232-294) which Items 2, 11, 17 touch.
>
> **Item 9 is superseded by 1220** — see Item 9 below for cross-reference.

## Origin / intent

Conversation with user (2026-05-26 08:32) walking through observed issues in
`docs/process-diagram.md` (generated from
`internal/atdd/runtime/statemachine/process-flow.yaml` by
`internal/atdd/runtime/diagram/diagram.go`). Twenty-two items in total
across the original walk + the BPMN-purity audit + the pre-rename naming
audit.

The 2026-05-26 /refine-plan walk results:

- **In scope (will ship in this plan)**: Items 2, 3, 7, 8, 10, 11, 12,
  13, 16, 17, 19, 20, 21, 22.
- **Superseded by another plan**: Item 9 → `plans/20260526-1220-fix-mark-ticket-state-transition-routing.md`
  (eliminates the `update-ticket` wrapper entirely; no `mark-ticket` rename
  to perform).
- **Deferred (revisit later)**: Items 1, 5, 6.
- **Deferred to its own plan**: Item 14 (domain-semantics change — ticket
  kind split for redesign).
- **Merged into other items**: 4 → 2 (RED/GREEN normalisation folded into
  the documentation: requirement); 15 → 12 (CALL_PARAMETERISED_CORE
  rename folded into the CALL_* prefix drop).
- **Dropped**: 18 (wrong premise — gateway-controlled loops are already
  BPMN-idiomatic; no marker needed).

## Scope

- **Renderer** (`internal/atdd/runtime/diagram/diagram.go`): label format,
  legend wording, `processAlias` map, new `error_end_event` shape, schema-
  validation hooks.
- **Statemachine schema** (`internal/atdd/runtime/statemachine/load.go`):
  require `documentation:` on call_activity (Item 2); hard-error on
  question-form gateway docs (Item 16); new `error_end_event` node type
  (Item 17).
- **YAML** (`internal/atdd/runtime/statemachine/process-flow.yaml`):
  per-call-site `documentation:` for ~81 call_activity nodes (Item 2),
  structural de-duplication of opportunistic-refactor (Item 3), CALL_*
  prefix removal (Item 12), CHOOSE_REFACTOR_TYPE insertion (Item 13),
  predicate-form gateway docs (Item 16), error end-events on relevant
  gateways (Item 17), refine-backlog-item rename (Item 10), ticket-kind
  gateway split (Item 11).
- **Output artefact** (`docs/process-diagram.md`): regenerated after each
  item.

Out of scope: domain semantics changes (process behaviour, ticket-kind
catalogue, agent prompts — except where 1220 already handles them). The
de-duplication in Item 3 preserves semantics. Item 14 (redesign-system-
structure split) is deferred to its own plan precisely because it
*does* change domain semantics.

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
`documentation:` line to every call_activity node that currently lacks
one. Drop the ID fallback in the renderer entirely. The point: an operator
reading the diagram should not see screaming-snake YAML node IDs anywhere.

> **Post-1220 note**: 1220 converts the four MARK_* call_activities into
> service_tasks. Item 2's pass operates on the post-1220 YAML — node count
> drops from ~85 to ~81 call_activities needing `documentation:`. Service
> tasks have their own labelling convention (already carry `documentation:`
> via `action:` mapping); Item 2 leaves them alone.

**Renderer change** (`diagram.go:383-390`, the `CallActivity` case in
`writeNode`):

- If `documentation:` is set and is different from the sub-process name →
  render `[doc — see § sub-process]`.
- If `documentation:` is set and is equal to the sub-process name → render
  `[doc]` (drop the redundant "see §" suffix).
- If `documentation:` is missing → the YAML is incomplete; emit a
  schema-validation error at parse time (catches new call_activity nodes
  that forget the doc field).

**Labelling convention (decided 2026-05-26): BPMN-pure verb-phrase, Title Case.**

- **Verb-object phrasing everywhere** (BPMN's canonical naming
  convention).
- **Title Case**: capitalise major words (nouns, verbs, adjectives,
  adverbs). Leave articles, prepositions, conjunctions ≤4 letters
  lowercase ("the", "of", "in", "and", "for"). Matches Camunda /
  Bizagi / BPMN.io style.
- **State constants stay uppercase** (`IN REFINEMENT`, `READY`).
  **Abbreviations stay uppercase** (`DSL`, `AT`, `CT`).
- **Stage prefixes dropped**: no RED/GREEN/Cover/REFACTOR prefixes in
  labels. The TDD-stage vocabulary lives in YAML section comments;
  removing it from labels favours BPMN purity. Visual TDD-stage signal
  is restored separately via Item 19 (border-colour metadata).
- **MARK_* nodes are out of scope** (1220 converts them to service_tasks).

**Worked examples**:

| Node | Label |
|---|---|
| `WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL` | "Write Failing Acceptance Tests" |
| `WRITE_AND_VERIFY_ACCEPTANCE_TESTS_PASS` | "Write Passing Acceptance Tests" |
| `IMPLEMENT_AND_VERIFY_SYSTEM` (`agent-action: implement-system`) | "Implement System" |
| `IMPLEMENT_AND_VERIFY_SYSTEM` (`agent-action: refactor-system`) | "Refactor System" |
| `CHANGE_SYSTEM_BEHAVIOR` (post-Item-12) | "Change System Behavior" |
| `COVER_SYSTEM_BEHAVIOR` | "Cover System Behavior" |
| `REDESIGN_SYSTEM_STRUCTURE` | "Redesign System Structure" |
| `REFACTOR_SYSTEM_STRUCTURE` | "Refactor System Structure" |
| `REFACTOR_TEST_STRUCTURE` | "Refactor Test Structure" |
| `ONBOARD_EXTERNAL_SYSTEM` | "Onboard External System" |
| `IMPLEMENT_TEST_LAYER` (`layer: at`) | "Implement Acceptance-Test Layer" |
| `IMPLEMENT_TEST_LAYER` (`layer: ct`) | "Implement Contract-Test Layer" |
| `IMPLEMENT_TEST_LAYER` (`layer: dsl`) | "Implement DSL Layer" |
| `VERIFY_TESTS_PASS_ACCEPTANCE` | "Verify Acceptance Tests Pass" |
| `VERIFY_TESTS_FAIL_CONTRACT_STUB` | "Verify Contract Tests Fail Against the Stub" |
| `COMPILE_TESTS` | "Compile Tests" |
| `BUILD_SYSTEM` | "Build the System" |
| `START_SYSTEM` | "Start the System" |
| `RUN_TESTS` | "Run Tests" |
| `COMMIT_SYSTEM` / `COMMIT_TESTS` / `COMMIT_LAYER` | "Commit System Changes" / "Commit Test Changes" / "Commit Layer Changes" |
| `DISABLE_ACCEPTANCE_TESTS` | "Disable Acceptance Tests" |
| `REFACTOR_OPPORTUNISTICALLY` (post-Item-3) | "Opportunistic Refactor (Loopable)" |
| `WRITE_AND_VERIFY_ACCEPTANCE_TESTS` (post-Item-12) | "Write and Verify Acceptance Tests" |
| `REFINE_BACKLOG_ITEM` (per Item 10) | "Refine Backlog Item" |
| `REFINE_ACCEPTANCE_CRITERIA` | "Refine Acceptance Criteria" |
| `APPROVE_PRE` / `APPROVE_POST` | "Request Approval" / "Confirm Approval" |
| `FIX` (post-Item-12, was `CALL_FIX`) | "Fix the Failure" |
| `EXECUTE_AGENT` | "Dispatch the Agent" |
| `EXECUTE_COMMAND` | "Dispatch the Command" |
| `FIX_UNEXPECTED_FAILING_TESTS` | "Fix Unexpected Test Failures" |
| `FIX_UNEXPECTED_PASSING_TESTS` | "Fix Unexpectedly Passing Tests" |
| `IMPLEMENT_EXTERNAL_SYSTEM_STUBS` | "Implement External-System Stubs" |
| `IMPLEMENT_DSL` | "Implement the DSL" |
| `IMPLEMENT_SYSTEM_DRIVER_ADAPTERS` | "Implement System Driver Adapters" |
| `IMPLEMENT_EXTERNAL_SYSTEM_DRIVER_ADAPTERS` | "Implement External-System Driver Adapters" |

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
- Q2.4 — *Decided 2026-05-26*: **Option C — generic envelope label.**
  `CALL_AGENT_ACTION` inside `implement-and-verify-system` gets
  `documentation: "Run the Configured Agent"`. The `${agent-action}`
  template is dropped from the label (still used in `process:` to
  resolve which sub-process to dispatch). Disambiguation between
  "Implement System" and "Refactor System" lives at the caller's
  `documentation:` (per Item 2's verb-phrase convention).

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
- id: REFACTOR_OPPORTUNISTICALLY
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
  `REFACTOR_OPPORTUNISTICALLY` (the call_activity). Confirm this is fine — no
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

## Item 9 — `update-ticket` sub-process name is too generic — *superseded by 1220*

> **Superseded by `plans/20260526-1220-fix-mark-ticket-state-transition-routing.md`**
> (decided 2026-05-26 11:41+ rehearsal). The wrapper subprocess is
> **eliminated entirely**, not renamed: 1220 converts the four MARK_*
> call_activities into direct service_tasks bound to four discrete state-
> transition actions (`move-to-in-refinement`, `move-to-ready`,
> `move-to-in-progress`, `move-to-in-acceptance`). The AC-writing agent
> prompt at `internal/assets/runtime/prompts/atdd/update-ticket.md` is
> deleted as dead code. Q9.1, Q9.2, Q9.3 all mooted — no `mark-ticket`
> subprocess to name or wire.
>
> After 1220 ships, this item closes with no execution work. The
> historical observation that motivated it (operator mental model is
> "state transition", not "update") is preserved by 1220's discrete
> action names.

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

> **Post-1220 note**: 1220 modifies MARK_IN_PROGRESS (line 237) and
> MARK_IN_ACCEPTANCE (line 272) within `implement-ticket`'s
> `sequence_flows` — same section Item 11 rewrites. Line numbers above
> are pre-1220 references; verify against post-1220 YAML at execution
> time. Logical conflict is none — MARK_* (service_tasks) and the
> gateway split are independent. Text-level conflict was the original
> driver for sequencing 1220 → 0832.

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

## Item 12 — Drop `CALL_*` prefix; establish role-based call_activity naming convention

> **Verb-first audit (2026-05-26)**: every post-Item-12 node ID must start
> with a verb (matches the existing pattern: `IMPLEMENT_*`, `WRITE_*`,
> `EXECUTE_*`, `COMPILE_*`, `BUILD_*`, `VERIFY_*`, `COMMIT_*`,
> `DISABLE_*`, `ENABLE_*`, `APPROVE_*`, `RUN_*`, `START_*`, `REFINE_*`,
> `REFACTOR_*`, `MARK_*`, `FIX_*`, `CHOOSE_*`).
>
> Two cases needed adjustment:
>
> - `CALL_AGENT_ACTION` → **`RUN_ACTION`** (verb-first; also generalises
>   "agent action" because the structural role at this call site is
>   "run the configured change step," not "invoke an agent" — the agent
>   dispatch happens one layer below in the MID sub-process). **Param
>   renames in lockstep**: `agent-action: implement-system` →
>   `action: implement-system` at every call site (currently 3 call
>   sites; same line-set as Item 2's mass edit).
> - `CALL_FIX` → `FIX` ✓ (verb-first, no change).
> - All other 7 `CALL_*` nodes ✓ verb-first after prefix drop.

**Observation**: `CALL_` prefix appears on 9 distinct node IDs
(`CALL_CHANGE_SYSTEM_BEHAVIOR`, `CALL_COVER_SYSTEM_BEHAVIOR`,
`CALL_REDESIGN_*`, `CALL_REFACTOR_*` ×2, `CALL_ONBOARD_*`,
`CALL_AGENT_ACTION`, `CALL_FIX`, `CALL_PARAMETERISED_CORE`). Bare-named
call_activity nodes outnumber them ~3:1 (`IMPLEMENT_AND_VERIFY_SYSTEM`,
`IMPLEMENT_SYSTEM_DRIVER_ADAPTERS`, `REFINE_BACKLOG`,
`WRITE_ACCEPTANCE_TESTS`, `BUILD_SYSTEM`, `COMMIT_TESTS`,
`EXECUTE_AGENT`, etc.). The CALL_ prefix is inconsistent and signals the
node *type* (Hungarian notation) rather than the *role* the call site
plays.

**BPMN convention**: BPMN does not prescribe a naming convention for
call_activity instances. In practice, call_activity nodes are named for
the **role they play at the call site**, not the sub-process they invoke.

**Direction (proposed)** — drop `CALL_` everywhere; apply two rules:

- If the call site plays a **role distinct** from the sub-process (e.g.
  RED step, opportunistic refactor) → use the **role-based name**
  (`OPP_REFACTOR_*`, `RED_WRITE_FAILING_ACCEPTANCE_TESTS`).
  (Note: `MARK_*` nodes were a canonical example here pre-1220, but
  1220 converts them to service_tasks, outside Item 12's call_activity
  scope.)
- If the call site **IS** the sub-process (1:1 delegation, no extra
  role) → use the **upper-snake form of the sub-process name**
  (`CHANGE_SYSTEM_BEHAVIOR`, `COVER_SYSTEM_BEHAVIOR`).

This rule also dictates Item 15's resolution (drop `CALL_PARAMETERISED_CORE`
in favour of the bare sub-process name).

**Files**:
- `internal/atdd/runtime/statemachine/process-flow.yaml` — rename the 9
  `CALL_*` nodes plus their incoming/outgoing edges.
- Statemachine tests referencing the old IDs.
- The renderer (`diagram.go`) — no change needed; the rule lives in YAML.

**Open questions for /refine-plan**:

- Q12.1 — *Decided 2026-05-26*: two-rule convention confirmed (role-based
  name where the call site has a role distinct from the sub-process;
  bare upper-snake form of the sub-process name for 1:1 delegations).
  Verb-first audit added above; two adjustments made (`RUN_ACTION` +
  `REFACTOR_OPPORTUNISTICALLY` + param rename `agent-action` → `action`).
- Q12.2 — *Decided 2026-05-26*: ship in this plan. Tightly coupled to
  Item 2's mass-edit (same YAML pass). The broader CYCLE/MID naming
  audit deferred by Q10.2 stays a separate future plan.

## Item 13 — Operator-choice gateways need a visible human-input signal

**Observation**: Gateways currently double as both decision-rendering
diamonds AND the elicitation point for the operator's choice. The
gateway documentation reads as a prompt ("Choose refactor type",
"Opportunistic refactor?", "Ticket kind?") but Mermaid renders the
diamond with no executor signal — readers cannot tell that a human is
involved.

**BPMN convention**: a gateway is a **pure routing construct** that
switches on a value some other node already produced. The *act of
choosing* belongs in an upstream user_task (rectangle). Strict BPMN
would render this as:

```
[Choose refactor type]  →  ◇GATE_REFACTOR_TYPE  →  branches
   (user_task, human)         (silent gateway)
```

**Inventory of gateways** (verified 2026-05-26):

| Gateway | Reads | Operator-input? |
|---|---|---|
| `GATE_REFACTOR_TYPE_CHOICE` | refactor-type-choice (elicited at gateway) | **Yes — needs split** |
| `GATE_OPPORTUNISTIC_REFACTOR` | refactor-type-choice (elicited at gateway) | Yes — but collapses per Item 3 |
| `GATE_APPROVED` | approval-outcome (set by APPROVE_PRE user_task upstream) | Already correctly paired |
| `GATE_TICKET_KIND` | ticket-kind (set during refinement; binding reads/parses) | No — computed from state |
| `GATE_TASK_SUBTYPE` (Item 11) | task-subtype (same as above) | No — computed from state |
| `GATE_*_CHANGED` (dsl-port, external-driver, system-driver) | port-change detection | No — computed |
| `GATE_EXPECTED_TEST_RESULT` | expected-test-result (passed-in param) | No — passed in |
| `GATE_TESTS_OUTCOME` | test-outcome (test-runner output) | No — computed |
| `GATE_OUTPUTS_AND_SCOPES_VALID` | validation result | No — computed |
| `GATE_FIX_ON_FAILURE` | config flag | No — config |
| `GATE_COMMAND_SUCCEEDED` | command exit status | No — computed |

**Net scope after Item 3 lands**: exactly **one** gateway requires the
13A treatment — `GATE_REFACTOR_TYPE_CHOICE` in the `refactor` TOP process.

**Direction (decided 2026-05-26): Option 13A — split operator-input
gateways into user_task + gateway pair.** Pure BPMN. Concrete impact:

```yaml
# AFTER (refactor process)
refactor:
  start: CHOOSE_REFACTOR_TYPE
  nodes:
    - id: CHOOSE_REFACTOR_TYPE
      type: user_task
      agent: human
      documentation: "Choose refactor type"
      outputs: refactor-type-choice
    - id: GATE_REFACTOR_TYPE_CHOICE
      type: gateway
      binding: refactor-type-choice
      # documentation removed — gateway is silent
    ...
  sequence-flows:
    - {from: CHOOSE_REFACTOR_TYPE, to: GATE_REFACTOR_TYPE_CHOICE}
    - {from: GATE_REFACTOR_TYPE_CHOICE, to: ..., when: ...}
    - {from: REFACTOR_SYSTEM_STRUCTURE, to: CHOOSE_REFACTOR_TYPE}  # loopback
    - {from: REFACTOR_TEST_STRUCTURE,   to: CHOOSE_REFACTOR_TYPE}  # loopback
    - {from: REDESIGN_SYSTEM_STRUCTURE, to: CHOOSE_REFACTOR_TYPE}  # loopback
```

Diagram impact: one yellow rectangle ("Choose refactor type") upstream
of the diamond; diamond becomes silent. Loopback edges from each
refactor cycle return to the rectangle.

**Files**:

- `internal/atdd/runtime/statemachine/process-flow.yaml` — add
  `CHOOSE_REFACTOR_TYPE` user_task; redirect loopback edges. Item 3's
  collapse of change-system-behavior step 3 removes
  `GATE_OPPORTUNISTIC_REFACTOR` independently.
- Statemachine tests referencing `GATE_REFACTOR_TYPE_CHOICE` as the
  start node of `refactor`.

**Runtime impact**: the new `CHOOSE_REFACTOR_TYPE` user_task needs a
binding to elicit the operator's choice and write `refactor-type-choice`
into state. Likely lands with Phase D (same as Item 11's binding work).

**Remaining checks at execution time**:

- Q13.2 — Confirm during execution that `GATE_APPROVED`'s upstream
  pairing is correct (the existing `APPROVE_PRE` user_task does the
  elicitation — gateway is silent or carries a routing-style
  documentation, not a prompt).
- Q13.3 — Item 11 is **not** affected by 13A. Both `GATE_TICKET_KIND`
  and `GATE_TASK_SUBTYPE` read pre-computed values (ticket
  classification happens during refinement, not at the gateway). No
  user_task pair needed at the implement-ticket gateways.

## Item 14 — Split `redesign-system-structure` into system-side and external-side

> ⏳ **Deferred to a separate plan** (2026-05-26): the only domain-semantics
> change in this plan; the ticket-kind split ripples through refinement,
> agents, and docs. Better tackled in its own focused plan that owns the
> end-to-end ticket-kind change. The brainstorm below remains useful as
> the seed for that follow-up plan.

**Observation**: `redesign-system-structure` (process-flow.yaml:488-512)
runs three steps:

1. `implement-system-driver-adapters` (system-side driver adapters)
2. `implement-external-system-driver-adapters` (external-side driver adapters)
3. `implement-and-verify-system` (re-implements + verifies the system)

So one cycle covers **both** system-side and external-side redesign.
An operator picking `task/redesign-system` gets both regardless.

**Direction (proposed)** — split into two CYCLEs:

- `redesign-system-structure` — `implement-system-driver-adapters` +
  `implement-and-verify-system`.
- `redesign-external-system-structure` —
  `implement-external-system-driver-adapters` (+ `implement-and-verify-system`
  if the system needs re-verification after the external-side change).

This also requires splitting the ticket-kind:

- `task/redesign-system` → routes to `redesign-system-structure`.
- `task/redesign-external-system` → routes to
  `redesign-external-system-structure` (new ticket kind).

Update the `GATE_TASK_SUBTYPE` gateway from Item 11 to add the new
branch.

**Open questions for /refine-plan**:

- Q14.1 — Confirm the split direction. Alternative: keep the unified
  cycle but document the operator can skip one half (no YAML change).
- Q14.2 — Does `implement-and-verify-system` need to fire in the
  external-side cycle too? Depends on whether changing external-system
  driver adapters can change system behaviour. (Likely yes — the
  system's port interface to the external system may shift.)
- Q14.3 — Naming for the new sibling: `redesign-external-system-structure`
  (proposed) vs `redesign-external-system-driver-structure` (more
  precise but verbose) vs other.
- Q14.4 — Scope: ship in this plan or as a separate structural-redesign
  plan? This is the only item that *changes domain semantics* (adds a
  new ticket kind), not just naming/layout.

## Item 15 — Rename `CALL_PARAMETERISED_CORE` — *merged into Item 12 on 2026-05-26*

The two `CALL_PARAMETERISED_CORE` nodes (process-flow.yaml:565, 588) get
renamed to `WRITE_AND_VERIFY_ACCEPTANCE_TESTS` as part of Item 12's
`CALL_*` prefix-drop pass — same rule (1:1 wrapper → bare upper-snake
form of the sub-process name), same YAML pass.

## Item 16 — Remove question-form documentation from computed gateways

**Observation**: Several gateways read pre-computed values but carry
question-form `documentation:` that misleads readers into thinking a
human chooses at that point. Examples:

- `GATE_TICKET_KIND` (process-flow.yaml:241-244): `"Ticket kind (type +
  optional task subtype)?"` — but `ticket-kind` is set during
  refinement; the gateway just reads/parses.
- Likely the same pattern on `GATE_*_CHANGED` and other computed
  gateways — inventory at execution time.

**BPMN convention**: a gateway's label, if any, should be a **predicate**
or **routing condition** ("ticket-kind", "dsl-port-changed"), not a
question. Questions imply elicitation, which belongs in an upstream
user_task (per Item 13).

**Direction (decided 2026-05-26)**: every computed gateway's
`documentation:` rewrites to the **predicate form** (the binding name as
the label). Example: `GATE_TICKET_KIND` → `documentation: "ticket-kind"`.

Question-form documentation is reserved for the operator-input gateways
that Item 13 splits; Item 13 strips that documentation when adding the
upstream user_task. End state: **no gateway in the YAML carries
question-form text**.

The renderer parses the gateway documentation; the schema validator
**hard-errors at parse time** on any gateway whose documentation ends
with `?` (question-form). Forces the convention; consistent with Item 2's
hard-error for missing call_activity docs.

**Files**:
- `internal/atdd/runtime/statemachine/process-flow.yaml` — audit and
  rewrite or drop gateway documentation fields.

**Open questions for /refine-plan**:

- Q16.1 — *Decided 2026-05-26*: predicate-form (binding name as label).
- Q16.2 — *Decided 2026-05-26*: hard-error at parse time on
  question-form gateway documentation (anything ending in `?`).

## Item 17 — Model error end-events for the no-edge-matched failure path

**Observation**: process-flow.yaml:210-213 documents that "operator-facing
parse errors / unrecognised types surface through the runtime's
no-edge-matched error path rather than dedicated STOP nodes." This means
exceptional exits — e.g. unrecognised ticket-kind — fire as a runtime
panic, **invisible in the diagram**.

**BPMN convention**: exceptional exits are modelled as **error
end-events** (circle with a bolt icon). They show readers where things
can blow up.

**Direction (decided 2026-05-26)**: introduce a new node type
`error_end_event` rendered as a **circle with red border + bolt icon
(⚡)**. Add one to each gateway that can fire a no-edge-matched error.
Concrete targets:

- `GATE_TICKET_KIND` — unrecognised ticket kinds.
- `GATE_TASK_SUBTYPE` (Item 11) — unrecognised subtypes.
- Possibly other gateways where the binding could return an
  unenumerated value (inventory at execution time — Q17.3).

**Files**:
- `internal/atdd/runtime/statemachine/load.go` — new YAML node type.
- `internal/atdd/runtime/diagram/diagram.go` — new shape / classDef
  (bolt-icon circle or coloured circle).
- `internal/atdd/runtime/statemachine/process-flow.yaml` — add the
  error end-events to relevant gateways.
- Runtime code that currently raises the "no-edge-matched" panic —
  may need to route to the new error end-event node so the trace
  surfaces it as a proper transition.

> **Post-1220 note**: Item 17 adds error end-events to `implement-ticket`'s
> gateway block (around lines 232-294 pre-1220). 1220 also touches that
> section (MARK_IN_PROGRESS line 237, MARK_IN_ACCEPTANCE line 272). The
> two changes are logically independent (gateways vs MARK_* state
> transitions), but text-level conflict was the original driver for the
> 1220 → 0832 sequencing. Verify pre-1220 line numbers above against the
> post-1220 YAML at execution time.

**Open questions for /refine-plan**:

- Q17.1 — *Decided 2026-05-26*: new node type `error_end_event`.
- Q17.2 — *Decided 2026-05-26*: red border + bolt icon (⚡), BPMN
  standard.
- Q17.3 — Audit which gateways can produce unenumerated values —
  inventory at execution time. Likely candidates beyond the two above:
  any `GATE_*_CHANGED` gateway, `GATE_OUTPUTS_AND_SCOPES_VALID`,
  `GATE_COMMAND_SUCCEEDED`.

## Item 18 — Add explicit loop-subprocess markers to looping flows — *dropped on 2026-05-26*

The post-Item-13 `refactor` shape (`CHOOSE_REFACTOR_TYPE` → gateway →
activities looping back to chooser / exit) is the canonical BPMN
**gateway-controlled loop** pattern. BPMN's `⟳` loop marker applies to
**single activities** (tasks, sub-processes, call-activities), not to
multi-activity flow loops. Adding `⟳` here would be non-standard.

Item 18 was built on a wrong premise. No action needed.

## Item 19 — TDD-stage visual metadata (colored border on relevant nodes)

**Observation**: Item 2 drops the RED/GREEN/Cover/REFACTOR stage prefixes
from node labels to favour BPMN purity. But the TDD-stage vocabulary is
pedagogically important in this teaching repo — a student reading
`change-system-behavior` should be able to see RED → GREEN → REFACTOR
adjacency.

**Direction (decided 2026-05-26)**: separate the **what** (label, BPMN-pure
verb-phrase from Item 2) from the **role** (TDD stage, visual marker).
Add a `tdd-stage:` field to relevant YAML nodes; renderer applies a
coloured border via classDef.

**Schema**:

```yaml
- id: WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL
  type: call_activity
  process: write-and-verify-acceptance-tests-fail
  documentation: "Write Failing Acceptance Tests"
  tdd-stage: red                            # ← new field
```

**Stage → colour mapping** (border colour, no fill conflict with executor
colors). **3-stage enum** (per Q19.1 decision): Cover folds into Green
(same outcome — test passes — and the cycle-name distinction lives in
the YAML cycle structure rather than node coloring).

| `tdd-stage:` | Border colour | Notes |
|---|---|---|
| `red`     | `#dc3545` (red)    | "RED" step — write a failing test |
| `green`   | `#28a745` (green)  | "GREEN" step — test passes (after RED implementation, or as a Cover step against existing behaviour) |
| `refactor` | `#007bff` (blue)  | "REFACTOR" step — improve without changing behaviour |

**Files**:
- `internal/atdd/runtime/statemachine/load.go` — accept `tdd-stage:` field
  (validate against the four-value enum; absent → no styling).
- `internal/atdd/runtime/diagram/diagram.go` — emit a new classDef per
  stage, apply to nodes with `tdd-stage:` set. Add a legend bullet
  explaining the colours.
- `internal/atdd/runtime/statemachine/process-flow.yaml` — annotate the
  four nodes that currently carry stage prefixes (and any others where a
  stage applies, audited at execution time).

**Open questions for /refine-plan**:

- Q19.1 — *Decided 2026-05-26*: 3-stage enum (`red | green | refactor`).
  Cover folds into Green (same outcome — test passes). The cycle-name
  distinction between `change-system-behavior` and `cover-system-behavior`
  carries the pedagogical signal; node coloring doesn't repeat it.
- Q19.2 — *Decided 2026-05-26*: **narrow scope**. Only call-site nodes
  that explicitly play a RED/GREEN/REFACTOR role at their call site.
  Likely candidates: WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL (RED),
  WRITE_AND_VERIFY_ACCEPTANCE_TESTS_PASS (GREEN), the two
  IMPLEMENT_AND_VERIFY_SYSTEM call sites in change-system-behavior
  (GREEN with `agent-action: implement-system`; REFACTOR with
  `agent-action: refactor-system`). Inventory other candidates at
  execution time; don't auto-propagate down into refactor-* /
  redesign-* cycles.
- Q19.3 — *Decided 2026-05-26*: **border-only**. Executor coloring uses
  fill (white = Service Task, dark blue = User Task LLM Agent, yellow =
  User Task Human); TDD-stage uses border (red/green/blue). Both
  signals visible without conflict.

## Item 20 — Sub-process headings to Title Case

**Observation**: The rendered diagram doc has section headings like
`## change-system-behavior`, `## implement-and-verify-system`,
`## refactor-system-structure`. These are the YAML process **names**
(kebab-case identifiers) shown verbatim. BPMN convention: process names
should read as human-readable activities ("Change System Behavior",
"Implement and Verify System"). The `processAlias` map already does this
for `main` (one entry), but other 50+ processes leak kebab identifiers
into the rendered headings.

**Direction (proposed)**: auto-Title-Case the kebab process name in the
renderer. `change-system-behavior` → "Change System Behavior". Special
cases handled by:

- `processAlias` entries take precedence (custom human name overrides
  auto-Title-Case). After Item 8, the map is empty by default; entries
  added only when auto-Title-Case is wrong (e.g. abbreviations: "DSL"
  not "Dsl").
- Abbreviation list: keep `DSL`, `AT`, `CT`, `BPMN`, `TDD` uppercase
  during Title-Case transformation.

Item 5's heading-prefix (`## TOP — Change System Behavior`) is currently
deferred, but if it later lands, the prefix sits in front of the
Title-Cased name.

**Files**:
- `internal/atdd/runtime/diagram/diagram.go::writeProcessSection` — wrap
  the heading emission in a Title-Case transformer. Add an abbreviation-
  preservation list.

**Open questions for /refine-plan**:

- Q20.1 — *Decided 2026-05-26*: **auto-Title-Case** at render time +
  abbreviation list + `processAlias` map as override fallback. No
  `display-name:` field added to YAML.
- Q20.2 — *Decided 2026-05-26*: abbreviation list = **`DSL`, `AT`, `CT`,
  `BPMN`, `TDD`, `ATDD`, `BDD`, `SUT`, `API`, `URL`, `DB`, `IO`**.
  All preserved as uppercase during Title-Case transformation.

## Item 21 — Edge labels: Title Case from kebab routing values

**Observation**: Gateways route on enum-style kebab values
(`refactor-system-structure`, `task/cover-legacy`, etc.). The renderer's
`edgeLabel()` (diagram.go:475-498) passes these through verbatim, so the
diagram edges show "refactor-system-structure" as a label. BPMN
convention: edge labels are short human-readable conditions ("Refactor
System Structure", "Yes", "Story", "Task / Cover Legacy").

**Direction (proposed)**: extend `edgeLabel()` to auto-Title-Case kebab
routing values, using the same abbreviation list as Item 20.

Examples:

| Current edge label | After |
|---|---|
| `refactor-system-structure` | "Refactor System Structure" |
| `task/cover-legacy` | "Task / Cover Legacy" |
| `task/refactor-system` | "Task / Refactor System" |
| `story` | "Story" |
| `bug` | "Bug" |
| `true` → "Yes" | unchanged (already translated) |
| `false` → "No" | unchanged |

The runtime-facing routing values in the YAML stay kebab (functional
identifiers; renaming would risk binding mismatches). The transformation
is render-time only.

**Files**:
- `internal/atdd/runtime/diagram/diagram.go::edgeLabel` — extend the
  pass-through branch to Title-Case kebab values.

**Open questions for /refine-plan**:

- Q21.1 — *Decided 2026-05-26*: shared Title-Case helper with Item 20.
  Same kebab-input, same Title-Case output, same abbreviation list.
- Q21.2 — *Decided 2026-05-26*: spaces around slash (defensive default).
  `task/cover-legacy` → "Task / Cover Legacy". Likely moot after Item 11
  removes the slash-bearing routing values, but the rule covers any
  surviving edges and future slash-bearing values.

## Item 22 — Start / End event labels

**Observation**: Renderer hardcodes `((Start))` and `((End))` for every
start_event and end_event (`writeNode` in diagram.go:374-378). BPMN
convention: start events are labelled with the **trigger** that begins
the process; end events are labelled with the **outcome**. E.g.:

- `refine-ticket` start: "Operator Refines Ticket" (or: "Ticket Picked
  for Refinement").
- `refine-ticket` end (REFINE_TICKET_END): "Ticket Marked READY".
- `implement-ticket` end (IMPLEMENT_TICKET_END): "Ticket Marked IN
  ACCEPTANCE".
- `change-system-behavior` end (CHANGE_SYSTEM_BEHAVIOR_END): "System
  Behavior Changed".

Currently the only signal a reader has is the node ID, which the
renderer doesn't print for start/end events. So readers see anonymous
"Start" / "End" circles.

**Direction (proposed)**: add an optional `documentation:` field to
`start_event` and `end_event` YAML nodes; renderer uses it if present,
falls back to "Start" / "End" otherwise.

Inventory (at execution time): every process's start and end events.
Most ends look like `<process-name>_END` which gives the renderer a hint
for an auto-derived fallback ("`REFINE_TICKET_END` → 'Refine Ticket
Complete'"), but explicit `documentation:` is preferred.

**Files**:
- `internal/atdd/runtime/statemachine/load.go` — allow `documentation:`
  on start_event / end_event (currently allowed but unused by the
  renderer).
- `internal/atdd/runtime/diagram/diagram.go::writeNode` — use the doc
  if present for start/end events; else current hardcoded fallback.
- `internal/atdd/runtime/statemachine/process-flow.yaml` — populate
  `documentation:` on every start_event / end_event.

**Open questions for /refine-plan**:

- Q22.1 — *Decided 2026-05-26*: short noun-phrase. Examples:
  - `REFINE_TICKET_END` → "Ticket Marked READY"
  - `IMPLEMENT_TICKET_END` → "Ticket Marked IN ACCEPTANCE"
  - `CHANGE_SYSTEM_BEHAVIOR_END` → "System Behavior Changed"
  - `REFACTOR_TOP_END` → "Refactor Complete"
- Q22.2 — *Decided 2026-05-26*: error end-events follow the same form.
  Examples: "Unknown Ticket Kind", "Unknown Task Subtype", "Unknown
  <Binding Name>".
- Q22.3 — *Decided 2026-05-26*: **strict require**. Every start_event
  and end_event must have a `documentation:` line. Parse error if
  missing. Consistent with Item 2's call_activity rule.

## Execution order

Items in execution order after the 2026-05-26 /refine-plan walk.

**In scope** (settled): Items 2, 3, 7, 8, 10, 11, 12, 13, 16, 17, 19, 20,
21, 22.
**Out of scope**: Items 1, 5, 6 (deferred), 4 (merged into 2), 9
(superseded by 1220), 14 (deferred to separate plan), 15 (merged into
12), 18 (dropped).

**Renderer-only changes first** (smallest blast radius; easy to review):

1. **Item 7** (legend wording) — Mermaid sample labels + bullet text +
   `writeExecutorStyling` doc comment to `Service Task (Automated)` /
   `User Task (Human)` / `User Task (LLM Agent)`.
2. **Item 8** (drop the `main` legacy-alias) — single deletion in the
   `processAlias` map; heading becomes `## main`.
3. **Item 20** (sub-process headings to Title Case) — auto-Title-Case
   the kebab process name in `writeProcessSection` heading emission;
   preserve abbreviations (`DSL`, `AT`, `CT`, `BPMN`, `TDD`, `ATDD`,
   `BDD`, `SUT`, `API`, `URL`, `DB`, `IO`).
4. **Item 21** (Title-Case kebab edge labels) — extend `edgeLabel()`
   with the same Title-Case helper; spaces-around-slash for any
   surviving slash-bearing routing values.

**Schema additions** (renderer + load.go):

5. **Item 17** (error end-events) — new `error_end_event` node type
   with red border + bolt icon (⚡); load.go accepts the type; renderer
   gets a new shape branch. Bakes in early so Items 11, 16 can use it.
6. **Item 19** (TDD-stage visual metadata) — new `tdd-stage:` field
   accepted in load.go (enum: `red | green | refactor`); renderer
   emits a classDef per stage; legend gets a new bullet section
   explaining the colours.
7. **Item 22** (start/end event labels) — strict-require
   `documentation:` on every `start_event` and `end_event`; renderer
   uses the doc for the label. Parse-time error if missing.

**YAML structural changes**:

8. **Item 16** (computed-gateway documentation cleanup) — every
   gateway's `documentation:` rewritten to predicate form (binding
   name) or stripped; parse-time hard-error on question-form gateway
   documentation.
9. **Item 13** (split operator-input gateways into user_task + gateway)
   — adds `CHOOSE_REFACTOR_TYPE` user_task to the `refactor` process;
   redirects loopback edges; strips question-form documentation from
   `GATE_REFACTOR_TYPE_CHOICE`.
10. **Item 12** (drop `CALL_*` prefix; verb-first naming) — YAML pass:
    rename 9 `CALL_*` nodes per the two-rule convention. `CALL_AGENT_ACTION`
    → `RUN_ACTION` (with param `agent-action` renamed to `action` at
    every call site). Adjective-first `OPPORTUNISTIC_REFACTOR` (Item 3
    target) becomes `REFACTOR_OPPORTUNISTICALLY`. Subsumes Item 15.
11. **Item 2** (require `documentation:` everywhere + apply convention)
    — mass YAML edit: ~81 call_activity nodes gain a `documentation:`
    line under the **BPMN-pure verb-phrase, Title Case** convention.
    Renderer drops the ID-fallback branch; load.go adds the schema-
    validation requirement.
12. **Item 3** (de-duplicate opportunistic-refactor block) — YAML
    change: collapses 6 nodes + 7 edges into one `call_activity`
    (`REFACTOR_OPPORTUNISTICALLY`) → `refactor`. Updates statemachine
    tests that reference the removed `OPP_*` IDs.
13. **~~Item 9~~ (superseded by 1220)** — no execution work in this plan;
    the `update-ticket` wrapper is eliminated by 1220.
14. **Item 10** (rename `refine-backlog` → `refine-backlog-item`) — YAML
    rename: process def + call site + section comment + diagram.go
    `processOrder` + `REFINE_BACKLOG_END` → `REFINE_BACKLOG_ITEM_END`.
15. **Item 11** (split ticket-kind gateway) — YAML structural change:
    `GATE_TICKET_KIND` value set shrinks to story/bug/task; new
    `GATE_TASK_SUBTYPE` gateway routes the five task subtypes; both
    gateways gain an `error_end_event` for unrecognised values (per
    Item 17). Stub binding for `task_subtype`; real wiring lands with
    Phase D.

Regenerate `docs/process-diagram*.md` after each item; commit per item.
The GitHub render-budget bug (deferred Item 6) is unrelated — readers
will still hit "Unable to render rich display" past ~40 diagrams until
Item 6 ships in a follow-up plan.

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
