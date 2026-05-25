## `gh optivem implement` — pre-flight refinement check + post-run refactor offer

**Date:** 2026-05-25
**Status:** idea — not yet refined. Run `/refine-plan` against this file before `/execute-plan`.
**Trigger:** observation while walking ATDD plans — `gh optivem implement --issue <n>` currently jumps straight into the WRITE/RED/GREEN cycle without checking whether the ticket has been through `backlog_refinement` first, and exits without prompting whether the user wants to follow up with a refactor pass. Both gaps cause friction: implementing un-refined tickets produces noisier cycles, and refactor opportunities surfaced during implementation are forgotten once the command exits.

## Cross-references

- `internal/atdd/runtime/statemachine/process-flow.yaml:288` — existing `backlog_refinement` sub-process, gated by `refine_requested` (currently set by the operator, not derived from ticket state).
- `internal/atdd/runtime/statemachine/process-flow.yaml:554` — existing `at_refactor_system` post-GREEN housekeeping refactor (embedded in every cycle, no-op if nothing to improve). The post-run *offer* described here is separable: a bigger, opt-in refactor pass at the ticket level, not the per-cycle housekeeping that already runs.
- `internal/atdd/runtime/statemachine/process-flow.yaml:1316` — `system-implementation-refactoring` change_type, which is the kind of ticket the post-run offer could spin up.
- `scripts/atdd-rehearsal.sh:240` — the harness that drives `gh optivem implement --issue <n>`.

## Idea 1 — pre-flight: check refinement before implementing

When `gh optivem implement --issue <n>` starts, before entering the BPMN cycle:

1. Determine whether the ticket has been refined. Open question (for `/refine-plan`): what is the source-of-truth signal?
    - A persisted marker on the issue itself (label, comment, frontmatter field)?
    - The presence of `update-ticket`-shaped AC sections in the ticket body?
    - A local `refinement_state` artifact written by `backlog_refinement`?
    - A timestamp comparison (ticket last modified vs. last refine run)?
2. If refined → proceed normally.
3. If **not** refined → fast-exit with a message naming the exact command the user should run, e.g.:

    ```
    Ticket #42 has not been through backlog refinement.
    Run:    gh optivem refine --issue 42
    Then:   gh optivem implement --issue 42
    ```

    Do not silently auto-invoke refine — the operator decides whether refinement is needed. Provide a `--skip-refine-check` escape hatch only if a real use case surfaces (default: no flag; if the operator wants to skip, they re-run with the marker set).

## Idea 2 — post-run: offer a refactor follow-up

When `gh optivem implement --issue <n>` exits successfully (cycle reached the terminal COMMIT, not error-exit):

1. Print a final block that surfaces the refactor command, e.g.:

    ```
    Implement complete for #42.
    Consider a refactor pass:
        gh optivem refactor --issue 42        # opens a refactor ticket linked to #42
    ```

2. The offer is informational — no interactive prompt, no auto-spawn. The user can copy-paste the command or ignore it. (Aligns with [[feedback_always_online_no_offline_flags]]-style philosophy of not adding interactive escape hatches without need, and with [[feedback_execute_plan_always_next_steps]]-style philosophy of always ending with a clear next-steps block.)

3. Open question (for `/refine-plan`): what does `gh optivem refactor --issue` actually do?
    - Create a new `system-implementation-refactoring` change_type ticket linked to the original?
    - Re-enter the BPMN cycle on the same ticket but force `change_type = system-implementation-refactoring`?
    - Just run a code-quality / dead-code / duplication scan and report?
    - This may be a separate plan entirely — Idea 2 may just be "surface the command" while the command itself is designed elsewhere.

## Items

1. - [ ] **Refine this plan.** Run `/refine-plan plans/upcoming/20260525-1753-implement-pre-refine-check-and-post-refactor-offer.md` to resolve the open questions above (refinement-state source-of-truth; what `gh optivem refactor` does; whether Idea 2 needs to be its own plan).

2. - [ ] **Design + land Idea 1** (pre-flight refinement check). Wire the check into the `implement` command entrypoint. Add tests for: refined-ticket path (proceeds), un-refined-ticket path (exits with command-naming message), missing-ticket path (existing error). Update `--help` Long text via the [help-text-updater agent](.claude/agents/...).

3. - [ ] **Design + land Idea 2** (post-run refactor offer). Print the refactor-command block on successful exit. If Idea 2 expands beyond "print a line" (i.e., a new `gh optivem refactor` subcommand is required), split it into its own plan per [[feedback_new_plan_not_extend]].

4. - [ ] **Update `CONTRIBUTING.md`** (and any ATDD process docs under `docs/atdd/process/`) to describe the refine-before-implement expectation and the refactor-after-implement convention. The pre-flight check is the *enforcement* of the doctrine; the docs are the *teaching* of it.

## Out of scope

- Changing `backlog_refinement` itself. This plan adds a gate around `implement`, not a redesign of the refinement sub-process.
- Auto-running refine or refactor. Both stay operator-driven — we surface the command, we don't invoke it.
- A persistent ticket-state database. The refinement-state signal should reuse whatever the existing BPMN flow already writes; this plan does not introduce a new storage layer.
