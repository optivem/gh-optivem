// runner_commands.go wires the `build system`, `run system`, `test system`,
// `stop system`, and `clean system` subcommands into the main package. The
// runner package is fully agnostic — these handlers just translate CLI flags
// into runner.* calls.
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

	flagSystemUsage = "Path to system.json"
	flagTestsUsage  = "Path to tests.json"

	errorFormat = "ERROR: %v\n"
)

// exitOnError prints a formatted error to stderr and exits with status 1 when
// err is non-nil. Used by every runner subcommand to keep the load/run
// boilerplate terse.
func exitOnError(err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, errorFormat, err)
	os.Exit(1)
}

// runBuildSystem implements `gh optivem build system [--system path] [--rebuild]`.
// Builds every entry in systems[] via `docker compose build`. With --rebuild,
// every layer is rebuilt from scratch (internally `docker compose build
// --no-cache`). Analog of dotnet's --no-incremental and gradle's
// --rerun-tasks — outcome-oriented naming.
func runBuildSystem(args []string) {
	fs := flag.NewFlagSet("build system", flag.ExitOnError)
	systemPath := fs.String("system", defaultSystemConfig, flagSystemUsage)
	rebuild := fs.Bool("rebuild", false, "Force a full rebuild from scratch (no layer cache reuse)")
	_ = fs.Parse(args)

	sys, err := runner.LoadSystem(*systemPath)
	exitOnError(err)
	exitOnError(runner.Build(sys, cwdForPath(*systemPath), runner.BuildOptions{Rebuild: *rebuild}))
}

// runRunSystem implements `gh optivem run system [--system path] [--restart] [--log-lines 50]`.
// Brings up every entry in systems[] and waits for health.
func runRunSystem(args []string) {
	fs := flag.NewFlagSet("run system", flag.ExitOnError)
	systemPath := fs.String("system", defaultSystemConfig, flagSystemUsage)
	restart := fs.Bool("restart", false, "Force tear-down + restart even if the system is already up")
	logLines := fs.Int("log-lines", 50, "Lines of compose logs to dump on health-probe failure")
	_ = fs.Parse(args)

	sys, err := runner.LoadSystem(*systemPath)
	exitOnError(err)
	opts := runner.SystemOptions{LogLines: *logLines, Restart: *restart}
	exitOnError(runner.Up(sys, cwdForPath(*systemPath), opts))
}

// runStopSystem implements `gh optivem stop system [--system path]`.
// Tears down every entry in systems[] and force-removes stray containers.
func runStopSystem(args []string) {
	fs := flag.NewFlagSet("stop system", flag.ExitOnError)
	systemPath := fs.String("system", defaultSystemConfig, flagSystemUsage)
	_ = fs.Parse(args)

	sys, err := runner.LoadSystem(*systemPath)
	exitOnError(err)
	exitOnError(runner.Down(sys, cwdForPath(*systemPath)))
}

// runCleanSystem implements `gh optivem clean system [--system path]`.
// Tears down every entry in systems[] and removes its named volumes plus
// locally-built images (`docker compose down -v --rmi local`). Analog of
// `dotnet clean` and `./gradlew clean` — deletes build outputs without
// touching dependency caches (registry-pulled images are kept).
func runCleanSystem(args []string) {
	fs := flag.NewFlagSet("clean system", flag.ExitOnError)
	systemPath := fs.String("system", defaultSystemConfig, flagSystemUsage)
	_ = fs.Parse(args)

	sys, err := runner.LoadSystem(*systemPath)
	exitOnError(err)
	exitOnError(runner.Clean(sys, cwdForPath(*systemPath)))
}

// runTestSystem implements:
//
//	gh optivem test system [--system path] [--tests path]
//	                       [--suite id] [--test name] [--sample]
//	                       [--no-build] [--no-start] [--restart]
//
// By default, builds images (incremental), starts the system if not already
// up, then runs setup commands and suites. Inspired by `dotnet test` and
// `./gradlew test` which build the test code implicitly before running.
//
// --no-build skips our explicit Build step (compose `up` may still build
// missing images). --no-start skips Up; the system must already be up or
// the runner errors out. --restart forces tear-down + restart during Up.
func runTestSystem(args []string) {
	fs := flag.NewFlagSet("test system", flag.ExitOnError)
	systemPath := fs.String("system", defaultSystemConfig, flagSystemUsage)
	testsPath := fs.String("tests", defaultTestsConfig, flagTestsUsage)
	suite := fs.String("suite", "", "Run only the suite with this id")
	test := fs.String("test", "", "Narrow execution to one test name (substituted into the suite's testFilter)")
	sample := fs.Bool("sample", false, "Use each suite's sampleTest field as the test name")
	noBuild := fs.Bool("no-build", false, "Skip the implicit build step (analog of dotnet test --no-build)")
	noStart := fs.Bool("no-start", false, "Skip the implicit start step; system must already be up")
	restart := fs.Bool("restart", false, "Force tear-down + restart during the implicit start step")
	_ = fs.Parse(args)

	sys, err := runner.LoadSystem(*systemPath)
	exitOnError(err)
	tests, err := runner.LoadTests(*testsPath)
	exitOnError(err)
	opts := runner.TestOptions{
		Suite:   *suite,
		Test:    *test,
		Sample:  *sample,
		NoBuild: *noBuild,
		NoStart: *noStart,
		Restart: *restart,
	}
	exitOnError(runner.RunTests(sys, tests, cwdForPath(*testsPath), opts))
}

// dispatchBuild routes `gh optivem build <noun>`. Currently only `system` is
// supported; new nouns can be added here without touching main().
func dispatchBuild(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: gh optivem build system [--system path] [--rebuild]")
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

// dispatchClean routes `gh optivem clean <noun>`.
func dispatchClean(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: gh optivem clean system [--system path]")
		os.Exit(1)
	}
	switch args[0] {
	case "system":
		runCleanSystem(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "Unknown clean target: %s\n", args[0])
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
