#!/usr/bin/env bash
set -euo pipefail

# Runs a manual scaffold against a randomly-named repo and, on success,
# deletes the created GitHub repos + SonarCloud projects via
# scripts/cleanup-orphans.sh. The local scaffold dir is deleted by
# `gh optivem init` itself (its default).
#
# On failure: nothing is deleted, so the scaffold dir + remote repos
# stay around for debugging.
#
# Usage:
#   bash scripts/manual-test.sh --owner <user> --system-name "Page Turner" \
#       --arch monolith --repo-strategy monorepo --lang java
#
#   bash scripts/manual-test.sh --no-cleanup --owner <user> ...
#
# All flags (except --no-cleanup) are forwarded to `gh optivem init`.
# The script supplies --repo; --no-cleanup is translated to --keep-local
# (which also suppresses the post-run remote cleanup). Do not pass
# --repo or --keep-local yourself — they will conflict.
#
# Orphan cleanup if the script is killed mid-run:
#   bash scripts/cleanup-orphans.sh --owner <user> --repos --sonar \
#       --prefixes "manual-test-" --delete

cd "$(git rev-parse --show-toplevel)"

NO_CLEANUP=0
OWNER=""
PASSTHROUGH=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-cleanup) NO_CLEANUP=1; shift ;;
    --owner)      OWNER="$2"; PASSTHROUGH+=("$1" "$2"); shift 2 ;;
    -h|--help)    sed -n '4,25p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *)            PASSTHROUGH+=("$1"); shift ;;
  esac
done

if [[ -z "$OWNER" ]]; then
  echo "ERROR: --owner is required" >&2
  exit 1
fi

if command -v openssl >/dev/null 2>&1; then
  SUFFIX=$(openssl rand -hex 8)
else
  SUFFIX=$(printf '%04x%04x%04x%04x' "$RANDOM" "$RANDOM" "$RANDOM" "$RANDOM")
fi
REPO="manual-test-${SUFFIX}"

if [[ "$NO_CLEANUP" == "1" ]]; then
  INIT_FLAGS=(--keep-local)
  CLEANUP_DESC="none (--no-cleanup: keep local dir + GitHub repos + Sonar projects)"
else
  INIT_FLAGS=()
  CLEANUP_DESC="full (local dir deleted by init; GitHub repos + Sonar projects deleted after)"
fi

echo "Manual test repo:   $REPO"
echo "Cleanup on success: $CLEANUP_DESC"
echo ""

if ! go run . init --repo "$REPO" "${INIT_FLAGS[@]}" ${PASSTHROUGH[@]+"${PASSTHROUGH[@]}"}; then
  echo ""
  echo "Scaffold failed — leaving local dir + GitHub repos + Sonar projects intact for debugging."
  echo "Clean up later with:"
  echo "  bash scripts/cleanup-orphans.sh --owner $OWNER --repos --sonar --prefixes \"manual-test-\" --delete"
  exit 1
fi

if [[ "$NO_CLEANUP" == "1" ]]; then
  echo ""
  echo "Done. --no-cleanup: local dir, GitHub repos, and Sonar projects kept."
  exit 0
fi

echo ""
echo "Scaffold succeeded. Deleting GitHub repos + Sonar projects for $REPO..."
exec bash scripts/cleanup-orphans.sh \
  --owner "$OWNER" \
  --repos --sonar \
  --prefixes "$REPO" \
  --delete
