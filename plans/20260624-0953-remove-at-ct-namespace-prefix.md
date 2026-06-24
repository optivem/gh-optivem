# 2026-06-24 09:53:00 UTC — Remove at-/ct- State Key Namespace Prefix

## TL;DR

**Why:** The `at-` and `ct-` prefixes on State keys (`at-dsl-port-changed`, `ct-test-names`, etc.) were introduced to prevent clobbering when the CT cascade ran *nested inside* the AT cascade — both writing to the same State simultaneously. Plan 20260624-0925 makes the two cascades sequential (CT completes before AT starts), so the clobbering problem no longer exists. The prefixes are now dead weight: they add noise to every key reference, complicate gate bindings, and obscure what the keys actually mean.

**End result:** All `at-*` and `ct-*` State keys are replaced with their bare names (`dsl-port-changed`, `test-names`, `external-driver-port-changed`, `system-driver-port-changed`, `isolated-test-names`). The `landingStateKey` namespacing function in `outputs.go` is removed or made a no-op. Gate bindings, YAML params, and tests are updated throughout. The `test-category` param is audited and removed if its only remaining purpose was namespacing.

## Outcomes

- State keys are named for what they mean, not which cascade wrote them: `dsl-port-changed` not `at-dsl-port-changed`.
- `landingStateKey` in `outputs.go` and `namespacedLandingKeys` are deleted (or collapsed to identity).
- All YAML `${at-test-names}`, `${ct-test-names}`, `test-category: acceptance/contract` params that existed solely for namespacing are cleaned up.
- Gate bindings in `bindings.go` read bare keys.
- 155 occurrences across 11 files are updated; `go build ./...` and all process-flow tests pass.

## Prerequisite

**This plan must run after plan `20260624-0925-lift-external-drivers-to-cycle.md` is fully executed and passing.** The sequential CT→AT ordering that makes the namespace safe to remove is introduced by that plan.

## ▶ Next executable step (resume here)

Step 1: Audit all usages of namespaced keys and `test-category` across the 11 affected files — establish the full rename map and whether `test-category` has any remaining purpose beyond namespacing.

## Steps

- [ ] Step 1: Audit — for each namespaced key (`at-dsl-port-changed`, `ct-dsl-port-changed`, `at-test-names`, `ct-test-names`, `at-external-driver-port-changed`, `ct-external-driver-port-changed`, `at-system-driver-port-changed`, `ct-system-driver-port-changed`, `at-isolated-test-names`, `ct-isolated-test-names`), confirm its bare replacement. Audit `test-category` param usages: does anything other than `landingStateKey` / namespacing consume it? Record the full picture before touching code.
- [ ] Step 2: Update `outputs.go` — remove `namespacedLandingKeys` map and simplify `landingStateKey` to return the key unchanged (or delete the function and inline). Update `validateOutputsAndScopes` and `validateOutputsAndScopesForExternalDriverPort` accordingly.
- [ ] Step 3: Update `gates/bindings.go` — rename all `at-*` and `ct-*` key references to bare keys (21 occurrences). If `test-category` is no longer used, remove any logic that branches on it.
- [ ] Step 4: Update `process-flow.yaml` — rename all `${at-*}` / `${ct-*}` param references and `test-category: acceptance/contract` params that existed solely for namespacing (51 occurrences). If `test-category` has no remaining purpose, remove those params entirely.
- [ ] Step 5: Update Go test files — `gates/bindings_test.go` (20), `actions/bindings_test.go` (30), `runtime/trace/trace_test.go` (3), `engine/statemachine/channels_test.go` (8), `process/transitions_test.go` (10), `actions/channel_test.go` (5), `actions/tracker.go` (1), `actions/channel.go` (1).
- [ ] Step 6: Run `go build ./...` and the full test suite. Fix any remaining references.

## Open questions

- Does `test-category` serve any purpose beyond namespacing (e.g., scope narrowing, prompt injection, verify-mode branching)? If yes, it stays as a param but `landingStateKey` still becomes a no-op. Step 1 answers this.
- Are there runtime prompt files under `internal/atdd/assets/` that reference `${at-test-names}` or similar? A grep of the assets dir should be added to Step 1.
