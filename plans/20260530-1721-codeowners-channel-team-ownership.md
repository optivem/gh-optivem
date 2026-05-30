# CODEOWNERS: channel → team enforcement (DEFERRED)

**Status:** deferred — do not pick up yet
**Created:** 2026-05-30 17:21 CEDT

## Why deferred

The operator explicitly wants the team split handled as an **informal
agreement** first (a written ownership convention, no enforcement), and does
**not** yet want code segregation by permissions. This plan captures the
*enforcement* variant for later, once the informal agreement has been lived-in
and the channel→team mapping is stable.

Do not start this until the operator says permission-based enforcement is
wanted.

## Goal (when picked up)

Generate a `CODEOWNERS` file so that PR review routing enforces the same
channel/team boundary that the informal agreement establishes:

- `api` channel artifacts → backend team
- `ui` channel artifacts → frontend team
- channel-agnostic (shared) layers → **both** teams must approve

## Dependency

Requires a declared **channel → team mapping** as a source of truth. That
mapping is the open decision deferred from the discussion (enrich `channels:`
entries with `team:`, vs. a sibling `ownership:` block, vs. hand-authored
CODEOWNERS). Resolve that first — it is the same SSoT the informal-agreement
doc will reference, so reuse it rather than inventing a second one.

## Path globs (the mapping CODEOWNERS encodes)

Derived from the four-layer ATDD stack. Channel-split layers route per team;
channel-agnostic layers route to both.

| Glob (per scaffolded project) | Channel | Owner |
| --- | --- | --- |
| `driver-adapter/api/**`, `system/**/api/**` | api | backend |
| `driver-adapter/ui/**`, `system/**/ui/**` | ui | frontend |
| `dsl-port/**`, `dsl-core/**`, `driver-port/**` | *(shared)* | backend **and** frontend |

(Resolve the exact globs against the canonical Family-B path keys in
`internal/projectconfig/path-keys.md` at execution time — do not hardcode
`shop`-specific layout.)

## Open questions to resolve at pickup

1. **Where is CODEOWNERS scaffolded?** Per scaffolded student repo (so each
   project gets one), or only in the operator's own project? If scaffolded,
   it becomes SSoT-codegen from the channel→team mapping, sibling to the
   `ChannelType.*` generation in
   `plans/20260530-1702-channels-field-channel-by-channel.md` (D3).
2. **Team handles.** CODEOWNERS needs real `@org/team` handles, which are
   org-specific — must come from config or a prompt, not hardcoded.
3. **Shared-layer "both teams" semantics.** GitHub CODEOWNERS lists multiple
   owners as *any-one-approves* by default; requiring *both* teams needs branch
   protection "require review from Code Owners" plus per-team required reviews.
   Confirm the org's branch-protection model supports the intended "both must
   sign off" before promising it.

## Related

- `plans/20260530-1702-channels-field-channel-by-channel.md` — the `channels:`
  SSoT + channel-by-channel system implementation this builds on. The channel
  axis there **is** the team boundary here.
- The informal ownership agreement (no-enforcement convention) — the
  not-deferred sibling the operator wants first.
