# Templated intake and label-driven classification

## Motivation

The `intake` process in `internal/atdd/runtime/statemachine/process-flow.yaml:141-211` fans out to four near-identical agents (`atdd-story`, `atdd-bug`, `atdd-task`, `atdd-chore`). Each one does the same shape of work — read the ticket body, extract the canonical sections (Acceptance Criteria, Checklist, Legacy Acceptance Criteria), classify the change. The differences between the four agents are small enough that they could be one prompt, and most of the work isn't actually creative — it's parsing markdown.

Today the agents have to be LLM-driven because ticket bodies are free-form markdown: headings drift ("AC" vs "Acceptance Criteria:" vs "## Acceptance Criteria"), sections appear in arbitrary order, and Given/When/Then blocks are written as prose. An LLM absorbs that noise; a parser would not.

The fix is upstream: enforce ticket structure at creation time with **GitHub Issue Forms** (`.github/ISSUE_TEMPLATE/*.yml`). With forms enforcing the structure:

- The ticket type lands in a label (`type:story` / `type:bug` / `type:task` / `type:chore`) — `auto-classify-ticket` becomes "read the label", no LLM.
- The structural-change classification lands in a single `change_kind` dropdown on `task.yml` (and is hardcoded for `chore.yml`) — `auto-classify-change-kind` becomes "read the dropdown / hardcoded value", no LLM.
- Section headings are guaranteed canonical — intake becomes a deterministic markdown parser, not an agent.
- The four type-specific intake agents collapse to one service task.
- `STOP_INTAKE` shrinks from "human approves scenarios" to "human resolves parse errors" (and disappears for happy-path tickets).

The win cascades: forms remove ambiguity → parser replaces agent → `STOP_INTAKE` shrinks → diagram simpler → less LLM dispatch → faster, cheaper, deterministic.

The pedagogical signal is preserved: students still write Acceptance Criteria as Given/When/Then, still produce a Checklist for structural work, still have a Legacy Acceptance Criteria backfill. They just do it inside a form whose required fields make it impossible to ship a ticket missing the section the cycle needs.

## Decisions

1. **Fallback for non-template tickets: reject.** Tickets without a `type:*` label or with missing/malformed canonical headings fail intake fast. No LLM fallback path, no retained intake agents for "legacy" tickets. Students learn the template path; one explicit rejection message teaches more than a graceful fallback hides. The four intake agents (`atdd-story`, `atdd-bug`, `atdd-task`, `atdd-chore`) are deleted outright in their intake role — see item 7.

   Implication for `STOP_CLASSIFY_CONFLICT`: it stays as the unhappy-path stop, but the resolution is "go fix the ticket (apply a `type:*` label or recreate via the template) and re-run", not "the human supplies the missing classification inline."

   Implication for `STOP_PARSE_ERROR`: same shape — "go fix the ticket body to match the template, then re-run." Not "the human writes the missing scenarios inline."

2. **Implementation order: forms-first.** Scaffold the four issue-form YAMLs (item 1) and submit one test ticket per form against a sandbox repo before any runtime work begins. The `parse_ticket_body` action's contract depends on the exact markdown GitHub emits when an issue form is submitted — observe that against real output rather than fitting forms to a parser written from docs. Items 2–9 only start once the rendered markdown shape is confirmed.

