#!/usr/bin/env bash
# GENERATED — DO NOT EDIT.
# Source: optivem/actions/shared/gh-retry.sh @ 8de8f0f85c9adf2e8a2222a9a3c42536fa6ca353
# Sync via: bash optivem/actions/scripts/sync-shared.sh
# gh-retry.sh — retry wrapper for `gh` CLI invocations.
#
# Source this file from any action.yml composite step that calls `gh`, then
# replace `gh ...` with `gh_retry ...`:
#
#   source "$GITHUB_ACTION_PATH/../shared/gh-retry.sh"
#   json=$(gh_retry api "repos/$owner/$repo/releases" --paginate)
#
# The wrapper buffers each attempt's stdout and stderr. On success, stdout is
# written to the function's stdout (preserving `$(...)` capture semantics) and
# stderr is forwarded to the caller's stderr. On transient failure (HTTP 5xx,
# network/DNS/TLS blips, connection resets), the call is retried up to 4 times
# with 5s → 15s → 45s backoff between attempts. On non-transient failure (4xx,
# auth, bad args, 404 existence probes), the wrapper returns the attempt's
# output and preserves the original non-zero exit code — callers that use exit
# code for flow control (e.g. `gh release view` to detect absence) keep
# working unchanged.
#
# Skip the wrapper for purely local probes that don't hit the GitHub API
# (`gh auth status`, `gh api rate_limit`).
#
# Set `GH_RETRY_DISABLE=1` to bypass the retry loop.

# shellcheck source=./retry-core.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/retry-core.sh"

_GH_RETRY_ATTEMPTS=4
_GH_RETRY_DELAYS=(5 15 45)

# shellcheck disable=SC2034  # referenced via grep -E
_GH_RETRY_RETRYABLE='HTTP 5[0-9][0-9]|timeout|timed out|i/o timeout|connection reset|connection refused|\bEOF\b|was closed|TLS handshake|tls:.*handshake|temporary failure in name resolution|no such host|Bad Gateway|Service Unavailable|Gateway Timeout|server error|Something went wrong while executing your query'
# shellcheck disable=SC2034
_GH_RETRY_HARD_FAIL='HTTP 4[0-9][0-9]|HTTP 403.*rate limit'

gh_retry() {
    if [[ "${GH_RETRY_DISABLE:-0}" == "1" ]]; then
        gh "$@"
        return $?
    fi
    _RETRY_CORE_ATTEMPTS="$_GH_RETRY_ATTEMPTS"
    _RETRY_CORE_DELAYS=("${_GH_RETRY_DELAYS[@]}")
    retry_with_policy "$_GH_RETRY_RETRYABLE" "$_GH_RETRY_HARD_FAIL" gh-retry -- gh "$@"
}
