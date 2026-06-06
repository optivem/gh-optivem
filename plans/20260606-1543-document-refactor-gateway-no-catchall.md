# Plan: Document why the refactor gateway needs no unknown catch-all

> **DECISION MADE (2026-06-06):** `GATE_REFACTOR_TYPE_CHOICE` deliberately has no unknown catch-all
> because its binding is a constrained re-prompting menu that provably emits only the 5 enumerated
> values — so the fix is a documenting comment, not an unreachable error-end-event. Settled in review
> discussion.
>
> Review finding §3b. Independent plan from the `process-flow.yaml` review; no dependency on the other
> review plans.

## Why

`GATE_REFACTOR_TYPE_CHOICE` (`process-flow.yaml:357-391`) enumerates 5 `refactor-type-choice` values
(4 refactor/redesign cycles + `none`) with **no unguarded catch-all edge**, while every other
enumerated gateway in the file routes unrecognised values to an `error-end-event`
(`UNKNOWN_TICKET_KIND`, `UNKNOWN_TASK_SUBTYPE`, `UNKNOWN_EXPECTED_TEST_RESULT`,
`UNKNOWN_TESTS_OUTCOME`). On first read this looks like a missing-branch gap.

It is not a gap — it is a deliberate, undocumented asymmetry. The `refactorTypeChoice` binding
(`gates/bindings.go:416-441`) is a **constrained menu, not a parser**: `promptio.SelectOneOfVia`
re-prompts on any unrecognised reply (`bindings.go:414-415`; test `bindings_test.go:594` feeds
`"refactor-the-world"` and confirms the re-prompt loop), `--auto` defaults to `none`, and a preseeded
`ctx.State` value (test/hand-debug only) short-circuits. So the binding **provably emits only the 5
enumerated values** in any real run. A catch-all here would be unreachable in production — dead
machinery, and a phantom error path on the rendered diagram, against
`[[feedback_schema_fields_earn_slot]]` and the teaching-repo no-dead-machinery ethos.

Contrast: `ticket-kind` / `task-subtype` are parsed from free-form ticket labels (out-of-set values
are realistic), and `expected-test-result` / `test-outcome` guard a derivation bug — so *their*
catch-alls fire on real inputs. The real defect for the refactor gate is only that the asymmetry is
unexplained.

## Items

1. **`internal/atdd/runtime/statemachine/process-flow.yaml`** — add a short comment on
   `GATE_REFACTOR_TYPE_CHOICE` (in the `refactor` process, ~line 357) stating that it carries no
   unknown catch-all *by design*: the `refactor-type-choice` binding is a re-prompting menu that
   defaults to `none` under `--auto`, so it only ever emits the 5 enumerated values — unlike the
   parse-derived `ticket-kind` / `task-subtype` gateways, whose catch-alls guard real unrecognised
   inputs. Keep it tight per `[[feedback_flag_non_token_efficient]]`.

2. **`.claude/agents/atdd/meta/bpmn-logic-audit.md`** — tighten Lens 2 so it does not false-positive on
   this case. Reword the catch-all rule from "cover every value **or** carry a catch-all" to: an
   enumerated gate needs a catch-all only when its **binding can emit a value outside the enumerated
   set** (parse-derived / state-derived bindings) — a constrained re-prompting menu binding that can
   only emit the enumerated set (e.g. `refactor-type-choice`) does not. When unsure whether a binding
   is constrained, the agent reads the binding in `gates/bindings.go` before flagging.

## Verification

- Re-reading the `refactor` process, a maintainer can tell at a glance that the missing catch-all is
  intentional and why.
- A subsequent `bpmn-logic-audit` run does not raise `GATE_REFACTOR_TYPE_CHOICE` as a missing-branch
  finding.
