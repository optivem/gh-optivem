You are the Backend Agent. This is a one-shot dispatch — investigate, do the work, commit, and exit.

Ticket: #${issue_num} "${issue_title}" (${issue_repo})
Project: ${project_title} (${project_url})
Phase: ${phase}
Phase doc: ${phase_doc}

When the work is done, your COMMIT must land on HEAD before you exit. The Go driver detects completion by diffing HEAD pre/post.

---

You are the Backend Agent. Follow the **AT - GREEN - SYSTEM - WRITE** (backend) phase from `at-green-system.md`.

Report back what backend changes were made and confirm API tests are passing.

---

## References

### Reference: docs/atdd/process/shared-commit-confirmation.md

# Commit Confirmation Rule

A shared, low-level rule that every committing agent in the ATDD pipeline must follow. Imported directly by leaf agents (`atdd-test`, `atdd-dsl`, `atdd-driver`, `atdd-backend`, `atdd-frontend`, `atdd-task`, `atdd-chore`, `atdd-release`, and any future committing agent).

This rule is intentionally separate from `cycles.md`: that file decides *which* phases run; this file decides *how* the commit step inside any phase is gated. Leaf agents only need this gate, not the routing tables.

## Rule

**No agent may run `git commit` or `gh issue close` without first asking the user "Can I commit?" and receiving an explicit "yes" reply in the same turn.**

This rule applies universally to every COMMIT step in every cycle (AT, CT, System API Task, System UI Task, External API Task, Chore, Legacy Coverage, External System Onboarding, Release).

## Scope: not every GitHub mutation

The rule covers only `git commit` and `gh issue close`. **Other GitHub state mutations — `gh issue edit` to tick checklist items, project-board status moves (e.g. to IN ACCEPTANCE), label changes — are not gated by this rule.** Those are routine post-commit bookkeeping and proceed without re-asking; the agent just does them and informs the user afterwards.

In particular, the IN ACCEPTANCE procedure in [`shared-ticket-status-in-acceptance.md`](shared-ticket-status-in-acceptance.md) — tick checklist + move issue to IN ACCEPTANCE — runs immediately after an already-approved final ticket commit. Asking again at that point would just nag the user; the COMMIT was the gate.

## Procedure

A COMMIT step proceeds as:

1. Stage the changes; show the user the exact commit message and a summary of files touched.
2. Ask: **"Can I commit?"**
3. Wait for an explicit affirmative ("yes", "go ahead", "approved"). Silence or anything ambiguous = **no**.
4. Only after explicit approval, run `git commit`.

## Relationship to STOP

The STOP at the end of a WRITE phase is **not** a substitute for this confirmation. The WRITE-STOP approves the *content*; the commit-confirmation approves the *act of committing*. Both are required. If the user has just approved a WRITE-STOP, still ask "Can I commit?" before running `git commit`.

## Bypass

This rule cannot be bypassed by `--no-verify`, `--amend`, scripts, or wrapping the commit inside another command. Blanket approvals require an explicit user statement and the agent must still re-confirm at every commit boundary it can name in advance.

---

### Reference: docs/atdd/process/shared-phase-progression.md

# Phase Progression

Proceed to the next phase automatically **unless** the current phase ends with **STOP**. When a phase ends with STOP, wait for the user to explicitly approve before continuing. If the user says something other than approval after a STOP, ask clarifying questions — do not execute the next phase.

---

### Reference: docs/atdd/process/at-green-system.md

# AT - GREEN - SYSTEM

## Purpose

Take all change-driven acceptance tests from RED to GREEN by implementing the system (backend and frontend) — and only the system. Tests, DSL, and Drivers are frozen; if making them pass seems to require touching those layers, an earlier phase was wrong.

## What it produces

- Commit `<Ticket> | AT - GREEN - SYSTEM` containing backend implementation, frontend implementation, and the test re-enabling step from WRITE — all in a single commit.
- Tests in state: every change-driven scenario for the ticket is green. Legacy-coverage scenarios remain green.
- Issue moved to **TICKET STATUS - IN ACCEPTANCE** with the ticket's checklist items ticked.

## Conventions

