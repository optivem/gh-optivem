#!/usr/bin/env bash
# Re-pull the bundled license texts from GitHub's licenses API.
#
# The texts live in internal/kernel/license/texts/<key>.txt and are the
# offered `license:` set (one file = one valid key). This script overwrites
# each one with the body of `gh api licenses/<key>`.
#
# Usage:
#   bash scripts/refresh-licenses.sh                # refresh every bundled key
#   bash scripts/refresh-licenses.sh mpl-2.0 ...    # refresh / add specific keys
#
# Adding a license: run with the new key, then add its name + SPDX id row to
# licenseMeta in internal/kernel/license/license.go and to the expected list
# in TestKeys_AreTheBundledTexts.
#
# Placeholders: no conversion is needed. The repo's fill convention IS
# GitHub's: `[year]` and `[fullname]`, which the API bodies for mit,
# bsd-2-clause and bsd-3-clause carry in their copyright line. A text that
# contains `[fullname]` gets the copyright holder filled in at scaffold time;
# every other text ships verbatim (license.HasFillableNotice). The GNU
# licenses and Apache use other spellings ("<year>  <name of author>",
# "[yyyy] [name of copyright owner]") inside their "how to apply" appendix,
# which is canonical text and is left alone. TestFillableKeysAreTheExpectedOnes
# fails if a refresh moves a license between the two classes.
#
# Not served by the API, so skipped here and maintained by hand: mit-0, 0bsd.
#
# Fails loudly: any gh api error or empty body aborts the run before the
# file is replaced.

set -euo pipefail

root="$(git rev-parse --show-toplevel)"
dir="$root/internal/kernel/license/texts"
not_in_api=" mit-0 0bsd "

if [[ $# -gt 0 ]]; then
  keys=("$@")
else
  keys=()
  for f in "$dir"/*.txt; do
    keys+=("$(basename "$f" .txt)")
  done
fi

tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT

for key in "${keys[@]}"; do
  if [[ "$not_in_api" == *" $key "* ]]; then
    echo "skip  $key (not served by the GitHub licenses API; maintained by hand)"
    continue
  fi
  # No leading slash: Git Bash on Windows rewrites "/licenses/..." into a
  # filesystem path.
  if ! body="$(gh api "licenses/$key" --jq .body)"; then
    echo "::error::gh api licenses/$key failed. Check the key against 'gh api licenses --jq .[].key' and that gh is authenticated." >&2
    exit 1
  fi
  if [[ -z "${body//[[:space:]]/}" ]]; then
    echo "::error::gh api licenses/$key returned an empty body; refusing to write a blank license text." >&2
    exit 1
  fi
  # $(...) strips trailing newlines; write exactly one, matching the API body.
  printf '%s\n' "$body" > "$tmp"
  mv "$tmp" "$dir/$key.txt"
  echo "wrote $key"
done
