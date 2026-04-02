#!/usr/bin/env bash
set -euo pipefail

# Deletes orphaned test repos (test-app-*) left behind by system tests.
#
# Required: TEST_OWNER=<github-user-or-org>
# Optional: DRY_RUN=1 (default) — list repos without deleting
#           DRY_RUN=0 — actually delete repos
#
# Usage:
#   TEST_OWNER=valentinajemuovic bash scripts/cleanup-orphans.sh          # dry run
#   TEST_OWNER=valentinajemuovic DRY_RUN=0 bash scripts/cleanup-orphans.sh  # real delete

if [ -z "${TEST_OWNER:-}" ]; then
  echo "ERROR: TEST_OWNER environment variable is required" >&2
  exit 1
fi

DRY_RUN="${DRY_RUN:-1}"

echo "Owner: $TEST_OWNER"
if [ "$DRY_RUN" = "1" ]; then
  echo "Mode: DRY RUN (set DRY_RUN=0 to actually delete)"
else
  echo "Mode: REAL DELETE"
fi
echo ""

# List all repos matching the test-app-* pattern
repos=$(gh repo list "$TEST_OWNER" --limit 1000 --json name --jq '.[].name | select(startswith("test-app-"))') || true

if [ -z "$repos" ]; then
  echo "No orphaned test repos found."
  exit 0
fi

count=$(echo "$repos" | wc -l | tr -d ' ')
echo "Found $count orphaned test repo(s):"
echo ""

for repo in $repos; do
  full="$TEST_OWNER/$repo"
  if [ "$DRY_RUN" = "1" ]; then
    echo "  [dry run] would delete $full"
  else
    echo "  Deleting $full ..."
    gh repo delete "$full" --yes
  fi
done

echo ""
if [ "$DRY_RUN" = "1" ]; then
  echo "Dry run complete. Run with DRY_RUN=0 to delete these repos."
else
  echo "Deleted $count repo(s)."
fi
