# 2026-06-26 07:03:00 UTC — Fix SC2086 actionlint failure in gh-cleanup-orphans.yml

## TL;DR

**Why:** The Commit Stage's new `actionlint` lint step (pinned v1.7.12, added in commit `68977bd6`) runs shellcheck over every workflow `run:` block, and it flags `gh-cleanup-orphans.yml:70` with SC2086 — `bash scripts/cleanup-orphans.sh $args` passes an unquoted string variable for deliberate word-splitting. This breaks Commit Stage (run [28222456736](https://github.com/optivem/gh-optivem/actions/runs/28222456736/job/83606576798)).
**End result:** `gh-cleanup-orphans.yml` builds its CLI arguments as a bash array and passes them quoted, so shellcheck/actionlint pass clean and the Commit Stage goes green.

## Outcomes

What we get out of this — the goals and deliverables:

- `actionlint` (with shellcheck) passes on `.github/workflows/gh-cleanup-orphans.yml` with no SC2086.
- Commit Stage's "Lint workflows" step exits 0 again.
- The `cleanup-orphans.sh` invocation passes its flags safely (array, properly quoted) rather than relying on unquoted string word-splitting.

## ▶ Next executable step (resume here)

Edit `.github/workflows/gh-cleanup-orphans.yml` lines 66–70 (the `Run cleanup` step's `run:` block): replace the string-built `args` with a bash array.

```bash
args=(--owner "$TEST_OWNER" --all --delete)
if [[ -n "$BEFORE_DATE" ]]; then
  args+=(--before "$BEFORE_DATE")
fi
bash scripts/cleanup-orphans.sh "${args[@]}"
```

Then verify: `actionlint .github/workflows/gh-cleanup-orphans.yml` must exit 0 (requires shellcheck installed locally; otherwise the actionlint shellcheck integration silently skips and the bug won't reproduce).

## Steps

- [ ] Step 1: In `.github/workflows/gh-cleanup-orphans.yml`, replace the string-built `args` (lines 66–70) with the bash-array form above (`args=(...)`, `args+=(...)`, `"${args[@]}"`).
- [ ] Step 2: Verify — run `actionlint .github/workflows/gh-cleanup-orphans.yml` with shellcheck installed; must exit 0 with no SC2086. (If shellcheck is unavailable locally, re-run the Commit Stage on the pushed branch and confirm "Lint workflows" passes.)

## Notes

- Single file, single language (YAML workflow). No twin elsewhere — `grep` for `$args` across `.github/workflows/` returns only this file.
- Does **not** reproduce on a box without shellcheck (actionlint skips the shellcheck integration). GitHub runners ship shellcheck, hence the CI-only failure.
