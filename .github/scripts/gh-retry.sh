#!/usr/bin/env bash
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

_GH_RETRY_ATTEMPTS=4
_GH_RETRY_DELAYS=(5 15 45)

# shellcheck disable=SC2034  # referenced via grep -E
_GH_RETRY_RETRYABLE='HTTP 5[0-9][0-9]|timeout|timed out|i/o timeout|connection reset|connection refused|\bEOF\b|was closed|TLS handshake|tls:.*handshake|temporary failure in name resolution|no such host|Bad Gateway|Service Unavailable|Gateway Timeout|server error'
# shellcheck disable=SC2034
_GH_RETRY_HARD_FAIL='HTTP 4[0-9][0-9]|HTTP 403.*rate limit'

gh_retry() {
    if [[ "${GH_RETRY_DISABLE:-0}" == "1" ]]; then
        gh "$@"
        return $?
    fi

    local attempt=1
    local code=0
    local stdout_file stderr_file
    stdout_file=$(mktemp -t gh-retry-out.XXXXXX)
    stderr_file=$(mktemp -t gh-retry-err.XXXXXX)

    while (( attempt <= _GH_RETRY_ATTEMPTS )); do
        : >"$stdout_file"
        : >"$stderr_file"
        gh "$@" >"$stdout_file" 2>"$stderr_file"
        code=$?

        if (( code == 0 )); then
            cat "$stdout_file"
            [[ -s "$stderr_file" ]] && cat "$stderr_file" >&2
            rm -f "$stdout_file" "$stderr_file"
            return 0
        fi

        local stderr_content
        stderr_content=$(cat "$stderr_file")

        # Hard-fail pass-through (4xx, rate-limit). Never retry — would only burn quota.
        if grep -Eqi "$_GH_RETRY_HARD_FAIL" <<<"$stderr_content"; then
            cat "$stdout_file"
            cat "$stderr_file" >&2
            rm -f "$stdout_file" "$stderr_file"
            return "$code"
        fi

        # Not a known transient pattern → pass through (e.g. 404 existence probe).
        if ! grep -Eqi "$_GH_RETRY_RETRYABLE" <<<"$stderr_content"; then
            cat "$stdout_file"
            cat "$stderr_file" >&2
            rm -f "$stdout_file" "$stderr_file"
            return "$code"
        fi

        local snippet
        snippet=$(head -n1 "$stderr_file" | tr -d '\r')

        if (( attempt < _GH_RETRY_ATTEMPTS )); then
            local delay_idx=$(( attempt - 1 ))
            if (( delay_idx >= ${#_GH_RETRY_DELAYS[@]} )); then
                delay_idx=$(( ${#_GH_RETRY_DELAYS[@]} - 1 ))
            fi
            local sleep_s=${_GH_RETRY_DELAYS[$delay_idx]}
            echo "::notice::[gh-retry] attempt $attempt failed (exit $code): $snippet -- retrying in ${sleep_s}s" >&2
            sleep "$sleep_s"
        else
            echo "::warning::[gh-retry] exhausted $_GH_RETRY_ATTEMPTS attempts (exit $code): $snippet" >&2
            cat "$stdout_file"
            cat "$stderr_file" >&2
            rm -f "$stdout_file" "$stderr_file"
            return "$code"
        fi

        (( attempt++ ))
    done

    rm -f "$stdout_file" "$stderr_file"
    return "$code"
}
