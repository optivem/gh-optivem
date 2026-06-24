package componenttest

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/build/runner"
)

// Component is one codebase carrying its own component-tests.yaml. Name is the
// logical id used by `--component` and the output grouping (backend / frontend
// for multitier, monolith for a single-component system, or a service name for
// microservices); Path is its repo-relative directory (resolved against the
// caller's cwd, like the system-test runner); Lang is display-only.
type Component struct {
	Name string
	Path string
	Lang string
}

// label renders a component header as "backend (java)", or just the name when
// no language is set.
func (c Component) label() string {
	if c.Lang == "" {
		return c.Name
	}
	return fmt.Sprintf("%s (%s)", c.Name, c.Lang)
}

// Options narrows a component-test run. Empty Suites means every suite; empty
// Components means every discovered component — so bare `run` (neither set) is
// every suite × every component, the CI-gate-equivalent full set.
type Options struct {
	Suites     []string
	Components []string
	Sample     bool
	Test       []string
}

// result records one suite outcome for the end-of-run summary.
type result struct {
	component string
	suite     string
	status    string // "PASSED" | "FAILED" | "PENDING"
	duration  time.Duration
}

// Run executes the selected suites across the selected components. Components is
// the full discovered set; opts.Components narrows it. Each component loads its
// own component-tests.yaml, resolves opts.Suites against that config, and runs
// the suites in declaration order. The first failure stops the run and is
// returned; the per-component/suite summary is printed regardless.
func Run(components []Component, opts Options) error {
	selected, err := selectComponents(components, opts.Components)
	if err != nil {
		return err
	}

	// Load + resolve every component up front so a missing config or a bad
	// --suite for any component fails before a single suite runs (no
	// half-executed gate).
	type plan struct {
		comp   Component
		suites []Suite
	}
	var plans []plan
	for _, c := range selected {
		cfg, err := Load(filepath.Join(c.Path, ConfigFileName))
		if err != nil {
			return err
		}
		suites, err := cfg.selectSuites(opts.Suites)
		if err != nil {
			return fmt.Errorf("component %s: %w", c.Name, err)
		}
		plans = append(plans, plan{comp: c, suites: suites})
	}

	var results []result
	defer func() { printSummary(results) }()
	for _, p := range plans {
		fmt.Fprintf(os.Stdout, "\n=== Component: %s ===\n", p.comp.label())
		for _, s := range p.suites {
			r, err := runSuite(p.comp, s, opts)
			results = append(results, r)
			if err != nil {
				return fmt.Errorf("component %s suite %s: %w", p.comp.Name, s.Name, err)
			}
		}
	}
	return nil
}

// runSuite runs (or skips) one suite. A pending suite is never executed; a
// requiresDocker suite preflights the Docker daemon and fails fast when it is
// unreachable. Otherwise the suite command runs in its resolved cwd with the
// suite env and any --test/--sample filter applied.
func runSuite(c Component, s Suite, opts Options) (result, error) {
	if s.Pending {
		fmt.Fprintf(os.Stdout, "\n--- %s: not implemented yet (pending) — skipping ---\n", s.Name)
		return result{c.Name, s.Name, "PENDING", 0}, nil
	}
	if s.RequiresDocker && !dockerAvailable() {
		return result{c.Name, s.Name, "FAILED", 0},
			fmt.Errorf("suite %q requires Docker but no Docker daemon is reachable (`docker info` failed) — start Docker Desktop / the daemon and retry", s.Name)
	}

	cwd := c.Path
	if s.Path != "" && s.Path != "." {
		cwd = filepath.Join(c.Path, s.Path)
	}
	// Component configs reuse the same <test> placeholder semantics as
	// tests.yaml; TestFilter/TestFilterJoin live at the config level there too,
	// but the per-suite filter here is supplied via the shared runner helper.
	cmd := runner.ApplyTestFilter(s.Command, "", "", pickFilterValue(s, opts))

	fmt.Fprintf(os.Stdout, "\n--- Running %s ---\n", s.Name)
	start := time.Now()
	runErr := runner.RunShell(cmd, cwd, s.Env)
	dur := time.Since(start)

	if s.TestReportPath != "" {
		report := filepath.Join(cwd, s.TestReportPath)
		if _, statErr := os.Stat(report); statErr == nil {
			fmt.Fprintf(os.Stdout, "Test report: %s\n", report)
		}
	}
	if runErr != nil {
		return result{c.Name, s.Name, "FAILED", dur}, runErr
	}
	return result{c.Name, s.Name, "PASSED", dur}, nil
}

// pickFilterValue chooses the test-name filter for a suite: an explicit --test
// wins; otherwise --sample uses the suite's sampleTest; otherwise no filter.
func pickFilterValue(s Suite, opts Options) []string {
	if len(opts.Test) > 0 {
		return opts.Test
	}
	if opts.Sample && s.SampleTest != "" {
		return []string{s.SampleTest}
	}
	return nil
}

// RunSetup runs the setupCommands of every selected component (in the
// component's own directory). Used by `gh optivem component-test setup`.
func RunSetup(components []Component, requested []string) error {
	selected, err := selectComponents(components, requested)
	if err != nil {
		return err
	}
	for _, c := range selected {
		cfg, err := Load(filepath.Join(c.Path, ConfigFileName))
		if err != nil {
			return err
		}
		if len(cfg.SetupCommands) == 0 {
			continue
		}
		fmt.Fprintf(os.Stdout, "\n=== Setup: %s ===\n", c.label())
		for _, sc := range cfg.SetupCommands {
			fmt.Fprintf(os.Stdout, "\n--- Setup: %s ---\n", sc.Name)
			if err := runner.RunShell(sc.Command, c.Path, sc.Env); err != nil {
				return fmt.Errorf("component %s setup %q: %w", c.Name, sc.Name, err)
			}
		}
	}
	return nil
}

