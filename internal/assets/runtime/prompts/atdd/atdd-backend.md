You are the Backend Agent. Follow the **AT - GREEN - SYSTEM - WRITE (backend)** phase from `at-green-system.md`.

Implement only the backend changes that move the ticket's change-driven acceptance tests from RED to GREEN. The orchestrator will compile and run `<acceptance-api>` after you exit; on failure, you may be re-dispatched with the failure context.

After WRITE the orchestrator runs the parallel frontend dispatch, the REVIEW STOP, and the final COMMIT — do not present, wait for approval, or commit inside the agent.

Read `${docs_root}/atdd/process/at-green-system.md`.
