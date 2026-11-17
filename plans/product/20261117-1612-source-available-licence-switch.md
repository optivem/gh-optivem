# 2026-11-17 15:12:41 UTC — Switch gh-optivem to a source-available licence before the HTML views ship

> Related: `20261117-1547-local-html-visualizations.md` (the HTML views are the first features released under the new licence) and `20261117-1547-greenfield-use-case-modelling.md`.

## TL;DR

**Why:** gh-optivem is AGPL-3.0, which lets any company run it unchanged, including in CI, without paying. Once the docs site and coverage views exist, a company could publish them from CI for free, so the planned paid team tier has nothing to charge for. Valentina is the only human author (2403 commits; the rest are `github-actions[bot]`), so future versions can be relicensed.
**End result:** from a named release, gh-optivem is source-available. It is free for individuals, education and small teams, and companies above a size limit need a paid commercial licence. The paid team features (publishing, history across runs, multi-repo dashboards) are reserved for a separate closed service, so they never land in the CLI. Versions up to v1.6.108 stay AGPL-3.0.

## Outcomes

- `LICENSE` holds the new licence text. A `LICENSE-AGPL` note, or the README, records that releases up to the last AGPL tag remain AGPL-3.0.
- The README `## License` section explains in plain words who uses it for free, who must buy, and how to buy.
- `gh optivem --help` (root `Long` text) carries a one-line licence notice.
- A `COMMERCIAL.md`, or a README section, describes the commercial licence and the contact or purchase path.
- A `CONTRIBUTING.md` requires a contributor licence agreement (CLA) before outside pull requests are merged, so Optivem keeps the right to sell commercial licences.
- The HTML visualizations plan marks "Publishing the docs site from CI" and "history across runs" as **reserved for the paid tier**, never to be added to the CLI.
- There is no licence-key check in the CLI at first: enforcement is by licence terms. A key check is reconsidered only if abuse is seen.

## ▶ Next executable step (resume here)

Run `/refine-plan` on this file to settle the open questions (the licence choice and the size limit matter most). Then execute Step 1.

## Steps

- [ ] Step 1: Pin the cutover. Record the last AGPL release tag (currently `v1.6.108`, so re-check at execution time) and name the first source-available release in this plan.
- [ ] Step 2: Write the new `LICENSE`, using the licence chosen under Open questions, with the size limit filled in. Keep a short note naming the AGPL-3.0 tag range, so earlier versions keep their licence.
- [ ] Step 3: Rewrite the README `## License` section to cover:
  - free use: individuals, education and teams up to N developers
  - paid use: companies above N
  - Optivem course students, if an exemption is chosen
  - how to buy
  - that versions up to the last AGPL tag remain AGPL-3.0
- [ ] Step 4: Add `COMMERCIAL.md` with the commercial licence terms summary and the purchase or contact path, and link it from the README.
- [ ] Step 5: Add `CONTRIBUTING.md` with the CLA requirement, and a CLA check if a GitHub CLA bot is chosen.
- [ ] Step 6: Add a one-line licence notice to the root command `Long` help text in `main.go`, and update `docs/cli-reference.md` if it mirrors the root help.
- [ ] Step 7: Add a short decision section to `20261117-1547-local-html-visualizations.md` (Out of scope). It states that CI publishing, history across runs and dashboards across repos are reserved for the closed paid service, and that the CLI's HTML output stays a local, single-repo snapshot.
- [ ] Step 8: Check every other licence mention in the repo and update each one. Look at the `go.mod` module comments, the scaffolded templates that copy a licence into student repos (those must keep their own licence, so don't relicense student projects), and the `gh extension` metadata.

## Verification (operator, not agent work)

- A lawyer reviews the final licence text, the size limit wording and the commercial terms before the first release under the new licence.
- Announce the change to existing users (README note, release notes, Optivem Circle) when the first source-available release ships.

## Open questions

- **Which licence?** Recommended: **PolyForm Small Business 1.0.0**. It is a standard, lawyer-written licence that is free below a size limit (fewer than 100 people and under about $1M revenue, as published), so it needs no custom legal text. The alternative is a custom "free up to N developers" licence, which gives finer control but needs its own legal review. FSL / Fair Source does not stop internal CI use, so it does not solve the problem on its own.
- **Size limit:** accept PolyForm's built-in threshold, or set a developer-count limit (e.g. 10 developers) in a custom licence? Recommended: PolyForm's built-in threshold for the first release, since it is ready-made.
- **Students:** add an explicit free-use grant for Optivem course participants and education? Recommended: yes, since the tool is part of the teaching.
- **CLA mechanism:** a GitHub CLA bot (e.g. CLA Assistant), or a CLA paragraph in `CONTRIBUTING.md` plus a manual check? Recommended: a paragraph plus a manual check while there are no outside contributors.
- **Purchase path:** contact form or email only for now, or a pricing page? Recommended: contact only until the paid tier exists.
