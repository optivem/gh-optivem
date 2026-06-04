# Narrow `implement-system-driver-adapters` write-scope to the per-channel member

**Created:** 2026-06-04 12:21 CEST
**Status:** Ready to execute — **decision locked to Option A** (implicit
narrowing in the resolver). The executor may override to B/C, but A is the
recommendation; no open questions block execution.

Spun off from `plans/20260604-0955-configurable-per-channel-adapter-folders.md`
(its final Item 5). That plan's Items 1–4, 6, 7 shipped in gh-optivem
`e752629`; this is the one remaining piece, carved out because it lives in a
**different subsystem** (the runtime scope checker / `process-flow.yaml`) than
the shipped config work and turns on a single design decision.

## TL;DR

**Why:** The per-channel System Driver adapter members are now configured and
read verbatim by the resume guard (`scoped.go`), but the *live* write-scope
check (`validate-outputs-and-scopes`) still resolves
`implement-system-driver-adapters`' `write: [system-driver-adapter]` to the
**whole** adapter layer. So a channel-split dispatch (e.g. `--channel api`) is
permitted to write any channel's folder, not just its own.
**End result:** when a dispatch carries `ctx.Params["channel"]`, the scope
check narrows the adapter layer to that channel's configured member
(`system-driver-adapter-channels.<ch>`), so it can only write its own folder.
No-`channels:` full runs keep the whole-layer scope unchanged.

## Background / why this is a choice

`implement-system-driver-adapters` (in
`internal/atdd/runtime/statemachine/process-flow.yaml`) carries a single static
`write: [system-driver-adapter]`. At runtime the live scope checker
`validate-outputs-and-scopes` (`internal/atdd/runtime/actions/bindings.go`
~1049) resolves it via `ResolveLayerPaths(write, cfg)` and prefix-checks the
dispatch's diff against the result.

**Shared-node constraint.** That same phase node is reused by BOTH:
- the per-channel dispatch — `ctx.Params["channel"]` set, via the
  `UnrollSystemDriverAdapterChannels` clone (`statemachine/channels.go`); and
- the no-`channels:` full run — no channel param.

One static `write:` line must serve both, so the narrowing **must** be
conditional on the runtime channel; it cannot be baked statically into the
line. This is what makes the mechanism a design choice rather than a one-liner.

**`check-phase-scope` is dormant** (no `phase-id` / `check_phase_scope` node is
wired in `process-flow.yaml`), so the only live consumer to teach is
`validate-outputs-and-scopes`. Keep `check-phase-scope` consistent with
whatever is chosen, but it has no live effect today.

**Lower urgency (why this was safe to split off):** the change only *tightens*
an already-working check — today's whole-layer scope is permissive, not wrong —
and the resume guard (0955 Item 4, shipped) already keys on the real member. So
this is defense-in-depth, not a correctness fix.

## Decision (locked: Option A — implicit narrowing in the resolver)

Keep `write: [system-driver-adapter]` in `process-flow.yaml`. In the
scope-checker code, after `ResolveLayerPaths` resolves the write set,
post-process: **if `ctx.Params["channel"]` is non-empty, replace the resolved
`system-driver-adapter` path with that channel's member**
(`cfg.SystemTest.SystemDriverAdapterChannels[channel]`, also available as the
`system-driver-adapter-channels.<ch>` key in `PlaceholderMap`). No channel
param → whole layer, unchanged.

**Why A over B/C:**
- **B (parameterized token** — `write: [system-driver-adapter-channels.${channel}]`
  + a params-aware `ResolveLayerPaths` that substitutes `${channel}`): more
  self-documenting in `process-flow.yaml`, but the shared-node constraint means
  it still needs a whole-layer fallback for the no-channel run — so it does NOT
  remove the conditional, it just spreads it across more places (token +
  params-aware resolver + a preflight-sweep `${...}`-skip in
  `runScopeResolutionChecks`). Same branch, larger surface area.
- **C (defer/drop):** legitimate given the low urgency, but the tightening is
  cheap under A and closes the permissive gap, so do it.
- A does the same job with the least surface area; a one-line comment on the
  `write:` entry in `process-flow.yaml` neutralises the "magic" objection.

## Items

1. **Narrow the resolved write-scope by channel (Option A).** In
   `validate-outputs-and-scopes` (`internal/atdd/runtime/actions/bindings.go`),
   after `allowed, err := ResolveLayerPaths(write, cfg)`, if
   `ctx.Params["channel"]` is non-empty, substitute the channel's member for
   the `system-driver-adapter` entry in `allowed`. Factor the substitution into
   a small helper so the dormant `check-phase-scope` (~473) can call the same
   thing and stay consistent. Pin the lookup to
   `cfg.SystemTest.SystemDriverAdapterChannels[channel]` (verbatim read);
   guard the missing-member case (channel set but no member) with a clear error
   rather than silently widening to the whole layer.

2. **Comment the shared node.** Add a one-line note on
   `implement-system-driver-adapters`' `write: [system-driver-adapter]` in
   `process-flow.yaml` recording that the scope checker narrows it to the
   channel member when a `${channel}` param is present (so the implicit
   behaviour is discoverable from the YAML).

3. **Tests.** Cover: (a) channel param set → `allowed` is the member, not the
   whole layer; (b) no channel param → whole layer unchanged; (c) channel set
   but member missing → error (not silent widen). Reuse the `scoped_test.go`
   fixture shape (`SystemDriverAdapterChannels` populated).

4. **Audit the no-channel and preflight paths stay green.** Confirm the
   preflight static scope sweep (`runScopeResolutionChecks`, which has no
   channel context) is unaffected — under A it resolves `system-driver-adapter`
   to the whole layer exactly as today, so no `${...}`-skip is needed (that
   cost only existed under B).

## Notes / constraints

- **Read members verbatim** — no runtime path construction (the doctrine 0955
  established; the `path.Join(root, channel)` smell was removed in 0955 Item 4).
- The members live **outside** the flat `Paths` map / `CanonicalPathKeys()`;
  resolve them via `SystemDriverAdapterChannels` or the dotted
  `system-driver-adapter-channels.<ch>` placeholder key, never by iterating
  `Paths`.
- Do **not** change the static `write:` list (that's Option B); keep the
  narrowing in the resolver.

## Related

- `plans/20260604-0955-configurable-per-channel-adapter-folders.md` — parent;
  shipped the schema/scaffolder/validator/footprint/preflight/docs this builds
  on (deleted once this spun off).
- `plans/20260530-1725-scoped-implement-by-layer-channel.md` — the scoped
  `implement` work that introduced the `${channel}` param and the resume guard
  this complements.
- `internal/atdd/runtime/actions/bindings.go` — `validate-outputs-and-scopes`
  (live), `check-phase-scope` (dormant), `ResolveLayerPaths`.
- `internal/atdd/runtime/statemachine/channels.go` —
  `UnrollSystemDriverAdapterChannels` (binds the `channel` param per dispatch).
