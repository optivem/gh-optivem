# Agent prompt fixes — remarks from prompt-file review

## Origin / intent

Conversation with user (2026-05-26 14:48) walking through observed issues
in the agent prompt files under
`internal/assets/runtime/prompts/atdd/*.md`. This plan is an
accumulating list of remarks; items will be appended as the walk
continues.

## Scope

`internal/assets/runtime/prompts/atdd/*.md` only — the prompt bodies
shipped to writing/fix agents. No runtime, no schema, no orchestration
changes.

## Distinguishing principle (applies to all items below)

When a prompt mentions its surroundings, classify the reference:

1. **Caller-name plumbing (bad — strip).** Naming the parent HIGH /
   CYCLE / wrapper for context, when the agent's behaviour does not
   change based on that name. Example: "called from
   `write-and-verify-acceptance-tests` HIGH, which is called from step 1
   of `change-system-behavior` CYCLE via the `-fail` wrapper". The agent
   does the same job regardless. Pure plumbing — costs tokens, rots when
   orchestration is renamed, invites scope creep.
2. **Contract-driven branch (keep, but reword to describe input shape).**
   When the agent legitimately behaves differently per caller. Example:
   `implement-system-driver-adapters` does translation work under
   `change-system-behavior` (no Checklist) vs. structural reshape under
   `redesign-system-structure` (Checklist supplied). The behaviour
   branches on **Checklist present?** — that's the real contract. Prefer
   wording that names the input ("If the Checklist section is empty…")
   over wording that names the caller ("when invoked by the
   change-system-behavior CYCLE…"). Caller names are a hint, not the
   trigger.
3. **Generic "calling CYCLE" reference (keep).** When the prompt explains
   the contract abstractly without naming anyone — e.g. fix-* tasks
   saying "the calling CYCLE expected this command to succeed". Tells
   the agent what its caller will do with the result, not who that
   caller is. Survives orchestration rename.

Rule of thumb: **if the parent BPMN were renamed tomorrow, would the
prompt still be correct?** If no, the reference is plumbing — rewrite
or strip.

## Items

### 1. Strip caller-name plumbing from prompt bodies

**Files in scope:**

- `write-acceptance-tests.md:7` — "This task is called from the
  `write-and-verify-acceptance-tests` HIGH orchestration, which is
  called from step 1 of the `change-system-behavior` CYCLE (via the
  `-fail` wrapper)." → drop. The first half of the sentence ("The
  Acceptance Criteria below were parsed from the ticket body during
  intake — write tests for them directly.") already explains the
  contract.
- `write-acceptance-tests.md:43-45` — "downstream MID tasks in the same
  HIGH orchestration (`implement-dsl`,
  `implement-system-driver-adapters`) in the same CYCLE reuse this
  list…" → keep the **why** (downstream tasks have no other way to
  learn the test names, so re-emit the full set on every re-run) but
  drop the HIGH/CYCLE naming. The agent doesn't need to know which
  downstream task consumes the output, only that something does.
- `write-contract-tests.md:7` — "It is called from the
  `implement-and-verify-external-system-driver-adapters-contract-tests`
  HIGH orchestration when a `change-system-behavior` CYCLE detects that
  external system driver ports changed." → drop. State the contract:
  "Write contract tests for the external-system driver ports listed
  below" (or whatever the real input is).
- `refine-acceptance-criteria.md:12-16` — "This is the
  `refine-acceptance-criteria` MID task — the sole step of the
  `refine-backlog` CYCLE, which TOP `refine-ticket` calls during
  backlog grooming. It runs **before** any execution CYCLE: TOP
  `implement-ticket` later picks an execution CYCLE via its ticket-kind
  gateway (e.g. `change-system-behavior` for stories/bugs,
  `cover-system-behavior` for…)." → drop the whole call-graph paragraph.
  Replace with a one-line scope statement if anything is needed at all.
- `implement-dsl.md:12-14` — the `change-system-behavior` /
  `cover-system-behavior` callsite pairing. **Audit needed**: is this a
  contract branch (Item 1 keeps it) or pure plumbing (Item 1 strips it)?
  Read the surrounding lines before editing.

**Not in scope of this item** (multi-caller contract branches —
classified as type 2 in the principle above):

- `implement-system-driver-adapters.md:2,11,12,30,33` — Checklist
  present/absent genuinely branches behaviour. Reword to lead with the
  input ("If the Checklist section is empty…") and demote caller names
  to a parenthetical hint, but do not strip.
- `implement-external-system-driver-adapters.md:2,11,12,30,35` — same
  shape as above.
- `implement-system.md:2,10,11,29` — same shape as above.
- `implement-external-system-stubs.md:17` — escalation reference to
  "the calling CYCLE" is generic (type 3); keep as-is.
- `fix-*.md` files — every "calling CYCLE" reference is generic (type
  3); keep as-is.

**Acceptance:**

- `grep -nE 'HIGH orchestration|step \d of|via the .-fail. wrapper|in the same CYCLE'`
  over `internal/assets/runtime/prompts/atdd/*.md` returns no hits in
  the files listed under "Files in scope" above.
- The multi-caller branch files still describe their two modes, but
  lead with input shape (Checklist present/absent) rather than caller
  name.
- No tests broken; the prompts are inert assets, behaviour change is
  zero.

### 2. Scope-bound reads, not just writes

**Observation.** Agents read far more files than they need.
`scope:` (defined in `scope.md`, enforced in the prompt header) currently
only constrains **what an agent may modify**. Reads are unrestricted —
in fact the preamble (`preamble.md:33-36`) explicitly says:

> `Read`, `Grep`, and `Glob` against the working tree are fine — those
> are legitimate work, not rediscovery.

That line was written to permit legitimate task-driven reads while
banning context-rediscovery `git`/`gh` calls, but in practice agents
read it as a green light to browse.

**Runtime confirmation.** `preamble.md` is prepended to every dispatched
prompt at `internal/atdd/runtime/agents/embed.go:27,68`; `scope.md` is
inserted right after, between preamble and the per-task body. Editing
either file affects every agent. The user picked the preamble; both are
valid locations and the choice gets settled during refinement.

**Proposed rule (to add to `preamble.md`, replacing or following the
current "Read/Grep/Glob are fine" paragraph):**

> Read only files you actually need for the work. The scope listed in
> the prompt's `scope:` frontmatter is the **complete** set of paths
> you may read or modify, with two narrow additions:
>
> 1. Files this prompt explicitly tells you to `Read` (e.g. lines like
>    "Read `${references_root}/atdd/architecture/test.md`").
> 2. Files you must inspect to satisfy an explicit Step in the prompt
>    body — e.g. when a Step says "implement the DSL interface", you
>    may read the DSL interface file even if its path is not in
>    `scope:`, because the Step makes that read load-bearing.
>
> Do not browse the codebase, do not "get oriented", do not read
> sibling files for context, do not grep broadly. If you cannot do the
> work without reading something outside scope and outside the two
> exceptions above, emit a `scope_exception` block (same shape as the
> write-side exception in `scope.md`) and exit.

**Tension to resolve during refinement:**

