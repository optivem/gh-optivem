# Wire the `fix` recovery path — set `failure-kind`, thread it into `fix`, enumerate kinds

> ⚠️ **Dependency / concurrency warning — do not start until plan
> `20260526-1300-ticket-body-parser-wire-and-validate.md` has fully
> landed (committed and its pickup marker removed).**
>
> The 1300 plan has uncommitted edits across `internal/atdd/runtime/`
> — including `actions/bindings.go`, `actions/bindings_test.go`,
> `driver/driver.go`, `gates/bindings.go`, `intake/parse.go`,
> `intake/sections.go`, `clauderun/clauderun.go`, and
> `statemachine/process-flow.yaml`. Every one of those files (except
> `intake/*` and `gates/bindings.go`) is in this plan's scope too.
> Starting this plan while 1300 is in flight will produce silent merge
> collisions: identical-author commits from a parallel Claude instance
> can absorb work in either direction, and the surface area
> (`statemachine/process-flow.yaml`, `actions/bindings.go`) is exactly
> the area where a partial-overlap edit produces a runtime that
> compiles but behaves wrong.
>
> Before picking this plan up: `git status --short` must show no
> modified files under `internal/atdd/runtime/`, and there must be no
> open `🤖 Picked up by agent` marker on the 1300 plan file (or that
> plan file must be deleted, meaning 1300 fully resolved).

## Origin / intent

ATDD rehearsal run on 2026-05-26 (`/atdd-rehearsal implement`) failed
deep in the call stack with:

```
[trace 13:04:16] OK RUN_COMMAND -> (no result)  (150ms)
   state: command-succeeded=false
[trace 13:04:16] > GATE_COMMAND_SUCCEEDED  kind=gateway binding=command-succeeded
[trace 13:04:16] > CALL_FIX  kind=call-activity process=fix
   …
[trace 13:07:30] FAIL RUN_AGENT -> dispatcher: load tuning for "fix-${failure-kind}":
                  agents: no embedded prompt for "fix-${failure-kind}"
```

The proximate trigger was a real `gh optivem system build` failure (~150 ms,
worth diagnosing separately). The bug exposed by that failure is that the
**`fix` recovery branch of every `execute-command` and `execute-agent` cycle is
not wired end-to-end** — the runtime walks into it and crashes with the
unsubstituted placeholder `fix-${failure-kind}` as the agent name.

Four independent gaps line up to cause the crash. Each gap is documented in
the code as "Phase D wiring" but never landed:

1. **`runCommand` never writes `failure-kind` to state.** Its own docstring
   (`internal/atdd/runtime/actions/bindings.go:601`) says it should set
   `failure-kind = "command-failed"` on failure, but only `command-succeeded`
   is written. Tests pin the missing behaviour as the current contract
   (`bindings_test.go:337`).
2. **`execute-command`'s `CALL_FIX` passes no params.** `fix`'s body
   references `${failure-kind}` but `process-flow.yaml:1652` calls the
   subprocess with `params:` absent; nothing flows in.
3. **`wrapCallActivity` expands templates against `ctx.Params` only.**
   `internal/atdd/runtime/statemachine/run.go:165` substitutes call-site
   param values from the parent's `Params` map. Values written to
   `ctx.State` (which is where `failure-kind` lives, set by
   `validateOutputsAndScopes`) are invisible to the substitution.
   So even in the `execute-agent` path — which does set `failure-kind` —
   the `fix` template still renders to `fix-${failure-kind}` literal.
4. **No `fix-command-failed.md` prompt exists.** `agents/embed_test.go:156`
   pins the closed set: `fix-unexpected-passing-tests`,
   `fix-unexpected-failing-tests`. Even if (1)–(3) were fixed, a command
   failure would land on a missing prompt.

