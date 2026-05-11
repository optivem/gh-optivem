# Noun-first CLI rename — `gh optivem <noun> <verb>`

> ⚠️ **Needs explicit human approval before implementation. Discuss first.**
> This plan is a draft. Do not execute any step until the author signs off on
> the rename matrix and the open questions in the final section.

> **Paired plan.** The hidden deprecation aliases registered here are dropped
> in [`20260511-2010-drop-verb-first-aliases.md`](20260511-2010-drop-verb-first-aliases.md)
> one release later.

## Context

The current command surface is verb-first:

```
gh optivem build system   |  gh optivem compile system
gh optivem run system     |  gh optivem compile system-tests
gh optivem stop system    |  gh optivem compile          (both tiers)
gh optivem clean system   |
gh optivem test system    |  gh optivem test setup
```

This is the opposite of `gh`'s own convention. Every first-party `gh` command
that operates on a single resource type is `gh <noun> <verb>`:
`gh repo create`, `gh pr list`, `gh issue close`, `gh run view`,
`gh workflow run`, `gh release edit`, …

The few bare-verb commands in `gh` itself (`gh browse`, `gh status`, `gh api`,
`gh completion`) all share one trait: they don't target a single resource type.
They span resources or are meta-level.

`gh optivem` should follow the same rule:

- **Tier-specific verbs** (build/start/stop/clean/compile/test against the
  system; run/setup/compile against the tests) → noun-first.
- **Cross-tier verbs** (`compile` today, `clean` if/when extended) → bare
  verb, same shape as `gh browse`.

## Final command surface

```
gh optivem init                        # bare verb — bootstrap
gh optivem upgrade                     # bare verb — self-update
gh optivem compile                     # bare verb — spans tiers
gh optivem config    init | validate
gh optivem system    build | start | stop | clean | compile
gh optivem test      run | setup | compile
gh optivem token     verify
gh optivem atdd      implement-ticket | manage-project | show diagram
                     (hidden: debug pick-top-ready | classify | next-phase | gate | release — proposed for removal in 20260511-2020-trim-atdd-debug-surface.md)
```

Three bare verbs (init, upgrade, compile), five nouns (config, system, test, token, atdd).

### Top-level surface review (every command audited)

| Command | Today | Decision | Rationale |
|---------|-------|----------|-----------|
| `gh optivem init` | bare verb | **keep bare** | `git init`/`npm init`/`terraform init`/`cargo init` idiom. Spans resources (creates repos, board, sonar, workflows). |
| `gh optivem upgrade` | bare verb | **keep bare** | Self-update on the binary, not a project resource. Same category as `cargo install --force`, `gh extension upgrade`. |
| `gh optivem compile` | bare verb | **keep bare** | Spans tiers (system + test). Children move under `system` / `test` nouns. |
| `gh optivem config` | noun + verbs | **unchanged** | Already noun-first (`config init`, `config validate`). |
| `gh optivem build/run/stop/clean system` | verb + noun | **renamed** | Becomes `gh optivem system <verb>`. See matrix below. |
| `gh optivem test system/setup` | mixed *(test already a tier noun)* | **partially renamed** | `test system` → `test run`; `test setup` unchanged. `test compile` added (was `compile system-tests`). |
| `gh optivem verify tokens` | verb + noun | **renamed** | Becomes `gh optivem token verify`. Strict noun-first: `token` is a real resource concept (real-world env-var credentials); `verify` is one verb on it (future: `token list` to show which credential env vars are configured). The `gh search repos/code/…` precedent was considered but rejected — it's the lone outlier in gh's own surface, and following the rule beats following the exception. |
| `gh optivem atdd` | noun + verbs | **unchanged** | Already noun-first (`atdd show`, `atdd diagram`, `atdd gate`, …). |

### Rename matrix

