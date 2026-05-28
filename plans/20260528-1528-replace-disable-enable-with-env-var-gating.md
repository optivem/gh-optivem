# Plan: Replace per-method disable/enable agents with permanent env-var gating

## Context

Today every change-system-behavior cycle pays for **two dedicated agents** (`test-disabler`, `test-enabler`) and **three BPMN call-sites** worth of orchestration just to keep WIP acceptance tests from breaking feature-branch CI between layer commits. Recent rehearsal (2026-05-28):

| # | Agent | Cost | Time |
|---|---|---|---|
| 2 | test-disabler | $0.05 | 28s |
| 4 | test-enabler | $0.04 | 23s |
| 5 | test-disabler | $0.04 | 25s |
| 7 | test-enabler | $0.04 | 25s |
| 8 | test-disabler | $0.05 | 22s |
| 9 | test-enabler | $0.04 | 22s |
| | **subtotal** | **$0.26 (5%)** | **2m25s (16%)** |

Beyond cost: each cycle edits the AT file twice per layer, adds noise to git blame (every layer commit shows `@Disabled` re-applied), and carries dispatcher complexity that earns its slot once per ticket — `renderDisableMarkerExample`, `renderDisableMarkerRemovalExample`, the `#`-prefix safety guard, the hard-fail-on-ambiguity rule in test-enabler. The whole apparatus exists to flip a single bit ("does this test run?") that the test runner already knows how to flip via an env var.

### Current call-sites

| BPMN process | Node | Line |
|---|---|---|
| `write-and-verify-acceptance-test-code` | `DISABLE_ACCEPTANCE_TESTS` (RED ends with test disabled in commit) | `process-flow.yaml:806` |
| `change-system-behavior` | `ENABLE_TESTS` (GREEN entry, uncommitted re-enable) | `process-flow.yaml:436` |
| `implement-test-layer` | `ENABLE_TESTS` + `DISABLE_TESTS` (per-layer enable→verify→disable→commit) | `process-flow.yaml:1172, 1212` |
| `disable-tests` | leaf process dispatching `test-disabler` | `process-flow.yaml:1667` |
| `enable-tests` | leaf process dispatching `test-enabler` | `process-flow.yaml:1691` |

### Proposal

The acceptance-test-writer emits the AT **with a permanent env-var gate** the first time the test is written. The gate stays in the committed code for the test's entire lifetime in the codebase — never removed by an enabler, never re-applied by a disabler. To run the test:

- The ATDD orchestrator's test-runner invocation sets `ATDD_RUN_WIP=1` (or equivalent per-language flag — see §Mechanism).
- Regular CI, local `mvn test` / `dotnet test` / `npx playwright test`, IDE runs — env var unset → the gate causes the test to be skipped silently.

The cycle becomes: writer adds gated test once, every verify step runs it under the env var, no file edits between layers, no per-stage disable/enable agents.

### Design decisions baked in (call out if you want them flipped)

1. **Gate stays permanently** — the AT is never "ungated" at ticket close. Rationale: the gate is invisible to regular CI (silent skip), so it costs nothing to leave in place; stripping it would re-introduce the file-edit-at-end-of-ticket cost we just eliminated. Trade-off: post-merge, the AT only runs when something sets `ATDD_RUN_WIP=1` — so the AT lane on main CI must opt in. If you want the AT to join regular CI after merge, add a "strip-gate-at-ticket-close" step (one final edit per ticket, still a net win vs 6 per ticket today).
2. **Env-var gating, not tag-based filtering** — env var is uniform across the three languages (JUnit `@EnabledIfEnvironmentVariable` is native; xUnit + Playwright use small idiomatic equivalents). Tag-based filtering would require per-project pom.xml / csproj / playwright.config.ts config changes that this plan would also have to propagate to the `shop` template and every scaffolded repo. Env var is a runtime decision the orchestrator makes once per dispatch.
3. **Acceptance-test-writer owns the gate emission** — not a separate "gate-tests" agent. The annotation is fixed boilerplate the writer already understands how to emit; adding a one-shot agent just for this would be churn.

## Mechanism per language

Per-test gate emitted by the writer:

