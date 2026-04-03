#!/usr/bin/env bash
set -euo pipefail

# Deletes orphaned test repos and their SonarCloud projects
# left behind by system tests.
#
# Required: TEST_OWNER=<github-user-or-org>
#           SONAR_TOKEN=<sonarcloud-token> (for SonarCloud cleanup)
# Optional: DRY_RUN=1 (default) — list without deleting
#           DRY_RUN=0 — actually delete
#           PREFIXES="test-app- page-turner- course-tester-" (space-separated)
#
# Usage:
#   TEST_OWNER=valentinajemuovic bash scripts/cleanup-orphans.sh          # dry run
#   TEST_OWNER=valentinajemuovic DRY_RUN=0 bash scripts/cleanup-orphans.sh  # real delete
#   TEST_OWNER=valentinajemuovic PREFIXES="my-app-" bash scripts/cleanup-orphans.sh  # custom prefixes

if [ -z "${TEST_OWNER:-}" ]; then
  echo "ERROR: TEST_OWNER environment variable is required" >&2
  exit 1
fi

DRY_RUN="${DRY_RUN:-1}"
SONAR_API_URL="${SONAR_API_URL:-https://sonarcloud.io/api}"

DEFAULT_PREFIXES="test-app- page-turner- course-tester-"
PREFIXES="${PREFIXES:-$DEFAULT_PREFIXES}"

echo "Owner: $TEST_OWNER"
echo "Prefixes: $PREFIXES"
if [ "$DRY_RUN" = "1" ]; then
  echo "Mode: DRY RUN (set DRY_RUN=0 to actually delete)"
else
  echo "Mode: REAL DELETE"
fi
echo ""

# Build a jq filter that matches any of the prefixes
jq_filter=""
for prefix in $PREFIXES; do
  if [ -n "$jq_filter" ]; then
    jq_filter="$jq_filter or "
  fi
  jq_filter="${jq_filter}startswith(\"${prefix}\")"
done

# --- GitHub repos ---

repos=$(gh repo list "$TEST_OWNER" --limit 1000 --json name --jq ".[].name | select(${jq_filter})") || true

if [ -z "$repos" ]; then
  echo "No orphaned test repos found."
else
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
      sleep 2
    fi
  done

  echo ""
  if [ "$DRY_RUN" = "1" ]; then
    echo "Dry run complete (repos). Run with DRY_RUN=0 to delete."
  else
    echo "Deleted $count repo(s)."
  fi
fi

# --- SonarCloud projects ---

echo ""

if [ -z "${SONAR_TOKEN:-}" ]; then
  echo "SONAR_TOKEN not set — skipping SonarCloud cleanup."
  exit 0
fi

sonar_projects=""
for prefix in $PREFIXES; do
  sonar_prefix="${TEST_OWNER}_${prefix}"
  echo "Searching SonarCloud for projects matching prefix: $sonar_prefix"

  found=$(curl -s \
    -H "Authorization: Bearer ${SONAR_TOKEN}" \
    "${SONAR_API_URL}/projects/search?organization=${TEST_OWNER}&ps=100" \
    | jq -r ".components[] | select(.key | startswith(\"${sonar_prefix}\")) | .key") || true

  if [ -n "$found" ]; then
    if [ -n "$sonar_projects" ]; then
      sonar_projects="$sonar_projects"$'\n'"$found"
    else
      sonar_projects="$found"
    fi
  fi
done

if [ -z "$sonar_projects" ]; then
  echo "No orphaned SonarCloud projects found."
  exit 0
fi

sonar_count=$(echo "$sonar_projects" | wc -l | tr -d ' ')
echo "Found $sonar_count orphaned SonarCloud project(s):"
echo ""

for project_key in $sonar_projects; do
  if [ "$DRY_RUN" = "1" ]; then
    echo "  [dry run] would delete SonarCloud project: $project_key"
  else
    echo "  Deleting SonarCloud project: $project_key ..."
    curl -s -X POST \
      -H "Authorization: Bearer ${SONAR_TOKEN}" \
      "${SONAR_API_URL}/projects/delete" \
      -d "project=${project_key}" > /dev/null
    sleep 1
  fi
done

echo ""
if [ "$DRY_RUN" = "1" ]; then
  echo "Dry run complete (SonarCloud). Run with DRY_RUN=0 to delete."
else
  echo "Deleted $sonar_count SonarCloud project(s)."
fi
