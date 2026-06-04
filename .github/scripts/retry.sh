#!/usr/bin/env bash
# GENERATED — DO NOT EDIT.
# Source: optivem/actions/shared/retry.sh @ 40dd2a5afd78a1a95808148250623ec96d6db41d
# Sync via: bash optivem/actions/scripts/sync-shared.sh
# retry.sh — unified retry wrapper for any shell command that hits an external
# service (gh CLI, docker registry, sonarscanner, git push/fetch, etc.).
#
# Used by the `optivem/actions/retry@v1` composite. Replaces the four
# tool-specific wrappers (gh-retry.sh, docker-retry.sh, sonar-retry.sh,
# git-retry.sh) — the transient + hard-fail regexes below are the union of
# all four, deduplicated. Concepts match across tools; only the specific
# phrasings differ, and the union is strictly broader without false-positive
# collisions (e.g. sonar output never contains `manifest unknown`).
#
# Usage:
#
#   source "$GITHUB_ACTION_PATH/../shared/retry.sh"
#   retry_run gh api repos/$owner/$repo/releases
#   retry_run docker pull node:22-alpine
#   retry_run bash ./run-sonar.sh
#   retry_run git push origin "$TAG"
#
# Behaviour: 4 attempts with 5s → 15s → 45s backoff. On HTTP 5xx, network
# blips, TLS/DNS errors, or known transient phrases across gh/docker/sonar/
# git tools — retry. On HTTP 4xx, auth errors, "not found" responses, or
# known hard-fail patterns — pass through immediately preserving exit code
# so callers using rc as a probe keep working. A short force-retry override
# (`_RETRY_FORCE_RETRY`) reclaims specific hard-fail-shaped phrasings that are
# in fact transient (e.g. SonarCloud JRE-provisioning 403) — it wins over the
# hard-fail list.
#
# Set `RETRY_DISABLE=1` to bypass the retry loop entirely.

# shellcheck source=./retry-core.sh
source "$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/retry-core.sh"

_RETRY_ATTEMPTS=4
_RETRY_DELAYS=(5 15 45)

# Union of transient patterns from gh-retry, docker-retry, sonar-retry,
# git-retry. Deduplicated; broader phrasings absorb narrower ones (e.g.
# `HTTP 5[0-9][0-9]` covers `HTTP 502|503|504` from git-retry).
#
# `Request failed with status code 5[0-9][0-9]` and `Bootstrapper: An error
# occurred` cover the SonarScanner JS bootstrapper, which reports HTTP failures
# as axios errors ("Request failed with status code NNN") rather than the
# `HTTP NNN` phrasing the other tools use. Its JRE-provisioning call to
# SonarCloud intermittently 403s under concurrent load; that surfaces as a
# bootstrapper error with no "Unauthorized"/"Forbidden" word, so it lands in
# transient here while a genuine auth failure (which does print those words)
# still hits the hard-fail list below.
# shellcheck disable=SC2034  # referenced via grep -E
_RETRY_RETRYABLE='HTTP 5[0-9][0-9]|Error 5[0-9][0-9] on https://|received unexpected HTTP status:? 5[0-9][0-9]|RPC failed.*HTTP 5[0-9][0-9]|Request failed with status code 5[0-9][0-9]|Bootstrapper: An error occurred|Internal Server Error|Bad Gateway|Service Unavailable|Gateway Timeout|server error|Something went wrong while executing your query|Endpoint request timed out|context deadline exceeded|Client\.Timeout|Operation timed out|timeout|timed out|i/o timeout|net/http: TLS handshake timeout|connection reset|Connection reset by peer|connection refused|\bEOF\b|unexpected EOF|was closed|http2: server sent GOAWAY|TLS handshake|tls:.*handshake|server certificate verification failed|temporary failure in name resolution|no such host|Could not resolve host|unable to access|Error response from daemon: Get "[^"]+": unknown'

# Union of hard-fail patterns. `HTTP 4[0-9][0-9]` absorbs explicit 401/403
# from sonar/git. Tool-specific phrasings retained because some appear
# without an HTTP code (docker `manifest unknown`, sonar `Project key ... does
# not exist`, git `pre-receive hook declined`).
# shellcheck disable=SC2034
_RETRY_HARD_FAIL='HTTP 4[0-9][0-9]|HTTP 403.*rate limit|[Uu]nauthorized|Forbidden|Not authorized|Permission denied|denied: permission|denied: requested access|requested access to the resource is denied|insufficient_scope|manifest unknown|name unknown|repository name not known|Project key .* does not exist|Project .* not found|repository .* not found|! \[remote rejected\]|pre-receive hook declined|fatal: protocol|fatal: bad refspec'

# Force-retry override: phrasings that match the hard-fail list above but are
# in fact known-transient infra calls. Checked BEFORE hard-fail (see
# retry-core.sh) so it wins.
#
# `Failed to query JRE metadata` / `/analysis/jres`: SonarCloud's JRE-
# provisioning endpoint (`api.sonarcloud.io/analysis/jres`) intermittently 403s
# under concurrent load. The Gradle Sonar plugin reports this as the literal
# `failed with HTTP 403 Forbidden`, which collides with `HTTP 4[0-9][0-9]` +
# `Forbidden` in the hard-fail list — so without this override it fails fast
# with zero retries. (The JS bootstrapper phrases the *same* failure as
# `Bootstrapper: An error occurred ... status code 403`, which has no `HTTP 4xx`
# token and no `Forbidden` word, so it already lands in transient.) This is a
# pre-analysis provisioning call, not where analysis token/project authz is
# enforced; a genuinely bad token still hard-fails on the analysis submission
# (`Not authorized` / `insufficient_scope`), and even if it also 403s here the
# only cost is exhausting the retries (~65s) before the same non-zero exit.
# shellcheck disable=SC2034
_RETRY_FORCE_RETRY='Failed to query JRE metadata|/analysis/jres'

retry_run() {
    if [[ "${RETRY_DISABLE:-0}" == "1" ]]; then
        "$@"
        return $?
    fi
    _RETRY_CORE_ATTEMPTS="$_RETRY_ATTEMPTS"
    _RETRY_CORE_DELAYS=("${_RETRY_DELAYS[@]}")
    _RETRY_CORE_FORCE_RETRY="$_RETRY_FORCE_RETRY"
    retry_with_policy "$_RETRY_RETRYABLE" "$_RETRY_HARD_FAIL" retry -- "$@"
}
