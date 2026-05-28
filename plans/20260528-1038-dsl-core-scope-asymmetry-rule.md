# 20260528-1038 — Relocate the dsl-core scope-asymmetry rule out of writer prompts

**Lineage:** Spinoff from `plans/20260527-2214-runtime-prompts-audit.md` Finding 3. The audit identified that the same load-bearing sentence about the deliberate `read:` / `write:` asymmetry is restated in two writer prompts and asked architecture-sync to decide where it should live instead.

## Problem

Two writer prompts carry the same load-bearing sentence at the end of Step 2:

- `internal/assets/runtime/agents/atdd/acceptance-test-writer.md:21`
- `internal/assets/runtime/agents/atdd/contract-test-writer.md:19`

The sentence:

> The asymmetric scope (dsl-core is writeable but not in `read:`) is deliberate: reading impl context would leak it into test design.

This rule is **content**: it explains *why* `dsl-core` appears in `write:` but not `read:` for these two MIDs, and it's the only thing stopping a confused agent from "reading dsl-core for context" before writing tests — which is exactly the failure mode the asymmetry is designed to prevent.

The rule is paid once per dispatch in each of two prompts. Worse, the dispatcher's rendered `${scope-block}` (`internal/atdd/runtime/clauderun/clauderun.go:909-942`) shows two flat lists ("You may **read** …" / "You may **modify** …") with no signal that the lists differ on purpose. The operator has to spot the asymmetry themselves; the prompt sentence is the only place the rationale ever reaches the agent.

The MIDs in `process-flow.yaml` already carry the rationale as YAML-source comments:

- Lines 1319-1321 above `write-acceptance-tests:` ("Asymmetric: the test-writer adds `TODO: DSL` placeholders to dsl-core but does not read existing dsl-core (which would leak implementation context into a test-writing task).")
- Line 1363 above `write-contract-tests:` ("Same asymmetry as write-acceptance-tests (CT side).")

These YAML comments are invisible at runtime — they document *the schema author's intent* for future editors of `process-flow.yaml`, but they never reach the dispatcher or the agent.

## Scope of the asymmetry (audited)

I checked every `read:` / `write:` pair in `process-flow.yaml`:

| MID | read | write | Pattern |
|---|---|---|---|
| `write-acceptance-tests` | at-test, dsl-port | at-test, dsl-port, **dsl-core** | **write \ read** non-empty |
| `write-contract-tests` | ct-test, dsl-port | ct-test, dsl-port, **dsl-core** | **write \ read** non-empty |
| `implement-dsl` | dsl-core, driver-port, driver-adapter, ext-driver-port, ext-driver-adapter | (same) | symmetric |
| `implement-system` | at-test, ct-test, dsl-port, dsl-core, driver-port, driver-adapter, ext-driver-port, ext-driver-adapter, system-path | system-path | `read ⊋ write` (guardrail) |
| `update-system` | system-path, driver-adapter, driver-port | system-path, driver-adapter | `read ⊋ write` (guardrail) |
| `implement-system-driver-adapters` | driver-port, driver-adapter | driver-adapter | `read ⊋ write` (guardrail) |
| `update-system-driver-adapters` | driver-port, driver-adapter | driver-adapter | `read ⊋ write` (guardrail) |
| `implement-external-system-driver-adapters` | ext-driver-port, ext-driver-adapter | ext-driver-adapter | `read ⊋ write` (guardrail) |
| `update-external-system-driver-adapters` | ext-driver-port, ext-driver-adapter | ext-driver-adapter | `read ⊋ write` (guardrail) |

**The write-only-not-read asymmetry is unique to the two test writers.** Every other phase is symmetric or read-only-not-write (the "read for context, edits require scope_exception" guardrail). This is load-bearing: the rule "write a TODO stub without reading the impl" is the entire point of the test-writer's `dsl-core` scope, and it does not generalise to any other MID.

## Proposed approach: (c) hybrid — dispatcher annotation + MID-level rationale field

After weighing the three options the audit proposed, I'm picking a **hybrid** with two parts that each do exactly one thing:

### Part 1 — Dispatcher: auto-derive a "write-only" note in `${scope-block}`

Extend `renderScopeBlock` (`internal/atdd/runtime/clauderun/clauderun.go:909`) to compute the set `write \ read` and, when non-empty, emit a single derived line after the "may modify" block:

```
- `dsl-core`: <path>

Write-only paths (in `write:` but not `read:`): dsl-core. Treat these as
append-only or edit-by-location — do not read their existing contents for context.
```

This makes the asymmetry **visible to the agent at every dispatch**, with a generic enforceable rule ("don't read for context"), and it auto-applies to any future write-only entry the operator introduces without further prompt-content edits. The rule is now where the operator can already see it (in the rendered scope block) and the agent doesn't need a paragraph telling them the lists are different.

