// Package clauderun shells out to the `claude` CLI to dispatch a named ATDD
// agent for the current phase, replacing v1's "pause and let the operator
// launch the agent in a second window" workflow.
//
// Dispatch builds a prompt from prompt.tmpl, invokes `claude` (interactive or
// `claude -p` autonomous), and detects success by diffing git HEAD before
// and after. The agent is expected to commit on the same branch — that
// commit landing is what the engine's downstream verify decorator keys off.
//
// The package exposes a ClaudeRunner / GitRunner pair so tests can inject
// canned exit codes and HEAD values; the production defaults exec the real
// CLIs the same way the gates / actions / classify / release packages do.
package clauderun

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/fatih/color"
)

//go:embed prompt.tmpl
var promptTmplSrc string

var promptTmpl = template.Must(template.New("clauderun").Parse(promptTmplSrc))

// Options bundles every input Dispatch needs to construct a prompt and run
// the subprocess. Zero values yield a usable configuration where it makes
// sense (Stdout/Stderr/Stdin default to the OS streams). Required fields
// (Agent, PhaseDoc, IssueNum, IssueTitle, IssueRepo, NodeDescription) are
// not zero-defaulted because missing them yields a meaningless prompt.
type Options struct {
	// Agent is the subagent name to launch (e.g. "atdd-test").
	Agent string

	// PhaseDoc is the relative path to the phase's process document
	// (e.g. "docs/atdd/process/at-red-test.md").
	PhaseDoc string

	// NodeDescription is the YAML node's `description:` — surfaced in the
	// prompt so the agent has the same context the operator would have read.
	NodeDescription string

	// Ticket context — pulled from Context keys populated by preResolveIssue.
	IssueNum     int
	IssueTitle   string
	IssueRepo    string
	ProjectTitle string
	ProjectURL   string

	// OverrideText is the per-node `--extra` text from override.Hooks,
	// interpolated into the prompt template. Empty string is fine.
	OverrideText string

	// RawPrompt, when non-empty, replaces the templated prompt entirely.
	// Used by override.Hooks Replace where the operator wants to swap the
	// whole prompt rather than append text via OverrideText. When set,
	// every other prompt-shaping field (NodeDescription, OverrideText, …)
	// is ignored.
	RawPrompt string

	// Autonomous — when true, run via `claude -p` (one-shot, headless).
	// When false, run interactively so the operator can observe / interject.
	Autonomous bool

	// RepoPath is the working directory the subprocess runs in (and the
	// directory git rev-parse / git log query). Empty → current cwd.
	RepoPath string

	// Stdout / Stderr targets for the dispatch banners and (in autonomous
	// mode) the streamed subprocess output. nil → os.Stdout / os.Stderr.
	Stdout io.Writer
	Stderr io.Writer

	// Stdin is the operator's TTY in interactive mode. nil → os.Stdin.
	Stdin io.Reader
}

// CommitInfo is the result of a successful Dispatch — the engine uses
// these to surface the new commit to the driver's stdout and to feed the
// downstream verify decorator.
type CommitInfo struct {
	SHA     string
	Subject string
}

// ClaudeRunner runs the `claude` CLI. The default implementation is
// execClaude. RunOpts is a struct rather than a varargs slice because the
// runner has to choose between interactive and `-p` invocations and stream
// stdout/stderr back to the driver during long autonomous runs.
type ClaudeRunner interface {
	Run(ctx context.Context, opts RunOpts) error
}

// RunOpts is the cross-cut between Options and the subprocess invocation.
// It hides the autonomous-vs-interactive flag-shape decision behind the
// runner so the production runner can evolve without touching Dispatch.
type RunOpts struct {
	Prompt     string
	Autonomous bool
	Dir        string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
}

// GitRunner runs the `git` CLI in a given directory. Mirrors the GhRunner
// shape used elsewhere but adds a working-directory parameter because the
// driver may dispatch against a sub-checkout.
type GitRunner interface {
	Run(ctx context.Context, dir string, args ...string) ([]byte, error)
}

