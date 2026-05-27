# Widen `implement-system` read scope to include the system-test layer

## Context

`internal/atdd/runtime/statemachine/process-flow.yaml:1389-1408` declares
the `implement-system` phase with:

```yaml
read:  [system-path]
write: [system-path]
```

The `system-implementer` agent's prompt
(`internal/assets/runtime/agents/atdd/system-implementer.md`) says it
"writes production code under the system surface (`${system-path}`) to
make the failing acceptance tests pass" — yet the agent cannot read:

- `at-test` — the failing acceptance test that defines *what* to
  implement,
- `ct-test` — contract tests the production system must satisfy,
- `dsl-port` / `dsl-core` — the DSL the AT speaks,
- `driver-port` / `driver-adapter` — the testkit port the adapter calls
  into the production system with, and its current wiring,
- `external-system-driver-port` / `external-system-driver-adapter` —
  how the AT stages out-of-process dependencies the system code may
  integrate with.

The agent is being asked to make a test pass while denied read access to
that test and to every layer above the production system that defines
its expected behaviour. In practice it then has to infer requirements
from either compile errors at the adapter→system seam or the ticket
prose alone — both of which are downstream of the actual specification
(the AT itself).

### Precedent in the same file

Two adjacent phases already model the asymmetric "read the spec, write
the code" shape this plan applies to `implement-system`:

- `implement-system-driver-adapters`
  (`process-flow.yaml:1442-1454`) — reads `driver-port` to see *what*
  to implement, writes only `driver-adapter`. The block-comment
  (lines 1438-1441) calls out the asymmetry explicitly.
- `update-system` (`process-flow.yaml:1417-1429`) — reads
  `system-path, driver-adapter, driver-port` "(must see what must not
  change)" but writes only `system-path` + `driver-adapter`.

The `refactor-system` and `refactor-tests` phases at lines 1605-1642
already enumerate the full system-test layer
(`at-test, ct-test, dsl-port, dsl-core, driver-port, driver-adapter,
external-system-driver-port, external-system-driver-adapter,
system-path`) on the read side, demonstrating the canonical scope
expression for "see everything the system-test surface exposes".

`implement-system` is the GREEN-stage analogue: it needs the same
visibility as the refactor phases on the read side because it is making
a failing AT go green, but its write target stays the narrower
production surface `system-path`.

### Why not also widen the prompt to imply external-system-driver scope

External-system-driver-port/adapter are included on the read side
because the failing AT may stage external interactions
(stubbed/mocked) that the production system must call into via real
clients. Without seeing the stubs the agent cannot match the
contract the AT expects. This mirrors the refactor phases — same
test-surface visibility, narrower write authority. The asymmetry is the
guarantee.

### Open questions resolved up-front

1. **Include `external-system-driver-*` on read?** Yes — refactor
   phases set the precedent for "full system-test layer on read", and
   excluding them would silently break ATs that stub external systems.
2. **Widen write scope at all?** No. The whole point of the
   asymmetric pattern is that the agent's authority to *change* code
   stays pinned at `system-path`; it gains visibility, not power. Any
   driver-side change still requires a `scope_exception`.
3. **Need a separate verb (e.g. `implement-system-from-at`)?** No.
   This is a scope correction on the existing verb, not a behaviour
   change. The agent's responsibilities are unchanged — only the
   inputs available to it. A verb split would imply a workflow change
   that does not exist here.

## Items

### 1. Widen `implement-system` read scope in `process-flow.yaml`

**Files touched:**

- `internal/atdd/runtime/statemachine/process-flow.yaml`
  (the `implement-system:` block, lines ~1389-1408)

**Change:** replace

```yaml
        read:  [system-path]
        write: [system-path]
```

with

```yaml
        read:  [at-test, ct-test, dsl-port, dsl-core, driver-port, driver-adapter, external-system-driver-port, external-system-driver-adapter, system-path]
        write: [system-path]
```

Add a block-comment immediately above `implement-system:` (matching the
style of the `update-system` comment at lines 1410-1416) explaining the
asymmetry:

> Asymmetric: reads the full system-test layer (the failing AT, its
> contracts, the DSL it speaks, the driver port/adapter pair and the
> external-system driver pair the AT may stage) so the agent can see
> *what* behaviour to implement. Writes only `system-path` — any
> change to driver/test code requires a `scope_exception`.

### 2. Update the `system-implementer` agent prompt to use the new visibility

**Files touched:**

- `internal/assets/runtime/agents/atdd/system-implementer.md`

**Change:** update Step 1 prose to reflect that the agent should
start by reading the failing AT (under `${at-test}`) to understand the
required behaviour, then trace through `${dsl-port}` /
`${driver-port}` / `${driver-adapter}` to see how the AT reaches the
production system, before writing the simplest implementation under
`${system-path}` that makes the AT pass.

