# 20260528-1045 — Dedupe re-entry policy across three implementer prompts

Lineage: spinoff of `plans/20260527-2214-runtime-prompts-audit.md` Finding 4
("Duplication of the 'If your previous WRITE didn't compile, fix the
broken/missing piece' instruction across implementers").

## Confirmed state

The duplication still exists. Line numbers have shifted slightly versus the
original audit; current locations are:

- `internal/assets/runtime/agents/atdd/external-system-driver-adapter-implementer.md:22`
  (inline at the end of the single Step 1, after the "do NOT read external
  source code" clause).
- `internal/assets/runtime/agents/atdd/system-driver-adapter-implementer.md:22`
  (inline at the end of the single Step 1).
- `internal/assets/runtime/agents/atdd/dsl-implementer.md:39`
  (under `## Additional Notes`; the audit cited `:62` but the file has since
  been trimmed to 39 lines — same block, same wording).

### Wording comparison

| File | Phrasing |
|---|---|
| `external-system-driver-adapter-implementer.md:22` | "If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits **(forgotten Driver stub (`${external-system-driver-adapter}`), signature mismatch, typo)** and fix it minimally." |
| `system-driver-adapter-implementer.md:22` | "If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits **(forgotten Driver stub, signature mismatch, typo)** and fix it minimally." |
| `dsl-implementer.md:39` | "If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits **(forgotten driver stub in the System Driver port (`${driver-port}`) or External System Driver port (`${external-system-driver-port}`), signature mismatch, typo)** and fix it minimally. **Do not change DSL Core (`${dsl-core}`) semantics.**" |

All three express the **same core policy**: if the previous WRITE failed to
compile, fix the broken/missing piece minimally rather than restarting from
scratch. The variation is in two per-agent details:

1. The **example list** of what "broken/missing piece" looks like — which
   stub kind, in which port. These are illustrative and per-agent.
2. A **DSL-specific guardrail** ("Do not change DSL Core semantics") that
   only the DSL implementer carries.

The shared core, stripped of per-agent particulars, is one sentence:

> If your previous WRITE didn't compile, instead fix the broken/missing
> piece in your prior edits (forgotten stub, signature mismatch, typo)
> and fix it minimally.

(Note: two test-writer agents — `acceptance-test-writer.md:21` and
`contract-test-writer.md:19` — carry a related but distinct re-entry
instruction tied to the DSL-stub asymmetric-scope clause. That is a
*different* policy ("when a prior run's stub-append edits didn't compile")
and out of scope for this plan. Flagging here so a future audit knows
not to fold them in without rechecking.)

## Mechanism review

Shared chunks live at `internal/assets/runtime/shared/` and currently number
three:

- `preamble.md` — prepended to every prompt.
- `scope.md` — prepended to every prompt (between preamble and body).
- `interactive-suffix.md` — appended only to non-headless dispatches.

The composition is wired in `internal/atdd/runtime/agents/embed.go`:

```go
return sharedPreamble + "\n\n" +
    sharedScope + "\n\n" +
    body + "\n", nil
```

The current shared-chunk mechanism is **unconditionally prepended** —
there is no body-marker include syntax. Adding a new opt-in chunk
would require new dispatcher machinery.

`${name}` substitutions are wired in `clauderun.go`'s
`renderPromptWithReferencesRoot`. The renderer already handles a mix of:

- Unconditional substitutions (always registered — e.g. `${phase}`,
  `${architecture}`, `${expected-outputs}`).
- Conditional substitutions (registered only when the source field is
  non-empty so `findUnfilledPlaceholders` catches misuse — e.g.
  `${acceptance-criteria}`, `${language}`, `${checklist}`).
- Helper-rendered substitutions (string built by a Go helper from per-
  dispatch context — e.g. `${disable-marker-example}`,
  `${disable-marker-removal-example}`, `${scope-block}`).

A new constant-text substitution would be a one-liner in the
unconditional-block:

```go
"re-entry-policy": "If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten stub, signature mismatch, typo) and fix it minimally.",
```

## Options considered

### (a) New shared chunk in `internal/assets/runtime/shared/`

Add `re-entry-policy.md`, load it via `mustReadAsset` in `embed.go`, and
prepend it to every agent prompt the way `preamble.md` and `scope.md` are
prepended today.

**Why rejected:** the current shared-chunk mechanism is unconditional. Only
three of twenty agents (the writing implementers) need this rule — and even
within "implementers," the test-writer agents need a *different* re-entry
clause. Universal prepend would add ~1 line × 17 agents that don't need it
× every dispatch. That trades one duplication for one over-include.

A variant — body-marker include like `${include:re-entry-policy}` — would
need new dispatcher machinery (asset-file lookup keyed on a body marker)
to gate inclusion. For a single one-sentence asset, the new mechanism is
heavier than the policy it carries.

### (b) New `${re-entry-policy}` substitution dispatched by the renderer — RECOMMENDED

Register `re-entry-policy` as an unconditional `${name}` substitution in
`renderPromptWithReferencesRoot`. The value is a Go-side string constant
(or a package-level `var` so future edits stay in one place). Each of the
three implementer prompts replaces its current re-entry sentence with
`${re-entry-policy}` plus a one-line per-agent appendix listing the
specific stub kind (and, for the DSL implementer, the
"Do not change DSL Core semantics" guardrail).

**Pros:**

- Matches the dispatcher's existing pattern for short cross-cutting
  clauses inlined into multiple prompts (`${disable-marker-example}`,
  `${disable-marker-removal-example}`).
- Opt-in by reference — non-implementer prompts pay nothing.
- One-line registration in the renderer; no new mechanism.
- Policy change is a one-character Go-string edit, propagates to all
  three prompts on next dispatch.

**Cons:**

- Policy text lives in Go source, not in markdown. For a one-sentence
  policy this is fine — `disable-marker-example` already has its
  per-language strings in Go source.

### (c) Status quo with wording-sync only

Re-write the three lines to identical wording (drop the per-agent example
lists) and leave them as three separate prompt lines.

**Why rejected:** doesn't address the audit finding. A future policy
change still requires three coordinated edits. Saves zero lines and
leaves the drift surface intact.

## Decision

**Option (b).** Add `${re-entry-policy}` as an unconditional Go-string
substitution in `renderPromptWithReferencesRoot`. The per-agent
appendices stay inline in each prompt (stub kind, DSL-specific
guardrail) because they are genuinely per-agent.

## Items

### 1. Add `${re-entry-policy}` substitution to the renderer

**File:** `internal/atdd/runtime/clauderun/clauderun.go`
**Location:** inside `renderPromptWithReferencesRoot`, in the
unconditional-substitution map (the one currently containing
`issue-num`, `issue-title`, `phase`, `architecture`, `subtype`,
`changed-files`, `references-root`).

**Add:**

```go
"re-entry-policy": rendererReEntryPolicy,
```

…and define the constant at package scope (near `nowFn` or alongside
the other render helpers):

```go
// rendererReEntryPolicy is the one-line "if your previous WRITE didn't
// compile, fix minimally" clause inlined into writing-implementer
// prompts via ${re-entry-policy}. Centralised here so a policy change
// (e.g. "re-runs always start fresh") needs only one edit. Per-agent
// specifics (which stub kind, agent-specific don't-touch clauses)
// stay in each prompt's body — this constant carries only the shared
// core.
const rendererReEntryPolicy = "If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten stub, signature mismatch, typo) and fix it minimally."
```

### 2. Rewrite the re-entry sentence in `external-system-driver-adapter-implementer.md`

**File:** `internal/assets/runtime/agents/atdd/external-system-driver-adapter-implementer.md:22`

**Current line (end of Step 1):**

> Implement the External System Driver adapters (`${external-system-driver-adapter}`) for real — replace each `TODO: External System Driver` prototype with actual logic. Do NOT read external-system source code to figure out behavior; rely on the contract tests and the published external API contract. If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten Driver stub (`${external-system-driver-adapter}`), signature mismatch, typo) and fix it minimally.

**Replacement:**

> Implement the External System Driver adapters (`${external-system-driver-adapter}`) for real — replace each `TODO: External System Driver` prototype with actual logic. Do NOT read external-system source code to figure out behavior; rely on the contract tests and the published external API contract. ${re-entry-policy} The "broken/missing piece" for this agent is typically a forgotten Driver stub under `${external-system-driver-adapter}`.

### 3. Rewrite the re-entry sentence in `system-driver-adapter-implementer.md`

**File:** `internal/assets/runtime/agents/atdd/system-driver-adapter-implementer.md:22`

**Current line (end of Step 1):**

> Implement the System Driver adapter (`${driver-adapter}`) for real — replace each `TODO: System Driver` prototype with actual logic. If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten Driver stub, signature mismatch, typo) and fix it minimally.