// Deps lets tests substitute fake runners. Production callers pass a
// zero-value Deps and Dispatch falls back to real `claude` / `git`.
type Deps struct {
	Claude ClaudeRunner
	Git    GitRunner
}

func (d Deps) withDefaults() Deps {
	if d.Claude == nil {
		d.Claude = execClaude{}
	}
	if d.Git == nil {
		d.Git = execGit{}
	}
	return d
}

// Dispatch builds the prompt, runs the subprocess, and verifies a commit
// landed. Returns CommitInfo on success. Errors are returned for:
//   - Subprocess exit status non-zero (stderr surfaced).
//   - Subprocess exit zero but HEAD unchanged (the agent ran clean but
//     produced no commit — same shape as v1's "abort" path).
//   - Any of the surrounding git / template steps failing.
func Dispatch(ctx context.Context, deps Deps, opts Options) (CommitInfo, error) {
	deps = deps.withDefaults()
	opts = opts.withDefaults()

	prompt := opts.RawPrompt
	if prompt == "" {
		var err error
		prompt, err = renderPrompt(opts)
		if err != nil {
			return CommitInfo{}, fmt.Errorf("clauderun: render prompt: %w", err)
		}
	}

	headBefore, err := readHEAD(ctx, deps.Git, opts.RepoPath)
	if err != nil {
		return CommitInfo{}, fmt.Errorf("clauderun: read HEAD before dispatch: %w", err)
	}

	writeEnterBanner(opts)
	startedAt := nowFn()

	runErr := deps.Claude.Run(ctx, RunOpts{
		Prompt:     prompt,
		Autonomous: opts.Autonomous,
		Dir:        opts.RepoPath,
		Stdin:      opts.Stdin,
		Stdout:     opts.Stdout,
		Stderr:     opts.Stderr,
	})
	if runErr != nil {
		writeExitBanner(opts, "", "", nowFn().Sub(startedAt), runErr)
		return CommitInfo{}, fmt.Errorf("clauderun: %s exited non-zero: %w", opts.Agent, runErr)
	}

	headAfter, err := readHEAD(ctx, deps.Git, opts.RepoPath)
	if err != nil {
		writeExitBanner(opts, "", "", nowFn().Sub(startedAt), err)
		return CommitInfo{}, fmt.Errorf("clauderun: read HEAD after dispatch: %w", err)
	}

	if headBefore == headAfter {
		writeExitBanner(opts, "", "", nowFn().Sub(startedAt), errNoCommit)
		return CommitInfo{}, fmt.Errorf("clauderun: %s exited cleanly but produced no commit (HEAD unchanged at %s)",
			opts.Agent, shortSHA(headBefore))
	}

	subject, err := readCommitSubject(ctx, deps.Git, opts.RepoPath, headAfter)
	if err != nil {
		writeExitBanner(opts, headAfter, "", nowFn().Sub(startedAt), err)
		return CommitInfo{}, fmt.Errorf("clauderun: read commit subject for %s: %w", shortSHA(headAfter), err)
	}

	writeExitBanner(opts, headAfter, subject, nowFn().Sub(startedAt), nil)
	return CommitInfo{SHA: headAfter, Subject: subject}, nil
}

// errNoCommit is the sentinel for "subprocess succeeded but HEAD didn't
// move". Surfaced through writeExitBanner so the operator sees the failure
// banner rather than a silent return.
var errNoCommit = errors.New("no commit produced")

// nowFn is a package-level seam so tests can pin elapsed time in banner
// output. Production points at time.Now.
var nowFn = time.Now

