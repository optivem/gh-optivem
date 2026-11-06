# Devcontainer: empty workspace on first open

## TL;DR

**Why:** on a first **Reopen in Container**, the Explorer shows nothing. [bootstrap.sh](../.devcontainer/bootstrap.sh) clones `optivem/gh-optivem` into the `gh-optivem-workspace` volume only when `gh` is logged in. A brand-new container never is (the `gh-optivem-gh-config` volume starts empty), so the clone is skipped. The only notice is an `::error::` line in the postCreate log, which scrolls away. Confirmed on 2026-11-06: both volumes were empty after a first open. The user then wondered if they should have used "Clone Repository in Container Volume" instead (they shouldn't have).
**End result:** a user who opens the container before logging in sees, on every attach, a clear message with the exact recovery steps. The README covers the symptom. After a successful bootstrap the script says to reload the window so the files appear.

## Workaround until this ships

Use **Reopen in Container**, not "Clone Repository in Container Volume". If the Explorer is empty, run this in the container terminal:

```bash
gh auth login
gh-optivem-bootstrap
```

Then run **Developer: Reload Window** from the Command Palette. This is needed only once, because the `gh` login is saved in the `gh-optivem-gh-config` volume and survives rebuilds.

## Items

1. **Attach-time banner.** Add `.devcontainer/check-workspace.sh`, baked into the image like `bootstrap.sh` (the repo's `.devcontainer/` isn't in the volume when it's empty, so the script can't live only in the repo). When `/workspaces/gh-optivem/.git` is missing, print a short boxed message:
   - the workspace is empty because `gh` isn't logged in;
   - run `gh auth login`, then `gh-optivem-bootstrap`, then **Developer: Reload Window**;
   - don't use "Clone Repository in Container Volume".

   Chain it into `postAttachCommand` in [devcontainer.json](../.devcontainer/devcontainer.json) *before* the existing `fix-git-credential-helpers.sh` guard. The script must exit 0 so attach never fails.
2. **Reload hint after a fresh clone.** In [bootstrap.sh](../.devcontainer/bootstrap.sh), when the clone branch ran, print `Clone done — run "Developer: Reload Window" to see the files.` after `post-create.sh`. VS Code opened the folder while it was still empty.
3. **README.** In [.devcontainer/README.md](../.devcontainer/README.md):
   - add a **Troubleshooting** entry: "Explorer is empty after first open" → cause (not logged in, so the clone was skipped) + the three recovery steps;
   - add one line under Setup step 2 saying **Reopen in Container** is the intended entry point, not **Clone Repository in Container Volume** (that creates its own volume and bypasses `workspaceMount` and the bootstrap flow).

## Open questions

- **Auto-login from a host token?** We could forward a host `GH_TOKEN` via `remoteEnv` so the first clone works without `gh auth login`. **Recommended: no.** It brings host token handling into the container and doesn't fit the documented interactive login, while the banner already makes recovery a 30-second step. Revisit if students keep hitting this.

## Verification

- Remove the `gh-optivem-workspace` and `gh-optivem-gh-config` volumes, **Rebuild Container**, and confirm the banner shows on attach. Then run the recovery steps and confirm the reload hint appears and the files load.
