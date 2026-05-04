# Minimal-but-safe test set after a driver-adapter change

## Problem

After an ATDD driver agent runs (`AT - RED - SYSTEM DRIVER - WRITE`, `CT - RED - EXTERNAL DRIVER - WRITE`, and any later phase that touches `driver-adapter/**`), the orchestrator asks the user whether to run tests for local verification. Today the only choices are:

- Run nothing — fast, but no proof the just-written adapter even compiles, let alone behaves.
- Run the full acceptance/contract suite — safe, but slow enough that students skip it, and the feedback loop is long enough that the agent's output goes stale before a failure is observed.

We want a third option: run **the smallest set of tests that is still regression-safe** — i.e. every test that traverses any changed adapter method, and nothing else.

## Key insight

The phrase "minimum set of tests that proves the adapter is traversed" is misleading. There are two candidate sets:

- **Traversal-proof set:** one test per changed adapter method. Proves the new path executes. Does **not** give regression assurance — a *different* test going through the same method could fail in a way the chosen one doesn't catch.
- **Affected set:** every test that traverses any changed adapter method. By construction, no test outside this set can be broken by the change (it doesn't touch the changed code), so the affected set is **simultaneously** the minimum that is regression-safe AND covers traversal of every changed method.

There is no real tradeoff: the affected set is what we want. "One per method" should be dropped from the framing.

## Architectural enabler

ATDD enforces a strict layering: `Test → DSL → DriverPort → DriverAdapter`. The `driver-port.md` rules forbid the DSL from calling adapters directly; adapters only ever satisfy port interfaces. This makes static caller analysis tractable, and the language-specific surface narrow enough that simple regex grep suffices for Java, .NET, and TypeScript:

1. Diff `driver-adapter/**` against the base ref → list of changed method names per file.
2. The matching `driver-port/**` interface method has the same name (adapters implement ports — `@Override`, `: IPort`, `implements IPort`).
3. Grep `dsl-core/**/usecase/**` for callers of those port methods → DSL methods.
4. Follow DSL → DSL helper edges transitively (a DSL method can call another DSL helper).
5. Grep test sources for callers of the affected DSL methods → test method names.
6. Group by suite tag (`@Channel(API)`, `@Channel(UI)`, contract real/stub) so each result feeds into the right `gh optivem test system --suite <suite> --test <name>` invocation.

False positives (an unrelated `.foo(` matching the regex) cost extra test runs — still safe. False negatives surface as **unmapped** changed methods (no DSL caller found), which the orchestrator handles by falling back to the full suite. Either way, regression safety is preserved.

## Approach

Add a new internal Go package `internal/atdd/runtime/testselect/` exposing one function the state machine can call from a verification node. The state machine, after the driver agent exits and the user confirms "yes, run tests", calls the function, decides whether to run the selected list or fall back, then invokes the existing test runner.

Selector signature:

```go
package testselect

type Selection struct {
    Suite    string   // "acceptance-api" | "acceptance-ui" | "contract-stub" | "contract-real"
    Tests    []string // fully-qualified or class-qualified test method names, sorted, deduplicated
}

type Result struct {
    Selections []Selection
    Unmapped   []ChangedMethod // changed adapter methods with no DSL caller — fall back to full suite
    Diagnostics []string       // human-readable trace of decisions, for the verification node to print
}

type ChangedMethod struct {
    File    string
    Method  string
    Layer   string // "shop" | "external"  (drives which suites are candidates)
}

func Select(repoRoot, baseRef string) (Result, error)
```

State-machine integration is a single new node, slotted after the driver agent's `*-WRITE` phases, that:

1. Prompts the user via the existing prompt mechanism: "Run tests for local verification? [y/N]".
2. On `n` / non-TTY default: log "skipped" and continue.
3. On `y`: call `testselect.Select(repoRoot, baseRef)`.
4. If `Result.Unmapped` is non-empty: print the unmapped methods, log "falling back to full suite", run all suites named by `Result.Selections[*].Suite` in their entirety.
5. If `Result.Unmapped` is empty: for each `Selection`, run `gh optivem test system --suite <s> --test <t>` (or the in-process equivalent) for each test name.
6. Surface pass/fail back to the user; the state machine continues regardless (the test result is informational at this stage — phase progression is gated separately).

The selector itself does no shell calls, no test execution, no LLM dispatch. It is pure file-system reads + regex.

## Items

### 1. New package: `internal/atdd/runtime/testselect/`

**Files (new):**
- `internal/atdd/runtime/testselect/testselect.go` — public `Select` entry point + `Result` / `Selection` / `ChangedMethod` types.
- `internal/atdd/runtime/testselect/diff.go` — diff parsing: `parseChangedMethods(repoRoot, baseRef string) ([]ChangedMethod, error)`. Shells out to `git diff --unified=0 <baseRef>...HEAD -- 'driver-adapter/**'`, intersects hunk line ranges with method-signature regions detected per language.
- `internal/atdd/runtime/testselect/grep.go` — regex search helpers: `findCallers(roots []string, methodName string) ([]Caller, error)`. Walks file trees; skips vendored / build directories.
- `internal/atdd/runtime/testselect/layout.go` — repo-layout discovery: where is `driver-port/`, `driver-adapter/`, `dsl-core/.../usecase/`, and the test-source root for each language present? Encoded as a small per-language config (Java / .NET / TypeScript), matching the conventions in `language-equivalents.md`.
- `internal/atdd/runtime/testselect/suite.go` — suite tagging: from a test method's source file, detect `@Channel(API)`, `@Channel(UI)`, contract-test conventions, and map to the suite placeholder used by `at-cycle-conventions.md` / `ct-cycle-conventions.md`.

**Public surface:** only `Select(repoRoot, baseRef string) (Result, error)` and the result types. Everything else is package-private.

### 2. Method-signature regexes per language

**File:** `internal/atdd/runtime/testselect/layout.go`.

Per-language definitions, in one place:

- **Java:** signature regex `^\s*(public|protected|private|@Override\s+)*\s*(static\s+)?\S+\s+(\w+)\s*\(` — capture group `\3` is the method name. Caller regex per method: `\.<methodName>\s*\(`.
- **.NET (C#):** signature regex similar, accounting for `public override` / `public async Task`. Caller regex: `\.<methodName>\s*\(`.
- **TypeScript:** signature regex `^\s*(public\s+|private\s+|protected\s+|async\s+)*(\w+)\s*\(` (TS class method bodies). Caller regex: `\.<methodName>\s*\(`.

Conservative on purpose. The grep returns line-level matches; the consumer counts unique test method names containing any match.

### 3. Diff → changed adapter methods

**File:** `internal/atdd/runtime/testselect/diff.go`.

- Run `git diff --unified=0 <baseRef>...HEAD -- '*driver-adapter*'` (the glob picks up any path containing `driver-adapter`, which is how the convention names them).
- Parse hunk headers (`@@ -a,b +c,d @@`) → list of `(file, addedLineRange)` tuples.
- For each touched file, scan the file's full text once for method-signature regions: each match starts at the signature line, ends at the matching `}` (depth-tracked), gives `(methodName, startLine, endLine)`.
- Intersect added line ranges with method regions → set of changed method names per file.
- Determine `Layer` for each changed method by path: `external/` → `"external"`, otherwise `"shop"`. Drives which suites are candidates downstream.

Edge cases:
- A method whose **signature** changed (renamed, parameter added) shows up as the new name only — the old name is gone from HEAD. That is intentional: tests calling the old name would no longer compile, so they aren't candidates.
- A whole new file added under `driver-adapter/` → all its methods are changed.
- A file moved from `shop/` to `external/` → diff shows it as delete + add; we pick up the add side and treat the methods as new.

### 4. Port lookup

**File:** `internal/atdd/runtime/testselect/testselect.go`.

For each `ChangedMethod`, find the matching port method by name lookup under `driver-port/**` (constrained to the same `Layer`). The matching is by name only — the architecture forbids name collisions across ports inside the same layer (each port serves one use case), and the port file path mirrors the adapter file path.

If no port method is found → mark the adapter method as **unmapped** (no DSL is reachable through a port that doesn't exist; this signals a private adapter helper or a stale adapter file). Goes into `Result.Unmapped`.

### 5. DSL caller traversal

**File:** `internal/atdd/runtime/testselect/testselect.go`.

For each port method, grep `dsl-core/**/usecase/**` for `\.<portMethodName>\s*\(` → list of source file + line. Map each match back to the enclosing DSL method by walking the file's method regions (same logic as Item 3).

Then transitively: any **other** DSL method that calls a DSL method already in the set is also affected. Iterate to fixed point. This catches DSL helpers that wrap port calls.

If no DSL caller is found for a port method → mark as **unmapped**. (A port that no DSL talks to is an authoring bug; the orchestrator should fall back to the full suite and the warning surfaces the bug.)

### 6. Test caller search

**File:** `internal/atdd/runtime/testselect/testselect.go`.

For each affected DSL method, grep test source roots for `\.<dslMethodName>\s*\(`. Test source roots come from the per-language layout config (Item 2): typically `src/test/java/` (Java), `**/*Tests/` (.NET), `tests/` or `**/*.spec.ts` (TS).

Map each match back to the enclosing test method (`@Test` in Java, `[Fact]` / `[Theory]` in .NET, `test(` / `it(` in TS). Detect using per-language test-method regexes maintained alongside the signature regex.

Deduplicate test names across all DSL methods → final set per layer.

### 7. Suite tagging

**File:** `internal/atdd/runtime/testselect/suite.go`.

For each test method:

- **Layer = `shop`:** look at the enclosing class (or file) for `@Channel(API)` / `@Channel(UI)`. Map to `acceptance-api` / `acceptance-ui` per `at-cycle-conventions.md`. If both annotations are present → emit the test under both suites.
- **Layer = `external`:** contract tests run against both real and stub by `ct-cycle-conventions.md`. After a `CT - RED - EXTERNAL DRIVER` change, the WRITE phase runs only the stub side (the prompt says so), so emit under `contract-stub` only. The orchestrator can override per the calling phase if needed.
- **Untagged tests:** emit a diagnostic and fall back to "all suites for this layer". They will run, but not optimally targeted.

### 8. State-machine verification node

**Files:**
- `internal/atdd/runtime/actions/bindings.go` — register a new action, e.g. `verify_run_tests_after_driver`.
- `internal/atdd/runtime/statemachine/embed.go` (and the YAML it embeds) — add a `service_task` node after each `*_DRIVER_WRITE` user task, before the existing REVIEW STOP.

The action does the prompt-and-dispatch flow described in **Approach** above. It calls `testselect.Select`, branches on `Unmapped`, and runs tests. Test execution itself reuses whatever path `gh optivem test system` already takes — the verification node is a thin wrapper over it; the selector decides *what* to run, not *how*.

Open question on placement: WRITE → REVIEW (STOP) is already a hard barrier for human approval. The verification step is feedback, not gating. Two slot options:

- **Before REVIEW** — verification runs automatically; user sees results when reviewing.
- **Inside REVIEW prompt** — REVIEW asks "review and choose: approve / run tests then approve / reject"; "run tests" path triggers the selector.

Lean: inside REVIEW, because the user might want to read the diff before deciding to spend time on tests. Item 11 below covers this UX choice.

### 9. Tests for the selector itself

**Files (new):**
- `internal/atdd/runtime/testselect/testselect_test.go` — table-driven tests against fixture repos under `internal/atdd/runtime/testselect/testdata/`.
- `internal/atdd/runtime/testselect/testdata/<lang>/...` — minimal Java / .NET / TS fixtures with a known DSL → port → adapter chain and known test methods.

Cases to cover:

- **Happy path, single method changed:** one adapter method changes, one DSL method calls it, two test methods exercise that DSL method → both test names returned, in the right suite.
- **Happy path, multiple methods changed, overlapping callers:** two adapter methods change; one test method exercises both → returned once (deduplicated).
- **Transitive DSL helper:** adapter method called by DSL helper A, helper A called by DSL method B, two tests call B → both tests returned.
- **Unmapped adapter method (no port):** changed method has no port match → `Result.Unmapped` non-empty, `Selections` for the unrelated mapped methods is still produced.
- **Unmapped port (no DSL caller):** port exists but no DSL talks to it → `Result.Unmapped` non-empty.
- **Untagged test (no `@Channel`):** test ends up in a diagnostic + a "fallback to all suites for this layer" entry rather than silently dropped.
- **Both channels:** test annotated with both `@Channel(API)` and `@Channel(UI)` appears in both selections.
- **Contract layer:** changed method under `external/` → only `contract-stub` selection (per CT phase contract).
- **No changes:** `Select` against a clean tree → empty `Result`, no error.
- **Renamed adapter method:** old-name DSL/test references are not in HEAD anymore; new-name flow runs as normal. Existing call sites that haven't been updated are a compile error and are out of this selector's concern.

Per language: at least one full chain fixture each for Java, .NET, TypeScript. Same scenarios per language so behaviour parity is provable.

### 10. Documentation

**Files:**
- `docs/atdd/process/at-cycle-conventions.md` — add a short section noting that the orchestrator runs a targeted subset of acceptance tests after the SYSTEM DRIVER WRITE phase, not the whole suite, and that an unmapped change triggers a full-suite fallback.
- `docs/atdd/process/ct-cycle-conventions.md` — mirror for contract tests.
- `internal/atdd/runtime/agents/prompts/atdd-driver.md` — remove the inline `gh optivem test system --test <TestMethodName>` instruction (the agent no longer runs tests; the orchestrator does), or replace with "tests are run by the orchestrator after you exit; do not run them yourself".

### 11. UX for the post-WRITE prompt

**File:** wherever the existing user-prompt mechanism lives (likely a helper in `internal/atdd/runtime/`).

Single prompt covers approve + verify in one place:

```
AT - RED - SYSTEM DRIVER - WRITE complete.

Selected tests for verification (3):
  acceptance-api: RegisterCustomerPositiveTest.shouldRegister
  acceptance-api: RegisterCustomerNegativeTest.shouldRejectMissingEmail
  acceptance-ui:  RegisterCustomerUiTest.shouldShowSuccess

  [r] run selected tests now
  [a] approve without running
  [x] reject and stop
  [f] run full suite instead of selected
```

`f` is the "I don't trust the selector for this change" escape hatch. Same path the unmapped-fallback takes internally.

### 12. Logging and diagnostics

**File:** `internal/atdd/runtime/testselect/testselect.go`.

`Result.Diagnostics` collects human-readable lines like:
- `port method "register" → DSL methods [registerCustomer, registerCustomerWithDefaults]`
- `DSL method "registerCustomer" → tests [RegisterCustomerPositiveTest.shouldRegister, RegisterCustomerNegativeTest.shouldRejectMissingEmail]`
- `unmapped: adapter method "internalDebugDump" — no port match`

Printed by the verification node when verbose output is requested (a config flag, defaulting off — students see the test list, instructors troubleshooting selection see the trace).

## Open questions

- **Base ref to diff against.** Driver phases produce a single commit at COMMIT, not at WRITE — at WRITE, the change is uncommitted. So the selector should diff against `HEAD` (previous commit), not against a base branch. Confirm: is there ever a case where the driver agent produces multiple commits before exiting (and we'd diff against the merge-base of the issue branch)? If so, the base ref must come from the orchestrator, not a hard-coded `HEAD`.
  - **Lean:** parameterise (`baseRef` argument), have the orchestrator pass the correct value per phase. Default to `HEAD` for the WRITE flow.
- **Where the prompt mechanism lives.** I haven't located a single existing user-prompt helper used across the state machine. If one exists, reuse it; if not, the verification node grows a small prompt helper of its own. This needs an exploration pass before Item 8 lands.
- **Test runner integration.** `gh optivem test system --suite <s> --test <t>` is the agent-facing surface. Inside the orchestrator, calling it via `os/exec` works but is wasteful. Is there an in-process entry point for the test runner we can call directly? If yes, prefer that for speed; if not, shell-out is fine for v1.
- **Contract test layer suite selection.** When a `CT - RED - EXTERNAL DRIVER - WRITE` runs, the prompt says "run against the stub". But the eventual GREEN cycle wants real-side runs. Should the selector tag results with all applicable suites and let the verification node pick based on calling phase, or should the calling phase be passed into `Select`? Latter is more explicit.
  - **Lean:** pass the phase into `Select` (or a pre-computed `SuiteScope`), keep the selector dumb about phase semantics.
- **Inheritance in tests.** A test method declared on a base class and inherited by N concrete subclasses is one source method but N distinct test names at runtime. Java: `@Test` on `BaseTest.shouldRegister` produces test results for every `*Test extends BaseTest`. The selector currently emits the source-level method name, which would map to one entry but actually run as many. For now, `--test BaseTest.shouldRegister` likely runs across all concrete subclasses already (JUnit selector behaviour); confirm before assuming. Worst case, accept extra runs (still safe).
- **Tests living outside the convention.** Smoke tests, embedded tests (`internal/atdd/runtime/driver/embedded_smoke_test.go`), repo-level Go tests — all are out of scope for the selector (they don't traverse the application's driver chain). Confirm the layout config excludes them cleanly so they don't show up as unmapped.
- **`@Disabled` / skip-aware.** A test currently disabled with `"AT - RED - DSL"` shouldn't be included — running it produces a known skip, which is misleading noise. The selector should read disable annotations and exclude tests whose reason matches "current phase or earlier in the pipeline". Pragmatic v1: skip-awareness off for the first cut, accept the noise, add it as v2 polish.

## Out of scope

- **Coverage-based selection.** Considered (running tests with `-coverprofile` and parsing the result to find which tests cover changed lines) but rejected: it requires running tests to decide which tests to run — chicken-and-egg, and the static path is reliable enough given ATDD's architectural constraints. If false negatives become a problem in practice, coverage-based fallback is a v2 option.
- **Cross-cutting changes.** A refactor that moves shared code from `dsl-core` itself (not via an adapter change) is not detected by this selector. Such changes shouldn't appear in driver phases anyway — they belong to AT-GREEN or DSL phases. If they do, the unmapped-fallback runs the full suite.
- **Frontend-only / backend-only changes.** AT-GREEN-FRONTEND and AT-GREEN-BACKEND don't touch `driver-adapter/**`, so the selector trivially returns empty. The verification node should detect "no driver-adapter changes" and skip the prompt entirely.
- **Selector for non-ATDD changes.** This package is ATDD-specific — `internal/atdd/runtime/`. A general "what tests should I run for this diff" tool is a different scope.
- **Caching the analysis.** A 100-method adapter file produces hundreds of regex matches; for a typical 1–3 changed methods, the cost is negligible. No cache in v1.
- **Configurable selectors.** Operators can't add per-project rules; the language layout config is hard-coded. Externalising to YAML is a v2 nicety if a project drifts from the conventions.

## Order of operations

1. Resolve the Open Questions, especially "where does the user-prompt helper live" and "is there an in-process test-runner entry point" — both shape Item 8.
2. Land Item 1 (package skeleton) + Item 2 (per-language layout config) together — minimal scaffolding, no logic, just a callable `Select` returning empty `Result`.
3. Land Items 3 + 4 + 5 + 6 together — the selector logic end to end, covered by Item 9 fixtures. Until Item 8 lands, no caller exists; the package is exercised only by tests.
4. Land Item 7 (suite tagging) in the same PR — small, but the result type isn't useful without it.
5. Land Item 9 (selector tests) in the same PR — non-negotiable; the regex layer is too easy to get subtly wrong without fixture coverage.
6. Land Item 8 (state-machine wiring) in a follow-up PR. By this point the selector is self-contained, well-tested, and easy to wire in.
7. Land Item 11 (UX prompt) in the same PR as Item 8.
8. Land Item 12 (diagnostics) in the same PR as Item 8.
9. Land Item 10 (docs) in the same PR as Item 8 — the user-visible behaviour change goes in alongside the doc that describes it.
10. **Manual rehearsal:** run a real `AT - RED - SYSTEM DRIVER - WRITE` cycle against `templates/shop`, change one driver-adapter method, observe the prompt, choose `r`, observe that exactly the expected test runs, watch it pass. Repeat with an artificial unmapped change (rename an adapter method without updating the port) and observe the full-suite fallback fires with the warning. Repeat once for `CT - RED - EXTERNAL DRIVER` to exercise the contract path.
