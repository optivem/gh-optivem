# Agent prompt fixes — remarks from prompt-file review

> ✅ **Spinoff `20260526-1536-fold-phase-scopes-into-process-flow.md`
> fully landed** as commit `6b2fd9f` ("atdd: fold phase-scopes.yaml
> into process-flow.yaml node scope"). The spinoff plan file is
> deleted; the fold mechanism (inline `read:`/`write:` lists on
> `process-flow.yaml` writing-agent nodes, `Engine.Scope()` accessor,
> 5 build-time guards) is in main.
>
> **Re-aligned post-fold (2026-05-26):**
> - Prompt-only items (**1, 6**) — fully aligned, can execute
>   independently. (Items **5, 7, 8** landed; items **2, 2a** also done.)
> - Item **3** — schema landed via 1536; this item now scopes to
>   per-phase asymmetric data + `scope.md kind:` field (see item body).
> - Items **4, 9, 10, 11** — unchanged from refinement; pick up the
>   post-fold node-scope model where they reference scope data.

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

Audit across all 17 prompts done during refinement (2026-05-26).
Every reference to a parent CYCLE / HIGH / sibling-task / `-fail`
wrapper / step-N positioning is classified below and assigned a
concrete treatment — no audit-during-execution pockets remain.

**Group A — STRIP entirely (Type 1 plumbing).** The agent's
behaviour does not change based on these names; drop the prose.

- `write-acceptance-tests.md:7` — "called from the
  `write-and-verify-acceptance-tests` HIGH orchestration, which is
  called from step 1 of the `change-system-behavior` CYCLE (via the
  `-fail` wrapper)." The first half of the same sentence ("The
  Acceptance Criteria below were parsed from the ticket body during
  intake — write tests for them directly.") already states the
  contract. Drop the second half.
- `write-contract-tests.md:7` — "called from the
  `implement-and-verify-external-system-driver-adapters-contract-tests`
  HIGH orchestration when a `change-system-behavior` CYCLE detects that
  external system driver ports changed." Replace with the contract:
  "Write contract tests for the external-system driver ports listed
  below" (or whatever the real input is).
- `refine-acceptance-criteria.md:12-16` — the full call-graph paragraph
  naming MID/CYCLE/TOP and the ticket-kind gateway. Drop entirely.
  Replace with a one-line scope statement if anything is needed.
- `implement-external-system-stubs.md:17` — escalation hint names two
  specific sibling tasks: "an earlier task in the calling CYCLE (the
  `write-contract-tests` or `implement-dsl` step) was wrong." Strip
  the sibling-task names; the generic "calling CYCLE" framing around
  them is fine to keep (Type 3). Resulting wording: "an earlier task
  in the calling CYCLE was wrong" (or "the upstream WRITE phase").