| Today | New |
|-------|-----|
| `gh optivem build system [--rebuild]` | `gh optivem system build [--rebuild]` |
| `gh optivem run system [--restart \| --log-lines \| --up-timeout]` | `gh optivem system start [--restart \| --log-lines \| --up-timeout]` |
| `gh optivem stop system` | `gh optivem system stop` |
| `gh optivem clean system` | `gh optivem system clean` |
| `gh optivem compile system` | `gh optivem system compile` |
| `gh optivem test system [--suite \| --test \| --sample \| --list]` | `gh optivem test run [--suite \| --test \| --sample \| --list]` |
| `gh optivem test setup` | `gh optivem test setup` *(unchanged)* |
| `gh optivem compile system-tests` | `gh optivem test compile` |
| `gh optivem compile` *(bare, both tiers)* | `gh optivem compile` *(unchanged — bare verb)* |
| `gh optivem verify tokens` | `gh optivem token verify` |

**Verb changes:**

- `run system` → `start` (pairs with `stop`; `run` is overloaded with `test run`).
- `test system` → `test run` (system is implicit when running tests; matches "run the tests").
- `compile system-tests` → `test compile` (drop the `system-` prefix; `test` is the established test-tier noun).
- `verify tokens` → `token verify` (parent and child swap; `token` becomes the noun, `verify` becomes one verb on it).

**Flag surfaces are unchanged on every command.**

## Steps

### Step 1 — Add the `system` parent and lift the handlers

In `runner_commands.go` (or a new `system_commands.go` — see Open Q1), add:

```go
func newSystemCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "system",
        Short: "Operate on the system tier",
    }
    cmd.AddCommand(
        newSystemBuildCmd(),
        newSystemStartCmd(),
        newSystemStopCmd(),
        newSystemCleanCmd(),
        newSystemCompileCmd(),
    )
    return cmd
}
```

Each child is the existing handler renamed:

- `newBuildSystemCmd` → `newSystemBuildCmd` (handler body unchanged)
- `newRunSystemCmd` → `newSystemStartCmd` (handler body unchanged; only `Use`, `Short`, `Example`, and the function name change)
- `newStopSystemCmd` → `newSystemStopCmd`
- `newCleanSystemCmd` → `newSystemCleanCmd`
- `newCompileSystemCmd` (in `compile_commands.go`) → `newSystemCompileCmd`, **moved** into the system tree

Each child's `Use: "system"` field becomes `Use: "build"` / `start` / `stop` /
`clean` / `compile`. Each child's `Example` field is updated to the new form.

### Step 2 — Update the `test` parent and add `test compile`

`newTestCmd` already exists with `test system` and `test setup` children. In
`runner_commands.go:269`:

- `newTestSystemCmd` → `newTestRunCmd` (rename function; `Use: "system"` → `Use: "run"`; update `Example`).
- `newTestSetupCmd` is unchanged structurally; only `Example` examples update.
- Add `newTestCompileCmd` — lift the body of `newCompileSystemTestsCmd`
  (`compile_commands.go:87`) into a new function on the `test` tree. `Use: "compile"`.

### Step 3 — Update bare `compile`

`newCompileCmd` (`compile_commands.go:30`) stays. Its `Run` (the both-tiers
sequence) is unchanged. Drop the `newCompileSystemCmd` and
`newCompileSystemTestsCmd` children — they have moved to `system compile` and
`test compile` respectively. Update the parent's `Long` + `Example`:

```go
Long: `Compile the in-scope source code for a scaffolded project.

Runs "system compile" then "test compile" in sequence, halting on first
failure. Distinct from "gh optivem system build", which runs "docker compose
build" against the system's container images.

Use the noun-scoped forms to narrow to one tier:
  gh optivem system compile   # system tier only
  gh optivem test compile     # test tier only`,
Example: `  gh optivem compile               # compile both tiers
  gh optivem system compile        # narrow to system tier
  gh optivem test compile          # narrow to test tier`,
