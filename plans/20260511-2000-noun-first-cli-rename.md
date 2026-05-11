# Noun-first CLI rename — `gh optivem <noun> <verb>`

> **Paired plan.** The hidden deprecation aliases registered by this plan
> are dropped in [`20260511-2010-drop-verb-first-aliases.md`](20260511-2010-drop-verb-first-aliases.md)
> one release later.

## Remaining work

- [ ] **Step 10 — Shop template (separate repo).** ⏳ Deferred: requires this
  PR to merge and a tagged gh-optivem release to ship first. The
  deprecation aliases keep the old forms working in the shop until then.
  Sequence:
  1. Merge this PR (new noun-first tree + hidden verb-first aliases).
  2. Cut a gh-optivem release that students can `gh extension upgrade
     optivem` to.
  3. Open a separate shop PR updating every callsite to the new forms
     (workflows, README, helper scripts). Find them with
     `grep -rEn 'gh optivem (build|run|stop|clean) system\b|gh optivem (test|compile) (system|system-tests)\b|gh optivem verify tokens\b'`
     in the shop checkout.
  4. Tag a new `meta-v*` release pinning the new shop SHA (see shop's
     `CONTRIBUTING.md`).

## Rename mapping (reference for Step 10)

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