**Replacement:**

> Implement the System Driver adapter (`${driver-adapter}`) for real — replace each `TODO: System Driver` prototype with actual logic. ${re-entry-policy} The "broken/missing piece" for this agent is typically a forgotten Driver stub under `${driver-adapter}`.

### 4. Rewrite the re-entry note in `dsl-implementer.md`

**File:** `internal/assets/runtime/agents/atdd/dsl-implementer.md:39`

**Current line (the only bullet under `## Additional Notes`):**

> - If your previous WRITE didn't compile, instead fix the broken/missing piece in your prior edits (forgotten driver stub in the System Driver port (`${driver-port}`) or External System Driver port (`${external-system-driver-port}`), signature mismatch, typo) and fix it minimally. Do not change DSL Core (`${dsl-core}`) semantics.

**Replacement:**

> - ${re-entry-policy} For this agent the "broken/missing piece" is typically a forgotten driver stub in the System Driver port (`${driver-port}`) or External System Driver port (`${external-system-driver-port}`). Do not change DSL Core (`${dsl-core}`) semantics in the fix.

### 5. Add / extend a renderer test for the new substitution

**File:** `internal/atdd/runtime/clauderun/clauderun_test.go`

Locate the existing `TestRenderPrompt_*` table-style suite (the same one
that asserts `${disable-marker-example}` expansion) and add one case per
implementer that:

