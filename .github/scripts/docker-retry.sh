#!/usr/bin/env bash
# GENERATED — DO NOT EDIT.
# Source: optivem/actions/shared/docker-retry.sh @ 46778d4347841d807d10db984c89c36b9ceaff60
# Sync via: bash optivem/actions/scripts/sync-shared.sh
# docker-retry.sh — retry wrapper for `docker` CLI invocations that hit a
# container registry (push, pull, inspect, buildx imagetools, etc.).
#
# Source this file from any action.yml composite step that shells out to
# `docker`, then replace `docker ...` with `docker_retry ...`:
#
#   source "$GITHUB_ACTION_PATH/../shared/docker-retry.sh"
#   docker_retry buildx imagetools create --tag "$new" "$old"
#   json=$(docker_retry inspect "$image")
#
# The wrapper buffers each attempt's stdout and stderr. On success, stdout is
# written to the function's stdout (preserving `$(...)` capture semantics) and
# stderr is forwarded to the caller's stderr. On transient failure (HTTP 5xx
# from the registry, network/DNS/TLS blips, daemon `context deadline
# exceeded`, connection resets, EOF), the call is retried up to 4 times with
# 5s → 15s → 45s backoff between attempts. On hard failure (unauthorized,
# denied, manifest/name unknown), the wrapper returns the attempt's output
# and preserves the original non-zero exit code so callers that use exit code
# for flow control keep working unchanged.
#
# Skip the wrapper for purely local docker invocations that don't touch a
# registry (`docker version`, `docker context ls`).
#
# Set `DOCKER_RETRY_DISABLE=1` to bypass the retry loop.

# shellcheck source=./retry-core.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/retry-core.sh"

_DOCKER_RETRY_ATTEMPTS=4
_DOCKER_RETRY_DELAYS=(5 15 45)

# shellcheck disable=SC2034  # referenced via grep -E
_DOCKER_RETRY_RETRYABLE='context deadline exceeded|Client\.Timeout|i/o timeout|timed out|connection reset|connection refused|\bEOF\b|unexpected EOF|was closed|TLS handshake|tls:.*handshake|temporary failure in name resolution|no such host|HTTP 5[0-9][0-9]|Internal Server Error|Bad Gateway|Service Unavailable|Gateway Timeout|server error|received unexpected HTTP status: 5[0-9][0-9]|received unexpected HTTP status 5[0-9][0-9]|net/http: TLS handshake timeout|http2: server sent GOAWAY'
# shellcheck disable=SC2034
_DOCKER_RETRY_HARD_FAIL='unauthorized|denied: permission|denied: requested access|manifest unknown|name unknown|repository name not known|requested access to the resource is denied|insufficient_scope|HTTP 4[0-9][0-9]'

docker_retry() {
    if [[ "${DOCKER_RETRY_DISABLE:-0}" == "1" ]]; then
        docker "$@"
        return $?
    fi
    _RETRY_CORE_ATTEMPTS="$_DOCKER_RETRY_ATTEMPTS"
    _RETRY_CORE_DELAYS=("${_DOCKER_RETRY_DELAYS[@]}")
    retry_with_policy "$_DOCKER_RETRY_RETRYABLE" "$_DOCKER_RETRY_HARD_FAIL" docker-retry -- docker "$@"
}
