# Plan: Print ticket link and split the system endpoints banner per system

## Context

After a successful `gh optivem implement --issue N`, the exit banner today emits one flat OK/DOWN list spanning every system in `systems.yaml`:

```
=== System endpoints ===
  OK Monolith: http://localhost:3111
  OK Monolith API: http://localhost:8111/health
  OK ERP API (Real): http://localhost:9111/erp/health
  OK Clock API (Real): http://localhost:9111/clock/health
  OK Monolith: http://localhost:3112
  OK Monolith API: http://localhost:8112/health
  OK ERP API (Stub): http://localhost:9112/erp/health
  OK Clock API (Stub): http://localhost:9112/clock/health
```

Two problems:

1. **Ticket URL is dropped.** `driver.preResolveIssue` (`internal/atdd/runtime/driver/driver.go:606`) resolves the issue at startup and stashes `issue.URL` onto `statemachine.Context` as `issue-url` via `writeResolvedIssue` (`driver.go:638`). The statemachine context is local to `driver.Run`, so by the time `implement_commands.go:152` calls `printSystemEndpointsBanner`, the URL is gone. The operator has to scroll back to the "Resolved issue …" line printed at startup to copy the link.
2. **The flat list merges two semantically different stacks.** The shop template (`../shop/docker/<lang>/<arch>/systems.yaml`) declares two `SystemEntry` rows — `label: real` (SUT against vendored simulators) and `label: stub` (SUT against WireMock stubs). Today's `runner.Status` (`internal/runner/status.go:33`) prints both back-to-back with no header between them.

Goal:

- Surface the resolved `tracker.Issue` back to the cobra layer via an out-pointer on `driver.Options`, so the banner can print `Ticket: <url>` without a second tracker call.
- Replace the single `=== System endpoints ===` header with one `=== System connected to <description> ===` header per `SystemEntry`. Declaration order owns the on-screen order — runner stays oblivious to "real vs stub" semantics.
- Update the shop template's six `systems.yaml` files: add `description:` to each entry and reorder so `stub` is declared first.

### Why declaration order (not "stub first" hardcoded in the runner)

`internal/runner/config.go`'s package doc says the runner has no concept of architecture, language, or suite flavor — those identities live in shop's filenames and directory names. Hardcoding "stub before real" inside `runner.Status` would be exactly that kind of coupling. Operator-owned ordering via `systems.yaml` keeps the runner generic; the shop template carries the "stub first" choice as a YAML-level decision the operator can override.

### Why out-pointer (not callback / return-value)

A callback or a returned result struct would also work, but:

- `driver.Run`'s signature is `func Run(ctx, opts) error`. Changing it to return `(Result, error)` ripples through every test (`internal/atdd/runtime/driver/driver_test.go` and callers).
- A callback `OnIssueResolved func(tracker.Issue)` would require the caller to allocate a closure that captures a local variable. Functionally equivalent to the out-pointer but with more indirection.
- An out-pointer (`ResolvedIssue *tracker.Issue`) is nil-safe by default — zero-valued Options keep working unchanged, no test fixture updates needed.

The state is known synchronously at start of run (`preResolveIssue` is line 352 of `driver.go`, well before `eng.RunProcess`). The out-pointer captures it at that exact moment with no allocation churn.

## Items

### Item 1 — Add `ResolvedIssue *tracker.Issue` out-pointer to `driver.Options`

**File:** `internal/atdd/runtime/driver/driver.go`

