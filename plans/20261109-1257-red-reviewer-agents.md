# 2026-11-09 11:57 UTC — Red: adversarial reviewer agents before human approval
> 🤖 **Picked up by agent** — `Valentina_Desk` at `2026-11-09T14:25:23Z`

## TL;DR

**Why:** In Red, the AT writer, DSL implementer and system-driver-adapter implementer produce the spec, and today a human approves their output directly. Nothing cross-checks the work first. The executor/reviewer/approver article: a skipped line or a wrong value becomes the spec, and with hundreds of criteria a human won't catch it. Driver design (e.g. `POST /api/order/deliver?id=` instead of `/api/orders/{id}/deliver`) is also invisible to the tests.
**End result:** After each Red writer, a matching reviewer agent checks the output adversarially. Blocking findings go back to the writer (capped). The human approval shows the findings and names the approver role.

Parent: `plans/20261109-1219-improved-atdd-process-flow.md` ("Execute Agent" diagram). Decided there:
- Q2: three reviewers, sharing one chunk.
- Q3: approvals stay.
- The commit order already matches the talk; only the channel order is pinned here.

## Outcomes

- Three new read-only agents, each paired with one writer:
  - `acceptance-test-reviewer`: every Given/When/Then has a line, every value matches, the tag is right, nothing extra.
  - `dsl-reviewer`: business vocabulary matches the use case.
  - `system-driver-adapter-reviewer`: one port method per use case with request/response, consistent REST design.
- One shared chunk `runtime/shared/adversarial-review.md` holds the common stance: compare against the spec, try to find what's wrong, change nothing.
- `execute-agent` gains an optional `reviewer` param and the nodes `GATE_REVIEWER_CONFIGURED → RUN_REVIEWER → GATE_REVIEW_FINDINGS → (capped) RUN_AGENT | APPROVE_POST`. Every call site without the param behaves as today.
- `APPROVE_POST` shows the reviewer findings and the approver role: "QA and/or Developers" for the AT, "Developers" for DSL and drivers.
- `RESOLVE_CHANNEL` orders channels API before UI.

## ▶ Next executable step (resume here)

Step 6: pin the API-before-UI channel order. Steps 1–5 are done: the reviewer agents and MIDs exist, `execute-agent` has the reviewer loop (`GATE_REVIEWER_CONFIGURED → RUN_REVIEWER → GATE_REVIEW_FINDINGS → REQUEST_REVISION (max-visits 2) → RUN_AGENT | SUMMARIZE_REVIEW → APPROVE_POST_REVIEWED`), the three Red writer MIDs pass `reviewer` + `approver-role` (the DSL reviewer only on the acceptance-test path, via `dsl-reviewer`), and blocking findings reach the writer's prompt as `${review-findings-section}`.

## Steps

- [ ] Step 6: Pin the API-before-UI order in `RESOLVE_CHANNEL`.
- [ ] Step 7: Tests: render matrix (monolith + multitier) for the new agents, statemachine tests for the new nodes, and `bpmn-logic-audit` on the change. Run with `-p 2`.

## Verification

- Operator: gift-wrap rehearsal on shop with a planted dropped `.then().shouldSucceed()`. The AT reviewer flags it.

## Decisions (proposed 2026-11-09, pending user OK — override any line)

- **Fix-loop cap: 2 passes**, then hand over to the human approval with the remaining findings attached. One pass often only half-fixes a finding; more than two means writer and reviewer disagree, which is a human call anyway.
- **Non-blocking findings (style) skip the writer loop** and are only shown at approval. Only blocking findings (wrong behaviour asserted, criterion not covered, test passes for the wrong reason) re-dispatch the writer.
- **Reviewer uses the same model as the writer.** Acceptance-test accuracy beats agent cost (see memory `feedback_acceptance_tests_accuracy_over_speed`); an adversarial reviewer that is weaker than the writer misses exactly the subtle errors it exists for. Revisit after rehearsals show the token cost.
