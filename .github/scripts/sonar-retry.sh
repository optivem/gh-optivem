#!/usr/bin/env bash
# GENERATED — DO NOT EDIT.
# Source: optivem/actions/shared/sonar-retry.sh @ feb2abf5a0b8614a33c0538e8887567ee99470da
# Sync via: bash optivem/actions/scripts/sync-shared.sh
# sonar-retry.sh — retry wrapper for sonarscanner invocations that hit
# sonarcloud.io (or a self-hosted SonarQube). Wraps any of:
#
#   - `sonar-scanner ...` (CLI on PATH)
#   - `dotnet sonarscanner begin/end ...`
#   - `mvn sonar:sonar ...`
#   - `./gradlew sonar ...`
#
# Source this file from a `run:` step, then wrap the scanner call:
#
#   source "$GITHUB_WORKSPACE/.github/workflows/scripts/sonar-retry.sh"
#   sonar_retry dotnet sonarscanner end /d:sonar.token="$SONAR_TOKEN"
#   sonar_retry ./mvnw sonar:sonar -Dsonar.token="$SONAR_TOKEN"
#   sonar_retry ./gradlew sonar
#
# Transient triggers covered:
#   - SonarCloud-specific: `Error 5XX on https://...`, `Endpoint request timed out`
#     (the wording observed in acceptance run 25865827466 that drove this engine).
#   - Generic network: HTTP 5xx, DNS/TLS/connection-reset/EOF/handshake.
#
# Hard-fail pass-through (never retried):
#   - 401 / 403 / Unauthorized / Forbidden — auth/token problems.
#   - "Project ... not found" / "Project key ... does not exist" — config errors.
#
# Skip the wrapper for purely local sonar setup (`sonar-scanner --version`,
# `dotnet sonarscanner --help`).
#
# Set `SONAR_RETRY_DISABLE=1` to bypass the retry loop.

# shellcheck source=./retry-core.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/retry-core.sh"

_SONAR_RETRY_ATTEMPTS=4
_SONAR_RETRY_DELAYS=(5 15 45)

# shellcheck disable=SC2034  # referenced via grep -E
_SONAR_RETRY_RETRYABLE='Error 5[0-9][0-9] on https://|Endpoint request timed out|HTTP 5[0-9][0-9]|Internal Server Error|Bad Gateway|Service Unavailable|Gateway Timeout|server error|timeout|timed out|i/o timeout|connection reset|connection refused|\bEOF\b|unexpected EOF|was closed|TLS handshake|tls:.*handshake|temporary failure in name resolution|no such host'
# shellcheck disable=SC2034
_SONAR_RETRY_HARD_FAIL='HTTP 401|HTTP 403|Unauthorized|Forbidden|Project key .* does not exist|Project .* not found|Not authorized'

sonar_retry() {
    if [[ "${SONAR_RETRY_DISABLE:-0}" == "1" ]]; then
        "$@"
        return $?
    fi
    _RETRY_CORE_ATTEMPTS="$_SONAR_RETRY_ATTEMPTS"
    _RETRY_CORE_DELAYS=("${_SONAR_RETRY_DELAYS[@]}")
    retry_with_policy "$_SONAR_RETRY_RETRYABLE" "$_SONAR_RETRY_HARD_FAIL" sonar-retry -- "$@"
}
