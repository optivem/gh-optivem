# Templated intake and form-driven classification

> 🤖 **Picked up by agent** — `Valentina_Desk` at `2026-05-04T10:46:06Z`

## Motivation

The `intake` process in `internal/atdd/runtime/statemachine/process-flow.yaml:141-211` fans out to four near-identical agents (`atdd-story`, `atdd-bug`, `atdd-task`, `atdd-chore`). Each one does the same shape of work — read the ticket body, extract the canonical sections (Acceptance Criteria, Checklist, Legacy Acceptance Criteria), classify the change. The differences between the four agents are small enough that they could be one prompt, and most of the work isn't actually creative — it's parsing markdown.

Today the agents have to be LLM-driven because ticket bodies are free-form markdown: headings drift ("AC" vs "Acceptance Criteria:" vs "## Acceptance Criteria"), sections appear in arbitrary order, and Given/When/Then blocks are written as prose. An LLM absorbs that noise; a parser would not.

The fix is upstream: enforce ticket structure at creation time with **GitHub Issue Forms** (`.github/ISSUE_TEMPLATE/*.yml`). With forms enforcing the structure:

- The ticket type lands in the **native GitHub issue type** field (Story / Bug / Task), set by the form's `type:` field — `auto-classify-ticket` becomes "read `issue.type`", no LLM.
- The structural-change subtype lands in a `subtype:*` **label** on task tickets — `auto-classify-subtype` becomes "read the label", no LLM.
- Section headings are guaranteed canonical — intake becomes a deterministic markdown parser, not an agent.
- The four type-specific intake agents collapse to one service task.
- `STOP_INTAKE` shrinks from "human approves scenarios" to "human resolves parse errors" (and disappears for happy-path tickets).

The win cascades: forms remove ambiguity → parser replaces agent → `STOP_INTAKE` shrinks → diagram simpler → less LLM dispatch → faster, cheaper, deterministic.

The pedagogical signal is preserved: students still write Acceptance Criteria as Given/When/Then, still produce a Checklist for structural work, still have a Legacy Acceptance Criteria backfill. They just do it inside a form whose required fields make it impossible to ship a ticket missing the section the cycle needs.

## Decisions

1. **Fallback for non-template tickets: reject.** Tickets without a native issue type, or (for tasks) without a `subtype:*` label, or with missing/malformed canonical headings, fail intake fast. No LLM fallback path, no retained intake agents for "legacy" tickets. Students learn the template path; one explicit rejection message teaches more than a graceful fallback hides. The four intake agents (`atdd-story`, `atdd-bug`, `atdd-task`, `atdd-chore`) are deleted outright in their intake role — see item 8.

   Implication for `STOP_CLASSIFY_CONFLICT`: it stays as the unhappy-path stop, but the resolution is "go fix the ticket (set its issue type, apply a `subtype:*` label, or recreate via the template) and re-run", not "the human supplies the missing classification inline."

   Implication for `STOP_PARSE_ERROR`: same shape — "go fix the ticket body to match the template, then re-run." Not "the human writes the missing scenarios inline."

2. **Implementation order: forms-first.** Scaffold the three issue-form YAMLs (item 1) and submit one test ticket per form against a sandbox repo before any runtime work begins. The `parse_ticket_body` action's contract depends on the exact markdown GitHub emits when an issue form is submitted — observe that against real output rather than fitting forms to a parser written from docs. Items 2–10 only start once the rendered markdown shape is confirmed.

