#!/usr/bin/env bash
# GENERATED — DO NOT EDIT.
# Source: optivem/actions/shared/retry.sh @ e1915a91e16c51374149cd8a4cd1b39f0677aec3
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
#
# jq-parsing caveat: retry-core.sh writes a live `::notice::[<prefix>] attempt
# N failed ... retrying in Ns` line to stderr on every retried-but-eventually-
# successful attempt. If a caller captures `retry_run`'s output with `2>&1`
# into a variable that then gets parsed as structured data (jq, etc.), that
# notice text lands ahead of the real payload and corrupts the parse whenever
# a retry happened before success. Capture stdout and stderr into separate
# variables/files instead when the result will be parsed as structured data.

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
#
# `scanner.sonarcloud.io/jres`: the standalone `sonar-scanner-cli` does the
# same JRE provisioning against a *different* endpoint and phrasing —
# `HttpException: GET https://scanner.sonarcloud.io/jres/OpenJDK...tar.gz failed
# with HTTP 403 Forbidden`. The path is `/jres/`, not `/analysis/jres`, so the
# clauses above miss it and it lands in the hard-fail list (`HTTP 4xx` +
# `Forbidden`) and fails fast. Same provisioning-call reasoning applies, so
# force-retry it too.
#
# `binaries.sonarsource.com`: SonarSource's binary CDN that serves the scanner
# distribution zip (`sonar-scanner-cli-X.Y.Z-linux-x64.zip`). It intermittently
# 403s on transient CDN/edge hiccups — the original trigger of this whole retry
# plan (acceptance run `26935724762`, where the *same commit* passed the same
# download 20 min earlier). It is a pure binary-download CDN, never an auth
# boundary, so *any* 403 naming this host is transient and safe to retry. A
# genuine auth 403 is raised against the analysis-submission endpoint (and prints
# `Not authorized`/`insufficient_scope`), not this host, so it still hard-fails.
#
# `exceeded a secondary rate limit`: GHCR reports a *throttle* using authz
# vocabulary — `error pulling image configuration: ... denied: permission_denied:
# Error from intermediary with HTTP status code 403 "Forbidden"` with the real
# reason only in the JSON body (`You have exceeded a secondary rate limit.
# Please wait a few minutes before you try again.`). That phrasing collides with
# `denied: permission` and `Forbidden` in the hard-fail list, so without this
# override a throttled `docker compose pull` fails fast with zero attempts
# (observed: gh-optivem run 31365614953, deploy step dead in 1799ms). A rate
# limit is a "come back later", not an authz decision, so it belongs in the
# retry path. The clause is keyed on the rate-limit sentence, not on the 403 —
# a genuine GHCR auth denial carries no such wording and still hard-fails.
#
# This override is shared across tools, so it also reclaims GitHub's *secondary*
# rate limit for `gh` calls — deliberately. Secondary limits are short-term
# abuse throttles whose documented remedy is exactly "wait and retry", whereas
# the *primary* limit (`HTTP 403: API rate limit exceeded`, hourly quota
# exhausted) stays hard-fail via `HTTP 403.*rate limit`: retrying that one
# cannot succeed within the backoff window and only burns quota. The two are
# distinguished by wording, so the split holds.
# shellcheck disable=SC2034
_RETRY_FORCE_RETRY='Failed to query JRE metadata|/analysis/jres|scanner\.sonarcloud\.io/jres|binaries\.sonarsource\.com|exceeded a secondary rate limit'

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
