#!/usr/bin/env bash
# GENERATED — DO NOT EDIT.
# Source: optivem/actions/shared/retry-core.sh @ b746f07b824242b2329a725d140ba0869c4d095d
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
#   - On non-zero with output matching `_RETRY_CORE_FORCE_RETRY` (optional
#     override): retry, even if the output would otherwise match <hard_fail_re>.
#     For known-transient infra calls that phrase their failure like a hard-fail
#     (e.g. SonarCloud's JRE-provisioning endpoint, which 403s under load and
#     prints `HTTP 403 Forbidden` — indistinguishable by regex from a genuine
#     auth 403). Checked BEFORE hard-fail so the override wins; default empty
#     (no override).
#   - On non-zero with output matching <hard_fail_re>: pass through immediately
#     (preserves exit code for callers using rc as a probe — e.g. 4xx, auth,
#     "not found").
#   - On non-zero with output matching <transient_re>: sleep per
#     `_RETRY_CORE_DELAYS`, retry up to `_RETRY_CORE_ATTEMPTS` times. After
#     exhaustion, pass through the last attempt's output and emit
#     `::warning::[<prefix>] exhausted N attempts ...`.
#   - On non-zero with output matching neither: pass through (unknown failure
#     mode — don't retry blindly).
#
# Classification matches the union of stdout + stderr: some tools log their
# failure diagnostics to stdout (e.g. the SonarScanner JS bootstrapper writes
# its `[ERROR] Bootstrapper: ...` lines to stdout), so a stderr-only match
# would miss them and never retry a genuinely transient failure.
#
# stdin caveat: the retry loop calls "$@" once per attempt. If the caller
# pipes stdin to `retry_run` (e.g. `printf '%s' "$pw" | retry_run docker
# login --password-stdin`), stdin is consumed on attempt 1 and empty on
# every retry. Wrap stdin-feeding commands in a shell function and retry
# the function instead — see docker-login/login.sh for the pattern.
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
        # `if/then/else/fi` keeps `set -e` (inherited from callers like start.sh)
        # from exiting the script the moment "$@" returns non-zero — without the
        # conditional, errexit fires before `code=$?` runs, so retries never
        # happen and the captured stdout/stderr never reach the log.
        if "$@" >"$stdout_file" 2>"$stderr_file"; then
            code=0
        else
            code=$?
        fi

        if (( code == 0 )); then
            cat "$stdout_file"
            [[ -s "$stderr_file" ]] && cat "$stderr_file" >&2
            rm -f "$stdout_file" "$stderr_file"
            return 0
        fi

        # Classify against both streams — see header note on stdout-logging tools.
        local match_content
        match_content=$(cat "$stdout_file" "$stderr_file")

        # Force-retry override: known-transient infra calls whose output looks
        # like a hard-fail (e.g. SonarCloud JRE provisioning printing `HTTP 403
        # Forbidden`). Checked first so it wins over hard-fail and routes the
        # failure down the retry path below regardless of <transient_re>.
        local force_match=0
        if [[ -n "${_RETRY_CORE_FORCE_RETRY:-}" ]] \
            && grep -Eqi "${_RETRY_CORE_FORCE_RETRY}" <<<"$match_content"; then
            force_match=1
        fi

        # Hard-fail pass-through: 4xx, auth, "not found". Never retry — burns quota.
        if (( ! force_match )) && [[ -n "$hard_fail_re" ]] && grep -Eqi "$hard_fail_re" <<<"$match_content"; then
            cat "$stdout_file"
            cat "$stderr_file" >&2
            rm -f "$stdout_file" "$stderr_file"
            return "$code"
        fi

        # Not a known transient (and not force-retried) → pass through (preserves rc for probes).
        if (( ! force_match )) && ! grep -Eqi "$transient_re" <<<"$match_content"; then
            cat "$stdout_file"
            cat "$stderr_file" >&2
            rm -f "$stdout_file" "$stderr_file"
            return "$code"
        fi

        # Snippet for the retry/exhaustion log: prefer stderr, fall back to
        # stdout for tools that report failures there.
        local snippet
        snippet=$(head -n1 "$stderr_file" | tr -d '\r')
        [[ -z "$snippet" ]] && snippet=$(head -n1 "$stdout_file" | tr -d '\r')

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
