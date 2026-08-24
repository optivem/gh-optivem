# 2026-08-24 11:19 CEST — `WriteLicense`: silent skip, untemplated manifests, unfilled placeholders

## TL;DR

**Why:** Scaffolded repos can ship with no LICENSE at all (a transient `gh api` failure is
downgraded to a warning), with a `package.json` license field that contradicts the LICENSE file,
and — for `mit`/`bsd-*` — with the copyright line left as literal `[year] [fullname]`. The
fetch-from-GitHub design also structurally cannot offer `mit-0` or `0bsd`, because the licenses
API does not serve them.

**End result:** license texts are bundled as embedded assets rather than fetched, so generation
has no network dependency and no silent-skip path; the copyright placeholders are filled for the
licenses whose notice is inline and left untouched for the ones that forbid modification; and the
scaffolded package manifests declare the same license the LICENSE file grants.

## Problem 1 — a failed fetch silently produces an unlicensed repo

`internal/scaffolding/steps/finalize.go:27` shells out to `gh api licenses/<key> --jq .body`.
The switch at `:28-35` handles both failure modes by warning and returning:

```go
case err != nil:
    log.Warnf("Could not fetch license template %q: %v -- skipping LICENSE file", cfg.License, err)
    return
case body == "":
    log.Warnf("License template %q returned empty body -- skipping LICENSE file", cfg.License)
    return
```

The scaffold then continues and reports success. The generated repo has **no LICENSE file**, which
under copyright default means all rights reserved — the opposite of what the operator asked for.
The only trace is one warning line in a long scaffold log, and `main.go:424` registers the step
inside `phaseApplyTemplate` where it scrolls past.

This is the same failure shape the workspace rule for `check-*` actions rejects: an indeterminate
result (network blip, auth failure, rate limit) coerced into a benign-looking outcome. A license
that could not be written is not a license that was skipped on purpose.

**Fix:** remove the network call entirely (Problem 4) and treat a missing bundled asset as a hard
failure — `log.Errorf` + non-zero exit, naming the key and the asset path.

## Problem 2 — package manifests are never templated

Nothing under `internal/` rewrites a package manifest's license field. Grepping `"license"`,
`PackageLicense` and `licenseExpression` across `internal/**/*.go` returns only flag and config
plumbing (`internal/config/config.go:571`, `internal/config/optivemyaml/optivemyaml.go:60`) —
no writer.

So a scaffolded project inherits whatever the template repo carries. As of shop `6b69c9e6`, that
is:

| File | Declares |
|---|---|
| `system/multitier/backend-typescript/package.json` | `"license": "MIT-0"` |
| `system-test/typescript/package.json` | `"license": "MIT-0"` |

An operator running `--license apache-2.0` gets `LICENSE` = Apache-2.0 and `package.json` =
`MIT-0`. npm, SBOM tooling and GitHub's own license detection read the manifest, so the
machine-readable answer contradicts the legal one. (Before `6b69c9e6` the same fields said
`CC0-1.0` and contradicted MIT — the mismatch predates the relicense.)

**Fix:** template the manifest license field off `cfg.License` at scaffold time, alongside the
existing README-footer substitution at `internal/scaffolding/steps/readme.go:117`. Add `.csproj`
`PackageLicenseExpression` if/when the .NET projects declare one.

## Problem 3 — placeholders are written verbatim, but only some may be filled

`writeLicenseToDir` (`finalize.go:51-56`) writes `body` unchanged. The GitHub templates carry
placeholders, so `mit` scaffolds emit:

```
Copyright (c) [year] [fullname]
```

The naive fix — replace every known token spelling — is **wrong**. Checked against the live API:

| Key | Placeholder | Line | Context | Fill? |
|---|---|---|---|---|
| `mit` | `[year] [fullname]` | 3 | operative copyright notice | yes |
| `bsd-2-clause` | `[year], [fullname]` | 3 | operative copyright notice | yes |
| `bsd-3-clause` | `[year], [fullname]` | 3 | operative copyright notice | yes |
| `apache-2.0` | `[yyyy] [name of copyright owner]` | 189 | inside `APPENDIX: How to apply the Apache License to your work` (:178) | no |
| `gpl-3.0` | `<year> <name of author>` | 635, 655 | inside `How to Apply These Terms to Your New Programs` (:623) | no |
| `unlicense` | none | — | — | n/a |

