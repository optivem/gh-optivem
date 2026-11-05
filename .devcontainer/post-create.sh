#!/usr/bin/env bash
set -euo pipefail

# Runs from the repo root, via gh-optivem-bootstrap (see bootstrap.sh), which has already clone-checked
# the workspace and given ~/.config/gh to vscode.
# ~/.claude/commands is a read-only bind mount from the host; skip it
sudo find "$HOME/.claude" -path "$HOME/.claude/commands" -prune -o -exec chown "$(id -u):$(id -g)" {} +
# The claude-code feature installs as root, so auto-update needs these writable
sudo chown -R "$(id -u):$(id -g)" "$(npm root -g)/@anthropic-ai"
CLAUDE_BIN="$(npm config get prefix)/bin/claude"
if [[ -L "$CLAUDE_BIN" ]]; then sudo chown -h "$(id -u):$(id -g)" "$CLAUDE_BIN"; fi

go mod download

echo "=== Verifying devcontainer tools ==="
for t in "gh --version" "go version" "docker --version" "node --version" "claude --version"; do
  printf "  %-10s %s\n" "${t%% *}" "$($t 2>&1 | head -1)"
done

echo ""
echo "Next: gh auth login, then 'go build ./...' and 'go test ./...'"
