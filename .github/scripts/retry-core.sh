#!/usr/bin/env bash
# GENERATED — DO NOT EDIT.
# Source: optivem/actions/shared/retry-core.sh @ 22b7b192eda46c9c6aef6add0929cfa2824a1f37
# Sync via: bash optivem/actions/scripts/sync-shared.sh
# retry-core.sh — generic retry engine shared by tool-specific wrappers
# (gh-retry.sh, docker-retry.sh, sonar-retry.sh).
#
# Each wrapper declares its own transient + hard-fail regex and a log prefix,
# then delegates to `retry_with_policy`. Centralising the loop here means a
# new transient pattern is a one-line edit in one place — not five.
#
#   retry_with_policy <transient_re> <hard_fail_re> <prefix> -- <cmd> [args...]
#
# Behaviour:
#   - On exit 0: stdout → caller's stdout, stderr → caller's stderr, return 0.
#   - On non-zero with stderr matching <hard_fail_re>: pass through immediately
#     (preserves exit code for callers using rc as a probe — e.g. 4xx, auth,
#     "not found").
#   - On non-zero with stderr matching <transient_re>: sleep per
#     `_RETRY_CORE_DELAYS`, retry up to `_RETRY_CORE_ATTEMPTS` times. After
#     exhaustion, pass through the last attempt's output and emit
#     `::warning::[<prefix>] exhausted N attempts ...`.
#   - On non-zero with stderr matching neither: pass through (unknown failure
#     mode — don't retry blindly).
#
# Wrappers can override `_RETRY_CORE_ATTEMPTS` / `_RETRY_CORE_DELAYS` per call
# from their own knobs (`_GH_RETRY_DELAYS`, etc.) so existing test harnesses
# that tweak those knobs keep working unchanged.

_RETRY_CORE_ATTEMPTS=4
_RETRY_CORE_DELAYS=(5 15 45)

retry_with_policy() {
    local transient_re="$1"; shift
    local hard_fail_re="$1"; shift
    local prefix="$1"; shift
    [[ "${1:-}" == "--" ]] && shift

    local attempts="$_RETRY_CORE_ATTEMPTS"
    local delays=("${_RETRY_CORE_DELAYS[@]}")
    local attempt=1
    local code=0
    local stdout_file stderr_file
    stdout_file=$(mktemp -t "${prefix}-out.XXXXXX")
    stderr_file=$(mktemp -t "${prefix}-err.XXXXXX")

    while (( attempt <= attempts )); do
        : >"$stdout_file"
        : >"$stderr_file"
        "$@" >"$stdout_file" 2>"$stderr_file"
        code=$?

        if (( code == 0 )); then
            cat "$stdout_file"
            [[ -s "$stderr_file" ]] && cat "$stderr_file" >&2
            rm -f "$stdout_file" "$stderr_file"
            return 0
        fi

        local stderr_content
        stderr_content=$(cat "$stderr_file")

        # Hard-fail pass-through: 4xx, auth, "not found". Never retry — burns quota.
        if [[ -n "$hard_fail_re" ]] && grep -Eqi "$hard_fail_re" <<<"$stderr_content"; then
            cat "$stdout_file"
            cat "$stderr_file" >&2
            rm -f "$stdout_file" "$stderr_file"
            return "$code"
        fi

        # Not a known transient → pass through (preserves rc for probes).
        if ! grep -Eqi "$transient_re" <<<"$stderr_content"; then
            cat "$stdout_file"
            cat "$stderr_file" >&2
            rm -f "$stdout_file" "$stderr_file"
            return "$code"
        fi

        local snippet
        snippet=$(head -n1 "$stderr_file" | tr -d '\r')

        if (( attempt < attempts )); then
            local delay_idx=$(( attempt - 1 ))
            if (( delay_idx >= ${#delays[@]} )); then
                delay_idx=$(( ${#delays[@]} - 1 ))
            fi
            local sleep_s=${delays[$delay_idx]}
            echo "::notice::[$prefix] attempt $attempt failed (exit $code): $snippet -- retrying in ${sleep_s}s" >&2
            sleep "$sleep_s"
        else
            echo "::warning::[$prefix] exhausted $attempts attempts (exit $code): $snippet" >&2
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
