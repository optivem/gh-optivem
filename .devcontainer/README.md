# Dev Container

Reproducible environment for building and testing the `gh-optivem` CLI, with Go, `gh`, Docker, Node 22, and Claude Code pre-installed (bash only). Works identically on Windows, macOS, and Linux.

## Prerequisites

Install once on your host machine:

- [Docker Desktop](https://www.docker.com/products/docker-desktop/) (Windows/macOS) or Docker Engine (Linux) — **must be running**
- [VS Code](https://code.visualstudio.com/) with the [Dev Containers extension](https://marketplace.visualstudio.com/items?itemName=ms-vscode-remote.remote-containers)
- A `~/.claude/commands` folder — it is bind-mounted into the container, and the container will not start without it

## Setup

1. Open the `gh-optivem/` folder **directly** in VS Code (not a parent like `academy/`). The `.devcontainer/` must sit at the root of the opened folder — multi-root workspaces and parent directories will not work.
2. Command Palette (`Ctrl+Shift+P` / `Cmd+Shift+P`) → **Dev Containers: Reopen in Container**.
   - ⚠️ Do **not** pick **Dev Containers: Add Dev Container Configuration Files…** — the repo already ships a config.
   - ⚠️ Do **not** use **Dev Containers: Clone Repository in Container Volume…** — it creates its own volume and bypasses the `gh-optivem-workspace` volume and bootstrap flow below. **Reopen in Container** is the only step needed.
3. First launch builds the image (~3-5 min); subsequent launches are instant.
4. [bootstrap.sh](bootstrap.sh) (baked into the image as `gh-optivem-bootstrap`) runs automatically. On first create it clones `optivem/gh-optivem` into the `gh-optivem-workspace` volume (see [Two clones](#two-clones-host-and-container)), then runs [post-create.sh](post-create.sh), which downloads Go modules and verifies tooling. The repo is public, so the clone is anonymous and needs no login.
5. [fix-git-credential-helpers.sh](fix-git-credential-helpers.sh) runs on every attach and repairs git credential helpers copied in from a Windows host (see Troubleshooting).
6. Authenticate with `gh auth login` (if you haven't already), then `bash scripts/install.sh`, `go build ./...` and `go test ./...`.

**Troubleshooting:** if the Explorer is empty after the first open, the clone failed — check the postCreate log, then run `gh-optivem-bootstrap` in the container terminal and **Developer: Reload Window**.

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

## Two clones: host and container

The container does **not** mount your host checkout. It works in its own Linux-native clone at `/workspaces/gh-optivem`, kept in the `gh-optivem-workspace` named volume (mounted at `/workspaces`). VS Code still reads `.devcontainer/` from the host folder you opened, but the code you edit inside the container is the volume's clone.

- **Sync only through GitHub:** commit and push on one side, `git pull` on the other. Uncommitted work in one clone is invisible to the other.
- **Why:** a shared bind mount can't hold a working `gh-optivem` binary for both OSes (`gh` runs `<repo>/gh-optivem` on every OS), and it brings CRLF line endings, root-owned files, `safe.directory` warnings, and slow 9p I/O.
- **Siblings** (e.g. `shop`): `gh repo clone optivem/shop /workspaces/shop` — no rebuild needed; they live in the same volume.
- **Reset the clone:** push anything you want to keep, then remove the volume (`docker volume rm gh-optivem-workspace`, with the container stopped) and **Dev Containers: Rebuild Container**. The next create clones fresh.

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

Command Palette → **Dev Containers: Rebuild Container**. The `gh-optivem-workspace` volume survives a rebuild, so your clone (including uncommitted work) is kept.
