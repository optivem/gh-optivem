# Capture & assert dispatch params in cycle tests

**Status: PROPOSED — awaiting human approval before implementation.**

## Motivation

`internal/atdd/runtime/statemachine/behavioral_cycle_test.go` and `internal/atdd/runtime/statemachine/structural_cycle_test.go` both build a spy that records a `[]string` trail of `process.NODE_ID` entries and assert it against a `want` slice. This proves *which* nodes fired in *what* order, but throws away two things the BPMN model carries:

1. **The resolved action/agent name** — for `user_task agent: ${agent}` nodes (`red_phase_cycle.WRITE`, `red_phase_cycle.WRITE_PROTOTYPES`, `structural_cycle.IMPLEMENT_STRUCTURAL_CHANGE`), the variable bit is exactly the agent name supplied by the enclosing call_activity. Today the spy's `AgentFn = func(string) NodeFn { return noop }` discards it.
2. **The active `ctx.Params` at dispatch time** — `wrapCallActivity` pushes the call_activity's `params:` onto the Context before running the sub-process. Five distinct call sites of `red_phase_cycle` (AT_RED_TEST / AT_RED_DSL / AT_RED_SYSTEM_DRIVER / CT_RED_TEST / CT_RED_DSL / CT_RED_EXTERNAL_DRIVER) and three call sites of `structural_cycle` (SYSTEM_INTERFACE_REDESIGN_CYCLE / EXTERNAL_SYSTEM_INTERFACE_REDESIGN_CYCLE / sut_cycle.CHORE_CYCLE) push different `agent` / `phase_doc` / `phase_label` / `change_type` bundles. The current trail can't tell any of them apart.

Both test files acknowledge the gap explicitly in comments:

> "the process-qualified trail can't distinguish the two dispatches by agent — both render to `red_phase_cycle.<NODE>` — so the agent distinction lives in the params (asserted by the bindings tests), not in this trail."

This plan proposes lifting that capability into the cycle tests directly.

## Goal

Extend the spy in `behavioral_cycle_test.go` and `structural_cycle_test.go` so each `service_task` / `user_task` dispatch records both **input params** (resolved action/agent name + `ctx.Params` snapshot) and the test assertion checks them alongside the existing trail order.

## Non-goals

- **Output params (`Outcome`).** Under noop mocks every dispatched node returns `Outcome{}`. Capturing it adds zero signal to flow tests. Routing correctness is already proven indirectly (next event in the trail fires per `when:` predicate). If a future test needs to assert specific outcomes for specific dispatches (e.g. seeded RED on a verify gate), that's a separate test, not a flow test.
- **Process-level `Outputs` (BPMN-style).** `Process.Outputs` is loaded from YAML (only `github_intake` declares one today: `[ticket_type, subtype, change_type]`) but not enforced by the engine — `RunProcess` doesn't validate that a process actually published its declared outputs. Adding enforcement is a separate concern; tests that want to verify outputs can read `ctx.Get(...)` directly post-run.
- **Gates (`Gateway` nodes).** Their outcome is already captured into `ctx.Set(binding, …)` by `wrapGateway` and observed by downstream `when:` predicates. The cycle tests don't include them in the trail today (routing scaffolding, not "steps the runner executes") and that stays.
- **End-to-end / per-binding tests.** `internal/atdd/runtime/gates/bindings_test.go`, `internal/atdd/runtime/actions/bindings_test.go`, etc. already cover binding-level concerns. This plan only touches the two cycle tests.

## Approach

### Step 1 — define a `DispatchEvent` and shared spy harness

Both cycle tests duplicate ~25 lines of identical Bind / mock / spy-decorator scaffolding. Factor it out into a new test helper file alongside the existing `loadsnapshot_test.go` (or whatever helper file currently hosts `loadSnapshot`):

```go
// dispatch_spy_test.go (package statemachine)

type DispatchEvent struct {
    Process  string            // e.g. "red_phase_cycle"
    NodeID   string            // e.g. "WRITE"
    Kind     NodeKind          // ServiceTask or UserTask
    Action   string            // for service_task: the registered action name (static)
    Agent    string            // for user_task: the resolved agent name (post ${…} expansion)
    ParamsIn map[string]string // shallow copy of ctx.Params at dispatch time
}

// dispatchSpy returns a bound Engine plus a pointer to the event log.
// Caller is responsible for setting Processes["main"].Start and seeding ctx.
func dispatchSpy(t *testing.T) (*Engine, *[]DispatchEvent) { … }
```

