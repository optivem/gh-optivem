# `gh optivem test disable` / `test enable` — replace `disable-test.sh` / `enable-test.sh` shell-outs

> ⚠️ **Needs explicit human approval before implementation. Discuss first.**
> This plan is a draft. Do not execute any step until the author signs off on
> the overall shape (and the open questions in the final section).

## Context

The state machine currently shells out to scripts that live in each target
project (same pattern as the old `compile-all.sh`, which has since been
replaced by `gh optivem compile`):

- `internal/atdd/runtime/actions/bindings.go:879` `disableChangeDriven` — loops
  over `CtxKeyDisableTargets` and invokes `./disable-test.sh <lang> "<reason>" <file>:<method>`
  once per target.
- `internal/atdd/runtime/actions/bindings.go:929` `enableChangeDriven` — mirror
  for `./enable-test.sh`.

The scripts own the per-language markup syntax:
- Java: `@Disabled("<reason>")`
- C#: `[Fact(Skip = "<reason>")]`
- TypeScript: `test.skip(true, "<reason>")`

Same problem `compile-all.sh` had: gh-optivem can't guarantee the script
exists in the target repo, can't unit-test the per-language logic from Go,
and the shop template has to keep its copies in sync.

## Design (per author's answers 2026-05-11)

- **Verb shape:** nested under existing `test` verb.
  - `gh optivem test disable <lang> "<reason>" <file>:<method>`
  - `gh optivem test enable  <lang> "<reason>" <file>:<method>`
- **Batch shape:** one target per invocation (state machine keeps looping in
  Go). Port the existing behavior as-is; no flag redesign.

## Steps

### Step 1 — Native per-language editor (`internal/testmarker/`)

Create a new package that owns the markup operations:

```go
package testmarker

type Language string // "java" | "csharp" | "typescript"

func Disable(lang Language, file, method, reason string) error
func Enable(lang Language, file, method, reason string) error
```

Per-language implementations live in sibling files (`java.go`, `csharp.go`,
`typescript.go`). v1 mirrors whatever the current shell scripts do — likely
line-based regex over the target file. Per
[testselect parsing layer escalation policy](../../../Users/valen_4rjvn9e/.claude/projects/C--GitHub-optivem-academy-gh-optivem/memory/feedback_testselect_parsing_escalation.md),
start with regex; escalate to tree-sitter only if regex proves insufficient.

Unit tests per language: disable inserts the right marker on the right line;
enable removes it; missing method errors with a clear message.

### Step 2 — Wire `gh optivem test disable` / `test enable` in `runner_commands.go`

Add `newTestDisableCmd()` / `newTestEnableCmd()` next to `newTestSystemCmd()`
(`runner_commands.go:268+`). Each parses three positional args
(`<lang> <reason> <file>:<method>`), calls `testmarker.Disable` /
`testmarker.Enable`, exits with the standard `exitOnError` convention.

Register them on the parent `test` command:

```go
cmd.AddCommand(newTestSystemCmd())
cmd.AddCommand(newTestSetupCmd())
cmd.AddCommand(newTestDisableCmd())  // new
cmd.AddCommand(newTestEnableCmd())   // new
```

### Step 3 — Swap state-machine shell-outs

In `internal/atdd/runtime/actions/bindings.go`:

- `disableChangeDriven` (line ~900): change
  `fmt.Sprintf("./disable-test.sh %s %s %s", ...)` to
  `fmt.Sprintf("gh optivem test disable %s %s %s", ...)`.
- `enableChangeDriven` (line ~950): same swap to `gh optivem test enable`.

Update `bindings_test.go` golden command strings
(currently lines 1710-1711, 1810-1811) to match the new form.

Update the action doc comments (lines 860-877, 913-928) to remove "v1 shells
out to ./disable-test.sh" and instead say "shells out to `gh optivem test
disable`, which owns per-language markup via `internal/testmarker`".

### Step 4 — Remove the scripts from the shop template

Once gh-optivem owns the operation natively, delete `disable-test.sh` and
`enable-test.sh` from the `shop` template (and any other scaffolded repos).
That's a separate commit in the `shop` repo, not gh-optivem.

### Step 5 — Verify

- Unit tests in `internal/testmarker/` pass (per-language correctness).
- `internal/atdd/runtime/actions/bindings_test.go` passes with the new
  golden command strings.
- End-to-end: run the AT RED phase against a freshly scaffolded shop project
  (no `disable-test.sh` in the target) and confirm `disableChangeDriven`
  succeeds without the script present.

## Open questions for discussion

1. **Package name.** `internal/testmarker/`? `internal/testtoggle/`?
   `internal/testdisable/`? The operation is "annotate a test with a disable
   marker", not "select a test" — what reads cleanly?
2. **Idempotency.** Should `test disable` on an already-disabled method be a
   no-op, an error, or update the reason? What does the current
   `disable-test.sh` do today?
3. **Marker detection.** Removing the marker on enable: do we match by exact
   reason string, by presence of any disable marker for that method, or by
   both? (Current scripts presumably encode an answer; need to read them.)
4. **Where do the existing scripts live?** Confirm they're in the `shop`
   repo and not elsewhere (e.g. `actions/`), so the Step 4 cleanup is scoped
   correctly.
5. **AT/CT split interaction.** The `at_green_system` flow calls
   `enableChangeDriven` to undo a prior RED-phase disable. Does the AT/CT
   creative/mechanical split refactor
   (`plans/20260505-230100-at-ct-cycle-creative-mechanical-split.md`) change
   this contract? If so, sequence accordingly.
6. **Parser strategy upfront.** Regex v1 per the escalation policy — but is
   there an obvious reason today's scripts can't be regex-based and would
   force tree-sitter from day one (e.g. nested annotation handling in Java)?