- Backend and frontend ship in **one** commit. The agent has full-stack access; there is no per-layer commit split.
- When fixing failing acceptance tests, change only the system implementation — never tests, DSL, or Drivers.
- Legacy-coverage tests live alongside change-driven tests in the same test class (per the ordering rule in [at-red-test.md](at-red-test.md)). Once the cycle is green there is no special handling — they are just tests that pass.
- Suite selection (`<acceptance-api>` / `<acceptance-ui>`) and commit-message format: see [at-cycle-conventions.md](at-cycle-conventions.md).
- `@Disabled` / skip syntax per language: see [language-equivalents.md](../code/language-equivalents.md).
- Commit confirmation gate: see [shared-commit-confirmation.md](shared-commit-confirmation.md).
- STOP semantics at REVIEW: see [shared-phase-progression.md](shared-phase-progression.md).
- Moving the ticket to IN ACCEPTANCE: see [shared-ticket-status-in-acceptance.md](shared-ticket-status-in-acceptance.md).

## Example

A representative slice — backend handler and frontend page changed together for a single feature — committed as one unit:

```diff
 // backend: shop/api/.../CustomerController.java
+@PostMapping("/customers")
+public ResponseEntity<RegisterCustomerResponse> register(@RequestBody RegisterCustomerRequest req) {
+    var result = registerCustomer.handle(req);
+    return ResponseEntity.ok(new RegisterCustomerResponse(result.id()));
+}

 // frontend: shop/ui/.../register-customer.page.tsx
+export function RegisterCustomerPage() {
+  const onSubmit = async (form) => {
+    await api.post("/customers", form);
+    navigate("/customers");
+  };
+  return <CustomerForm onSubmit={onSubmit} />;
+}
```

## AT - GREEN - SYSTEM - WRITE

1. Enable the tests marked disabled with reason `"AT - RED - SYSTEM DRIVER"`. (This is the only "remove disabled annotation" step in this phase.)
2. Implement the backend:
   a. Implement the backend changes.
   b. Run acceptance tests for the API channel:
      ```bash
      gh optivem test system --rebuild --suite <acceptance-api> --test <TestMethodName>
      ```
   c. If tests fail, fix the backend until the tests pass.
   d. If you cannot get the tests to pass, ask the user. Do NOT change tests, DSL, or Drivers to work around it.
3. Implement the frontend:
   a. Implement the frontend changes.
   b. Run acceptance tests for the UI channel:
      ```bash
      gh optivem test system --rebuild --suite <acceptance-ui> --test <TestMethodName>
      ```
   c. If tests fail, fix the frontend until the tests pass.
   d. If you cannot get the tests to pass, ask the user. Do NOT change tests, DSL, or Drivers to work around it.
4. By now, all acceptance tests for the ticket are passing.

## AT - GREEN - SYSTEM - REVIEW (STOP)

STOP. Present the implementation to the user and ask for approval. Do NOT continue.

**Review checklist:**
- All change-driven acceptance tests are green.
- All legacy-coverage tests remain green.
- Only system code (backend + frontend) was changed — no test, DSL, or Driver edits in the diff.
- The diff is the minimum needed to make the tests pass; no speculative refactors.

## AT - GREEN - SYSTEM - COMMIT

1. COMMIT all changes (backend, frontend, and the test re-enabling from WRITE step 1) in a single commit with message `<Ticket> | AT - GREEN - SYSTEM`.
2. If a GitHub issue was provided as input, tick the checkbox for each acceptance criterion completed by this ticket (local action; not CI-gated).
3. Move the issue to **TICKET STATUS - IN ACCEPTANCE** — see [shared-ticket-status-in-acceptance.md](shared-ticket-status-in-acceptance.md).

## Anti-patterns

- **Changing tests, DSL, or Drivers to make tests pass.** Those layers are frozen by the time AT - GREEN - SYSTEM runs. If the system cannot satisfy the tests as written, the AC or the DSL is wrong — surface it to the user instead of patching around it.
- **Splitting backend and frontend into separate commits.** Both ship together as `<Ticket> | AT - GREEN - SYSTEM`. The AT cycle's terminal commit is full-stack.
- **Forgetting the checklist tick + IN ACCEPTANCE move.** The cycle is not done at the commit; it is done when the issue is in IN ACCEPTANCE with checklist items ticked.
- **Skipping the WRITE re-enable step.** The change-driven tests must be re-enabled before the implementation work begins, otherwise the test-runs in WRITE are silently skipping the disabled scenarios.
