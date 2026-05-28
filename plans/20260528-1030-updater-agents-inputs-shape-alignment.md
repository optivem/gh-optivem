# 20260528-1030 — Updater agents `## Inputs` shape alignment

**Lineage:** spinoff from `plans/20260527-2214-runtime-prompts-audit.md` Finding 1 ("`external-system-driver-adapter-updater.md` parameter list omits `checklist`, but Step 1 references it"), which routed this drift to `architecture-sync` for SSoT confirmation. This plan is the routing destination.

**Scope:** five "updater"-class ATDD agent prompt bodies. Align their `## Inputs` section to a single canonical shape that matches the dispatcher's substitution contract. **No edits to `process-flow.yaml`** — the MID layer is already the SSoT and is already correct.

## Background — what the SSoT actually is

The original audit Finding 1 calls the "MID parameter declarations in `process-flow.yaml`" the SSoT. That phrasing is loose. Concrete SSoT mechanics (confirmed by code reading):

- `internal/atdd/runtime/statemachine/process-flow.yaml` MID nodes declare a `params:` map containing **only** `task-name`, `agent`, `category` — see e.g. lines 1481–1485 (`update-system`), 1535–1539 (`update-system-driver-adapters`), 1587–1591 (`update-external-system-driver-adapters`), 1737–1740 (`refactor-tests`), 1760–1763 (`refactor-system`). These params are dispatcher-internal — they do **not** become prompt-body placeholders.
- The MID node also declares `read:` / `write:` scope keys (e.g. lines 1486–1487, 1540–1541, 1592–1593, 1741–1742, 1764–1765). These flow into the agent body via the `${scope-block}` substitution.
- `${architecture}` and `${checklist}` are **dispatcher-global substitutions** seeded into `Context.State` outside the MID layer:
  - `architecture` — seeded by `seedScopeState` from project config (`internal/atdd/runtime/driver/driver.go:488-490`).
  - `checklist` — seeded by `parseTicket` from the ticket body (referenced at `internal/atdd/runtime/driver/driver.go:953`; comment block at 460–477 documents the State-vs-Params split).
- Every dispatched agent therefore receives both substitutions automatically; whether the prompt body chooses to render them is a documentation decision in the prompt, not a wiring decision in the MID.

**SSoT confirmation:** the MID `read:`/`write:` arrays plus the dispatcher-global State seeds (`architecture`, `checklist`) together are the SSoT. Plan items below align prompts to that SSoT.

## Side-by-side comparison — current state

Quick legend: "front-matter Architecture line" = the `Architecture: ${architecture}` line that sits *above* `## Inputs`, separately from any `### Parameters` block inside `## Inputs`.

| Agent (prompt body)                                      | MID (process-flow.yaml) | Front-matter `Architecture:` line | `### Parameters` block? | `### Checklist` substitution? | Notes |
|---|---|---|---|---|---|
| `system-updater.md`                                      | `update-system` @ 1473                                | yes, line 8 (above Inputs) | YES — lists **both** `architecture` and `checklist` (lines 16–19) | YES (lines 21–23) | Only agent that lists `checklist` as a parameter. Front-matter `Architecture:` line is **redundant** with the in-block Parameters entry. |
| `external-system-driver-adapter-updater.md`              | `update-external-system-driver-adapters` @ 1579       | yes, line 8 (above Inputs) | NO | YES (lines 16–18) | Finding 1's flagged case — no Parameters block but Checklist is substituted. |
| `system-driver-adapter-updater.md`                       | `update-system-driver-adapters` @ 1527                | yes, line 8 (above Inputs) | NO | YES (lines 16–18) | Same shape as `external-system-driver-adapter-updater.md`. |
| `system-refactorer.md`                                   | `refactor-system` @ 1751                              | no separate above-Inputs line; `Architecture: ${architecture}` sits **inside** the `### Parameters` block (line 18) | YES — lists **only** `architecture` (line 18) | YES (lines 20–22) | Different position for the Architecture line (inside the block, not above). |
| `test-refactorer.md`                                     | `refactor-tests` @ 1728                               | no separate above-Inputs line | YES — lists **only** `architecture` (line 18) | YES (lines 20–22) | No Architecture rendering at all outside the bullet list (relies entirely on the Parameters bullet). |

Three distinct shapes for what should be one consistent contract:

- **Shape A** (`system-updater`): front-matter Architecture line **plus** an exhaustive Parameters block (lists both substitutions) **plus** the Checklist block. Redundant — Architecture is rendered twice.
- **Shape B** (the two driver-adapter updaters): front-matter Architecture line, **no** Parameters block, Checklist block. Checklist is substituted but never enumerated as an input.
- **Shape C** (the two refactorers): **no** front-matter Architecture line; Parameters block lists only `architecture` (omits `checklist`); Checklist block follows. The Parameters block under-declares.

