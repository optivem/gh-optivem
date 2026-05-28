# 20260528-1028 — system-implementer Prose-vs-Scope Drift

Spinoff from `plans/20260527-2214-runtime-prompts-audit.md` (Finding 2: "`system-implementer.md:22` prose names a subset of its read-scope"). The audit routed this to architecture-sync / process-audit; this plan proposes the concrete edits.

## Background — finding and evidence

**Prose at `internal/assets/runtime/agents/atdd/system-implementer.md:22`:**

> Read the failing acceptance test to see the required behaviour, then trace through the DSL, the driver port, and the driver adapter to see how the test reaches the production system (and which stubbed external interactions, if any, the test stages). The scope block above lists every layer you may read.

Layers named in prose: `at-test` ("the failing acceptance test"), `dsl-port` ("the DSL"), `driver-port`, `driver-adapter`, and a vague gesture at the external-system layer ("which stubbed external interactions, if any, the test stages").

**MID-declared scope at `internal/atdd/runtime/statemachine/process-flow.yaml:1456`** (the audit cited `:1442`, which was the comment header; the `read:` line is now `1456`):

```yaml
read:  [at-test, ct-test, dsl-port, dsl-core, driver-port, driver-adapter, external-system-driver-port, external-system-driver-adapter, system-path]
write: [system-path]
```

Comment header at `process-flow.yaml:1438-1442` explaining intent:

> Asymmetric: reads the full system-test layer (the failing AT, its contracts, the DSL it speaks, the driver port/adapter pair and the external-system driver pair the AT may stage) so the agent can see *what* behaviour to implement. Writes only system-path — any change to driver/test code requires a scope_exception.

**Gap:** Prose drops `ct-test`, `dsl-core`, `external-system-driver-port`, `external-system-driver-adapter` (and names `at-test` and `dsl-port` only in colloquial form, without their `${...}` placeholders).

## Intent vs drift — analysis

Both the prose and the expanded scope landed in the **same commit**, `ff4b616` (Wed 2026-05-27, "fix(runner): multi-test filter for TypeScript and .NET"). That commit:

- Expanded the MID scope from `read: [system-path]` to the full 9-layer list, and
- Added the new Step 1 prose ("Read the failing acceptance test … trace through the DSL, the driver port, and the driver adapter … The scope block above lists every layer you may read.").

So the prose under-enumeration is **not stale drift from a smaller scope era** — the author knew the scope was 9 layers wide while writing 3-layer prose, and explicitly hedged with the trailing disclaimer. The prose is a deliberate sketch leaning on `${scope-block}` for the authoritative list.

However, the prose is the **odd one out** among implementer prompts. Every other ATDD writing agent enumerates its layers using `${placeholder}` references (e.g. `acceptance-test-writer.md:21` references both `${dsl-port}` and `${dsl-core}` explicitly; `dsl-implementer.md:22-23` enumerates all four port/adapter pairs by placeholder). The system-implementer prose breaks that pattern by:

1. Naming layers in conversational prose ("the DSL") rather than by placeholder (`${dsl-port}`).
2. Listing a strict subset, then telling the agent to consult the scope block for the rest.
3. Saying nothing concrete about the external-system pair, which is in scope precisely because acceptance tests stage stub external interactions through it.

The audit's concern stands: for tickets where the AT stages external-system behaviour, the agent needs to trace through the external-system driver port/adapter pair to understand the stub contract, and the prose currently doesn't tell it that's a valid path to follow. The "every layer you may read" hedge protects against scope-exception complaints, but it's a weak nudge compared to naming the layer.

## Decision needed — widen prose, or narrow scope?

There are two coherent fixes. **This is a one-question decision and the executor should not pick blind.**

### Option A — Widen prose to match the full scope (recommended default)

Rewrite Step 1 to enumerate the four trace-paths (DSL, driver pair, external-system driver pair, contract tests) using their `${placeholder}` references. This brings the prompt into shape-alignment with `dsl-implementer.md` / `acceptance-test-writer.md` and removes the under-sell.

Trade-off: more tokens per dispatch (~40-60 tokens added), and the prose can no longer be a one-line sketch.

### Option B — Narrow MID scope to match the prose

Drop `ct-test`, `dsl-core`, `external-system-driver-port`, `external-system-driver-adapter` from the `read:` set, leaving `[at-test, dsl-port, driver-port, driver-adapter, system-path]`.

Trade-off: the agent loses the ability to read external-system stub configuration when an AT stages it. Per the comment at `process-flow.yaml:1438-1442` the broader scope is described as deliberate ("the external-system driver pair the AT may stage"). Picking Option B contradicts that comment and would require re-writing it too.

### Recommendation

