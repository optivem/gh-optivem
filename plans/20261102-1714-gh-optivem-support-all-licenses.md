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

In this repo, find the license validation (search for `must be one of`) and the `licenses/*.txt` embed. Record the valid-key list, where templates are rendered, and how `copyright-holder` is substituted. Answer the "bundle vs fetch" open question before changing any code.

## Steps

- [ ] Step 1: In this repo, find the license key list, the embedded `licenses/*.txt` templates, the `LICENSE` writer and the `copyright-holder` substitution.
- [ ] Step 2: Decide how to get the license texts (see Open questions), then add the missing licenses: at minimum agpl-3.0, plus the rest of GitHub's list.
- [ ] Step 3: Change validation to derive the valid keys from the available templates, so adding a license no longer means editing a hard-coded list.
- [ ] Step 4: Mark which licenses get `copyright-holder` substituted and which ship unchanged.
- [ ] Step 5: Add tests: every key validates, every key produces a non-empty `LICENSE`, the copyright holder is substituted where expected, and an unknown key fails with the full list.
- [ ] Step 6: Update the gh-optivem docs or config comments that list the valid `license` values.
- [ ] Step 7: Release a new gh-optivem version, then run `gh extension upgrade optivem` locally.
- [ ] Step 8: In the event-hub repo (outside this repo, at `$(git rev-parse --show-toplevel)/../../event-hub`), set `gh-optivem.yaml` to `license: agpl-3.0`, remove the comment and check that `gh optivem` commands still load the config.

## Open questions

- **Bundle or fetch the license texts?** Bundling keeps scaffolding offline and deterministic. Fetching from GitHub's `/licenses/{key}` API covers every license with no upkeep, but needs network and auth. My suggestion is to bundle, using a small script that refreshes the bundled files from the API.
- **Which licenses count as "all"?** GitHub's API list (about 13 keys), or the full SPDX list (600+ keys)? My suggestion is GitHub's list plus the current extras (mit-0, 0bsd).
- **Allow source-available licenses (FSL, BUSL)?** GitHub doesn't offer them, but FSL came up as an option for EventHub. Include them now, or wait until they're needed?
- **Should keys accept SPDX ids (`AGPL-3.0-only`) as well as GitHub keys (`agpl-3.0`)?**
