package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// TestOptions narrows or modifies a tests run.
type TestOptions struct {
	// Suite, when non-empty, limits the run to the suite with this id.
	Suite string
	// Test, when non-empty, narrows execution to one test name. Injected
	// into the suite's Command via TestsConfig.TestFilter.
	Test string
	// Sample, when true, uses each suite's sampleTest field as the test
	// name (if both Sample is set and Test is non-empty, Test wins).
	Sample bool
	// Health overrides default HTTP-probe parameters.
	Health HealthOptions
}

// SuiteResult records the outcome of one suite — used to print the summary
// table at the end of a run, even when one suite failed mid-way.
type SuiteResult struct {
	Name     string
	Status   string // "PASSED" | "FAILED"
	Duration time.Duration
}

// RunTests runs setupCommands once, then iterates suites in tests:
//
//  1. If sys is non-nil, ensures every system in sys is currently up via the
//     IsAnyURLUp probe. If a system isn't up, returns an error — the runner
//     does not start it implicitly. (Use `gh optivem run system` first.)
//  2. Runs each setupCommand in cwd. A failure aborts before any suite runs.
//  3. Filters suites per opts.Suite. Errors out with the available ids if
//     opts.Suite doesn't match any suite.
//  4. Runs each remaining suite. After the last suite (or first failure),
//     prints a summary table.
//
// Returns the first suite failure or nil. The summary table is printed
// regardless, so the user always sees per-suite status.
func RunTests(sys *SystemConfig, tests *TestsConfig, cwd string, opts TestOptions) error {
	if sys != nil {
		for _, s := range sys.Systems {
			if !IsAnyURLUp(s, opts.Health) {
				return fmt.Errorf("system %s is not running — start it first with `gh optivem run system`", s.Label)
			}
		}
	}

	for _, sc := range tests.SetupCommands {
		fmt.Fprintf(os.Stdout, "\n--- Setup: %s ---\n", sc.Name)
		if err := runShell(sc.Command, cwd, sc.Env); err != nil {
			return fmt.Errorf("setup %q: %w", sc.Name, err)
		}
	}

	suites, err := selectSuites(tests, opts.Suite)
	if err != nil {
		return err
	}

	results := make([]SuiteResult, 0, len(suites))
	defer func() {
		printSummary(results)
	}()

	for _, suite := range suites {
		start := time.Now()
		err := runOneSuite(suite, tests.TestFilter, cwd, opts)
		dur := time.Since(start)
		status := "PASSED"
		if err != nil {
			status = "FAILED"
		}
		results = append(results, SuiteResult{
			Name:     suite.Name,
			Status:   status,
			Duration: dur,
		})
		if err != nil {
			return fmt.Errorf("suite %s: %w", suite.Name, err)
		}
	}
	return nil
}

func selectSuites(tests *TestsConfig, suiteID string) ([]Suite, error) {
	if suiteID == "" {
		return tests.Suites, nil
	}
	suite := tests.FindSuite(suiteID)
	if suite == nil {
		return nil, fmt.Errorf("suite %q not found. Available: %s",
			suiteID, strings.Join(tests.SuiteIDs(), ", "))
	}
	return []Suite{*suite}, nil
}

func runOneSuite(suite Suite, testFilter, cwd string, opts TestOptions) error {
	suiteDir := cwd
	if suite.Path != "" && suite.Path != "." {
		suiteDir = filepath.Join(cwd, suite.Path)
	}

	for _, ic := range suite.TestInstallCommands {
		fmt.Fprintf(os.Stdout, "Installing test dependencies: %s\n", ic)
		if err := runShell(ic, suiteDir, nil); err != nil {
			return fmt.Errorf("install %q: %w", ic, err)
		}
	}

	cmd := suite.Command
	if filterValue := pickFilterValue(suite, opts); filterValue != "" && testFilter != "" {
		expr := strings.ReplaceAll(testFilter, "<test>", filterValue)
		cmd = appendTestFilter(cmd, expr)
	}

	fmt.Fprintf(os.Stdout, "\n--- Running %s ---\n", suite.Name)
	if err := runShell(cmd, suiteDir, suite.Env); err != nil {
		if suite.TestReportPath != "" {
			report := filepath.Join(suiteDir, suite.TestReportPath)
			if _, statErr := os.Stat(report); statErr == nil {
				fmt.Fprintf(os.Stdout, "Test report: %s\n", report)
			}
		}
		return err
	}
	if suite.TestReportPath != "" {
		fmt.Fprintf(os.Stdout, "Test report: %s\n", filepath.Join(suiteDir, suite.TestReportPath))
	}
	return nil
}

