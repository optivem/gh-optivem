# Templated intake and form-driven classification

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

- [ ] **Cross-repo follow-up: shop-side phase doc** — ⏳ Deferred: create `shop/docs/atdd/process/system-interface-redesign.md` (consolidating the SYSTEM API REDESIGN / SYSTEM UI REDESIGN sections of `task-and-chore-cycles.md` into one channel-agnostic phase doc) and remove the now-stale separate sections. The runtime YAML param `phase_doc: docs/atdd/process/system-interface-redesign.md` already references it, but no test in gh-optivem fails on its absence — the WRITE agent gracefully falls back to the ticket body. Also update `task-and-chore-cycles.md`'s "Purpose" header and the rest of the file to drop the old `system-api-task` / `system-ui-task` / `external-api-task` / `chore` ticket-type names. Crosses repo boundary; better as its own PR in the `shop` template.

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