| Lang | Gate annotation | Extra |
|---|---|---|
| Java (JUnit 5) | `@EnabledIfEnvironmentVariable(named = "ATDD_RUN_WIP", matches = "1", disabledReason = "ATDD WIP — set ATDD_RUN_WIP=1 to run")` above `@Test` | Add `import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;` |
| C# (xUnit) | `[Fact]` decorated with `[Trait("ATDD", "WIP")]` AND a one-line body guard: `Skip.IfNot(Environment.GetEnvironmentVariable("ATDD_RUN_WIP") == "1");` | Requires `Xunit.SkippableFact` NuGet pkg (alt: replace `[Fact]` with `[SkippableFact]`). If the package is undesirable, fall back to throwing a `SkipException` analogue or `if (...) return;` early-return (less idiomatic). |
| TypeScript (Playwright) | First line of test body: `test.skip(process.env.ATDD_RUN_WIP !== "1", "ATDD WIP — set ATDD_RUN_WIP=1 to run");` | No import change. Uses Playwright's runtime `test.skip(condition, reason)` overload (different from the definition-time `test.skip(title, body)` overload the current disabler uses). |

Each annotation is **permanent** — the writer adds it once, no subsequent agent edits it.

## Items

### Item 1 — Replace the disable-marker renderer with a gate-marker renderer

**File:** `internal/atdd/runtime/clauderun/clauderun.go`

Replace `renderDisableMarkerExample` (line 1090) with `renderGateMarkerExample` emitting the per-language annotation from §Mechanism. Output is consumed by the acceptance-test-writer prompt via a new placeholder `${gate-marker-example}` (rename from `${disable-marker-example}`).

Delete `renderDisableMarkerRemovalExample` (line 1124) entirely — no enabler to consume it.

The new helper has the same shape as the old one: returns `""` when language is empty/unrecognised; caller registers the placeholder only when non-empty so an absent value surfaces via `findUnfilledPlaceholders`.

### Item 2 — Update acceptance-test-writer to emit the gate

**File:** `internal/assets/runtime/agents/atdd/acceptance-test-writer.md`

Append a Step 3 (or fold into Step 1) instructing the writer: "For every Acceptance Test method you add, prepend the gate annotation shown below:

```
${gate-marker-example}
```

The gate is permanent — it stays in the committed code so feature-branch CI and local test runs silently skip the AT. The ATDD orchestrator sets `ATDD_RUN_WIP=1` when running verify steps, which lifts the gate for that invocation only."

Remove the existing Step-2 stub-bodies-for-`TODO: DSL`-throws scaffolding only if it's no longer relevant — leave it in place; this plan does not touch DSL-stub logic.

### Item 3 — Pass `ATDD_RUN_WIP=1` from verify-tests to the test runner

**Files:**
- `internal/atdd/runtime/statemachine/process-flow.yaml` — `verify-tests-pass` and `verify-tests-fail` processes
- The shell helpers / Go runners that exec `mvn test` / `dotnet test` / `npx playwright test`

Find the test-runner invocation that `verify-tests-pass` / `verify-tests-fail` ultimately calls. Inject `ATDD_RUN_WIP=1` into its environment. This is the only place the env var is set — every other test invocation in the system (operator-invoked, CI-invoked, IDE-invoked) leaves it unset.

Verify the runner uses `os/exec` with explicit `cmd.Env = append(os.Environ(), "ATDD_RUN_WIP=1")` (not shell-string concatenation) so the variable propagates portably on Windows + Linux.

### Item 4 — Remove ENABLE_TESTS / DISABLE_TESTS from BPMN call-sites

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

Three deletions:

**4a.** `write-and-verify-acceptance-test-code` (line 806–812 + sequence flows 838–839): delete `DISABLE_ACCEPTANCE_TESTS` node. Re-wire `VERIFY_TESTS_PASS_ACCEPTANCE → COMMIT_TEST_CODE` and `VERIFY_TESTS_FAIL_ACCEPTANCE → COMMIT_TEST_CODE` (skip the disable step).

**4b.** `change-system-behavior` (line 436–441 + sequence flow 468–469): delete `ENABLE_TESTS` node. Re-wire `WRITE_AND_VERIFY_ACCEPTANCE_TESTS_FAIL → IMPLEMENT_AND_VERIFY_SYSTEM`.

**4c.** `implement-test-layer` (lines 1172–1177 + 1212–1217 + sequence flows 1237–1238 + 1245–1246): delete both `ENABLE_TESTS` and `DISABLE_TESTS` nodes. Re-wire `RUN_ACTION → COMPILE_TESTS` and `VERIFY_TESTS_PASS_FILTERED → COMMIT_LAYER` / `VERIFY_TESTS_FAIL_FILTERED → COMMIT_LAYER`.

### Item 5 — Delete the `disable-tests` and `enable-tests` leaf processes

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

Delete the two leaf process definitions:
- `disable-tests` (line 1667–1688)
- `enable-tests` (line 1690–1712)