3. **Type via native GitHub issue type, not labels.** GitHub Issues now have a first-class `type` field (Story / Bug / Task) — a typed enum, not a label. Use it.

   - Forms set the field via `type:` (alongside `name:` and `description:`).
   - `auto-classify-ticket` reads `issue.type` from the GitHub API and writes its lowercased name to context as `ticket_type`.
   - No `type:*` labels exist on disk; no label-naming convention to maintain.

   **Setup prerequisite (out of scope for the runtime change but in scope for rollout):** the `optivem` org needs Story, Bug, and Task configured as issue types. Bug and Task are GitHub defaults; Story must be added once as a custom type at the org level. This is a one-time admin task — no scaffolder automation required (it's an org-level setting, not a per-repo one).

4. **Subtype via `subtype:*` labels, not a form dropdown.** The structural subtype is carried by a label, applied by the student after filing.

   Three labels, one per value:

   | Label | `subtype` value | Cycle |
   |---|---|---|
   | `subtype:system-interface-redesign` | `system-interface-redesign` | `da_cycle` → `structural_cycle` |
   | `subtype:external-system-interface-redesign` | `external-system-interface-redesign` | `da_cycle` → `ct_subprocess` |
   | `subtype:system-implementation-change` | `system-implementation-change` | `sut_cycle` |

   Behavioral tickets (Story / Bug) **do not get a `subtype:*` label** — `run_cycle` dispatches them via `ticket_type` straight into `at_cycle`. `subtype:*` is only meaningful for tasks.

   Why labels, not a dropdown:

   - **Symmetric mechanism.** Subtype joins the same world as type-routing already lives in (post-issue-creation, GitHub-native).
   - **Editable post-hoc.** Wrong subtype = swap labels in one click. With a dropdown, the student would have to re-file the issue (the dropdown value is baked into the body and can't be re-read after edit without re-rendering the form).
   - **Form simpler.** `task.yml` loses its dropdown entirely — same fields as story/bug minus AC.
   - **No body parsing for this field.** Parser doesn't need a subtype section in the body — the value lives on a label.
   - **Workflow-friendly.** Bots and Actions can apply labels; they can't edit form-rendered dropdown values.

   **No channel concept.** The old design had four orthogonal runtime fields (`change_type`, `change_subtype`, `change_scope`, `change_channel`). The previous draft of this plan collapsed them to a four-value `subtype` enum that included `system-api-redesign` and `system-ui-redesign` as separate values. That distinction did nothing the framework cared about: both cycles called the same agent (`atdd-task`), the same `structural_cycle`, with the same params bar a phase label and a phase_doc path that pointed at non-existent files. Worse, it forced the *human filing the ticket* to pre-classify what the WRITE agent derives anyway, and it doesn't generalize: a student repo with a CLI driver, mobile driver, or admin-UI driver would need framework code changes to onboard another channel.

   The fix is to drop the channel concept entirely. The WRITE agent reads the system, the ticket body, and the Checklist, and figures out which driver(s) — `SystemApiDriver`, `SystemUiDriver`, `SystemMobileDriver`, etc. — to modify. The framework only forks on distinctions that change the *flow*: behavioral vs structural (different cycles), system vs external-system interface (the latter pulls in `ct_subprocess` and `external_system_onboarding`), interface vs implementation (`da_cycle` vs `sut_cycle`).

   **One structural form, not two.** With channel gone, `task.yml` and `chore.yml` would have identical fields and differ only in their `subtype:*` label. Collapse them: a single `task.yml` covers all structural work, and the runtime dispatches off `subtype`. There is no `chore` ticket type — the implementation-change variant is just one of the three `subtype:*` label values applied to a Task issue.

## Items

Sequence: forms first (so the parser has something concrete to target), then runtime changes, then retire the old agents. One PR per item.

### 5. Add `auto-classify-subtype` service task (label reader)

**Files:**
- `internal/atdd/runtime/actions/bindings.go`
- `internal/atdd/runtime/actions/bindings_test.go`
- `internal/atdd/runtime/statemachine/process-flow.yaml`

New action `ClassifySubtype` runs after `ClassifyTicket`. Sets a single field, `subtype`, by reading `subtype:*` labels on the ticket:

| `ticket_type` | `subtype` set to | Source |
|---|---|---|
| story, bug | (unset — behavioral) | — |
| task | one of `system-interface-redesign` / `external-system-interface-redesign` / `system-implementation-change` | the `subtype:*` label on the issue |

```go
func (a *Actions) ClassifySubtype(ctx context.Context) error {
    if ctx.Get("ticket_type") != "task" {
        return nil // behavioral tickets have no subtype
    }
    labels := a.Board.IssueLabels(ctx)
    var subtypeLabels []string
    for _, l := range labels {
        if strings.HasPrefix(l, "subtype:") {
            subtypeLabels = append(subtypeLabels, l)
        }
    }
    switch len(subtypeLabels) {
    case 0:
        return ErrSubtypeMissing  // → STOP_CLASSIFY_CONFLICT
    case 1:
        ctx.Set("subtype", strings.TrimPrefix(subtypeLabels[0], "subtype:"))
        return nil
    default:
        return ErrSubtypeAmbiguous // → STOP_CLASSIFY_CONFLICT, "exactly one subtype:* label expected"
    }
}
```

For `story` / `bug` it skips entirely — `subtype` is undefined for behavioral tickets and `run_cycle`'s top gate (`ticket_type`) routes them away from the `subtype` gate before that field is ever read.

The runtime fields `change_type`, `change_subtype`, `change_scope`, `change_channel` are **removed** from the context in this same change. Any binding or gate that reads them migrates to `subtype` (item 7 covers the gate updates).

`STOP_CLASSIFY_CONFLICT` now has two failure modes for tasks: missing `subtype:*` label, and multiple `subtype:*` labels. Both are resolved by editing the issue's labels and re-running.

### 6. Replace 4-way intake fan-out with `parse_ticket_body` service task

**Files:**
- `internal/atdd/runtime/actions/bindings.go`
- `internal/atdd/runtime/intake/parse.go` (new)
- `internal/atdd/runtime/intake/parse_test.go` (new)
- `internal/atdd/runtime/statemachine/process-flow.yaml`

New action `ParseTicketBody` (service task) replaces the fan-out. It:

1. Reads the issue body.
2. For each canonical heading (constants from item 3), extracts the section content.
3. For behavioral tickets (story, bug): produces a list of `Scenario` structs from the AC section.
4. For structural tickets (task): produces a list of `ChecklistItem` structs from the Checklist section.
5. For any ticket: extracts `LegacyAcceptanceCriteria` (empty struct when absent — drives the existing `legacy_acceptance_criteria_section_present` gate at `process-flow.yaml:225`).
6. On parse failure (missing required section, malformed scenario, etc.): returns an error → `STOP_INTAKE` becomes the parse-error stop.

Updated `intake` flow:

```yaml
intake:
  start: CLASSIFY_TICKET
  nodes:
    - id: CLASSIFY_TICKET
      type: service_task
      action: classify_ticket
    - id: GATE_CLASSIFY_CONFIDENT
      type: gateway
      binding: classify_confident
    - id: STOP_CLASSIFY_CONFLICT
      type: user_task
      agent: human
    - id: CLASSIFY_SUBTYPE
      type: service_task
      action: classify_subtype
    - id: PARSE_BODY
      type: service_task
      action: parse_ticket_body
    - id: GATE_PARSE_OK
      type: gateway
      binding: parse_ok
    - id: STOP_PARSE_ERROR
      type: user_task
      agent: human
      description: "STOP - HUMAN REVIEW — fix ticket body / re-run"
    - id: INTAKE_END
      type: end_event
  sequence_flows:
    - {from: CLASSIFY_TICKET,         to: GATE_CLASSIFY_CONFIDENT}
    - {from: GATE_CLASSIFY_CONFIDENT, to: CLASSIFY_SUBTYPE,  when: "classify_confident == true"}
    - {from: GATE_CLASSIFY_CONFIDENT, to: STOP_CLASSIFY_CONFLICT, when: "classify_confident == false"}
    - {from: STOP_CLASSIFY_CONFLICT,  to: CLASSIFY_SUBTYPE}
    - {from: CLASSIFY_SUBTYPE,    to: PARSE_BODY}
    - {from: PARSE_BODY,              to: GATE_PARSE_OK}
    - {from: GATE_PARSE_OK,           to: INTAKE_END,            when: "parse_ok == true"}
    - {from: GATE_PARSE_OK,           to: STOP_PARSE_ERROR,      when: "parse_ok == false"}
    - {from: STOP_PARSE_ERROR,        to: PARSE_BODY}
```

Note: `GATE_TICKET_TYPE` and the four `ATDD_*` user_tasks disappear. `STOP_INTAKE` ("approve scenarios") disappears — humans no longer approve agent output because there is no agent.

### 7. Update `run_cycle` and `da_cycle` to gate on `subtype`

**Files:**
- `internal/atdd/runtime/statemachine/process-flow.yaml`
- `internal/atdd/runtime/gates/bindings.go`
- `internal/atdd/runtime/gates/bindings_test.go`
- `internal/atdd/runtime/gates/registry.go`

Today `run_cycle` (`process-flow.yaml:254-289`) gates on `change_type` then `change_subtype`, and `da_cycle` (`process-flow.yaml:648-693`) gates on `change_scope` then `change_channel`. With the flat `subtype` enum (Decision 4) those four nested gates collapse to one gate per cycle. The system/UI split inside `da_cycle` also collapses (Decision 4) — both used the same agent and the same `structural_cycle`, so there's nothing to split on.

**`run_cycle` rewrite:**

```yaml
run_cycle:
  start: GATE_TICKET_TYPE
  nodes:
    - id: GATE_TICKET_TYPE
      type: gateway
      binding: ticket_type
      description: "Behavioral or structural?"
    - id: GATE_SUBTYPE
      type: gateway
      binding: subtype
      description: "Cycle by subtype"
    - id: AT_CYCLE
      type: call_activity
      flow: at_cycle
    - id: DA_CYCLE
      type: call_activity
      flow: da_cycle
    - id: SUT_CYCLE
      type: call_activity
      flow: sut_cycle
    - id: CYCLE_END
      type: end_event
  sequence_flows:
    - {from: GATE_TICKET_TYPE, to: AT_CYCLE,         when: "ticket_type in [story, bug]"}
    - {from: GATE_TICKET_TYPE, to: GATE_SUBTYPE, when: "ticket_type == task"}
    - {from: GATE_SUBTYPE, to: DA_CYCLE,         when: "subtype in [system-interface-redesign, external-system-interface-redesign]"}
    - {from: GATE_SUBTYPE, to: SUT_CYCLE,        when: "subtype == system-implementation-change"}
    - {from: AT_CYCLE,  to: CYCLE_END}
    - {from: DA_CYCLE,  to: CYCLE_END}
    - {from: SUT_CYCLE, to: CYCLE_END}
```

**`da_cycle` rewrite:**

```yaml
da_cycle:
  start: GATE_SUBTYPE
  nodes:
    - id: GATE_SUBTYPE
      type: gateway
      binding: subtype
      description: "System or external-system interface?"
    - id: SYSTEM_INTERFACE_CYCLE
      type: call_activity
      flow: structural_cycle
      params:
        phase: "SYSTEM INTERFACE REDESIGN"
        agent: atdd-task
        phase_doc: docs/atdd/process/system-interface-redesign.md
        subtype: system-interface-redesign
    - id: EXTAPI_CYCLE
      type: call_activity
      flow: ct_subprocess
    - id: DA_END
      type: end_event
  sequence_flows:
    - {from: GATE_SUBTYPE, to: SYSTEM_INTERFACE_CYCLE, when: "subtype == system-interface-redesign"}
    - {from: GATE_SUBTYPE, to: EXTAPI_CYCLE,           when: "subtype == external-system-interface-redesign"}
    - {from: SYSTEM_INTERFACE_CYCLE, to: DA_END}
    - {from: EXTAPI_CYCLE,           to: DA_END}
```

The WRITE agent (`atdd-task`) reads the system, the ticket body, and the Checklist to figure out which driver(s) — API, UI, mobile, CLI, admin, etc. — to modify. The framework no longer pre-classifies the channel.

**Gate-binding changes (`internal/atdd/runtime/gates/bindings.go`):**
- Add: `Subtype(ctx) string` — returns the value set by `ClassifySubtype` (item 5).
- Remove: `ChangeType`, `ChangeSubtype`, `ChangeScope`, `ChangeChannel` and their registry entries.
- Keep: `TicketType` (already exists; now consumed by `run_cycle` directly instead of solely by intake).

### 8. Retire the four intake agents

**Files (delete):**
- `docs/atdd/process/intake-story.md`
- `docs/atdd/process/intake-bug.md`
- `docs/atdd/process/intake-task.md`
- `docs/atdd/process/intake-chore.md`
- Any embedded agent prompt files for `atdd-story`, `atdd-bug`, `atdd-task`, `atdd-chore` under `internal/atdd/runtime/agents/` (intake role only)

**Files (update):**
- `internal/atdd/runtime/agents/registry.go` — remove the four agent registrations (intake role)
- `internal/atdd/runtime/agents/embed.go` — remove the four embed directives (intake role)

Cross-check: `atdd-task` is reused as the WRITE agent in `da_cycle`'s `SYSTEM_INTERFACE_CYCLE` (item 7), and `atdd-chore` is reused as the WRITE agent in `sut_cycle` (`process-flow.yaml:709`). Those are *not* the intake agents — they're structural-cycle WRITE agents that share the names. Confirm whether intake and WRITE roles share a registry entry in `internal/atdd/runtime/agents/registry.go` before deleting; if so, keep the agent and only retire its intake role. If they're distinct, the name collision is itself a smell — fix during this work.

### 9. Update tests, diagrams, docs

**Files:**
- `internal/atdd/runtime/statemachine/transitions_test.go`
- `internal/atdd/runtime/statemachine/structural_cycle_test.go`
- `internal/atdd/runtime/diagram/diagram.go` (if intake fan-out has special-case rendering)
- `internal/atdd/runtime/diagram/diagram_test.go`
- `docs/atdd/process/system-interface-redesign.md` (new) — single phase doc for system Driver Adapter redesigns. Replaces the never-existed `sysapi-redesign.md` / `sysui-redesign.md` references in the old `da_cycle`. The agent reads the system and ticket body to determine which driver(s) — API, UI, mobile, CLI, etc. — to modify.
- `docs/process-diagram.md` — regenerate
- `docs/images/process-diagram-*-intake-cycle.svg`, `*-da-cycle.svg`, `*-run-cycle.svg` — regenerate (intake fan-out collapses; `da_cycle` and `run_cycle` lose their inner gates and channel split)
- `CLAUDE.md` / agent-facing docs that reference the four intake agents, the deleted `chore` ticket type, or `system-api-redesign` / `system-ui-redesign` as separate kinds
- `internal/atdd/runtime/statemachine/process-flow.yaml` header comment (`process-flow.yaml:248-253`) — update the prose describing `change_type` / `change_subtype` / `change_scope` / `change_channel` to describe the single `subtype` field, sourced from a `subtype:*` label on task tickets

Run `gh optivem atdd regen-diagrams` (or whatever the regen entry point is — see `internal/atdd/runtime/diagram/`) and commit the updated SVGs.

### 10. Smoke test against real GitHub

**Files:** none (manual).

After items 1–9 land:
1. `gh optivem init` a fresh sandbox repo → confirm forms appear in `.github/ISSUE_TEMPLATE/` and the three `subtype:*` labels exist (`gh label list`).
2. Open each of the three forms in the GitHub UI, fill in, submit → confirm the native issue type (Story / Bug / Task) is set on the resulting issue and the rendered body matches what the parser expects.
3. For a task ticket, apply one of the `subtype:*` labels by hand. Run `gh optivem atdd implement-ticket --issue N` → confirm intake passes through `parse_ticket_body` to `RUN_LEGACY_CYCLE` / `RUN_CYCLE` without dispatching any LLM agent, and that `run_cycle` / `da_cycle` route to the correct sub-cycle for each `subtype` value.
4. Create one ticket *outside* the template (raw markdown, no native issue type) → confirm `STOP_CLASSIFY_CONFLICT` fires with a clear message.
5. Create a Task issue with no `subtype:*` label → confirm `STOP_CLASSIFY_CONFLICT` fires with the "missing subtype label" message. Apply a label and re-run; confirm it proceeds.
6. Create a Task issue with two `subtype:*` labels → confirm `STOP_CLASSIFY_CONFLICT` fires with the "ambiguous subtype" message.

## Out of scope

- Migrating existing tickets in production student repos — the new path applies to newly-created tickets; legacy tickets keep their current handling until manually re-classified.
- A YAML linter for issue forms (overkill — four small files, manually verified).
- Org-level configuration of the `Story` custom issue type. The runtime depends on it being present (Decision 3) but creating it is a one-time admin task, not a code change.
- Repurposing the deleted intake agents for other phases — if there's a reuse opportunity, capture it in a separate plan.
- Any change to `at_cycle`, `sut_cycle`, `ct_subprocess`, `external_system_onboarding`, `structural_cycle`, or `legacy_acceptance_criteria` flow internals — they don't consume `change_type` / `change_subtype` / `change_scope` / `change_channel` directly and are unaffected by Decision 4. (`run_cycle` and `da_cycle` *do* change — see item 7 — because they were the gates that consumed those fields.)
