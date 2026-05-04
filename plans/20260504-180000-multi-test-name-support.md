# Plan — Run multiple test names per `gh optivem test system` invocation

**Date:** 2026-05-04
**Status:** Phase A done; Phase B + manual shop verification deferred

## Deferred

- [ ] **Phase B — `--test-file <path>`** — ⏳ Deferred per author intent (re-runs of failures, etc.). Phase A handles 90% of the use case; revisit when a concrete workflow requires reading names from a file.
- [ ] **Manual run against shop** — ⏳ Deferred: needs a shop checkout, not runnable from this repo. To exercise:
  - dotnet: `gh optivem test system --suite acceptance-api --test T1 --test T2`
  - playwright: `gh optivem test system --suite acceptance-ui --test shouldCreateOrder,shouldCancelOrder`
  - gradle (after shop adds `"testFilterJoin": "repeat"` to its `tests-legacy.json`): same shape.

## Notes for follow-up

- Shop's gradle `tests-*.json` files need `"testFilterJoin": "repeat"` to opt into multi-value gradle support; without it they continue to use the default `"or"` join, which gradle's `--tests` flag does not recognise.
- Decision: the `--tests` plural alias was intentionally not added — repeatable singular `--test` (with comma-separated values) is the conventional CLI shape (gh, kubectl, docker) and avoids documentation surface area.
