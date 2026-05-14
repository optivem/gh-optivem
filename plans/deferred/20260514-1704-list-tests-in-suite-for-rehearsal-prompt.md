# `gh optivem test run --list --suite <id>` — enumerate test names so the rehearsal `[p]ick specific tests` prompt can show a menu

> ⚠️ **Needs explicit human approval before implementation. Discuss first.**
> This plan is a draft. Do not execute any step until the author signs off on
> the overall shape (and the open questions in the final section).

## Context

In the structural-cycle CHOOSE_TESTS step, when the operator picks the
`[p]ick specific tests` scope, `promptSpecificTests` at
`internal/atdd/runtime/actions/bindings.go:1259-1287` shows a numbered
suite menu (driven by `gh optivem test run --list`), then drops to a
free-text prompt:

```
Test names in <suite> (comma-separated):
```

There's no enumeration of test names per suite, because:

- `internal/runner/config.go:95-104` `Suite` declares only `sampleTest`
  (singular) — no `tests:` field.
- The runner substitutes whatever the operator types into the suite's
  `testFilter` template; the actual names live inside the test code, not
  `tests.yaml`.

A typo (e.g. `sdgdg`) is accepted at the prompt and only fails
downstream when the suite runs with no matching tests.

## Goal

Add a discovery path so the `[p]ick specific tests` prompt can show a
numbered menu of real test names in the chosen suite, falling back to
the current free-text prompt when discovery isn't available.

## Design (per author's answers 2026-05-14)

**Discovery source:** dry-run via a new optional `listTestsCommand`
field on `Suite` in `tests.yaml`. The CLI shells out, parses stdout
one-name-per-line.

**Per-framework support (verified 2026-05-14):**

| Framework | Native discovery | Notes |
|---|---|---|
| .NET / xUnit (`dotnet test`) | ✅ `--list-tests` flag | Output prefixed with `"The following Tests are available:"`; lines are fully-qualified `Namespace.Class.Method` — must trim header and take last `.`-segment to match `DisplayName~<test>` |
| Playwright / TS (`npx playwright test`) | ✅ `--list` flag | Output prefixed with `"Listing tests:"`; each line is `[project] › file:L:C › title` — must trim header and take last ` › ` segment |
| Java / Gradle / JUnit (`.\gradlew.bat test`) | ❌ no native flag | `--dry-run` lists tasks, not test methods. Needs a custom Gradle task or external tooling |

This plan ships all three: the dotnet and TS cases use native flags
out of the box; the Java case requires a small custom Gradle task in
the shop's Java template.

**Missing-`listTestsCommand` behavior:** open question — see final
section.

## Steps

### Step 1 — Extend `Suite` and `tests.yaml` schema

In `internal/runner/config.go`:

```go
type Suite struct {
    ...existing fields...
    ListTestsCommand string `json:"listTestsCommand,omitempty" yaml:"listTestsCommand,omitempty"`
}
```

Update the schema-doc comment block at the top of every shop
`tests.yaml` (dotnet, java, typescript) to document the new optional
field. Loader-side: nothing to validate — empty means "not declared".

### Step 2 — `gh optivem test run --list --suite <id>` behavior

In `test_commands.go` (`newTestRunCmd`, around lines 50-96), extend the
`--list` branch:

```go
if list {
    if len(suites) == 0 {
        for _, id := range tests.SuiteIDs() { fmt.Fprintln(os.Stdout, id) }
        return
    }
    if len(suites) > 1 {
        exitOnError(fmt.Errorf("--list with --suite accepts only one suite at a time"))
    }
    suite := tests.FindSuite(suites[0])
    if suite == nil { exitOnError(fmt.Errorf("suite not found: %s", suites[0])) }
    if suite.ListTestsCommand == "" {
        exitOnError(fmt.Errorf("suite %q has no listTestsCommand declared in tests.yaml", suite.ID))
    }
    names, err := runner.ListTestsInSuite(*suite, cwdForPath(resolvedTests))
    exitOnError(err)
    for _, n := range names { fmt.Fprintln(os.Stdout, n) }
    return
}
```

The new `runner.ListTestsInSuite` lives next to `RunTests` in
`internal/runner/tests.go`. It:

1. Resolves cwd the same way `runOneSuite` does (suite.Path joined
   under tests-cwd).
2. Shells out `Suite.ListTestsCommand` (no `<test>` substitution — this
   is the raw discovery command).
3. Parses stdout — one name per non-empty line, after trimming a
   per-framework header. Header recognition is bound to the command's
   first token:
   - starts with `dotnet` → strip lines containing
     `"The following Tests are available"`; for the remaining lines,
     take last `.`-segment.
   - starts with `npx playwright` (or `playwright`) → strip lines
     containing `"Listing tests"` and `"Total: "`; for the remaining
     lines, take last ` › ` segment.
   - starts with `gradle` / `.\gradlew` → assume the custom task
     already emits bare names one per line; no transformation.
   - else → treat output as bare names one per line (caller's
     responsibility).

Unit-test the parser per framework with captured-fixture output.

### Step 3 — Wire the rehearsal `[p]ick specific tests` menu

In `internal/atdd/runtime/actions/bindings.go`:

1. Add `listSuiteTests(suiteID)` next to `listSystemSuites` (line
   ~1184). Shells out `gh optivem test run --list --suite <id>`;
   returns `(names []string, declared bool, err error)`. `declared=false`
   indicates the suite has no `listTestsCommand` — the error from the
   CLI is recognised by its message.