The YAML itself flags this openly at `process-flow.yaml:92-97` and
`1690-1694` ("Phase D wires the param-derivation gate so this becomes
`${fix-task-name}`; for now the literal template documents the convention").
This plan closes that gap.

## Design decisions (resolved 2026-05-26)

1. **`failure-kind` flows through ctx.Params, not ctx.State.** Two options
   were considered:

   - **(a)** Extend `wrapCallActivity` / `ExpandParams` to fall back to
     `ctx.State` after `ctx.Params`. Generalises substitution, but blurs
     the param/state boundary the runtime currently keeps clean: params are
     call-site values, state is run-scoped accumulation. Future templates
     would silently pick up state keys with no declared call-site contract.
   - **(b)** Pass `failure-kind: ${failure-kind}` explicitly at every
     `CALL_FIX` site, treating the value as a parent-state-to-child-param
     bridge resolved at dispatch time.

   **Chosen: (b), with a small extension.** `ExpandParams` is extended to
   read from `ctx.State` for keys that don't appear in `params` (additive
   fallback — params still win). This keeps every `${…}` substitution
   resolved against the same scope chain (params → state), without
   requiring the caller to declare a passthrough param for every state
   key. The state-fallback path is documented as the bridge for binding-
   written values (`failure-kind`, `test-outcome`, etc.) that downstream
   templates want to consume.

   *Why not pure (b):* call-site verbosity. Every `CALL_FIX` and every
   downstream subprocess would need `failure-kind: ${failure-kind}` in its
   params block, repeated through the call stack. The runtime already
   walks state for gate `binding:` reads (gateway predicates evaluate
   against state, not params); allowing templates to do the same is a
   consistent rule.

2. **`runCommand` sets `failure-kind = "command-failed"` on failure.** Matches
   the existing docstring (`bindings.go:601`) and the YAML's `fix` MID
   doc-block convention (Q-late-5 β-convention: `"fix-" + failure.kind`).
   The kind is a stable label, not a templated message; `"command-failed"`
   is the closed-set entry.

3. **A new `fix-command-failed.md` prompt is added.** Lives alongside the
   existing two under `internal/assets/runtime/prompts/atdd/`. Body
   mirrors the structure of `fix-unexpected-failing-tests.md` (Diagnose,
   Investigate, Remediate, COMMIT outputs) but framed around a failed
   shell command: receives the command line, exit code, stderr tail, and
   working-tree dirty file listing in its prompt. `embed_test.go`'s
   `wantKinds` slice is extended to pin the new kind.

4. **The `fix-on-failure: false` flag stays as-is.** It's already wired
   correctly through `fixOnFailureEnabled` (`gates/bindings.go:311`) and
   the YAML edges (`1625-1626`). No change in this plan.

## Scope

In scope:

- `internal/atdd/runtime/actions/bindings.go` — set
  `ctx.Set("failure-kind", "command-failed")` inside `runCommand` when
  the shell call errors. Update the docstring's "Writes" block to list
  `failure-kind` alongside `command-succeeded` / `test-outcome`.
- `internal/atdd/runtime/actions/bindings_test.go` — extend
  `TestRunCommand_FailureRoutes_NotErrors` to assert `failure-kind` is
  set to `"command-failed"`. Add a happy-path assertion that
  `failure-kind` is **not** set on success (mirroring the test-outcome
  pattern at line 329).
- `internal/atdd/runtime/statemachine/run.go` — extend `ExpandParams` to
  accept a `state map[string]any` (or take a `*Context`) and substitute
  `${name}` from state when the key is not in params. Update the
  docstring to describe the params-then-state scope chain. Update
  `wrapCallActivity` to pass the live state alongside parent params
  (already inside the closure — `ctx.State` is reachable).
- `internal/atdd/runtime/driver/driver.go` — update the four
  `statemachine.ExpandParams(…, ctx.Params)` call sites in the
  dispatchers (`newHumanStopDispatcher:745`, `newApproveDispatcher:780`,
  `newClaudeRunDispatcher:830`/`839`/`859`, `promptForAgent:949`/`950`)
  to use the new state-aware signature so user-facing banners see the
  same substitutions the engine sees.
- `internal/atdd/runtime/statemachine/run_test.go` — add unit tests for
  the params-then-state precedence in `ExpandParams`. Add an
  integration-style test that a `call-activity` referencing a
  state-written key in its templated param resolves correctly.
