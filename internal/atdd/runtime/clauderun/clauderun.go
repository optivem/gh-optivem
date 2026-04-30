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
	"encoding/json"
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
	Run(ctx context.Context, opts RunOpts) (RunResult, error)
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

// RunResult is what the runner reports back to Dispatch. Usage is best-effort
// — populated only when the runner can parse a structured envelope (currently
// autonomous mode via `claude -p --output-format json`). Interactive mode
// leaves it nil and the banner falls back to elapsed-time-only.
type RunResult struct {
	Usage *TokenUsage
}

// TokenUsage is the cost/throughput summary surfaced in the exit banner.
// Field names mirror the `claude -p --output-format json` envelope so the
// JSON shape can be decoded directly into this struct.
type TokenUsage struct {
	InputTokens              int     `json:"input_tokens"`
	OutputTokens             int     `json:"output_tokens"`
	CacheCreationInputTokens int     `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int     `json:"cache_read_input_tokens"`
	TotalCostUSD             float64 `json:"-"`
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

	runResult, runErr := deps.Claude.Run(ctx, RunOpts{
		Prompt:     prompt,
		Autonomous: opts.Autonomous,
		Dir:        opts.RepoPath,
		Stdin:      opts.Stdin,
		Stdout:     opts.Stdout,
		Stderr:     opts.Stderr,
	})
	if runErr != nil {
		writeExitBanner(opts, "", "", nowFn().Sub(startedAt), runResult.Usage, runErr)
		return CommitInfo{}, fmt.Errorf("clauderun: %s exited non-zero: %w", opts.Agent, runErr)
	}

	headAfter, err := readHEAD(ctx, deps.Git, opts.RepoPath)
	if err != nil {
		writeExitBanner(opts, "", "", nowFn().Sub(startedAt), runResult.Usage, err)
		return CommitInfo{}, fmt.Errorf("clauderun: read HEAD after dispatch: %w", err)
	}

	if headBefore == headAfter {
		writeExitBanner(opts, "", "", nowFn().Sub(startedAt), runResult.Usage, errNoCommit)
		return CommitInfo{}, fmt.Errorf("clauderun: %s exited cleanly but produced no commit (HEAD unchanged at %s)",
			opts.Agent, shortSHA(headBefore))
	}

	subject, err := readCommitSubject(ctx, deps.Git, opts.RepoPath, headAfter)
	if err != nil {
		writeExitBanner(opts, headAfter, "", nowFn().Sub(startedAt), runResult.Usage, err)
		return CommitInfo{}, fmt.Errorf("clauderun: read commit subject for %s: %w", shortSHA(headAfter), err)
	}

	writeExitBanner(opts, headAfter, subject, nowFn().Sub(startedAt), runResult.Usage, nil)
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

func writeExitBanner(opts Options, sha, subject string, elapsed time.Duration, usage *TokenUsage, runErr error) {
	w := opts.Stdout
	if runErr != nil {
		red := color.New(color.FgRed, color.Bold)
		fmt.Fprintln(w, red.Sprint(banner))
		fmt.Fprintln(w, red.Sprintf("❌ AGENT FAILED: %s  (%s%s)", opts.Agent, elapsed.Round(elapsedRound), formatUsageSuffix(usage)))
		fmt.Fprintln(w, red.Sprintf("   %s", runErr))
		fmt.Fprintln(w, red.Sprint(banner))
		return
	}
	green := color.New(color.FgGreen, color.Bold)
	fmt.Fprintln(w, green.Sprint(banner))
	fmt.Fprintln(w, green.Sprintf("✅ EXITED AGENT: committed %s  (%s%s)",
		shortSHA(sha), elapsed.Round(elapsedRound), formatUsageSuffix(usage)))
	if subject != "" {
		fmt.Fprintln(w, green.Sprintf("   %q", subject))
	}
	fmt.Fprintln(w, green.Sprint(banner))
}

// formatUsageSuffix renders ", 12.4k in / 1.8k out, $0.18" if usage is non-nil
// and non-empty. Returns "" otherwise so the banner gracefully degrades to
// elapsed-time-only when the runner couldn't extract a JSON envelope (e.g.
// interactive mode, or an autonomous-mode parse failure).
func formatUsageSuffix(usage *TokenUsage) string {
	if usage == nil {
		return ""
	}
	in := usage.InputTokens + usage.CacheCreationInputTokens + usage.CacheReadInputTokens
	out := usage.OutputTokens
	if in == 0 && out == 0 && usage.TotalCostUSD == 0 {
		return ""
	}
	return fmt.Sprintf(", %s in / %s out, $%.2f", formatTokens(in), formatTokens(out), usage.TotalCostUSD)
}

func formatTokens(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

const elapsedRound = time.Second

// ---------------------------------------------------------------------------
// Real subprocess implementations
// ---------------------------------------------------------------------------

type execClaude struct{}

// Run invokes the `claude` CLI.
//
// Interactive mode → `claude <prompt>` with stdin/stdout/stderr connected
// directly so the operator sees the full Claude Code UI and can interject.
//
// Autonomous mode → `claude -p <prompt> --allowed-tools Task --output-format json`:
//
//   - --allowed-tools Task narrows the host's tool surface to the subagent
//     dispatch primitive only. The host has no legitimate reason to call
//     git / gh / Read / Grep before dispatching — those tools belong to the
//     subagent (which runs in isolated context). Restricting up-front
//     forces the dispatch and saves the tokens of speculative pre-work.
//   - --output-format json buffers the run into a single JSON envelope
//     containing `total_cost_usd` and `usage.{input,output,cache_*}_tokens`
//     so we can surface cost/throughput in the exit banner. The trade-off
//     is no streaming output during the run; acceptable here because items
//     1+2 leave the host with nothing to say (it dispatches and exits).
//
// JSON parsing is best-effort — a future CLI version that changes the
// envelope shape leaves Usage nil and the banner falls back gracefully.
func (execClaude) Run(ctx context.Context, opts RunOpts) (RunResult, error) {
	if opts.Autonomous {
		return runAutonomous(ctx, opts)
	}
	return runInteractive(ctx, opts)
}

func runInteractive(ctx context.Context, opts RunOpts) (RunResult, error) {
	// Interactive: pass the prompt as a positional argument so it seeds
	// the first user turn. Subsequent turns come from the TTY.
	cmd := exec.CommandContext(ctx, "claude", opts.Prompt)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	return RunResult{}, cmd.Run()
}

func runAutonomous(ctx context.Context, opts RunOpts) (RunResult, error) {
	args := []string{
		"-p", opts.Prompt,
		"--allowed-tools", "Task",
		"--output-format", "json",
	}
	cmd := exec.CommandContext(ctx, "claude", args...)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Stdin = opts.Stdin

	// Capture stdout for JSON parsing. The buffered envelope is dumped to
	// opts.Stdout after the run so the operator still gets the host's
	// final result text, just not streaming.
	var captured bytes.Buffer
	cmd.Stdout = &captured
	cmd.Stderr = opts.Stderr

	runErr := cmd.Run()

	usage, resultText := parseClaudeJSON(captured.Bytes())
	if resultText != "" {
		fmt.Fprintln(opts.Stdout, resultText)
	} else if runErr != nil && captured.Len() > 0 {
		// Run failed before the JSON envelope landed — surface the raw
		// bytes so the operator sees whatever claude did print.
		opts.Stdout.Write(captured.Bytes())
	}
	return RunResult{Usage: usage}, runErr
}

// parseClaudeJSON decodes the `claude -p --output-format json` envelope.
// Returns (nil, "") when the bytes don't decode — callers treat that as
// "no usage data, fall back to elapsed-time-only banner".
func parseClaudeJSON(b []byte) (*TokenUsage, string) {
	if len(bytes.TrimSpace(b)) == 0 {
		return nil, ""
	}
	var env struct {
		Result       string     `json:"result"`
		TotalCostUSD float64    `json:"total_cost_usd"`
		Usage        TokenUsage `json:"usage"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return nil, ""
	}
	usage := env.Usage
	usage.TotalCostUSD = env.TotalCostUSD
	return &usage, env.Result
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
