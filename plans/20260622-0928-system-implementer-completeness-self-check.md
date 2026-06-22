# 2026-06-22 09:28:00 UTC — system-implementer completeness self-check (prevent half-done GREEN)

## TL;DR

**Why:** Rehearsal run #68 ("Apply automatic quantity discount on cart lines") stalled an unattended run because the `system-implementer` agent declared "done" with the feature only half-built — it created an `order_lines` migration but never wired the quantity-discount rule, leaving 3 of 5 acceptance-api tests RED. The verify gate then correctly escalated the still-RED suite to the human-gated `unexpected-failing-tests-fixer`, which has no operator to approve it overnight, so the run yielded.
**End result:** `system-implementer.md` gains a closing self-check step that forces the agent to confirm each acceptance scenario has real production code behind it (and that any schema migration is fully wired) before declaring done — so a future implementer can't ship a migration-only / rule-missing implementation that stalls the pipeline at the human fixer.

## Outcomes

What we get out of this — the goals and deliverables:

- `internal/atdd/assets/runtime/agents/atdd/system-implementer.md` instructs the agent to re-read **each** scenario in `${at-test}` and confirm a concrete production code path satisfies **every** assertion (e.g. discount rate, subtotal, per-line independence) before finishing — not just that the suite compiles.
- The same prompt makes explicit that a schema migration is **incomplete on its own**: whenever the agent adds a migration it must also add the code that **writes** to and **reads** from the new columns, plus the business rule that populates them ("a migration alone greens nothing").
- The class of failure where the implementer leaves the actual business rule unimplemented (or a migration unwired) is caught at the agent, before the verify gate escalates a still-RED suite to the human-gated fixer and stalls an unattended run.
- No change to the BPMN flow, the `gh optivem` commands, or the rehearsal wrapper — they behaved correctly in #68 (the human gate on the fixer is intentional and "never bypassable", and the unattended exit-32 PENDING is expected).

## ▶ Next executable step (resume here)

Edit `internal/atdd/assets/runtime/agents/atdd/system-implementer.md` to append a closing "Step 4" (self-check before done) after the existing Step 3, covering both (a) per-scenario code-path confirmation against `${at-test}` and (b) "a migration must be wired with read + write + rule" guidance. Keep it concise and consistent with the existing numbered-step style; match the prompt's existing voice and the `${placeholder}` references already in use (`${at-test}`, `${system-surface}`, `${channel}`). Stop at a working-tree edit (no rebuild needed — runtime prompts are read live). This is the whole change.

## Steps

- [ ] Step 1: Re-read the current `system-implementer.md` (ends at line 31) to confirm the exact step numbering, voice, and `${placeholder}` set before editing.
- [ ] Step 2: Append a **standalone closing "Step 4"** self-check that requires the agent, before declaring done, to walk **each** scenario in `${at-test}` and name the production code path that satisfies every assertion — flagging any scenario with no real implementation behind it as not-done. Keep the wording **generic** (applies to all features), not specific to the #68 discount rule.
- [ ] Step 3: Within that Step 4, state that a schema migration is incomplete on its own — adding a migration obliges the agent to also wire the **write** path, the **read** path, and the **business rule** that populates the new columns. Use the migration-without-wiring case as a **one-line illustration** of the generic rule, not as the rule itself.
- [ ] Step 4: Sanity-check the edit reads naturally with the existing Steps 1–3 and doesn't restate the shared preamble/scope rules already concatenated in; keep it tight.

## Decisions (settled)

- **Phrasing scope:** generic self-check ("every assertion in every scenario", "wire reads + writes + rule"), with the #68 migration-without-wiring case as a **one-line illustration** — keeps the instruction broadly applicable while making it stick.
- **Placement:** a **standalone closing "Step 4"**, so the "before you finish, verify" intent is unmissable rather than buried in the existing Step 2.
