# Scope-exception envelope must be available on every writing-agent MID

## Context

During the 2026-05-28 rehearsal of issue #71, the `system-implementer`
agent identified that it needed to write outside its declared scope
(`system/db/migrations/V20260528113000__add_gift_wrap.sql` —
outside `system-path = system/monolith/typescript`). Its prompt's
`scope.md` chunk instructs it to escape cleanly by emitting the
scope-exception envelope:

```
gh optivem output write \
  scope-exception-files=path/to/out-of-scope.go \
  scope-exception-reason="<one-line rationale>"
```

The agent tried twice. Both attempts returned:

```
ERROR: "gh optivem output write" must run inside a gh-optivem agent dispatch
       (GH_OPTIVEM_OUTPUT_FILE is not set)
```

The agent's final message captured the failure exactly:

> The envelope facility isn't active here (no GH_OPTIVEM_OUTPUT_FILE
> env var), so I cannot emit it. The migration column is genuinely
> required — without it, the gift_wrap flag cannot round-trip through
> persistence and neither test can pass. I'll proceed with the
> minimal implementation including the schema column, and flag the
> out-of-scope migration write in my final summary.

The agent wrote the file. `VALIDATE_OUTPUTS_AND_SCOPES` then failed.
The orchestrator routed into `FIX`, which asks the operator to
approve remediation — for a write the agent had legitimate grounds to
escalate cleanly, and would have, if the envelope had been reachable.

## Root cause

`internal/atdd/runtime/clauderun/clauderun.go:1643-1654`:

```go
func subprocessEnv(opts RunOpts) []string {
    envApproval := opts.AutoSpec != "" && opts.ConfirmSpec != ""
    if !envApproval && opts.OutputFilePath == "" && opts.OutputKeysSpec == "" {
        return nil
    }
    env := os.Environ()
    if opts.OutputFilePath != "" {
        env = append(env, "GH_OPTIVEM_OUTPUT_FILE="+opts.OutputFilePath)
    }
    if opts.OutputKeysSpec != "" {
        env = append(env, "GH_OPTIVEM_OUTPUT_KEYS="+opts.OutputKeysSpec)
    }
    ...
}
```

`GH_OPTIVEM_OUTPUT_FILE` is exported only when `OutputFilePath` is
non-empty, which only happens when the MID declares an `outputs:`
block. Three of the writing-agent MIDs declare one (the implementers
of acceptance tests, DSL, and driver adapters — they emit signal
flags like `dsl-port-changed`, `external-driver-port-changed`). The
remaining writing-agent MIDs — including `implement-system` and
`update-system` — declare none.

