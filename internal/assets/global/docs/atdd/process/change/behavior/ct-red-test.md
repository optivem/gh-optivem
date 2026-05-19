# CT - RED - TEST

Write External System Contract Tests against the existing DSL surface; add `"TODO: DSL"` prototypes for new DSL methods so the result compiles.

## Scope

This phase touches the `ct_test`, `dsl_port`, `dsl_core` layers (bare
layer names; resolved physical paths live in `gh-optivem.yaml paths:`
— inspect with `gh optivem process scope CT_RED_TEST`).

See [the scope rule](../../shared/scope.md).

## Steps

1. Write External System Contract Tests against the existing DSL surface. If new DSL methods are needed, call them in the test as if they exist.
2. If you need to add methods to the DSL interface, then implement the DSL Core by implementing method prototypes by throwing a runtime exception `"TODO: DSL"`, so that compilation works.
