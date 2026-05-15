package shell

import "regexp"

// Retry policy for SonarCloud API calls. Mirrors the bash sonar_retry wrapper
// in optivem/actions/shared/sonar-retry.sh: retries 5xx and network/TLS/DNS
// transients, passes through 4xx (incl. 401/403/404) immediately so callers
// see auth/not-found errors as themselves rather than a retried failure.
//
// Defined in its own file so both internal/shell/sonarcloud.go and
// internal/sonar/sonar.go (which imports this package) share one regex pair —
// adding a new transient pattern is a one-line edit.
var (
	sonarRetryTransient = regexp.MustCompile(
		`(?i)HTTP 5\d\d|Bad Gateway|Service Unavailable|Gateway Timeout|` +
			`i/o timeout|timeout|connection reset|connection refused|` +
			`TLS handshake|temporary failure in name resolution|no such host|` +
			`EOF|broken pipe`)

	sonarRetryHardFail = regexp.MustCompile(`(?i)HTTP 4\d\d`)
)

// SonarRetryTransient is the transient-pattern regex used by SonarCloud retry
// wrappers. Exported so callers in sibling packages (internal/sonar) can plug
// it into RetryWithPolicy without redefining the pattern.
func SonarRetryTransient() *regexp.Regexp { return sonarRetryTransient }

// SonarRetryHardFail is the hard-fail-pattern regex used by SonarCloud retry
// wrappers. Exported alongside SonarRetryTransient.
func SonarRetryHardFail() *regexp.Regexp { return sonarRetryHardFail }