- `implement-dsl.md:12-14` — caller-name half of the `<!-- … -->` HTML
  block ("Callers from the implement-and-verify-dsl HIGH on the AT
  side (change-system-behavior CYCLE) pass `true`; callers on the CT
  side (cover-system-behavior CYCLE) pass `false`"). The full HTML
  comment removal is owned by **Item 8**; the parameter semantics
  half of the same block lifts into the `### Parameters` sub-heading
  (Item 8's concrete rewrite). Item 1's contribution is the
  classification: caller-name half is Type 1 plumbing, retained
  parameter semantics is Type 2 contract — sans caller names.

**Group B — REWORD (Type 2 contract branch).** The agent legitimately
branches per caller, but the trigger is the input shape, not the
caller's name. Reword to lead with the input; strip CYCLE names.

- `write-acceptance-tests.md:43-45` — "downstream MID tasks in the same
  HIGH orchestration (`implement-dsl`,
  `implement-system-driver-adapters`) in the same CYCLE reuse this
  list…" Keep the **why** (downstream tasks need the test names; the
  test set must be re-emitted in full on every re-run because there
  is no other channel to learn it) but strip HIGH/CYCLE/sibling-task
  names. The agent doesn't need to know which downstream task
  consumes the output, only that something does.
- `implement-system-driver-adapters.md:2-3, 11-12, 30-31, 33`,
  `implement-external-system-driver-adapters.md:2-3, 11-12, 30-31, 35`,
  `implement-system.md:2-3, 10-11, 29` — three multi-caller prompts
  with the same shape (yaml header comment naming callers, callsite
  catalogue under a `Callers` heading, Step 1 branching on Checklist
  present/absent, plus various follow-on references).
  **Item 10 resolved to a verb split** — these 3 files lose their
  multi-caller mechanics entirely. Each splits into an `implement-*`
  variant (translation: fill TODOs) and an `update-*` variant
  (reshape: apply Checklist). After the split, each resulting prompt
  has one caller, no callsite catalogue, no branching prose, no
  CYCLE names. The Item 1 deliverable on these three current files
  is: strip everything that was multi-caller plumbing, since the
  files themselves are being split. The new `update-*` files
  inherit only the reshape algorithm; the existing `implement-*`
  files retain only the translation algorithm.
- `fix-unexpected-failing-tests.md:11` — "behaviour-preserving by
  definition (e.g. `refactor-system-structure`, `refactor-test-structure`,
  or the structural steps of `redesign-system-structure`)." Drop the
  `e.g.` list of specific CYCLE names; keep the abstract
  "behaviour-preserving caller class" framing. Line 59
  ("change-cycle WRITE policy") **stays** — that names a doctrine
  class of caller, not an orchestration; it would survive a BPMN
  rename.

**Group C — KEEP AS-IS.** No action under Item 1.

- All "calling CYCLE" references in `fix-command-failed.md`,
  `fix-missing-output.md`, `fix-scope-diff.md`,
  `fix-unexpected-passing-tests.md`, and the remaining lines of
  `fix-unexpected-failing-tests.md` not listed in Group B — Type 3
  generic; they explain the abstract contract without naming the
  caller; they survive BPMN renames.
- `fix-unexpected-failing-tests.md:59` "change-cycle WRITE policy" —
  doctrine class, not orchestration name. Keep.
- `refactor-system.md:26-27`, `refactor-tests.md:26-27` — reference
  the **ticket-kind taxonomy** (`task/system-redesign`,
  `task/system-refactor`, `story`, `bug`), not orchestration names.
  Out of scope of Item 1. (These are stable taxonomy refs that pass
  the BPMN-rename test.)
- `disable-tests.md`, `enable-tests.md` — no caller-name plumbing
  detected. Nothing to do.

**Acceptance:**

- `grep -nE 'HIGH orchestration|step \d of|via the .-fail. wrapper|in the same CYCLE'`
  over `internal/assets/runtime/prompts/atdd/*.md` returns no hits.
- `grep -nE '(change-system-behavior|cover-system-behavior|redesign-system-structure|refine-backlog|refine-ticket|implement-ticket|write-and-verify|implement-and-verify)'`
  over the same set returns no hits **except** for the YAML
  frontmatter / hardcoded scope keys that Items 4/9 own — i.e. zero
  hits in prose bodies.
- The three current multi-caller files split into `implement-*`
  + `update-*` pairs per Item 10. Each resulting prompt has zero
  CYCLE names in its body prose and no caller-name plumbing.
- `implement-external-system-stubs.md:17` no longer names
  `write-contract-tests` or `implement-dsl` as upstream siblings.
- `fix-unexpected-failing-tests.md:11` no longer carries the `e.g.`
  list of specific CYCLE names; the abstract "behaviour-preserving
  caller class" framing survives.
- No tests broken; the prompts are inert assets, behaviour change is
  zero.

### 2a. Precondition — `phase-scopes.yaml` fold

**✅ Done by `plans/20260526-1536-fold-phase-scopes-into-process-flow.md`**
(implementation landed in the working tree 2026-05-26; commit pending
at the time of this item's resolution).

Post-fold reality (confirmed by audit 2026-05-26):

- `process-flow.yaml` writing-agent nodes carry inline `read:` /
  `write:` lists.
- `phase-scopes.yaml` deleted; `Engine.Scope(processName) (read,
  write []string, ok bool)` accessor on `statemachine.Engine`.
- `phase_scopes_test.go` carries 5 guards: ReverseFK,
  LayersAreCanonical, NoDuplicatesPerList, NonEmptyLayerLists,
  ReadWriteShape.
- All `LEGACY_*` ids removed.
- `gh optivem process scope <phase>` emits the post-fold shape.

Where this plan's Items 3, 4, 9 referenced "remapped phase ids" —
substitute the actual post-fold MID names from `process-flow.yaml`
at execution time.

### 3. Split `scope` into read-scope and write-scope

**Status (re-aligned post-fold, 2026-05-26):** the **schema mechanism
landed via 1536** — `process-flow.yaml` writing-agent nodes already
carry inline `read:` / `write:` lists, the `Engine.Scope()` accessor
returns both, and the build-time guards (`ReadWriteShape`,
`NoDuplicatesPerList`, `NonEmptyLayerLists`, `LayersAreCanonical`)
enforce the explicit-lists rule.

**What this item now owns** (the data-tuning + scope.md change that
1536 deliberately left to the parent):

1. **Apply the asymmetric splits from the audit table below** to the
   5 phases where `read != write` is correct. The current implementation
   has `read == write` on every node (1536 seeded the schema with
   symmetric defaults; per-phase tuning is parent-plan work per the
   1536 "Out of scope" note).
2. **Add a `kind:` field** (`read` vs `write`) to the
   `scope_exception` block in `internal/assets/runtime/shared/scope.md`
   so a read-side overreach and a write-side overreach are
   distinguishable on the way out. (Audit confirmed: no `kind:` field
   today.)
3. **Tighten Item 11 Case B** — moving driver-port out of `write` on
   the 3 driver-port phases (rows 3-5 below) makes the inline
   "Driver-port guardrail" prose redundant; the universal
   `scope_exception` mechanism handles escalation.

The audit and rule below remain valid as the per-phase data spec.

**Observation (extends Item 2).** Item 2 says "only read what's in
scope." But for several phases, the *write* scope is legitimately
wider than the *read* scope, because the agent writes placeholder
stubs into a layer it must not otherwise look at. Conversely, for a
few phases the *read* scope is legitimately wider than the *write*
scope (the agent must read a layer to know its shape but must not
modify it — driver-port guardrails).

Concrete example — `write-acceptance-tests` (current stale ID
`AT_RED_TEST`):

```yaml
AT_RED_TEST: [at-test, dsl-port, dsl-core]   # pre-Item-2a stale ID
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
implementation context into a test-writing task. The split:

```yaml
<NEW_ID_FOR_AT_RED_TEST>:
  read:  [at-test, dsl-port]
  write: [at-test, dsl-port, dsl-core]
```

**Schema rule (settled during refinement):** every phase declares
`read:` and `write:` **as two separate lists, always explicit, no
flat shorthand**. Duplication accepted when they match.
**No subset constraint** between them — neither `read ⊆ write` nor
`write ⊆ read` is enforced, because both directions of asymmetry
occur in practice (see audit below). Build-time validation in
`internal/atdd/phase_scopes_test.go` enforces only that every
layer name in either list resolves through `canonicalPathKeys()`.

**Audit — phases needing asymmetric split** (using current stale
IDs; substitute remapped IDs from Item 2a at execution time):

| Phase (stale ID) | Current (write) | Proposed read | Proposed write | Why asymmetric |
|---|---|---|---|---|
| `AT_RED_TEST` | `[at-test, dsl-port, dsl-core]` | `[at-test, dsl-port]` | `[at-test, dsl-port, dsl-core]` | Test-writer adds `TODO: DSL` placeholders to dsl-core; doesn't need to read existing dsl-core |
| `CT_RED_TEST` | `[ct-test, dsl-port, dsl-core]` | `[ct-test, dsl-port]` | `[ct-test, dsl-port, dsl-core]` | Same shape — CT side |
| `AT_RED_SYSTEM_DRIVER` | `[driver-port, driver-adapter]` | `[driver-port, driver-adapter]` | `[driver-adapter]` | Driver-adapter implementer reads driver-port to see what to implement; the "Driver-port guardrail" prose in `implement-system-driver-adapters.md:32` collapses into scope (Item 11 Case B) |
| `CT_RED_EXTERNAL_SYSTEM_DRIVER` | `[external-system-driver-port, external-system-driver-adapter]` | `[external-system-driver-port, external-system-driver-adapter]` | `[external-system-driver-adapter]` | Same shape — external driver side; `implement-external-system-driver-adapters.md:35` collapses |
| `SYSTEM_INTERFACE_REDESIGN_CYCLE` | `[system-path, driver-adapter]` | `[system-path, driver-adapter, driver-port]` | `[system-path, driver-adapter]` | implement-system reshapes the surface; reads driver-port to see what must not change. `implement-system.md:36` Driver-port guardrail collapses |

**Symmetric phases (read == write)** — the remaining ~12 entries
(after Item 2a's LEGACY_* deletions). Both lists are identical;
the duplication is accepted per the no-shorthand rule.

**Knock-on changes:**

- `gh optivem process scope <phase>` (the CLI command referenced in
  every prompt's `scope:` frontmatter comment) emits both sets.
  Open question for refinement (Item 4): one combined `scope:`
  map with `read:` / `write:` sub-keys, or two top-level keys
  `read_scope:` / `write_scope:`?
- The preamble rule from Item 2 ("only read files in scope") binds
  to the **read** scope, not the write scope. Item 2's wording
  already cites the `read:` set explicitly.
- The `scope_exception` block in `scope.md` grows a `kind:` field
  (`read` vs `write`) so a read-side overreach and a write-side
  overreach are distinguishable on the way out.
- **Items 11 Case B nearly collapses for free** — the 3 driver-port
  phases above remove the layer from `write`, so the "STOP and
  present before editing driver-port" prose becomes redundant; the
  universal `scope_exception` mechanism handles it.

**Acceptance:**

- `phase-scopes.yaml` schema supports the explicit `read:` /
  `write:` shape on every phase; the 5 phases in the audit table
  above declare asymmetric lists (in their remapped IDs from
  Item 2a).
- `gh optivem process scope <phase>` emits both sets.
- `scope.md` and `preamble.md` consume the right one for each
  side of the rule (writes → write-scope, reads → read-scope).
- `scope_exception` block grows a `kind:` field.
- Build-time test rejects bare layer names that resolve through
  neither `canonicalPathKeys()` nor `system-path`. No subset
  constraint is enforced.
- One end-to-end rehearsal of `write-acceptance-tests` shows the
  agent reading only `at-test` + `dsl-port` paths (plus
  explicitly-named architecture refs from the prompt body), while
  still writing the `TODO: DSL` placeholders into `dsl-core`.

### 4. Render scope keys + resolved paths into the dispatched prompt

**Status (post-spinoff):** the SSoT question collapses now that
`phase-scopes.yaml` is being folded into `process-flow.yaml`
(spinoff plan `20260526-1536-fold-phase-scopes-into-process-flow.md`).
The per-phase scope lives **on the BPMN node** — single SSoT, no
prompt-frontmatter mirror, no parity test needed.

**Observation (presupposed by Items 2 + 3).** Items 2 and 3 assume
the agent can see which paths belong to its scope. Today it cannot:

- The per-prompt frontmatter is `scope: {}` (literally empty — see
  `write-acceptance-tests.md:5` and every other prompt that pins
  to layer keys). The CLI comment "query resolved scope:
  `gh optivem process scope <phase>`" is documentation for the
  human prompt-author, not data the agent ever receives.
- `scope.md` (prepended at dispatch via `embed.go:81-84`) tells
  the agent: "the set of paths its agent may modify, listed in
  the `scope:` frontmatter at the top of the prompt you are
  reading." But the frontmatter is empty, so the agent is being
  pointed at nothing.
- Enforcement is server-side only: `check-phase-scope` runs
  *after* the agent commits and diffs the tree. The agent itself
  never sees a path list at write time.

**Consequence — the "DSL interface" question.** When prose in
the prompt body says "DSL interface", the agent has no mapping
that human phrase to the layer key `dsl-port`, and no
`dsl-port → acceptance-test/java/.../dsl/` resolution visible to
it. It infers the path from filename patterns it sees during its
(unbounded — see Item 2) Reads, which is exactly the over-reading
loop we want to close.

**Design (settled during refinement, 2026-05-26).** Two halves:

**Half 1 — `## Scope` block rendered at dispatch.** The runtime
reads the BPMN node's `read:` / `write:` lists (post-spinoff),
joins each key against the project's `gh-optivem.yaml paths:`,
and injects a `## Scope` section with key + resolved-path bullets
into the body the agent actually sees:

```
## Scope

You may **read** files under these paths:

- `dsl-port`: acceptance-test/java/src/main/java/.../dsl/
- `at-test`:  acceptance-test/java/src/test/java/.../tests/

You may **modify** files under these paths:

- `at-test`:  acceptance-test/java/src/test/java/.../tests/
- `dsl-port`: acceptance-test/java/src/main/java/.../dsl/
- `dsl-core`: acceptance-test/java/src/main/java/.../core/

Reading or writing outside this set requires a `scope_exception`
block.
```

**Placement in the four-heading skeleton** (Item 6): settle in the
Item 6 walk. Lean **sub-heading under `## Inputs`** — scope is an
input to the agent. Marked as TBD-by-Item-6.

**Half 2 — inline `${key}` substitution at every layer mention.**
Wherever a human name for a layer appears in the prompt prose,
follow it with the existing Family B `${key}` substitution:

Source-on-disk (what the prompt-author writes):

> Implement the DSL Core (`${dsl-core}`) for real — replace each
> `TODO: DSL` prototype with actual logic.
> If you need to add methods to the DSL Port (`${dsl-port}`), add
> prototype methods that throw a runtime exception.

Agent sees at dispatch (after Family B substitution):

> Implement the DSL Core
> (`acceptance-test/java/src/main/java/.../core/`) for real —
> replace each `TODO: DSL` prototype with actual logic.
> If you need to add methods to the DSL Port
> (`acceptance-test/java/src/main/java/.../dsl/`), add prototype
> methods that throw a runtime exception.

**Convention:**

- Use the existing `${key}` syntax (dollar prefix, Family B
  per `feedback_substitutable_paths_in_docs.md`). Do not
  introduce a parallel `{key}` syntax.
- Wrap the substitution in backticks for code formatting:
  ``(`${dsl-core}`)``.
- **Singular consistent** — both the human name and the key are
  singular: `DSL Core (\`${dsl-core}\`)`, `DSL Port
  (\`${dsl-port}\`)`. No plural human prose ("DSL Ports") even
  if it reads naturally — consistency wins.
- **Every occurrence** — tag every mention of a layer, not just
  the first. Verbose by ~12 chars per mention; the verbosity
  is small and the agent never has to scan back to find the
  first mapping.
- The `## Scope` block (Half 1) still enumerates everything in
  one place; the inline annotations make per-step references
  unambiguous.

**Per-prompt audit — layers requiring inline annotation:**

| Prompt | Layers mentioned in prose (current human names) | Inline `${key}` needed |
|--------|------------------------------------------------|-----------------------|
| `implement-dsl.md` | "DSL Core", "DSL interface", "System Driver port/interface", "External System Driver port/interface" | `${dsl-core}`, `${dsl-port}`, `${driver-port}`, `${external-system-driver-port}` |
| `implement-system-driver-adapters.md` | "System Driver port", "System Driver adapter(s)" | `${driver-port}`, `${driver-adapter}` (already used at lines 31, 32) |
| `implement-external-system-driver-adapters.md` | "External System Driver port", "External System Driver adapter(s)", "Ext* DTOs", "Real driver", "Stub driver(s)" | `${external-system-driver-port}`, `${external-system-driver-adapter}` (already used at lines 31, 33, 34) |
| `implement-system.md` | "system surface", "Driver-port" | `${system-path}` (or equivalent post-fold), `${driver-port}` (already used at line 36) |
| `implement-external-system-stubs.md` | "External System stub", "tests/DSL/Drivers" | `${external-system-driver-adapter}` and refs to test/DSL/driver keys |
| `write-acceptance-tests.md` | "Acceptance Test(s)", "DSL interface", "DSL Core" | `${at-test}`, `${dsl-port}`, `${dsl-core}` |
| `write-contract-tests.md` | "External System Contract Test(s)", "DSL interface", "DSL Core" | `${ct-test}`, `${dsl-port}`, `${dsl-core}` |
| `disable-tests.md`, `enable-tests.md` | "test methods", "disable marker(s)" (no layer-key prose; works on the substituted file list directly) | n/a |
| `refactor-system.md` | "system/" | `${system-path}` |
| `refactor-tests.md` | "test code layer", "acceptance tests", "contract tests", "DSL", "driver ports/adapters", "external-system driver ports/adapters" | every layer key the test layer spans |
| `refine-acceptance-criteria.md` | (no layer-key prose — works on the parsed-concepts artifact only) | n/a |
| `fix-*.md` (5 files) | (refer to layers abstractly via `${changed_files}` / `${allowed_roots}`; the new `## Scope` mechanism replaces `${allowed_roots}`) | n/a for prose; the `## Scope` block still renders |

**Existing mechanism this replaces — `${allowed_roots}`.**
`implement-system.md:15-16`, `implement-system-driver-adapters.md:16-17`,
`implement-external-system-driver-adapters.md:16-17`,
`refactor-system.md:12-13`, `refactor-tests.md:12-13` carry a
loose prose block:

```
Allowed write roots:
${allowed_roots}
```

— followed by an imperative line:

> Edit ONLY files under the "Allowed write roots" listed at the
> top of this prompt.

That mechanism does roughly what the new `## Scope` block does
but worse:

- **Write-only.** Silent on read-scope (Item 3's split impossible).
- **Prose, not structured.** No key labels.
- **Duplicated guardrail line** across the five files. The
  imperative collapses into the preamble (Item 2) / `scope.md`.
- **Tells the agent the answer but not the question.** The
  substituted paths land in the body, but the keys
  (`driver-port`, `driver-adapter`, …) that connect those paths
  to scope-rule references are absent.

So `## Scope` is **not** a new mechanism — it is a structured
replacement of `${allowed_roots}` that adds the keys, the
read/write split (Item 3), and a consistent location. The
existing `${allowed_roots}` substitution wiring re-targets to
emit the `## Scope` block; it's a refactor of the rendering
step, not a new resolver.

**Cleanup that drops out of Item 4 once `## Scope` lands:**

- Every `Allowed write roots:` / `${allowed_roots}` block in the
  5 prompts above is removed.
- Every "Edit ONLY files under the 'Allowed write roots'…"
  imperative is removed (lines 23 of
  `implement-external-system-driver-adapters.md` /
  `implement-system.md` /
  `implement-system-driver-adapters.md`, line 19 of
  `refactor-system.md` / `refactor-tests.md`). The rule moves
  to preamble (Item 2) or `scope.md`.
- The `${allowed_roots}` entry in the substituted-parameter
  inventory either disappears entirely (if `## Scope` is rendered
  directly) or is renamed / refocused per the new resolver.

**Per-language path resolution.** The dispatched `## Scope` block
shows paths from the **current project's** `gh-optivem.yaml`. So
a rehearsal against a Java project shows Java paths; against a
TypeScript project, TS paths. The layer keys are stable across
languages.

**Build-time guards:**

- Every layer key in any `## Scope` block (and any inline
  `${key}` annotation in prose) resolves through
  `canonicalPathKeys()`.
- Every layer key referenced in prose is also listed in either
  `read:` or `write:` for that phase node (catches drift between
  inline annotations and actual scope).

**Cross-links.**

- **Item 2** (scope-bound reads): the rule cites the `## Scope`
  block's `read:` set.
- **Item 3** (read/write split): the `## Scope` block renders
  both sets.
- **Item 6** (skeleton): `## Scope` placement decided in Item 6
  walk.
- **Item 9** (every prompt declares scope): the `scope: {}`
  frontmatter goes away post-fold — scope lives on the BPMN
  node. Every prompt loses its frontmatter `scope:` block; the
  rendered `## Scope` block in the body is the sole agent-facing
  declaration.

**Acceptance:**

- Every dispatched prompt (any phase that pins to layer keys)
  carries a rendered `## Scope` block showing **both** `read:` and
  `write:` sets (per Item 3) with keys and resolved paths.
- Every layer mention in prose is annotated with `${key}`
  Family B substitution (singular human name + singular key,
  every occurrence).
- `${allowed_roots}` mechanism removed from all 5 prompts that
  use it; the redundant "Edit ONLY files under…" imperative is
  removed (rule lives in preamble / scope.md).
- One end-to-end rehearsal of `write-acceptance-tests` shows
  the agent's dispatched prompt containing the resolved `## Scope`
  block, inline `${key}` annotations resolved to actual paths,
  and the agent's tool-use trace shows reads bounded to the
  rendered `read:` paths only.
- Build-time guard catches layer-key drift: every inline
  `${key}` in any prompt body matches a key in the same phase's
  `read:` or `write:` list (post-fold).

### 6. Standardise the prompt skeleton: `## Inputs` / `## Steps` / `## Outputs` / `## Additional Notes`

**Status (resolved during refinement, 2026-05-26):**

- Four-heading skeleton applies to **every** prompt, including fix-*.
- **Option C for fix-*:** `## Additional Notes` is allowed richer
  sub-headings (`### Why you were dispatched`, `### Exception to
  the anti-rediscovery rule`, `### Anti-patterns`) to preserve
  diagnostic structure under a single skeleton.
- **`## Outputs` is optional** — dropped for fix-* (no structured
  output beyond a prose diagnosis).
- **`## Rubric` in `refine-acceptance-criteria.md`** moves to
  `## Additional Notes` as reference material consulted by Steps.

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

**Per-prompt audit — current heading vs target:**

| Prompt | Current top-level headings | Skeleton fit / changes |
|--------|----------------------------|-----------------------|
| `disable-tests.md` | `## Inputs`, `## Annotation reason format`, `## Steps` | Has 1/4 → lift `## Annotation reason format` to `### Annotation reason format` under `## Inputs` |
| `enable-tests.md` | `## Inputs`, `## Strip filter`, `## Steps` | Has 1/4 → lift `## Strip filter` to `### Strip filter` under `## Inputs` |
| `implement-dsl.md` | `## Steps`, `## Phase-output flags` | Add `## Inputs` for `${parsed_concepts}` + `### Parameters` for `${touches-system-driver}` (Item 8). `## Phase-output flags` collapses into `## Outputs` (see definitions below). |
| `implement-system-driver-adapters.md` | `## Checklist`, `## Steps` | `## Checklist` → `### Checklist` under `## Inputs`. Add `## Outputs` if flags. |
| `implement-external-system-driver-adapters.md` | `## Checklist`, `## Steps` | Same shape |
| `implement-system.md` | `## Checklist`, `## Steps` | Same shape |
| `implement-external-system-stubs.md` | `## Steps` | Add `## Inputs` for substituted variables |
| `refactor-system.md` | `## Checklist`, `## Steps` | `## Checklist` → `### Checklist` under `## Inputs` |
| `refactor-tests.md` | `## Checklist`, `## Steps` | Same |
| `refine-acceptance-criteria.md` | `## Role in the flow` (Item 1 strips), `## Inputs`, `## Outputs`, `## Rubric for AC coverage`, `## Steps` | `## Rubric` → `## Additional Notes` |
| `write-acceptance-tests.md` | `## Acceptance Criteria`, `## Steps`, `## Outputs` | `## Acceptance Criteria` → `### Acceptance Criteria` under `## Inputs`; loose recovery prose at line 19 lifts to `## Additional Notes` |
| `write-contract-tests.md` | `## Steps` | Add `## Inputs`; loose recovery prose at line 14 lifts to `## Additional Notes` |
| `fix-*.md` (5 files) | `## Why you were dispatched`, `## Inputs you receive`, `## Exception to the anti-rediscovery rule`, `## What to do`, `## Anti-patterns`, `## Failing command`, ..., `## Allowed roots` | **Option C:** `## Inputs you receive` → `## Inputs` (with per-variable content as paragraphs or `### `-sub-headings); `## What to do` → `## Steps`; `## Outputs` omitted; the rest (`### Why you were dispatched`, `### Exception to the anti-rediscovery rule`, `### Anti-patterns`) move under `## Additional Notes` as sub-headings. Per-variable trailing blocks (`## Failing command\n${command}` etc.) collapse into `## Inputs`. |

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
- `fix-*.md` (5 files) — Option C: same four headings, with
  `## Additional Notes` carrying `### Why you were dispatched`,
  `### Exception to the anti-rediscovery rule`, `### Anti-patterns`
  as sub-headings to preserve diagnostic structure. `## Outputs`
  omitted (prose diagnosis only).

**Acceptance:**

- Every prompt (writing-agent and fix-*) follows the four-heading
  skeleton (`## Outputs` and `## Additional Notes` optional but,
  when present, named exactly this).
- `grep -nE '^## (Acceptance Criteria|Checklist|Phase-output flags|Rubric|Strip filter|Annotation reason format|Why you were dispatched|Inputs you receive|Exception to the anti-rediscovery rule|What to do|Anti-patterns|Failing command|Exit code|Captured stderr tail|Changed files|Allowed roots|Role in the flow)$' internal/assets/runtime/prompts/atdd/*.md`
  returns zero hits as top-level headings (sub-headings under
  `## Inputs` / `## Outputs` / `## Additional Notes` are fine).
- Re-run / recovery wording (today inline in
  `write-acceptance-tests.md:19`, `write-contract-tests.md:14`,
  `implement-dsl.md:20`, and similar lines in other prompts)
  lives under `## Additional Notes`, not interleaved with the
  Steps.
- fix-* Option C applied: structured sub-headings under
  `## Additional Notes` carry the diagnostic guidance.

### 9. Every prompt declares `scope:` in frontmatter — and the multi-caller case

**Status (re-aligned post-fold, 2026-05-26):** Item 9 collapses to
three sub-tasks:

1. **Strip `scope:` from every prompt frontmatter.** Today's
   matrix (audited post-fold):
   - `scope: {}` (6 prompts): `disable-tests`, `enable-tests`,
     `implement-dsl`, `implement-external-system-stubs`,
     `write-acceptance-tests`, `write-contract-tests` — get
     stripped.
   - `scope: none` (1 prompt): `refine-acceptance-criteria` —
     gets stripped.
   - **Missing entirely** (10 prompts): no action needed; the
     line never existed.
   - All 17 land with no `scope:` field in frontmatter.
2. **Build-time guard.** Add a test in `phase_scopes_test.go` that
   rejects any prompt carrying a `scope:` field in frontmatter.
   Post-fold, that line is dead; if it reappears it's a confused
   new prompt-author.
3. **Drop the `scope: none` frontmatter fallback** in
   `TestPhaseScopes_ReverseFK_WritingAgentsScoped`. The guard
   currently accepts either inline node scope **or** a `scope:
   none` frontmatter — post-Item-9-step-1, no prompt has frontmatter
   scope at all, so the fallback is dead. Simplify the guard to
   check only inline node scope. (The `refine-acceptance-criteria`
   case — where the agent is artifact-only — should be handled by
   a `scope: none` declaration **on the BPMN node**, not in the
   prompt frontmatter; cross-check the spinoff's handling of
   artifact-only agents during execution.)

**Sub-case A (multi-caller scope) — fully delegated to Item 10.**
The three original A1/A2/A3 options collapse:

- **A1 (per-caller `by-caller:` map in frontmatter):** dead by
  virtue of Item 4 Option B. No frontmatter scope at all.
- **A2 (split prompts):** equivalent to Item 10 Option II.
- **A3 (union scope, runtime narrowing):** redundant post-fold —
  each call site in `process-flow.yaml` carries its own node
  scope; no union needed in frontmatter (which has no scope
  anyway).

**Sub-case B (fix-* node scope) — delegated to the spinoff plan.**
The spinoff ensures every BPMN node (including fix-* dispatch
nodes) has `read:`/`write:` declared inline. The shape for
diagnose-only nodes (e.g. `write: []`) is the spinoff's call.

**Cross-links:**

- **Item 4** — settled the SSoT and frontmatter-drops conventions.
- **Item 10** — owns multi-caller prompt body shape.
- **Spinoff plan** (`20260526-1536-fold-phase-scopes-into-process-flow.md`)
  — owns node-scope declarations including fix-*.

**Acceptance:**

- `grep -nE '^scope:' internal/assets/runtime/prompts/atdd/*.md`
  returns zero hits.
- Build-time guard rejects any future reintroduction of `scope:`
  in prompt frontmatter.

**Original observation (preserved for context).** Of 17 prompts under
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

**Status (resolved during refinement, 2026-05-26): verb split.**
Each of the 3 multi-caller prompts splits into two prompts with
distinct verbs that encode the work-shape — no `${mode}`
parameter, no branching anywhere.

**The verb taxonomy (proposed by the user):**

| change-system-behavior CYCLE | redesign-system-structure CYCLE | Work shape |
|------------------------------|--------------------------------|------------|
| `implement-system-driver-adapters` | `update-system-driver-adapters` | implement = fill TODOs placed by upstream; update = modify existing impl per Checklist |
| `implement-external-system-driver-adapters` | `update-external-system-driver-adapters` | Same shape: implement vs update |
| `implement-system` | `update-system` | implement = production code so tests pass; update = reshape system surface per Checklist |

**Why verb-led naming is better than `${mode}` parameter:**

- **Self-documenting names.** "implement" naturally evokes "create
  new"; "update" naturally evokes "modify existing". No glossary
  needed.
- **Aligns with the corpus's existing taxonomy.** `refactor-*`
  (no behaviour change), `write-*-tests`, `disable-tests`,
  `enable-tests`, `fix-*` already follow a verb-led scheme.
  `implement-*` and `update-*` extend it consistently.
- **No `${mode}` parameter; no branching anywhere.** Each prompt
  has one algorithm. The agent's prompt carries only the
  instructions relevant to its dispatch.
- **No "Step 1: branch on mode" anti-pattern.** Each agent's
  Step 1 is real work.
- **BPMN dispatch identity = agent name.** The cycle's identity
  maps directly to the agent name; no `params: { mode: ... }`
  needed.

**Cross-cutting follow-on — `implement-dsl`'s
`${touches-system-driver}` collapse.** The existing parameter in
`implement-dsl` is the same anti-pattern in a different shape:

- AT-side call site (change-system-behavior, touches-system-driver=true):
  agent may write to `driver-port`.
- CT-side call site (cover-system-behavior, touches-system-driver=false):
  agent stays bounded to External System Driver port.

The work itself is the same shape (write real DSL impl); only the
**scope** differs. Post-fold (spinoff plan) each call site
declares its own node scope: AT-side has `write: [dsl-core,
driver-port]`; CT-side has `write: [dsl-core,
external-system-driver-port]`. **Scope IS the signal; the
parameter is redundant.** The `System Driver Interface Changed`
output flag is emitted only when the agent actually wrote to
`driver-port` (which scope controls).

Flag as **follow-on item** (separate plan or appended item):
collapse `implement-dsl`'s `${touches-system-driver}` parameter
into scope-as-signal. Out of scope of Item 10's immediate
deliverables (the 3 verb splits) but worth pursuing once the
verb-split lands and the pattern is established.

**Concrete changes for the 3 multi-caller files:**

- **`implement-system-driver-adapters.md`** — strip Step 1's
  "Branch on Checklist" anti-pattern; the prompt becomes a single
  algorithm (find `TODO: System Driver` markers, fill with real
  impl). Steps renumber from 2,3,4 → 1,2,3.
- **NEW `update-system-driver-adapters.md`** — new prompt with a
  single algorithm (read Checklist, identify affected adapter
  files, apply each Checklist entry). Inherits the multi-caller
  file's Step 2-4 content (the reshape algorithm) verbatim, no
  branching.
- **`implement-external-system-driver-adapters.md`** — same
  split: strip the Checklist branch, becomes the implement (fill
  TODOs) variant.
- **NEW `update-external-system-driver-adapters.md`** — the
  reshape variant (Ext* DTOs + Real/Stub drivers per Checklist).
- **`implement-system.md`** — same split: strip Step 1's branch,
  strip Step 3's "Escalation when no Checklist is supplied"
  (cannot occur — there is no Checklist branch any more).
- **NEW `update-system.md`** — the reshape variant (execute
  Checklist on system surface; per-channel updates).

**BPMN side post-rename:**

- `change-system-behavior` CYCLE call-activities dispatch
  `implement-*` agents.
- `redesign-system-structure` CYCLE call-activities dispatch
  `update-*` agents.
- The call-activity's `agent:` param carries the new name; no
  `mode:` param needed.

**Precondition (cross-plan dependency, out of scope of this
plan).** For the verb split to land cleanly, ticket parsing/intake
must validate Checklist presence for `task/system-redesign`-kind
tickets. A redesign-kind ticket with no Checklist body is a
validation error caught upstream, not a runtime concern in the
agent prompt. This validation belongs in ticket parsing or in the
BPMN dispatch gate — **not in the prompt corpus**. Flag as
follow-on item / new issue. (May already exist partially in
`refine-acceptance-criteria` flow — audit during execution.)

**Per-prompt audit:**

| Current prompt | Current branch site(s) | Action |
|----------------|------------------------|--------|
| `implement-system-driver-adapters.md:29-31` | Step 1 "Branch on Checklist" | Strip branch; keep translation algorithm only |
| `implement-system-driver-adapters.md:30-31 (b)` | Step 1 (b) reshape branch | Lift into new `update-system-driver-adapters.md` |
| `implement-external-system-driver-adapters.md:29-31` | Step 1 "Branch on Checklist" | Same split |
| `implement-external-system-driver-adapters.md:30-31 (b)` + Steps 2-4 | reshape branch + reshape Steps | Lift into new `update-external-system-driver-adapters.md` |
| `implement-system.md:28-30` | Step 1 "Branch on Checklist" | Same split |
| `implement-system.md:30 (b)` + per-channel Steps | reshape branch + per-channel updates | Lift into new `update-system.md` |
| `implement-system.md:37` | Step 3 escalation | Delete entirely (Item 11 — empty-Checklist case can't occur, escalation collapses to `scope_exception`) |
| `refactor-system.md`, `refactor-tests.md` | n/a | No mode branching; Checklist is content, not signal — no change |
| `implement-dsl.md` | `${touches-system-driver}` parameter | Flagged for follow-on collapse (scope-as-signal) |

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
  there." **Specifically: Item 3's per-phase asymmetric data
  (which moves driver-port out of `write` on the 3 driver-port
  phases per the audit table) is a hard precondition for Case B
  collapse.** Today (post-1536, pre-Item-3-data) every node has
  `read == write`, so driver-port is still in `write` for those
  phases and the inline "Driver-port guardrail" prose cannot be
  collapsed yet. Sequence: land Item 3 data, then land Item 11
  Case B deletions.
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
