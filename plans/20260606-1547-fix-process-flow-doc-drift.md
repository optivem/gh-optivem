# Plan: Fix doc-block drift in process-flow.yaml (node-type enum + stale "disable" comments)

> **DECISION MADE (2026-06-06):** the header "Document shape" enum and two inline comments have
> drifted from the encoding; correct them. No behavior change — comments only. Settled in review
> discussion.
>
> Review finding §4. Independent plan from the `process-flow.yaml` review; no dependency on the other
> review plans.

## Why

Two unrelated-but-adjacent documentation defects in `process-flow.yaml`, both comment-only:

**§4a — header node-type enum + field list is incomplete.** The "Document shape" block (~`:131`)
lists node types as `start-event, end-event, service-task, user-task, gateway, call-activity`, but the
parser `parseKind` (`load.go:234-248`) also accepts **`error-end-event`** — which the YAML uses 15+
times (every `UNKNOWN_*`, `STOP_*`, `*_FIX_EXHAUSTED`, `*_REJECTED_END`, infra-halt). The field list
also omits node-level fields the parser carries (`load.go` RawNode): `tdd-stage` (enum
red/green/refactor), `read` / `write` / `scope` / `scope-rationale`, and `max-visits` /
`on-max-visits`. A reader can't reconstruct the real schema from the header.

**§4b — stale "disable" references.** The disable/enable steps were replaced by the permanent env-var
gate the orchestrator lifts at verify time (correctly explained at `:816` and `:1254` — leave those).
Two comments still imply a disable step exists:
- `:689` — "every command-runner child (compile/verify/**disable**/commit)" — there is no disable
  child; the command-runner children are compile / verify / commit.
- `:2107` — "**disabler**/writer agents create new files" — there is no disabler agent.

## Items

1. **`process-flow.yaml` header enum (§4a)** — in the "Document shape" comment (~`:131`), add
   `error-end-event` to the node-type list, and document the node-level fields the parser accepts:
   `tdd-stage` (red / green / refactor), `read` / `write` / `scope` / `scope-rationale` (on
   writing-agent call-activities), and `max-visits` + `on-max-visits` (the visit-cap pair). Source of
   truth is `load.go` (`parseKind` + the RawNode struct) — match it exactly. Keep it tight per
   `[[feedback_flag_non_token_efficient]]`.
2. **`process-flow.yaml:689` (§4b)** — drop `disable/` from the parenthetical so it reads
   "(compile/verify/commit)".
3. **`process-flow.yaml:2107` (§4b)** — change "disabler/writer agents" to "writer agents" (the
   `--include-untracked` rationale still holds: writer agents create new files).

## Verification

- The header "Document shape" enum + field list matches `load.go`'s `parseKind` and RawNode exactly.
- No remaining `disable`/`disabler` comment implies a disable *step* exists (the two surviving
  mentions at `:816` / `:1254` correctly state the step was removed).
