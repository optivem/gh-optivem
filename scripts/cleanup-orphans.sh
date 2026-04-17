#!/usr/bin/env bash
set -euo pipefail

# Deletes orphaned test repos, their SonarCloud projects, Docker
# containers/images, and local clone directories left behind by system tests.
#
# All cleanup targets are opt-in. Pass --all to enable everything,
# or pick individual targets with --repos, --sonar, --docker, --tmp.
#
# Usage:
#   bash scripts/cleanup-orphans.sh --owner valentinajemuovic --all               # dry run all
#   bash scripts/cleanup-orphans.sh --owner valentinajemuovic --all --delete       # real delete all
#   bash scripts/cleanup-orphans.sh --owner valentinajemuovic --repos --sonar      # dry run repos + sonar
#   bash scripts/cleanup-orphans.sh --owner valentinajemuovic --repos --delete     # real delete repos only
#   bash scripts/cleanup-orphans.sh --owner valentinajemuovic --all --prefixes "my-app-"

usage() {
  echo "Usage: $0 --owner <github-user-or-org> [options]"
  echo ""
  echo "Required:"
  echo "  --owner <name>        GitHub user or org (or set TEST_OWNER env var)"
  echo ""
  echo "Targets (all opt-in):"
  echo "  --all                 Enable all cleanup targets"
  echo "  --sonar               Clean up SonarCloud projects"
  echo "  --repos               Clean up GitHub repos"
  echo "  --docker              Clean up Docker containers and images"
  echo "  --tmp                 Clean up local orphan directories"
  echo ""
  echo "Options:"
  echo "  --delete              Actually delete (default: dry run)"
  echo "  --before <date>       Only include repos created before this date (exclusive, ISO 8601 e.g. 2026-04-01)"
  echo "  --prefixes <list>     Space-separated prefixes (default: test-app- page-turner- course-tester- ct-)"
  echo "  --tmp-dir <path>      Local orphan dir (default: <academy>/.tmp)"
  echo "  --sonar-token <tok>   SonarCloud token (or set SONAR_TOKEN env var)"
  echo "  -h, --help            Show this help"
  exit 1
}

# --- Parse arguments ---

TEST_OWNER="${TEST_OWNER:-}"
DRY_RUN=1
CLEAN_SONAR=0
CLEAN_REPOS=0
CLEAN_DOCKER=0
CLEAN_TMP=0
TMP_DIR=""
DEFAULT_PREFIXES="test-app- page-turner- course-tester- ct-"
PREFIXES=""
BEFORE_DATE=""
SONAR_TOKEN="${SONAR_TOKEN:-}"
SONAR_API_URL="${SONAR_API_URL:-https://sonarcloud.io/api}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --owner)      TEST_OWNER="$2"; shift 2 ;;
    --all)        CLEAN_SONAR=1; CLEAN_REPOS=1; CLEAN_DOCKER=1; CLEAN_TMP=1; shift ;;
    --sonar)      CLEAN_SONAR=1; shift ;;
    --repos)      CLEAN_REPOS=1; shift ;;
    --docker)     CLEAN_DOCKER=1; shift ;;
    --tmp)        CLEAN_TMP=1; shift ;;
    --delete)     DRY_RUN=0; shift ;;
    --before)     BEFORE_DATE="$2"; shift 2 ;;
    --prefixes)   PREFIXES="$2"; shift 2 ;;
    --tmp-dir)    TMP_DIR="$2"; shift 2 ;;
    --sonar-token) SONAR_TOKEN="$2"; shift 2 ;;
    -h|--help)    usage ;;
    *)            echo "Unknown option: $1" >&2; usage ;;
  esac
done

PREFIXES="${PREFIXES:-$DEFAULT_PREFIXES}"

if [[ -z "$TEST_OWNER" ]]; then
  echo "ERROR: --owner is required" >&2
  echo ""
  usage
fi

if [[ "$CLEAN_SONAR" == "0" ]] && [[ "$CLEAN_REPOS" == "0" ]] && [[ "$CLEAN_DOCKER" == "0" ]] && [[ "$CLEAN_TMP" == "0" ]]; then
  echo "ERROR: No cleanup targets selected. Use --all or pick targets (--repos, --sonar, --docker, --tmp)." >&2
  echo ""
  usage
fi

# --- Header ---

echo "Owner: $TEST_OWNER"
echo "Prefixes: $PREFIXES"
if [[ -n "$BEFORE_DATE" ]]; then
  echo "Before: $BEFORE_DATE (exclusive)"
