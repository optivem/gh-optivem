# 2026-05-20 11:45 UTC — Rename `chore` → `task-system-implementation-refactoring`

> 🤖 **Picked up by agent (execute, batch-then-review)** — `Valentina_Desk` at `2026-05-20T10:54:31Z`

**Status:** REFINED (small dated plan; ready to execute)

**Origin:** promoted from Item 9 of `plans/20260519-1537-post-meta-bpmn-topics.md` (DECIDED, ready for promotion). Item 4 of the source plan proposed dropping the `task-` prefix from the two `task-*-redesign` prompts; that proposal was reversed during refinement of this plan in favor of *keeping* `task-` as a namespace for standalone work-item-type prompts (see Decision A). The *why* (refactoring-not-change framing, symmetry with `task-system-interface-redesign` / `task-external-system-interface-redesign`) lives in the source plan; this plan captures the mechanics only.

---

## Decisions

### Decision A — canonical name

`chore` → `task-system-implementation-refactoring`. Symmetry triple becomes:

- `task-system-interface-redesign` — interface change, system-side
- `task-external-system-interface-redesign` — interface change, external-side
- `task-system-implementation-refactoring` — implementation change, system-side (was `chore`)

The `task-` prefix groups standalone work-item-type prompts as a namespace distinct from inner-loop phase prompts (`at-*`, `ct-*`) and utility prompts (`fix-verify`, `disable-tests`, `enable-tests`). The previously-drafted "drop `task-`" action (former Decision B) is reversed: `task-` *stays*, and the rename adds it to the new prompt rather than removing it from the existing two.

### Decision B — short form for commit-prefix / phase label

**Decided: Option 1.** Use the long kebab `system-implementation-refactoring` everywhere `change_type:` appears — both at the routing-condition surface (`process-flow.yaml:306`) and at the commit-prefix surface (`process-flow.yaml:1234`). No short form (no `SYSTEM-IMPL-REFACTOR` alias). Rationale: one fewer translation surface; easier `grep`.

