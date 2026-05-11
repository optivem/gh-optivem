# `gh optivem compile system` / `compile system-tests` — replace `compile-all.sh` shell-out

> The subcommand and the unify-with-`gh optivem init` work landed
> 2026-05-11; the shop-template multi-config fan-out (Step 4) landed the
> same day in the shop repo. This file is now a stub tracking only the
> deferred / out-of-scope items below.

## Step 5 — `compile-targeted.sh` (deferred)

The state machine has a parallel `compileTargeted` action
(`internal/atdd/runtime/actions/bindings.go:743-759`) that shells out
to `./compile-targeted.sh <scope>`. It's currently **unwired** — no
YAML node calls it (see comment at lines 717-723). Revisit when the
AT/CT creative/mechanical split refactor
(`plans/20260505-230100-at-ct-cycle-creative-mechanical-split.md`)
needs targeted compile.

## Out of scope

- `disable-test.sh` / `enable-test.sh` — same shell-script pattern but
  different concern.
- Reading any docker-orchestration config from `gh-optivem.yaml` —
  unrelated; that conversation belongs elsewhere.