fi
targets=""
[[ "$CLEAN_SONAR" == "1" ]] && targets="${targets} sonar"
[[ "$CLEAN_REPOS" == "1" ]] && targets="${targets} repos"
[[ "$CLEAN_DOCKER" == "1" ]] && targets="${targets} docker"
[[ "$CLEAN_TMP" == "1" ]] && targets="${targets} tmp"
echo "Targets:$targets"
if [[ "$DRY_RUN" == "1" ]]; then
  echo "Mode: DRY RUN (pass --delete to actually delete)"
else
  echo "Mode: REAL DELETE"
fi
echo ""

# Build a jq filter that matches any of the prefixes
jq_filter=""
for prefix in $PREFIXES; do
  if [[ -n "$jq_filter" ]]; then
    jq_filter="$jq_filter or "
  fi
  jq_filter="${jq_filter}startswith(\"${prefix}\")"
done

# --- SonarCloud projects ---
# Cleaned up first so the GitHub repo (their cross-reference) still exists
# if this step fails partway.

if [[ "$CLEAN_SONAR" != "1" ]]; then
  :
elif [[ -z "$SONAR_TOKEN" ]]; then
  echo "SONAR_TOKEN not set — skipping SonarCloud cleanup."
elif ! command -v jq >/dev/null 2>&1; then
  echo "jq not installed — skipping SonarCloud cleanup."
else
  sonar_projects=""
  for prefix in $PREFIXES; do
    sonar_prefix="${TEST_OWNER}_${prefix}"
    echo "Searching SonarCloud for projects matching prefix: $sonar_prefix"

    found=$(curl -s \
      -H "Authorization: Bearer ${SONAR_TOKEN}" \
      "${SONAR_API_URL}/projects/search?organization=${TEST_OWNER}&ps=100" \
      | jq -r ".components[] | select(.key | startswith(\"${sonar_prefix}\")) | .key") || true

    if [[ -n "$found" ]]; then
      if [[ -n "$sonar_projects" ]]; then
        sonar_projects="$sonar_projects"$'\n'"$found"
      else
        sonar_projects="$found"
      fi
    fi
  done

  if [[ -z "$sonar_projects" ]]; then
    echo "No orphaned SonarCloud projects found."
  else
    sonar_count=$(echo "$sonar_projects" | wc -l | tr -d ' ')
    echo "Found $sonar_count orphaned SonarCloud project(s):"
    echo ""

    for project_key in $sonar_projects; do
      if [[ "$DRY_RUN" == "1" ]]; then
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
    if [[ "$DRY_RUN" == "1" ]]; then
      echo "Dry run complete (SonarCloud). Run with --delete to delete."
    else
      echo "Deleted $sonar_count SonarCloud project(s)."
    fi
  fi
fi

echo ""

# --- GitHub repos ---

if [[ "$CLEAN_REPOS" == "1" ]]; then
  before_jq=""
  if [[ -n "$BEFORE_DATE" ]]; then
    before_jq=" and .createdAt < \"${BEFORE_DATE}\""
  fi
  repos=$(gh repo list "$TEST_OWNER" --limit 1000 --json name,createdAt --jq "[.[] | select((.name | ${jq_filter})${before_jq})] | sort_by(.createdAt) | .[].name") || true

  if [[ -z "$repos" ]]; then
    echo "No orphaned test repos found."
  else
    count=$(echo "$repos" | wc -l | tr -d ' ')
    echo "Found $count orphaned test repo(s):"
    echo ""

    for repo in $repos; do
      full="$TEST_OWNER/$repo"
      if [[ "$DRY_RUN" == "1" ]]; then
        echo "  [dry run] would delete $full"
      else
        echo "  Deleting $full ..."
        gh repo delete "$full" --yes
        sleep 10
      fi
    done

    echo ""
    if [[ "$DRY_RUN" == "1" ]]; then
      echo "Dry run complete (repos). Run with --delete to delete."
    else
      echo "Deleted $count repo(s)."
    fi
  fi
fi

echo ""

# --- Docker containers and images ---

if [[ "$CLEAN_DOCKER" != "1" ]]; then
  :
elif ! command -v docker >/dev/null 2>&1; then
  echo "docker not installed — skipping Docker cleanup."
