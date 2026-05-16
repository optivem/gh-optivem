# ACCEPTANCE TEST CYCLE

RED - GREEN - REFACTOR

Inside each of these steps:

## PRE

1. Analyze Acceptance Criteria, is it written with Gherkin GIVEN-WHEN-THEN.
2. Does it have adequate positive and negative scenarios.

## RED

### RED: Test
1. For every Acceptance Criterion, write a corresponding Acceptance Test. This should be a mechanicla 1:1 translation.
2. If you need to add methods to DSL interface, then implement the DSL Core by implementing method prototypes by throwing a runtime exception  `"TODO: DSL"`, so that compilation works.
3. Set flag: `DSL Interface Changed: yes|no`


### RED: DSL
1. Implement the DSL Core for real — replace each "TODO: DSL" prototype with actual logic.
2. If you need add additional Driver interface methods:
   (a) In the System Driver Interface: implement prototype methods by throwing `"TODO: System Driver"` exception
   (b) In the External System Driver Interface: implement prototype methods by throwing `"TODO: External System Driver"` exception
3. Set flags regarding the Driver Interfaces that were changed:
   (a) Set flag: `System Driver Interface Changed: yes|no`
   (b) Set flag: `External System Driver Interface Changed = yes|no`

### RED: System Driver
1. Implement the System Driver Adapters for real - replace each "TODO: System Driver" prototype with actual logic.


## RED: External System Driver
1. Go to the ATDD - CT Cycle

## GREEN
1. Implement the System - do the simplest implementation possible with the goal of making the Acceptance Tests pass

## REFACTOR
1. Refactor the System (if any improvements are seen) - propose first, then implement