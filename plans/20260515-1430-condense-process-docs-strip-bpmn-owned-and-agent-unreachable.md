# Plan: Condense the ATDD process docs — strip content that's BPMN-owned or unreachable by the agent

## Background

The ATDD pipeline is owned end-to-end by the BPMN orchestrator
(`internal/atdd/runtime/statemachine/process-flow.yaml`) and its
service-task / call-activity actions
(`internal/atdd/runtime/actions/`). Every dispatched agent under
`internal/assets/runtime/prompts/atdd/atdd-*.md` performs a **single
bounded action** — WRITE (most agents), DIAGNOSE (`atdd-fix-verify`),
or STUBS (`atdd-stubs`). All other lifecycle steps — compile, run,
disable, review, commit, tick checklist, move issue, re-dispatch on
failure, parallel sibling dispatch, post issue comment — are
BPMN-owned. The shared preamble (`internal/assets/runtime/shared/preamble.md`)
prepended to every dispatch states verbatim:

> "the agent must never run `git commit`, `git add`, `gh issue close`,
> the compile commands, or the test commands."

Despite that, the process docs under
`internal/assets/global/docs/atdd/process/` (16 files) still contain
substantial prose that:

1. **Restates BPMN-owned behaviour** (e.g. commit-message construction)
   as if it were a doc-side rule the agent must follow.
2. **Names mechanics the agent can't reach** (e.g. suite placeholders
   only consumed by `gh optivem test run` commands the agent is
   forbidden to run, container rebuild commands, fix-loops driven by
   the agent re-running tests until they pass).
3. **Documents lifecycle plumbing** (checklist ticking, status moves,
   issue-comment posting) that BPMN service_tasks own end-to-end.

