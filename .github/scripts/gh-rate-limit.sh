#!/usr/bin/env bash
# gh-rate-limit.sh — rate-limit pre-check for `gh` API loops.
#
# Source this file alongside gh-retry.sh when a step makes many `gh` calls
# in quick succession (loops, pagination, fan-outs). Call `wait_for_rate_limit`
# before each iteration. If the remaining core-API quota is below
# RATE_LIMIT_THRESHOLD (default 50), the function sleeps until the reset time
# so the next call lands in a fresh window rather than hitting 403.
#
#   source "$GITHUB_ACTION_PATH/../shared/gh-retry.sh"
#   source "$GITHUB_ACTION_PATH/../shared/gh-rate-limit.sh"
#   for repo in "${repos[@]}"; do
#     wait_for_rate_limit
#     gh_retry api "repos/$repo/releases"
#   done
#
# Why this is separate from gh-retry.sh:
#   - gh-retry.sh retries transient 5xx/network errors but hard-fails on
#     rate-limit 403 (retrying would burn quota faster).
#   - This helper is the caller-side complement that prevents hitting the 403
#     in the first place.
#
# Why `gh api rate_limit` bypasses gh_retry: retrying a rate-limit probe on
# transient failure is fine, but routing it through the retry wrapper creates
# a circular dependency (wait_for_rate_limit is itself called from retry-ful
# code). The probe is cheap and idempotent — raw gh is correct here.

RATE_LIMIT_THRESHOLD="${RATE_LIMIT_THRESHOLD:-50}"

wait_for_rate_limit() {
    local remaining probe_err probe_stderr
    probe_err=$(mktemp -t gh-rate-limit-err.XXXXXX)
    # If the probe fails (no token, offline, etc.) don't block — let the real
    # call surface the underlying error. We still surface stderr as a notice so
    # the failure is visible in the workflow log instead of silently discarded.
    if ! remaining=$(gh api rate_limit --jq '.resources.core.remaining' 2>"$probe_err"); then
        probe_stderr=$(tr '\n' ' ' <"$probe_err")
        rm -f "$probe_err"
        [[ -n "$probe_stderr" ]] && echo "::notice::[rate-limit] probe failed: ${probe_stderr}— continuing without pre-check" >&2
        return 0
    fi
    rm -f "$probe_err"
    [[ -z "$remaining" ]] && return 0

    if (( remaining < RATE_LIMIT_THRESHOLD )); then
        local reset_ts now_ts wait_secs reset_err reset_stderr
        reset_err=$(mktemp -t gh-rate-limit-err.XXXXXX)
        if ! reset_ts=$(gh api rate_limit --jq '.resources.core.reset' 2>"$reset_err"); then
            reset_stderr=$(tr '\n' ' ' <"$reset_err")
            rm -f "$reset_err"
            [[ -n "$reset_stderr" ]] && echo "::notice::[rate-limit] reset probe failed: ${reset_stderr}— skipping wait" >&2
            return 0
        fi
        rm -f "$reset_err"
        now_ts=$(date +%s)
        wait_secs=$(( reset_ts - now_ts + 5 ))

        if (( wait_secs > 0 )); then
            echo "::notice::[rate-limit] ${remaining} requests remaining (threshold ${RATE_LIMIT_THRESHOLD}); sleeping ${wait_secs}s for reset" >&2
            sleep "$wait_secs"
        fi
    fi
}
