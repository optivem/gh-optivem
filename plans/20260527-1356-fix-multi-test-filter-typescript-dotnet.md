# Fix multi-test filter for TypeScript and .NET

## Context

Sibling fix: today (2026-05-27) the Java `--test A,B` pathway was repaired
by setting `testFilterJoin: repeat` in `system-test/java/tests.yaml` (and
the legacy / shop copies). Gradle's `--tests` is repeatable, so the join
mode just emits one `--tests '*.<name>'` per value and the original
`|`-joined argument never gets constructed.

The same `gh optivem test run --test A,B` invocation is **still broken**
for TypeScript and .NET, through two different mechanisms. Both descend
from `applyTestFilter` (`internal/runner/tests.go:196`) using the empty /
`"or"` join, which joins the names with a literal `|` and substitutes
once.

### Sub-bug 1 — Windows `cmd.exe` consumes `|` for batch-file targets (TypeScript)

TypeScript's filter is `--grep '<test>'`. With two names → `--grep 'A|B'`.
`splitCommand` (`internal/shell/github.go:177`) strips the single quotes
on parse, so `exec.Command` receives the bare arg `A|B`.

Playwright is invoked via `npx.cmd` on Windows. Go's `os/exec`
auto-detects the `.cmd` extension and routes through `cmd.exe /c`.
`syscall.EscapeArg` only adds outer quotes for args containing whitespace
or `"`, so `A|B` is passed unquoted to cmd.exe, which interprets `|` as
a pipe operator. Same failure mode as the original Java bug, just via
`npx.cmd` instead of `gradlew.bat`.

Playwright's `--grep` semantically accepts `|` as regex alternation, so
the *intent* of the join is correct — the breakage is purely the
Windows shell layer. The fix has to live below `applyTestFilter`,
because no testFilter-level change can avoid emitting `|` for
multi-name regex alternation.

`testFilterJoin: repeat` is not a viable shortcut: playwright's `--grep`
is single-valued (last-flag-wins), so repeating the flag silently drops
all but the last name.

### Sub-bug 2 — `dotnet test --filter` doesn't OR with bare `|` (any OS)

.NET's filter is `&DisplayName~<test>`. The `&` prefix marks an
injection fragment (`appendTestFilter` splices it into the existing
`--filter '...'` arg via `filterInjectionRE`). With two names →
`&DisplayName~T1|T2`. The injected filter expression becomes
`FullyQualifiedName~Smoke&DisplayName~T1|T2`.

This is not the OR the runner intends. `dotnet test --filter` syntax
treats `|` as OR between *full* property expressions; the right-hand
operand of an OR has to be its own `Property{=,~}Value` term, not a bare
value. Per dotnet test's filter grammar, `DisplayName~T1|T2` parses with
`T2` standing in for a property name and effectively matches everything,
not just T2. dotnet's `--filter` is also single-valued, so repeating it
isn't an option either.

The correct expression is `&(DisplayName~T1|DisplayName~T2)`. To produce
that, the substitution mechanism needs a new shape: take a per-name
*fragment template* (`DisplayName~<test>`), substitute it once per name,
join the substituted fragments with `|`, wrap in `( ... )`, and then
inject the parenthesised group with the original `&` prefix.

This is the second class of join behaviour (after `repeat`) that doesn't
fit the default `or` semantics. Adding it as a third
`testFilterJoin` mode keeps the per-language config self-describing.

### Why one plan, not two

The two sub-bugs are independent in mechanism but share the same
trigger (`--test A,B`), the same caller (`applyTestFilter` →
`runShell`), and the same per-language config surface
(`testFilter` / `testFilterJoin` in `system-test/<lang>/tests.yaml`).
Bundling them avoids a partial fix that says "multi-test works for Java
and TypeScript but silently mis-filters .NET" — which is harder to
diagnose than the current loud failure.

## Items

### 1. Add cmd.exe-aware arg escaping to `runShell` for batch-file targets on Windows

**Files touched:**

- `internal/runner/tests.go` (`runShell`, ~lines 236–256)
- new unit-test file or extend `internal/runner/tests_test.go`

**Change:** before calling `cmd.Run()`, detect when:

- `runtime.GOOS == "windows"`, AND
- the resolved executable (`pathx.NormalizeExe(parts[0])`) ends in
  `.bat`, `.cmd`, or after `LookPath` resolves to one (so `npx` →
  `npx.cmd` is caught)

In that branch, override Go's default command-line composition by
setting `cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: ...}` with a
hand-built line that:

1. Applies the C-runtime escaping rules (`syscall.EscapeArg` equivalent)
   to each arg first, then
2. Wraps any arg containing **cmd metacharacters** (`|`, `&`, `<`, `>`,
   `^`, `(`, `)`, `%`) in an additional outer pair of double quotes, so
   cmd.exe leaves them as literal argv to the batch file.

