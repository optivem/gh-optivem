package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/kernel/pathx"
)

// TestOptions narrows or modifies a tests run.
type TestOptions struct {
	// Suite, when non-empty, limits the run to the suites with these ids.
	// Order in the run follows tests.json declaration order, not slice
	// order, so two invocations with the same ids run in the same order
	// regardless of how the user typed them.
	Suite []string
	// Test, when non-empty, narrows execution to the given test names.
	// Injected into the suite's Command via TestsConfig.TestFilter and
	// joined per TestsConfig.TestFilterJoin.
	Test []string
	// Sample, when true, uses each suite's sampleTest field as the test
	// name (if both Sample is set and Test is non-empty, Test wins).
	Sample bool
	// Health overrides default HTTP-probe parameters used by the pre-run
	// probe (every entry in systems.yaml must be responding before any suite
	// runs).
	Health HealthOptions
}

// SuiteResult records the outcome of one suite — used to print the summary
// table at the end of a run, even when one suite failed mid-way.
type SuiteResult struct {
	Name     string
	Status   string // "PASSED" | "FAILED"
	Duration time.Duration
}

// RunTests iterates suites in tests:
//
//  1. If sys is non-nil, probes every entry in sys.Systems and errors out
//     if any isn't responding to its health probe (caller must have already
//     started the system via `gh optivem system start`).
//  2. Filters suites per opts.Suite (a set; empty means all). Errors out
//     with the available ids if any requested id doesn't match.
//  3. Runs each remaining suite. After the last suite (or first failure),
//     prints a summary table.
//
// testsCwd is tests.json's dir (setupCommands and suite.path resolve against
// it). systemCwd is unused today but retained in the signature so callers don't
// need a per-call branch on whether sys is supplied.
//
// setupCommands are not run by this verb — invoke `gh optivem test setup` (or
// runner.RunSetup) explicitly before tests. This mirrors mainstream
// service-lifecycle CLIs where each phase is a separate verb.
//
// Returns the first failure or nil. The summary table is printed regardless,
// so the user always sees per-suite status.
func RunTests(sys *SystemConfig, tests *TestsConfig, systemCwd, testsCwd string, opts TestOptions) error {
	_ = systemCwd
	if sys != nil {
		for _, s := range sys.Systems {
			if !IsAnyURLUp(s, opts.Health) {
				return fmt.Errorf("system %s is not running — start it first with `gh optivem system start`", s.Label)
			}
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

	// Track executed-test counts across the suites that opted into the count
	// guard (TestCountPath set). A selection that runs every suite to a clean
	// exit yet executes zero tests is the empty-selection hole: the filter
	// resolved to nothing, so a verify would green (or spin a fixer) without
	// exercising a single test. We fail the run below so it routes to the same
	// infra halt as the non-zero-exit runners.
	anyCounted := false
	totalExecuted := 0

	// Presence check for named verifies. A category like `acceptance` fans
	// out to partitioned sub-suites (isolated vs non-isolated, API vs UI), and
	// a named test lives in only the partition matching its tags/channel — so
	// the other sub-suites legitimately match nothing. We therefore do NOT
	// fail per-suite on an empty slice; instead we union the executed method
	// names across all sub-suites and require every requested --test to have
	// run somewhere. anyNamed gates this so a config with no TestCountPath
	// can't false-fail (back-compat: no report source → no presence enforcement).
	wantNames := len(opts.Test) > 0
	anyNamed := false
	executedNames := map[string]bool{}

	for _, suite := range suites {
		start := time.Now()
		executed, counted, names, err := runOneSuite(suite, tests.TestFilter, tests.TestFilterJoin, testsCwd, opts)
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
		if counted {
			anyCounted = true
			totalExecuted += executed
		}
		if wantNames && suite.TestCountPath != "" {
			anyNamed = true
			for n := range names {
				executedNames[n] = true
			}
		}
	}

	// For a named run with an observable report, the presence check subsumes
	// the zero-count guard: if every requested name ran, tests executed by
	// definition. A requested name that ran in NO partition is a wiring/typo/
	// gated-off fault, not a red — surfaced with a fixed prefix the verify
	// classifier routes to the infra halt.
	if wantNames && anyNamed {
		var missing []string
		for _, want := range opts.Test {
			if !nameExecuted(want, executedNames) {
				missing = append(missing, want)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("requested test(s) never executed: %s — not found in any selected suite; check the test name, that it compiled, and that it isn't gated off (e.g. GH_OPTIVEM_RUN_WIP_TESTS)", strings.Join(missing, ", "))
		}
		return nil
	}

	if anyCounted && totalExecuted == 0 {
		return fmt.Errorf("0 tests executed for the given selection — the suite/test filter matched nothing on any selected suite; check --suite / --test against the available tests")
	}
	return nil
}

// nameExecuted reports whether a requested --test token ran. It is satisfied
// by an exact method-name match or by the token appearing as a substring of an
// executed name, mirroring the runners' own filter semantics (gradle matches
// the bare method; dotnet `~` and playwright `--grep` are substring). The
// tolerance keeps a manual partial filter (e.g. `--test=Cancel`) from
// false-failing while still catching a token that matched nothing at all.
func nameExecuted(requested string, executed map[string]bool) bool {
	if executed[requested] {
		return true
	}
	for e := range executed {
		if strings.Contains(e, requested) {
			return true
		}
	}
	return false
}

// RunSetup runs every entry in tests.SetupCommands in testsCwd. Used by the
// `gh optivem test setup` verb. Each failure halts the run with a wrapped
// error naming the failing command.
func RunSetup(tests *TestsConfig, testsCwd string) error {
	for _, sc := range tests.SetupCommands {
		fmt.Fprintf(os.Stdout, "\n--- Setup: %s ---\n", sc.Name)
		if err := runShell(sc.Command, testsCwd, sc.Env); err != nil {
			return fmt.Errorf("setup %q: %w", sc.Name, err)
		}
	}
	return nil
}

func selectSuites(tests *TestsConfig, suiteIDs []string) ([]Suite, error) {
	if len(suiteIDs) == 0 {
		return tests.Suites, nil
	}
	want := make(map[string]bool, len(suiteIDs))
	for _, id := range suiteIDs {
		want[id] = true
	}
	var missing []string
	for _, id := range suiteIDs {
		if tests.FindSuite(id) == nil {
			missing = append(missing, id)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("suite(s) not found: %s. Available: %s",
			strings.Join(missing, ", "), strings.Join(tests.SuiteIDs(), ", "))
	}
	var picked []Suite
	for _, s := range tests.Suites { // preserve declaration order
		if want[s.ID] {
			picked = append(picked, s)
		}
	}
	return picked, nil
}

// runOneSuite runs a single suite. It returns the number of tests that
// executed and whether that count is meaningful (counted): counted is true
// only when the suite declares a TestCountPath and the run exited cleanly, so
// the caller can distinguish "ran zero tests" from "opted out of counting".
// When a name filter is active (opts.Test non-empty) and the suite declares a
// TestCountPath, names carries the bare method names that executed in this
// suite, so RunTests can assert every requested test ran somewhere across the
// partitioned sub-suites; otherwise names is nil. On any error, it returns
// (0, false, nil, err).
func runOneSuite(suite Suite, testFilter, testFilterJoin, cwd string, opts TestOptions) (executed int, counted bool, names map[string]bool, err error) {
	suiteDir := cwd
	if suite.Path != "" && suite.Path != "." {
		suiteDir = filepath.Join(cwd, suite.Path)
	}

	for _, ic := range suite.TestInstallCommands {
		fmt.Fprintf(os.Stdout, "Installing test dependencies: %s\n", ic)
		if err := runShell(ic, suiteDir, nil); err != nil {
			return 0, false, nil, fmt.Errorf("install %q: %w", ic, err)
		}
	}

	cmd := applyTestFilter(suite.Command, testFilter, testFilterJoin, pickFilterValue(suite, opts))

	fmt.Fprintf(os.Stdout, "\n--- Running %s ---\n", suite.Name)
	if err := runShell(cmd, suiteDir, suite.Env); err != nil {
		if suite.TestReportPath != "" {
			report := filepath.Join(suiteDir, suite.TestReportPath)
			if _, statErr := os.Stat(report); statErr == nil {
				fmt.Fprintf(os.Stdout, "Test report: %s\n", report)
			}
		}
		return 0, false, nil, err
	}
	if suite.TestReportPath != "" {
		fmt.Fprintf(os.Stdout, "Test report: %s\n", filepath.Join(suiteDir, suite.TestReportPath))
	}
	if suite.TestCountPath != "" {
		countPath := filepath.Join(suiteDir, suite.TestCountPath)
		n, err := countExecutedTests(countPath)
		if err != nil {
			return 0, false, nil, fmt.Errorf("counting executed tests: %w", err)
		}
		// Collect executed method names from the same report only for named
		// runs — the presence check is the sole consumer, and unfiltered runs
		// would parse every report for nothing.
		var nm map[string]bool
		if len(opts.Test) > 0 {
			nm, err = executedTestNames(countPath)
			if err != nil {
				return 0, false, nil, fmt.Errorf("reading executed test names: %w", err)
			}
		}
		return n, true, nm, nil
	}
	return 0, false, nil, nil
}

func pickFilterValue(suite Suite, opts TestOptions) []string {
	if len(opts.Test) > 0 {
		return opts.Test
	}
	if opts.Sample && suite.SampleTest != "" {
		return []string{suite.SampleTest}
	}
	return nil
}

// applyTestFilter substitutes the supplied test names into testFilter and
// merges the result into command. join controls multi-value semantics:
//
//	"" / "or"     — join names with "|" and substitute once. Works for runners
//	                that already treat "|" as alternation in their filter
//	                syntax at the value level (playwright/jest `--grep`).
//	"repeat"      — substitute the whole testFilter once per name and append
//	                each result independently. Required when the flag itself
//	                must repeat (e.g. gradle's `--tests T1 --tests T2`).
//	"fragment-or" — for "&"-prefixed injection fragments only. Substitute the
//	                template once per name, join the substituted fragments
//	                with "|", wrap in "(...)", and inject as one expression.
//	                Required for `dotnet test --filter` where "|" only ORs
//	                full property terms, not bare values
//	                (`&(DisplayName~T1|DisplayName~T2)`).
//
// Returns command unchanged when names is empty or testFilter is empty.
func applyTestFilter(command, testFilter, join string, names []string) string {
	if len(names) == 0 || testFilter == "" {
		return command
	}
	if join == "repeat" {
		for _, name := range names {
			expr := strings.ReplaceAll(testFilter, "<test>", name)
			command = appendTestFilter(command, expr)
		}
		return command
	}
	if join == "fragment-or" {
		if !strings.HasPrefix(testFilter, "&") {
			fmt.Fprintf(os.Stderr, "testFilterJoin: fragment-or requires testFilter to begin with '&'; got %q — leaving command unchanged\n", testFilter)
			return command
		}
		body := strings.TrimPrefix(testFilter, "&")
		fragments := make([]string, len(names))
		for i, name := range names {
			fragments[i] = strings.ReplaceAll(body, "<test>", name)
		}
		expr := "&(" + strings.Join(fragments, "|") + ")"
		return appendTestFilter(command, expr)
	}
	expr := strings.ReplaceAll(testFilter, "<test>", strings.Join(names, "|"))
	return appendTestFilter(command, expr)
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
// streamed to the user's terminal and the last 16 KB are mirrored into the
// returned error so a failure's wrap is self-contained (the live stream may
// have scrolled off or been redirected to a log file). The command is parsed
// via the same quote-aware splitter used elsewhere in this codebase to avoid
// `sh -c` and the platform-specific shell quoting it brings.
func runShell(command, cwd string, env map[string]string) error {
	parts, err := splitCommand(command)
	if err != nil {
		return fmt.Errorf("invalid command %q: %w", command, err)
	}
	if len(parts) == 0 {
		return errors.New("empty command")
	}
	cmd := exec.Command(pathx.NormalizeExe(parts[0]), parts[1:]...)
	cmd.Dir = cwd
	tail := &tailWriter{cap: 16 * 1024}
	cmd.Stdout = io.MultiWriter(os.Stdout, tail)
	cmd.Stderr = io.MultiWriter(os.Stderr, tail)
	if len(env) > 0 {
		cmd.Env = mergeEnv(os.Environ(), env)
	}
	// On Windows, args destined for .bat/.cmd targets need to survive cmd.exe's
	// metacharacter parsing (|, &, <, >, etc.) before the batch file's CRT
	// sees them. No-op on non-Windows.
	applyCmdExeQuoting(cmd, parts)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w\nstderr tail:\n%s", command, err, tail.String())
	}
	return nil
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
