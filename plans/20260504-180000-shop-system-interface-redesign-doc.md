# Shop-side phase doc for `system-interface-redesign`

## Context

The runtime change in `plans/20260504-160000-templated-intake-and-classification.md` replaced the four-value `change_*` classification with a single `subtype` field carried on a `subtype:*` label, and collapsed the System API / System UI / Mobile / CLI / admin channels into one `system-interface-redesign` cycle. The corresponding YAML now points at a phase doc that doesn't yet exist on disk:

```yaml
# internal/atdd/runtime/statemachine/process-flow.yaml — gh-optivem
da_cycle:
  ...
  - id: SYSTEM_INTERFACE_CYCLE
    type: call_activity
    flow: structural_cycle
    params:
      phase: "SYSTEM INTERFACE REDESIGN"
      agent: atdd-task
      phase_doc: docs/atdd/process/system-interface-redesign.md   # ← does not exist yet
      subtype: system-interface-redesign
```

The `phase_doc` value is a relative path inside the *student repo* (the scaffolded shop template), not gh-optivem itself. The WRITE agent reads it via the standard prompt-substitution path (`Phase doc: ${phase_doc}` in `atdd-task.md`) and the agent then loads the file as a reference.

No test in gh-optivem fails on the missing file — the WRITE agent gracefully degrades to working from the ticket body — but every WRITE dispatch in a real student repo will surface a "phase doc not found" warning until the file lands.

## Motivation

The old `task-and-chore-cycles.md` documented two parallel cycles ("SYSTEM API REDESIGN" and "SYSTEM UI REDESIGN") whose mechanics differ only in the boundary-specific files they touch. With the channel concept retired (see the parent plan's Decision 4), that doc has two stale problems:

1. It describes a "SYSTEM API REDESIGN" / "SYSTEM UI REDESIGN" split that no longer exists at the runtime level — both phases are now one `SYSTEM INTERFACE REDESIGN` cycle.
2. It uses the old ticket-type names (`system-api-task`, `system-ui-task`, `external-api-task`, `chore`) throughout, which no longer match the issue forms or the runtime classification fields.

The fix is to consolidate the doc into one channel-agnostic phase reference and drop the old type names.

## Items

### 1. Create `shop/docs/atdd/process/system-interface-redesign.md`

**Files (new):**
- `docs/atdd/process/system-interface-redesign.md` (in the `shop` repo)

Single phase doc covering the WRITE / REVIEW / TEST / COMMIT mechanics of the SYSTEM INTERFACE REDESIGN cycle, channel-agnostic. Source content: the existing "SYSTEM API REDESIGN / SYSTEM UI REDESIGN" section in `task-and-chore-cycles.md`, generalized so it covers any system-side driver (API, UI, mobile, CLI, admin, …).

Key edits relative to the source:
- Title: "SYSTEM INTERFACE REDESIGN" (no API/UI suffix).
- Intro: "Reshape the system's surface — controllers, DTOs, routes, status codes, error format; or page structure, form fields, navigation, copy, selectors; or the equivalents for any other channel the student repo exposes." Replace the two-paragraph API-vs-UI fork with a single paragraph that lists driver folders by example, not exhaustively.
- Step 1 of WRITE: have the agent read the Checklist + system tree to determine which driver(s) to modify, instead of branching on a pre-classified channel.
- Drop the "boundary ∈ {API, UI}" framing — replace with "the boundary the ticket targets, identified by the agent from the Checklist."
- Commit-message phase: `<Ticket> | SYSTEM INTERFACE REDESIGN`.

The agent prompt in gh-optivem (`internal/atdd/runtime/agents/prompts/atdd-task.md`) was already updated to match this framing in the parent plan; this doc is the long-form reference the prompt links to.

### 2. Update `shop/docs/atdd/process/task-and-chore-cycles.md`

**Files (edit):**
- `docs/atdd/process/task-and-chore-cycles.md` (in the `shop` repo)

Bring the doc in line with the new classification:
- "Purpose" header: replace "the four structural cycles triggered by ticket types `system-api-task`, `system-ui-task`, `external-api-task`, and `chore`" with "the structural cycles triggered by the three `subtype:*` labels: `subtype:system-interface-redesign`, `subtype:external-system-interface-redesign`, `subtype:system-implementation-change`."
- Replace the standalone "SYSTEM API REDESIGN / SYSTEM UI REDESIGN" section with a stub that points to `system-interface-redesign.md` (item 1) for the channel-agnostic mechanics.
- Replace every occurrence of `system-api-task` / `system-ui-task` / `external-api-task` / `chore` (as ticket-type names) with the new subtype names. Keep historical references that explain *why* the names changed if a migration note adds value; otherwise just rename.
- Keep the EXTERNAL API REDESIGN section (still routes through CT — only the heading wording changes to match the `external-system-interface-redesign` subtype).
- Keep the CHORE section but reframe it as "the WRITE phase of the `system-implementation-change` subtype" — the cycle still exists, just under the runtime name `sut_cycle` rather than a separate ticket-type cycle.

### 3. Cross-check other shop-side references

**Files (search and update as needed):**
- `docs/atdd/process/glossary.md`
- `docs/atdd/process/cycles.md`
- `docs/atdd/architecture/system.md`
- `docs/atdd/architecture/driver-port.md`
- `docs/atdd/architecture/driver-adapter.md`
- Any other `docs/atdd/**/*.md` that mentions the deprecated names

Grep the shop docs tree for the deprecated ticket-type names and the deprecated phase names. Update each occurrence to the new vocabulary; leave migration notes only where students are actively likely to find old tickets in flight.

```bash
grep -rn -E 'system-api-task|system-ui-task|external-api-task|atdd-chore|SYSTEM API REDESIGN|SYSTEM UI REDESIGN' docs/
```

## Out of scope

- Any change to gh-optivem itself — the runtime work is already merged in the parent plan.
- Creating analogous docs for `external-system-interface-redesign` or `system-implementation-change` if they don't already exist; only consolidate what's there. (Open follow-up plans if either subtype needs its own dedicated phase doc.)
- Migrating in-flight student tickets to the new subtype names. The new docs apply to newly-filed tickets; old tickets keep their current handling.
- Updating the `atdd-chore` agent prompt in gh-optivem — already done in the parent plan.
