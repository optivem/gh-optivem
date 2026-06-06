# Plan: Remove the dead `compile` and `compile-system` processes

> **DECISION MADE (2026-06-06):** `compile` and `compile-system` are unreachable from every entry
> point and redundant with `compile-tests` / `build-system`; delete both. Confirmed in review
> discussion.
>
> Review finding §2. Independent plan from the `process-flow.yaml` review; no dependency on the §1
> plans.

## Why

`compile` (`gh optivem compile`) and `compile-system` (`gh optivem system compile`) are defined in
`process-flow.yaml` but **never invoked**:

- No `process: compile` / `process: compile-system` call-activity anywhere (only `compile-tests` is
  called — `process-flow.yaml:831, :1134, :1196`).
- No `${action}` template resolves to them (the only templated actions are `implement-system` /
  `update-system` / `refactor-system`).
- Not in the `--target` selector — `targetSlices` (`driver/target.go:82-86`) maps only to
  `shared-contract`, `implement-and-verify-system-driver-adapters`, and `implement-and-verify-system`.
- No `RunProcess`-by-name reaches them.

They survive only as entries in the diagram render-ordering list (`diagram/diagram.go:96-97`), so
today they would draw as **orphan boxes** (a process node with no incoming edge). They are also
redundant: `compile-tests` and `build-system` (which compiles the system image) already cover the
compile/build needs in every cascade.

## Items

1. **`internal/atdd/runtime/statemachine/process-flow.yaml`** — delete the `compile:` process
   (≈ lines 1841-1859) and the `compile-system:` process (≈ lines 1861-1879). Leave `compile-tests`
   untouched.
2. **`internal/atdd/runtime/diagram/diagram.go`** — remove the `"compile"` and `"compile-system"`
   entries from the ordering list (`:96-97`); keep `"compile-tests"`.
3. **Test sweep** — confirm no test references the deleted *processes* by name and update any that
   assert the full process set or diagram ordering (e.g. `diagram/diagram_test.go`). Note: the
   `ctx.Params["command"] = "gh optivem compile"` fixtures in `actions/bindings_test.go` use the
   literal command string, not the process, and are unaffected. Scope `go test` per
   `[[feedback_go_test_windows.md]]`.

## Verification

- `gh optivem implement` and the `--target` slices still load and run (the engine validates all
  call-activity targets at load).
- The regenerated architecture diagram no longer shows orphan `compile` / `compile-system` nodes.
  (Do not regenerate the diagram in this plan — the regenerate-diagram workflow handles it on push,
  per `[[feedback_plans_no_diagram_regen.md]]`.)