3. **Single `change_kind` enum, no separate channel.** The four orthogonal runtime fields (`change_type`, `change_subtype`, `change_scope`, `change_channel`) collapse to one field, `change_kind`, with four flat values:

   | `change_kind` value | Replaces (old fields) | Cycle |
   |---|---|---|
   | `system-api-redesign` | `structure` + `interface` + `system` + `api` | `SYSAPI_CYCLE` |
   | `system-ui-redesign` | `structure` + `interface` + `system` + `ui` | `SYSUI_CYCLE` |
   | `external-system-interface-redesign` | `structure` + `interface` + `external_system` | `ct_subprocess` (via `EXTAPI_CYCLE`) |
   | `system-implementation-change` | `structure` + `implementation` + `system` (chore) | `sut_cycle` |

   Behavioral tickets (story / bug) **do not get a `change_kind`** — `run_cycle` dispatches them via `ticket_type` straight into `at_cycle`. `change_kind` is only meaningful for structural tickets (task / chore).

   Why one field, not two coupled dropdowns: GitHub issue forms can't conditionally show a channel field based on another dropdown. A two-dropdown design would need an "n/a" option for `external-system-interface-redesign` plus parser-side validation that "n/a" only pairs with the external value. Folding the channel into the value name removes that coupling — the form has *more values* in the case where channel matters and *no separate channel field at all*. Side benefit: `da_cycle` collapses two nested gates (`change_scope` × `change_channel`) into one (`change_kind`).

   Why values describe the *work* not the *ticket type* (no `-task` / `-chore` suffix): the ticket type already lives in `ticket_type` (set by the form label). `change_kind` exists to describe *what kind of change*. Suffixing values with the container's name (`-task`) duplicates a field we already have; suffixing with the work (`-redesign`, `-change`) tells you something the label doesn't.

## Items

Sequence: forms first (so the parser has something concrete to target), then runtime changes, then retire the old agents. One PR per item.

### 1. Author the four issue forms in shop

**Files:**
- `shop/.github/ISSUE_TEMPLATE/story.yml` (new)
- `shop/.github/ISSUE_TEMPLATE/bug.yml` (new)
- `shop/.github/ISSUE_TEMPLATE/task.yml` (new)
- `shop/.github/ISSUE_TEMPLATE/chore.yml` (new)
- `shop/.github/ISSUE_TEMPLATE/config.yml` (new — `blank_issues_enabled: false`)

**Form shape (common to all four):**

| Field | Type | Required | Notes |
|---|---|---|---|
| `Context` | textarea | yes | Why the ticket exists. One paragraph. |
| `Legacy Acceptance Criteria` | textarea | no | Backfill section. Empty when absent. |
| `Links` | input | no | Related issues / PRs / docs. |

**Type-specific fields:**

- `story.yml`, `bug.yml`: add `Acceptance Criteria` textarea (required, Given/When/Then placeholder, `render: markdown`). `bug.yml` also adds `Steps to Reproduce` (textarea, required). No `change_kind` — behavioral tickets go straight to `at_cycle`.
- `task.yml`: add `Checklist` textarea (required, `- [ ]` placeholder, `render: markdown`); add `change_kind` dropdown (required) with three values:
  - `system-api-redesign`
  - `system-ui-redesign`
  - `external-system-interface-redesign`
- `chore.yml`: add `Checklist` textarea (required). No `change_kind` dropdown — chore is always hardcoded to `system-implementation-change` at classify-time (item 5).

**Labels:**
- Each form sets `labels: ["type:story"]` (etc.) so `auto-classify-ticket` can read the label.

**Validation (manual, before runtime work):** create one test ticket per form against a sandbox repo, dump the rendered markdown body, confirm headings are exactly `## Acceptance Criteria`, `## Legacy Acceptance Criteria`, `## Checklist`, etc. The parser's contract depends on this.

### 2. Wire the forms into the scaffolder

**Why this item exists at all:** today's scaffolder (`internal/steps/apply_template.go:128`) doesn't bulk-copy `shop/`. It explicitly copies named subdirectories: workflows via `CopyWorkflows`, system code via `files.CopyDir`, externals, system-tests, docker, cloud-run scripts, docs via `copyDocs`. Dropping `.github/ISSUE_TEMPLATE/*.yml` into shop without a corresponding scaffolder step means the YAMLs sit unused — the new repo never receives them.

**Scope:** five-line addition. No fixups, no content replacements, no per-language variation. Issue forms are fully scaffold-agnostic — same files in every student repo.

**Files:**
- `internal/steps/apply_template.go` — add a `copyIssueTemplates(shop, repoDir)` helper alongside `copyDocs` / `copyExternals`, and call it once from `ApplyTemplate` before the arch-specific dispatch.