```

### Step 4 — Rename `verify` parent to `token` and register both trees in `main.go`

Rename `verify_commands.go` → `token_commands.go`. Inside:

- `newVerifyCmd` → `newTokenCmd`; the parent's `Use: "verify"` → `Use: "token"`;
  `Short` updated to "Operate on the credentials gh-optivem consumes from the
  environment".
- `newVerifyTokensCmd` → `newTokenVerifyCmd`; the child's `Use: "tokens"` →
  `Use: "verify"`; `Short` and `Example` updated.
- File-header comment rewritten: the parent is no longer "preflight checks
  namespace"; it's the `token` noun, which today owns one verb (`verify`)
  and may own `list` etc. in future.

`newRootCmd` in `main.go:77`. Replace the current `AddCommand` block
(lines 93-106) with:

```go
cmd.AddCommand(
    newInitCmd(),
    newConfigCmd(),
    newSystemCmd(),     // new noun-first parent
    newTestCmd(),       // existing parent; children renamed
    newCompileCmd(),    // bare verb, unchanged shape
    newAtddCmd(),
    newTokenCmd(),      // renamed from newVerifyCmd
    newUpgradeCmd(),

    // Hidden verb-first aliases for one release. Drop in v1.6 per
    // plans/20260511-2010-drop-verb-first-aliases.md.
    newDeprecatedBuildCmd(),
    newDeprecatedRunCmd(),
    newDeprecatedStopCmd(),
    newDeprecatedCleanCmd(),
    newDeprecatedVerifyCmd(),
)
```

Note `newBuildCmd`, `newRunCmd`, `newStopCmd`, `newCleanCmd`, `newVerifyCmd`
disappear from the user-visible surface; their handlers move into the new
noun parents (`system` and `token`) or stay under existing parents
(`test compile`).

### Step 5 — Hidden deprecation aliases

For each verb-first form that's being moved, register a hidden Cobra command
that prints a stderr deprecation warning and delegates to the new handler.

Pattern (define once in a new `deprecated_commands.go`):

```go
// newDeprecatedAlias wires <oldParentUse> <oldChildUse> as a hidden alias of
// <newForm>. The handler comes from `child` (built by the same constructor the
// new tree uses), with its Use overridden so it routes under the deprecated
// parent. PreRun prints a one-line warning to stderr before the new handler
// runs. Drop in v1.6 — see plans/20260511-2010-drop-verb-first-aliases.md.
func newDeprecatedAlias(oldParentUse, oldChildUse, newForm string, child *cobra.Command) *cobra.Command {
    child.Use = oldChildUse
    child.Hidden = true
    child.PreRun = func(c *cobra.Command, args []string) {
        fmt.Fprintf(os.Stderr,
            "DEPRECATED: `gh optivem %s %s` will be removed in v1.6. "+
                "Use `gh optivem %s` instead.\n", oldParentUse, oldChildUse, newForm)
    }
    parent := &cobra.Command{
        Use:    oldParentUse,
        Hidden: true,
        Short:  "DEPRECATED: use `" + newForm + "`",
    }
    parent.AddCommand(child)
    return parent
}
```

Then in `main.go` / `deprecated_commands.go`:

```go
func newDeprecatedBuildCmd() *cobra.Command {
    return newDeprecatedAlias("build", "system", "system build", newSystemBuildCmd())
}
func newDeprecatedRunCmd() *cobra.Command {
    return newDeprecatedAlias("run", "system", "system start", newSystemStartCmd())
}
func newDeprecatedStopCmd() *cobra.Command {
    return newDeprecatedAlias("stop", "system", "system stop", newSystemStopCmd())
}
func newDeprecatedCleanCmd() *cobra.Command {
    return newDeprecatedAlias("clean", "system", "system clean", newSystemCleanCmd())
}
func newDeprecatedVerifyCmd() *cobra.Command {
    return newDeprecatedAlias("verify", "tokens", "token verify", newTokenVerifyCmd())
}
```

Constructors like `newSystemBuildCmd()` return a fresh Cobra command each call,
so the deprecated alias and the new-tree registration get independent
instances — the deprecation `PreRun` only fires when the alias path is taken.

The `test` family is trickier because `test` is a real (non-deprecated)
parent. Register the deprecated `test system` child directly on `newTestCmd`
as a hidden sibling of `test run`:

```go
testCmd.AddCommand(newDeprecatedTestSystemCmd())  // hidden alias of `test run`
// `test setup` keeps its name — no alias needed.
```

For `compile system` / `compile system-tests`, register the deprecated forms
as hidden children of the bare `compile` parent (same place they live today),
each delegating to `system compile` / `test compile`.

Cobra prints unknown-subcommand suggestions, so a user typing
`gh optivem build` will get suggested both `gh optivem system build` (via
fuzzy match on the new tree) and the hidden `gh optivem build system` (via
the alias). The deprecation warning fires only when the hidden form actually
runs.

### Step 6 — Update first-party callers

These callsites invoke (not just document) the verb-first forms and must be
updated to the new forms. The deprecation warnings would otherwise fire on
every internal run:

| File | Form today | Form after |
|------|-----------|-----------|
| `internal/atdd/runtime/actions/bindings.go` | `gh optivem test system …`, `gh optivem run system`, `gh optivem stop system`, `gh optivem compile` | `gh optivem test run …`, `gh optivem system start`, `gh optivem system stop`, `gh optivem compile` *(unchanged)* |
| `internal/atdd/install.go` | check `gh optivem compile` etc. | unchanged (bare form survives) — verify no `build system` / `run system` refs |
| `internal/atdd/runtime/testselect/testselect.go` | sample-test invocation strings | update to `test run --sample` |
| `internal/runner/tests.go` | error message text: "start it first with `gh optivem run system`" | "start it first with `gh optivem system start`" |
| `scripts/manual-test-runner-shop.sh` | runner integration script | rewrite invocations |
| `internal/atdd/runtime/statemachine/process-flow.yaml` | action strings consumed by the state machine | update each call-site |

### Step 7 — Update first-party tests

Golden command strings in tests need to match the new forms:

- `runner_commands_test.go` — full file; renames CLI invocations and expected strings.
- `internal/runner/tests_test.go` — error message goldens that mention `run system`.
- `internal/atdd/runtime/actions/bindings_test.go` — action-string goldens for every action that invokes the runner/compile.
- `internal/steps/replacements_test.go` — only if it asserts on doc strings; otherwise untouched.

### Step 8 — Update first-party docs and comments

Pure-prose references; no behavior change:

- `README.md` — usage snippets, examples.
- `docs/gh-monitoring-process.md:52-53` — `gh optivem test system` examples.
- `compile_commands.go` (header comment), `compile_summary.go` (header comment).
- `internal/compiler/compiler.go`, `internal/projectconfig/config.go`,
  `internal/steps/verify.go`, `internal/steps/optivem_yaml.go` — doc
  comments mentioning command examples.
- `internal/atdd/runtime/agents/prompts/atdd-backend.md`,
  `atdd-chore.md`, `atdd-driver.md`, `atdd-dsl.md`, `atdd-frontend.md`,
  `atdd-stubs.md`, `atdd-test.md` — these are LLM-read; agents will copy
  whatever wording is there, so they must use the new forms.

### Step 9 — Historical plan files

Per the project convention (see `20260511-1830-bash-sonar-invocation.md`,
"Historical plan files" section), append a one-line note to:

- `plans/20260505-220100-verify-runs-from-wrong-cwd.md`
- `plans/20260511-1418-gh-optivem-test-disable-enable-subcommand.md`

```
> **2026-05-11:** This plan's command examples use the pre-rename
> verb-first forms. See `20260511-2000-noun-first-cli-rename.md`
> for the noun-first equivalents (build system → system build, etc.).
```

Do not rewrite the bodies — they are historical.

### Step 10 — Shop template (separate repo)

The shop template repo invokes the scaffolded commands in its workflows and
README. Affected files (verify with `grep -rEn 'gh optivem (build|run|stop|clean|test|compile)' .` in the shop checkout):

- `.github/workflows/*.yml` — any `run: gh optivem …` step.
- `README.md` — usage examples.
- `compile-all.sh` / similar helper scripts.

**Sequencing:** the gh-optivem deprecation aliases (Step 5) keep the old
forms working in the shop until the shop is updated. So:

1. Merge this plan's gh-optivem PR (new tree + hidden aliases).
2. Cut a gh-optivem release that students can `gh extension upgrade optivem` to.
3. Open a separate shop PR updating every callsite to the new forms.
4. Tag a new `meta-v*` release pinning the new shop SHA (see
   `CONTRIBUTING.md`).

The deprecation aliases give a clean rollout window where neither half is
broken regardless of merge order.

### Step 11 — Verify

- `go build ./...` clean.
- `scripts/test.sh` clean (per `feedback_go_test_windows.md` —
  do not run `go test ./...` unconstrained).
- Manual smoke: `gh optivem system build`, `system start`, `system stop`,
  `system clean`, `system compile`, `test run`, `test setup`, `test compile`,
  `token verify`, bare `compile` — all complete successfully against a
  scaffolded project.
- Manual smoke of every deprecated alias: confirms the warning prints to
  stderr exactly once and exit code matches the new form.
- `gh-acceptance-stage` matrix green.

## Out of scope

- **`test clean` and bare `clean` semantics.** Today only `clean system`
  exists (docker compose down -v --rmi local). Extending `clean` to remove
  test-tier artifacts (`node_modules`, build outputs, Playwright caches,
  `allure-results/`) is a separate feature, not a rename. The bare verb
  `clean` is reserved here but not implemented; attempts to invoke it
  currently fall through to Cobra's "did you mean" handler.
- **Renaming `system_test:` / `system_test.config:` keys in `gh-optivem.yaml`.**
  The CLI's `test` noun is independent of the YAML key naming. The YAML
  schema stays as-is — the runner code already maps `SystemTest` to the
  `test` command tree internally.
- **`scripts/manual-test-runner-shop.sh`** — Step 6 covers updating the
  invocations; the script itself is not renamed or restructured.
- **Cobra `Aliases` field on `system` / `test` parents.** Considered for
  exposing `system build` *and* `build system` as siblings; rejected because
  Cobra's `Aliases` work for sibling names of one command, not cross-tree.
  The hidden-alias approach (Step 5) is the right tool.

## Open questions

1. **File organization.** Keep all subcommand wiring in `runner_commands.go`,
   or split into `system_commands.go` + `test_commands.go` (matching the
   noun-first surface)? Recommendation: split, with `runner_commands.go`
   retained for the shared helpers (`exitOnError`, `resolveSystemPath`, etc.).
2. **Deprecation-warning rate.** One stderr line per invocation, every time?
   Or once per session (suppress if already warned in this process — only
   matters for the state machine which calls into the runner many times in
   one run)? Recommendation: every invocation. The state machine should be
   updated to the new forms in Step 6, so duplicate warnings only fire when
   someone outside our code is still on the old form, where loud is better
   than quiet.
3. **Drop timing.** Plan 2 (drop) is sequenced for v1.6 (i.e. one minor
   release after this lands). Is one release the right gap given how slowly
   scaffolded student repos churn, or longer (v1.7, v2.0)?
4. **`gh optivem stop` as a future bare verb.** Worth reserving the bare
   `stop` namespace now (returns "did you mean `gh optivem system stop`?")
   or leave Cobra's default unknown-command handling? Recommendation: leave
   default; reserving namespaces speculatively is over-design.