Keep the existing `${scope-block}` mechanism — it already renders the
full read/write lists, so the explicit list of paths in the prompt
body should remain prose ("the failing AT", "the contracts", "the
driver port the adapter calls into") rather than re-enumerating
placeholders. The scope-block is the source of truth for what's
in scope; the prompt body explains *how to use* that scope.

Do not add a new "verify" sub-step — running tests stays out of scope
for this agent per existing convention; the change-cycle's own
verification step runs the AT.

### 3. Mirror the scope widening in the driver-test inline YAML fixtures

**Files touched:**

- `internal/atdd/runtime/driver/driver_test.go`
  (the synthetic `implement-system:` blocks at ~lines 129-141 and
  ~lines 180-192 — both currently declare `read: [system-path]`)

**Change:** replace both `read: [system-path]` lines under
`implement-system:` with the same widened read list from Item 1. Leave
`write: [system-path]` unchanged.

Inspect the surrounding test assertions: at least one assertion
(driver_test.go:759) checks that the rendered prompt contains
`- `system-path`: system/monolith/typescript`. Verify whether the
test also asserts the *absence* of other path keys; if it does,
extend the assertion to expect the additional keys to now appear, in
the same `${name}: ${resolved}` format the scope-block renderer
produces. If it does not, no further test edit is needed — the
existing assertion still passes because `system-path` remains in the
rendered list.

### 4. Audit other inline YAML fixtures referencing `implement-system`

**Files touched (audit only; edit if matches found):**

- `internal/atdd/runtime/clauderun/clauderun_test.go`
- `internal/atdd/runtime/statemachine/transitions_test.go`
- any other `*_test.go` under `internal/atdd/` that grep matches for
  the literal block

**Change:** grep for the literal pattern
`implement-system:` and `read:  [system-path]` (two-space indent) in
each file. For every inline YAML occurrence that mirrors the production
`implement-system` phase shape, apply the same read-list widening as
Item 3. For occurrences that are *unrelated* (e.g. a `task-name`
string literal in a fixture map, not a process-flow node), leave
untouched.

If a fixture intentionally uses the *narrow* `[system-path]` read scope
to test the old behaviour, replace with the *new* `[at-test, ct-test,
…, system-path]` list — the old behaviour is no longer the system's
behaviour, so fixtures must match.

### 5. Regenerate the statemachine-derived diagrams referenced from docs

**Files touched (audit only):**

- `internal/atdd/runtime/diagram/diagram.go` (read; do not edit
  unless it renders scope-list strings directly into a diagram
  asset that this plan invalidates)

**Change:** none unless the diagram renderer bakes the read-list into
its output. The repository convention (per CLAUDE memory: "Plans must
not include diagram regeneration steps") is that the
`regenerate-diagram` GitHub Actions workflow auto-regenerates
`docs/process-diagram.md` and `docs/images/*.svg` on push to `main`.
This item exists only as an audit checkpoint — if the diagram source
encodes scope strings, no manual regen step is added here; the
post-merge workflow will pick the change up.

## Verification

Out-of-scope for agent execution; for the operator after Items 1-4 land:

- Re-run an active rehearsal whose change-cycle reaches
  `implement-system` (e.g. the `gift-wrap an order` rehearsal at
  `worktrees/rehearsal-20260527-135607`). Inspect the dispatched
  prompt's scope-block — confirm it lists `at-test`, `ct-test`,
  `dsl-port`, `dsl-core`, `driver-port`, `driver-adapter`,
  `external-system-driver-port`, `external-system-driver-adapter`,
  and `system-path` under "You may **read** files under these paths:",
  and lists only `system-path` under "You may **modify** files under
  these paths:".
- Confirm the agent can now reference the failing AT directly in its
  reasoning (visible in the prompt log under
  `.gh-optivem/runs/<ts>/NNN-system-implementer.prompt.md` and the
  subsequent Claude response).

## Non-goals

- Widening `write` scope. The asymmetry is the guarantee — read more,
  write the same.
- Splitting `implement-system` into a new verb. This is a scope
  correction on the existing verb, not a workflow change.
- Changing `update-system`'s scope. Its read list is intentionally
  narrower because reshape work must not consult the AT for behaviour
  cues (the behaviour is the invariant being preserved).
- Touching any `phase-scopes.yaml` sidecar. Per memory
  ("No `PhasesDeferredByPlan` mechanism"), scope lives inline in
  `process-flow.yaml`; no sidecar exists for this repo.
- Adding a `scope_exception` allowlist for the new read keys. Read
  expansions never need exceptions — they only relax, not tighten.

## Cross-references

- Adjacent precedent (asymmetric scope, comment style):
  `update-system` at `process-flow.yaml:1410-1429` and
  `implement-system-driver-adapters` at `process-flow.yaml:1438-1454`.
- Full-system-test-layer read precedent: refactor phases at
  `process-flow.yaml:1605-1642`.
- Agent prompt: `internal/assets/runtime/agents/atdd/system-implementer.md`.