The sister in-flight plan
`20260515-1230-tighten-atdd-prompts-and-phase-docs-to-agent-scope.md`
already strips REVIEW (STOP) and COMMIT sections from 8 of the phase
docs and trims orchestrator/tracker/language leakage from 13 agent
prompts. That plan **explicitly excludes** the two `cycles-conventions`
docs (it says "the commit-message format already lives there and is
correct. Leave it"), the GREEN/REFACTOR/DA docs beyond the two it
touches, and several other process docs (`cycles.md`,
`task-and-chore-cycles.md`, `glossary.md`,
`shared-phase-progression.md`, `shared-ticket-status-in-acceptance.md`,
`system-interface-redesign.md`, `diagram-phase-details.md`). It also
does **not** touch the build/run/test command blocks **inside** WRITE
sections of the phase docs it does edit.

This plan picks up where that one stops. Same heuristic, broader
sweep: **a process doc should describe only what the agent's WRITE /
DIAGNOSE / STUBS action does, plus invariants that genuinely
constrain the agent's edits. Everything else is duplication of
BPMN/preamble state and must come out.**

## Goal

Every doc under `internal/assets/global/docs/atdd/process/` describes
**only**:

- What the WRITE/DIAGNOSE/STUBS action produces (state, not commit
  bookkeeping).
- Invariants the agent must respect (scope rules, what not to edit,
  what to declare/use, ordering inside the WRITE step).
- Conceptual cross-references to docs the agent legitimately consults
  (architecture, glossary, language equivalents).

Process docs **never**:

- Restate BPMN-owned plumbing (commit-message format, checklist
  ticking, status moves, issue-comment posting, disable-on-COMMIT,
  re-dispatch routing).
- Embed CLI command blocks the agent is forbidden to run (`gh optivem
  test run`, `gh optivem system build/start/stop`, `git commit/add`,
  `gh issue close`, `./gradlew build`, `./compile-all.sh`,
  `npx tsc --noEmit`, `dotnet build`).
- Reference placeholders or annotations whose only consumers are the
  forbidden CLI blocks (e.g. suite-name placeholders consumed solely
  by `gh optivem test run`).
- Describe agent-driven fix-loops ("run tests, if they fail fix the
  code, repeat") — that loop is the BPMN's RUN_TESTS → fix-verify
  dispatch.

## Examples (representative, not exhaustive — Phase 1 sweeps all docs)

These three are the user-surfaced examples. Phase 1's job is to find
every other instance of the same patterns across the 16 process docs;
do not treat the list below as the full edit scope.

### Example 1 — `## Commit Message Format` in `at-cycle-conventions.md` / `ct-cycle-conventions.md`

Both conventions docs carry a `## Commit Message Format` section
specifying the `<Ticket> | <Phase>` pattern, the `#<issue-number> | `
prefix rule, and the "do NOT append `- WRITE` / `- REVIEW` /
`- COMMIT`" guard.

**Why this is duplication.** The BPMN's `commitPhase` action
(`internal/atdd/runtime/actions/bindings.go:617`) constructs the
message itself:

```go
msg := fmt.Sprintf("%s | %s", title, changeType)
```

— with `title` from the `issue_title` context key (written by
`pick_top_ready` / `move_to_in_progress`) and `changeType` from the
call-activity `change_type` param. The "no `- WRITE / - REVIEW /
- COMMIT` suffix" guard is structurally guaranteed: `change_type` is
the phase **prefix only** by construction in `process-flow.yaml`.
The agent never authors a commit message; the doc-side rule is
unreachable.

**Sub-finding worth surfacing as a follow-up, not folded in here.**
The `#<issue-number> | ` prefix that the conventions docs prescribe
is **not** implemented in `commitPhase` (it only emits `title | change_type`).
Either the prefix has been intentionally dropped and the conventions
doc is stale, or it's a gap in `commitPhase`. Phase 2 confirms which;
the fix lives in the wiring layer, not this doc-condense plan.

### Example 2 — `## Suite Selection` in `at-cycle-conventions.md` / `ct-cycle-conventions.md` and the `Suite selection (...) and commit-message format: see ...` cross-references in 4 phase docs

Both conventions docs map test-channel annotations (`@Channel(API)` /
`@Channel(UI)`) and real-vs-stub pairs to suite placeholders
(`<acceptance-api>`, `<acceptance-ui>`, `<suite-contract-real>`,
`<suite-contract-stub>`), and 4 phase docs cross-link the section.

**Why this is unreachable.** The suite placeholders' only consumers
inside the process docs are `gh optivem test run --suite <…>`
command blocks — commands the preamble forbids the agent from
running. Once Example 3's command blocks are stripped, the
placeholders have **zero remaining consumers** in the agent-facing
docs, and the `## Suite Selection` section becomes dead weight. The
BPMN's RUN_TESTS service_task already knows which suite to invoke per
phase (`process-flow.yaml`'s `suite:` params).

**Edge case to verify in Phase 1.** The `@Channel(API)` / `@Channel(UI)`
test annotations themselves *may* still be agent-relevant — the agent
authors tests in WRITE phases and chooses which channel to annotate.
That choice is described inside the test-authoring phase doc itself
(`at-red-test.md`), not via the suite-name mapping. The
**channel→suite-name mapping** is what's BPMN-only; the
**channel-annotation rule** is agent-only. Phase 1 separates these.

### Example 3 — `## What it produces` bullets framed as "Commit `<Ticket> | <PHASE>` containing X"

Every phase doc opens with a `## What it produces` section whose
first bullet is shaped like
`at-red-system-driver.md`'s:

> Commit `<Ticket> | AT - RED - SYSTEM DRIVER` containing real System
> Driver implementations under
> `system-test/<lang>/.../testkit/driver/adapter/shop/`.

**Why this is wrong now.** The agent never creates the commit; the
BPMN's `commit_phase` call_activity does. Phrasing the produced
artefact as "Commit X containing Y" reads as "the agent commits this"
— but what the doc actually needs to convey is **the state of the
working tree after the WRITE step**, regardless of who later turns
that state into a commit. Reword to:

> State after WRITE: real System Driver implementations exist under
> `system-test/<lang>/.../testkit/driver/adapter/shop/`. (The
> orchestrator turns this into a commit; the agent does not.)

— or, more tersely, just describe the file-tree state and drop the
parenthetical (the heuristic itself is the point: state, not commit
bookkeeping).

**Overlap with sister plan.** This is the same edit the sister plan's
Phase 3a step 3 already prescribes (*"Update the `## What it
produces` blurb at the top so it stops describing the COMMIT as if
the agent does it"*). Articulated here because it sharpens the
heuristic and surfaces the broader pattern: any `Commit <Ticket> |
<PHASE>` wording outside the conventions docs is the same
duplication — the message string is BPMN-constructed. Treat all such
occurrences uniformly.

### Example 4 — Build / start / test command blocks inside WRITE sections of `at-green-system.md` (and parallels)

`at-green-system.md:51-56` and `:61-66` instruct the agent to:

```bash
gh optivem system build --rebuild
gh optivem system start --restart
gh optivem test run --suite <acceptance-api> --test <TestMethodName>
```

…during the WRITE phase, with a "If tests fail, fix the backend until
the tests pass" loop wrapped around it. The preamble forbids every
one of these commands; the verification loop the prose describes is
literally the BPMN's RUN_TESTS → fix-verify dispatch pattern.

**Why this is wrong now.** The agent does WRITE only. The
build-rebuild-restart-run cycle is owned by the BPMN's COMPILE /
BUILD / START / RUN service_tasks, and any failure routes to
`atdd-fix-verify` via re-dispatch. The agent doesn't loop on its own.

**Same pattern likely present in.** `at-red-system-driver.md:49-50`,
`ct-red-test.md:44,49`, `ct-red-external-driver.md:46`,
`ct-green-stubs.md:44` all embed `gh optivem test run --suite ...`
commands inside WRITE-step prose. Phase 1 finds every instance.

A second sub-pattern of the same category — the **file-scope prose**
inside WRITE sections (e.g. `at-red-system-driver.md:36`'s "File
scope: only files under `system-test/<lang>/.../testkit/driver/port/shop/`
and `…/adapter/shop/<channel>`. Do NOT touch `external/` siblings"
). This *is* an agent-imperative invariant (constrains what the
agent edits), so it **stays** by the heuristic — but it raises an
adjacent design question (see Q8) about whether the constraint
belongs in the doc body at all or should be promoted to a
machine-enforced frontmatter / `${allowed_roots}`-style restriction.
Out of scope for this plan; flagged as a follow-up.

**What replaces the prose, if anything.** The agent still needs to
know: write the implementation that makes the change-driven tests
pass; if you cannot, ask the user and do not patch around it via test
/ DSL / Driver edits. That invariant survives — but as a one-line
WRITE rule, not a multi-step CLI procedure.

## Heuristic to apply across all 16 process docs

For every sentence, list item, code block, or section in each process
doc, ask:

1. **Does the BPMN own this action?** (commit, compile, build, start,
   stop, run, disable, re-enable as a discrete step, tick, move,
   post-comment, dispatch.) → Drop the prose / block entirely. Do
   **not** "relocate" or "demote" — the BPMN config is the only
   source.
2. **Is the agent forbidden by the preamble from doing this?** (Any
   `git`, `gh issue`, `gh optivem test`, `gh optivem system`,
   compile-tool invocation.) → Drop.
3. **Is the only consumer of this prose a section / command being
   dropped?** (e.g. suite placeholders whose only consumers are
   stripped `gh optivem test run` blocks.) → Drop.
4. **Is this an agent-imperative WRITE invariant?** (Scope rule,
   ordering inside the WRITE step, "do not edit X", "declare Y
   before using it", "channel annotation choice", architecture-layer
   reference.) → Keep.
5. **Does it describe state the WRITE step produces?** (Tests in
   state X, code in state Y, regardless of who commits it.) → Keep,
   but reword anything phrased as "the agent commits X" to "after
   WRITE, the codebase is in state X".

## Affected files

All 16 docs under `internal/assets/global/docs/atdd/process/`. Phase 1
inventories every one; Phase 3 edits only those with leakage found.

**Convention docs (2)** — primary target of Examples 1 and 2:
1. `at-cycle-conventions.md`
2. `ct-cycle-conventions.md`

**Phase docs (8)** — overlap with the in-flight sister plan, which
strips REVIEW/COMMIT sections; this plan picks up the build/run/test
command blocks **inside** the WRITE sections of the same docs (and
any other BPMN-owned prose the sister plan leaves behind):
3. `at-red-test.md`
4. `at-red-dsl.md`
5. `at-red-system-driver.md`
6. `at-green-system.md`
7. `ct-red-test.md`
8. `ct-red-dsl.md`
9. `ct-red-external-driver.md`
10. `ct-green-stubs.md`

**Cycle / structural docs (6)** — not touched by the sister plan;
Phase 1 audits for the same patterns:
11. `cycles.md`
12. `task-and-chore-cycles.md`
13. `system-interface-redesign.md`
14. `shared-phase-progression.md`
15. `shared-ticket-status-in-acceptance.md`
16. `glossary.md`

**Diagram doc (1)** — Mermaid prose; Phase 1 confirms whether the
condensed-doc edits invalidate any diagram captions:
17. `diagram-phase-details.md`

## Open questions

### Q1 — Coordination with the in-flight sister plan (`20260515-1230-tighten-atdd-prompts-and-phase-docs-to-agent-scope.md`)

The sister plan is mid-execution (Picked up by `Valentina_Desk` at
`2026-05-15T11:09:20Z`; 9 prompt files currently modified in the
working tree). It edits the same 8 phase docs this plan touches.

**Options:**

- **(a) Sequence after.** Wait for the sister plan to land, then
  Phase 1 of this plan inventories what's left. Cleanest; no merge
  conflict risk. Slowest.
- **(b) Coordinate within sister plan.** Fold Examples 1–3 into the
  sister plan's scope (re-open it). Violates
  `feedback_new_plan_not_extend.md` — broadening scope means a new
  plan that cross-references the original, not editing the original.
- **(c) Run in parallel on disjoint files.** This plan touches the
  conventions docs + cycle/structural docs + diagram doc first
  (10 files) which the sister plan doesn't touch; phase-doc edits
  (8 files) wait until sister lands.

**Recommendation: (c).** Maximizes throughput without conflicts.
Phase 3 splits into Phase 3a (10 sister-untouched docs, can start
now) and Phase 3b (8 phase docs, gated on sister-plan completion).

### Q2 — Where does the `#<issue-number> | ` commit-message prefix go?

The conventions docs say every commit gets prefixed with
`#<issue-number> | ` when an issue number was provided. The BPMN's
`commitPhase` action does not implement this prefix. Three
possibilities:

- **(a) Drop the prefix rule.** It's stale; the design moved on; the
  conventions doc is just out of date. Phase 2 confirms by grepping
  recent commits — if none have the prefix, it's been deprecated.
- **(b) Add the prefix to `commitPhase`.** If the prefix is still
  desired, the fix is in the action, not the doc — read `issue_num`
  from context and prepend. Separate small wiring plan.
- **(c) Leave the rule in a BPMN-owned doc** (not the process docs)
  if it's a spec the implementation is supposed to meet but doesn't
  yet.

**TBD** — Phase 2 verifies which.

### Q3 — Does the channel-annotation rule (`@Channel(API)` / `@Channel(UI)`) survive in `at-red-test.md`?

Suite-name mapping is BPMN-only; channel annotation on the test
itself is agent-authored. The current `at-red-test.md` rule "annotate
each test with the channel it covers" is a legitimate WRITE
invariant. **Resolution: keep the channel-annotation rule in
`at-red-test.md`; drop the channel→suite-name mapping from the
conventions doc.**

Verify in Phase 1 that no other phase doc relies on the channel
mapping for an agent-relevant decision.

### Q4 — `cycles.md`'s `## Commit Handoff` and `## Resume Detection` sections

`cycles.md` has `## Commit Handoff` (line 276) and `## Resume
Detection` (line 282) sections that describe orchestrator-level
behaviour. Per the heuristic, these are BPMN-owned plumbing — but
`cycles.md` is the **architectural overview** doc, intended as a
reader's map of the pipeline. There's a legitimate case for keeping a
high-level "the orchestrator handles commits and resume" sentence
even if the per-phase mechanics live in the BPMN config.

**Options:**

- **(a) Drop both sections entirely.** Strict application of the
  heuristic.
- **(b) Collapse each to a one-line cross-reference** to the BPMN
  config / `task-and-chore-cycles.md`'s shared commit sub-process.
- **(c) Keep, on the grounds that `cycles.md` is overview-flavoured
  and the audience differs.**

**Recommendation: (b).** Preserves the navigational value of the
overview doc without duplicating the BPMN's authoritative spec. **TBD**
— defer to per-section read in Phase 1.

### Q5 — `shared-ticket-status-in-acceptance.md` — is the whole doc agent-relevant?

Quick scan of headings:
- `## Agents are CI-unaware`
- `## When the ticket enters IN ACCEPTANCE`
- `## Procedure (agent side)`
- `## Beyond IN ACCEPTANCE (human responsibility)`

The "Procedure (agent side)" heading suggests legitimate
agent-invariant content. The other three look like BPMN/human-process
narration. **TBD** — Phase 1 reads end-to-end and classifies bullet by
bullet.

### Q6 — `task-and-chore-cycles.md`'s `## Shared structural-cycle TEST` and `## Shared structural-cycle COMMIT`

These sections describe shared sub-process behaviour. Are they
agent-facing (the agent at chore/task time reads them to understand
what happens around its WRITE) or orchestrator-facing (BPMN config
docs)?

If the agent never needs to know what happens around its WRITE (per
sister plan Q9), then both sections are pure BPMN narration → drop.
**TBD** — confirm against sister plan's Q9 resolution.

### Q7 — `diagram-phase-details.md` Mermaid captions

The diagram doc has captions like `RUN_API[Run acceptance-api tests]`
(line 118) and `RUN_UI[Run acceptance-ui tests]` (line 122). These are
node labels in the BPMN flow diagram — they describe what the BPMN
does, not what the agent does. **Resolution: keep as-is.** The
diagram describes the pipeline; the BPMN owning those steps is
exactly the point.

But: if Example 2's `## Suite Selection` is dropped, do the diagram
captions need updating to use generic `Run API channel tests`
instead of the suite-placeholder name? Probably not — but flag in
Phase 1.

### Q8 — Should file-scope rules be machine-enforced via frontmatter rather than (or in addition to) doc-body prose?

User-raised during the walk-through of `at-red-system-driver.md`'s
`File scope: only files under system-test/<lang>/…/driver/port/shop/`
sentence. The current model is:

- The dispatcher already substitutes `${allowed_roots}` into the
  prompt body (per sister plan Q6 / `clauderun.go:443-447`).
- The agent reads the prose ("only files under X") and is trusted to
  honour it.
- The runtime does **not** enforce a file-system restriction; if the
  agent edits a file outside the allowed root, nothing rejects the
  edit.

The question is whether to promote file-scope from prose-trust to
machine-enforcement — e.g. an `allowed_paths:` / `restricted_paths:`
frontmatter declared by each agent, validated against `Edit` / `Write`
tool calls at dispatch time.

**Why this is out of scope here.** This plan condenses the docs; it
does not redesign the harness contract. Machine-enforced scope is a
significant framework change (parser + composer + per-tool-call
validation + per-agent migration) — the same category as sister plan
Q14's deferred frontmatter `reads:` / `inputs:` proposal. **Recommend
treating as an adjacent follow-up plan** alongside Q14's other
frontmatter additions.

**Phase-1 implication for *this* plan.** File-scope prose **stays** in
the doc bodies during the condense — it's an agent-imperative
invariant, not BPMN-owned plumbing. The frontmatter promotion is a
separate, additive change that can happen later without re-editing
the doc bodies (or, if the follow-up lands, can remove the prose at
that time).

## Items

### Phase 1 — Inventory across all 16 docs

For each doc, capture in this plan's Appendix:

- **Section / line range** of each leakage instance.
- **Leakage category** (BPMN-owned action / preamble-forbidden
  command / unreachable placeholder / agent-driven fix-loop /
  duplicate of in-flight sister plan's drops).
- **Disposition** (drop entirely / reword to invariant / keep).
- **Genuinely-new content under a dropped section** that must be
  preserved (per `feedback_drop_dont_relocate.md`: check if an
  upstream mechanism already covers it before relocating; usually
  drop, sometimes fold into a surviving WRITE rule).

Phase 1 produces the Appendix table. Do **not** edit any doc in
Phase 1.

### Phase 2 — Verification (read-only)

1. **Confirm `commitPhase` does not prepend `#<issue-number> | `.**
   Read `internal/atdd/runtime/actions/bindings.go:617-635`. If
   confirmed: Q2 resolves to either (a) drop the prefix rule from
   conventions, or (b) flag as a separate wiring follow-up. Phase 2's
   only job here is to surface which.
2. **Confirm suite placeholders' consumers in the codebase.**
   `grep -rn "<acceptance-api>\|<acceptance-ui>\|<suite-contract-real>\|<suite-contract-stub>" .`
   — if every match is either inside a `gh optivem test run` block
   the agent is forbidden to run, or inside the BPMN config / runtime
   action that consumes them programmatically, then dropping the
   `## Suite Selection` section is safe.
3. **Confirm `gh optivem system build/start/stop` and `gh optivem
   test run` are nowhere referenced in any prompt body** that the
   agent reads. (The sister plan's Q10 handles prompt-side; this is
   a cross-check that the process docs are the last remaining
   surface.)

Read-only — no code or doc changes.

### Phase 3a — Strip BPMN-owned and agent-unreachable prose from the 10 sister-untouched docs

For each of the 10 docs the sister plan doesn't touch (`at-cycle-conventions.md`,
`ct-cycle-conventions.md`, `cycles.md`, `task-and-chore-cycles.md`,
`system-interface-redesign.md`, `shared-phase-progression.md`,
`shared-ticket-status-in-acceptance.md`, `glossary.md`,
`diagram-phase-details.md`, plus any non-RED/GREEN-system phase doc
not in scope of the sister plan):

1. Apply the heuristic section-by-section per the Phase 1 Appendix
   disposition.
2. Drop entire sections (e.g. `## Commit Message Format`, `## Suite
   Selection`) where every line is BPMN-owned or unreachable.
3. Reword inline sentences that mix agent-imperative with BPMN
   narration — keep the imperative half, drop the narration half.
4. Update incoming cross-references — any `see [conventions.md §
   Commit Message Format]` link in a sibling doc needs to either
   point somewhere still meaningful or be dropped.

### Phase 3b — Strip the build/run/test command blocks (and parallels) inside WRITE sections of the 8 sister-plan phase docs

**Gated on the sister plan landing.** Once `at-red-test.md`,
`at-red-dsl.md`, `at-red-system-driver.md`, `at-green-system.md`,
`ct-red-test.md`, `ct-red-dsl.md`, `ct-red-external-driver.md`,
`ct-green-stubs.md` have had their REVIEW/COMMIT sections removed:

1. For each remaining WRITE section, remove `gh optivem system
   build/start/stop` and `gh optivem test run` command blocks
   entirely.
2. Replace the "run tests, if fail fix the code" loop wording with a
   one-line WRITE invariant: "implement the change-driven tests'
   passing path; if it can't be made to pass without editing tests /
   DSL / Drivers, ask the user."
3. Drop now-orphaned suite-placeholder references.
4. Cross-check `at-red-test.md` retains the channel-annotation rule
   (Q3) and that no other doc relies on the channel→suite mapping
   for an agent-relevant decision.

### Phase 4 — Verification

1. `grep -rn` for each of these strings across
   `internal/assets/global/docs/atdd/process/` — should return zero
   hits in agent-facing prose (some may remain inside the diagram
   doc's BPMN labels, which is fine):
   - `gh optivem test run`
   - `gh optivem system build` / `gh optivem system start` /
     `gh optivem system stop`
   - `./compile-all.sh` / `./gradlew build` / `npx tsc --noEmit` /
     `dotnet build`
   - `git commit` / `git add` / `gh issue close`
   - `## Commit Message Format`
   - `## Suite Selection`
   - `<acceptance-api>` / `<acceptance-ui>` /
     `<suite-contract-real>` / `<suite-contract-stub>` outside the
     diagram doc and outside the channel-annotation rule in
     `at-red-test.md`
2. `go test ./internal/atdd/...` (Windows test policy: `-p 2`).
3. Skim each updated doc end-to-end as if you were the agent —
   confirm every remaining sentence is either a WRITE invariant or
   a state description, never a procedure the agent is forbidden to
   execute.
4. Run one end-to-end cycle (one AT, one CT) and confirm BPMN
   behaviour unchanged — this plan is doc-only and should produce
   no behavioural delta.

## Out of scope

- **Editing agent prompts under `internal/assets/runtime/prompts/atdd/`.**
  The sister plan owns that surface; this plan is process-docs-only.
- **Wiring the `#<issue-number> | ` prefix in `commitPhase`.** If
  Phase 2 surfaces this as a real gap (Q2 → (b)), it's a separate
  small wiring plan, not this one.
- **Promoting `post_issue_comment` to a BPMN service_task.** Sister
  plan Q4 owns this; this plan trusts its decision.
- **GREEN / REFACTOR / DA phase docs that don't exist yet.** If the
  pipeline grows new phase docs later carrying the same patterns, a
  future sweep folds them in (per
  `feedback_new_plan_not_extend.md`).
- **Diagram regeneration.** `diagram-phase-details.md` is auto-
  generated from prose elsewhere; this plan does not regenerate.
- **Restructuring the docs' top-level organisation** (folder layout,
  filenames, table of contents). Pure content-condense only.

## Related

- **Sister plan (in flight, picked up by `Valentina_Desk` at
  `2026-05-15T11:09:20Z`):**
  `plans/20260515-1230-tighten-atdd-prompts-and-phase-docs-to-agent-scope.md`
  — strips REVIEW/COMMIT sections from 8 phase docs and trims agent
  prompts. This plan picks up the conventions docs, cycle/structural
  docs, and the build/run/test command blocks inside the same 8
  phase docs' WRITE sections.
- **BPMN commit action:**
  `internal/atdd/runtime/actions/bindings.go:617-635` —
  `commitPhase` constructs the commit message; the source of truth
  that makes the conventions docs' `## Commit Message Format`
  redundant.
- **BPMN flow config:**
  `internal/atdd/runtime/statemachine/process-flow.yaml` — owns
  every service_task / call_activity this plan attributes BPMN
  ownership to (commit, compile, build, start, stop, run, disable,
  tick, move, dispatch).
- **Shared preamble:** `internal/assets/runtime/shared/preamble.md`
  — explicit "agent must never run …" rule that makes the
  command-block prose in WRITE sections unreachable.
- **Shared footer:** `internal/assets/runtime/shared/session-end.md`
  — covers the STOP-HUMAN-REVIEW present-and-ask prompt; relevant
  context for why various "present what changed" prose elsewhere
  is also redundant.
- **Memory:** `feedback_drop_dont_relocate.md` — when refactoring
  ownership of a section, check whether an upstream mechanism
  already covers it; if so, drop entirely rather than demote /
  relocate. Direct guidance for Phase 3a/3b dispositions.
- **Memory:** `feedback_new_plan_not_extend.md` — this is a fresh
  plan, not an extension of the sister plan, despite scope overlap
  on the 8 phase docs. Cross-references in Related.
- **Memory:** `feedback_materialize_dont_expand.md` — when stripping
  derivations from docs, keep the derivation black-box (don't
  redesign the BPMN's commit/dispatch contract while you're at it).

## Appendix — Phase 1 inventory (populated during Phase 1)

For each doc, list every leakage instance with section, lines,
category, and disposition.

| Doc | Section / lines | Leakage category | Disposition | Notes |
|-----|-----------------|------------------|-------------|-------|
| at-cycle-conventions.md | `## Commit Message Format` (lines 11-19) | BPMN-owned (commitPhase) | Drop section entirely | Example 1; verify Q2 prefix gap |
| at-cycle-conventions.md | `## Suite Selection` (lines 3-9) | Unreachable (consumers are forbidden `gh optivem test run` blocks) | Drop section entirely | Example 2; verify Q3 channel-annotation rule survives in `at-red-test.md` |
| ct-cycle-conventions.md | `## Suite Selection` (lines 7-14) | Unreachable | Drop section entirely | Mirror of Example 2 |
| ct-cycle-conventions.md | `## Commit Message Format` (lines 16-24) | BPMN-owned | Drop section entirely | Mirror of Example 1 |
| at-green-system.md | WRITE step 2.b / 3.b command blocks (lines 51-56, 61-66) | Preamble-forbidden CLI + agent-driven fix-loop | Replace with one-line WRITE invariant | Example 3; Phase 3b (gated on sister plan) |
| at-red-system-driver.md | TBD (lines ~49-50 grep hit) | Preamble-forbidden CLI | TBD | Phase 3b |
| ct-red-test.md | TBD (lines ~44, 49 grep hits) | Preamble-forbidden CLI | TBD | Phase 3b |
| ct-red-external-driver.md | TBD (line ~46 grep hit) | Preamble-forbidden CLI | TBD | Phase 3b |
| ct-green-stubs.md | TBD (line ~44 grep hit) | Preamble-forbidden CLI | TBD | Phase 3b |
| at-red-test.md | TBD | TBD | TBD | Phase 1 reads end-to-end |
| at-red-dsl.md | TBD | TBD | TBD | Phase 1 reads end-to-end |
| ct-red-dsl.md | TBD | TBD | TBD | Phase 1 reads end-to-end |
| cycles.md | `## Commit Handoff` (line 276), `## Resume Detection` (line 282) | BPMN-owned plumbing | TBD per Q4 | Recommend collapse to cross-ref |
| cycles.md | Other sections | TBD | TBD | Phase 1 reads end-to-end |
| task-and-chore-cycles.md | `## Shared structural-cycle TEST`, `## Shared structural-cycle COMMIT` | TBD per Q6 | TBD | |
| task-and-chore-cycles.md | Other sections | TBD | TBD | |
| system-interface-redesign.md | `## SYSTEM INTERFACE REDESIGN - REVIEW (STOP)`, `## SYSTEM INTERFACE REDESIGN - TEST`, `## SYSTEM INTERFACE REDESIGN - COMMIT` | Mirror of sister plan's strip pattern | Drop sections entirely | Same shape as RED/GREEN phase docs |
| shared-phase-progression.md | TBD | TBD | TBD | Phase 1 reads end-to-end |
| shared-ticket-status-in-acceptance.md | `## Agents are CI-unaware`, `## When the ticket enters IN ACCEPTANCE`, `## Procedure (agent side)`, `## Beyond IN ACCEPTANCE (human responsibility)` | TBD per Q5 | TBD | "Procedure (agent side)" likely keep; others likely drop |
| glossary.md | TBD | Likely none (definitions, not procedure) | Likely keep | Phase 1 confirms |
| diagram-phase-details.md | `RUN_API` / `RUN_UI` Mermaid node labels (lines 118, 122) | BPMN flow diagram label | Keep (Q7) | Diagram describes pipeline; BPMN ownership is the point |