```go
func copyIssueTemplates(shop, repoDir string) {
    src := filepath.Join(shop, ".github", "ISSUE_TEMPLATE")
    if _, err := os.Stat(src); err == nil {
        files.CopyDir(src, filepath.Join(repoDir, ".github", "ISSUE_TEMPLATE"))
    }
}
```

Optionally extend `ValidateNoLeftoverTemplateRefs` (`apply_template.go:768`) to confirm the four expected `*.yml` files landed — but issue forms have no templated content, so a missing-file check is the only meaningful post-condition.

### 3. Hardcode section-heading constants

**Files:**
- `internal/atdd/runtime/intake/sections.go` (new — package may need creating)

```go
package intake

const (
    SectionAcceptanceCriteria       = "Acceptance Criteria"
    SectionLegacyAcceptanceCriteria = "Legacy Acceptance Criteria"
    SectionChecklist                = "Checklist"
    SectionStepsToReproduce         = "Steps to Reproduce"
    SectionContext                  = "Context"
    SectionLinks                    = "Links"
)
```

Single source of truth for headings. Not configurable (see motivation in the prior conversation — pedagogical signal lives in the canonical names).

### 4. Replace `auto-classify-ticket` with a label reader

**Files:**
- `internal/atdd/runtime/actions/bindings.go`
- `internal/atdd/runtime/actions/bindings_test.go`
- `internal/atdd/runtime/statemachine/process-flow.yaml`

Today's `classify_ticket` action presumably runs an LLM (or heuristic) over the ticket body. Replace with:

```go
func (a *Actions) ClassifyTicket(ctx context.Context) error {
    labels := a.Board.IssueLabels(ctx)
    ticketType := pickTypeLabel(labels) // "type:story" → "story"
    if ticketType == "" {
        return ErrTicketTypeUnknown // forces STOP_CLASSIFY_CONFLICT
    }
    ctx.Set("ticket_type", ticketType)
    ctx.Set("classify_confident", true)
    return nil
}
```

`STOP_CLASSIFY_CONFLICT` stays in the YAML as the unhappy-path stop — it now triggers when the ticket has no `type:*` label, not when the LLM was uncertain.

### 5. Add `auto-classify-change-kind` service task

**Files:**
- `internal/atdd/runtime/actions/bindings.go`
- `internal/atdd/runtime/actions/bindings_test.go`
- `internal/atdd/runtime/statemachine/process-flow.yaml`

New action `ClassifyChangeKind` runs after `ClassifyTicket`. Sets a single field, `change_kind`, based on `ticket_type` plus (for `task` only) the form's `change_kind` dropdown:

| `ticket_type` | `change_kind` set to | Source |
|---|---|---|
| story, bug | (unset — behavioral) | — |
| task | one of `system-api-redesign` / `system-ui-redesign` / `external-system-interface-redesign` | form dropdown rendered into the issue body (e.g. `### Change kind\n\nsystem-api-redesign`) |
| chore | `system-implementation-change` | hardcoded |

The action reads the issue body, locates the `### Change kind` section rendered by the issue-form `change_kind` dropdown, and writes the value to context. For `chore` it skips the body read and hardcodes the value. For `story` / `bug` it skips entirely — `change_kind` is undefined for behavioral tickets and `run_cycle`'s top gate (`ticket_type`) routes them away from the `change_kind` gate before that field is ever read.

The runtime fields `change_type`, `change_subtype`, `change_scope`, `change_channel` are **removed** from the context in this same change. Any binding or gate that reads them migrates to `change_kind` (item 7 covers the gate updates).

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
4. For structural tickets (task, chore): produces a list of `ChecklistItem` structs from the Checklist section.
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
    - id: CLASSIFY_CHANGE_KIND
      type: service_task
      action: classify_change_kind
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
    - {from: GATE_CLASSIFY_CONFIDENT, to: CLASSIFY_CHANGE_KIND,  when: "classify_confident == true"}
    - {from: GATE_CLASSIFY_CONFIDENT, to: STOP_CLASSIFY_CONFLICT, when: "classify_confident == false"}
    - {from: STOP_CLASSIFY_CONFLICT,  to: CLASSIFY_CHANGE_KIND}
    - {from: CLASSIFY_CHANGE_KIND,    to: PARSE_BODY}
    - {from: PARSE_BODY,              to: GATE_PARSE_OK}
    - {from: GATE_PARSE_OK,           to: INTAKE_END,            when: "parse_ok == true"}
    - {from: GATE_PARSE_OK,           to: STOP_PARSE_ERROR,      when: "parse_ok == false"}
    - {from: STOP_PARSE_ERROR,        to: PARSE_BODY}
