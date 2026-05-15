# Plan: Thin ATDD agent prompts and align names with process docs

## Background

`internal/assets/runtime/prompts/atdd/` ships 13 agent prompt files. Two
issues today:

**1. Duplication with process docs.** Each runtime prompt restates a slice
of what its companion process doc at `internal/assets/global/docs/atdd/process/*.md`
already says — the one-line WRITE rule, editorial nudges ("rare at this
phase — typically every method already has a prototype"), occasional
shape-of-the-phase commentary. The process docs are the canonical specs
(Purpose / Conventions / Example / WRITE steps / Anti-patterns); the
prompts add agent identity, the FIX-compile-errors fallback, mechanics
("Do not present or wait…"), and the reads list — but they also restate
content the docs already own.

**2. Naming drift.** Prompt names don't follow a consistent scheme.
Some encode the phase (`atdd-driver-at`, `atdd-driver-ct`), some encode
the side (`atdd-backend`, `atdd-frontend`), some are role-only
(`atdd-chore`, `atdd-stubs`), and one is a long compound
(`atdd-task-system-interface-redesign`). Meanwhile the process docs have
a consistent `<cycle>-<state>-<role>.md` scheme. The mismatch makes it
hard to see at a glance which prompt maps to which phase doc.

This plan does two things:

- **Consolidate** each prompt's content against its process doc — for
  each line of prompt prose, either drop it (the doc already says
  this), migrate it into the doc (the doc doesn't yet cover this and
  the content is worth keeping), or keep it in the prompt (it's
  agent-mechanics, not phase semantics). The bias is toward migration:
  the goal is consolidation, not information loss.
- **Rename** each prompt to match its process doc by name (dropping the
  redundant `atdd-` prefix since the directory is already
  `runtime/prompts/atdd/`).

Both are net improvements regardless of any larger architectural call
about how the prompt and process layers relate.

## Deferred — for a follow-up plan, not this one

Two related questions are intentionally **out of scope** for this plan:

- **Merging AT/CT pairs into single files.** Earlier drafts of this
  plan proposed collapsing `atdd-driver-at` + `atdd-driver-ct` into one
  `atdd-driver.md` discriminated by a `${phase_doc}` parameter. That
  conflicts with the rename direction (a merged file can't have a
  phase-specific name like `at-red-system-driver.md`), so this plan
  drops the merge and keeps one prompt per phase.
- **Inlining the prompt layer into the process docs.** After thinning,
  each prompt will be ~5–10 lines: agent identity + FIX branch +
  mechanics + reads list. Whether to collapse those into the process
  docs themselves (so each phase has one file, not two) is a real
  architectural call with tradeoffs — audience mismatch (process docs
  render on GitHub for humans), N:M mapping (one process doc can have
  multiple agents executing it), reversibility (two-layer → inline is
  easy; inline → two-layer is harder). Defer the decision until after
  thinning makes the residual prompt size visible. If the residual
  feels small enough to swallow per phase, inline; otherwise keep
  two-layer.

## Out of scope

- Auditing or rewriting the process docs for their own sake. Process
  doc edits are **in scope only** as the receiving end of content
  migration from prompts (see Implementation step 1) — not as a
  general doc-improvement pass. The process-audit agent owns the
  broader review cycle and runs separately.
- Reworking `atdd-fix-verify.md`'s body content beyond the rename —
  it's a singleton with a different shape (diagnostic agent with
  `${verify_results}` / `${changed_files}` / `${allowed_roots}`
  substitution; already minimal relative to its purpose; not
  duplicative of any process doc).
- Reworking `atdd-stubs.md`'s body content beyond the rename — it's
  a placeholder pending ownership.
- Adding new agents or new phase-doc reads.

## End-result example — driver-at, before → after

### Before — `atdd-driver-at.md` (19 lines)

