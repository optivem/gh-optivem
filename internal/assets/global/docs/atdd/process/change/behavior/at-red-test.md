# AT - RED - TEST

Write acceptance tests; add `"TODO: DSL"` prototypes so the result compiles.

## Scope

This phase touches the `at_test`, `dsl_port`, `dsl_core` layers (bare
layer names; resolved physical paths live in `gh-optivem.yaml paths:`
— inspect with `gh optivem process scope AT_RED_TEST`).

See [the scope rule](../../shared/scope.md).

## Steps

1. For every Acceptance Criterion, write a corresponding Acceptance Test. This should be a mechanical 1:1 translation.
2. If you need to add methods to DSL interface, then implement the DSL Core by implementing method prototypes by throwing a runtime exception  `"TODO: DSL"`, so that compilation works.
3. Set flag: `DSL Interface Changed: yes|no`