else
  # Build a grep pattern that matches any of the prefixes
  grep_pattern=""
  for prefix in $PREFIXES; do
    if [[ -n "$grep_pattern" ]]; then
      grep_pattern="$grep_pattern|"
    fi
    grep_pattern="${grep_pattern}${prefix}"
  done

  # --- Containers (stopped and running) ---
  orphan_containers=$(docker ps -a --format '{{.Names}}' | grep -E "^(${grep_pattern})" || true)

  if [[ -z "$orphan_containers" ]]; then
    echo "No orphaned Docker containers found."
  else
    container_count=$(echo "$orphan_containers" | wc -l | tr -d ' ')
    echo "Found $container_count orphaned Docker container(s):"
    echo ""

    for container in $orphan_containers; do
      if [[ "$DRY_RUN" == "1" ]]; then
        echo "  [dry run] would remove container: $container"
      else
        echo "  Removing container: $container ..."
        docker rm -f "$container"
      fi
    done

    echo ""
    if [[ "$DRY_RUN" == "1" ]]; then
      echo "Dry run complete (containers). Run with --delete to delete."
    else
      echo "Removed $container_count container(s)."
    fi
  fi

  echo ""

  # --- Images ---
  orphan_images=$(docker images --format '{{.Repository}}:{{.Tag}}' | grep -E "(${grep_pattern})" || true)

  if [[ -z "$orphan_images" ]]; then
    echo "No orphaned Docker images found."
  else
    image_count=$(echo "$orphan_images" | wc -l | tr -d ' ')
    echo "Found $image_count orphaned Docker image(s):"
    echo ""

    for image in $orphan_images; do
      if [[ "$DRY_RUN" == "1" ]]; then
        echo "  [dry run] would remove image: $image"
      else
        echo "  Removing image: $image ..."
        docker rmi -f "$image"
      fi
    done

    echo ""
    if [[ "$DRY_RUN" == "1" ]]; then
      echo "Dry run complete (images). Run with --delete to delete."
    else
      echo "Removed $image_count image(s)."
    fi
  fi
fi

echo ""

# --- Local orphan directories ---

if [[ "$CLEAN_TMP" == "1" ]]; then
  if [[ -z "$TMP_DIR" ]]; then
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    TMP_DIR="$(cd "$script_dir/../.." && pwd)/.tmp"
  fi

  echo "Scanning local orphan dir: $TMP_DIR"

  if [[ ! -d "$TMP_DIR" ]]; then
    echo "No local orphan directory found — skipping."
  else
    safe_dirs=()
    unsafe_dirs=()

    for dir in "$TMP_DIR"/*/; do
      [[ -d "$dir" ]] || continue
      dir="${dir%/}"
      name="$(basename "$dir")"

      if [[ ! -d "$dir/.git" ]]; then
        safe_dirs+=("$dir")
        continue
      fi

      dirty="$(git -C "$dir" status --porcelain 2>/dev/null || echo "ERR")"
      if [[ "$dirty" == "ERR" ]]; then
        unsafe_dirs+=("$name (git status failed)")
        continue
      fi
      if [[ -n "$dirty" ]]; then
        unsafe_dirs+=("$name (uncommitted changes)")
        continue
      fi

      unpushed="$(git -C "$dir" log --branches --not --remotes --oneline 2>/dev/null || echo "ERR")"
      if [[ "$unpushed" == "ERR" ]]; then
        unsafe_dirs+=("$name (unpushed check failed)")
        continue
      fi
      if [[ -n "$unpushed" ]]; then
        unsafe_dirs+=("$name (unpushed commits)")
        continue
      fi

      safe_dirs+=("$dir")
    done

    if [[ ${#unsafe_dirs[@]} -gt 0 ]]; then
      echo ""
      echo "Skipping ${#unsafe_dirs[@]} dir(s) with local work:"
      for entry in "${unsafe_dirs[@]}"; do
        echo "  ! $entry"
      done
    fi

    if [[ ${#safe_dirs[@]} -eq 0 ]]; then
      echo ""
      echo "No safe-to-delete local orphan directories found."
    else
      echo ""
      echo "Found ${#safe_dirs[@]} safe-to-delete local orphan dir(s):"
      echo ""

      for dir in "${safe_dirs[@]}"; do
        if [[ "$DRY_RUN" == "1" ]]; then
          echo "  [dry run] would delete $dir"
        else
          echo "  Deleting $dir ..."
          rm -rf "$dir"
        fi
      done

      echo ""
      if [[ "$DRY_RUN" == "1" ]]; then
        echo "Dry run complete (local dirs). Run with --delete to delete."
      else
        echo "Deleted ${#safe_dirs[@]} local orphan dir(s)."
      fi
    fi
  fi
fi