For comparison, the **implementer** agents (which do *not* receive `checklist`) consistently use: front-matter Architecture line + `### Parameters` block listing only `architecture`. See `system-implementer.md:6-18`, `system-driver-adapter-implementer.md:6-18`, `external-system-driver-adapter-implementer.md:6-18`. The `dsl-implementer.md` is the outlier — no Parameters block, explicit "This task does not receive a substituted artifact input" sentence at line 16. The **writer** agents (`acceptance-test-writer.md`, `contract-test-writer.md`) likewise use no Parameters block; they render the artifact directly under `### Acceptance Criteria`.

## Canonical shape — proposal

Rationale for the canon:

1. The front-matter `Architecture: ${architecture}` line above `## Inputs` is **redundant** with anything inside `## Inputs`. It pre-dates the `### Parameters` block convention. Drop it everywhere it appears alongside a Parameters block (shapes A, B).
2. Inside `## Inputs`, render every **value-bearing** substitution the agent receives, in a consistent order: Scope first, then Parameters bullets, then any substituted artifact block (Checklist / Acceptance Criteria) verbatim.
3. The `### Parameters` block lists every substitution **whose value text appears in the prompt body via `${...}`** — for the five updaters that is `architecture` and `checklist`. Both are SSoT-seeded by the dispatcher; the bullet list documents what arrived, not what was MID-declared.
4. The `### Checklist` block itself renders the substituted text; the Parameters bullet describes what the substitution is. Both are kept — the bullet documents the contract, the block carries the payload. This matches how `acceptance-test-writer.md` treats `acceptance-criteria` (no Parameters bullet because there isn't an `### Parameters` section at all — but the artifact block is present).

**Canonical shape for the five updater agents:**

```markdown
## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).
- `checklist` — the parsed list of changes to apply, surfaced verbatim below.

### Checklist

${checklist}
```

No front-matter `Architecture:` line above `## Inputs`. No `Architecture: ${architecture}` line inside the Parameters block (the bullet **is** the rendering of the substitution — the bullet text describes the parameter, not its value; that's the same convention the implementers already follow).

## Items

### Item 1 — `system-updater.md` — drop redundant Architecture line, add `checklist` bullet

**File:** `internal/assets/runtime/agents/atdd/system-updater.md`

**Current (lines 6–23):**

```markdown
The update-system task reshapes the system surface (`${system-path}`) to absorb a structural-redesign change. A Checklist parsed from the ticket body lists the changes to apply across affected channels.

Architecture: ${architecture}

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).
- `checklist` — the parsed list of changes to apply across affected channels.

### Checklist

${checklist}
```

**Edit:** delete the standalone `Architecture: ${architecture}` line (currently line 8) **and** the blank line that follows it. Tighten the `checklist` bullet wording to match the canon ("the parsed list of changes to apply, surfaced verbatim below") so the five agents read identically.

**Resulting block (lines 6–onwards):**

```markdown
The update-system task reshapes the system surface (`${system-path}`) to absorb a structural-redesign change. A Checklist parsed from the ticket body lists the changes to apply across affected channels.

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).
- `checklist` — the parsed list of changes to apply, surfaced verbatim below.

### Checklist

${checklist}
```

Optional follow-up (not part of this item; flagged so the executor doesn't accidentally double-edit): the introductory paragraph's "A Checklist parsed from the ticket body lists the changes to apply across affected channels." sentence now duplicates the `checklist` bullet. Leave as-is for this plan — alignment first, prose deduplication separately if desired.

### Item 2 — `external-system-driver-adapter-updater.md` — add Parameters block, drop redundant Architecture line

**File:** `internal/assets/runtime/agents/atdd/external-system-driver-adapter-updater.md`

**Current (lines 6–18):**

```markdown
The update-external-system-driver-adapters task reshapes the external-system driver layer (`${external-system-driver-adapter}`) (Ext* DTOs (`${external-system-driver-adapter}`), Real driver (`${external-system-driver-adapter}`), Stub driver (`${external-system-driver-adapter}`)) to match a new external API so DSL, Gherkin, and tests stay untouched. A Checklist parsed from the ticket body lists the changes to apply.

Architecture: ${architecture}

## Inputs

### Scope

${scope-block}

### Checklist

${checklist}
```

**Edit:** delete the standalone `Architecture: ${architecture}` line (currently line 8) and its trailing blank line; insert a `### Parameters` block between `### Scope` and `### Checklist`.

**Resulting block:**

```markdown
The update-external-system-driver-adapters task reshapes the external-system driver layer (`${external-system-driver-adapter}`) (Ext* DTOs (`${external-system-driver-adapter}`), Real driver (`${external-system-driver-adapter}`), Stub driver (`${external-system-driver-adapter}`)) to match a new external API so DSL, Gherkin, and tests stay untouched. A Checklist parsed from the ticket body lists the changes to apply.

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).
- `checklist` — the parsed list of changes to apply, surfaced verbatim below.

### Checklist

${checklist}
```

### Item 3 — `system-driver-adapter-updater.md` — add Parameters block, drop redundant Architecture line

**File:** `internal/assets/runtime/agents/atdd/system-driver-adapter-updater.md`

**Current (lines 6–18):**

```markdown
The update-system-driver-adapters task absorbs a structural-redesign change inside the System Driver adapter layer (`${driver-adapter}`) so DSL, Gherkin, and tests stay untouched. A Checklist parsed from the ticket body lists the changes to apply.

Architecture: ${architecture}

## Inputs

### Scope

${scope-block}

### Checklist

${checklist}
```

**Edit:** same shape change as Item 2 — drop the `Architecture: ${architecture}` line + trailing blank, insert a `### Parameters` block listing both substitutions.

**Resulting block:**

```markdown
The update-system-driver-adapters task absorbs a structural-redesign change inside the System Driver adapter layer (`${driver-adapter}`) so DSL, Gherkin, and tests stay untouched. A Checklist parsed from the ticket body lists the changes to apply.

## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).
- `checklist` — the parsed list of changes to apply, surfaced verbatim below.

### Checklist

${checklist}
```

### Item 4 — `system-refactorer.md` — add missing `checklist` bullet, fix Architecture-line position

**File:** `internal/assets/runtime/agents/atdd/system-refactorer.md`

**Current (lines 10–22):**

```markdown
## Inputs

### Scope

${scope-block}

### Parameters

Architecture: ${architecture}

### Checklist

${checklist}
```

**Edit:** replace the prose-style `Architecture: ${architecture}` line under `### Parameters` with the canonical two-bullet list. This fixes two issues at once: (a) the Parameters block was missing a `checklist` entry even though the agent receives one, and (b) the Architecture line was rendered as a non-bullet prose sentence inside a section that should be a bullet list.

**Resulting block:**

```markdown
## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).
- `checklist` — the parsed list of changes to apply, surfaced verbatim below.

### Checklist

${checklist}
```

### Item 5 — `test-refactorer.md` — add missing `checklist` bullet

**File:** `internal/assets/runtime/agents/atdd/test-refactorer.md`

**Current (lines 10–22):**

```markdown
## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).

### Checklist

${checklist}
```

**Edit:** append a `checklist` bullet after the `architecture` bullet.

**Resulting block:**

```markdown
## Inputs

### Scope

${scope-block}

### Parameters

- `architecture` — architecture profile for the target project (Java/.NET/TS × monolith/multitier).
- `checklist` — the parsed list of changes to apply, surfaced verbatim below.

### Checklist

${checklist}
```

## Out-of-scope (not in this plan)

- `process-flow.yaml` edits. The MID layer already declares the right scope (`read:` / `write:`) and dispatches the right agent; `architecture` and `checklist` flow via `Context.State` seeds by design (`driver.go:478-494`, `:935-954`). No MID-layer change is needed.
- Implementer-agent shape alignment. Implementers (`system-implementer`, `system-driver-adapter-implementer`, `external-system-driver-adapter-implementer`) already use a coherent shape with a front-matter Architecture line + Parameters bullet. They redundantly render `${architecture}` twice (front-matter line + bullet) — that's the same redundancy this plan fixes for updaters, but the implementers don't share the original audit Finding 1 trigger, so deferring to a separate sweep keeps this plan tightly scoped to Finding 1's five agents.
- `dsl-implementer.md` and the writer agents (`acceptance-test-writer.md`, `contract-test-writer.md`). They follow a different convention (no Parameters block) because the substituted artifact (`acceptance-criteria`) is the focus; aligning them is a separate design decision.
- Front-matter `Architecture: ${architecture}` deduplication in the **implementer** agents. Listed here only to document why this plan leaves them alone — see point above.

## Verification

After execution, the executor should confirm:

- All five updater prompts have identical `## Inputs` skeletons (Scope → Parameters with both bullets → Checklist).
- No file still contains a standalone `Architecture: ${architecture}` line above `## Inputs`.
- `rg "^Architecture:" internal/assets/runtime/agents/atdd/{system-updater,external-system-driver-adapter-updater,system-driver-adapter-updater,system-refactorer,test-refactorer}.md` returns zero matches.
- `rg "checklist.*parsed list of changes to apply, surfaced verbatim below" internal/assets/runtime/agents/atdd/` returns five matches (one per updater).
- `go test ./internal/atdd/...` still passes (no test should be coupled to the front-matter Architecture line; if any test grep-pins it, that's a separate finding to surface).