```
---
model: sonnet
effort: medium
---
You are the Driver Agent. Follow the phase specified in the input:

- **AT - RED - SYSTEM DRIVER - WRITE** — replace "TODO: Driver" System
  Driver prototypes with real Driver logic. If your impl references a
  System Driver method that doesn't yet have a prototype, add the
  "TODO: Driver" stub in the same step (rare at this phase — typically
  every method already has a prototype from AT - RED - DSL). The
  result must compile. See `at-red-system-driver.md`.
- **FIX compile errors** — your previous WRITE didn't compile. Locate
  the broken/missing piece in your prior edits (forgotten Driver stub,
  signature mismatch, typo) and fix it minimally.

Apply Driver Port Rules from `driver-port.md`.

Do not present or wait for approval inside the agent.

Read ${docs_root}/atdd/process/at-cycle-conventions.md.
Read ${docs_root}/atdd/process/at-red-system-driver.md.
Read ${docs_root}/atdd/architecture/driver-port.md.
Read ${docs_root}/atdd/code/language-equivalents/${language}.md.
```

### After — `at-red-system-driver.md` (10 lines)

```
---
model: sonnet
effort: medium
---
You are the Driver Agent. Follow the phase referenced below.

If your previous WRITE didn't compile, instead fix the broken/missing
piece in your prior edits (forgotten Driver stub, signature mismatch,
typo) and fix it minimally.

Do not present or wait for approval inside the agent.

Read ${docs_root}/atdd/process/at-cycle-conventions.md.
Read ${docs_root}/atdd/process/at-red-system-driver.md.
Read ${docs_root}/atdd/architecture/driver-port.md.
Read ${docs_root}/atdd/code/language-equivalents/${language}.md.
```

Gone from prompt:
- The WRITE-rule restatement (already in the doc's WRITE section).
- The "Apply Driver Port Rules" line (replaced implicitly by the
  `Read driver-port.md` line already in the reads list).
- "The result must compile" (now in the doc's WRITE section — see below).
- The editorial nudge about stub-adding being rare (now in the doc's
  Conventions section — see below).

Migrated **into** `at-red-system-driver.md` (illustrative — the
process doc gains content the prompt used to carry):

```diff
 ## AT - RED - SYSTEM DRIVER - WRITE

 1. Enable the tests marked disabled with reason `"AT - RED - DSL"`.
 2. Implement the System Drivers — replace each "TODO: Driver" prototype
    with actual logic. Stay within `${driver_port}/${sut_namespace}/` and
    `${driver_adapter}/${sut_namespace}/`. Model new methods on existing
    Driver methods in the same file.
+3. If your impl references a System Driver method that doesn't yet
+   have a prototype, add the `"TODO: Driver"` stub in the same step
+   (rare at this phase — typically every method already has a
+   prototype from AT - RED - DSL).
+
+The result must compile.

 **Scope:** Only System Driver code. No test, DSL, system, or external-driver edits.
```

File renamed from `atdd-driver-at.md` to `at-red-system-driver.md` —
matches the process doc it points at.

## Rename mapping

| Current | Renamed | Matches process doc? |
|---|---|---|
| `atdd-driver-at.md` | `at-red-system-driver.md` | exact |
| `atdd-driver-ct.md` | `ct-red-external-driver.md` | exact |
| `atdd-dsl-at.md` | `at-red-dsl.md` | exact |
| `atdd-dsl-ct.md` | `ct-red-dsl.md` | exact |
| `atdd-test-at.md` | `at-red-test.md` | exact |
| `atdd-test-ct.md` | `ct-red-test.md` | exact |
| `atdd-backend.md` | `at-green-system-backend.md` | subscope of `at-green-system.md` |
| `atdd-frontend.md` | `at-green-system-frontend.md` | subscope of `at-green-system.md` |
| `atdd-stubs.md` | `ct-green-stubs.md` | exact |
| `atdd-chore.md` | `chore.md` | subscope of `task-and-chore-cycles.md` |
| `atdd-task-system-interface-redesign.md` | `task-system-interface-redesign.md` | subscope of `system-interface-redesign.md` |
| `atdd-task-external-system-interface-redesign.md` | `task-external-system-interface-redesign.md` | subscope of `system-interface-redesign.md` |
| `atdd-fix-verify.md` | `fix-verify.md` | (singleton — no process doc) |

13 files stay 13 files; the count drops only if the deferred inline
decision goes that way later. Side benefit: the dispatcher's agent name
and the matching process doc filename become the same string for the
9 prompts with an exact-match doc. That removes the indirection between
"which agent" and "which process doc to read."

## Implementation

### 1. Per-prompt audit and thin

For each prompt, read it side-by-side with the process doc(s) it
already lists in its reads list. Remove from the prompt anything the
doc already covers. Keep what the doc doesn't own:

- Frontmatter (`model:`, `effort:`).
- Agent identity (`You are the X Agent`).
- FIX-compile-errors branch (driver/dsl/test prompts only — process
  docs treat RED-state runs as one-shot, not retry-able, so the FIX
  branch has no doc to point at).
- `Do not present or wait for approval inside the agent.`
- Reads list — `Read ${docs_root}/...` lines. Drop a read, the agent
  doesn't read it.
- `${acceptance_criteria}` injection (test-at only).
- `${verify_results}`, `${changed_files}`, `${allowed_roots}` (fix-verify only).
- `${architecture}`, `${allowed_roots}`, `${checklist}` (task / chore).

**Default policy: merge, don't delete.** When prompt content isn't
already covered by its process doc, migrate it into the doc rather
than dropping it. The point of this refactor is consolidation, not
information loss — the process doc becoming more complete is a win,
not a side effect. The "Drop, don't relocate" memory rule applies
only in the strict subset where the doc already covers the same
ground; double-covering is the only thing to avoid.

For each prompt line, the audit produces one of:

- **Drop** — the process doc already says the same thing. Remove
  from prompt; don't touch the doc.
- **Migrate** — the prompt has nuance the doc doesn't (an editorial
  nudge, an edge-case rule, a "rare at this phase…" clarifier).
  Move the content into the doc's appropriate section (Conventions,
  WRITE steps, or Anti-patterns) and remove from prompt. This is
  the **default** when the doc doesn't already cover it.
