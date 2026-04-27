// runner_commands.go wires the `build system`, `run system`, `test system`,
// and `stop system` subcommands into the main package. The runner package is
// fully agnostic — these handlers just translate CLI flags into runner.*
// calls.
//
// Working-dir contract: each command operates against the user's current
// working directory. JSON config paths default to ./system.json and
// ./tests.json; both can be overridden with --system / --tests.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/optivem/gh-optivem/internal/runner"
)

// cwdForPath returns the directory to run docker / setup / suite commands in
// for a given config file. Compose paths in system.json are relative to
// system.json's directory; setup commands and suite.path in tests.json are
// relative to tests.json's directory. This lets shop's source layout
// (system.json under <lang>/<arch>/, tests-*.json + package.json under
// <lang>/) work without per-layout flags. In a scaffolded project both files
// live in the same directory and this is just ".".
func cwdForPath(configPath string) string {
	dir := filepath.Dir(configPath)
	if dir == "" {
		return "."
	}
	return dir
}

const (
	defaultSystemConfig = "./system.json"
	defaultTestsConfig  = "./tests.json"
)

// runBuildSystem implements `gh optivem build system [--system path]`.
// Builds every entry in systems[] via `docker compose build`.
func runBuildSystem(args []string) {
	fs := flag.NewFlagSet("build system", flag.ExitOnError)
	systemPath := fs.String("system", defaultSystemConfig, "Path to system.json")
	_ = fs.Parse(args)

	sys, err := runner.LoadSystem(*systemPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	if err := runner.Build(sys, cwdForPath(*systemPath)); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

// runRunSystem implements `gh optivem run system [--system path] [--restart] [--log-lines 50]`.
// Brings up every entry in systems[] and waits for health.
func runRunSystem(args []string) {
	fs := flag.NewFlagSet("run system", flag.ExitOnError)
	systemPath := fs.String("system", defaultSystemConfig, "Path to system.json")
	restart := fs.Bool("restart", false, "Force tear-down + restart even if the system is already up")
	logLines := fs.Int("log-lines", 50, "Lines of compose logs to dump on health-probe failure")
	_ = fs.Parse(args)

	sys, err := runner.LoadSystem(*systemPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	opts := runner.SystemOptions{
		LogLines: *logLines,
		Restart:  *restart,
	}
	if err := runner.Up(sys, cwdForPath(*systemPath), opts); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

// runStopSystem implements `gh optivem stop system [--system path]`.
// Tears down every entry in systems[] and force-removes stray containers.
func runStopSystem(args []string) {
	fs := flag.NewFlagSet("stop system", flag.ExitOnError)
	systemPath := fs.String("system", defaultSystemConfig, "Path to system.json")
	_ = fs.Parse(args)

	sys, err := runner.LoadSystem(*systemPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	if err := runner.Down(sys, cwdForPath(*systemPath)); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

// runTestSystem implements:
//
//	gh optivem test system [--system path] [--tests path]
//	                       [--suite id] [--test name] [--sample]
//
// Runs setup commands then iterates suites. The system must already be up
// (the runner verifies via an HTTP probe and errors out otherwise) — bring
// it up first with `gh optivem run system`.
func runTestSystem(args []string) {
	fs := flag.NewFlagSet("test system", flag.ExitOnError)
	systemPath := fs.String("system", defaultSystemConfig, "Path to system.json")
	testsPath := fs.String("tests", defaultTestsConfig, "Path to tests.json")
	suite := fs.String("suite", "", "Run only the suite with this id")
	test := fs.String("test", "", "Narrow execution to one test name (substituted into the suite's testFilter)")
	sample := fs.Bool("sample", false, "Use each suite's sampleTest field as the test name")
	_ = fs.Parse(args)

	sys, err := runner.LoadSystem(*systemPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	tests, err := runner.LoadTests(*testsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
	opts := runner.TestOptions{
		Suite:  *suite,
		Test:   *test,
		Sample: *sample,
	}
	if err := runner.RunTests(sys, tests, cwdForPath(*testsPath), opts); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

// dispatchBuild routes `gh optivem build <noun>`. Currently only `system` is
// supported; new nouns can be added here without touching main().
func dispatchBuild(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: gh optivem build system [--system path]")
		os.Exit(1)
	}
	switch args[0] {
	case "system":
		runBuildSystem(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown build target: %s\n", args[0])
		os.Exit(1)
	}
}

// dispatchStop routes `gh optivem stop <noun>`.
func dispatchStop(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: gh optivem stop system [--system path]")
		os.Exit(1)
	}
	switch args[0] {
	case "system":
		runStopSystem(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown stop target: %s\n", args[0])
		os.Exit(1)
	}
}

// dispatchRun routes `gh optivem run <noun>`.
func dispatchRun(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: gh optivem run system [flags]")
		os.Exit(1)
	}
	switch args[0] {
	case "system":
		runRunSystem(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown run target: %s\n", args[0])
		os.Exit(1)
	}
}

// dispatchTest routes `gh optivem test <noun>`.
func dispatchTest(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: gh optivem test system [--system path] [--tests path]")
		os.Exit(1)
	}
	switch args[0] {
	case "system":
		runTestSystem(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown test target: %s\n", args[0])
		os.Exit(1)
	}
}
