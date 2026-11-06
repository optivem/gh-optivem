#!/usr/bin/env bash
set -euo pipefail

# postCreateCommand. The container works in its own clone of gh-optivem in the gh-optivem-workspace
# volume, synced with the host only via GitHub. On first create, clone it; then run the repo's
# post-create steps. Re-run by hand (`gh-optivem-bootstrap`) if the clone failed.

REPO_DIR=/workspaces/gh-optivem

# Named volumes are created root-owned; gh must be able to write its login state before the clone
sudo chown -R "$(id -u):$(id -g)" "$HOME/.config/gh"

REPO_URL=https://github.com/optivem/gh-optivem.git
FRESH_CLONE=false

if [[ ! -d "$REPO_DIR/.git" ]]; then
  # The repo is public, so an anonymous clone works before `gh auth login`; git push/pull later go
  # through VS Code's forwarded credentials (or gh, once logged in)
  echo "Cloning $REPO_URL into $REPO_DIR..."
  if ! git clone "$REPO_URL" "$REPO_DIR"; then
    echo "::error::Could not clone $REPO_URL into $REPO_DIR (check the network). Retry with 'gh-optivem-bootstrap'." >&2
    exit 1
  fi
  FRESH_CLONE=true
fi

cd "$REPO_DIR"
bash .devcontainer/post-create.sh

if [[ "$FRESH_CLONE" == true ]]; then
  echo ""
  echo "Clone done. If the Explorer is still empty, run 'Developer: Reload Window'."
fi
