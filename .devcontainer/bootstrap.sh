#!/usr/bin/env bash
set -euo pipefail

# postCreateCommand. The container works in its own clone of gh-optivem in the gh-optivem-workspace
# volume, synced with the host only via GitHub. On first create, clone it; then run the repo's
# post-create steps. Re-run by hand (`gh-optivem-bootstrap`) after `gh auth login` if the clone failed.

REPO_DIR=/workspaces/gh-optivem

# Named volumes are created root-owned; gh must be able to write its login state before the clone
sudo chown -R "$(id -u):$(id -g)" "$HOME/.config/gh"

if [[ ! -d "$REPO_DIR/.git" ]]; then
  if ! gh auth status >/dev/null 2>&1; then
    echo "::error::$REPO_DIR is empty and gh is not logged in, so optivem/gh-optivem can't be cloned." >&2
    echo "Run 'gh auth login', then 'gh-optivem-bootstrap'." >&2
    exit 1
  fi
  mkdir -p "$REPO_DIR"
  echo "Cloning optivem/gh-optivem into $REPO_DIR..."
  gh repo clone optivem/gh-optivem "$REPO_DIR"
fi

cd "$REPO_DIR"
bash .devcontainer/post-create.sh