// CompileResult records one component's compile outcome so callers can render a
// per-component summary row (mirroring the system/system-test compile rows).
type CompileResult struct {
	Component string
	Lang      string
	Path      string
	Duration  time.Duration
	Err       error
}

// Compile runs the compileCommands of every selected component (in the
// component's own directory), halting on the first failure. It backs both
// `gh optivem component-test compile` and the component-test leg of the bare
// `compile` aggregate.
//
// Unlike RunSetup it is tolerant of a component that has nothing to compile:
// a missing component-tests.yaml or an empty compileCommands list skips that
// component silently (no result row). compileCommands is an optional, additive
// field, so the bare `compile` aggregate must not start failing on a project
// that has components but no component-test compile declared. It returns one
// CompileResult per component that actually ran a compile (including the
// failing one) plus the first error encountered.
func Compile(components []Component, requested []string) ([]CompileResult, error) {
	selected, err := selectComponents(components, requested)
	if err != nil {
		return nil, err
	}
	var results []CompileResult
	for _, c := range selected {
		path := filepath.Join(c.Path, ConfigFileName)
		if _, statErr := os.Stat(path); errors.Is(statErr, fs.ErrNotExist) {
			continue // nothing to compile for this component
		}
		cfg, err := Load(path)
		if err != nil {
			return results, err
		}
		if len(cfg.CompileCommands) == 0 {
			continue
		}
		fmt.Fprintf(os.Stdout, "\n=== Compile: %s ===\n", c.label())
		start := time.Now()
		runErr := runCompileCommands(c, cfg.CompileCommands)
		results = append(results, CompileResult{
			Component: c.Name,
			Lang:      c.Lang,
			Path:      c.Path,
			Duration:  time.Since(start),
			Err:       runErr,
		})
		if runErr != nil {
			return results, runErr
		}
	}
	return results, nil
}

// runCompileCommands runs a component's compileCommands in its directory,
// halting on the first failure.
func runCompileCommands(c Component, cmds []SetupCommand) error {
	for _, cc := range cmds {
		fmt.Fprintf(os.Stdout, "\n--- Compile: %s ---\n", cc.Name)
		if err := runner.RunShell(cc.Command, c.Path, cc.Env); err != nil {
			return fmt.Errorf("component %s compile %q: %w", c.Name, cc.Name, err)
		}
	}
	return nil
}

// List prints each selected component and its suite ids (pending ones marked),
// then returns — the component-tier `--list`, without running anything.
func List(components []Component, requested []string) error {
	selected, err := selectComponents(components, requested)
	if err != nil {
		return err
	}
	for _, c := range selected {
		cfg, err := Load(filepath.Join(c.Path, ConfigFileName))
		if err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "%s\n", c.label())
		for _, s := range cfg.Suites {
			marker := ""
			if s.Pending {
				marker = " (pending)"
			}
			fmt.Fprintf(os.Stdout, "  %s%s\n", s.ID, marker)
		}
	}
	return nil
}

// selectComponents filters the discovered components by the requested names.
// Empty requested means all. An unknown name fails loud with the available set.
func selectComponents(all []Component, requested []string) ([]Component, error) {
	if len(all) == 0 {
		return nil, fmt.Errorf("no components found — set system.path (monolith) or system.backend/frontend.path (multitier) in gh-optivem.yaml")
	}
	if len(requested) == 0 {
		return all, nil
	}
	byName := make(map[string]Component, len(all))
	names := make([]string, len(all))
	for i, c := range all {
		byName[c.Name] = c
		names[i] = c.Name
	}
	var picked []Component
	var missing []string
	seen := make(map[string]bool, len(requested))
	for _, req := range requested {
		if seen[req] {
			continue
		}
		seen[req] = true
		c, ok := byName[req]
		if !ok {
			missing = append(missing, req)
			continue
		}
		picked = append(picked, c)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("component(s) not found: %s. Available: %s",
			strings.Join(missing, ", "), strings.Join(names, ", "))
	}
	return picked, nil
}

// dockerAvailable reports whether a Docker daemon is reachable, by running
// `docker info` quietly (output discarded — this is a preflight, not a suite).
func dockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

// printSummary renders the per-component/suite results table at the end of a
// run. No-op when nothing ran (e.g. an error before the first suite).
func printSummary(results []result) {
	if len(results) == 0 {
		return
	}
	fmt.Fprintln(os.Stdout)
	fmt.Fprintln(os.Stdout, "Component Test Results:")
	fmt.Fprintln(os.Stdout, strings.Repeat("-", 78))
	for _, r := range results {
		fmt.Fprintf(os.Stdout, "  %-14s %-28s %-8s %s\n", r.component, r.suite, r.status, formatDur(r.duration))
	}
	fmt.Fprintln(os.Stdout, strings.Repeat("-", 78))
}

// formatDur renders a duration as mm:ss.mmm, matching the system-test runner's
// summary format.
func formatDur(d time.Duration) string {
	d = d.Round(time.Millisecond)
	mins := int(d / time.Minute)
	secs := int(d/time.Second) % 60
	ms := int(d/time.Millisecond) % 1000
	return fmt.Sprintf("%02d:%02d.%03d", mins, secs, ms)
}