### Step 2 — capture mechanism

The spy uses three coordinated pieces:

1. **AgentFn / ActionFn registry mocks** that back-fill the *most recently appended* event with the resolved name and a `ctx.Params` snapshot:

   ```go
   var events []DispatchEvent
   eng.AgentFn = func(name string) NodeFn {
       return func(ctx *Context) Outcome {
           e := &events[len(events)-1]
           e.Agent = name
           e.ParamsIn = cloneParams(ctx.Params)
           return Outcome{}
       }
   }
   eng.ActionFn = func(name string) NodeFn { /* same shape, sets .Action */ }
   ```

   This works because the registry is called *after* the decorator (see step 3) appends a new event for the node, so `events[len-1]` is always the right one. For `agent: ${agent}` user_tasks the runtime calls `e.AgentFn(resolvedName)` per-dispatch (see `run.go`'s `resolve` for `UserTask` with `${`); for static names the closure built once at Bind closes over `name`. Both end up appending the right value.

2. **`GateFn`** — unchanged from today (echoes `ctx[binding]`). Gates don't need params capture.

3. **Per-node decorator** — same shape as today, but appends a `DispatchEvent` instead of a string:

   ```go
   for _, process := range eng.Processes {
       procName := process.Name
       for id, node := range process.Nodes {
           if node.Kind != ServiceTask && node.Kind != UserTask {
               continue
           }
           proc, nid, kind, inner := procName, node.ID, node.Kind, node.Fn
           node.Fn = func(ctx *Context) Outcome {
               events = append(events, DispatchEvent{Process: proc, NodeID: nid, Kind: kind})
               return inner(ctx)
           }
           process.Nodes[id] = node
       }
   }
   ```

The ordering is: decorator appends → decorator calls inner → inner is the resolver-built wrapper that calls AgentFn/ActionFn-built body → that body back-fills `events[len-1]`. Single-goroutine, deterministic, no synchronization needed.

### Step 3 — assertion shape

`want` becomes `[]DispatchEvent`. Use `reflect.DeepEqual` (matches existing style) and on mismatch dump both slices with a small `formatEvent(e DispatchEvent) string` helper for human-readable diffs:

```go
want := []DispatchEvent{
    {Process: "main",           NodeID: "MOVE_TICKET_IN_PROGRESS",  Kind: ServiceTask, Action: "move_to_in_progress"},
    {Process: "github_intake",  NodeID: "CLASSIFY_TICKET_TYPE",     Kind: ServiceTask, Action: "classify_ticket_type"},
    // …
    {Process: "red_phase_cycle", NodeID: "WRITE",                   Kind: UserTask,    Agent: "atdd-test",
        ParamsIn: map[string]string{
            "agent":       "atdd-test",
            "phase_doc":   "docs/atdd/process/at-red-test.md",
            "phase_label": "AT - RED - TEST",
            "change_type": "AT - RED - TEST",
        }},
    {Process: "red_phase_cycle", NodeID: "STOP_RED_REVIEW",         Kind: UserTask,    Agent: "human", ParamsIn: …},
    // …
}
```

This blows up the line count of `want` substantially. Two mitigations:

- **Param baseline helpers**: define `atRedTestParams()`, `atRedDslParams()`, `ctRedExternalDriverParams()` etc. in the shared helper file so each event reads `ParamsIn: atRedTestParams()`. Six helpers cover every distinct `red_phase_cycle` dispatch site; three cover `structural_cycle`.
- **Event constructor helpers**: `userTask("red_phase_cycle", "WRITE", "atdd-test", atRedTestParams())`, `serviceTask("red_phase_cycle", "COMPILE", "compile_targeted", atRedTestParams())`. Reduces each row to one line.

With both, the new `want` is roughly 1.5× the size of the current `want`, in exchange for catching every dispatch param bug end-to-end.

### Step 4 — handle the templated-agent edge cases

`agent: ${agent}` resolves at dispatch time. If the call_activity didn't push an `agent` param, `ExpandParams` leaves the `${agent}` placeholder literal in the name, and the registry mock receives the literal `"${agent}"` string. Today this would silently noop because the mock doesn't care about names; with capture, it would surface in `want` as `Agent: "${agent}"` — a clear failure mode worth keeping rather than hiding. **No special-casing needed; the test will fail loudly, which is exactly what we want.**

### Step 5 — migrate the two existing tests

- `TestImplementTicket_SystemInterfaceRedesign` (the only test in `structural_cycle_test.go`): one `want` slice, ~14 events, all three `structural_cycle` params (`change_type=SYSTEM INTERFACE REDESIGN`, `agent=atdd-task`, `phase_doc=…`, `subtype=system-interface-redesign`) flow into `IMPLEMENT_STRUCTURAL_CHANGE` (templated agent), `COMPILE`, `CHOOSE_TESTS`, `RUN_TESTS`, `APPROVE_COMMIT`, `COMMIT_STRUCT`, `TICK_CHECKLIST`. `COMMIT_STRUCT` also has node-level `params: {change_type: ${change_type}}` — capture verifies it landed.
- `TestImplementTicket_Behavioral_TestOnly` / `_TestAndDSL` / `_TestAndDSLAndExternal` (three tests in `behavioral_cycle_test.go`): same shape, but with five distinct `red_phase_cycle` dispatches in the largest case (TEST + DSL + CT-TEST + CT-DSL + CT-EXTERNAL-DRIVER), plus the two `green_phase_cycle` dispatches (atdd-backend, atdd-frontend) inside `at_green_system`.

After migration, the comment-based section markers (`// AT - RED - TEST (red_phase_cycle dispatched with agent=atdd-test)` etc.) become **redundant** — each event entry now structurally encodes what the comment describes. Drop the in-slice comments at the same time; keep the doc-comments at the top of each test (those describe test intent, not test data).

## Trade-offs

**Pros**
- Closes the gap the test comments themselves call out: the agent distinction now lives in the test, not just in the bindings tests.
- Catches regressions in `wrapCallActivity`'s param push/pop, in `ExpandParams`, and in node-level `params:` (e.g. `COMMIT_STRUCT`'s `change_type: ${change_type}`) — none of which the trail-only test catches today.
- The in-slice comments that currently label sections become structural data — fewer comments to maintain, and the data can't drift from reality.
- Factoring the duplicated spy scaffolding into a shared helper is overdue and pays for itself the moment a third cycle test is added.

