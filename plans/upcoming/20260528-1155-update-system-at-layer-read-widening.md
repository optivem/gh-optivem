# Should `update-system` get the same AT-layer read widening as `implement-system`?

> ⏸ **Stub — needs discussion.** Items intentionally not yet written.
> The shape of the answer depends on whether the reshape agent is
> trusted to use AT visibility well, vs whether it should infer
> "don't break the tests" from compile-and-test failure signals alone.

## Context

Commit `454eb64` (2026-05-27) widened `implement-system`'s read scope
from `[system-path]` to the full system-test layer:

```
read: [at-test, ct-test, dsl-port, dsl-core, driver-port, driver-adapter,
       external-system-driver-port, external-system-driver-adapter, system-path]
```

The rationale (see the now-deleted plan
`20260527-1507-widen-implement-system-read-scope.md`, viewable via
`git show 454eb64`): the implementer is asked to make a failing AT
pass, so denying it read access to the AT (and the DSL / driver
layers above) forced it to infer requirements from compile errors
and ticket prose — both downstream of the actual specification.

`update-system` (`process-flow.yaml:1477+`) is the **reshape variant**
of `implement-system`. Same agent class, different verb: change the
system's *shape* without changing its *behaviour*. Its current scope:

```
read:  [system-path, driver-adapter, driver-port]
write: [system-path, driver-adapter]
```

It cannot read the ATs, the DSL, the contract tests, or the
external-driver layer.

## The question

Should `update-system` get the same AT-layer read widening?

### Arguments for widening

1. **Symmetry with `implement-system`.** Both phases are dispatches
   of the same agent class on the same SUT surface. The reshape
   verb does not change the system-test layer that defines what the
   reshape must preserve.
2. **Operational visibility.** A reshape that needs to preserve
   behaviour cannot verify behaviour without seeing the tests that
   define it. Today the reshape relies on the downstream
   `build-system` / `run-tests` phases to catch any behaviour drift,
   which is a slow, after-the-fact signal.
3. **Refactor doctrine.** `refactor-system` (the third writing-agent
   phase in this family) already reads the full system-test layer
   (`process-flow.yaml:1605+`). It would be inconsistent for the
   middle phase (`update-system`) to be the only one denied.

### Arguments against widening

1. **Scope discipline.** A reshape that knows the AT exists may be
   tempted to "fix" a test it considers wrong, or to encode AT
   assumptions into the reshape that should not survive the next AT
   rewrite. Narrower reads → narrower temptation.
2. **Test-loop forcing function.** Today's "reshape blind, then
   `run-tests` tells you" loop is slower but produces unambiguous
   behaviour-preservation evidence. Letting the agent peek at the
   tests up-front trades a clear pass/fail signal for a possibly
   tautological one ("the agent read the test, then wrote code that
   passes that test by inspection — was behaviour actually
   preserved?").
3. **YAGNI.** The current scope works for the rehearsal scenarios.
   Widening it pre-empts a problem that hasn't shown up yet.

## Open questions before items get written

1. **What's the failure mode this would fix?** If we can't point at
   a specific reshape dispatch that failed because of missing AT
   visibility, the change is speculative. If we can, the failure
   mode constrains the right answer (e.g. if the problem is "reshape
   agent removed a column that's only used in a stub setup", then
   the right fix may be widening `external-system-driver-adapter`
   read access specifically, not the whole AT layer).

2. **Should the contract-test layer be included?** `implement-system`
   reads `ct-test` because contract tests pin the stub contract the
   production system must satisfy. The same logic *might* apply to
   reshape, but contract tests are typically more stable across
   reshapes than across feature work, so the marginal value is
   lower.

3. **Cross-cut with the migration-path plan.** The sibling plan
   `20260528-1145-db-migrations-as-first-class-scope-key.md` adds
   `system-db-migration-path` to `update-system`'s `read:`/`write:`.
   If this plan also widens the read scope, the resulting MID body
   becomes large. Worth deciding which lands first and whether the
   migration-path plan should leave the AT-layer-read question
   alone (it currently does — flagged as "Deferred to a separate
   plan").

## Items

(Not yet written — depends on discussion above.)

## References

- `internal/atdd/runtime/statemachine/process-flow.yaml:1477+` —
  `update-system` MID, current scope.
- `internal/atdd/runtime/statemachine/process-flow.yaml:1605+` —
  `refactor-system` MID, full system-test-layer reads (precedent for
  the widening shape).
- `git show 454eb64` — the rationale for `implement-system`'s
  widening, which this plan considers extending.
- Sibling plan `plans/upcoming/20260528-1145-db-migrations-as-first-class-scope-key.md`
  — adds the migration-path key to `update-system`; deferred this
  read-widening question to this plan rather than bundling.
