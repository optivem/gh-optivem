Read the in-flight plan files and write a coordination meta-plan: dependency graph, conflicts, consolidation findings (each with a decided resolution), execution units, and execution waves (parallel-safe batches) — so the user knows which agent sessions to spin up in parallel.

This command is **read-only on the plans themselves**. Never edit a plan, never invoke `/execute-plan`, never commit. You do the analysis and write the meta-plan **inline in the main session** — there is no separate agent to spawn.

You audit *coordination across plans*. You do NOT audit the logical correctness of any individual plan's content, you do NOT propose edits to plan items, and you do NOT decide what a plan should do — only how the plans relate to each other and in what order they can be safely executed.

## Input

The scope is provided as `$ARGUMENTS`. The contract is **required first plan, optional last plan**:

- **Required:** the first plan to coordinate. Either a full filename (`20260518-1144-atdd-bpmn-orchestration.md`) or a date/slug prefix that uniquely identifies one file (`20260518-1144`).
- **Optional:** the last plan, given the same way. When present, the range is **inclusive on both ends** — `[first..last]` by filename sort order.

Accepted argument shapes:

- `<first>` — coordinate from `<first>` onwards (every `*.md` directly under `plans/` whose filename sorts ≥ `<first>`, skip `plans/deferred/`).
- `<first> <last>` (space-separated) or `<first>..<last>` (range syntax) — coordinate the inclusive range `[first..last]`.

**If `$ARGUMENTS` is empty, do NOT default to "everything".** Stop and ask the user: "Which plan should coordination start from? (filename or date prefix; optionally pass a last plan to bound the range.)" — then re-enter with the answer. Empty input is treated as missing required argument, not as "coordinate everything".

**Resolve and validate the inputs** before starting the analysis:

1. Resolve the plans directory dynamically from the current repo root (`git rev-parse --show-toplevel`). Never hardcode absolute paths.
2. `Glob` `plans/*.md`, sort lexicographically.
3. Resolve `<first>` to a real file. If `<first>` is a prefix, expect exactly one match — multiple matches or zero matches is an error; report the candidates (or "no match") and stop.
4. If `<last>` is given, resolve it the same way; require `<last> >= <first>` in sort order.
5. Compute the in-scope list as the contiguous slice. This resolved list is what you analyse — don't re-resolve the prefix later.

**Referenced-only plans.** Plans outside the in-scope slice but referenced as hard dependencies (per "Dependency signals" below) by an in-scope plan are still **read** so the dependency graph is complete — but mark them as "referenced-only, not in coordination scope" in the meta-plan and do not include them in execution units or waves. `plans/deferred/*.md` is always referenced-only.

## What to do

Do the coordination analysis **inline in the main session** and `Write` the meta-plan yourself. Complete conflict + consolidation analysis BEFORE building the wave plan, and **decide** every consolidation in the meta-plan (not as an open question) so the wave plan operates on a stable post-consolidation unit set. Output path convention: `plans/{YYYYMMDD-HHMM}-meta-{slug}.md` (timestamp = current UTC via `date -u +%Y%m%d-%H%M`; `slug` = a 2–4 word topic).

**Optional escape hatch (default: inline).** For a **large** in-scope set — where reading every plan in full would bloat the main session's context — you MAY delegate the plan reads + analysis to a single `general-purpose` subagent that returns the finished meta-plan (path + the 4-line summary). Give it the resolved in-scope list, the original first/last arguments, and the full workflow below as its brief. This keeps the token-isolation benefit available without a dedicated, separately-synced agent. **Default to inline** for small/medium sets; only reach for the subagent when the read volume genuinely warrants it. Never spawn the subagent with `isolation: "worktree"`.

### Workflow

**The ordering of these steps matters.** Conflict + consolidation analysis MUST complete before the wave plan is built, because sequencing is computed on the *post-consolidation* unit set — not on the raw plan list. Otherwise the meta-plan can confidently sequence two plans that actually need to merge or run atomically, and the user follows a sequence that is wrong by construction.

1. **Discover.** You already resolved the in-scope slice above. Resolve the directory dynamically (`git rev-parse --show-toplevel` if needed) — never hardcode paths.

2. **Read every in-scope plan in full.** Never summarise from titles or headers alone. Per the consistency-check rule, enumerate concretely before reaching any conclusion.

3. **Extract a structured record for each plan** — see "Per-plan extraction" below. Hold these as parallel lists so cross-plan comparisons are easy.

4. **Compute the dependency graph (provisional).** Edges come from explicit cross-plan language (see "Dependency signals" below), not from your inference about what *should* depend on what. Detect cycles — they are always findings, never silently broken. This is provisional because consolidation in step 6 can collapse nodes and remove the edges between them.

