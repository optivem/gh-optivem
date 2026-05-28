# Plan: Layer-qualify BPMN commit messages

## Context

Spun off from the BPMN commit-message binding work (shipped in commits `c6582b9` and `d0e0b00`; plan file `plans/20260527-1052-bpmn-commit-message-binding.md` since deleted per the "delete completed plans" convention). That work centralised a single uniform message — `#${ticket-id} ${issue-title}` — inside the `commit:` subprocess at `internal/atdd/runtime/statemachine/process-flow.yaml:2005`, so the rehearsal stops failing on `commit message is required`.

Goal here: enrich the messages so an operator scanning `git log` on a rehearsal branch can tell which BPMN layer produced each commit, using the canonical layer vocabulary from `system-test.paths:`.

## Discussion outcomes (2026-05-28)

The original draft proposed a `… - ${suite} ${cycle_phase} layer` suffix bound at each of four call sites. A chat session and follow-up discussion locked the following:

| Decision | Choice | Why |
|---|---|---|
| Commit format | `#${ticket-id} ${issue-title} - LAYER` (existing `#` prefix, ALL-CAPS layer tag) | `#` preserves GitHub auto-linking (per d0e0b00); ALL-CAPS gives a strong `rg` token and reads as a tag |
| Vocabulary | Canonical layer names from `system-test.paths:` (see [internal/projectconfig/path-keys.md](../internal/projectconfig/path-keys.md) §"Family B — named locations") | The commit suffix names the same layer the BPMN's per-phase `write:` scope already targets — two views of the same noun |
| Pluralisation | Drop suffix when single-instance; plural when the layer holds multiple concrete artifacts; plural for cross-layer umbrella | `DSL`, `SYSTEM` singular; `ACCEPTANCE TESTS`, `SYSTEM DRIVER ADAPTERS`, `EXTERNAL DRIVER ADAPTERS`, `TESTS` plural |
| Wiring | Subprocess constructs `message: "#${ticket-id} ${issue-title} - ${layer}"`; each call site binds `layer:` | Preserves d0e0b00's single-source format; strict-mode `ExpandParams` catches missed bindings naturally |
| Param name | `layer` (bare noun) | Matches `suite` / `category` / `action` convention; avoids overloading "phase" (already used by `internal/atdd/phase_scopes.go` for BPMN execution-phase machinery) |
| Grain | Per-call-site (today's 4 BPMN commit sites) | Matches the "main target layer" framing; per-dispatch would 2-3× per-feature commit count without adding information |
| Behavioral/structural axis | Dropped | The suffix already carries the signal — `- TESTS` reads as structural, `- SYSTEM` reads as behavioral |
| Inline-bracket shape (e.g. `#69 [Impl DSL] Add …`) | Rejected | Trailing suffix reads more naturally as English |

### Locked end-state for feature `#69`

```
abc1234 #69 Add product search - ACCEPTANCE TESTS
def5678 #69 Add product search - DSL
ghi9abc #69 Add product search - SYSTEM DRIVER ADAPTERS
jkl0def #69 Add product search - EXTERNAL DRIVER ADAPTERS
pqr5678 #69 Add product search - SYSTEM
stu9abc #69 Add product search - TESTS
```

### Mapping to BPMN sites

| BPMN call site | Layer label | Canonical Family B / system-path key(s) |
|---|---|---|
| `COMMIT_TEST_CODE` (process-flow.yaml:813) | `ACCEPTANCE TESTS` | `at-test` |
| `COMMIT_LAYER` (process-flow.yaml:1213) | `DSL` / `SYSTEM DRIVER ADAPTERS` / `EXTERNAL DRIVER ADAPTERS` | `dsl-port` + `dsl-core` / `driver-port` + `driver-adapter` / `external-system-driver-port` + `external-system-driver-adapter` |
| `COMMIT_SYSTEM` (process-flow.yaml:1078) | `SYSTEM` | `system-path` |
| `COMMIT_TESTS` (process-flow.yaml:1131) | `TESTS` | cross-layer (umbrella over the test stack) |

## Items

### Item 1 — Add `${layer}` to the commit subprocess message format

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

At the `commit:` subprocess (~:1995-2009), extend the message format to splice `${layer}`:

```yaml
commit:
  name: "Commit"
  start: EXECUTE_COMMAND
  nodes:
    - id: EXECUTE_COMMAND
      type: call-activity
      process: execute-command
      name: "Dispatch the Command"
      params:
        command: "gh optivem commit --yes --include-untracked"
        message: "#${ticket-id} ${issue-title} - ${layer}"
        category: ${category}
```

The strict-mode `ExpandParams` rule (already in place for `${message}` per d0e0b00) means any call site that forgets `layer:` fails fast at dispatch with a precise error, not as a literal-leak in the rendered CLI.

### Item 2 — Bind `layer:` at the three fixed call sites

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

```yaml
# COMMIT_TEST_CODE (~:813)
params:
  category: test-commit
  layer: "ACCEPTANCE TESTS"

# COMMIT_SYSTEM (~:1078)
params:
  category: prod-commit
  layer: "SYSTEM"

# COMMIT_TESTS (~:1131)
params:
  category: test-commit
  layer: "TESTS"
```

### Item 3 — Thread `layer:` through `implement-test-layer` for `COMMIT_LAYER`

`COMMIT_LAYER` (process-flow.yaml:1213) lives inside the shared `implement-test-layer` subprocess. Three parents call that subprocess with different target layers; each parent binds `layer:` as a literal, and `COMMIT_LAYER` passes it through to the `commit:` subprocess.

**File:** `internal/atdd/runtime/statemachine/process-flow.yaml`

```yaml
# implement-and-verify-dsl (~:841) — IMPLEMENT_TEST_LAYER node at ~:845
params:
  task-name: implement-dsl
  action: implement-dsl
  expected-test-result: ${expected-test-result}
  tests: ${tests}
  layer: "DSL"

# implement-and-verify-system-driver-adapters (~:862) — IMPLEMENT_TEST_LAYER node at ~:866
params:
  task-name: implement-system-driver-adapters
  action: implement-system-driver-adapters
  expected-test-result: ${expected-test-result}
  tests: ${tests}
  layer: "SYSTEM DRIVER ADAPTERS"

# implement-and-verify-external-system-driver-adapters (~:883) — IMPLEMENT_TEST_LAYER node at ~:887
params:
  task-name: implement-external-system-driver-adapters
  action: implement-external-system-driver-adapters
  expected-test-result: ${expected-test-result}
  tests: ${tests}
  layer: "EXTERNAL DRIVER ADAPTERS"

# COMMIT_LAYER (~:1213) — pass-through from implement-test-layer scope
params:
  category: test-commit
  layer: ${layer}
```

`${layer}` reaches `COMMIT_LAYER` via the same scope-flow `${suite}` / `${test-names}` already use inside `implement-test-layer` (process-flow.yaml:1195, 1203, 1075-1076).

### Item 4 — Tests

**File:** `internal/atdd/runtime/actions/bindings_test.go`

The four existing `runCommand` binding cases from c6582b9 (splice, escape, no-message no-op, non-commit ignored) should pass unchanged — the message string changes but the binding mechanism doesn't. If a fixture pins the exact dispatched message literal, update it to the suffixed form: `#69 Add product search - ACCEPTANCE TESTS`.

**File:** `internal/atdd/runtime/statemachine/run_test.go`

Mirror `TestStrictExpand_MissingMessageBindingFailsFast` (the regression for unbound `${message}`) with a `TestStrictExpand_MissingLayerBindingFailsFast` for unbound `${layer}`. Same shape, swap the param name.

**File:** `internal/atdd/runtime/release/release_test.go`

`TestCommit_ConfirmTrueRunsAddAndCommit` (line 79-98) uses a third pre-existing message shape (`#42 | Register Customer | AT - GREEN - SYSTEM`). That's a `Commit()` unit test asserting message round-trip, not BPMN integration — **leave it alone**.

### Item 5 — Re-run the rehearsal

`bash scripts/atdd-rehearsal.sh 69 --config gh-optivem-monolith-typescript.yaml` and inspect `git log` on the rehearsal branch. Expect six layer-qualified commits per feature in BPMN-natural order:

```
#69 Add product search - ACCEPTANCE TESTS
#69 Add product search - DSL
#69 Add product search - SYSTEM DRIVER ADAPTERS
#69 Add product search - EXTERNAL DRIVER ADAPTERS
#69 Add product search - SYSTEM
#69 Add product search - TESTS
```

Layer-impl commits may repeat if the cycle runs multiple iterations of the same layer (e.g. a DSL re-impl after a port change).

## Out of scope

- **`CONTRACT TESTS` label.** `write-contract-tests` (process-flow.yaml:1376) has no commit site today, and the whole contract-test cascade is explicitly marked as a structural-call-graph orphan (process-flow.yaml:909, Q31.a / Phase D). When that gap is closed, a fifth label (`CONTRACT TESTS`) will need to be added using the same `layer:` mechanism. Not this plan.
- **Writing-agent-authored free-form messages.** Letting agents emit a `commit-message` output is a richer change tracked separately.
- **Branch / PR title alignment.** Different convention, different layer; not this plan.
- **Per-dispatch commit grain.** Considered and rejected — would 2-3× per-feature commit count without adding information beyond the layer label.

## Verification

- `go test ./internal/atdd/...` passes (use `-p 2` per Windows test memory).
- `git log --oneline` on a fresh rehearsal branch shows six layer-qualified commits per feature in BPMN order.
- `rg '^#\d+ [^-]+ - ACCEPTANCE TESTS$'` (and the other five labels) matches the expected number of commits per feature.
