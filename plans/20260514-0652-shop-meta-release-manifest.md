# Shop: decouple meta and flavor versions via prerelease-recorded manifest

## Context

`optivem/shop`'s `meta-release-stage.yml` has been failing on every cycle for
weeks (last success: `meta-v1.0.86-rc.305`, 2026-05-13 10:45 UTC; every
`meta-v1.0.87-rc.*` since has failed at the `check` job — runs 25801842099,
25817320202, 25818879005, 25835925382). The failure is identical every time:

> `No QA-approved RC (qa/signoff commit-status with state=success) found for
> monolith-java at v1.0.87. Was the acceptance pipeline run to completion and
> qa-signoff dispatched?`

### Root cause

Two version trackers are supposed to march in lockstep but have no mechanism
to keep them aligned:

| Layer | File | Bumped by |
|---|---|---|
| Meta | `VERSION` (root) | `meta-release-stage`'s `bump-patch-version-meta` job (only on full success) |
| Flavor (×6) | `system/<arch>/<lang>/VERSION` | each `<flavor>-prod-stage`'s `bump-patch-version` (on per-flavor success, including standalone `workflow_dispatch`) |

[`meta-release-stage.yml:175-179`](https://github.com/optivem/shop/blob/main/.github/workflows/meta-release-stage.yml#L175-L179)
in the `Find flavor RCs to promote` step does an exact-match lookup:

```bash
git tag --list "${flavor}-v${VERSION}-rc.*"
```

with `${VERSION}` taken from root `VERSION`. This implicitly assumes
`flavor.VERSION == meta.VERSION` for all 6 flavors.

Once any failure (transient or otherwise) prevents `meta-release-stage` from
completing while at least one flavor prod-stage runs to completion (either as
part of a partial meta-release or via direct `workflow_dispatch`), the flavor
bumps and the meta does not. Drift becomes permanent — the strict lookup
can never resolve, and there is no automatic recovery path.

Current state on `main` (verified 2026-05-14 06:50 UTC):

```
VERSION                                         1.0.87
system/monolith/java/VERSION                    1.0.88
system/monolith/dotnet/VERSION                  1.0.88
system/monolith/typescript/VERSION              1.0.88
system/multitier/java/VERSION                   1.0.88
system/multitier/dotnet/VERSION                 1.0.88
system/multitier/typescript/VERSION             1.0.88
```

Cleanup (`cleanup.yml`, daily at 23:00 UTC, `retention-days=1`) deletes any
`monolith-java-v1.0.87-rc.*` tags that did exist; new prereleases mint
`monolith-java-v1.0.88-rc.*` (because the flavor's component VERSION has
moved on). So the v1.0.87 flavor RC tags can be neither found nor regenerated.

### Design intent

Meta and flavor versions are **independent**. Each flavor moves at its own
cadence; meta is just a counter for the bundled-and-tested-together meta
releases. Ad-hoc `workflow_dispatch` of any single `<flavor>-prod-stage` is
a first-class workflow and must not break the meta path.

The contract `meta-release-stage` enforces should be:

> Meta-prerelease records the exact set of flavor RCs it tested together.
> Meta-release promotes exactly those RCs — no version-name lookup, no
> "latest QA-approved" substitution.

This eliminates the version-matching contract entirely. The version numbers
become labels; the manifest becomes the truth.

## Scope (shop repo: `../shop`)

### Phase 1 — Write the manifest at prerelease time

In `_meta-prerelease-pipeline.yml`'s `tag-meta-rc` job, after the per-flavor
pipelines and qa-signoffs have completed:

1. Collect the resolved RC tag for each flavor (already known at this point,
   since each flavor's pipeline emitted it).
2. Build a JSON manifest:

   ```json
   {
     "meta_rc": "meta-v1.0.87-rc.314",
     "tested_at": "2026-05-14T01:18:00Z",
     "flavors": {
       "monolith-java":        "monolith-java-v1.0.88-rc.1104",
       "monolith-dotnet":      "monolith-dotnet-v1.0.88-rc.1124",
       "monolith-typescript":  "monolith-typescript-v1.0.88-rc.1129",
       "multitier-java":       "multitier-java-v1.0.88-rc.1046",
       "multitier-dotnet":     "multitier-dotnet-v1.0.88-rc.1065",
       "multitier-typescript": "multitier-typescript-v1.0.88-rc.1066"
     }
   }
   ```

3. Create the meta-rc tag as **annotated**, with the manifest as the
   annotation body:

   ```bash
   git tag -a "meta-v${VERSION}-rc.${N}" -m "$(cat manifest.json)"
   git push origin "meta-v${VERSION}-rc.${N}"
   ```

The manifest travels with the tag, atomically. Read back with
`git cat-file -p <tag>` or `gh api repos/.../git/tags/<sha>`.

### Phase 2 — Read the manifest at release time

Rewrite the `Find flavor RCs to promote` step in `meta-release-stage.yml`:

1. `git cat-file tag <RC_TAG>` to extract the annotation.
2. Strip the standard tag header lines, parse the JSON body.
3. For each flavor in the manifest:
   - Re-verify `qa/signoff=success` is *still* present on the RC tag's commit
     (defensive — someone could revoke between prerelease and release).
   - Verify the RC tag still exists locally.
4. Emit `${flavor}-rc=<manifested-tag>` outputs as today.

The strict `<flavor>-v${VERSION}-rc.*` lookup, the version-matching check,
and any reliance on root `VERSION` for flavor resolution all disappear.

The downstream `promote-*` jobs and `tag-meta-release` are unchanged.

### Phase 3 — Adjust cleanup for referential integrity

`cleanup.yml` runs `cleanup-prereleases` with `retention-days=1`. With
the manifest design, an RC tag named in an unpromoted meta-rc manifest
must not be deleted before that meta-rc has had a chance to release.

Default approach: bump retention to **7 days** (covers a long weekend +
queued release run). Cheap, no new logic.

Stretch: teach `cleanup-prereleases` (in the `optivem/actions` repo) to
read outstanding meta-rc manifests and skip-list any flavor RCs they
reference. Better but out of this plan's scope unless the simple bump
turns out to be insufficient.

### Phase 4 — Unblock the current stuck state

Independent of the workflow rewrite, `main` is stuck. To get the next
release through under either the old or new design:

- Manually bump root `VERSION` from `1.0.87` to `1.0.88` and commit to
  `main`. Next prerelease will mint `meta-v1.0.88-rc.X` and
  `*-v1.0.88-rc.X`, qa/signoff statuses (already present) will line up,
  and meta-release will pass.

This is a one-shot manual fix. Once the manifest design is in place, this
recovery step is no longer needed for future drift.

## Open questions

These need to be decided before drafting the workflow edits.

1. **What does the meta version number mean post-decoupling?**
   - **(a) Keep semver-ish auto-patch** on every release — familiar handle,
     low semantic value.
   - **(b) Use semver intentionally** — major/minor only when the *meta
     layer itself* changes (workflows, orchestration, flavor set);
     patches for routine releases.
   - **(c) Drop X.Y.Z entirely** — `meta-rc.314` / `meta-release-314` as the
     only identifier. Most honest about the new semantics; biggest UX shift.

   Default assumption if undecided: **(a)** — preserves current behavior of
   `bump-patch-version-meta`, no extra change.

2. **Does meta-release still require all 6 flavors?**
   - Today: hardcoded list of 6 in `meta-release-stage.yml` and
     `_meta-prerelease-pipeline.yml`.
   - With the manifest, a meta release naturally accepts whatever set the
     prerelease decided to bundle. Could allow shipping with 5 if one
     flavor is broken; could allow adding a 7th flavor later.
   - Or keep "always 6" as a fixed contract — manifest just always lists 6.

   Default assumption if undecided: **always 6** (matches today, smaller
   blast radius).

3. **Cleanup retention strategy.**
   - **(a) Bump `retention-days` to 7** in `cleanup.yml`. One-line change.
   - **(b) Manifest-aware cleanup** — read outstanding meta-rc manifests
     in `cleanup-prereleases` (cross-repo change in `optivem/actions`),
     never delete a referenced flavor RC.
   - **(c) Both** — (a) now as a safety margin, (b) later as the
     principled fix.

   Default assumption if undecided: **(a)** — covers the realistic gap
   (prereleases ~2/day, releases auto-fire on tag push within minutes).

4. **What about `meta-prerelease-dry-run.yml` and `meta-bump-all.yml`?**
   - `meta-prerelease-dry-run.yml`: should it also write a manifest, or skip
     (since it doesn't tag)? Likely skip — it's a dry-run, no tag, no
     downstream consumer.
   - `meta-bump-all.yml`: this bumps all flavors simultaneously. Post-decouple,
     does this workflow still make sense, or is it a relic of the lockstep
     model? Could keep as a convenience for "everyone bump together at the
     start of a sprint" but it's no longer required for correctness.

   Default assumption if undecided: leave both alone in this plan; revisit
   in a follow-up.

5. **Defensive re-check at release time: hard fail or warn?**
   - If a flavor RC's `qa/signoff` was revoked between prerelease and
     release (e.g. someone manually reset the commit-status), should
     meta-release fail the whole release, or proceed with a warning?
   - Hard fail is safer (a revoke is presumably intentional). But it
     re-introduces a recoverable-only-by-human stuck state if the revoke
     was a mistake.

   Default assumption if undecided: **hard fail** — safer; revoke is rare
   and intentional; manifest path then needs a human-driven re-prerelease
   to recover, which is the right escalation.

6. **Migration: do we need to handle in-flight meta-rcs without manifests?**
   - When the new `meta-release-stage.yml` lands, will there be any
     `meta-v*-rc.*` tags from the old prerelease in the queue without
     a manifest body?
   - Either: deploy both workflow changes (prerelease + release) in a
     single PR, gate on the absence of legacy meta-rcs, or have the new
     release-stage fall back to the old version-lookup if no manifest is
     present.

   Default assumption if undecided: **single PR, both edits together,
   delete any unpromoted manifest-less meta-rc tags first**. Simplest.

## Reuse references

- **Existing `tag-meta-rc` step** in `_meta-prerelease-pipeline.yml` —
  the place where the manifest write hooks in. Already has access to
  per-flavor outputs via `needs.<job>.outputs`.
- **Existing `gh_retry` helper** at
  `.github/workflows/scripts/gh-retry.sh` — used by the current
  `Find flavor RCs to promote` step for the qa/signoff API calls;
  same helper used for the defensive re-check.
- **`optivem/actions/trigger-and-wait-for-workflow@v1`** — unchanged;
  `promote-*` jobs still call this with `{"version": "<manifested-rc>"}`.

## Out of scope

- **Changes to per-flavor pipelines or commit/qa stages.** Those already
  produce flavor RC tags and qa/signoff statuses correctly.
- **Cross-repo changes to `optivem/actions`** (e.g. manifest-aware
  cleanup). Phase 3's stretch goal; defer unless the simple retention
  bump proves insufficient.
- **Changing what "all flavors" means.** Open question 2 — out of scope
  for this plan unless explicitly answered "no, allow partial".
- **Removing root `VERSION`.** Open question 1 option (c) — out of scope
  for this plan unless explicitly chosen.
- **Auditing other workflows for similar version-matching assumptions.**
  Followup if this pattern appears elsewhere.

## Verification plan

1. **Local replay (dry run).** Pick a recent meta-rc tag, build a manifest
   from its likely inputs, run the new lookup logic against it, confirm
   it resolves the same RCs the old logic would have *if it weren't
   stuck*.
2. **Phase 4 first.** Manually bump root `VERSION` to `1.0.88` and observe
   one successful end-to-end release under the *current* code. This
   proves the system can succeed and gives a baseline.
3. **Then deploy the manifest changes.** First successful manifest-driven
   release confirms read/write parity.
4. **Then induce drift on purpose.** Dispatch a single `<flavor>-prod-stage`
   directly so it bumps that flavor independently. Confirm next
   meta-prerelease + meta-release still complete (proves the design fixes
   the original failure mode).

## Critical files (in `../shop`)

- `.github/workflows/_meta-prerelease-pipeline.yml` — Phase 1 edit.
- `.github/workflows/meta-release-stage.yml` — Phase 2 edit.
- `.github/workflows/cleanup.yml` — Phase 3 edit.
- `VERSION` — Phase 4 manual bump (one-shot).