2. Modify `promptSpecificTests` (line 1259) so that after the suite
   pick:
   - call `listSuiteTests(picked[0])`
   - if names returned: print a numbered menu (re-use the helper that
     backs `promptSuiteMenu`, parameterised on the input slice) and
     accept multi-pick by index or by name.
   - if `declared=false`: fall back to today's free-text prompt
     (see final-section open question for whether to also warn).
   - on error: propagate as `error` and let `gatherTestScope` print
     `<err> — try again.` and loop.

Build the command the same way as today: `gh optivem test run --suite
<id> --test <n1> --test <n2>`. The selection-to-command construction
in `promptSpecificTests` doesn't change.

### Step 4 — Populate `listTestsCommand` in shop's three `tests.yaml`

**.NET** (`../shop/system-test/dotnet/tests.yaml`):
Append `listTestsCommand:` to each suite — same value as `command`,
with the per-run loggers/env stripped and `--list-tests` appended.
Example for `smoke-stub`:

```yaml
listTestsCommand: dotnet test --filter 'FullyQualifiedName~.Latest.SmokeTests' --list-tests
```

**TypeScript** (`../shop/system-test/typescript/tests.yaml`):
Append `--list` to the suite's `command` and place under
`listTestsCommand`:

```yaml
listTestsCommand: npx playwright test --project=smoke-test tests/latest/smoke --list
```

**Java** (`../shop/system-test/java/tests.yaml`): see Step 5.

### Step 5 — Java/Gradle custom `printTests` task

In `../shop/system/java/build.gradle` (or wherever the test task is
declared), add:

```gradle
tasks.register('printTests') {
    description = 'Print fully-qualified JUnit test method names, one per line.'
    dependsOn 'testClasses'
    doLast {
        def discovery = ... // JUnit Platform Launcher discover()
                            // emit method names one per line to stdout
    }
}
```

Two viable implementations — pick one in Step 5a:

- **Option A — JUnit Platform Launcher API**: depend on
  `org.junit.platform:junit-platform-launcher` in `buildScript`; use
  `LauncherDiscoveryRequestBuilder` to discover from the test classpath
  and walk the `TestPlan` for `MethodSource` identifiers.
- **Option B — Classpath scan + reflection**: walk
  `sourceSets.test.output.classesDirs`, load each class, find methods
  annotated `@Test` / `@ParameterizedTest`. Less elegant but no extra
  dependency.

Then add to each Java suite in `tests.yaml`:

```yaml
listTestsCommand: .\gradlew.bat printTests -q
```

Note: Gradle's `-q` suppresses task progress; the task itself must
write *only* bare names to stdout.

### Step 6 — Documentation

- `README.md`: extend the `gh optivem test run --list` section to
  document the `--list --suite <id>` form.
- `CONTRIBUTING.md` (referenced line 24, the rehearsal script): no
  change needed — the rehearsal flow auto-picks up the new behavior.
- Shop `tests.yaml` schema-comment header (Step 1) documents
  `listTestsCommand` for future template diffs.

### Step 7 — Meta version bump

Bump `meta` version so existing scaffolded repos can pick up the new
`listTestsCommand` entries via `gh optivem scaffold copy`. The change
is purely additive (no existing field changes), so the bump is a
minor revision, not breaking.

## Open questions

1. **Missing-`listTestsCommand` fallback in `promptSpecificTests`** —
   when the operator picks a suite whose tests.yaml entry has no
   `listTestsCommand` declared, should the rehearsal:
   - (a) silently fall back to today's free-text prompt (additive, no
     regression for older scaffolded repos)
   - (b) error and refuse to enter `[p]ick specific tests` until the
     suite is upgraded (forces tests.yaml completeness; breaks any
     older scaffolded repo until upgraded)
   - (c) fall back, but print a one-line `note: <suite> has no
     listTestsCommand — type names manually` above the prompt.

   Default assumption pending decision: **(a)**, purely additive.

2. **Gradle implementation choice (Step 5)** — Option A (JUnit Platform
   Launcher API, cleaner, extra dep) vs Option B (classpath
   reflection scan, no extra dep, more code). Default assumption: **A**.

3. **Stdout vs stderr discipline for `dotnet test --list-tests`** — the
   "The following Tests are available:" header sometimes appears on
   stdout interleaved with the rest. Decide whether the Step 2 parser
   strips by line-content match (resilient) or by skipping the first N
   lines (brittle). Default assumption: **line-content match on every
   line**, not by index.

4. **Header parsing robustness** — should the Step 2 parser bail with a
   clear error if the expected header isn't found (defensive), or
   pass-through whatever it sees (lenient)? Default assumption:
   **lenient** — header is optional, any non-empty line becomes a
   name. The framework-binding match is a hint, not a guard.

5. **Should we also accept index-or-name input on the test-name menu?**
   `promptSuiteMenu` uses `parsePicks` which accepts both 1-based
   indices and case-insensitive suite ids. The test-name menu should
   probably accept the same — but multi-pick on dotnet's potentially
   long FQN list could get unwieldy. Default assumption: **yes, share
   the same parser**, since the menu shows the short (last-segment)
   names.

## Cross-references

- Related: [`gh optivem test disable` / `test enable` deferred plan](20260511-1418-gh-optivem-test-disable-enable-subcommand.md)
  — same general pattern of moving per-language test-tier logic from
  shell scripts / inline prompts into `gh optivem test` subcommands.
- Source: prompt at `internal/atdd/runtime/actions/bindings.go:1268`
  (`Test names in %s (comma-separated):`) — the UX gap that motivated
  this plan.