// renderPrompt expands prompt.tmpl with opts. Public-ish for the test
// file; not exported.
func renderPrompt(opts Options) (string, error) {
	var buf bytes.Buffer
	if err := promptTmpl.Execute(&buf, opts); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// RenderPrompt is the public counterpart to renderPrompt: it expands
// prompt.tmpl with opts and returns the prompt string Dispatch would
// hand to the subprocess, without invoking it. The driver's agent
// dispatcher uses this for the --interactive prompt-review hook so the
// operator can preview the prompt and append last-minute additions.
//
// If opts.RawPrompt is non-empty, it is returned verbatim — RenderPrompt
// mirrors Dispatch's "RawPrompt wins" rule.
func RenderPrompt(opts Options) (string, error) {
	if opts.RawPrompt != "" {
		return opts.RawPrompt, nil
	}
	return renderPrompt(opts)
}

func (o Options) withDefaults() Options {
	if o.Stdout == nil {
		o.Stdout = os.Stdout
	}
	if o.Stderr == nil {
		o.Stderr = os.Stderr
	}
	if o.Stdin == nil {
		o.Stdin = os.Stdin
	}
	return o
}

// ---------------------------------------------------------------------------
// HEAD detection
// ---------------------------------------------------------------------------

func readHEAD(ctx context.Context, git GitRunner, dir string) (string, error) {
	out, err := git.Run(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func readCommitSubject(ctx context.Context, git GitRunner, dir, sha string) (string, error) {
	out, err := git.Run(ctx, dir, "log", "-1", "--format=%s", sha)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// ---------------------------------------------------------------------------
// Banners
// ---------------------------------------------------------------------------

const banner = "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

func writeEnterBanner(opts Options) {
	cyan := color.New(color.FgCyan, color.Bold)
	w := opts.Stdout
	fmt.Fprintln(w, cyan.Sprint(banner))
	mode := "interactive"
	if opts.Autonomous {
		mode = "autonomous"
	}
	fmt.Fprintln(w, cyan.Sprintf("🤖 ENTERING AGENT: %s  (%s)", opts.Agent, mode))
	if opts.IssueNum > 0 || opts.IssueTitle != "" {
		fmt.Fprintln(w, cyan.Sprintf("   Issue: #%d %q  Repo: %s",
			opts.IssueNum, opts.IssueTitle, opts.IssueRepo))
	}
	fmt.Fprintln(w, cyan.Sprint(banner))
}

func writeExitBanner(opts Options, sha, subject string, elapsed time.Duration, runErr error) {
	w := opts.Stdout
	if runErr != nil {
		red := color.New(color.FgRed, color.Bold)
		fmt.Fprintln(w, red.Sprint(banner))
		fmt.Fprintln(w, red.Sprintf("❌ AGENT FAILED: %s  (%s)", opts.Agent, elapsed.Round(elapsedRound)))
		fmt.Fprintln(w, red.Sprintf("   %s", runErr))
		fmt.Fprintln(w, red.Sprint(banner))
		return
	}
	green := color.New(color.FgGreen, color.Bold)
	fmt.Fprintln(w, green.Sprint(banner))
	fmt.Fprintln(w, green.Sprintf("✅ EXITED AGENT: committed %s  (%s)",
		shortSHA(sha), elapsed.Round(elapsedRound)))
	if subject != "" {
		fmt.Fprintln(w, green.Sprintf("   %q", subject))
	}
	fmt.Fprintln(w, green.Sprint(banner))
}

const elapsedRound = time.Second

// ---------------------------------------------------------------------------
// Real subprocess implementations
// ---------------------------------------------------------------------------

type execClaude struct{}

// Run invokes the `claude` CLI. Autonomous → `claude -p <prompt>`.
// Interactive → `claude <prompt>` with the caller's stdin/stdout/stderr
// connected directly so the operator sees the full Claude Code UI and
// can interject.
//
// `claude -p` prints incrementally; we connect stdout/stderr directly
// (not buffered) so output streams as the model produces it, matching the
// "interactive UX even in autonomous mode" goal from the design plan.
func (execClaude) Run(ctx context.Context, opts RunOpts) error {
	var args []string
	if opts.Autonomous {
		args = append(args, "-p", opts.Prompt)
	} else {
		// Interactive: pass the prompt as a positional argument so it
		// seeds the first user turn. Subsequent turns come from the TTY.
		args = append(args, opts.Prompt)
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	if err := cmd.Run(); err != nil {
		return err
	}
	return nil
}

type execGit struct{}

func (execGit) Run(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return out, fmt.Errorf("git %s: %w (stderr: %s)",
				strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
		}
		return out, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}
