package version

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// CheckGhCLI verifies the user's `gh` CLI meets MinGhCLIVersion. It returns
// a nil error on success and an actionable error otherwise — gh missing,
// unparseable output, or version below the floor. Designed to be called
// from a Cobra PersistentPreRunE on the gh-dependent subcommands.
func CheckGhCLI() error {
	out, err := exec.Command("gh", "--version").Output()
	if err != nil {
		return fmt.Errorf("could not run `gh --version` — is the GitHub CLI installed? https://cli.github.com/\n  underlying error: %w", err)
	}
	got, err := parseGhVersion(string(out))
	if err != nil {
		return fmt.Errorf("could not parse gh version: %w", err)
	}
	want, err := parseSemver(MinGhCLIVersion)
	if err != nil {
		return fmt.Errorf("internal: bad MinGhCLIVersion %q: %w", MinGhCLIVersion, err)
	}
	if compareSemver(got, want) < 0 {
		return fmt.Errorf(
			"gh CLI %d.%d.%d is older than the supported floor %s — please upgrade (winget upgrade GitHub.cli / brew upgrade gh / etc.)",
			got[0], got[1], got[2], MinGhCLIVersion,
		)
	}
	return nil
}

var ghVersionRE = regexp.MustCompile(`gh version\s+(\d+\.\d+\.\d+)`)

// parseGhVersion extracts the X.Y.Z triple from `gh --version` output.
// Example input: "gh version 2.92.0 (2026-04-28)\nhttps://...\n"
func parseGhVersion(out string) ([3]int, error) {
	m := ghVersionRE.FindStringSubmatch(out)
	if m == nil {
		first, _, _ := strings.Cut(out, "\n")
		return [3]int{}, fmt.Errorf("unexpected `gh --version` output: %q", strings.TrimSpace(first))
	}
	return parseSemver(m[1])
}

func parseSemver(s string) ([3]int, error) {
	parts := strings.SplitN(s, ".", 3)
	if len(parts) != 3 {
		return [3]int{}, fmt.Errorf("not in X.Y.Z form: %q", s)
	}
	var v [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return [3]int{}, fmt.Errorf("bad semver component %q in %q", p, s)
		}
		v[i] = n
	}
	return v, nil
}

func compareSemver(a, b [3]int) int {
	for i := range 3 {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return 1
		}
	}
	return 0
}
