#!/usr/bin/env bash
set -euo pipefail

# Runs a manual test scaffold against a randomly-named repo.
#
# On success: deletes the local scaffold dir, the GitHub test repos, and
# the SonarCloud projects. Use --no-cleanup to keep everything for
# inspection. Cleanup is always skipped on failure, regardless of
# --no-cleanup, so you can debug a failed scaffold.
#
# Usage:
#   bash scripts/manual-test.sh --owner <user> --system-name "Page Turner" \
#       --arch monolith --repo-strategy monorepo --lang java
#
#   bash scripts/manual-test.sh --no-cleanup --owner <user> ...
#
# All flags (except --no-cleanup) are forwarded to `gh optivem init`.
# The script supplies --repo, --test, --delete-test-repos (or --keep-local
# when --no-cleanup is passed). Do not pass --repo, --test, --keep-local,
# --delete-test-repos, or --random-suffix yourself — they will conflict.
#
# Orphan cleanup if the script is killed mid-run:
#   bash scripts/cleanup-orphans.sh --owner <user> --all \
#       --prefixes "manual-test-" --delete

cd "$(git rev-parse --show-toplevel)"

NO_CLEANUP=0
PASSTHROUGH=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-cleanup) NO_CLEANUP=1; shift ;;
    -h|--help)    sed -n '4,25p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *)            PASSTHROUGH+=("$1"); shift ;;
  esac
done

if command -v openssl >/dev/null 2>&1; then
  SUFFIX=$(openssl rand -hex 8)
else
  SUFFIX=$(printf '%04x%04x%04x%04x' "$RANDOM" "$RANDOM" "$RANDOM" "$RANDOM")
fi
REPO="manual-test-${SUFFIX}"

if [[ "$NO_CLEANUP" == "1" ]]; then
  CLEANUP_FLAGS=(--keep-local)
  CLEANUP_DESC="none (--no-cleanup: keep local dir + GitHub repos + Sonar projects)"
else
  CLEANUP_FLAGS=(--delete-test-repos)
  CLEANUP_DESC="full (delete local dir + GitHub repos + Sonar projects)"
fi

echo "Manual test repo:   $REPO"
echo "Cleanup on success: $CLEANUP_DESC"
echo ""

exec go run . init --repo "$REPO" --test "${CLEANUP_FLAGS[@]}" ${PASSTHROUGH[@]+"${PASSTHROUGH[@]}"}
