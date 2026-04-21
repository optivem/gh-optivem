# Plan: Pin optivem/shop version in gh-optivem releases

## Status

Phases A, B, C are implemented:
- `ShopRef`/`ShopTag` in [internal/version/version.go](../internal/version/version.go) and [internal/config/config.go](../internal/config/config.go)
- `verified-shop-sha` commit-status flow in [.github/workflows/gh-acceptance-stage.yml](../.github/workflows/gh-acceptance-stage.yml)
- ldflag injection + status-read in [.github/workflows/gh-release-stage.yml](../.github/workflows/gh-release-stage.yml) and [.goreleaser.yml](../.goreleaser.yml)

Manual verification items (A5, B4, C5) and Phase D (verify-release-chain workflow,
which was never created) have been removed from this plan. Only the optional
cleanup of pre-pinning releases remains.

## Items

- [ ] **Delete previously-released gh-optivem binaries**
  All releases cut before this work track shop HEAD (non-deterministic) and have no `verified-shop-sha` lineage. Rather than migrate them, delete them so users only ever install pinned, validated builds.
  - List existing releases: `gh release list --repo optivem/gh-optivem`
  - Delete each: `gh release delete <tag> --repo optivem/gh-optivem --cleanup-tag --yes`
  - Stop and confirm with user before running deletions — this is destructive and visible to anyone who installed a prior version.

## Out of scope

- Auto-bumping shop SHA in gh-optivem on shop merges (a future workflow that watches shop merges, re-runs acceptance-stage-full, and opens a PR to update the pin — keeps gh-optivem from drifting behind shop without manual intervention. Defer until the manual flow proves itself.)
- One-click release via verify-release-chain (rejected 2026-04-17). Keeping scheduled-verify (nightly acceptance-stage-full writes commit status) + manual-tag-push release as the sole path. Release cadence is low, tag strategy is thorny, and decoupling "green" from "ship" is valuable. If tag friction becomes painful later, a thin `release.yml` dispatch wrapper is cheaper than baking release into verify-release-chain.
