# 2026-11-09 11:57 UTC — Red: adversarial reviewer agents before human approval

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

Review this plan with the user. After that, Step 1 (agent prompts + shared chunk) can run via `/execute-plan`.

## Steps

- [ ] Step 1: Write the three reviewer prompts under `internal/atdd/assets/runtime/agents/atdd/` and the shared chunk under `runtime/shared/`. Define the reviewer output contract (findings list with blocking/non-blocking) in the agent outputs schema.
- [ ] Step 2: Pin each reviewer's scope in `phase-scopes.yaml`: read the writer's diff plus the use case, write nothing.
- [ ] Step 3: Add the reviewer nodes and `reviewer` param to `execute-agent` in `process-flow.yaml`, with a `max-visits` cap on the findings → writer loop. Audit the new loop-back for statemachine deadlocks before running tests.
- [ ] Step 4: Set `reviewer:` at the three Red call sites (`write-acceptance-tests`, `implement-dsl`, `implement-system-driver-adapters`). Check that the contract-test cascade's DSL dispatch is not affected unless intended.
- [ ] Step 5: Pass the findings and approver role into the `APPROVE_POST` question text.
- [ ] Step 6: Pin the API-before-UI order in `RESOLVE_CHANNEL`.
- [ ] Step 7: Tests: render matrix (monolith + multitier) for the new agents, statemachine tests for the new nodes, and `bpmn-logic-audit` on the change. Run with `-p 2`.

## Verification

- Operator: gift-wrap rehearsal on shop with a planted dropped `.then().shouldSucceed()`. The AT reviewer flags it.

## Open questions

- Fix-loop cap for reviewer findings: 1 or 2 passes before handing over to the human?
- Should non-blocking findings (style) skip the writer loop and only be shown at approval? Recommended: yes.
- Should the reviewer use a cheaper model than the writer? It is a comparison task.