```

Note: `GATE_TICKET_TYPE` and the four `ATDD_*` user_tasks disappear. `STOP_INTAKE` ("approve scenarios") disappears — humans no longer approve agent output because there is no agent.

### 7. Update `run_cycle` and `da_cycle` to gate on `change_kind`

**Files:**
- `internal/atdd/runtime/statemachine/process-flow.yaml`
- `internal/atdd/runtime/gates/bindings.go`
- `internal/atdd/runtime/gates/bindings_test.go`
- `internal/atdd/runtime/gates/registry.go`

Today `run_cycle` (`process-flow.yaml:254-289`) gates on `change_type` then `change_subtype`, and `da_cycle` (`process-flow.yaml:648-693`) gates on `change_scope` then `change_channel`. With the flat `change_kind` enum (Decision 3) those four nested gates collapse to one gate per cycle.

**`run_cycle` rewrite:**

```yaml
run_cycle:
  start: GATE_TICKET_TYPE
  nodes:
    - id: GATE_TICKET_TYPE
      type: gateway
      binding: ticket_type
      description: "Behavioral or structural?"
    - id: GATE_CHANGE_KIND
      type: gateway
      binding: change_kind
      description: "Cycle by change kind"
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
    - {from: GATE_TICKET_TYPE, to: GATE_CHANGE_KIND, when: "ticket_type in [task, chore]"}
    - {from: GATE_CHANGE_KIND, to: DA_CYCLE,         when: "change_kind in [system-api-redesign, system-ui-redesign, external-system-interface-redesign]"}
    - {from: GATE_CHANGE_KIND, to: SUT_CYCLE,        when: "change_kind == system-implementation-change"}
    - {from: AT_CYCLE,  to: CYCLE_END}
    - {from: DA_CYCLE,  to: CYCLE_END}
    - {from: SUT_CYCLE, to: CYCLE_END}
```

**`da_cycle` rewrite:**

```yaml
da_cycle:
  start: GATE_CHANGE_KIND
  nodes:
    - id: GATE_CHANGE_KIND
      type: gateway
      binding: change_kind
      description: "Driver Adapter target?"
    - id: SYSAPI_CYCLE
      type: call_activity
      flow: structural_cycle
      params:
        phase: "SYSTEM API REDESIGN"
        agent: atdd-task
        phase_doc: docs/atdd/process/sysapi-redesign.md
        subtype: system-api-redesign
    - id: SYSUI_CYCLE
      type: call_activity
      flow: structural_cycle
      params:
        phase: "SYSTEM UI REDESIGN"
        agent: atdd-task
        phase_doc: docs/atdd/process/sysui-redesign.md
        subtype: system-ui-redesign
    - id: EXTAPI_CYCLE
      type: call_activity
      flow: ct_subprocess
    - id: DA_END
      type: end_event
  sequence_flows:
    - {from: GATE_CHANGE_KIND, to: SYSAPI_CYCLE, when: "change_kind == system-api-redesign"}
    - {from: GATE_CHANGE_KIND, to: SYSUI_CYCLE,  when: "change_kind == system-ui-redesign"}
    - {from: GATE_CHANGE_KIND, to: EXTAPI_CYCLE, when: "change_kind == external-system-interface-redesign"}
    - {from: SYSAPI_CYCLE, to: DA_END}
    - {from: SYSUI_CYCLE,  to: DA_END}
    - {from: EXTAPI_CYCLE, to: DA_END}