- **Replace prose with Read** — prose was naming a rules doc the
  process doc doesn't transitively cause the agent to read; replace
  with an explicit `Read ${docs_root}/atdd/architecture/<file>.md`
  in the prompt's reads list.
- **Keep in prompt** — content is agent-mechanics, not phase
  semantics. Stays in the prompt; doesn't belong in a human-facing
  process doc. Examples: the FIX-compile-errors branch ("your
  previous WRITE didn't compile…"), `Do not present or wait for
  approval inside the agent.`, and the `${param}` placeholders.

The receiving-end edits in process docs should be additive and
minimal — drop the prompt's content into the right section verbatim
(or near-verbatim) rather than rewriting the doc to accommodate. If
a migration would require restructuring the doc, stop and flag it
for a follow-up process-doc pass; don't expand the scope here.

### 2. Rename to match process docs

Per the table above. Drop the `atdd-` prefix universally.

### 3. Update dispatch sites

- `internal/assets/runtime/statemachine/process-flow.yaml`: every
  `agent:` field → new name per the rename table.
- `internal/atdd/runtime/clauderun/clauderun_test.go:171-177`:
  agent-name allowlist enumerates all 13 prompts. Update to the new
  names.
- `internal/atdd/runtime/statemachine/dispatch_spy_test.go:242-330`:
  expected `agent` strings per phase. Update per rename.
- `internal/atdd/runtime/statemachine/{behavioral_cycle,structural_cycle}_test.go`:
  spot-check for any of the renamed agents.
- `internal/atdd/runtime/driver/driver.go:502`, `:525` (comment-only
  references to `atdd-backend, atdd-frontend` and
  `atdd-task / atdd-chore`): update wording.
- Anywhere else `atdd-` agent names appear in source or test files
  (the grep covered the main sites at plan-write time; re-grep at
  execution time to catch any drift).

## Acceptance criteria

- All Go tests pass: `internal/atdd/runtime/clauderun/...`,
  `.../driver/...`, `.../statemachine/...`, `.../agents/...`. Run
  with `-p 2` per the Windows-test memory rule, or via
  `scripts/test.sh`.
- The embedded-prompt smoke test (`embedded_smoke_test.go`)
  succeeds against the new prompt names.
- After `assets sync` to a scaffolded repo, the runtime mirror
  contains exactly the 13 renamed prompt files — no orphaned
  `atdd-*.md` files.
- Manual rehearsal at a fresh ATDD slice (one AT cycle + one CT
  cycle) reaches each renamed agent and the dispatched prompt log
  (`runs/NNN-*.prompt.md`) shows: agent identity, the FIX branch
  (if present in source), the reads list with substituted paths,
  and **no** WRITE-rule restatement or editorial nudges. Confirmed
  by reading the rendered log, not by trusting the source.
- For each thinned prompt, the agent's actual behaviour in the
  rehearsal matches the pre-thinning behaviour. If a behaviour
  regresses, the dropped prose was load-bearing — restore it,
  ideally into the process doc with a rationale.

## Risks and watch-outs

- **A dropped line was actually load-bearing.** Mitigated by the
  migrate-don't-delete default — most content should land in the
  process doc, not vanish. The remaining risk is for lines explicitly
  dropped as "already covered" where the existing doc coverage turns
  out to be weaker than the prompt's version. Mitigated by the
  rehearsal step. If an agent regresses, restore the prose into the
  process doc with a one-line rationale.
- **Renaming is invasive in source.** Every `atdd-*` agent string in
  Go source / tests / YAML / comments needs updating. Run the Go test
  suite per-prompt, not at the end.
- **`atdd-fix-verify.md` is intentionally fat.** Don't thin it on
  autopilot — its `${failure_type}` branching, anti-patterns, and
  retry-budget discussion aren't duplicated in any process doc. The
  rename is fine; body changes are out of scope.
- **Per the "Check for concurrent agents" memory rule**, before pickup
  grep `plans/*.md` for an existing marker on this file and check
  `git status` for in-flight prompt edits.
- **The deferred decisions (merge, inline) must remain deferred.**
  Easy to drift into doing them while you're already touching every
  prompt. Don't. They're separate plans for a reason — they have
  bigger risk surface and warrant their own design pass.

## How to roll this out incrementally

The thinning + rename are independent per prompt. Land one prompt per
commit:

1. **`ct-red-external-driver.md`** (was `atdd-driver-ct.md`).
2. **`at-red-dsl.md`**, **`ct-red-dsl.md`**.
3. **`at-red-test.md`**, **`ct-red-test.md`** — extra care for the
   `${acceptance_criteria}` block; ensure the dispatcher still
   substitutes it for AT and leaves it alone (or substitutes empty)
   for CT, depending on current dispatcher behaviour.
4. **`at-green-system-backend.md`**, **`at-green-system-frontend.md`**.
5. **`ct-green-stubs.md`** (was `atdd-stubs.md`) — rename only; body
   stays the TBD placeholder.
6. **`chore.md`**.
7. **`task-system-interface-redesign.md`**,
   **`task-external-system-interface-redesign.md`**.
8. **`fix-verify.md`** — rename only.

Each step is one commit. Per the `/commit` vs raw-git memory rule,
these are agent-authored surgical commits — use raw `git`, not
`/commit`.

## Follow-up plans (not this one)

After this plan ships, the residual prompts will be ~5–10 lines each.
Two follow-up decisions become much easier to evaluate:

- **Inline?** Can each prompt's residual content move into its
  process doc so each phase is one file? Tradeoffs covered in the
  Deferred section above; revisit with concrete thinned prompts in
  hand.
- **Extract shared mechanics?** The `Do not present or wait` line and
  the language-equivalents read still repeat across most prompts.
  Worth a snippet/include mechanism only if the shared content grows;
  per "Drop, don't relocate," not worth it for two lines.

Both are explicitly out-of-scope here.
