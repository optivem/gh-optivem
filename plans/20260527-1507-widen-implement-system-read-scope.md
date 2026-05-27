# Widen `implement-system` read scope to include the system-test layer

🤖 **Picked up by agent** — `Valentina_Desk` at `2026-05-27T17:48:48Z`

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
