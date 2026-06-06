# Plan: harden verify so a bad suite name can't spin the fixer for hours

## Why

Rehearsal-log analysis (10 runs, issues #69/#71/#72, 2026-06-04 → 06-06) found that
agent *selection and model tiering are efficient* — the waste was concentrated in **one
agent, `unexpected-failing-tests-fixer`**, which burned almost all the lost wall-clock:

- `rehearsal-20260606-103046-72…` — fixer ran **2h27m26s → `no changes`** (produced nothing).
- `rehearsal-20260604-165322-72…` — fixer hung with no exit.
- `rehearsal-20260604-102555-69…` — fixer 5m32s + 3m22s (second → `no changes`).
- A clean run (`rehearsal-20260606-132208-72…`) fixed 5 files in **7m20s** — proving the
  agent itself is fine *when fed a real signal*.

### Root cause (a three-link chain, not a slow agent)

The 103046 trace (lines 2385–2471) shows the whole failure:

```
VERIFY_TESTS_PASS_FILTERED  params=suite=contract,test-names=shouldBeAbleToGetProductWeight
RUN_COMMAND -> command-exit-code=1, command-succeeded=false, failure-kind=command-failed,
  command-line=gh optivem test run --suite=contract --test=shouldBeAbleToGetProductWeight
  command-stderr-tail=ERROR: suite(s) not found: contract.
    Available: … contract-stub, contract-stub-isolated, contract-real, …
GATE_TESTS_OUTCOME → FIX_UNEXPECTED_FAILING_TESTS   (test-outcome == fail)
EXECUTE_AGENT agent=unexpected-failing-tests-fixer, category=human
APPROVE_PRE -> verdict=unexpected-fail  (12m0.154s)   ← human-gate stall, unattended
RUN_AGENT  unexpected-failing-tests-fixer (interactive)  (2h27m26.962s)  ← then spins, no changes
```

1. **Bad suite name reaches the runner.** The verify ran `--suite=contract`, but `contract`
   is not a registered suite *or* group — `testselect.ExpandSuiteGroups` only knows
   `acceptance` as a default group (`internal/atdd/runtime/testselect/suite.go:28`), so
   `contract` passes through unchanged and `runner.selectSuites` rejects it
   (`internal/runner/tests.go:133`, `"suite(s) not found: %s. Available: %s"`). This is
   channel-split drift: the taxonomy gained `-stub`/`-real`/`-api`/`-ui`, but a verify call
   site emitted the bare group name.
2. **A command that never ran a test is mis-classified as a red test.** `runCommand`
   (`internal/atdd/runtime/actions/bindings.go:838-879`) sets `test-outcome=fail` on any
   non-zero `gh optivem test run`, then asks `classifyShellErr` whether it was infra. The
   `infraPatterns` table (`internal/atdd/runtime/actions/verify_classify.go:71-100`) has **no
   row for `suite(s) not found`**, so it falls through to `classRed` and stays
   `test-outcome=fail`.
3. **`fail` dispatches the fixer on a non-signal.** `GATE_TESTS_OUTCOME`
   (`process-flow.yaml:1311-1314`) routes `fail → FIX_UNEXPECTED_FAILING_TESTS`. The fixer is
   handed `verify_failure_output = "ERROR: suite(s) not found: contract"` — not a compile
   error, not a red assertion, nothing in its read/write scope to fix (cf.
   `unexpected-failing-tests-fixer.md:5,15,27`). It flails for hours and exits `no changes`.

### What is already in place (and why it didn't save these runs)

- The **suite-existence preflight** (`runSuiteExistenceChecks`, commit `3cd9df1`) landed
  **2026-06-06 14:18** — *after* every rehearsal here. None of these runs had it. It would
  have caught a *renamed/missing declared* suite at startup.
- The **infra-halt path already exists**: `test-outcome == infra → TESTS_INFRA_HALT`
  (`process-flow.yaml:1313`), fed by `classifyShellErr`. The mechanism is right; the
  `suite(s) not found` pattern is just not in the table yet.

So this plan does not build new machinery — it closes two gaps in existing machinery and
defines one unattended-mode policy.

## Items

### 1. Preflight: validate the *effective* suite at each verify node, not just `suite:` literals

`collectSuiteLiterals` (`internal/atdd/runtime/preflight/preflight.go:322`) sweeps explicit
`suite:` params and **deliberately skips `${…}` placeholders and omitted overrides** (see its
own comment: placeholder values "originate at the literal call site the sweep already sees").
That assumption is exactly what let `contract` through: the value the runner saw was not a
plain `suite: contract` literal — it resolved from the node's fallback/placeholder, which the
sweep does not evaluate.

Extend the preflight to **double-validate**: keep the existing literal sweep *and* add a sweep
of the *effective* suite each `verify-tests-*` call site will actually emit — resolve the same
`suite:`-else-`tests:` fallback the runtime uses (`bindings.go:808` reads `ctx.Params["suite"]`
via ExpandParams's state fallback), run it through group expansion, and assert every resulting
id is declared in `tests.yaml`. An ungrouped name like `contract` that expands to itself and is
absent from `tests.yaml` must fail preflight.

**No duplicated resolution logic — this is a hard constraint.** A validator that re-derives
"effective suite" with its own copy of the fallback/expansion rule will drift from the runtime,
which is the exact bug class this plan fixes. So:

- Resolve through the **same two primitives the runtime uses**: `statemachine.ExpandParams`
  (`run.go:345`, placeholder + state-fallback) and `testselect.ExpandSuiteGroups`
  (`testselect/suite.go:47`, group expansion). No parallel fallback rule in the preflight.
- Make the reuse **structural, not coincidental**: if the `suite:`-else-`tests:` →
  `ExpandSuiteGroups` resolution is not already a single named helper, extract it into one
  (e.g. `EffectiveSuites(params, state, projectGroups)`), and have **both** the runtime emit
  path (`bindings.go` `runCommand`, where `--suite=` is appended) **and** the preflight call
  it. One function = the validator literally checks the value the runtime will emit, so the
  two cannot diverge.
- The only preflight-specific code is gathering each verify node's **static param scope** from
  the engine graph — which `collectSuiteLiterals` (`preflight.go:328`) already half-does by
  walking nodes. Thread the call-activity param env down so `ExpandParams` resolves what is
  statically knowable.
- **Consolidate an existing divergence** as part of this item, don't add a third copy:
  `preflight.expandSuiteLiteral` (`preflight.go:359`) hand-builds `acceptance-<ch>` /
  `acceptance-isolated-<ch>` from `cfg.Channels` (channel-derived), while
  `testselect.AcceptanceSuites` (`testselect/suite.go`) returns a **fixed** `{api,ui}` list used
  by the CLI and the runtime's BPMN emission. These disagree for any non-`{api,ui}` channel set.
  Reconcile to a single channel-aware source so preflight and runtime expand `acceptance`
  identically.
- If a verify node's suite is **not statically resolvable** (a `${…}` that resolves only from
  runtime state a prior agent stamps), make *that* a preflight failure — do **not** simulate the
  runtime to guess the value. Safer contract, and zero new resolution code.

- Touches: `internal/atdd/runtime/preflight/preflight.go` (`collectSuiteLiterals` /
  `runSuiteExistenceChecks` / `expandSuiteLiteral`), and tests in `preflight_test.go`.
- Add a regression case: a verify node whose effective suite is an ungrouped/unknown name →
  preflight returns the existing `"… required by the ATDD process flow but not declared …"`
  failure line (so the 103046 disaster becomes a sub-second startup error).

### 2. Classify `suite(s) not found` (and `gh optivem` CLI usage errors) as infra, not red

Add a row to `infraPatterns` (`internal/atdd/runtime/actions/verify_classify.go:71-100`)
matching the runner's fixed prefix `suite(s) not found:` (emitted at `internal/runner/tests.go:133`),
labelled e.g. `"unknown test suite"`. Per the table's own guidance, match the runner prefix,
not the OS suffix. Consider a companion row for the orchestrator's own arg-rejection errors
(`unknown flag`, `unknown command`) so any `gh optivem test run` invocation that never starts a
test routes to `classInfra` → `test-outcome=infra` → the existing `TESTS_INFRA_HALT`.

- Touches: `internal/atdd/runtime/actions/verify_classify.go` and `verify_classify_test.go`
  (add a table case asserting `"ERROR: suite(s) not found: contract. Available: …"` →
  `classInfra` with the new label).
- No gateway change needed — `infra → TESTS_INFRA_HALT` already exists.

### (dropped) Unattended human-gate policy — redundant

An earlier draft proposed a third item: under `--auto --headless`, hard-stop at the fixer's
human gate instead of blocking. **Dropped — the mechanism already exists.** The fixer is
`category: human` (`process-flow.yaml:1748`), so its `APPROVE_PRE` already stops and asks a
human; that *is* "surface for review." The 12-min stall and the 2h27m spin were not a missing
policy — they were downstream of the **spurious dispatch** that items 1 & 2 remove. Once a bad
suite can no longer reach the runner, the fixer only fires on a real failure, where a human
approving an interactive session is the intended design. With a wall-clock kill explicitly
rejected, there is no software change left for this item to make.

## Verification

- `go build ./...`
- `go test ./internal/atdd/runtime/preflight/... ./internal/atdd/runtime/actions/... ./internal/runner/...`
  via `scripts/test.sh` or `-p 2` — never unbounded `go test ./...` on Windows.
- Rehearsal re-run of the #72 contract path (the one that produced the 2h27m spin) under
  `--auto --headless`: confirm it now either (a) fails at preflight naming the bad suite, or
  (b) if a bad suite still reaches the runner, halts at `TESTS_INFRA_HALT` in seconds — and in
  neither case dispatches `unexpected-failing-tests-fixer`.
- Confirm a *genuine* red test still routes `fail → FIX_UNEXPECTED_FAILING_TESTS` (item 2 must
  not over-classify real assertion failures as infra).

## Concurrency note

At plan-authoring time the working tree has uncommitted edits to
`internal/atdd/runtime/statemachine/process-flow.yaml`, `internal/atdd/runtime/driver/driver.go`,
and `internal/atdd/runtime/actions/bindings_test.go` (likely the in-flight
`20260606-1410-decide-where-acceptance-isolation-is-set` work). Items 1–3 touch
`preflight.go`, `verify_classify.go`, and the dispatcher — re-check `git status` and re-read
those three files before editing, in case the line anchors above have shifted.

## Out of scope

- Reworking the `unexpected-failing-tests-fixer` agent body — it is correct when fed a real
  failure (the 7m20s clean run proves it).
- Any wall-clock / no-progress kill timer on agents — explicitly rejected; long *attended*
  runtime is legitimate, and the fixer's existing `category: human` gate already surfaces for
  review (see the dropped item above).
- Registering `contract`/`e2e` as default suite groups in Go — that is a separate taxonomy
  decision; item 1 catches the gap regardless of whether the group is added.
- The cost outlier (`dsl-implementer` $3.51 / 4.0M input tokens on `rehearsal-20260604-102555`)
  — noted in the analysis, not part of this fixer-spin fix.