```

**Gate-binding changes (`internal/atdd/runtime/gates/bindings.go`):**
- Add: `ChangeKind(ctx) string` — returns the value set by `ClassifyChangeKind` (item 5).
- Remove: `ChangeType`, `ChangeSubtype`, `ChangeScope`, `ChangeChannel` and their registry entries.
- Keep: `TicketType` (already exists; now consumed by `run_cycle` directly instead of solely by intake).

### 8. Retire the four intake agents

**Files (delete):**
- `docs/atdd/process/intake-story.md`
- `docs/atdd/process/intake-bug.md`
- `docs/atdd/process/intake-task.md`
- `docs/atdd/process/intake-chore.md`
- Any embedded agent prompt files for `atdd-story`, `atdd-bug`, `atdd-task`, `atdd-chore` under `internal/atdd/runtime/agents/`

**Files (update):**
- `internal/atdd/runtime/agents/registry.go` — remove the four agent registrations
- `internal/atdd/runtime/agents/embed.go` — remove the four embed directives

Cross-check: search for any remaining references to these agent names in `da_cycle` etc. — `da_cycle` reuses `atdd-task` for `SYSAPI_CYCLE` and `SYSUI_CYCLE` (`process-flow.yaml:666, 675`). That's *not* the intake agent — it's a structural-cycle WRITE agent that happens to share the name. Confirm whether they're the same agent in `internal/atdd/runtime/agents/registry.go` before deleting; if so, keep the agent and only retire its intake role. If they're distinct, the rename ambiguity is itself a smell — fix during this work.

### 9. Update tests, diagrams, docs

**Files:**
- `internal/atdd/runtime/statemachine/transitions_test.go`
- `internal/atdd/runtime/statemachine/structural_cycle_test.go`
- `internal/atdd/runtime/diagram/diagram.go` (if intake fan-out has special-case rendering)
- `internal/atdd/runtime/diagram/diagram_test.go`
- `docs/process-diagram.md` — regenerate
- `docs/images/process-diagram-*-intake-cycle.svg`, `*-da-cycle.svg`, `*-run-cycle.svg` — regenerate (intake fan-out collapses; `da_cycle` and `run_cycle` lose their inner gates)
- `CLAUDE.md` / agent-facing docs that reference the four intake agents
- `internal/atdd/runtime/statemachine/process-flow.yaml` header comment (`process-flow.yaml:248-253`) — update the prose describing `change_type` / `change_subtype` / `change_scope` / `change_channel` to describe the single `change_kind` field instead

Run `gh optivem atdd regen-diagrams` (or whatever the regen entry point is — see `internal/atdd/runtime/diagram/`) and commit the updated SVGs.

### 10. Smoke test against real GitHub

**Files:** none (manual).

After items 1–9 land:
1. `gh optivem init` a fresh sandbox repo → confirm forms appear in `.github/ISSUE_TEMPLATE/`.
2. Open each of the four forms in the GitHub UI, fill in, submit → confirm `type:*` label is applied and rendered body matches what the parser expects (especially the `### Change kind` section on the task form).
3. Run `gh optivem atdd implement-ticket --issue N` against each ticket type → confirm intake passes through `parse_ticket_body` to `RUN_LEGACY_CYCLE` / `RUN_CYCLE` without dispatching any LLM agent, and that `run_cycle` / `da_cycle` route to the correct sub-cycle for each `change_kind` value.
4. Create one ticket *outside* the template (raw markdown, no `type:*` label) → confirm `STOP_CLASSIFY_CONFLICT` fires with a clear message.

## Out of scope

- Migrating existing tickets in production student repos — the new path applies to newly-created tickets; legacy tickets keep their current handling until manually re-classified.
- A YAML linter for issue forms (overkill — five small files, manually verified).
- Repurposing the deleted intake agents for other phases — if there's a reuse opportunity, capture it in a separate plan.
- Any change to `at_cycle`, `sut_cycle`, `ct_subprocess`, `external_system_onboarding`, `structural_cycle`, or `legacy_acceptance_criteria` flow internals — they don't consume `change_type` / `change_subtype` / `change_scope` / `change_channel` directly and are unaffected by Decision 3. (`run_cycle` and `da_cycle` *do* change — see item 7 — because they were the gates that consumed those fields.)