Run `go test ./internal/atdd/runtime/statemachine/...` to confirm no other process references them.

### Item 6 — Delete the test-disabler and test-enabler agent prompts

**Files:**
- `internal/assets/runtime/agents/atdd/test-disabler.md` — delete
- `internal/assets/runtime/agents/atdd/test-enabler.md` — delete

Update the embed/registry if it enumerates agent files explicitly (`internal/atdd/runtime/agents/embed.go` or equivalent).

### Item 7 — Remove `renderDisableMarkerRemovalExample` and its placeholder wiring

**File:** `internal/atdd/runtime/clauderun/clauderun.go`

- Delete `renderDisableMarkerRemovalExample` (covered in Item 1; this item is the cross-reference cleanup).
- Delete the `params["disable-marker-removal-example"] = ex` registration at line 839.
- Rename the line-836 registration to `params["gate-marker-example"]` per Item 1.

Run `go test ./internal/atdd/runtime/clauderun/...` — the existing `clauderun_test.go` covers the per-language render branches and will need its `disable` → `gate` rename + the removal branch deleted.

### Item 8 — Update statemachine transitions / fixtures

**Files:**
- `internal/atdd/runtime/statemachine/transitions_test.go` — drop the assertions for the removed nodes and their edges.
- `internal/atdd/runtime/statemachine/run_test.go` — any walkthrough fixture that exercises `disable-tests` / `enable-tests` needs to drop those stubbed dispatches.

Pre-flight per `feedback_statemachine_test_loop_hazard.md`: run `go test ./internal/atdd/runtime/statemachine/ -p 2 -timeout 60s` first.

### Item 9 — Update language-equivalents docs and architecture diagram

**Files:**
- `docs/atdd/code/language-equivalents.md` (or equivalent — verify path before editing): drop the `@Disabled` / `[Fact(Skip=...)]` / `test.skip(...)` row; add a "WIP gate" row with the new per-language annotation.
- Architecture YAML at `internal/atdd/runtime/architecture/architecture.yaml`: remove the `test-disabler` and `test-enabler` nodes and any incoming edges. Regeneration is handled by CI per `feedback_never_edit_generated_diagrams.md` — do not run the diagram regen locally.

### Item 10 — Rehearse on a real ticket

After all edits land:
- `bash scripts/atdd-rehearsal.sh <issue> --config gh-optivem-monolith-java.yaml` against a fresh ticket that exercises full RED→DSL→adapter→GREEN.
- Confirm: no `test-disabler` / `test-enabler` dispatches appear in the rehearsal log, the AT is committed with the gate annotation in every layer commit, verify steps successfully run the gated test under `ATDD_RUN_WIP=1`, and the AT is silently skipped when running `mvn test` directly in the rehearsal worktree.
- Repeat for `gh-optivem-monolith-csharp.yaml` and `gh-optivem-monolith-typescript.yaml`.

## Out of scope

- **Stripping the gate at ticket close.** Recommended permanent gate (see "Design decisions baked in" §1). If you want the AT to join regular post-merge CI without an opt-in env var, add a Item 11 that introduces a `strip-gate` step at the end of `change-system-behavior` (after REFACTOR_OPPORTUNISTICALLY) — one file edit per ticket, still net win vs 6 today.
- **Tag-based filtering instead of env-var gating.** See "Design decisions baked in" §2 for rationale.
- **CT-side gating.** The forward-looking `[at-test, ct-test]` symmetry in the current `disable-tests` / `enable-tests` leaf processes was never exercised (reason-format hardcodes `AT`). The new gate annotation applies only to AT methods the writer authors; if CT-side gating is needed later, the writer for CTs can reuse `renderGateMarkerExample`.
- **`Xunit.SkippableFact` NuGet adoption.** §Mechanism's xUnit row assumes it. If the workspace currently has no SkippableFact reference, the C# row needs a fallback choice — flag during refinement; not blocking for Java + TS rollout.
- **Diagram regeneration.** Auto-regenerated by CI (`regenerate-diagram` workflow) per `feedback_plans_no_diagram_regen.md`.

## Verification

- `go test ./internal/atdd/runtime/clauderun/...` and `go test ./internal/atdd/runtime/statemachine/...` pass.
- `go test ./internal/atdd/... -p 2` passes.
- Rehearsal (Item 10) shows no disable/enable dispatches and a green AT under the orchestrator + a skipped AT under direct `mvn test`.
- `find . -name '*.md' -path '*/agents/atdd/*' | xargs grep -l 'test-disabler\|test-enabler'` returns no matches (all references removed).
