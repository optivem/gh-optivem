# Agent prompt fixes — remarks from prompt-file review

> ✅ **Spinoff `20260526-1536-fold-phase-scopes-into-process-flow.md`
> fully landed** as commit `6b2fd9f` ("atdd: fold phase-scopes.yaml
> into process-flow.yaml node scope").
>
> ✅ **Session 1 (Items 3 + 9 + 10) landed (2026-05-26).** Per-phase
> asymmetric `read != write` data, `scope.md kind:` field, prompt
> frontmatter `scope:` strip (7 prompts), `scope: none` declaration moved
> to the BPMN node (`Engine.IsScopeNone`), build-time guard rejecting
> `scope:` in any prompt frontmatter, and the 3-way verb split
> (`implement-*` / `update-*` pairs for `system`,
> `system-driver-adapters`, `external-system-driver-adapters`) plus the
> matching `redesign-*-structure` CYCLE re-pointings.
>
> ✅ **Session 2 (Items 1 + 6 + 11) landed (2026-05-26).** Prompt-body
> sweep across all 17 prompts via 11 parallel subagents:
> - Item 1 caller-name plumbing stripped (Type 1) and contract refs
>   reworded (Type 2) per the Distinguishing principle.
> - Item 6 four-heading skeleton (`## Inputs` / `## Steps` / `## Outputs` /
>   `## Additional Notes`) applied; legacy headings (`## Checklist`,
>   `## Acceptance Criteria`, `## Phase-output flags`, `## Rubric`,
>   `## Strip filter`, `## Annotation reason format`, fix-* diagnostic
>   blocks, `## Role in the flow`) lifted into sub-headings or relocated
>   per Option C for fix-*. Recovery prose lifted out of `## Steps` into
>   `## Additional Notes`.
> - Item 11 "Driver-port guardrail" + "do not modify X" prose stripped
>   from the system-pair, driver-pair, and external-driver-pair files
>   (Cases A and B both collapse into scope now that Item 3's asymmetric
>   `read`/`write` data is in place). `scope.md` updated with explicit
>   "Scope is the complete contract" section.
>
> **Remaining (post-Session-2):**
> - Item **4** — rendered `## Scope` block + inline `${key}` annotations
>   + `Allowed write roots:` cleanup (Session 3).

## Origin / intent

Conversation with user (2026-05-26 14:48) walking through observed issues
in the agent prompt files under
`internal/assets/runtime/prompts/atdd/*.md`. This plan is an
accumulating list of remarks; items will be appended as the walk
continues.

## Scope

`internal/assets/runtime/prompts/atdd/*.md` only — the prompt bodies
shipped to writing/fix agents. No runtime, no schema, no orchestration
changes — **except Session 3 (Item 4)** which adds a runtime rendering
step for the `## Scope` block.

## Execution strategy

Session 3 (the only remaining session) runs in a fresh `/clear`-ed
window.

**Session 3 — Render + Item 4.** Main session handles the runtime
renderer change + tests (the rendering of `## Scope` from BPMN node
`read:` / `write:` lists, joined against the project's
`gh-optivem.yaml paths:`). In parallel, dispatch **~10 subagents** for
per-prompt inline `${key}` annotations + `Allowed write roots:` block
stripping on the 12 prompts in Item 4's audit table.

The session begins by adding a pickup marker to this plan and ends by
removing it (or deleting the plan file outright once Item 4 lands).

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

**Placement in the four-heading skeleton** (Item 6, now landed):
**sub-heading under `## Inputs`** — scope is an input to the agent.
(Item 6 left the placement TBD-by-Item-4; Session 3 finalises it
as `### Scope` under `## Inputs`.)

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
  imperative is removed. The rule moves to preamble (Item 2) or
  `scope.md` (already updated in Session 2's Item 11 close-out).
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
  both sets. (Item 3 landed in Session 1.)
- **Item 6** (skeleton): `## Scope` renders as `### Scope`
  sub-heading under `## Inputs`. (Item 6 landed in Session 2.)
- **Item 9** (every prompt declares scope): the `scope: {}`
  frontmatter goes away post-fold — scope lives on the BPMN
  node. Every prompt loses its frontmatter `scope:` block; the
  rendered `## Scope` block in the body is the sole agent-facing
  declaration. (Item 9 landed in Session 1.)
- **Item 11** (do-not-modify prose collapse): the prose strip
  landed in Session 2, but until Item 4's `## Scope` block
  renders, the agent only sees scope via the legacy
  `${allowed_roots}` block. Item 4 finalises the contract.

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

---

*Additional items to be appended as the walk continues.*