5. **Detect file collisions and classify conflicts.** Cross-reference every plan's touched-file set. Every file appearing in two or more plans is a collision-risk; classify each per "Conflict detection" below. A plan pair with a *hard conflict* or a *dependency inversion* cannot be sequenced independently — it must either consolidate, atomic-execute, or have one side rewritten. Surface every conflict before moving on.

6. **Detect consolidation candidates and decide each one.** For every plan pair that shares files, has cyclic dependencies, has mutual "filed from" / "consumed by" language, or that the plans themselves flag as "execute atomically", produce a consolidation finding. **Decide the resolution per finding** — merge / atomic-single-session / re-order — and mark one as recommended. Do not leave these as open questions in the wave plan; resolve them now so step 8 has a stable input.

7. **Collapse the plan set into "execution units".** A *unit* is what wave grouping operates on:
   - A standalone plan whose consolidation findings (if any) all resolved to "no change" = one unit.
   - A pair/group of plans resolved to "atomic single-session execution" = one unit (with the constituent plans listed).
   - A pair/group of plans resolved to "merge" = one *prospective* unit with a note that the merge must happen as a pre-step before any wave runs.
   Rebuild the dependency graph at the unit level — edges between merged constituents collapse to self-edges and disappear; edges from outside the unit re-target the unit as a whole.

8. **Group units into execution waves.** A wave is a set of units with no unmet dependencies on units outside earlier waves and no mutual file collisions. Within a wave, list which units can spawn as parallel agent sessions and which must serialise on a shared file. This is the output that answers the user's "what can I run in parallel right now" question.

9. **Write the meta-plan** to `plans/{YYYYMMDD-HHMM}-meta-{slug}.md` (timestamp = current UTC; `slug` = a 2–4 word topic, e.g. `bpmn-coordination`). Use `Bash` to compute the timestamp (`date -u +%Y%m%d-%H%M`). The meta-plan presents the sections in **the same order the workflow produced them** — conflicts and consolidation findings come BEFORE the wave plan, so a reader sees what was resolved before they see the sequence.

10. **STOP after writing.** Do not edit any plan; do not invoke `/execute-plan`; do not commit. Report the meta-plan path and a 4-line summary to chat (see "After writing" below).

### Per-plan extraction

For each in-scope plan, capture:

- **Status.** Look for explicit markers at the top: `✅ REFINED`, `✅ Partial execute`, `⚠️ NOT YET REFINED`, `⏳ Deferred`. Also scan for "landed in commit `<sha>`", "item N — ✅ done", "items 1, 2 — ✅ landed". If items are individually marked, count `done / in-flight / deferred / remaining`.
- **Touched files.** Walk the plan for code/doc paths it modifies. Treat any path that appears in an "Edits to …", "Files in scope", "Locations" table, or under an Item heading as a touch. Distinguish *primary* (the plan owns the edit) from *referenced* (the plan mentions the file but doesn't edit it).
- **Declared dependencies.** Headings/phrases that mark hard ordering: `Hard dependencies`, `Pre-requisites`, `Hand-off`, `Execute order`, `must land before`, `must land after`, `depends on`, `consumes`, `signature inherited from`, `predecessor`, `successor`. Resolve each to the depended-on plan's filename when possible.
- **Sibling references.** `Sibling plans referenced:` blocks — these aren't always hard dependencies; they may be informational. Capture them, but don't promote to edges unless the body language is strict.
- **Concurrency markers.** `[[feedback_check_concurrent_agents]]` and similar pickup-marker references — they signal the plan author already knows collisions are likely.
- **Deferred items.** Items the plan explicitly defers to a separate plan (under `plans/deferred/` or "filed as a separate plan") — these are NOT execution work for this plan and should be excluded from its touched-file set.

### Dependency signals (what counts as an edge)

Promote to an edge in the graph:

- Direct statements: "must land before/after", "executes after", "blocks", "is a hard prerequisite of".
- Signature/contract inheritance: "this plan consumes `X` introduced by plan `Y`".
- File-edit chains: "plan Y must rename `X` before this plan adds `X.foo`".
- Status references: "item N depends on item M of plan Y being landed".

Do NOT promote to an edge:
- Generic "see also" or "sibling plan" pointers without ordering language.
- Cross-references that only justify a decision (rationale, not gating).

If you are unsure whether a reference is an edge, list it under **Needs-decision** in the meta-plan and pick the conservative default (treat as edge, so the user sees the implication of breaking it).

### Conflict detection

For each file that appears in two or more plans' touched-file sets, classify:

1. **Hard conflict** — both plans rewrite the same lines / functions / sections in incompatible ways. Cannot land independently. Must consolidate or strictly serialise with rebase between them.
2. **Soft conflict** — both plans edit the file but in non-overlapping regions (e.g. different functions, different doc sections). Safe to run sequentially with a rebase; unsafe to run in parallel agent sessions because the second session will see stale context.
3. **Coordination conflict** — one plan renames or restructures the file; another plan edits the file's old shape. Whichever runs first dictates the other's mechanical updates. Flag the rewalk requirement.
4. **Dependency inversion** — plan A's edges say it lands after B, but A's touched files include changes that B explicitly assumes are not yet present. Report as a meta-plan finding; the user must resolve before either runs.
5. **Cycle** — A depends on B and B depends on A (possibly transitively). Always a finding; never silently broken. Recommend either splitting one plan or executing the cyclic portion as a single atomic session.

For each conflict, name the file, both plans, the specific items inside each plan that touch it, and the conservative resolution.

### Consolidation candidates

Surface a consolidation finding when:

- Two plans touch the same single file with edits that are mechanically intertwined (e.g. both add fields to the same struct, both rename keys in the same map).
- The combined execute-order language across plans says "this plan and that plan should land in a single atomic session by a single executor" (this language appears verbatim in some real plans — surface it loudly).
- Two plans each defer a "cross-link follow-up" to the other — they are mutually waiting; merging them removes the wait.

For each candidate, propose ONE of:
- **Merge plans** into a single fresh plan file (cite `[[feedback_new_plan_not_extend]]` — never edit an existing plan in place; write a fresh combined plan).
- **Atomic single-session execution** — keep the plans separate as written, but execute them in one agent session so the file-collision is resolved in working memory rather than via rebase.
- **Re-order to eliminate the entanglement** — when the entanglement is artificial and proper sequencing makes it disappear.

Mark one as recommended per the claude/CLAUDE.md rule and explain why in one sentence.

### Execution waves (the parallel-sessions output)

Build waves greedily:

- **Wave 1** = plans with no unmet dependencies AND no file collisions with each other.
- **Wave N** = plans whose dependencies are all satisfied by wave 1..N-1 AND no file collisions with other wave-N plans.

Within each wave, group plans into **parallel-safe batches**:
- Plans in the same batch touch disjoint file sets and can spawn as independent agent sessions.
- Plans in different batches within the same wave touch shared files and must serialise; the user runs batch A, waits for completion, then runs batch B.

For each batch, list:
- Which plans (and which items inside each, if the plan is partially landed).
- Which files each plan owns in this batch.
- Estimated session count (one agent per plan unless the plan body says otherwise).
- Any pre-execute commands the plan's `Hand-off` section names (e.g. "grep `plans/*.md` for pickup markers" — include the actual grep).

If the user explicitly asked "what agent sessions can I spin up in parallel right now," the meta-plan's **Wave 1, Batch A** section answers that question; everything after is the rest of the runway.

### Meta-plan output format

Write a single markdown file with these sections. Omit any section that would be empty (don't pad).

```markdown
# {YYYY-MM-DD HH:MM UTC} — Plan coordination meta-plan: {topic}

**Plans analysed:** N in-scope, M referenced-only

## Per-plan status snapshot

| Plan | Status | Items done / total | Touched files (primary) | Notes |
|---|---|---|---|---|
| `20260518-1144-...` | ✅ refined | 0 / 8 | `process-flow.yaml`, … | depends on legacy-coverage marker |
| ... | ... | ... | ... | ... |

## Dependency graph

```
20260518-1500-...  ──►  20260518-1742-...  ──►  20260518-1530-...  ──►  20260518-1144-...
                                                      │
                                                      └►  20260519-0704-...
```

(Render as a fenced ASCII tree or arrow list — small graphs only. For graphs >10 nodes, emit a `mermaid` block instead.)

## Conflicts

(Omit if none.)

### 1. `internal/projectconfig/paths_defaults.go` — hard conflict
- Plan `20260518-1742-...` item 3a rewrites `at_test` stems and adds `ct_test`.
- Plan `20260519-0704-...` item 2 renames `external_driver_*` → `external_system_driver_*`.
- **Why hard:** both plans rewrite `canonicalPathKeys()` and `pathStems()` in the same atomic edit. If `1742` lands first, `0704` must rebase its key list against the new `ct_test` entry; if `0704` lands first, `1742` must rebase its stem rewrites against the renamed keys.
- **Recommended resolution:** atomic single-session execution — one executor lands both in one commit. (Per the plans' own Hand-off sections, this is already suggested as a "pragmatic alternative".)

## Consolidation findings (decided)

(Omit if none. Each finding includes a decided resolution — these are NOT open questions.)

### 1. `20260518-1742` + `20260518-1530` — atomic single-session
- Both touch `paths_defaults.go` and `phase-scopes.yaml`; `1530` references `1742`'s `ct_test` as a hard dependency, and `1742` references `1530`'s `DefaultPaths` signature as a hard dependency. Bidirectional waiting.
- **Resolution (recommended):** keep separate but execute as a single atomic session (one executor, one commit). **Why:** plans were filed under `[[feedback_new_plan_not_extend]]`; merging now would violate that rule. Atomic session resolves the entanglement without rewriting either plan.
- **Alternative considered:** merge into a fresh plan. Rejected because both plans are refined and `/execute-plan`-ready; merging would re-open the refinement walk.

## Execution units (post-consolidation)

The wave plan below operates on these units, not on raw plan files. Each unit is what one agent session executes; units that consolidate multiple plans are noted.

| Unit | Plans | Type | Touched files (primary) |
|---|---|---|---|
| U1 | `20260518-...` | standalone | `path-a.go`, `doc-b.md` |
| U2 | `20260518-1742` + `20260518-1530` | atomic-single-session | `paths_defaults.go`, `phase-scopes.yaml`, … |
| U3 | `20260518-1144` | standalone | `process-flow.yaml`, … |

(If a unit is a prospective merge, list it with status `merge-pending` and name the pre-step required before any wave can run.)

## Needs-decision (genuine ambiguity only)

(Omit if none. Do NOT use this section for unresolved consolidations — those are decided in the section above. Use this only for choices that genuinely change the wave structure and that you cannot resolve from the plans alone.)

### 1. Is `20260518-2236` (NOT YET REFINED) in scope right now?
- Plan's own Open question 1 explicitly gates the rest of its execution on a reconciliation with `20260518-1530`. Until that question is answered, the plan cannot be sequenced.
- **Question for user:** defer this plan until refined, or block the entire wave on resolving its Open question 1?

## Execution waves

### Wave 1 — can start now

**Batch A (parallel-safe, N agent sessions):**
- Plan `20260518-...` items 1, 2 — touches `path-a.go`, `doc-b.md` — fresh agent.
- Plan `20260518-...` items 3-5 — touches `path-c.yaml` — fresh agent.
- (Both batch-A plans touch disjoint files; spawn in parallel.)

**Batch B (serial after Batch A — shares `paths_defaults.go`):**
- Plan `20260518-1742` + `20260518-1530` (items 3, 5, 6) — atomic single-session execution per consolidation candidate #1 above.

### Wave 2 — after wave 1 lands

...

## Pre-execute checks (apply before any wave starts)

- `grep -l "PICKUP\|in-flight\|claimed by" plans/*.md plans/deferred/*.md`
- `git status` — confirm no uncommitted changes on the files touched by wave 1.
- Confirm any open questions in the `Needs-decision` section above have been answered.

## Out of scope of this meta-plan

- Plan content correctness (use `process-audit` or the plan's own refine cycle).
- Architecture/code alignment (use `architecture-sync`).
- Actual execution (use `/execute-plan` against each plan in turn).
```

### After writing

Print exactly these lines to chat (substitute real values):

```
Meta-plan: plans/{timestamp}-meta-{slug}.md
Plans analysed: {N in-scope}, {M referenced-only}
Conflicts: {count}  Consolidation candidates: {count}  Needs-decision: {count}
Wave 1 batches: A={n} (parallel), B={n} (serial), … — see meta-plan for details.
```

Do not re-summarise the meta-plan inline beyond these four lines — the user reads the file.

## After

Suggest the natural follow-ups in one short line:

- `/refine-plan plans/{path}` — walk the meta-plan and tweak any decisions before acting on them.
- For each wave-1 batch in the meta-plan, the user can spawn a fresh agent session per batch member (the meta-plan names the files each session should touch).

## Rules

- Read-only on the input plans. Never `Edit` or `Write` to them. Never invoke `/execute-plan`, never commit, never push.
- Do not invent dependencies the plans don't state. If a plausible dependency is unstated, list it under **Needs-decision** with the question for the user — don't add it as an edge silently.
- Never use `isolation: "worktree"` when spawning the optional escape-hatch subagent (per claude/CLAUDE.md).
- Always mark one option as **recommended** when surfacing a resolution choice, and explain why in one sentence (per claude/CLAUDE.md).
- Minimise choices the user has to make. If a finding has an obvious conservative default, state it as the recommendation and proceed in the meta-plan; only escalate to **Needs-decision** when the choice genuinely changes the wave structure.
- Resolve paths dynamically (`git rev-parse --show-toplevel`, `pwd`). Never hardcode absolute Windows or POSIX paths.
- When a graph has more than ~10 nodes, render as a `mermaid` flowchart block, not ASCII arrows.
- Skip empty meta-plans. If every plan is in a single wave with no conflicts and no consolidations, report that in chat and do not write a meta-plan file.
