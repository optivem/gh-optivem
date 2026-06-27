package actions

import (
	"context"
	"io"
	"os"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/outlog"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
	trackergithub "github.com/optivem/gh-optivem/internal/atdd/runtime/tracker/github"
	"github.com/optivem/gh-optivem/internal/build/runner"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// Deps bundles the side-effecting collaborators every action may need. All
// fields are optional; a zero-value Deps falls back to real shell-outs and
// the OS stdin/stdout. Tests pass non-nil fakes for hermeticity.
type Deps struct {
	Gh       GhRunner
	Git      GitRunner
	Shell    ShellRunner // for the BPMN Phase D `run-command` primitive
	Prompter Prompter
	// Stdout is the back-compat single-writer fallback used when Out is
	// nil (tests that pre-date the level architecture). Production paths
	// populate Out, and realShell pipes subprocess stdout to Out.Detail
	// — fixing the pre-existing "subprocess output bypasses --log-file"
	// leak that existed when realShell wrote directly to os.Stdout.
	Stdout io.Writer
	Stderr io.Writer
	// Out routes Fprint sites (including subprocess stdout in realShell)
	// by level. nil → withDefaults builds outlog.Default(Stdout) so test
	// fixtures that only set Stdout still see every write.
	Out        *outlog.Out
	ProjectURL string // optional — explicit override for tracker operations
	RepoPath   string // optional — defaults to current working directory
	// Config is the already-loaded gh-optivem.yaml. Threaded in by the
	// driver so scope-checking actions (check-phase-scope,
	// validate-outputs-and-scopes) read the same file the operator passed
	// via --config / $GH_OPTIVEM_CONFIG, not a hard-coded
	// <repoPath>/gh-optivem.yaml. nil is treated as a wiring bug — the
	// affected actions surface a hard error.
	Config *projectconfig.Config
	// TestsConfig is the already-loaded tests.yaml (runner.TestsConfig),
	// threaded in by the driver exactly as Config is. resolve-channel /
	// validate-channels-registered read the RED acceptance run's on-disk
	// report through runner.NamesInReport (plan 20260619-1139, decision #6) to
	// answer channel membership without running anything. nil is treated as a
	// wiring bug — the affected actions hard-error when a baked channel needs it.
	TestsConfig *runner.TestsConfig
	// TestsCwd is the directory the test runner resolves suite paths against —
	// filepath.Dir of the resolved tests.yaml, made absolute against the repo
	// (the driver joins cfg.SystemTest.Config onto repoPath). The RED acceptance
	// reports live under <TestsCwd>/<suite.Path>/<testCountPath>, so this is the
	// base runner.NamesInReport reads from. Empty when no tests.yaml is wired.
	TestsCwd string
	// Tracker is the seam tracker-shaped actions (SetStatus, ReadBody,
	// Classify, Subtypes, FindIssue) route through. Optional — withDefaults
	// constructs a github adapter from ProjectURL + Gh when unset. Tests
	// inject fakes either by setting ProjectURL + a fake Gh (the
	// constructed github tracker then routes through the fake), or by
	// setting Tracker directly for full control.
	Tracker tracker.Tracker
	// Engine is the loaded process-flow state machine. Scope-checking
	// actions (validate-outputs-and-scopes, check-phase-scope) call
	// Engine.Scope(processName) to resolve per-phase read / write lists
	// from the inline node scope in process-flow.yaml. nil is a wiring
	// bug — the affected actions surface a hard error.
	Engine *statemachine.Engine
}

// Prompter is the same interface gates uses; redefined here so the actions
// package does not import gates (each registry stays self-contained).
type Prompter interface {
	Ask(prompt string) (string, error)
}

// GhRunner runs the `gh` CLI.
type GhRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// GitRunner runs the `git` CLI.
type GitRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ShellRunner runs an arbitrary command. We use a bash-style CommandLine
// (no argv split) so the BPMN Phase D `run-command` primitive can pass
// any templated command line verbatim and tests can match against "the
// exact string you would type at a prompt".
//
// The returned ShellResult carries stdout, stderr, and the exit code so
// `runCommand` can surface a diagnostic payload (failure-kind +
// command-line + command-exit-code + command-stderr-tail) into ctx.State
// when the command fails, which the downstream `fix-command-failed`
// dispatch consumes via its prompt placeholders. Stderr is also embedded
// in the returned error for human-readable surfacing.
type ShellRunner interface {
	Run(ctx context.Context, commandLine string) (ShellResult, error)
}

// ShellResult is the rich return of a shell dispatch. Stdout / Stderr
// are populated for every run (success or failure); ExitCode is 0 on
// success and the OS-reported exit status on failure (or -1 when the
// process never started, e.g. command not found — Go's
// `*exec.ExitError` returns -1 in that case via ExitCode()).
type ShellResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

func (d Deps) withDefaults() Deps {
	if d.Gh == nil {
		d.Gh = realGh{}
	}
	if d.Git == nil {
		d.Git = realGit{}
	}
	if d.Prompter == nil {
		d.Prompter = stdinPrompter{}
	}
	if d.Stdout == nil {
		d.Stdout = os.Stdout
	}
	if d.Stderr == nil {
		d.Stderr = os.Stderr
	}
	if d.Out == nil {
		d.Out = outlog.Default(d.Stdout)
	}
	// realShell wiring must follow Out defaulting — its writers are read
	// from d.Out.Detail so subprocess byte streams route to the verbose
	// sink (and only there, by default). Earlier "Shell == nil" branch
	// position would have referenced d.Out before it was populated.
	if d.Shell == nil {
		d.Shell = realShell{stdout: d.Out.Detail, stderr: d.Stderr}
	}
	if d.Tracker == nil {
		// Default to a github adapter wrapping the (possibly fake) Gh
		// runner. Production callers set ProjectURL from gh-optivem.yaml
		// (driver.go) so SetStatus / Verify / FindIssue resolve against a
		// real project; tests that don't exercise project ops can omit
		// ProjectURL and the placeholder below keeps github.New from
		// rejecting the call — issue-body ops (ReadBody / Classify)
		// only need Issue.URL anyway.
		url := d.ProjectURL
		if url == "" {
			url = "https://github.com/orgs/placeholder/projects/0"
		}
		if t, err := trackergithub.New(url, ghAdapter{d.Gh}); err == nil {
			d.Tracker = t
		}
	}
	return d
}

type actions struct {
	deps Deps
}

// WorkingTreeFingerprint is a snapshot of dirty working-tree files
// captured immediately before an agent runs. Keys are repo-relative
// paths (the same paths `git status --porcelain` reports); values are
// hex-encoded SHA-256 hashes of the file bytes on disk at snapshot
// time, or "" for paths the snapshotter saw in `git status` but could
// not read (deleted between enumeration and read — equivalent to a
// post-snapshot delete).
//
// Clean tracked files are intentionally absent: a file clean at
// snapshot time and dirty afterwards appears in the post-state
// `git status` and is added to the delta as "absent in snapshot,
// present now".
type WorkingTreeFingerprint map[string]string