The double-wrap is the canonical workaround: Go's escaping protects the
arg from CRT re-tokenisation; the outer quotes protect it from cmd.exe.

### 2. Add `fragment-or` join mode to `applyTestFilter`

**Files touched:**

- `internal/runner/tests.go` (`applyTestFilter`, ~lines 196–209;
  `appendTestFilter`, ~lines 223–228)
- `internal/runner/config.go` (no field change, but update the doc
  comment on `TestFilterJoin` to enumerate `repeat`, `or`, and the new
  `fragment-or`)
- `internal/runner/tests_test.go` (new sub-tests for the new mode)
- `README.md` (`testFilterJoin` reference, if it documents the modes)

**Behaviour:** when `join == "fragment-or"`:

1. Strip the leading `&` from `testFilter` (assert it's present;
   `fragment-or` only makes sense for injection fragments — return the
   command unchanged with a clear error otherwise so misconfigured
   YAML fails loudly at runtime, not silently).
2. For each name, substitute `<test>` into the stripped template.
3. Join the substituted fragments with `|`.
4. Wrap the joined result in `( ... )`.
5. Re-prepend `&`.
6. Hand the resulting fragment to `appendTestFilter`, which still
   handles the `&`-prefix injection into the existing `--filter '...'`
   arg via `filterInjectionRE`.

For names `[T1, T2]` and template `&DisplayName~<test>`, the produced
fragment is `&(DisplayName~T1|DisplayName~T2)`, injected to become
`--filter 'FullyQualifiedName~Smoke&(DisplayName~T1|DisplayName~T2)'`.

For a single name, the parens are still emitted (`&(DisplayName~T1)`).
That's harmless to dotnet's parser and keeps the multi/single paths
uniform — no special-case branch.

### 3. Set `testFilterJoin: fragment-or` in .NET tests.yaml

**Files touched (shop template — propagation source):**

- `C:\GitHub\optivem\academy\shop\system-test\dotnet\tests.yaml`
- `C:\GitHub\optivem\academy\shop\system-test\dotnet\tests.legacy.yaml`

Insert `testFilterJoin: fragment-or` on the line immediately following
`testFilter:`, matching the Java fix layout.

**Files touched (active rehearsal worktrees — verify these still exist
before editing; they're ephemeral):**

- any `worktrees/rehearsal-*/system-test/dotnet/tests.yaml`
- any `worktrees/rehearsal-*/system-test/dotnet/tests.legacy.yaml`

### 4. No tests.yaml change needed for TypeScript

The TS `testFilter: --grep '<test>'` with the default `or` join already
emits the semantically-correct regex `A|B`. Item 1 fixes the Windows
shell layer underneath it. No `testFilterJoin:` entry is needed in
`system-test/typescript/tests.yaml`.

This item exists in the plan only as a checkpoint: confirm during
Item 1's tests that the TS pathway needs no per-language config change,
so the shop template is intentionally left as-is.

### 5. Unit-test coverage

**Files touched:**

- `internal/runner/tests_test.go` — add cases for `fragment-or` join
  (single name, two names, three names, missing `&` prefix error).
- new test file (e.g. `internal/runner/runshell_windows_test.go`) gated
  by `//go:build windows` — exercise `runShell` against a tiny `.bat`
  fixture that echoes its argv to stdout, assert args containing `|`,
  `&`, `(`, `)` round-trip intact.

The Windows test is build-tagged so the cross-platform `go test ./...`
on a non-Windows CI runner still passes; on Windows runners (and the
operator's local Windows dev box) it executes.

## Verification

Out-of-scope for agent execution; for the operator after Items 1–3 land:

- TS: in a TypeScript scaffold or rehearsal worktree, run
  `gh optivem test run --suite=acceptance-api --test=T1,T2` against
  two real playwright tests. Confirm playwright runs *both* (its
  `Running N tests` summary should match) and no cmd.exe "is not
  recognized" error appears.
- .NET: same invocation against a dotnet scaffold/rehearsal. Confirm
  via `dotnet test`'s `Passed!`/`Failed!` summary that exactly the two
  named tests ran (not "everything matched" — the symptom of the
  current OR-fragment misparse).

## Non-goals

- Reworking `splitCommand` itself. The fix is in `runShell` because the
  problem is specifically about how parsed args meet the Windows
  process-launch layer; `splitCommand` is correct as a quote-aware
  tokeniser.
- Generalising the cmd-metacharacter escaping to non-batch targets.
  `.exe` and `.com` invocations don't route through cmd.exe and are
  unaffected. Adding escaping there would be defensive code with no
  failing case.
- Touching the `repeat` or default `or` join modes. They are correct
  for their respective runners; Item 2 adds a third mode rather than
  changing the existing two.