1. Renders the prompt for `dsl-implementer`,
   `system-driver-adapter-implementer`,
   `external-system-driver-adapter-implementer`.
2. Asserts the rendered text contains the literal first words of
   `rendererReEntryPolicy` ("If your previous WRITE didn't compile").
3. Asserts `${re-entry-policy}` does NOT survive in the rendered text
   (it has been substituted).

This is a regression seam — if a future edit re-introduces the literal
re-entry sentence in one of the three prompts (drift), the test does
not directly catch it, but if anyone deletes the constant or the
registration, the substitution check fires.

## Verification

- Run `go test ./internal/atdd/runtime/clauderun/...` after Item 5 lands;
  it must pass.
- Run `go build ./...` after Item 1 lands to confirm the new constant
  and map entry compile.
- Manually render one of the three implementer prompts via the
  `RenderPrompt` test seam (or by invoking `gh optivem` against a
  scratch ticket) and confirm:
  - The literal sentence "If your previous WRITE didn't compile,
    instead fix the broken/missing piece in your prior edits (forgotten
    stub, signature mismatch, typo) and fix it minimally." appears
    exactly once per prompt.
  - The per-agent appendix (stub kind / DSL guardrail) follows it.

## Notes for the executor

- Do NOT also fold in the test-writer re-entry instruction
  (`acceptance-test-writer.md:21`, `contract-test-writer.md:19`). That
  is a *different* policy — see the "Confirmed state" section above.
- Do NOT add `re-entry-policy.md` under `internal/assets/runtime/shared/`
  — Option (a) was rejected (universal-prepend over-include).
- Per the project's "Renames autonomous, content changes gated" rule,
  Items 2-4 (prompt body rewrites) are content changes and should be
  gated for review before commit. Item 1 (renderer addition) and
  Item 5 (renderer test) are also content; gate the batch together.
