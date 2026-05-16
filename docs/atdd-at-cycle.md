# ACCEPTANCE TEST CYCLE

RED - GREEN - REFACTOR

Inside each of these steps:

## PREP

1. Analyze Acceptance Criteria, is it written with Gherkin GIVEN-WHEN-THEN.
2. DOes it have adequate positive and negative scenarios.

## RED

### RED: Test
1. Write Acceptance Test based on Acceptance Criterion.
2. Repeat this for all Acceptance Criteria.
3. If you need to add methods to DSL interface, then implement the DSL method prototypes by throwing a runtime exception  `"TODO: DSL"`, so that compilation works.

### RED: DSL
1. Implement the DSL for real — replace each "TODO: DSL" prototype with actual logic.
2. If you need add additional Driver interface methods, implement a prototype method by throwing a `"TODO: Driver"` exception.
3. 

- Flag set: `External System Driver Interface Changed = yes|no`.
- Flag set: `System Driver Interface Changed = yes|no`.

### RED: System Driver