### Part 2 — `process-flow.yaml`: per-MID `scope-rationale:` field (optional, free-form)

Add an optional sibling field on `EXECUTE_AGENT`, parallel to `read:` / `write:`:

```yaml
- id: EXECUTE_AGENT
  type: call-activity
  process: execute-agent
  name: "Dispatch the Agent"
  params:
    task-name: write-acceptance-tests
    agent: acceptance-test-writer
    category: test-agent
  read:  [at-test, dsl-port]
  write: [at-test, dsl-port, dsl-core]
  scope-rationale: |
    dsl-core is write-only because the test-writer appends `TODO: DSL`
    stubs there so the project compiles; reading existing dsl-core content
    would leak implementation context into test design.
```

The dispatcher renders this (when present) directly under the auto-derived "Write-only paths" line in `${scope-block}`, prefixed with `Why:` so the rationale lands next to the rule it explains. Empty / absent → no rendered block — non-asymmetric MIDs are unaffected. This is the schema slot the YAML comment was approximating; it earns its slot because the dispatcher actually branches on its presence.

### Why hybrid, not (a) or (b) alone

- **Pure (a) — inline YAML comment in the MID.** The comments already exist (lines 1319-1321, 1363); they didn't stop us from having to encode the same rule in prompts because *they're invisible to the agent*. YAML comments are for the schema editor, not the dispatcher. Picking (a) without a rendering change means the prompt sentence stays where it is — the audit finding is unresolved.

- **Pure (b) — only the dispatcher-side annotation.** The auto-derived "write-only paths" line is enough for the generic rule ("don't read for context") but loses the per-MID *reason* — "this isn't an arbitrary policy; it exists because reading impl leaks into test design." For the two test-writer MIDs the rationale is load-bearing teaching material; collapsing it to a generic "don't read for context" loses the explanation an operator new to ATDD needs. The free-form field carries the per-MID why.

- **Hybrid (c).** Part 1 surfaces the *what* generically (every dispatch, any write-only path). Part 2 carries the *why* per-MID (currently only the two test writers populate it). They don't overlap; each is the minimum mechanism for what it does. Per the "schema fields must earn their slot" rule, `scope-rationale:` earns its slot because the dispatcher renders it conditionally and findUnfilledPlaceholders catches a missing one if a future MID body references it — that's the branching that justifies the slot.

### Net effect on the two writer prompts

After this change, the trailing sentence in both prompts becomes redundant and gets deleted:

- `acceptance-test-writer.md:21` — drop the final sentence of Step 2 (from "The asymmetric scope (dsl-core is writeable but not in `read:`) is deliberate…" through end of line). Keep the preceding instruction about limiting dsl-core reads to "identifying where to append or what to fix" — that's the *concrete behaviour* the agent must perform; the *rationale* now lives in the scope block.
- `contract-test-writer.md:19` — same delete.

The "limit your dsl-core read to identifying where to append" sentence can stay in the prompts because it's an action instruction, not a rationale; the dispatcher-side "Write-only paths" line states the rule, the prompt restates the test-writer-specific application (append vs. read for context). Whether to also collapse the action instruction is a follow-up question outside this plan's scope (the dispatcher annotation already does the heavy lifting).

## Items

### 1. Add `scope-rationale:` to the YAML node schema and engine accessor

**Files:**
- `internal/atdd/runtime/statemachine/types.go` — add a `ScopeRationale string` field on the relevant Raw-node struct (whatever currently holds `Read` / `Write`), and extend `Engine.Scope` (line 157) to additionally return the rationale string — or add a sibling `Engine.ScopeRationale(processName) (string, bool)` accessor to keep `Scope`'s signature stable. Pick the lower-churn shape during execution.
- `internal/atdd/runtime/statemachine/load.go` — verify the loader passes the new field through from YAML to the Raw struct (likely already automatic if it round-trips via `yaml.v3` tags).

**Acceptance:** `Engine.Scope("write-acceptance-tests")` (or its sibling) returns the rationale string when present; returns empty when absent. No existing call site breaks.

### 2. Pipe rationale through the driver to `clauderun.Options`

**Files:**
- The driver call site that populates `Options.ScopeRead` / `Options.ScopeWrite` from `Engine.Scope` (grep `ScopeRead:` in the driver package). Add a parallel `ScopeRationale` field on `Options` and seed it alongside the existing two.

**Acceptance:** When a MID declares `scope-rationale:`, the dispatch carries it into `Options.ScopeRationale`. When the MID omits the field, `Options.ScopeRationale` is empty.

### 3. Extend `renderScopeBlock` to emit the asymmetry annotation + rationale

**File:** `internal/atdd/runtime/clauderun/clauderun.go` — modify `renderScopeBlock` (line 909). Pseudocode for the new tail of the function:

```go
// Compute write \ read (preserving write-order for stable output).
readSet := map[string]bool{}
for _, k := range read {
    readSet[k] = true
}
var writeOnly []string
for _, k := range write {
    if !readSet[k] {
        writeOnly = append(writeOnly, k)
    }
}
if len(writeOnly) > 0 {
    fmt.Fprintf(&b, "\nWrite-only paths (in `write:` but not `read:`): %s. "+
        "Treat these as append-only or edit-by-location — do not read their "+
        "existing contents for context.\n", strings.Join(writeOnly, ", "))
    if rationale != "" {
        fmt.Fprintf(&b, "Why: %s\n", strings.TrimSpace(rationale))
    }
}
b.WriteString("\nReading or writing outside this set requires a `scope_exception` block.")
```

Change the function signature to `renderScopeBlock(read, write []string, paths map[string]string, rationale string)` and update both call sites (production + tests).

**Acceptance:**
- For `write-acceptance-tests` / `write-contract-tests`, the rendered block now ends with the auto-derived "Write-only paths:" line + "Why: dsl-core is write-only because …" + the existing scope_exception sentence.
- For every other MID (symmetric or read-only-not-write), nothing new is rendered.

### 4. Populate `scope-rationale:` on the two affected MIDs

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

For `write-acceptance-tests` (around line 1353-1354) and `write-contract-tests` (around line 1389-1390), add the new field directly under `write:`:

```yaml
        read:  [at-test, dsl-port]
        write: [at-test, dsl-port, dsl-core]
        scope-rationale: |
          dsl-core is write-only because the test-writer appends `TODO: DSL`
          stubs there so the project compiles; reading existing dsl-core
          content would leak implementation context into test design.
```

(Same wording for `write-contract-tests`, swap `at-test` → `ct-test` if the prose ever names it.)

Also delete the YAML comments at lines 1319-1321 and 1363 that currently encode the rationale at the schema-editor level — they're now redundant with the inline `scope-rationale:` and would just drift.

**Acceptance:** Both MIDs carry the field; no other MID does (until/unless a future write-only asymmetry is introduced, at which point the operator can choose whether to add a rationale or accept the generic auto-derived line).

### 5. Delete the duplicated sentence from both writer prompts

**Files:**
- `internal/assets/runtime/agents/atdd/acceptance-test-writer.md:21` — strip from the last sentence of Step 2: ` The asymmetric scope (dsl-core is writeable but not in `read:`) is deliberate: reading impl context would leak it into test design.` Leave the preceding "Limit your dsl-core read to identifying where to append or what to fix — do not read existing method bodies or browse other dsl-core files to 'understand the structure'." sentence intact (action instruction, not rationale).
- `internal/assets/runtime/agents/atdd/contract-test-writer.md:19` — same delete.

**Acceptance:** Both prompts no longer carry the asymmetry-rationale sentence; the agent still receives the rationale via `${scope-block}` at dispatch time.

### 6. Tests

**File:** `internal/atdd/runtime/clauderun/clauderun_test.go` — extend the existing `${scope-block}` rendering tests (around line 283-289) with two cases:

- **write-only-with-rationale:** `ScopeRead = [dsl-port]`, `ScopeWrite = [dsl-port, dsl-core]`, `ScopeRationale = "test rationale text"`. Assert the rendered block contains "Write-only paths (in `write:` but not `read:`): dsl-core" AND "Why: test rationale text" AND the existing "`scope_exception`" line.
- **symmetric-no-rationale:** `ScopeRead = ScopeWrite = [system-path]`, `ScopeRationale = ""`. Assert the rendered block does NOT contain "Write-only paths" and does NOT contain "Why:".

**File:** `internal/atdd/runtime/statemachine` test for `Engine.Scope` (or the new accessor) — add a fixture covering "rationale present" and "rationale absent" cases.

**Acceptance:** Both new test cases pass; existing tests for the symmetric-scope path (line 270-289) still pass unchanged.

## Verification

- Run the clauderun + statemachine test packages (scoped, not `go test ./...` — see the Windows hazard memory note).
- Eyeball the rendered `${scope-block}` for `write-acceptance-tests` by dispatching a dry-run prompt log (`Options.PromptLogPath` set, `Headless=true` doesn't matter for the log).
- Confirm `process-flow.yaml` still loads cleanly via the statemachine loader (its existing schema-validation test will catch a typo in the new field tag).

## Non-goals

- Generalising `scope-rationale:` to read-only-not-write MIDs. Those already have the "Reading or writing outside this set requires a `scope_exception` block" line, which is the generic guardrail; per-MID rationale for the symmetric / read-supercedes-write cases is out of scope until/unless a concrete need surfaces.
- Touching the audit's Findings 1, 2, 4 — those are separate plans.
- Editing `docs/atdd/architecture/*.md` — the asymmetry is a *process-flow scoping* rule, not an *architectural layer* rule, so it doesn't belong in the per-layer architecture docs.