Add field to `Options` (alongside `IssueNum` and `ProjectURL`, since it's the same conceptual group):

```go
// ResolvedIssue, when non-nil, receives the tracker.Issue that
// preResolveIssue looked up at startup. Lets the cobra layer print
// the ticket URL in its exit banner without re-querying the tracker.
// Optional; nil leaves existing behaviour untouched.
ResolvedIssue *tracker.Issue
```

In `preResolveIssue` (line 606), after `writeResolvedIssue(sCtx, issue)` on line 623, append:

```go
if opts.ResolvedIssue != nil {
    *opts.ResolvedIssue = issue
}
```

No `withDefaults` changes — nil is the legitimate "caller doesn't care" sentinel.

### Item 2 — Add optional `Description` field to `runner.SystemEntry`

**File:** `internal/runner/config.go`

Add field to `SystemEntry` (line 42):

```go
// Description is the operator-supplied suffix for the per-system
// banner header, rendered as "=== System connected to <description> ===".
// Optional; the banner falls back to Label when empty. The runner
// itself never branches on the value — purely display text.
Description string `json:"description,omitempty" yaml:"description,omitempty"`
```

No validation in `LoadSystem` — the field is optional and free-form. Tests in `internal/runner/config_test.go` continue to pass without modification (existing fixtures don't set `description:`; the YAML unmarshaller leaves the field as the zero string).

### Item 3 — Per-system header in `runner.Status`

**File:** `internal/runner/status.go`

Refactor the `for _, s := range sys.Systems` loop body (lines 45-58) to print a header before each system's probes:

```go
for _, s := range sys.Systems {
    desc := s.Description
    if desc == "" {
        desc = s.Label
    }
    if desc != "" {
        fmt.Fprintf(w, "=== System connected to %s ===\n", desc)
    }
    for _, c := range s.Components {
        if c.URL == "" {
            continue
        }
        probe(c.Name, c.URL)
    }
    for _, e := range s.ExternalSystems {
        if e.URL == "" {
            continue
        }
        probe(e.Name, e.URL)
    }
}
```

The `if desc != ""` guard is defensive — `LoadSystem` already requires `label`, but `runner.Status` is called with arbitrary `*SystemConfig` (tests construct one directly).

Order is declaration order. The runner does not sort.

### Item 4 — Print ticket line and consume `ResolvedIssue` in `implement`

**File:** `implement_commands.go`

Add import (alongside the existing `driver` import on line 34):

```go
"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
```

In the `Run` closure (around line 136 where the `driver.Run` Options literal is constructed), declare and thread the out-pointer:

```go
var resolved tracker.Issue
runErr := driver.Run(context.Background(), driver.Options{
    IssueNum:            issue,
    ResolvedIssue:       &resolved,
    // ...existing fields...
})
if runErr == nil {
    printRunEndBanner(cmd.ErrOrStderr(), cfg, resolved.URL)
}
exitOnError(runErr)
```

Rename `printSystemEndpointsBanner` → `printRunEndBanner` and update its signature:

```go
func printRunEndBanner(w io.Writer, cfg *projectconfig.Config, ticketURL string) {
    if ticketURL != "" {
        fmt.Fprintf(w, "\nTicket: %s\n", ticketURL)
    }
    if cfg == nil || cfg.System.Config == "" {
        return
    }
    sys, err := runner.LoadSystem(cfg.System.Config)
    if err != nil {
        return
    }
    fmt.Fprintln(w)
    _ = runner.Status(w, sys, runner.StatusOptions{})
}
```

Key changes vs the current implementation (`implement_commands.go:176`):

- Removes the `=== System endpoints ===` outer wrapper. The per-system `=== System connected to ... ===` headers from Item 3 take its place — "two print outs" instead of one banner with two unlabelled halves.
- Ticket line printed first (above all system blocks); blank line before the first system header for readability.
- Best-effort semantics preserved: nil cfg, empty `System.Config`, or `LoadSystem` errors still suppress the system block — but the ticket line still prints if URL is set, since it doesn't depend on `cfg.System`.

### Item 5 — Update shop template `systems.yaml` files (6 files, autonomous batch)

**Files:** all under `../shop/docker/`

- `dotnet/monolith/systems.yaml`
- `dotnet/multitier/systems.yaml`
- `java/monolith/systems.yaml`
- `java/multitier/systems.yaml`
- `typescript/monolith/systems.yaml`
- `typescript/multitier/systems.yaml`

For each file:

- Reorder the two `systems:` entries so `label: stub` is declared first, `label: real` second.
- Add `description: External System Stubs` to the stub entry.
- Add `description: External System Test Instances` to the real entry.

Example (java/monolith — adapt indent/comment style per-file):

```yaml
systems:
  - label: stub
    description: External System Stubs
    composeFile: docker-compose.local.stub.yml
    containerName: my-shop-stub
    components:
      - name: Monolith
        url: http://localhost:3112
        containerName: system
      - name: Monolith API
        url: http://localhost:8112/health
        containerName: system
    externalSystems:
      - name: ERP API (Stub)
        url: http://localhost:9112/erp/health
        containerName: external-system-stubs
      - name: Clock API (Stub)
        url: http://localhost:9112/clock/health
        containerName: external-system-stubs
  - label: real
    description: External System Test Instances
    composeFile: docker-compose.local.real.yml
    containerName: my-shop-real
    components:
      - name: Monolith
        url: http://localhost:3111
        containerName: system
      - name: Monolith API
        url: http://localhost:8111/health
        containerName: system
    externalSystems:
      - name: ERP API (Real)
        url: http://localhost:9111/erp/health
        containerName: external-system-simulators
      - name: Clock API (Real)
        url: http://localhost:9111/clock/health
        containerName: external-system-simulators
```

The block comment near the top of `java/monolith/systems.yaml` ("`label: real` brings up …`label: stub` swaps …") should be re-ordered to match — describe `stub` first.

Teaching repo: no migration path for existing scaffolded projects. Operators regenerate their `systems.yaml` from the updated template (per `feedback_teaching_repo_no_legacy.md`).

### Item 6 — Update `implement_commands_test.go` banner tests

**File:** `implement_commands_test.go`

The two existing tests assume `printSystemEndpointsBanner(w, cfg)` with the global `=== System endpoints ===` header. Update:

- Rename references from `printSystemEndpointsBanner` to `printRunEndBanner` and update call sites to pass the new `ticketURL` argument.
- `TestPrintSystemEndpointsBanner_writesBannerWithConfig`:
  - Drop the `=== System endpoints ===` literal assertion.
  - Add an assertion that `=== System connected to real ===` appears (label fallback — fixture has no description set).
  - Keep the `DOWN api:` assertion.
  - Call as `printRunEndBanner(&buf, cfg, "")` to confirm the no-ticket branch.
- `TestPrintSystemEndpointsBanner_silentOnNilConfig` and `_silentOnEmptyConfigPath`:
  - Keep, but call as `printRunEndBanner(&buf, cfg, "")` and re-assert `buf.Len() == 0`.

Add three new tests:

- `TestPrintRunEndBanner_printsTicketLine`: pass a non-empty `ticketURL`, nil cfg → expect output to contain `Ticket: https://…` and nothing else.
- `TestPrintRunEndBanner_descriptionInHeader`: fixture systems.yaml with `description: External System Stubs` → expect `=== System connected to External System Stubs ===` in output (description wins over label).
- `TestPrintRunEndBanner_ticketAndSystemsTogether`: fixture with two systems (stub first, real second) + non-empty ticketURL → assert ticket line appears before both headers, and the stub header appears before the real header (declaration-order preservation).

### Item 7 — Update `runner` status tests for the per-system header

**File:** `internal/runner/status_test.go`

The existing tests target `PrintEndpoints` (a different function, untouched by this plan) — no changes needed.

If any test in this file targets `runner.Status` directly (verify by reading the file), add header-presence assertions matching the new format. If no such test exists, add one:

- `TestStatus_emitsPerSystemHeader`: build a `SystemConfig` with two entries (`label: stub` first, `label: real` second; one with `Description: External System Stubs`, the other with `Description` empty). Call `Status(&buf, sys, StatusOptions{Timeout: 50 * time.Millisecond})` against unreachable URLs. Assert:
  - `=== System connected to External System Stubs ===` appears before `=== System connected to real ===` (description wins for one; label fallback for the other; declaration order preserved).

## Out of scope

- **Changing `runner.PrintEndpoints` output.** Separate function, called from `system up` (`internal/runner/system.go:184`), not from the implement banner. Adding per-system headers there would be a consistency win but is a different surface — out of scope for the user's "implement banner + ticket link" request.
- **Re-fetching the issue via tracker at end-of-run.** The out-pointer in Item 1 reads the URL that `preResolveIssue` already resolved synchronously at startup; no extra API call.
- **Sorting systems in `runner.Status`.** Declaration order is operator-owned via `systems.yaml`. Hardcoding "stub before real" inside the runner would couple it to specific shop-template labels (violates the `internal/runner/config.go` package-doc stance).
- **Validating `description` in `LoadSystem`.** Optional field; absent values fall back to `label`. Adding a "description required" rule would invalidate every pre-existing scaffolded `systems.yaml` for no operator benefit.
- **Migration of existing scaffolded projects.** Teaching repo: operators regenerate from the updated shop template (`feedback_teaching_repo_no_legacy.md`).

## Verification

- `go test ./internal/runner/... -p 2` passes.
- `go test ./internal/atdd/runtime/driver/... -p 2` passes.
- `go test ./... -p 2 -timeout 5m` passes (Windows memory-safety guidance from `feedback_go_test_windows.md`).
- `gh optivem system status` against a freshly-scaffolded shop project: expect two `=== System connected to ... ===` blocks, stub block above real block, no `=== System endpoints ===` wrapper line.
- `gh optivem implement --issue <N>` against a freshly-scaffolded shop monolith Java project: confirm the exit banner begins with `Ticket: https://github.com/<org>/<repo>/issues/<N>`, followed by the two per-system blocks in the same stub-then-real order.