Option A. The expanded scope was added intentionally in `ff4b616` with a comment block defending it; narrowing now would undo that decision without new evidence. The token cost of an enumerated trace-path list is small and consistent with peer prompts. But the user has to confirm — this is a contract-shape question, not a typo fix.

## Items

### 1. [DECISION] Confirm widen-prose (Option A) vs narrow-scope (Option B)

**Question for the user:** Widen the system-implementer prose to enumerate `${dsl-port}` / `${dsl-core}` / `${driver-port}` / `${driver-adapter}` / `${external-system-driver-port}` / `${external-system-driver-adapter}` / `${ct-test}` explicitly (Option A), or narrow the MID `read:` set to the smaller layer list named in prose today (Option B)?

If Option A: proceed to Item 2.
If Option B: proceed to Item 3 instead, and skip Item 2.

### 2. [PROSE EDIT — gated on Option A] Rewrite Step 1 of `system-implementer.md`

**File:** `internal/assets/runtime/agents/atdd/system-implementer.md`

**Current line 22:**

> Read the failing acceptance test to see the required behaviour, then trace through the DSL, the driver port, and the driver adapter to see how the test reaches the production system (and which stubbed external interactions, if any, the test stages). The scope block above lists every layer you may read.

**Proposed replacement (Option A — enumerate by placeholder, peer-aligned):**

> Read the failing Acceptance Test (`${at-test}`) to see the required behaviour, then trace through the DSL Port (`${dsl-port}`) and DSL Core (`${dsl-core}`) to the System Driver port/adapter pair (`${driver-port}`, `${driver-adapter}`) to see how the test reaches the production system. If the test stages stub external interactions, also read the External System Driver port/adapter pair (`${external-system-driver-port}`, `${external-system-driver-adapter}`) and the Contract Tests (`${ct-test}`) to see the stub contract the implementation must satisfy.

**Rationale:**
- Brings the prompt into shape-alignment with peer implementer prompts (`dsl-implementer.md:22-23`, `acceptance-test-writer.md:21`) which enumerate every in-scope layer by `${placeholder}`.
- Closes the audit Finding 2 gap by naming `ct-test`, `dsl-core`, and the external-system driver pair explicitly.
- Drops the "The scope block above lists every layer you may read" hedge — once the prose enumerates the layers, the hedge is redundant. `${scope-block}` remains the canonical reference for path values.

**Verification:**
- Inspect rendered prompt for one `implement-system` dispatch (any change-system-behavior CYCLE ticket) — confirm the new line substitutes all seven placeholders.
- Confirm no peer prompt regressed (the change is local to `system-implementer.md`).

### 3. [MID EDIT — gated on Option B, mutually exclusive with Item 2] Narrow `implement-system` `read:` set

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

**Current line 1456:**

```yaml
read:  [at-test, ct-test, dsl-port, dsl-core, driver-port, driver-adapter, external-system-driver-port, external-system-driver-adapter, system-path]
```

**Proposed replacement (Option B):**

```yaml
read:  [at-test, dsl-port, driver-port, driver-adapter, system-path]
```

**Also rewrite the comment header at `process-flow.yaml:1438-1442`:**

Current:

> Asymmetric: reads the full system-test layer (the failing AT, its contracts, the DSL it speaks, the driver port/adapter pair and the external-system driver pair the AT may stage) so the agent can see *what* behaviour to implement. Writes only system-path — any change to driver/test code requires a scope_exception.

Proposed:

> Asymmetric: reads the AT, the DSL port (signatures only), and the driver port/adapter pair so the agent can see *what* behaviour to implement. Writes only system-path — any change to driver/test code, or any read of DSL Core / contract tests / external-system driver code, requires a scope_exception.

**Rationale:**
- Aligns the contract with the existing prose intent in `system-implementer.md:22`.
- Loses the ability to inspect external-system stub config; the agent would need to file `scope_exception` for tickets that stage external behaviour.

**Verification:**
- Re-run statemachine tests under `internal/atdd/runtime/statemachine/` to confirm no test pins the prior 9-layer read set as a fixture.
- Walk one change-system-behavior CYCLE that stages external-system behaviour (e.g. eshop) and confirm the agent either succeeds with the narrower scope or emits a `scope_exception` cleanly.
- Verify no other MID copies the 9-layer set verbatim and so needs the same narrowing.

## Out-of-scope (do not touch in this plan)

- The other three audit findings (audit plan items 1, 3, 4) — they have their own routing.
- The `${scope-block}` renderer in `internal/atdd/runtime/clauderun/clauderun.go:909-942` — its behaviour is correct regardless of which option is chosen here.
- Peer implementer prompts whose prose-vs-scope alignment is already correct (`dsl-implementer.md`, `acceptance-test-writer.md`, `system-driver-adapter-implementer.md`, `external-system-driver-adapter-implementer.md`).