**Namespace clarification (consequence of Decision A's Story Y choice):**

Two distinct namespaces are now in play and must not be conflated:

- **Prompt-filename / `agent:` namespace** uses the `task-` prefix: `task-system-implementation-refactoring.md`, and `agent: task-system-implementation-refactoring` in `process-flow.yaml`.
- **`change_type:` namespace** does *not* use `task-`: `change_type: system-implementation-refactoring`. `change_type:` describes the kind of ticket / classification routing keys off, not which prompt file to load.

The two values differ by exactly the `task-` prefix. Anyone touching `process-flow.yaml` should expect this mismatch — it is intentional.

---

## Sequencing

**Independent.** No architectural dependencies on other in-flight plans:

- Orthogonal to the asset-tree rename (`plans/20260520-1145-runtime-references-tree-rename.md`) — touches prompts under `runtime/prompts/atdd/`, not under `global/docs/`.
- Orthogonal to the AT_GREEN collapse (drafted in parallel by another agent) — touches different `process-flow.yaml` regions.
- Orthogonal to the orphan-promotion plan (`plans/20260520-1109-ac-refinement-and-at-refactor-agent-steps.md`).

Land whenever; ideally before any new feature work that would refer to either name, so the new code uses the canonical form from day one.

---

## Surfaces to touch

### Prompt files

**Rename:**

- `internal/assets/runtime/prompts/atdd/chore.md` → `task-system-implementation-refactoring.md`. (The two `task-*-redesign.md` files stay as-is — Decision A's Story Y choice keeps the `task-` prefix as the standalone-work-item-type namespace.)

**Body rewrite — Pattern A homogenization** (consequence of Story Y; brings the new prompt in line with the two existing `task-*` prompts so the namespace is operationally meaningful, not just a filename prefix):

- Replace the opening line `"You are the Chore Agent. Implement the CHORE - WRITE phase as described below."` with `"You are the Task Agent. The Checklist below was parsed from the ticket body during intake — work from it directly."` (matches existing `task-*-redesign` prompts).
- Drop the `CHORE - WRITE` phase framing entirely. Refactoring is a whole task, not a phase of an inner loop — phase framing was inherited from a misclassification.
- Collapse the existing scope paragraph at line 8 + the elaboration at line 21 into a single guardrail block above the substitution variables: `"This task covers internal refactor / rename / move / dependency upgrade / build tweak / dead-code removal / internal abstraction change inside system/. No boundary or behavioral impact. system/ only; drivers, tests, DSL, Gherkin untouched."` (verbatim scope list preserved — it's load-bearing guardrail prose).
- Add `${architecture}` and `${allowed_roots}` substitutions (matches existing `task-*` prompts).
- Add `${checklist}` substitution at the end under a `## Checklist` heading.
- Keep frontmatter (`model: sonnet`, `effort: medium`) — sizing is correct for bounded checklist-driven work.

**Intake-agent consolidation (Option II — merge `chore` into `task`):**

Today there are four LLM-driven intake agents: `atdd-story` / `atdd-bug` / `task` / `chore` (see `internal/atdd/runtime/actions/bindings.go:396` and `internal/atdd/runtime/intake/sections.go:2`). `task` intakes the two redesign ticket types and produces a `${checklist}`; `chore` intakes refactoring tickets and produces the prose-driven Pattern B body. Under Story Y's namespace and Big Bang Pattern A, the `task` intake agent becomes the canonical intake for the whole standalone-work-item namespace and absorbs `chore`. Result: three intake agents (`atdd-story` / `atdd-bug` / `task`).

Required work:

- Verify the current `task` intake-agent prompt against refactoring-shaped tickets. If `task` intake is currently narrow (only redesign-shaped tickets), extend its instructions to classify *all three* task subtypes (interface redesign, external interface redesign, implementation refactoring) and emit a checklist for each. This is a precondition for Item 4's body rewrite, not a follow-up.
- Remove the `chore` intake-agent entry-point (prompt definition + any code path that dispatches to it for `change_type: system-implementation-refactoring`).
- Re-point dispatch: tickets with `change_type: system-implementation-refactoring` route to the `task` intake agent (not `chore`).
- Surfaces to inspect: `internal/atdd/runtime/actions/bindings.go` (intake-agent dispatch), `internal/atdd/runtime/intake/sections.go` (intake-agent registry / comment), any prompt-rendering code in `internal/atdd/runtime/clauderun/` or `internal/atdd/runtime/driver/` that special-cases the `chore` intake agent name.
- Update both comment lines that enumerate the four intake agents to enumerate three.

### `process-flow.yaml`

Path: `internal/atdd/runtime/statemachine/process-flow.yaml`. Line numbers below are verified against HEAD at refinement time; re-verify with grep before editing if HEAD has moved.

**Lines 1199 + 1208 — KEEP AS-IS.** `agent: task-system-interface-redesign` and `agent: task-external-system-interface-redesign` already match the (unchanged) prompt filenames; Story Y keeps the `task-` prefix on these references. No edit needed.

**Rename action — agent reference:**

- Line 1235: `agent: chore` → `agent: task-system-implementation-refactoring` (matches the new prompt filename per Decision A + Decision B namespace).

**Rename action — `change_type:` overload resolution** (Decision B, Option 1 — long kebab everywhere `change_type:` appears for this case; no `task-` per the namespace split):

- Line 306 (routing condition): `change_type == system-implementation-change` → `change_type == system-implementation-refactoring`.
- Line 1234 (commit-prefix / phase label): `change_type: CHORE` → `change_type: system-implementation-refactoring`. After this edit the two surfaces hold the same value; the overload is dissolved.

**Rename action — cycle id (for internal consistency):**

- Line 1230 (cycle definition): `- id: CHORE_CYCLE` → `- id: SYSTEM_IMPLEMENTATION_REFACTORING_CYCLE` (or shorter `REFACTORING_CYCLE` if other cycle ids in the file are short — verify against `SUT_CYCLE` and siblings before picking).
- Line 1228: `start: CHORE_CYCLE` → matching new id.
- Line 1242: `- {from: CHORE_CYCLE, to: SUT_END}` → matching new id.

**Comment updates:**

- Line 6: `# system-implementation-change subtypes)` → `# system-implementation-refactoring subtypes)`.
- Line 273: `# - system-implementation-change → sut_cycle (System Under Test internal)` → `# - system-implementation-refactoring → sut_cycle (System Under Test internal)`.
- Line 667: `# Shared by system-api-task, system-ui-task, and chore via call_activity` → `# Shared by system-api-task, system-ui-task, and task-system-implementation-refactoring via call_activity` (matches new agent ref).
- Line 1224: `# Routes through structural_cycle with the chore` → `# Routes through structural_cycle with the system-implementation-refactoring task`.

**Stale `phase_doc:` (line 1236):**

`phase_doc: docs/atdd/process/change/structure/system-implementation-change.md` — the referenced file is gone (per the source plan's Item 5 inlining work, `plans/20260519-1537-post-meta-bpmn-topics.md`). Drop the field or repoint per that plan's prompt-sourcing model. Also review the other `phase_doc:` fields at lines 1200 and 1209 for the redesign cycles — they reference `docs/atdd/process/change/structure/{system,external-system}-interface-redesign.md`; verify whether those files still exist or also need to be dropped/repointed.

**Sweep:**

Final grep against current HEAD to catch anything the line-number list misses:

```
grep -nE 'chore|CHORE|system-implementation-change' internal/atdd/runtime/statemachine/process-flow.yaml
```

Expect: zero matches after all edits land (or only intentional historical references in comments — none expected).

### Go code

Touch points verified at refinement time against HEAD. Re-grep before editing if HEAD has moved.

**`change_type:` / subtype value renames (`system-implementation-change` → `system-implementation-refactoring`):**

- `internal/steps/github_setup.go:80` — label `subtype:system-implementation-change` + description `"Structural change to system internals (no test-stack artifact)"` → `"Refactoring of system internals (no boundary or behavioral change)"`.
- `internal/atdd/runtime/actions/bindings.go:378` — error message that enumerates subtypes (`subtype:system-interface-redesign / subtype:external-system-interface-redesign / subtype:system-implementation-change`).
- `internal/atdd/runtime/actions/bindings.go:496` — `switch case "system-implementation-change":` value.
- `internal/atdd/runtime/actions/bindings_test.go:468` — JSON test fixture with `subtype:system-implementation-change` label.
- `internal/atdd/runtime/gates/bindings.go:241` — comment.
- `internal/atdd/runtime/gates/bindings.go:249, 275` — interactive prompt strings that enumerate change-type / subtype options.
- `internal/atdd/runtime/gates/bindings.go:258, 283` — `switch case "system-implementation-change":` values.
- `internal/atdd/runtime/gates/bindings_test.go:212` — test value.

**Agent-ref renames (`chore` → `task-system-implementation-refactoring`):**

- `internal/atdd/runtime/clauderun/clauderun_test.go:173` — `"at-green-system", "chore",` in a valid-agent list.
- `internal/atdd/runtime/clauderun/clauderun_test.go:253` — `opts.Agent = "chore"`.
- `internal/atdd/runtime/clauderun/clauderun_test.go:254` — comment `// The chore prompt now inlines phase-doc placeholders…`.
- `internal/atdd/runtime/driver/driver.go:526` — comment `the task / chore prompts substitute via ${allowed_roots}` → drop the slash (just `the task prompts`) since intake consolidation removes the separate `chore` intake agent.
- `internal/atdd/runtime/driver/driver_test.go:656` — comment `(task-* / chore)` → `(task-*)`.

**Intake-agent enumeration comments (consequence of Item 4 intake consolidation):**

- `internal/atdd/runtime/actions/bindings.go:396` — `// four LLM-driven intake agents (atdd-story / atdd-bug / task / chore).` → `// three LLM-driven intake agents (atdd-story / atdd-bug / task).`
- `internal/atdd/runtime/intake/sections.go:2` — same edit.

**KEEP AS-IS (Story Y) — `task-` prefix references are correct and must not be stripped:**

- All `task-system-interface-redesign` / `task-external-system-interface-redesign` references throughout `internal/atdd/runtime/clauderun/clauderun_test.go`, `driver/driver_test.go`, `statemachine/dispatch_spy_test.go`, `statemachine/structural_cycle_test.go`. When sweeping, these should pass through untouched.

**Sweep:**

After edits, final grep across `internal/` (excluding the kept references):

```
grep -rnE 'chore|CHORE|system-implementation-change' internal/ --include='*.go'
```

Expect: zero matches. Then re-grep with the wider net including the kept references and inspect manually to confirm `task-system-interface-redesign` / `task-external-system-interface-redesign` references are still semantically correct (haven't been broken by an edit elsewhere).

### Documentation

**`docs/process-diagram.md`** (canonical Mermaid source — SVGs are generated from this file):

- Line 138: `GATE_CHANGE_TYPE -- system-implementation-change --> SUT_CYCLE` → `… system-implementation-refactoring …` (Mermaid edge label; matches Decision B `change_type:` value).
- Line 491: `CHORE_CYCLE["CHORE_CYCLE — see § Structural Cycle (shared)"]` → matching new id from Item 5 (e.g., `SYSTEM_IMPLEMENTATION_REFACTORING_CYCLE` or `REFACTORING_CYCLE` — must equal what was chosen at `process-flow.yaml:1230`).
- Line 494: `CHORE_CYCLE --> SUT_END` → matching new id.
- Inspect surrounding section headings and prose for any other "chore" / "Chore Cycle" / "system-implementation-change" references that didn't appear in the grep (lowercase prose, table-of-contents entries, etc.).

**SVGs (regenerated, not hand-edited):**

After `docs/process-diagram.md` is updated, run `scripts/render-svgs.sh` to regenerate every `docs/images/process-diagram-*.svg`. The two SVGs that currently embed the old strings (`process-diagram-5-run-cycle.svg`, `process-diagram-17-sut-cycle.svg`) will pick up the new labels automatically. Verify the regenerated set has zero remaining old-string matches:

```
grep -lE 'chore|CHORE|system-implementation-change' docs/images/*.svg
```

Expect: empty.

**`plans/deferred/20260518-2236-migrate-process-docs-hierarchy.md`:**

Multiple references to the old `chore` naming throughout the deferred plan (lines 42, 74, 90, 100, 130–132 at refinement time). Rather than rewrite the deferred plan in place — it's deferred precisely because it isn't being executed — add a one-line note at the top:

> **Note (2026-05-20):** Superseded re: `chore` naming by `plans/20260520-1145-system-implementation-refactoring-rename.md`. References to `chore.md`, `subtype:system-implementation-change`, and `CHORE_CYCLE` below are stale; map them to `task-system-implementation-refactoring.md`, `subtype:system-implementation-refactoring`, and the new cycle id (see that plan for Decisions A/B) when/if this plan is reactivated.

**Sweep:**

```
grep -rniE 'chore|CHORE|system-implementation-change' docs/ --include='*.md'
grep -rlE 'chore|CHORE|system-implementation-change' docs/images/*.svg
```

Expect: zero remaining matches in both passes (or only intentional historical references in changelog-style files, none expected).

### GitHub labels (migration — Decision C)

The label `subtype:system-implementation-change` is created in *scaffolded* repos by `internal/steps/github_setup.go`. It's not present in this tool's own repo (verified at refinement time: zero issues open/closed carry it here). The migration is per-scaffolded-repo, run by operators on repos they administer.

**Decided: Option 2 (bulk-migrate via `gh label edit`).** Documented as a one-liner; no subcommand built unless scaffolded-repo count grows enough to make manual runs painful.

**Operator step (per scaffolded repo with the old label):**

```
gh label edit subtype:system-implementation-change \
  --name subtype:system-implementation-refactoring \
  --description "Refactoring of system internals (no boundary or behavioral change)" \
  --repo <owner>/<repo>
```

`gh label edit --name` renames the label in place; GitHub re-applies the new name to every existing issue that carries it. One command per repo, no per-issue iteration needed.

**Detection (find which scaffolded repos still have the old label):**

```
# From the academy workspace root, for each repo:
gh label list --repo <owner>/<repo> --search subtype:system-implementation-change
```

Any repo where the search returns a row needs the `gh label edit` step. Scripting this into a workspace-wide loop is fine if it's ergonomic; building it into `gh optivem workspace migrate-labels` is deferred until repo count justifies it.

**Rejected options:**

- **Option 1 (dual-recognition)** — permanent Go-code complexity to accept both labels; violates the long-term-coherence principle Story Y / Big Bang chose elsewhere in this plan.
- **Option 3 (forward-only)** — functional regression in existing scaffolded repos; old issues become unrecognized by new Go code.

---

## Out of scope (explicitly)

- **CI consistency walk** (Item 4 residual — a test that walks `process-flow.yaml` and asserts every `agent:` / `phase_doc:` resolves to an existing file). Not promoted here. Pick up when someone hits another dangling-reference bug.
- Any rename of the other two siblings in the triple (`task-system-interface-redesign`, `task-external-system-interface-redesign`). Their `task-` prefix is *kept* under Decision A's Story Y choice; no edits to their filenames or to `agent:` references that point at them.
- Restructuring `change_type:` semantics beyond Decision B.
- Homogenizing the *body* of the two existing `task-*-redesign.md` prompts further (e.g., reshaping their checklist contract or merging their scope prose with the new refactoring prompt). Pattern A is adopted by the new prompt; the existing two are not touched.

---

## Done when

- All file renames landed (only `chore.md` → `task-system-implementation-refactoring.md`; the two `task-*-redesign.md` files are KEPT).
- Sweep returns no stale references: `grep -rnE 'chore\.md|chore\b|CHORE|system-implementation-change' .` (or only intentional historical references — this plan, the source plan, changelog entries). Note that `task-system-interface-redesign` and `task-external-system-interface-redesign` are *not* in the sweep — they are correct and must remain.
- `go test ./...` (with `-p 2` per memory) passes.
- One acceptance run end-to-end through a `change_type: system-implementation-refactoring` ticket confirms: (a) intake routes to the (consolidated) `task` intake agent, (b) `${checklist}` is populated, (c) routing through the renamed cycle node works, (d) commit prefix uses the new value.
- Intake-agent consolidation verified: `chore` intake entry-point removed; `task` intake handles all three task subtypes; the two comment lines now enumerate three intake agents.
- GitHub label migration executed (per Decision C — `gh label edit` per scaffolded repo) or explicitly deferred with a note.
- A short changelog entry about the renames if the project keeps one.