Net result: agents whose MIDs declare no outputs cannot emit the
envelope at all. The `scope.md` instruction lies to them. They have
two equally bad choices: write out of scope silently, or refuse and
exit (which the prompt also forbids — "do not stop mid-dispatch to
present a plan or ask for approval").

## Why this is a bug, not a config choice

The `scope.md` chunk is reachable from every writing-agent prompt —
its preamble explicitly tells the agent to use the envelope as the
universal escape hatch. The doctrine is **"every writing-agent MID
exposes the envelope channel"**, not "outputs-declaring MIDs only".

Today the implementation accidentally couples envelope availability
to the presence of other structured outputs (signal flags). That
coupling is incidental — the envelope keys (`scope-exception-files`,
`scope-exception-reason`) are listed alongside flag keys in the three
MIDs that have an `outputs:` block today (see
`process-flow.yaml:1334-1337`, `1373-1376`, `1411-1414`), purely
because they share the same JSONL channel. But the converse — "no
flag outputs → no envelope channel" — was never an intentional
design decision; it's a fallthrough in `subprocessEnv`.

## Items

### Item 1 — Decide the contract shape

Two ways to fix this. Pick one.

| Option                                                            | Mechanism                                                                                                                                                                                                          | Pros                                                                                                                                  | Cons                                                                                                                          |
|-------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------|
| **A.** Always export `GH_OPTIVEM_OUTPUT_FILE` for writing agents | Drop the `OutputFilePath == ""` short-circuit in `subprocessEnv` *when the dispatch is a prod-agent / writing-agent tier*. Synthesise an output-file path even when no MID outputs are declared.                  | Doctrine-aligned: every writing-agent dispatch can emit the envelope. Zero per-MID YAML churn.                                        | Couples envelope availability to a tier tag (`category: prod-agent`). The runner needs to know agent tier at dispatch time.   |
| **B.** Add `scope-exception-*` to every writing-agent MID's `outputs:` block | Add an `outputs:` block to `implement-system`, `update-system`, and any other writing-agent MID without one, with at minimum `scope-exception-files` (string-list, optional) and `scope-exception-reason` (string, optional). | Per-MID self-evident: anyone reading the YAML sees the envelope keys explicitly. No runner-side tier coupling.                       | Boilerplate: every writing-agent MID repeats the two keys. Easy to forget on the next new MID — same trap re-opens.           |

**Recommendation: A.** The envelope is doctrine for **every**
writing-agent dispatch — it shouldn't be per-MID opt-in. Option B
makes the YAML *look* explicit but encodes the same rule N times,
and the bug recurs the next time someone adds a writing-agent MID
and forgets the outputs block. Option A puts the rule once, where
the rule actually lives (the runner).

(The `category: prod-agent` tag already exists on every writing-agent
MID — see `process-flow.yaml:1459, 1496, 1517, 1546, 1566` — so the
tier coupling is just reading an existing param, not introducing a
new concept.)

### Item 2 — Implement Option A in the runner

**File:** `internal/atdd/runtime/clauderun/clauderun.go`.

In `subprocessEnv` (or wherever the per-dispatch env composition
lives), branch on the agent tier:

```go
// Pseudocode — adjust to the real opts shape.
needsEnvelope := opts.Category == "prod-agent"  // or whatever tier check
if needsEnvelope && opts.OutputFilePath == "" {
    opts.OutputFilePath = synthesisedEnvelopePath(opts)  // see Item 3
}
```

`synthesisedEnvelopePath` returns the same per-dispatch JSONL path
the existing outputs channel uses, just generated for the
no-outputs-declared case. The dispatcher's reader already treats a
missing file as an empty result (per the doc-comment at
`clauderun.go:332-336`), so a synthesised path that the agent never
writes to is a no-op downstream.

### Item 3 — Decide where the synthesised path lives

The existing outputs file lives at
`.gh-optivem/runs/<run-id>/<NNN>-<agent>.outputs.jsonl` (alongside
the prompt + events files — see the dir listing for the
2026-05-28 rehearsal run). The natural synthesised location is the
same file, just created on demand instead of pre-declared.

**Recommendation:** use the same filename pattern. The CLI reader
already knows how to parse this shape; the runtime side adds the
write-side symmetry.

### Item 4 — Surface envelope-only outputs in scope validation

`VALIDATE_OUTPUTS_AND_SCOPES` consumes the outputs file to detect a
`scope_exception` envelope and route accordingly (per
`process-flow.yaml:1325-1326` comment: "scope-exception-* ride the
same channel; the scope_exception_requested gate consumes them").

After Item 2, an envelope written by a no-MID-outputs agent
(`implement-system`, `update-system`) must be picked up by the same
gate. Verify:

- `validate-outputs-and-scopes` action reads the outputs file even
  when the MID declares no `outputs:` keys.
- The `scope_exception_requested` binding evaluates true when
  `scope-exception-files` is non-empty, regardless of whether any
  MID-declared output keys are present.

Likely no code change needed (the envelope keys are absence-tolerated
per the same comment), but the test gap below pins it.

### Item 5 — Test coverage

- `internal/atdd/runtime/clauderun/clauderun_test.go` —
  `TestSubprocessEnv_ExportsOutputFileForProdAgentsWithoutOutputsBlock`
  (or similar). Covers the new branch.
- `internal/atdd/runtime/actions/bindings_test.go` (or wherever
  `validate-outputs-and-scopes` is tested) — envelope is recognised
  on a MID that declares no `outputs:` keys.
- End-to-end: dispatch a fake `implement-system` agent that emits a
  `scope-exception-files` envelope, assert the `FIX` activity is
  *not* invoked and the `scope_exception_requested` gate routes the
  envelope through its declared handler.

### Item 6 — Regression check against rehearsal #71

After landing, re-run the 2026-05-28 rehearsal scenario **without**
the sibling migrations plan
(`20260528-1145-db-migrations-as-first-class-scope-key.md`) landed.
The `system-implementer` should:

1. Detect it needs a migration write.
2. Successfully emit `scope-exception-files=system/db/migrations/V…__add_gift_wrap.sql` via `gh optivem output write`.
3. Exit cleanly.
4. The orchestrator routes the envelope through `scope_exception_requested` (whatever its current handler does — operator approval, ticket escalation, etc.), **not** into `FIX`.

If the migrations plan also lands, the envelope path is exercised
only by *genuine* exceptions (e.g. driver-port edits) — which is the
correct steady state.

## Out of scope

- **The migration path key itself.** Covered by sibling plan
  `20260528-1145-db-migrations-as-first-class-scope-key.md`. That
  plan removes migration writes from being scope exceptions at all
  (they become an in-scope, declared write target). This plan
  ensures the envelope works for the *remaining* genuine exceptions
  on every writing-agent MID.
- **Read-scope envelopes.** The envelope today is used only for
  out-of-scope *writes*. Scope-bound reads also exist (per
  `scope.md`: "Reading or writing outside this set requires a
  `scope_exception` block"), but the runtime enforcement is on the
  write side. Read-side envelope support, if ever needed, is a
  separate plan.
- **Non-writing agents.** Reading-only or pure-utility agents (e.g.
  `acceptance-test-writer` is a writing agent; agents like the
  hypothetical `dry-run-checker` would not be) don't need the
  envelope and are excluded by the `category: prod-agent` gate in
  Item 2.
- **`outputs:` block ergonomics for declared flags.** The three
  MIDs that already declare `outputs:` (with `dsl-port-changed`
  etc.) continue to do so — those flags are load-bearing signals
  consumed by downstream gates and are not envelope-shaped. This
  plan does not propose unifying flags + envelope into one
  mechanism.

## Open questions to resolve before implementation starts

1. **Tier discrimination: `category: prod-agent` vs an explicit tag?**
   `category: prod-agent` is set on every writing-agent MID today and
   is the simplest discriminator. But it's a tier label, not a
   capability label — a future non-prod-agent MID that legitimately
   needs the envelope (e.g. a hypothetical writing-agent at a
   different tier) would be missed. Alternative: a new
   `envelope-channel: true` param at the MID level, defaulted on for
   `prod-agent`. **Tentative: stick with the `category: prod-agent`
   check** — simpler, and the envelope-needing tier is exactly the
   prod-agent tier today.

2. **Pre-create the outputs file vs lazy-create on first write?**
   The existing path doc-comments
   (`clauderun.go:332-336`) say "the file is NOT pre-created — when
   the agent makes no writes, the file simply does not exist after
   the run." Synthesising a path that the agent then never writes to
   leaves an empty filesystem state, which is fine. Pre-creating it
   to an empty file changes that. **Tentative: lazy** — preserves
   the existing semantics. The synthesised path exists in the env
   var only; the file is born when (if) the agent writes the first
   envelope line.

## References

- `internal/atdd/runtime/clauderun/clauderun.go:1629-1654` —
  `subprocessEnv()`, the gate that currently couples envelope
  availability to outputs-block presence.
- `internal/atdd/runtime/clauderun/clauderun.go:332-336` — comment
  on the lazy-creation semantics of the outputs file.
- `internal/atdd/runtime/statemachine/process-flow.yaml:1325-1337` —
  precedent `outputs:` block on `implement-and-verify-acceptance-tests`
  (showing the envelope keys alongside flag outputs).
- `internal/atdd/runtime/statemachine/process-flow.yaml:1447-1468` —
  `implement-system` MID, the canonical example of a writing-agent
  with no `outputs:` block.
- `internal/assets/runtime/shared/scope.md` — the prompt chunk that
  tells agents to use the envelope as the universal scope-exception
  escape hatch (the contract this plan makes runtime-true).
- `2026-05-28T09:35:29Z` event in
  `.gh-optivem/runs/20260528-092225/010-system-implementer.events.jsonl`
  — captures the exact `GH_OPTIVEM_OUTPUT_FILE is not set` failure
  for the gift-wrap migration write.
- Sibling plan `plans/upcoming/20260528-1145-db-migrations-as-first-class-scope-key.md`
  — orthogonal fix: removes migration writes from being scope
  exceptions at all.
