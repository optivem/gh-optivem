# Noun-first CLI rename — `gh optivem <noun> <verb>`

> **Paired plan.** The hidden deprecation aliases registered by this plan
> are dropped in [`20260511-2010-drop-verb-first-aliases.md`](20260511-2010-drop-verb-first-aliases.md)
> one release later.

## Remaining work

- [ ] **Step 10 — Shop release.** ⏳ Deferred: callsite migration was applied
  early (across all shop workflows, docs, plans, and helper scripts) but
  the published release steps still need a human:
  1. Cut a gh-optivem release that students can `gh extension upgrade
     optivem` to.
  2. Once the shop callsite PR is merged, tag a new `meta-v*` release
     pinning the new shop SHA (see shop's `CONTRIBUTING.md`).

## Rename mapping (reference)

| Old form | New form |
|---|---|
| `gh optivem build system [--rebuild]` | `gh optivem system build [--rebuild]` |
| `gh optivem run system [...]` | `gh optivem system start [...]` |
| `gh optivem stop system` | `gh optivem system stop` |
| `gh optivem clean system` | `gh optivem system clean` |
| `gh optivem compile system` | `gh optivem system compile` |
| `gh optivem test system [...]` | `gh optivem test run [...]` |
| `gh optivem compile system-tests` | `gh optivem test compile` |
| `gh optivem test setup` | `gh optivem test setup` *(unchanged)* |
| `gh optivem compile` *(bare, both tiers)* | `gh optivem compile` *(unchanged)* |
| `gh optivem verify tokens` | `gh optivem token verify` |
