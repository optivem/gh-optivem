# Plan: shop meta-prerelease opt-in to test against a gh-optivem ref (HEAD)

**Target repo:** `optivem/shop` (all edits land in `../shop`, authored here per the
gh-optivem `plans/` convention — shop has no `plans/` dir).

**Status:** Executed 2026-06-06 — all 24 files edited in `../shop` (uncommitted).
actionlint clean on all 23 workflows; composite action validated; 17 install
sites routed through `install-gh-optivem`; safety gate in place. Pending: commit
+ live dry-run verification (see Verification).

## Why

Today every shop pipeline installs the gh-optivem CLI with
`gh extension install optivem/gh-optivem`, which pulls the **latest goreleaser
release**. That correctly mirrors the real student/teacher flow (they install the
released extension), so it must stay the default everywhere.

But before cutting a gh-optivem release we sometimes want to exercise shop's
*real* prerelease pipeline (real shop systems, real ATDD acceptance + legacy +
cross-lang + drift) against **unreleased gh-optivem** — main HEAD, a feature
branch, or a specific SHA. gh-optivem's own pipeline has the mirror knob
(`debug-shop-head`: test gh-optivem-HEAD against shop-HEAD, forced
`skip-prerelease=true`); shop currently has no symmetric knob.

This plan adds an opt-in `gh-optivem-ref` string input, **on the dry-run entry
point only**, that builds gh-optivem from the given ref instead of installing the
release. Empty (the default everywhere) = today's behavior exactly.

## Design decisions (resolved)

- **Host = dry-run only.** The input is added to `meta-prerelease-dry-run.yml`,
  never to `meta-prerelease-stage.yml`. The stage mints the `meta-v*-rc` tag that
  `meta-release-stage` promotes to a real release; a HEAD-verified run must never
  be able to mint that tag (else shop ships artifacts validated against an
  unreleased gh-optivem). Enforced by gating `trigger-stage` on
  `gh-optivem-ref == ''` (see Item 2) — symmetric to gh-optivem forcing
  `skip-prerelease=true` under `debug-shop-head`.
- **Shape = arbitrary ref (string), default `''`.** `''` = latest release
  (unchanged). Any branch/tag/SHA = build gh-optivem from source at that ref.
  Covers "main HEAD" plus feature-branch / pinned-SHA testing.
