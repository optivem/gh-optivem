FROM mcr.microsoft.com/devcontainers/base:ubuntu-24.04

# The workspace is a named volume mounted at /workspaces (no bind mount of the host checkout).
# Docker seeds an empty named volume from the image on first mount, so pre-creating the workspace
# folder here means it exists, owned by vscode, before any lifecycle command runs in it.
RUN mkdir -p /workspaces/gh-optivem && chown -R 1000:1000 /workspaces

# Baked into the image because the repo (and its .devcontainer/) isn't in the volume until this clones it
COPY bootstrap.sh /usr/local/bin/gh-optivem-bootstrap
RUN chmod 755 /usr/local/bin/gh-optivem-bootstrap