func pickFilterValue(suite Suite, opts TestOptions) string {
	if opts.Test != "" {
		return opts.Test
	}
	if opts.Sample {
		return suite.SampleTest
	}
	return ""
}

// filterInjectionRE matches "--filter '<existing-fragment>" so we can inject
// an "&Category=..." style fragment inside the existing single-quoted value
// rather than appending a new flag.
var filterInjectionRE = regexp.MustCompile(`(--filter\s+'[^']*)`)

// appendTestFilter is the runner-side equivalent of the PS1 Append-TestFilter
// helper. Two cases:
//
//   - filterExpression starts with "&" — it's an expression fragment (dotnet
//     style); inject inside the existing --filter '...' arg.
//   - otherwise — it's a full flag (typescript "--grep '...'"); append as a
//     new argument.
func appendTestFilter(command, filterExpression string) string {
	if strings.HasPrefix(filterExpression, "&") {
		return filterInjectionRE.ReplaceAllString(command, "${1}"+filterExpression)
	}
	return command + " " + filterExpression
}

// runShell executes a command string with optional env overlay. Output is
// streamed to the user's terminal. The command is parsed via the same quote-
// aware splitter used elsewhere in this codebase to avoid `sh -c` and the
// platform-specific shell quoting it brings.
func runShell(command, cwd string, env map[string]string) error {
	parts, err := splitCommand(command)
	if err != nil {
		return fmt.Errorf("invalid command %q: %w", command, err)
	}
	if len(parts) == 0 {
		return errors.New("empty command")
	}
	cmd := exec.Command(parts[0], parts[1:]...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(env) > 0 {
		cmd.Env = mergeEnv(os.Environ(), env)
	}
	return cmd.Run()
}

// mergeEnv returns base with any matching keys in overlay overwritten, and
// any new keys appended.
func mergeEnv(base []string, overlay map[string]string) []string {
	out := make([]string, 0, len(base)+len(overlay))
	overridden := make(map[string]bool, len(overlay))
	for k := range overlay {
		overridden[k] = true
	}
	for _, kv := range base {
		eq := strings.IndexByte(kv, '=')
		if eq > 0 && overridden[kv[:eq]] {
			continue
		}
		out = append(out, kv)
	}
	for k, v := range overlay {
		out = append(out, k+"="+v)
	}
	return out
}

func printSummary(results []SuiteResult) {
	if len(results) == 0 {
		return
	}
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Suite Results:")
	fmt.Fprintln(os.Stdout, strings.Repeat("-", 70))
	for _, r := range results {
		fmt.Fprintf(os.Stdout, "  %-45s %-8s %s\n", r.Name, r.Status, formatDur(r.Duration))
	}
	fmt.Fprintln(os.Stdout, strings.Repeat("-", 70))
}

func formatDur(d time.Duration) string {
	d = d.Round(time.Millisecond)
	mins := int(d / time.Minute)
	secs := int(d/time.Second) % 60
	ms := int(d/time.Millisecond) % 1000
	return fmt.Sprintf("%02d:%02d.%03d", mins, secs, ms)
}

// splitCommand splits a command string on whitespace, respecting single and
// double quotes. The shell package has the same helper for its purposes;
// duplicated here so the runner package has no shell dependency.
func splitCommand(s string) ([]string, error) {
	var parts []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)

	flush := func() {
		if current.Len() > 0 {
			parts = append(parts, current.String())
			current.Reset()
		}
	}

	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote && c == quoteChar:
			inQuote = false
		case inQuote:
			current.WriteByte(c)
		case c == '"' || c == '\'':
			inQuote = true
			quoteChar = c
		case c == ' ' || c == '\t':
			flush()
		default:
			current.WriteByte(c)
		}
	}
	if inQuote {
		return nil, fmt.Errorf("unterminated %c quote", quoteChar)
	}
	flush()
	return parts, nil
}