- `internal/assets/runtime/prompts/atdd/fix-command-failed.md` — new
  prompt. Body follows the existing fix-* shape (preamble, failure
  context placeholders, remediation rules, COMMIT outputs block).
  Placeholders consumed: `${command}`, `${command_exit_code}`,
  `${command_stderr_tail}`, `${changed_files}`.
- `internal/atdd/runtime/agents/embed_test.go` — extend `wantKinds`
  with `"command-failed"`.
- `internal/atdd/runtime/clauderun/clauderun.go` —
  `Options.CommandLine` / `Options.CommandExitCode` /
  `Options.CommandStderrTail` fields plus their registration in the
  prompt renderer's `params` map (load-bearing only when the dispatched
  agent is `fix-command-failed` — mirror the `fix-unexpected-*`
  `${changed_files}` pattern at `driver.go:924-939`).
- `internal/atdd/runtime/driver/driver.go::newClaudeRunDispatcher` —
  populate the new `cOpts.Command*` fields when dispatching
  `fix-command-failed`. Source values from `ctx.State` keys written by
  `runCommand` (see below).
- `internal/atdd/runtime/actions/bindings.go::runCommand` — write
  `command-line`, `command-exit-code`, `command-stderr-tail` into
  `ctx.State` on failure (alongside the new `failure-kind`). These
  feed the new prompt placeholders.

Out of scope (separate plans / follow-up):

- **Root-cause of the `gh optivem system build` failure that triggered
  the rehearsal crash.** Diagnose in a separate session — this plan is
  about closing the recovery path, not the upstream build bug.
- **`fix-scope-diff` and `fix-missing-output` prompts.** The
  `validateOutputsAndScopes` action already writes those failure-kinds
  (`bindings.go:670,705`), so once Items 1–4 land, the
  `execute-agent` → CALL_FIX path will also crash on the missing
  prompts. Worth landing in the same wave, but the prompt design
  (what the fix agent should do for a scope-diff vs missing-output
  failure) is a separate authoring exercise. Tracked as a follow-up
  plan rather than absorbed here, since the body and the per-kind
  remediation steps need writing-agent thought, not just plumbing.
- **`spike` ticket-kind** and other Q-late-5 fix kinds not currently
  reachable in the flow.
- **Telemetry / metrics on fix-* dispatch frequency.** Useful for
  diagnosing flaky tests / commands long-term; defer.

## Reference: `failure-kind` lookup table

The closed set after this plan lands:

| failure-kind                | Set by                                                        | Reachable when                                  | Prompt                          |
|---                          |---                                                            |---                                              |---                              |
| `command-failed`            | `runCommand` (new, Item 1)                                    | LOW `execute-command` GATE_COMMAND_SUCCEEDED=false | `fix-command-failed.md` (new, Item 5) |
| `missing-output`            | `validateOutputsAndScopes` (`bindings.go:670`)                | LOW `execute-agent` agent emitted incomplete outputs | *Out of scope — separate plan*   |
| `scope-diff`                | `validateOutputsAndScopes` (`bindings.go:705`)                | LOW `execute-agent` working-tree changes outside scope | *Out of scope — separate plan*   |
| `unexpected-passing-tests`  | `fix-unexpected-passing-tests` cycle (legacy MID convention) | `verify-tests-fail` saw a pass                  | `fix-unexpected-passing-tests.md` (existing) |
| `unexpected-failing-tests`  | `fix-unexpected-failing-tests` cycle (legacy MID convention) | `verify-tests-pass` saw a fail                  | `fix-unexpected-failing-tests.md` (existing) |

The last two are pre-existing MID-name conventions where the "kind" is
baked into the MID's task-name rather than computed at runtime — they
predate the Q-late-5 β-convention and are left untouched in this plan.

## Items

### Item 1 — `runCommand` writes `failure-kind` + diagnostic state on failure

**Files:**
- `internal/atdd/runtime/actions/bindings.go::runCommand` — on shell
  error: `ctx.Set("failure-kind", "command-failed")`,
  `ctx.Set("command-line", cmd)`,
  `ctx.Set("command-exit-code", <exit code from runShell>)`,
  `ctx.Set("command-stderr-tail", <last N stderr lines>)`. Update the
  docstring's "Writes" block.