For Apache-2.0 and GPL-3.0 the placeholders live in an instructional appendix describing how to
head your *source files*; the licence document itself ships verbatim. GPL-3.0 line 6 says so
outright: "of this license document, but changing it is not allowed."

**Fix:** gate substitution on the key, not the token.

```go
// licensesWithFillableNotice are the keys whose text carries the copyright
// notice inline, where the placeholder must be filled. Apache-2.0 and GPL-3.0
// carry placeholders only inside an instructional appendix and must ship
// verbatim — GPL-3.0 forbids modifying the document at all.
var licensesWithFillableNotice = map[string]bool{
	"mit": true, "mit-0": true, "bsd-2-clause": true, "bsd-3-clause": true,
}
```

**Decided — the copyright holder gets its own key.** `cfg.Owner`
(`internal/config/config.go:562`, "GitHub username or org (required)") is the only candidate
present today, but it is a handle rather than a legal entity, so it yields
`Copyright (c) 2026 optivem`. Add an optional `copyright-holder` key that falls back to
`cfg.Owner` when unset — zero-config stays zero-config, and a real name is expressible where it
matters.

Plumbing, mirroring how `license` itself is threaded:

| Site | Change |
|---|---|
| `internal/kernel/projectconfig/config.go` | add a `CopyrightHolder string` field with yaml tag `copyright-holder,omitempty`, beside `License` (:172) |
| `internal/config/config.go` | `--copyright-holder` flag, default `""`, beside the `--license` flag (:571) |
| `internal/config/optivemyaml/optivemyaml.go` | carry it through beside `License: cfg.License` (:60) |
| `internal/scaffolding/steps/finalize.go` | resolve `holder := cfg.CopyrightHolder; if holder == "" { holder = cfg.Owner }` |

Deliberately **not** added to `configinit`'s interactive prompt (`prompt.go:164` region) — it is
an optional refinement, and one more question in the init flow costs more than it returns. It is
set in `gh-optivem.yaml` by operators who care.

## Problem 4 — `mit-0` and `0bsd` cannot be offered at all

The offered set is `mit, apache-2.0, gpl-3.0, bsd-2-clause, bsd-3-clause, unlicense`
(`internal/config/config.go:571`, `internal/config/configinit/prompt.go:164`,
`internal/kernel/projectconfig/license.go`).

The GitHub licenses API serves only:

```
agpl-3.0 apache-2.0 bsd-2-clause bsd-3-clause bsl-1.0 cc0-1.0 epl-2.0
gpl-2.0 gpl-3.0 lgpl-2.1 mit mpl-2.0 unlicense
```

No `mit-0`, no `0bsd`. Since shop itself is now MIT-0 (`6b69c9e6`), the scaffolder cannot offer
the licence its own template uses. Note `cc0-1.0` *is* served, which is the likely origin of the
CC0 in the shop manifests.

**Fix:** bundle the texts under `internal/scaffolding/assets/licenses/<key>.txt` and embed with
`//go:embed`. This subsumes Problem 1 (no network), enables `mit-0`/`0bsd`, and makes the
Problem 3 rule a property of assets under our control rather than of upstream text that can
change silently.

## Verification

- Unit test per offered key: fillable keys leave no `[`/`<` placeholder after expansion; the
  others are byte-identical to the bundled asset.
- Unit test: an unknown or missing asset key fails the step rather than skipping it.
- Unit test: manifest license field matches `cfg.License` after templating.
- Unit test: `copyright-holder` wins when set; `cfg.Owner` is used when it is empty.
- Manual: `gh optivem init --license mit-0` produces a filled MIT-0 LICENSE, a matching
  `package.json` field, and a README badge agreeing with both.

## Related

- shop `6b69c9e6` — relicensed shop to MIT-0 and aligned the two `package.json` fields.
- `internal/scaffolding/steps/readme.go:117` — `readmeFooter(cfg.LicenseName(), cfg.Owner)`;
  the badge already derives from the same key and stays consistent.
