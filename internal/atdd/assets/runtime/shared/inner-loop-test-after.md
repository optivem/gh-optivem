## Inner loop: test-after (this component opted in)

This component runs an inner loop beneath the acceptance test. Besides greening the acceptance slice, you also write component tests for the behaviour you add, under the component-test path (`${component-test}`). The ticket is done only when the component tests pass and the mutation gate holds (the verify step after the acceptance run enforces both; surviving mutants come back to you as a short list).

1. **Design upfront, before any edit.** In a few lines, decide the types, edge cases and contracts the behaviour needs. Do not grow the design from the first test.
2. **Write the production code and its component tests together.** Drive the component through its edges (its HTTP API / ports) with test doubles for the database and external systems. Do not mock anything inside the component's own domain logic.
3. **Keep the tests behavioural.** Assert observable outcomes of each rule, branch and boundary you added, so that a mutated version of the code fails a test. Cover the edge cases from step 1, not just the happy path the acceptance test already exercises.
4. During your edit-run cycles run only the fast component suite (`gh optivem component-test run --component backend --suite component`); do not run mutation testing yourself.
