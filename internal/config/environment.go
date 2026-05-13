// Package config — environment display.
//
// ShowEnvironment is the informational counterpart to VerifyTokens: it prints
// each credential env var so the user can confirm what their shell is
// exporting, without leaking the actual secret to terminal scrollback or
// shell history. Token values are masked to first-4/last-4 characters;
// DOCKERHUB_USERNAME is printed in full because it's not a secret.
package config

import (
	"fmt"
	"os"
)

// ShowEnvironment prints each credential env var to stdout. Format:
//
//	NAME=value           (USERNAME — plain)
//	NAME=abcd...wxyz (N chars)  (tokens — masked, N is the full length)
//	NAME=NOT SET         (empty)
//
// Always returns; this is informational and has no failure path.
func ShowEnvironment() {
	e := readEnvTokens()
	entries := []struct {
		name    string
		val     string
		isToken bool
	}{
		{"DOCKERHUB_USERNAME", e.dockerHubUsername, false},
		{"DOCKERHUB_TOKEN", e.dockerHubToken, true},
		{"SONAR_TOKEN", e.sonarToken, true},
		{"GHCR_TOKEN", e.ghcrToken, true},
		{"WORKFLOW_TOKEN", e.workflowToken, true},
		{"REPO_TOKEN", e.repoToken, true},
	}
	for _, x := range entries {
		fmt.Fprintf(os.Stdout, "%s=%s\n", x.name, renderEnvValue(x.val, x.isToken))
	}
}

// renderEnvValue returns the display string for one env var value. Empty
// values become "NOT SET". Non-token values pass through. Token values are
// shown as first-4/last-4 with a length suffix so the user can spot a
// truncated paste; tokens shorter than 12 chars are fully masked because
// first-4/last-4 would leak most of the value.
func renderEnvValue(val string, isToken bool) string {
	if val == "" {
		return "NOT SET"
	}
	if !isToken {
		return val
	}
	if len(val) < 12 {
		return fmt.Sprintf("*** (%d chars — suspiciously short)", len(val))
	}
	return fmt.Sprintf("%s...%s (%d chars)", val[:4], val[len(val)-4:], len(val))
}