- `internal/atdd/runtime/actions/runshell.go` (or wherever
  `Deps.Shell.Run` lives) — ensure the shell adapter surfaces exit
  code + stderr separately so `runCommand` can stash them. If the
  current Shell interface only returns `(stdout, err)`, extend it to
  `(result, err)` with `result.ExitCode` / `result.Stderr` fields, or
  add a sibling method — pick the smaller-blast-radius option after
  reading the adapter.
- `internal/atdd/runtime/actions/bindings_test.go` —
  - `TestRunCommand_HappyPath`: assert `failure-kind` is NOT set on
    success (mirror the `test-outcome` check at line 329).
  - `TestRunCommand_FailureRoutes_NotErrors`: assert
    `failure-kind == "command-failed"`, `command-line == "gh optivem
    commit"`, and `command-exit-code` matches the fake shell's exit.

**Verify:** `go test ./internal/atdd/runtime/actions/...` passes.

### Item 2 — `ExpandParams` params-then-state scope chain

**Files:**
- `internal/atdd/runtime/statemachine/run.go::ExpandParams` — change
  signature to `ExpandParams(s string, params map[string]string, state
  map[string]any) string` (or accept `*Context` directly — pick the
  shape that minimises caller-site churn after reading every call site).
  Substitute `${name}` from params first; for keys not present in params,
  attempt state and stringify (mirror `Context.GetString`'s coercion).
  Update the docstring to describe the scope chain.
- `internal/atdd/runtime/statemachine/run.go::wrapCallActivity` — pass
  `ctx.State` alongside `prev` (`ctx.Params`) when expanding
  `raw.Process` (line 148) and each `raw.Params[k]` value (line 165).
- `internal/atdd/runtime/driver/driver.go` — update every
  `statemachine.ExpandParams(…, ctx.Params)` site to pass `ctx.State`
  too:
  - `newHumanStopDispatcher:745` — `raw.Documentation`
  - `newApproveDispatcher:780` — `raw.Documentation`
  - `newClaudeRunDispatcher:830` — `raw.Agent`
  - `newClaudeRunDispatcher:839` — node-param values
  - `newClaudeRunDispatcher:859` — `raw.Documentation`
  - `promptForAgent:949/950` — `raw.Agent` + `raw.Documentation`
- `internal/atdd/runtime/statemachine/run_test.go` — add:
  - `TestExpandParams_ParamsTakePrecedenceOverState`
  - `TestExpandParams_StateFallbackForUnknownParamKey`
  - `TestExpandParams_NilStateBehavesLikeOldSignature` (regression
    insurance)
  - integration test: a process whose call-activity passes
    `foo: "value-${state-only-key}"` resolves correctly when
    `state-only-key` is set in ctx.State by an upstream service-task.

**Verify:** `go test ./internal/atdd/runtime/...` passes (the runtime
test surface is broad; expect minor fixture churn).

### Item 3 — Wire `failure-kind` through `execute-command` → `fix` → `execute-agent`

No YAML edits needed beyond verification — after Item 2, the existing
`fix` body's `task-name: "fix-${failure-kind}"` resolves correctly
against the state value Item 1 writes. The end-to-end path:

1. `runCommand` fails → writes `failure-kind=command-failed` to state (Item 1).
2. `GATE_COMMAND_SUCCEEDED` routes false → CALL_FIX.
3. CALL_FIX dispatches the `fix` process with empty params; `wrapCallActivity` pushes a merged scope (parent params + nothing).
4. `fix`'s inner EXECUTE_AGENT renders its templated params: `task-name: "fix-${failure-kind}"` → `ExpandParams` looks up `failure-kind` in params (absent) then state (`"command-failed"`) → resolves to `"fix-command-failed"` (Item 2).
5. EXECUTE_AGENT's RUN_AGENT renders `agent: ${task-name}` → resolves to `"fix-command-failed"` against the now-merged params.
6. Dispatcher loads tuning for `"fix-command-failed"` → succeeds (Item 5).

**Files:** none (this is a verification item — the wiring falls out of
Items 1 + 2 + 5).

**Verify:** add an integration test in
`internal/atdd/runtime/statemachine/run_test.go` (or alongside the
existing statemachine integration tests) that drives a synthetic
`execute-command` with a failing shell adapter and asserts the
dispatch lands on the `fix-command-failed` agent name. The test uses
a fake agent registry that records the dispatched name without
actually calling Claude. Memory: `feedback_statemachine_test_loop_hazard`
— audit gate fixtures before running and scope the test to one
process to avoid the 20GB-RAM loopback hazard.

### Item 4 — Same wiring works for `execute-agent` validation failures

Verify Items 1 + 2 also fix the `execute-agent` → CALL_FIX path that
`validateOutputsAndScopes` writes `failure-kind` for (`missing-output`,
`scope-diff`). The wiring is identical; only the prompts are missing.

**Files:** none (verification + plan-the-followup item).

**Verify:** add a unit test asserting `ExpandParams` resolves
`${failure-kind}` correctly from each of the two
`validateOutputsAndScopes` failure paths. Stub agent registry as in
Item 3.

Then document the follow-up: a sibling plan file
`plans/<timestamp>-fix-missing-output-and-scope-diff-prompts.md`
authoring the two missing prompts. Out of scope here; gate the YAML
landing of the recovery branch's exhaustiveness on that plan.

### Item 5 — Author `fix-command-failed.md` prompt

**Files:**
- `internal/assets/runtime/prompts/atdd/fix-command-failed.md` — new
  prompt. Frontmatter (model, effort) mirrors
  `fix-unexpected-failing-tests.md`. Body:
  - "You are the **fix-command-failed** agent."
  - Receives `${command}` (the failed shell command line),
    `${command_exit_code}`, `${command_stderr_tail}`, `${changed_files}`
    (working-tree dirty list, populated by `fixChangedFiles` extension
    in Item 6).
  - Asks the agent to (a) inspect the working tree, (b) reproduce the
    failure locally, (c) make the minimum patch, (d) re-run the
    command to confirm exit-0, (e) COMMIT with a structured outputs
    block.
- `internal/atdd/runtime/agents/embed_test.go::TestFixKindPromptsExist`
  — add `"command-failed"` to `wantKinds`. The list now reads:
  ```go
  wantKinds := []string{
      "unexpected-passing-tests",
      "unexpected-failing-tests",
      "command-failed",
  }
  ```
- `internal/atdd/runtime/agents/embed_test.go` — extend
  `TestPrompt_StripsFrontmatter` indirectly (auto-covered by the walk
  over `Names()`).

**Verify:** `go test ./internal/atdd/runtime/agents/...` passes;
`gh optivem architecture show` regeneration (if affected) clean.

### Item 6 — Dispatcher plumbing for the new prompt placeholders

**Files:**
- `internal/atdd/runtime/clauderun/clauderun.go` — add
  `Options.CommandLine`, `Options.CommandExitCode` (int),
  `Options.CommandStderrTail` (string) fields. In the prompt-render
  body (around line 524-557 where `acceptance_criteria` is registered
  load-bearing), add a block: only register `params["command"] =
  opts.CommandLine` (etc.) when non-empty. Document the load-bearing
  semantics in the `Options.Command*` doc-comments.
- `internal/atdd/runtime/clauderun/clauderun_test.go` — extend the
  load-bearing-placeholder regression test to cover
  `${command}` fail-fast when a prompt references it and the value is
  empty (mirroring `${checklist}` and `${acceptance_criteria}`).
- `internal/atdd/runtime/driver/driver.go::newClaudeRunDispatcher` —
  populate `cOpts.CommandLine` / `cOpts.CommandExitCode` /
  `cOpts.CommandStderrTail` from `ctx.GetString("command-line")` /
  `ctx.GetInt("command-exit-code")` / `ctx.GetString("command-stderr-tail")`.
  These read from state, which Item 1 populates.
- `internal/atdd/runtime/driver/driver.go::fixChangedFiles` — extend
  the switch (line 925-928) to include `"fix-command-failed"` so the
  prompt receives a working-tree listing alongside the command
  failure context.

**Verify:** `go test ./internal/atdd/runtime/clauderun/...` passes;
`go test ./internal/atdd/runtime/driver/...` passes.

### Item 7 — Update doc-comments noting "Phase D will wire this"

**Files:**
- `internal/atdd/runtime/statemachine/process-flow.yaml:92-97` — rewrite
  the `fix-*` agent inventory doc-block to describe the realised wiring
  (params-then-state substitution; closed set of failure-kinds; new
  `command-failed` kind). Drop the "Phase D" forward-reference.
- `internal/atdd/runtime/statemachine/process-flow.yaml:1690-1694` —
  rewrite the inline comment on the templated `task-name` to describe
  the realised resolution path (state fallback in `ExpandParams`).
  Drop the "Phase D wires the param-derivation gate so this becomes
  `${fix-task-name}`" claim — the value resolves directly now.
- `internal/atdd/runtime/agents/embed_test.go:151` — rewrite the
  `TestFixKindPromptsExist` comment to drop the "Phase D will wire the
  binding that emits `failure-kind`; until then this test guards the
  convention" framing, since the binding now exists.

**Verify:** `go vet ./...` clean; comments match the realised flow.

## Sequencing

- Item 1 (runCommand writes failure-kind) — leaf; land first. No
  dependents until Items 3/4 verify.
- Item 2 (ExpandParams scope chain) — leaf, broad blast radius
  (touches every dispatcher + the engine). Land second, in its own
  commit, with the wider runtime test pass as gate.
- Item 5 (new prompt + embed_test) — independent of 1/2/6; can land
  in parallel with Item 2.
- Item 6 (dispatcher plumbing for new placeholders) — depends on
  Items 1 + 5 (state keys exist; prompt exists).
- Items 3 + 4 (end-to-end verification) — depend on Items 1, 2, 5, 6.
- Item 7 (doc cleanup) — runs after all the above land; no code dep.

Total ~7 commits over the plan, biggest is Item 2 (engine signature
change across dispatchers).

## Verification (end-to-end)

After landing, re-run the rehearsal scenario that surfaced the bug:

1. Reset to a state where `gh optivem system build` will fail (the
   upstream root cause from the original trace).
2. Run `gh optivem implement` against the rehearsal ticket.
3. Observe in the trace:
   - `RUN_COMMAND` fails → `command-succeeded=false`,
     `failure-kind=command-failed` in state.
   - `GATE_COMMAND_SUCCEEDED` routes false → `CALL_FIX`.
   - `fix` process dispatches `EXECUTE_AGENT` with `task-name`
     resolved to `"fix-command-failed"` (not the literal
     `fix-${failure-kind}`).
   - `RUN_AGENT` dispatches the `fix-command-failed` agent with the
     prepared-prompt banner showing `command:`, `exit code:`,
     `stderr tail:`, `changed files:` populated from state.
   - Agent's COMMIT lands → `APPROVE_POST` → `EXECUTE_AGENT_END` →
     `FIX_END` → back to `EXECUTE_COMMAND_END`. The cycle continues.

If the agent's fix attempt re-runs the build command and it still
fails, the *outer* re-verify lands a fresh validation failure with
`fix-on-failure: false` (already wired) — single-attempt remediation,
no infinite loop.

## Cross-references

- Origin trace: rehearsal run on 2026-05-26, `[trace 13:04:16]` onwards.
- Adjacent in-flight plan: `20260526-1300-ticket-body-parser-wire-and-validate.md`
  (also wiring Phase D gaps; orthogonal — that one is ticket-body
  parsing, this one is recovery-path dispatch).
- Memory: `feedback_statemachine_test_loop_hazard` — Item 3's
  integration test must scope to a single process and audit gate
  fixtures to avoid the 20GB-RAM loopback hazard.
- Memory: `feedback_no_layer_coding_in_names` — `failure-kind` values
  (`command-failed`, `missing-output`, `scope-diff`) describe scope,
  not layer. No `fix_command_failed_lowprimitive` suffix etc.
