# Process diagram cleanup — deferred items only

All in-scope work shipped 2026-05-26 (see git history for the four
commits — Items 2, 3, 10, 11 each on its own). Remaining sections
below are the deferred backlog (Items 1, 5, 6) — pick them up as
separate plans when ready.

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