**Cons**
- `want` slices grow ~1.5× even with helpers. Failure messages get more verbose.
- Coupling the cycle tests to the exact `params:` strings means YAML edits (e.g. renaming `phase_label` text) require test updates. This is the *intended* property — silent param renames should be caught — but it does mean more frequent test churn during YAML editing.
- The `events[len-1]` back-fill mechanism is mildly clever. A code comment in `dispatch_spy_test.go` explaining the decorator-then-registry ordering invariant is warranted (this is a non-obvious WHY).

## Suggested order of work

1. Add `dispatch_spy_test.go` with `DispatchEvent`, `dispatchSpy(t)`, and the param-baseline helpers.
2. Migrate `structural_cycle_test.go` first — single test, smallest blast radius. Verify the new shape on this one before scaling.
3. Migrate the three behavioral tests. Drop the in-slice section-marker comments as part of the same change (their information has moved into the data).
4. Run `go test ./internal/atdd/runtime/statemachine/...` and confirm green.

No production code changes required — this is test-only.

## Open questions for the human

- Do you want the captured params asserted against the *full* `ctx.Params` snapshot (every key visible at dispatch, including upstream merged values), or filtered to just the call_activity-pushed keys for this dispatch? Full snapshot is more honest; filtered is less brittle. **Recommendation: full snapshot, with a helper that builds the expected map per call site so the brittleness lives in one place.**
- Should `gateway` nodes be added to the trail (with `binding` + `Outcome` capture) as a follow-up? They're currently excluded as routing scaffolding. Out of scope for this plan; flag if you want a separate plan drafted.
