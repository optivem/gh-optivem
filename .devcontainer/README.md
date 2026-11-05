# Dev Container

Reproducible environment for building and testing the `gh-optivem` CLI, with Go, `gh`, Docker, Node 22, PowerShell, and Claude Code pre-installed. Works identically on Windows, macOS, and Linux.

## Prerequisites

Install once on your host machine:

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (Windows/macOS) or Docker Engine (Linux) — **must be running**
- [VS Code](https://code.visualstudio.com/) with the [Dev Containers extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers)
- A `~/.claude/commands` folder — it is bind-mounted into the container, and the container will not start without it

## Setup

1. Open the `gh-optivem/` folder **directly** in VS Code (not a parent like `academy/`). The `.devcontainer/` must sit at the root of the opened folder — multi-root workspaces and parent directories will not work.
2. Command Palette (`Ctrl+Shift+P` / `Cmd+Shift+P`) → **Dev Containers: Reopen in Container**.
   - ⚠️ Do **not** pick **Dev Containers: Add Dev Container Configuration Files…** — the repo already ships a config.
3. First launch builds the image (~3-5 min); subsequent launches are instant.
4. [post-create.sh](post-create.sh) runs automatically, downloads Go modules, and verifies tooling.
5. [fix-git-credential-helpers.sh](fix-git-credential-helpers.sh) runs on every attach and repairs git credential helpers copied in from a Windows host (see Troubleshooting).
6. Authenticate with `gh auth login`, then `go build ./...` and `go test ./...`.

**Troubleshooting:** if you see an **"Add configuration to workspace / user data folder"** prompt or a template picker, VS Code did **not** detect the existing config. Cancel, close the folder, and reopen `gh-optivem/` directly — do not pick a template.

**Troubleshooting (Windows):** if the container fails to start with `accessing specified distro mount service: stat /run/guest-services/distro-services/ubuntu.sock: no such file or directory`, enable your distro in Docker Desktop → **Settings → Resources → WSL Integration** and click **Apply & Restart**. If it's already enabled, run `wsl --shutdown`, restart Docker Desktop, and try again.

**Troubleshooting (Windows):** if pushes and pulls print `'C:\Program Files\GitHub CLI\gh.exe' auth git-credential get: ... not found`, `gh auth setup-git` on Windows wrote credential helpers with the Windows path and Dev Containers copied them in. [fix-git-credential-helpers.sh](fix-git-credential-helpers.sh) points them at the container's `gh` on every attach, so this should clear up on its own. To fix it at the source, in PowerShell:

```powershell
foreach ($u in 'https://github.com', 'https://gist.github.com') {
  git config --global --unset-all "credential.$u.helper"
  git config --global --add "credential.$u.helper" ""
  git config --global --add "credential.$u.helper" "!gh auth git-credential"
}
```

## Pinned tool versions

Versions are set in [devcontainer.json](devcontainer.json); every dev gets the same env.

| Tool   | Version |
|--------|---------|
| go     | 1.25 (matches `go.mod`) |
| gh     | latest  |
| node   | 22      |
| claude | latest  |

## Claude Code

Login state lives in the `gh-optivem-claude-config` volume, so it survives rebuilds. `CLAUDE_CONFIG_DIR` points at `~/.claude` so that `.claude.json` is stored in the volume too. Your host's personal slash commands (`~/.claude/commands`) are bind-mounted read-only on top of it; edit them on the host. The devcontainer feature installs Claude Code as root, so `post-create.sh` gives the `@anthropic-ai` scope folder to `vscode`; without that, auto-updates fail with `no_permissions`.

## Rebuilding after config changes

Command Palette → **Dev Containers: Rebuild Container**.