- Some prompts (`write-acceptance-tests.md:25-27`,
  `implement-dsl.md`, etc.) explicitly `Read ${references_root}/...`
  files that are not in scope. Exception 1 above covers those. But it
  means the prompt-author has to be exhaustive — anything the agent
  needs must be either in `scope:` or in an explicit `Read` line. Worth
  auditing during execution.
- Greps. Targeted greps for symbols mentioned in the prompt (e.g. "find
  the `CustomerService` class") are clearly legitimate. Open-ended
  greps ("look for related tests") are exactly the over-reading the
  user is complaining about. Wording should make the test "is the
  thing you're searching for named by the prompt or required by a
  Step?" — if yes, fine; if no, scope_exception.
- `scope.md` vs `preamble.md`. The rule is conceptually a scope rule,
  so co-locating with the write-side rule in `scope.md` would be
  tidier. User picked preamble — defer the call to refinement.

**Acceptance:**

- New paragraph in `preamble.md` (or `scope.md`, per refinement
  decision) stating the read-side rule.
- The old "Read, Grep, and Glob against the working tree are fine"
  sentence at `preamble.md:33-36` is rewritten or removed — it
  currently contradicts the new rule.
- Spot-check one fresh dispatch (e.g. a `write-acceptance-tests` run
  on a small ticket) and confirm the agent's tool-use trace shows
  scope-bounded reads only.

### 3. Split `scope` into read-scope and write-scope

**Observation (extends Item 2).** Item 2 says "only read what's in
scope." But for several phases, the *write* scope is legitimately
wider than the *read* scope, because the agent writes placeholder
stubs into a layer it must not otherwise look at.

Concrete example — `write-acceptance-tests` (phase `AT_RED_TEST`):

```yaml
AT_RED_TEST: [at-test, dsl-port, dsl-core]   # internal/atdd/phase-scopes.yaml:25
```

The list is correct **as a write set**:

- `at-test` — the new acceptance tests being authored.
- `dsl-port` — new method signatures added to the DSL interface so
  the tests compile.
- `dsl-core` — new placeholder methods that throw `"TODO: DSL"`
  (per `write-acceptance-tests.md:16`, Step 2), so compilation
  works end-to-end.

But the same list is **wrong as a read set** — the agent does not
need to read existing `dsl-core` to author tests; doing so leaks
implementation context into a test-writing task. Read scope should
be:

```yaml
AT_RED_TEST:
  read:  [at-test, dsl-port]
  write: [at-test, dsl-port, dsl-core]
```

**Other phases with the same shape (read ⊊ write):**

- `CT_RED_TEST: [ct-test, dsl-port, dsl-core]` — same asymmetry.
- `LEGACY_AT_TEST: [at-test, dsl-port, dsl-core]` — same.
- `LEGACY_CT_TEST: [ct-test, dsl-port, dsl-core]` — same.

For every other phase in `phase-scopes.yaml` the lists likely
collapse (`read == write`); inventory needed during refinement.

**Schema change to `phase-scopes.yaml`:** support either a flat list
(shorthand for `read == write`) or an explicit `{read:, write:}` map
when they differ. Build-time validation in
`internal/atdd/phase_scopes_test.go` enforces:

- `read ⊆ write` (you can't write where you're not allowed to read —
  the inverse asymmetry isn't useful).
- Every layer name in either list resolves through
  `canonicalPathKeys()`.

**Knock-on changes:**

- `gh optivem process scope <phase>` (the CLI command referenced in
  every prompt's `scope:` frontmatter comment) needs to emit both
  sets — likely as two keys in the substituted frontmatter, e.g.
  `read_scope:` and `write_scope:`, or the single `scope:` block
  grows from a list into a map. **Open question for refinement**:
  one combined `scope:` map or two top-level keys?
- The preamble rule from Item 2 ("only read files in scope") binds
  to the **read** scope, not the write scope. Wording in Item 2 to
  be updated once Item 3 lands.
- The `scope_exception` block in `scope.md` should grow a `kind:` field
  (`read` vs `write`) so a read-side overreach and a write-side
  overreach are distinguishable on the way out.

**Acceptance:**

- `phase-scopes.yaml` schema supports the split; the four phases
  above declare distinct `read:` / `write:` lists.
- `gh optivem process scope <phase>` emits both sets.
- `scope.md` and `preamble.md` consume the right one for each
  side of the rule (writes → write-scope, reads → read-scope).
- Build-time test asserts `read ⊆ write` and rejects bare layer
  names that resolve to neither.
- One end-to-end rehearsal of `write-acceptance-tests` shows the
  agent reading only `at-test` + `dsl-port` paths (plus
  explicitly-named architecture refs from the prompt body), while
  still writing the `TODO: DSL` placeholders into `dsl-core`.

### 4. Render scope keys + resolved paths into the dispatched prompt

**Observation (presupposed by Items 2 + 3).** Items 2 and 3 assume the
agent can see which paths belong to its scope. Today it cannot:

- `internal/atdd/phase-scopes.yaml` holds the layer keys per phase
  (`[at-test, dsl-port, dsl-core]`).
- Keys resolve to real paths via `gh-optivem.yaml paths:` +
  `canonicalPathKeys()` in
  `internal/projectconfig/paths_defaults.go`.
- The per-prompt frontmatter is `scope: {}` (literally empty — see
  `write-acceptance-tests.md:5` and every other prompt that pins to
  layer keys). The CLI comment "query resolved scope:
  `gh optivem process scope <phase>`" is documentation for the
  human prompt-author, not data the agent ever receives.
- `scope.md` (prepended at dispatch via `embed.go:81-84`) tells the
  agent: "the set of paths its agent may modify, listed in the
  `scope:` frontmatter at the top of the prompt you are reading."
  But the frontmatter is empty, so the agent is being pointed at
  nothing.
- Enforcement is server-side only: `check-phase-scope` runs
  *after* the agent commits and diffs the tree. The agent itself
  never sees a path list at write time.

**Consequence — your "DSL interface" question.** When prose in the
prompt body says "DSL interface", the agent has no glossary mapping
that human phrase to the layer key `dsl-port`, and no
`dsl-port → acceptance-test/java/.../driver/` resolution visible
to it. It infers the path from filename patterns it sees during
its (unbounded — see Item 2) Reads, which is exactly the
over-reading loop we want to close.

**Design (settled during walk — refinement may revise):**

Combine SSoT-in-frontmatter (durable, project-independent) with
runtime-rendered resolution in the body (agent-visible, project-
specific). Two halves:

**Half 1 — frontmatter carries the keys, hardcoded.** Every
writing-agent prompt declares its scope as keys in the
frontmatter:

```yaml
---
model: sonnet
effort: medium
scope:
  read:  [at-test, dsl-port]
  write: [at-test, dsl-port, dsl-core]
---
```

The keys are the durable spec. They do not change per project —
`dsl-port` means the DSL Port layer regardless of which language
the project uses. Operators reading the prompt file see exactly
what the agent is and isn't allowed to touch, without running a
resolver.

`phase-scopes.yaml` either disappears (frontmatter becomes the
sole SSoT) or shrinks to a generated index (settle during
refinement — see "Open questions" below).

**Half 2 — dispatch renders a `## Scope` block in the body.**
At dispatch, the runtime reads the frontmatter scope, joins each
key against the project's `gh-optivem.yaml paths:`, and injects a
`## Scope` section with key: value bullets into the body the
agent actually sees:

```
## Scope

You may **read** files under these paths:

- `at-test`:  acceptance-test/java/src/test/java/.../tests/
- `dsl-port`: acceptance-test/java/src/main/java/.../dsl/

You may **modify** files under these paths (superset of read):

- `at-test`:  acceptance-test/java/src/test/java/.../tests/
- `dsl-port`: acceptance-test/java/src/main/java/.../dsl/
- `dsl-core`: acceptance-test/java/src/main/java/.../core/
```

Source-on-disk shows keys; dispatched prompt shows both keys and
paths. The mapping is mechanical — no per-prompt prose to drift.

**Where does `## Scope` sit in the four-heading skeleton from
Item 6?** Three options for refinement:

- (a) Fifth top-level heading: `## Inputs` / `## Scope` /
  `## Steps` / `## Outputs` / `## Additional Notes`.
- (b) Sub-heading under `## Inputs`: `### Scope` alongside
  `### Acceptance Criteria` etc.
- (c) Part of `## Inputs` body, no sub-heading — first paragraph
  / first block.

Lean (b): the resolved scope **is** an input to the agent (it
tells the agent where it may read and write), so nesting under
`## Inputs` keeps the four-heading skeleton intact. Settle during
refinement.

**Parity enforcement — build-time test, not `config validate`.**

`gh optivem config validate` is end-user-facing — it validates
the operator's `gh-optivem.yaml`. The frontmatter-vs-`phase-
scopes.yaml` parity is a developer-time / build-time invariant.
Wrong audience.

The right home is the existing build-time guard at
`internal/atdd/phase_scopes_test.go`. That file already loads
all three sources (`phase-scopes.yaml`, `process-flow.yaml`, and
the agents package — `phase_scopes_test.go:10-13`) and asserts
reverse-FK invariants between them. Extend it with assertions of
the shape:

- For every writing-agent phase id in `process-flow.yaml`, the
  dispatched agent's prompt frontmatter declares a `scope:`
  block (or `scope: none`).
- The frontmatter's `scope.read` and `scope.write` lists equal
  the `phase-scopes.yaml` entry for the same phase (if
  `phase-scopes.yaml` survives this item).
- Every key in either list resolves through `canonicalPathKeys()`.
- `read ⊆ write` (cross-link to Item 3).

**Open questions for refinement:**

- **Does `phase-scopes.yaml` survive?** Three answers possible:
  (i) drop it — frontmatter is sole SSoT, no second file;
  (ii) keep it as a generated mirror (build target regenerates
  it from the frontmatter corpus); (iii) keep it as a parallel
  hand-edited file with parity-enforced equality. (i) is the
  cleanest; (ii) is convenient if other tooling already consumes
  `phase-scopes.yaml` shape; (iii) is two-places-to-edit
  friction.
- **Prompt-name ↔ phase-id mapping.** Frontmatter lives on the
  prompt file (keyed by prompt name, e.g.
  `write-acceptance-tests`). `phase-scopes.yaml` keys on BPMN
  phase id (e.g. `AT_RED_TEST`). The parity check needs the
  mapping. `process-flow.yaml` carries it
  (`UserTask.Agent` / `CallActivity.Params["agent"]` per
  `phase_scopes_test.go:38-52`); the existing guard already
  joins on it, so this is not new infrastructure.
- **Where does `## Scope` sit in the four-heading skeleton?**
  (a) / (b) / (c) above.
- **Per-language path resolution.** The dispatched `## Scope`
  block shows paths from the **current project's**
  `gh-optivem.yaml`. So a rehearsal against a Java project
  shows Java paths; against a TypeScript project, TS paths.
  The frontmatter keys are stable across languages.

**Existing mechanism this replaces — `${allowed_roots}`.**
`implement-system-driver-adapters.md:16-17` (and likely others)
already substitute write paths into the body via a loose prose
labelling:

```
Allowed write roots:
${allowed_roots}
```

— followed by an imperative line:

> Edit ONLY files under the "Allowed write roots" listed at the
> top of this prompt.

That mechanism does roughly what `## Scope` proposes, but worse:

- **Write-only.** "Allowed write roots" is silent on read-scope.
  Doesn't carry the Item 3 read/write split.
- **Prose, not structured.** No key labels, no `read:` /
  `write:` headings; just a substituted blob.
- **Duplicated guardrail line.** The "Edit ONLY files under…"
  imperative is repeated across prompts (audit during
  execution); collapses into the preamble (Item 5) or
  `scope.md` (Item 2) — wherever the scope-bound-reads rule
  lands.
- **Tells the agent the answer but not the question.** The
  substituted paths land in the body, but the keys
  (`driver-port`, `driver-adapter`, …) that connect those
  paths to scope-rule references in the prompt body and to
  `phase-scopes.yaml` are absent.

So Item 4's `## Scope` block is **not** a new mechanism — it is
a structured replacement of `${allowed_roots}` that adds the
keys, the read/write split, and a consistent location. The
runtime substitution for `${allowed_roots}` is already wired;
re-targeting it to emit the `## Scope` block is a refactor of
the rendering step, not a new resolver.

**Cleanup that drops out of Item 4 once `## Scope` lands:**

- Every `Allowed write roots:` / `${allowed_roots}` block in
  the prompt corpus is removed (the data moves into `## Scope`).
- Every "Edit ONLY files under the 'Allowed write roots'…"
  imperative is removed (the rule moves to the preamble /
  `scope.md` per Item 2).
- The `${allowed_roots}` entry in the substituted-parameter
  inventory either disappears entirely (if `## Scope` is
  rendered from frontmatter directly) or is renamed /
  refocused.

**Resolved-block shape (what the agent actually sees at dispatch),
regardless of A vs B:**

```
## Scope (resolved at dispatch)

You may **read** files under these paths:

- at-test   → acceptance-test/java/src/test/java/.../tests/
- dsl-port  → acceptance-test/java/src/main/java/.../dsl/

You may **modify** files under these paths (superset of read):

- at-test   → acceptance-test/java/src/test/java/.../tests/
- dsl-port  → acceptance-test/java/src/main/java/.../dsl/
- dsl-core  → acceptance-test/java/src/main/java/.../core/

Reading or writing outside this set requires a `scope_exception`
block.
```

**Glossary problem (separable but related).** Even with the
resolved block above, the prompt *body* still uses prose like
"the DSL interface" and "the DSL Core". The agent needs to map
those to layer keys (`dsl-port`, `dsl-core`). Options:

1. Rewrite prose to use keys directly: "the DSL Port layer (the
   files under `dsl-port`)" instead of "the DSL interface".
2. Add a glossary block to `scope.md` (or the resolved-block
   above): "DSL interface = `dsl-port`; DSL Core = `dsl-core`;
   System Driver Adapter = `driver-adapter`; …"
3. Both — keys in prose where natural, plus a small glossary
   for the human-name fallbacks.

Decide during refinement; (3) seems lowest-risk.

**Acceptance:**

- Every dispatched prompt (any phase that pins to layer keys)
  carries a resolved-paths block showing **both** `read` and
  `write` sets (per Item 3) with keys and resolved paths.
- Source-of-truth question (A vs B) decided and documented in
  the plan before execution starts; build-time invariants
  preserved either way.
- One end-to-end rehearsal of `write-acceptance-tests` shows
  the agent's dispatched prompt containing the resolved block,
  and the agent's tool-use trace shows reads bounded to the
  resolved `read:` paths only.
- Glossary mechanism (per the three options above) decided and
  applied to at least the four "core" layer names that show up
  in agent prose: DSL Port, DSL Core, System Driver, External
  System Driver.

### 5. Move universal tool-use rules into the preamble

**Observation.** Two paragraphs from `write-acceptance-tests.md` were
flagged as candidates for promotion:

1. *"When you have multiple edits to the same file, make them in one
   Write or one Edit-with-larger-context call rather than several
   sequential Edits. Each tool round-trip costs latency and tokens; a
   file's interface additions, impl methods, and wiring are typically
   one cohesive change."* — `write-acceptance-tests.md:21`
2. *"Do not present or wait for approval inside the agent."* —
   `write-acceptance-tests.md:23`

Neither is acceptance-test-specific:

- **Rule 1 (batch edits per file)** appears in only one prompt today,
  but the rule itself applies to every file-modifying agent — DSL
  implementation, driver adapters, system implementation, refactors.
  The single-location placement is a duplication-rot symptom, not
  evidence of task scope.
- **Rule 2 (no inline approval)** is already duplicated **9 times**
  verbatim across the prompt corpus:
  `implement-dsl.md:22`, `implement-system-driver-adapters.md:25`,
  `implement-system.md:24`,
  `implement-external-system-driver-adapters.md:25`,
  `disable-tests.md:44`, `enable-tests.md:43`,
  `refine-acceptance-criteria.md:75`, `write-contract-tests.md:16`,
  `write-acceptance-tests.md:23`. That's textbook preamble material.

**Relationship to existing preamble content.** The preamble
(`preamble.md:38-40`) currently says:

> When the work is done, do not summarise and do not commit — exit
> cleanly. The orchestrator drives compile, test runs, disabling,
> and commits as separate service tasks; the agent must never run
> `git commit`, `git add`, `gh issue close`, the compile commands,
> or the test commands.

This covers "don't commit / don't summarise" but is silent on
"don't present, don't wait for approval" and silent on "batch
edits per file". Both belong in the same neighbourhood.

**Proposed preamble additions:**

Append to the "Don't commit, don't summarise" section (or extend
the title to "Don't commit, don't summarise, don't ask"):

> Do not present a plan and wait for approval inside the agent.
> The orchestrator gates approvals between phases; an agent that
> stops mid-dispatch to ask the operator something will hang the
> pipeline. If you genuinely cannot proceed (e.g. an ambiguous
> Acceptance Criterion, an out-of-scope edit required, a
> contradiction between two inputs), emit the appropriate
> structured exit block (`scope_exception` from `scope.md`, or a
> task-specific `blocker:` block when defined) and exit.

Add a new short section "Edit cohesion":

> When you have multiple edits to the same file, make them in one
> `Write` or one `Edit` call with a larger context window rather
> than several sequential `Edit`s. Each tool round-trip costs
> latency and tokens; a file's interface additions, impl methods,
> and wiring are typically one cohesive change.

**Strip from prompt bodies after promotion:**

- Rule 2 (no inline approval): delete the line from all 9 prompts
  listed above.
- Rule 1 (batch edits): delete `write-acceptance-tests.md:21`.

**Acceptance:**

- `preamble.md` carries both rules (in the worded forms above or
  refined equivalents).
- `grep -nF 'Do not present or wait for approval' internal/assets/runtime/prompts/atdd/*.md`
  returns zero hits.
- `grep -nF 'multiple edits to the same file' internal/assets/runtime/prompts/atdd/*.md`
  returns zero hits.
- Existing `embed_test.go` golden assertions (if any) updated to
  reflect the new preamble content.

### 6. Standardise the prompt skeleton: `## Inputs` / `## Steps` / `## Outputs` / `## Additional Notes`

**Observation.** `write-acceptance-tests.md` uses `## Acceptance
Criteria` as the heading for the substituted ticket-body block.
Other writing-agent prompts use different headings — `## Checklist`,
or no heading at all — because the substituted variable differs
(`${acceptance_criteria}` vs `${checklist}`).

The result: the prompt corpus has no consistent skeleton. An
operator (or an agent inspecting another prompt) cannot grep for
a common heading to find "what was substituted in", nor predict
where re-run guidance lives, nor where the `outputs:` YAML block
will appear.

**Proposed skeleton (four canonical headings, applies to every
writing-agent prompt):**

```
## Inputs

${acceptance_criteria}   ## or ${checklist}, ${verify_results}, etc.

## Steps

1. …
2. …
3. …

## Outputs

```
outputs:
  test_names:
    - …
```

## Additional Notes

- If your previous run didn't compile, instead fix the broken /
  missing piece in your prior edits (forgotten DSL stub, typo,
  signature mismatch) and fix it minimally. Do not change test
  intent.
- …
```

**What each heading is for (definitions — keep tight so the
skeleton doesn't drift):**

- **`## Inputs`** — every value the prompt expects to be
  substituted at dispatch (`${acceptance_criteria}`,
  `${checklist}`, `${verify_results}`, `${changed_files}`, …).
  If multiple inputs are substituted, nest under `### …`
  sub-headings or list them in a short bulleted preamble. **The
  phase-specific input shape is in the substituted variable, not
  the heading.**
- **`## Steps`** — the numbered sequence of actions the agent
  performs. Mechanical, testable. Each step ends in a verifiable
  state. **`## Steps` contains ONLY the numbered list (1., 2.,
  3., …) and nothing else** — no interleaved prose paragraphs,
  no "if your previous run…" recovery notes, no tool-use rules,
  no Read directives. If something is not a numbered step, it
  belongs in `## Inputs`, `## Outputs`, `## Additional Notes`,
  or the preamble. Sub-bullets under a step (a/b/c) are fine
  when they refine that single step; mid-section paragraph
  breaks are not.
- **`## Outputs`** — everything the agent must emit at the end of
  its final response for downstream phases to consume. Two
  shapes possible (a prompt may have one, both, or neither):
  - **Structured `outputs:` YAML block** — e.g.
    `test_names`, `parsed_concepts`. Schema is part of the
    contract.
  - **Phase-output flags** — e.g. `DSL Interface Changed: yes|no`,
    `System Driver Interface Changed: yes|no`. Today
    `implement-dsl.md:39-46` has these under a separate
    `## Phase-output flags` heading; that heading collapses
    into `## Outputs`, with an `### Phase-output flags`
    sub-heading (or just a `### Flags` sub-heading) if the
    prompt has both YAML outputs and flags.

  Either way, the flags table and the YAML block both live under
  `## Outputs`; no parallel top-level heading for either.
- **`## Additional Notes`** — orthogonal-to-Steps guidance:
  re-run / recovery behaviour ("if your previous run didn't
  compile…"), exception escalation pointers, edge-case
  clarifications. Optional, but when present, named exactly this
  so its location is predictable.

**What `## Additional Notes` is NOT for** (anti-drift):

- Caller-name plumbing (covered by Item 1 — strip entirely).
- Tool-use rules that apply to every agent (covered by Item 5 —
  move to preamble).
- Reference-doc `Read …` lines (those stay in their own short
  block at the end of the body — or get reorganised under
  `## Inputs` once Item 7's audit decides which survive; settle
  during refinement).
- Anything that should be a numbered `## Steps` entry but the
  prompt-author was unsure where to put it.

**Files in scope** — every prompt the corpus today (every prompt
needs the skeleton, even if it ends up with `## Additional Notes`
empty / absent):

- `write-acceptance-tests.md` (`## Acceptance Criteria` →
  `## Inputs`; lift the "if your previous run didn't compile…"
  paragraph into `## Additional Notes`).
- `write-contract-tests.md` (audit current heading).
- `implement-dsl.md`, `implement-system-driver-adapters.md`,
  `implement-external-system-driver-adapters.md`,
  `implement-system.md`, `implement-external-system-stubs.md`
  (audit current headings — most likely have `## Checklist` or
  no input heading at all; each has its own "if you've run
  this before…" wording that lifts into `## Additional Notes`).
- `disable-tests.md`, `enable-tests.md` (likely substitute test
  targets — confirm).
- `refine-acceptance-criteria.md` (audit).
- `fix-*.md` (audit — they substitute `${verify_results}`,
  `${command}`, etc.; today they have richer structure with
  Diagnosis Protocol etc. — refinement to decide whether the
  four-heading skeleton applies or fix-* keeps its own).

**Acceptance:**

- Every writing-agent prompt follows the four-heading skeleton
  (`## Additional Notes` optional but, when present, named
  exactly this).
- `grep -nE '^## (Acceptance Criteria|Checklist)$' internal/assets/runtime/prompts/atdd/*.md`
  returns zero hits as top-level headings (sub-headings under
  `## Inputs` are fine).
- Re-run / recovery wording (today inline in
  `write-acceptance-tests.md:19` and similar lines in other
  prompts) lives under `## Additional Notes`, not interleaved
  with the Steps.
- The fix-* corpus has a refinement decision: adopt the
  four-heading skeleton, or carry its own (heavier) structure —
  documented in the plan before execution.

### 7. Audit reference-doc Reads per phase

**Observation.** Most writing-agent prompts contain a block of
`Read ${references_root}/...` directives near the end of the prompt
body. Inventory:

- `write-acceptance-tests.md:25-27` — `test.md`, `dsl-core.md`,
  `language-equivalents/${language}.md`.
- `write-contract-tests.md:18-20` — `test.md`, `dsl-core.md`,
  `language-equivalents/${language}.md`.
- `implement-dsl.md:24-26` — `dsl-core.md`, `driver-port.md`,
  `language-equivalents/${language}.md`.
- `implement-system-driver-adapters.md:35-37` — `driver-port.md`,
  `driver-adapter.md`, `language-equivalents/${language}.md`.
- `implement-external-system-driver-adapters.md:38-41` —
  `system.md`, `driver-port.md`, `driver-adapter.md`,
  `language-equivalents/${language}.md`.
- `implement-system.md:40-42` — `system.md`, `driver-port.md`,
  `driver-adapter.md`.
- `disable-tests.md:46`, `enable-tests.md:45` —
  `language-equivalents/${language}.md` only.

**The docs are short** — total line counts:
`driver-adapter.md` 41, `driver-port.md` 16, `dsl-core.md` 8,
`dsl-port.md` 36, `system.md` 42, `test.md` 12, per-language
~47. Token cost per dispatch is modest, not prohibitive, but the
Reads look copy-pasted by family rather than curated per phase.

**Per-phase relevance audit (proposed during refinement):**

For each prompt, ask: *does the agent's behaviour change if this
file is unread?* Concrete examples that suggest a curation, not
just a delete:

- `write-acceptance-tests` Reads `dsl-core.md` — but this phase
  only adds `throw "TODO: DSL"` placeholder stubs to dsl-core.
  The 8-line doc covers with-methods, string-only fields, alias
  fields, verification-class shape — all rules that bind on
  `implement-dsl`, not on placeholder authoring. **Probable
  drop.** Keep `test.md` (highly relevant: positive/negative
  test-class structure) and `language-equivalents/${language}.md`
  (highly relevant: per-language syntax).
- `implement-external-system-driver-adapters` Reads four files
  including `system.md`. This phase reshapes the external-system
  driver layer; does `system.md` (the SUT architecture doc) bind
  on it? Audit.
- `implement-system` Reads `driver-port.md` + `driver-adapter.md`
  but **does not** Read `language-equivalents/${language}.md`
  (per the grep above). Inconsistent with sibling phases — is
  this deliberate (system code is more idiomatic, no special
  syntax cheat-sheet needed) or a miss? Audit.
- `disable-tests` / `enable-tests` Read only the
  language-equivalents doc — correct for mechanical-marker work,
  but confirm no other doc encodes the marker grammar.

**Process for the audit:** for each prompt × each currently-Read
doc, classify as **(a) load-bearing — keep**, **(b) redundant or
out-of-phase — drop**, or **(c) currently missing but needed —
add**. Decide each case in the refinement walk.

**Default disposition (user signal during walk, 2026-05-26):** the
operator's sense is the current Reads do more harm than good
right now — the doctrine in those docs is not load-bearing
relative to the substituted ticket inputs, and the agent treats
them as low-signal context. **Default to drop unless someone in
refinement defends a specific Read** (with a concrete "the agent
will write X wrong without this doc" argument). This inverts the
usual presumption — keep-by-default is the safe choice for prose
content, but here the corpus shape suggests the Reads were
copy-pasted by family rather than curated, so the default-keep
presumption isn't earned.

Two consequences if default-drop wins:

- Most prompts end up with zero `Read ${references_root}/...`
  directives — the references-root tree stays as project
  documentation but isn't pulled into every dispatch.
- The `${references_root}` parameter may become unused across
  the prompt corpus, in which case its substitution wiring can
  be removed (per the inventory in Item 8).

**Relationship to other items:**

- Item 2 (scope-bound reads) needs to allowlist whatever survives
  this audit, so the references-root Reads must be enumerated
  in the per-prompt frontmatter or in a shared block, not just
  emitted in prose. (Otherwise the read-scope rule fights the
  prompt's own Read directives.)
- Item 5 (move universal rules to preamble) does **not** apply
  here: the references-root Reads are per-phase by definition;
  they belong in the prompt body, just curated.

**Acceptance:**

- Each prompt's Read block is documented as the result of an
  explicit audit (`keep / drop / add` decisions captured in the
  plan during refinement).
- `dsl-core.md` removed from `write-acceptance-tests.md` Reads
  unless refinement explicitly justifies it.
- No prompt Reads a doc that another phase in the same CYCLE
  already authoritatively binds on (no copy-paste-by-family).

### 8. Strip HTML "comment" pseudo-hiding; document parameters in the body

**Observation.** `implement-dsl.md:8-16` carries a block that looks
like a hidden comment:

```html
<!--
Parameter: ${touches-system-driver} (boolean). Gates whether this
invocation may add methods to the System Driver port and emit the
`System Driver Interface Changed` flag. Callers from the
implement-and-verify-dsl HIGH on the AT side (change-system-behavior
CYCLE) pass `true`; callers on the CT side (cover-system-behavior
CYCLE) pass `false`, since their DSL work is bounded to the
external-system-driver port.
-->
```

Two problems with this shape:

1. **HTML comments are not hidden from Claude.** The agent reads
   the entire prompt body the orchestrator dispatches, comment
   syntax or not. So the framing "this is internal documentation
   the agent won't see" is false. The block is just documentation
   that happens to be styled with `<!-- -->` brackets, paying no
   rendering benefit and confusing prompt-authors about what the
   agent actually consumes.
2. **The content is caller-name plumbing (Item 1) bolted to
   parameter documentation that the agent genuinely needs.** The
   second half of the block ("Callers from the
   implement-and-verify-dsl HIGH on the AT side …") is the same
   pattern Item 1 strips. The first half ("`${touches-system-
   driver}` (boolean). Gates whether this invocation may add
   methods to the System Driver port…") is real, load-bearing
   parameter semantics — Steps 2 and 3 of the same prompt branch
   on this parameter, but the body never explains what the
   parameter means.

**Inventory.** Single hit today
(`grep -nE '^<!--' internal/assets/runtime/prompts/atdd/*.md`
returns only `implement-dsl.md`), but the underlying issue —
**no canonical home for substituted-parameter documentation** —
applies to every prompt that takes parameters. Inventory of
substituted variables across the corpus:

```
${acceptance_criteria}   ${allowed_roots}         ${architecture}
${changed_files}         ${checklist}             ${command}
${command_exit_code}     ${command_stderr_tail}   ${disable_targets}
${driver-adapter}        ${driver-port}           ${external-system-driver-adapter}
${external-system-driver-port}                    ${failing-task-name}
${language}              ${loop}                  ${missing-outputs}
${parsed_concepts}       ${phase}                 ${prev_phase}
${references_root}       ${system-test-path}      ${ticket_id}
${touches-system-driver} ${verify_results}        ${violating-paths}
```

26 distinct parameters; only one of them
(`${touches-system-driver}`) has any body-level documentation at
all (and that one is hidden in a fake HTML comment).

**Proposed fix.**

1. Strip the HTML comment from `implement-dsl.md:8-16` entirely.
2. Add a `### Parameters` sub-heading under `## Inputs` (per
   Item 6's four-heading skeleton) for every prompt whose body
   branches on a substituted parameter. Document each parameter
   as `name (type): semantics — what the agent must do
   differently based on the value`, with no caller names.
3. For substituted parameters whose body does NOT branch on
   the value (`${language}`, `${references_root}`,
   `${ticket_id}`, etc.), no documentation needed — the
   parameter is just a string interpolation; the agent doesn't
   need semantics to use it correctly.

**Concrete rewrite for `implement-dsl.md`:**

```
## Inputs

${parsed_concepts}   ## or whatever the DSL agent receives

### Parameters

- `${touches-system-driver}` (boolean):
  - `true` — you may add prototype methods to the System Driver
    port and you must set the `System Driver Interface Changed`
    flag in `## Outputs`.
  - `false` — the System Driver port is out of scope for this
    invocation. Touch only the External System Driver port;
    set only the `External System Driver Interface Changed`
    flag.
```

— with no mention of which CYCLE passes which value. The
parameter description tells the agent **what to do** based on the
value, not **who** is passing it.

**Cross-links.**

- Item 1 strips the caller-name half of the existing HTML
  comment (`implement-and-verify-dsl HIGH on the AT side`,
  `change-system-behavior CYCLE`, etc.).
- Item 6 introduces `## Inputs` and the four-heading skeleton;
  this item adds `### Parameters` as a sub-heading under
  `## Inputs`.
- This item also resolves the "what's the home for ${variable}
  semantics" question implicitly raised by Item 4's per-language
  resolution and Item 7's per-phase reference-doc curation.

**Acceptance:**

- `grep -nE '^<!--' internal/assets/runtime/prompts/atdd/*.md`
  returns zero hits.
- Every prompt whose `## Steps` branches on a substituted
  parameter has a `### Parameters` sub-heading documenting that
  parameter's semantics, stripped of caller names.
- No prompt documents `${language}` / `${references_root}` /
  `${ticket_id}` / similar pure interpolation — those don't
  warrant a `### Parameters` entry.
- Inventory above is reconciled: every parameter the body
  branches on is documented; every parameter the body does not
  branch on is not.

### 9. Every prompt declares `scope:` in frontmatter — and the multi-caller case

**Observation.** Of 17 prompts under
`internal/assets/runtime/prompts/atdd/`, **10 have no `scope:`
field in frontmatter at all**:

```
MISSING  implement-external-system-driver-adapters.md
MISSING  implement-system.md
MISSING  implement-system-driver-adapters.md
MISSING  refactor-system.md
MISSING  refactor-tests.md
MISSING  fix-command-failed.md
MISSING  fix-missing-output.md
MISSING  fix-scope-diff.md
MISSING  fix-unexpected-failing-tests.md
MISSING  fix-unexpected-passing-tests.md
```

The build-time guard at `internal/atdd/phase_scopes_test.go`
requires "every writing-agent phase id in `process-flow.yaml` is
either in `phase-scopes.yaml` or declares `scope: none` in its
prompt frontmatter" — but it doesn't require a `scope:` field on
**every** prompt. The 10 missing files slip through.

This is **a precondition** for Item 4 (frontmatter as SSoT
carrying the keys): you can't move SSoT to the frontmatter if
half the corpus is silent about its scope.

**Two sub-cases the 10 missing prompts fall into:**

**Sub-case A — multi-caller prompts (5 files).**
`implement-system-driver-adapters`,
`implement-external-system-driver-adapters`,
`implement-system`, `refactor-system`, `refactor-tests`. These
serve more than one caller with different scopes per caller —
e.g. `implement-system-driver-adapters` runs under both
`change-system-behavior` (translation work, no Checklist) and
`redesign-system-structure` (structural reshape, Checklist
supplied). The phase-scopes.yaml mapping for these:

- `AT_RED_SYSTEM_DRIVER: [driver-port, driver-adapter]`
- `SYSTEM_INTERFACE_REDESIGN_CYCLE: [system-path, driver-adapter]`

— **different sets**. So a single flat `scope:` block in the
frontmatter can't represent both. Three design options to
surface during refinement:

- **A1. Per-caller scope in the frontmatter.**
  ```yaml
  scope:
    by-caller:
      AT_RED_SYSTEM_DRIVER:
        read:  [driver-port, driver-adapter]
        write: [driver-port, driver-adapter]
      SYSTEM_INTERFACE_REDESIGN_CYCLE:
        read:  [driver-port, driver-adapter]
        write: [system-path, driver-adapter]
  ```
  Pro: complete information at the prompt-author seat. Con:
  frontmatter grows; dispatch must select which branch applies.
- **A2. One prompt per caller (split the file).** Drop
  multi-caller dispatch entirely. Each prompt serves exactly
  one caller; differences (Checklist present/absent etc.) move
  into per-prompt body. Pro: simplest scope model. Con: code
  duplication across siblings.
- **A3. Union scope at dispatch.** Frontmatter declares the
  **union** of all callers' scopes; the dispatch pipeline
  narrows to the current caller's scope by reading
  `phase-scopes.yaml` for the phase id. Pro: minimal frontmatter
  change; runtime narrows. Con: source-on-disk scope is wider
  than any actual dispatch, making operator inspection
  misleading.

Item 4 currently presumes a flat `scope:` block. This item is
where multi-caller gets resolved.

**Sub-case B — fix-* / recovery prompts (5 files).** The five
`fix-*` files diagnose failures and emit structured exit blocks
(`scope_exception`, `blocker:`, plain prose diagnoses); they
don't routinely write to the working tree. Their natural
declaration is `scope: none` (the doctrinal exemption already
defined in `scope.md`).

Quick audit needed:

- Do any fix-* prompts ever modify files? Today
  `fix-unexpected-failing-tests` and `fix-unexpected-passing-
  tests` may legitimately rewrite a test or SUT line as part of
  the diagnosis recovery — confirm by re-reading the prompt
  bodies during refinement. If yes, they need real scope
  keys, not `scope: none`.
- `fix-command-failed`, `fix-missing-output`, `fix-scope-diff`
  are diagnostic-only — almost certainly `scope: none`.

**Strengthen the build-time guard.** Extend
`phase_scopes_test.go` so it asserts:

- **Every** prompt under
  `internal/assets/runtime/prompts/atdd/*.md` declares
  `scope:` in frontmatter — either a concrete block (with
  `read:` / `write:` per Item 3, or `by-caller:` map per A1
  if that wins, or just keys per A3) or the literal
  `scope: none`.
- Reverse FK still holds: every concrete frontmatter scope
  key resolves through `canonicalPathKeys()`, and matches
  `phase-scopes.yaml` for the same phase id (per Item 4's
  parity check).

**Cross-links.**

- Item 1 already noted the multi-caller files
  (`implement-system-driver-adapters` et al.) keep their
  multi-caller branches but reword to lead with input shape.
  This item adds the **scope** dimension to the multi-caller
  problem: input shape (Checklist present/absent) is what the
  body branches on; the scope set is what the frontmatter
  needs to carry, and the two might not align cleanly.
- Item 3 (read/write split) applies inside each per-caller
  branch in option A1.
- Item 4 design assumes scope is present; this item makes
  that universally true.

**Acceptance:**

- All 17 prompts have a `scope:` field in frontmatter — either
  concrete (with the shape Item 3 + Item 4 + option A1/A2/A3
  settle on) or `scope: none`.
- Build-time guard in `phase_scopes_test.go` asserts presence
  on every prompt, not just every writing-agent phase id.
- Multi-caller design decision (A1 / A2 / A3) made during
  refinement and documented in the plan before execution.
- fix-* audit completed; each fix-* prompt declares the right
  side (`scope: none` for diagnose-only, concrete keys for
  diagnose-and-repair).

### 10. Mode detection belongs in BPMN, not in the prompt body

**Observation.**
`implement-system-driver-adapters.md:29-31` (Step 1, "Branch on
Checklist") asks the agent to inspect the substituted Checklist
section and self-identify which CYCLE called it:

> 1. **Branch on Checklist.**
>    (a) If the Checklist section above is empty or absent, you
>        are running under **change-system-behavior**: implement
>        the System Driver adapters for real — replace each
>        `TODO: System Driver` prototype with actual logic. […]
>    (b) If the Checklist is non-empty, you are running under
>        **redesign-system-structure**: update the matching
>        System Driver adapter(s) under
>        `${driver-adapter}/<channel>` to absorb the change […]

The orchestrator **already knows** which caller it dispatched
from — it picked one of two call sites in `process-flow.yaml`.
But it does not tell the agent; instead it substitutes
`${checklist}` (empty or populated) and asks the agent to
re-derive the caller from the substituted body.

This is the same anti-pattern family as Item 1 (caller-name
plumbing), with a twist: Item 1 strips *names* of orchestration
context that don't affect agent behaviour. This item targets
*derivation* of orchestration context that **does** affect
agent behaviour — but the derivation should happen in BPMN, not
the prompt.

**Files in scope** (audit during refinement — every multi-caller
prompt from Item 9 sub-case A is a candidate):

- `implement-system-driver-adapters.md:29-31` — confirmed
  Checklist-empty/non-empty branch.
- `implement-external-system-driver-adapters.md` — same shape
  per the agent prose at lines 2, 11, 12, 30, 35 (audit body).
- `implement-system.md` — same shape per the agent prose at
  lines 2, 10, 11, 29 (audit body).
- `refactor-system.md`, `refactor-tests.md` — audit for similar
  multi-caller branching.

**Three resolution options for refinement (ordered by lift):**

**Option I — BPMN passes an explicit `${mode}` parameter.**
The call-activity in `process-flow.yaml` substitutes a discrete
value:

```yaml
- id: IMPLEMENT_SYSTEM_DRIVER_TRANSLATION
  type: call-activity
  process: implement-system-driver-adapters
  params:
    mode: translation   ## or "reshape" at the redesign call site
```

The prompt branches on `${mode}` with no Checklist inspection
and no CYCLE names:

> 1. If `${mode}` is `translation`, replace each TODO with real
>    logic. If `${mode}` is `reshape`, apply the Checklist to
>    the adapter layer.

Pro: minimal disruption. Con: branching still lives in the
prompt body; scope still has to handle two cases (Item 9 A1).

**Option II — split into two prompts.** Each call site
dispatches a distinct prompt:

- `implement-system-driver-adapters-translation.md` (translation
  mode only)
- `implement-system-driver-adapters-reshape.md` (reshape mode
  only)

Pro: no branching anywhere — each prompt has one scope
(resolves Item 9 sub-case A as A2), one Steps block, no
Checklist detection. Cleanest information flow. Con: code
duplication across siblings; renames in `process-flow.yaml`.

**Option III — keep body branching, strip caller names.**
Branching stays in the prompt but the agent isn't asked to
identify a caller — only to act on input shape:

> 1. If `## Inputs` carries no Checklist, replace each TODO
>    with real logic. If `## Inputs` carries a Checklist, apply
>    it to the adapter layer.

Pro: lightest touch — only prose rewording. Con: orchestration
still has two implicit call modes; the empty-vs-non-empty
Checklist remains a load-bearing signal the BPMN side is
silent about.

**Cross-links.**

- **Item 1** strips caller *names*; this item targets caller
  *detection*. Both forms of orchestration leak.
- **Item 9 sub-case A** asks how multi-caller prompts model
  their per-caller scope. Option II here (split) collapses
  the question — no multi-caller, no per-caller scope. Option
  I narrows it to a `${mode}`-keyed lookup. Option III leaves
  it as-is.
- **Item 6 `## Steps`** — current Step 1 in
  `implement-system-driver-adapters.md` is a "Branch on
  Checklist" instruction with nested (a)/(b). Item 6's "Steps
  contains ONLY the numbered list" rule allows sub-bullets
  under a single step, so the current shape is technically
  compliant — but Option II eliminates the branch entirely
  (cleaner against Item 6).

**Mode-conditional rules extend beyond `## Steps`.** A second
instance of the same anti-pattern appears in `implement-system.md`
(audit during execution):

> 3. **Escalation when no Checklist is supplied.** If you cannot
>    make the tests pass without touching acceptance tests, DSL,
>    Driver interfaces, or Driver adapters, **stop and ask the
>    user** — do not patch around it. Needing to touch a frozen
>    layer signals that an earlier task was wrong; the user
>    decides whether to rewind.

Same shape: "If [shape of substituted inputs], do X". The agent
is asked to detect Checklist-absence and apply a mode-specific
escalation policy. Whichever Option (I / II / III) wins for the
Steps branch must apply the same way to mode-conditional
escalation rules. They're not a separate problem — they're the
same problem in a different section.

**Acceptance:**

- Decision on Option I / II / III made during refinement and
  recorded in the plan before execution.
- No prompt's `## Steps` contains text of the form "you are
  running under X CYCLE" (Item 1 + this item, together).
- No prompt's `## Additional Notes` (or wherever escalation
  rules land) keys on "no Checklist supplied" / "Checklist is
  empty" — escalation rules either come from `${mode}` (Option
  I), are unconditional in their split prompt (Option II), or
  key on `## Inputs` shape without CYCLE names (Option III).
- The agent never re-derives "which caller dispatched me" from
  the shape of substituted inputs unless the BPMN side cannot
  carry that signal explicitly (which it always can, by passing
  a parameter — so this acceptance criterion is "never").

### 11. "Do not modify X" rules collapse into scope

**Observation.** `implement-system-driver-adapters.md:32-33`
contains two "do not modify" lines:

> 2. **Driver-port guardrail.** Do NOT modify any file under
>    `${driver-port}/` casually. If an interface change is
>    unavoidable, STOP and present to the user: the method(s)
>    you want to change, why the adapter alone cannot absorb the
>    change, the proposed new signature(s). Wait for explicit
>    user approval before editing any `${driver-port}/` file.
> 3. Do not modify acceptance tests, DSL, Gherkin, or the
>    system surface from this task. The redesign caller invokes
>    `implement-system` separately for the surface change; the
>    change-system-behavior caller has tests/DSL/system already
>    in place.

`implement-system.md` has a similar "Escalation when no Checklist
is supplied. If you cannot make the tests pass without touching
acceptance tests, DSL, Driver interfaces, or Driver adapters,
stop and ask the user…" — same shape.

**Two cases inside these lines, both collapse into scope:**

**Case A — pure forbidden layers.** Step 3 in
`implement-system-driver-adapters.md` lists acceptance tests, DSL,
Gherkin, system surface as forbidden. These layers are **not in
this phase's write-scope** (`phase-scopes.yaml`
`AT_RED_SYSTEM_DRIVER: [driver-port, driver-adapter]`). Once Item
2's scope-bound-reads + Item 3's read/write split + Item 4's
rendered `## Scope` block are in place, the agent already knows
these layers are forbidden — it sees them as outside both `read`
and `write` in `## Scope`. The text in Step 3 is **redundant
with scope itself**.

**Case B — write-with-escalation.** Step 2 of the same prompt
says driver-port is in scope but writes there require operator
approval. This looked like a third scope bucket
(`write-with-escalation`) but actually collapses too: the
canonical escape hatch in `scope.md` is `scope_exception`. If
driver-port is moved **out of write-scope** (it's read-only by
default), then the agent that needs to write there emits
`scope_exception` — exactly the "STOP and present" behaviour the
Step 2 prose describes, but routed through the universal
mechanism rather than reinvented inline.

**Same logic applies to the escalation rule in
`implement-system.md` Step 3** ("if you cannot make the tests
pass without touching frozen layers, stop and ask"): redundant
with `scope_exception` once those layers are out of write-scope.

**Proposed rule (to add to `scope.md` or as preamble guidance):**

> The prompt does not need to enumerate forbidden layers in
> prose. `## Scope` (the rendered block from Item 4) is the
> complete contract:
>
> - To write a file: the file's path must fall under a path
>   listed in `## Scope` `write`. Anything else → `scope_
>   exception`.
> - To touch a layer that requires operator approval: that
>   layer is **not** in `write`. The agent emits
>   `scope_exception` and exits; the orchestrator surfaces it;
>   the operator approves or rejects. This is the universal
>   "STOP and present" path.

**Files in scope (audit during refinement):**

- `implement-system-driver-adapters.md:32-33` — strip Step 2's
  "Driver-port guardrail" prose; move driver-port out of
  `write-scope` if escalation is the real intent. Strip Step
  3 entirely; scope already encodes it.
- `implement-system.md` Step 3 (escalation when no Checklist) —
  strip; the AT / DSL / Driver layers are already not in this
  phase's write-scope.
- `implement-external-system-driver-adapters.md` — audit body
  for similar "do not modify" lines.
- `implement-external-system-stubs.md:17` — already references
  the calling CYCLE's escalation contract; audit whether the
  inline escalation language survives or collapses.
- Every other prompt — grep for "do not modify" / "do NOT
  modify" / "frozen layer" / "stop and ask" and audit.

**Cross-links.**

- **Item 3** (read/write split): the move from
  "write-with-escalation" to "read-only + scope_exception"
  requires the read/write split — without it, you can't
  express "the agent can see driver-port but can't write
  there."
- **Item 4** (rendered `## Scope`): the agent sees the
  complete contract in the dispatched body; no "don't modify"
  prose needed.
- **Item 10** (mode detection in BPMN): the "if no Checklist"
  escalations are the *Checklist-presence* anti-pattern; this
  item shows they're *also* redundant with scope.

**Acceptance:**

- `grep -nE 'Do not modify|Do NOT modify|frozen layer|stop and ask|guardrail' internal/assets/runtime/prompts/atdd/*.md`
  returns zero or near-zero hits after execution.
- Every previous "don't modify X" rule has been resolved one
  of three ways:
  - X is moved out of write-scope (Case B → universal
    `scope_exception` handles escalation).
  - X is already out of write-scope (Case A → rule was
    redundant; deleted).
  - X is in write-scope and the "don't modify" prose was
    wrong (rare — write-scope is right, prose is removed).
- `scope.md` is updated to make the "do not enumerate
  forbidden layers in prose" expectation explicit.

---

*Additional items to be appended as the walk continues.*
