# 2026-11-02 16:14:51 UTC — gh-optivem: support all common licenses

## TL;DR

**Why:** EventHub moved to AGPL-3.0, but `gh-optivem.yaml` still says `license: mit` with a workaround comment. `gh optivem` checks the field against a fixed list (`config: license %q must be one of %s`) and only has 8 bundled templates: mit, mit-0, apache-2.0, gpl-3.0, bsd-2-clause, bsd-3-clause, 0bsd and unlicense. AGPL isn't one of them.
**End result:** `gh optivem` accepts every license GitHub supports, including agpl-3.0, and writes the correct `LICENSE`. EventHub's `gh-optivem.yaml` says `license: agpl-3.0`, with no workaround comment.

## Outcomes

- `license: agpl-3.0` in `gh-optivem.yaml` passes validation and scaffolds a correct AGPL-3.0 `LICENSE`.
- Every license key GitHub supports is accepted: agpl-3.0, gpl-2.0, gpl-3.0, lgpl-2.1, lgpl-3.0, mpl-2.0, epl-2.0, bsl-1.0, apache-2.0, mit, bsd-2-clause, bsd-3-clause, unlicense and cc0-1.0. The current extras, mit-0 and 0bsd, keep working.
- The error message for an unknown key lists all valid keys.
- Licenses that use a copyright-holder placeholder get it filled from `copyright-holder`. Licenses meant to ship unchanged (GPL family, Apache, MPL, etc.) are written unchanged.
- EventHub's `gh-optivem.yaml` reads `license: agpl-3.0`, and the comment is gone.

## ▶ Next executable step (resume here)

Steps 1–6 are done and pushed (commit e1008306). The rest is waiting on the operator: run gh-acceptance-stage and then gh-release-stage to promote the RC that the commit stage built from that push, then run `gh extension upgrade optivem`. After that, run Step 8.

## Steps

- [ ] Step 7: Release a new gh-optivem version, then run `gh extension upgrade optivem` locally. — ⏳ Deferred: pushed only (2026-11-02); operator triggers acceptance and release stages.
- [ ] Step 8: In the event-hub repo (outside this repo, at `$(git rev-parse --show-toplevel)/../../event-hub`), set `gh-optivem.yaml` to `license: agpl-3.0`, remove the comment and check that `gh optivem` commands still load the config.

## Decisions (resolved 2026-11-02)

- **Bundle or fetch:** bundle the texts as embedded `licenses/*.txt`, plus a refresh script that pulls them from GitHub's `/licenses/{key}` API.
- **Scope:** GitHub's API list plus mit-0 and 0bsd.
- **FSL/BUSL:** not now. A later addition is a single `.txt` file.
- **SPDX ids:** not accepted. GitHub keys only.