- **Mechanism = build from source, in one shared composite action.**
  `gh extension install --pin` needs a release asset at the ref; `main` has none.
  So a non-empty ref means: setup Go → `git clone optivem/gh-optivem` at the ref →
  `go build -o gh-optivem .` → `gh extension install .` (the same dance as this
  repo's `scripts/install.sh`). To avoid copying that branch into ~9 install
  sites, it lives in one composite action that every site calls.

## Threading map (entry → leaf)

```
meta-prerelease-dry-run.yml            [+input, gate trigger-stage]
  → _meta-prerelease-pipeline.yml      [+input; install in `check`; forward to variant + cross-lang + drift]
      → prerelease-pipeline-{variant}.yml  (×6) [+input; forward]
          → _prerelease-pipeline.yml   [+input; install in `local`; forward to acceptance + legacy]
              → {prefix}-acceptance-stage.yml         (×6) [+input; install]
              → {prefix}-acceptance-stage-legacy.yml  (×6) [+input; install]
      → cross-lang-system-verification.yml [+input; install]
      → drift.yml                          [+input; install]
```

Install sites that consume the ref (replace the inline `gh extension install`
step with the composite action): `_meta-prerelease-pipeline.yml` `check`,
`_prerelease-pipeline.yml` `local`, the 6 acceptance-stage, the 6
acceptance-stage-legacy, `cross-lang-system-verification.yml`, `drift.yml` = 16
install sites, all routed through the one composite action.

Note: `*-commit-stage.yml` workflows do **not** install gh-optivem, so the
`commit` job in `_meta-prerelease-pipeline.yml` is left untouched.

## Items

### Item 1 — New composite action `install-gh-optivem` (NEW FILE)

Create `../shop/.github/actions/install-gh-optivem/action.yml`:

- Input `ref` (string, default `''`).
- Input `github-token` (required) — passed to `gh` as `GH_TOKEN`.
- Step A (`if: ref == ''`): current behavior — `gh extension install
  optivem/gh-optivem`, wrapped in `optivem/actions/retry@v1` (match the existing
  install steps, which already use retry).
- Step B (`if: ref != ''`), in order:
  - `actions/setup-go@v6` with `go-version-file` pointing at the cloned repo's
    `go.mod` (clone first, or use a fixed `go-version`; prefer cloning first then
    `go-version-file: <clone>/go.mod` so the build always matches gh-optivem's
    pinned toolchain).
  - clone: `git clone https://github.com/optivem/gh-optivem "$RUNNER_TEMP/gh-optivem" && git -C "$RUNNER_TEMP/gh-optivem" checkout "<ref>"` (wrap the network step in `optivem/actions/retry@v1`).
  - build: `( cd "$RUNNER_TEMP/gh-optivem" && go build -o gh-optivem . )` — binary
    name must be exactly `gh-optivem` for `gh extension install <dir>` to find it.
  - install: `gh extension remove optivem 2>/dev/null || true; gh extension
    install "$RUNNER_TEMP/gh-optivem"`.
  - echo the resolved `gh optivem --version` into the step summary so every run
    records which gh-optivem it actually used (release vs `dev-<sha>`).
- Match the house style of the existing composite actions under
  `../shop/.github/actions/` (see `build-flavor-rc-manifest/action.yml`).

### Item 2 — `meta-prerelease-dry-run.yml`

- Add `workflow_dispatch` input `gh-optivem-ref` (string, default `''`,
  description: "Build & install gh-optivem from this ref (branch/tag/SHA) instead
  of the latest release. Empty = latest release. When set, the run will NOT
  auto-trigger meta-prerelease-stage, so it can never mint a meta-rc tag.").
- Add `[gh-optivem-ref:{0}]` segment to `run-name` when non-empty (match the
  existing optional-segment style).
- Forward `gh-optivem-ref: ${{ inputs.gh-optivem-ref }}` into the
  `_meta-prerelease-pipeline.yml` call `with:`.
- **Safety gate:** change the `trigger-stage` job `if:` from
  `success() && inputs.auto-trigger-stage` to
  `success() && inputs.auto-trigger-stage && inputs.gh-optivem-ref == ''`, and add
  a `summarize` note when skipped-due-to-ref so it's visible why no stage fired.
- `build-manifest-artifact` is read-only preview — leave unchanged.

### Item 3 — `_meta-prerelease-pipeline.yml`

- Add `workflow_call` input `gh-optivem-ref` (string, default `''`).
- `check` job: replace the inline "Install gh-optivem CLI extension" step with
  `uses: ./.github/actions/install-gh-optivem` passing `ref` + `github-token`.
- `local` job: add `"gh-optivem-ref": "${{ inputs.gh-optivem-ref }}"` to the
  `workflow-inputs` JSON for `prerelease-pipeline-${{ matrix.variant }}.yml`.
- `pipeline` job: same addition to its `workflow-inputs` JSON.
- `cross-lang` job: forward `gh-optivem-ref: ${{ inputs.gh-optivem-ref }}` into the
  `cross-lang-system-verification.yml` `with:`.
- `drift` job: forward `gh-optivem-ref: ${{ inputs.gh-optivem-ref }}` into the
  `drift.yml` `with:`.
- `commit` job: untouched (commit stages don't install gh-optivem).

### Item 4 — `prerelease-pipeline-{variant}.yml` (×6)

`monolith-java`, `monolith-dotnet`, `monolith-typescript`, `multitier-java`,
`multitier-dotnet`, `multitier-typescript`. For each:

- Add `workflow_dispatch` input `gh-optivem-ref` (string, default `''`).
- Forward `gh-optivem-ref: ${{ inputs.gh-optivem-ref }}` into the
  `_prerelease-pipeline.yml` call `with:`.
- (Optional) add the `run-name` segment for parity with Item 2.

### Item 5 — `_prerelease-pipeline.yml`

- Add `workflow_call` input `gh-optivem-ref` (string, default `''`).
- `local` job: replace the inline install step with the composite action
  (`ref` + `github-token`).
- `acceptance-stage` job: add `"gh-optivem-ref": "${{ inputs.gh-optivem-ref }}"`
  to the `workflow-inputs` JSON for `{prefix}-acceptance-stage.yml`.
- `acceptance-stage-legacy` job: same addition for
  `{prefix}-acceptance-stage-legacy.yml`.

### Item 6 — `{prefix}-acceptance-stage.yml` (×6) + `{prefix}-acceptance-stage-legacy.yml` (×6)

12 leaf workflows. For each:

- Add `workflow_dispatch` input `gh-optivem-ref` (string, default `''`). (These
  also run on cron/push; those paths get the default `''` = release, unchanged.)
- Replace the "Install gh-optivem CLI extension" step with the composite action
  passing `ref: ${{ inputs.gh-optivem-ref }}` + `github-token`.

### Item 7 — `cross-lang-system-verification.yml` + `drift.yml`

- Add `workflow_call` input `gh-optivem-ref` (string, default `''`) to each.
- Replace each install step with the composite action.

## Verification (user-driven — not agent steps)

- `actionlint` over all 24 touched shop workflows (shop's own `lint-workflows.yml`
  covers this on push; can also run locally).
- Confirm the default path is byte-for-byte behavior-equivalent: a normal
  scheduled/dispatch run with `gh-optivem-ref` empty must still `gh extension
  install optivem/gh-optivem` everywhere (no Go build, no clone).
- Dispatch `meta-prerelease-dry-run.yml` with `gh-optivem-ref: main`,
  `variant: monolith-typescript` (cheapest), `skip-acceptance-legacy: false` to
  exercise both leaf install paths. Confirm in the step summaries that every
  install site reports a `dev-<sha>` gh-optivem version, and that `trigger-stage`
  is **skipped** (proving a HEAD run cannot cascade to the real stage).
- Confirm a `gh-optivem-ref` pointing at a non-existent ref fails fast in the
  composite action's clone/checkout step with a clear error.

## Notes / open questions

- The composite action clones over HTTPS using the job's `GITHUB_TOKEN`. gh-optivem
  is a public repo, so an unauthenticated clone also works; keep the token for
  rate-limit headroom on the busy acceptance matrix.
- Cross-reference: this is the shop-side mirror of gh-optivem's `debug-shop-head`
  input in `.github/workflows/_gh-acceptance-pipeline.yml`.
