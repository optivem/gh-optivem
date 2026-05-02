// Package clauderun shells out to the `claude` CLI to dispatch a named ATDD
// agent for the current phase, replacing v1's "pause and let the operator
// launch the agent in a second window" workflow.
//
// Dispatch reads the embedded per-agent prompt (see internal/atdd/runtime/
// agents/embed.go), substitutes ${name} placeholders against the live ticket
// context, invokes `claude` (interactive or `claude -p` autonomous), and
// detects success by diffing git HEAD before and after. The agent is
// expected to commit on the same branch — that commit landing is what the
// engine's downstream verify decorator keys off.
//
// v2 architectural note: there is no parent-claude harness or Task-tool
// indirection. The rendered prompt IS the agent's full one-shot input —
// `claude -p` runs the agent's instructions directly.
//
// The package exposes a ClaudeRunner / GitRunner pair so tests can inject
// canned exit codes and HEAD values; the production defaults exec the real
// CLIs the same way the gates / actions / classify / release packages do.
package clauderun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
)

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

	// Scope axes — copied from optivem.yaml's scope block via
	// driver.seedScopeParams. Empty values render as the broadest option
	// ("both" for Architecture, "all" for the language axes), matching the
	// convention used in the prompt templates' Scope section.
	Architecture string
	SystemLang   string
	TestLang     string

	// OverrideText is the per-node `--extra` text from override.Hooks,
	// interpolated into the prompt template. Empty string is fine.
	OverrideText string

	// RawPrompt, when non-empty, replaces the templated prompt entirely.
	// Used by override.Hooks Replace where the operator wants to swap the
	// whole prompt rather than append text via OverrideText. When set,
	// every other prompt-shaping field (NodeDescription, OverrideText, …)
	// is ignored.
	RawPrompt string

	// PromptOverride, when non-empty, replaces the embedded agent prompt
	// body (i.e. agents.Prompt(opts.Agent)) with this string. Unlike
	// RawPrompt, the override still goes through ${name} expansion against
	// the live ticket context and still has OverrideText appended. Used by
	// the driver's `--agent-prompt name=path` flag, where the operator
	// wants to swap the canonical prompt for one named agent without
	// touching the surrounding render machinery.
	PromptOverride string

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
//   - Subprocess exit status non-zero (stderr surfaced; a small classifier
//     turns known rate-limit / auth signatures into actionable messages
//     before falling through to the generic wrapper).
//   - Subprocess exit zero but HEAD unchanged (the agent ran clean but
//     produced no commit — same shape as v1's "abort" path).
//   - Subprocess exit zero but the agent switched branches mid-run (the
//     pre/post snapshot diff catches this so the operator gets a clear
//     "switched branches" message instead of a confusing "no commit").
//   - Any of the surrounding git / template steps failing.
//
// Pre/post snapshots also surface stranded untracked files (created by
// the agent but never `git add`ed) as a non-fatal warning after the
// success banner.
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

	preState, err := snapshotRepo(ctx, deps.Git, opts.RepoPath)
	if err != nil {
		return CommitInfo{}, fmt.Errorf("clauderun: snapshot before dispatch: %w", err)
	}

	writeEnterBanner(opts)
	startedAt := nowFn()

	// Tee stderr so we can classify rate-limit / auth failures after a
	// non-zero exit without losing the operator-visible stream. Bounded
	// to a reasonable cap to avoid pathological memory growth on a chatty
	// runner.
	var stderrCapture cappedBuffer
	stderrCapture.cap = 64 * 1024
	runStderr := io.MultiWriter(opts.Stderr, &stderrCapture)

	runResult, runErr := deps.Claude.Run(ctx, RunOpts{
		Prompt:     prompt,
		Autonomous: opts.Autonomous,
		Dir:        opts.RepoPath,
		Stdin:      opts.Stdin,
		Stdout:     opts.Stdout,
		Stderr:     runStderr,
	})
	if runErr != nil {
		writeExitBanner(opts, "", "", nowFn().Sub(startedAt), runResult.Usage, runErr)
		if classified := classifyRunError(stderrCapture.Bytes()); classified != nil {
			return CommitInfo{}, fmt.Errorf("clauderun: %s: %w", opts.Agent, classified)
		}
		return CommitInfo{}, fmt.Errorf("clauderun: %s exited non-zero: %w", opts.Agent, runErr)
	}

	postState, err := snapshotRepo(ctx, deps.Git, opts.RepoPath)
	if err != nil {
		writeExitBanner(opts, "", "", nowFn().Sub(startedAt), runResult.Usage, err)
		return CommitInfo{}, fmt.Errorf("clauderun: snapshot after dispatch: %w", err)
	}

	if postState.branch != preState.branch {
		switchErr := fmt.Errorf("agent switched branches mid-run (was %q, now %q) — original-branch HEAD unchanged at %s",
			preState.branch, postState.branch, shortSHA(preState.head))
		writeExitBanner(opts, "", "", nowFn().Sub(startedAt), runResult.Usage, switchErr)
		return CommitInfo{}, fmt.Errorf("clauderun: %s: %w", opts.Agent, switchErr)
	}

	if preState.head == postState.head {
		writeExitBanner(opts, "", "", nowFn().Sub(startedAt), runResult.Usage, errNoCommit)
		return CommitInfo{}, fmt.Errorf("clauderun: %s exited cleanly but produced no commit (HEAD unchanged at %s)",
			opts.Agent, shortSHA(preState.head))
	}

	subject, err := readCommitSubject(ctx, deps.Git, opts.RepoPath, postState.head)
	if err != nil {
		writeExitBanner(opts, postState.head, "", nowFn().Sub(startedAt), runResult.Usage, err)
		return CommitInfo{}, fmt.Errorf("clauderun: read commit subject for %s: %w", shortSHA(postState.head), err)
	}

	if newUntracked := diffUntracked(preState.untracked, postState.untracked); len(newUntracked) > 0 {
		writeUntrackedWarning(opts, newUntracked)
	}

	writeExitBanner(opts, postState.head, subject, nowFn().Sub(startedAt), runResult.Usage, nil)
	return CommitInfo{SHA: postState.head, Subject: subject}, nil
}

// errNoCommit is the sentinel for "subprocess succeeded but HEAD didn't
// move". Surfaced through writeExitBanner so the operator sees the failure
// banner rather than a silent return.
var errNoCommit = errors.New("no commit produced")

// nowFn is a package-level seam so tests can pin elapsed time in banner
// output. Production points at time.Now.
var nowFn = time.Now

// renderPrompt reads the embedded prompt for opts.Agent (or opts.PromptOverride
// when non-empty), expands ${name} placeholders against the ticket context,
// and appends opts.OverrideText (if any) as a trailing block. Public-ish for
// the test file; not exported.
func renderPrompt(opts Options) (string, error) {
	var body string
	if opts.PromptOverride != "" {
		body = opts.PromptOverride
	} else {
		var err error
		body, err = agents.Prompt(opts.Agent)
		if err != nil {
			return "", err
		}
	}
	params := map[string]string{
		"issue_num":     strconv.Itoa(opts.IssueNum),
		"issue_title":   opts.IssueTitle,
		"issue_repo":    opts.IssueRepo,
		"project_title": opts.ProjectTitle,
		"project_url":   opts.ProjectURL,
		"phase":         opts.NodeDescription,
		"phase_doc":     opts.PhaseDoc,
		"architecture":  scopeOrDefault(opts.Architecture, "both"),
		"system_lang":   scopeOrDefault(opts.SystemLang, "all"),
		"test_lang":     scopeOrDefault(opts.TestLang, "all"),
	}
	rendered := statemachine.ExpandParams(body, params)
	if opts.OverrideText != "" {
		rendered = strings.TrimRight(rendered, "\n") + "\n\n" + opts.OverrideText + "\n"
	}
	return rendered, nil
}

// scopeOrDefault returns fallback when value is empty, else value. The
// prompt-template Scope block uses "both" / "all" as the broadest options;
// an unset axis in optivem.yaml is the same intent.
func scopeOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// RenderPrompt is the public counterpart to renderPrompt: it returns the
// prompt string Dispatch would hand to the subprocess, without invoking
// it. The driver's agent dispatcher uses this for the --interactive
// prompt-review hook so the operator can preview the prompt and append
// last-minute additions.
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
// Repo snapshot (pre/post) and HEAD detection
// ---------------------------------------------------------------------------

// repoState is the "before"/"after" snapshot Dispatch takes around the
// runner call. The diff catches two failure modes:
//   - Branch-switch: the agent runs `git checkout -b feature/foo`,
//     commits there, and never returns. HEAD on the original branch is
//     unchanged → without snapshot we'd halt with the misleading "no
//     commit produced"; with snapshot we say "switched branches".
//   - Stranded untracked files: the agent created files but never
//     `git add`ed them. The commit lands fine but the new files sit
//     outside it (silent data-loss class). Snapshot diff surfaces them
//     as a non-fatal warning.
type repoState struct {
	head      string
	branch    string
	untracked map[string]bool
}

func snapshotRepo(ctx context.Context, git GitRunner, dir string) (repoState, error) {
	headOut, err := git.Run(ctx, dir, "rev-parse", "HEAD")
	if err != nil {
		return repoState{}, fmt.Errorf("rev-parse HEAD: %w", err)
	}
	branchOut, err := git.Run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return repoState{}, fmt.Errorf("rev-parse --abbrev-ref HEAD: %w", err)
	}
	statusOut, err := git.Run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return repoState{}, fmt.Errorf("status --porcelain: %w", err)
	}
	return repoState{
		head:      strings.TrimSpace(string(headOut)),
		branch:    strings.TrimSpace(string(branchOut)),
		untracked: parseUntracked(statusOut),
	}, nil
}

// parseUntracked picks out the `??<space><path>` rows from `git status
// --porcelain` output. Other status codes (modified, staged, etc.) are
// ignored — only untracked files are the silent-data-loss class we
// care about for the post-dispatch warning.
func parseUntracked(porcelain []byte) map[string]bool {
	m := map[string]bool{}
	for line := range strings.SplitSeq(string(porcelain), "\n") {
		if len(line) >= 4 && line[0] == '?' && line[1] == '?' {
			path := strings.TrimSpace(line[3:])
			if path != "" {
				m[path] = true
			}
		}
	}
	return m
}

// diffUntracked returns paths present in post but not pre, sorted for
// stable banner output.
func diffUntracked(pre, post map[string]bool) []string {
	var out []string
	for p := range post {
		if !pre[p] {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
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
// Stderr classification (rate limit / auth)
// ---------------------------------------------------------------------------

// rateLimitSignatures are case-insensitive substrings that mean "claude
// refused to dispatch because of a billing / rate limit". First match
// wins. Patterns are deliberately broad — false positives here only
// change the wording of an already-failing run.
var rateLimitSignatures = []string{
	"rate limit",
	"rate_limit_error",
	"weekly limit",
	"5-hour limit",
	"usage limit",
	"quota exceeded",
	"too many requests",
}

// authSignatures are case-insensitive substrings that mean "claude
// refused because credentials are missing or invalid". The pre-flight
// check at driver startup catches this before the first dispatch in
// the happy path; this branch covers credentials expiring mid-run.
var authSignatures = []string{
	"not authenticated",
	"auth required",
	"authentication required",
	"invalid api key",
	"please run /login",
	"please log in",
	"please login",
}

// classifyRunError inspects the captured stderr from a non-zero claude
// exit and returns a more actionable error when a known signature
// matches. Returns nil meaning "fall through to the generic wrapper".
func classifyRunError(stderr []byte) error {
	tail := lastLines(stderr, 20)
	lower := strings.ToLower(string(tail))
	for _, sig := range rateLimitSignatures {
		if strings.Contains(lower, sig) {
			return errors.New("rate limit hit on Claude subscription; weekly cap likely exhausted — re-run after the next reset window or upgrade your plan")
		}
	}
	for _, sig := range authSignatures {
		if strings.Contains(lower, sig) {
			return errors.New("claude CLI is not authenticated — run `claude /login` (credentials live in ~/.claude/) before re-dispatching")
		}
	}
	return nil
}

// lastLines returns the trailing n lines of b. Used to bound the
// classifier's scan to the most recent error output — if the runner
// printed a wall of progress text before failing, we want to look at
// the failure tail, not the noise above it.
func lastLines(b []byte, n int) []byte {
	if n <= 0 || len(b) == 0 {
		return b
	}
	count := 0
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] == '\n' {
			count++
			if count > n {
				return b[i+1:]
			}
		}
	}
	return b
}

// cappedBuffer is a write-only buffer that drops bytes past `cap`.
// Used to capture stderr for classification without unbounded memory
// growth on a runner that streams a lot of output before failing.
type cappedBuffer struct {
	buf bytes.Buffer
	cap int
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	if c.cap > 0 && c.buf.Len() >= c.cap {
		return len(p), nil
	}
	if c.cap > 0 && c.buf.Len()+len(p) > c.cap {
		p = p[:c.cap-c.buf.Len()]
	}
	return c.buf.Write(p)
}

func (c *cappedBuffer) Bytes() []byte { return c.buf.Bytes() }

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

// writeUntrackedWarning surfaces files the agent created but never
// `git add`ed. Emitted after writeExitBanner on the success path so
// the operator sees the commit SHA first, then the warning. Non-fatal:
// the operator may have intended to leave them (e.g. ad-hoc scratch
// files) — but the typical case is they meant to commit them and the
// stranded files are silent data loss.
func writeUntrackedWarning(opts Options, paths []string) {
	yellow := color.New(color.FgYellow, color.Bold)
	w := opts.Stdout
	fmt.Fprintln(w, yellow.Sprintf("⚠  %s left %d untracked file(s) outside the commit:", opts.Agent, len(paths)))
	for _, p := range paths {
		fmt.Fprintln(w, yellow.Sprintf("    %s", p))
	}
}

const elapsedRound = time.Second

// ---------------------------------------------------------------------------
// Real subprocess implementations
// ---------------------------------------------------------------------------

// promptArgvLimit is the threshold above which materializePrompt spills
// the prompt to a tempfile instead of passing it as an argv argument.
// Windows' CreateProcess caps the full command line at ~32K chars; macOS
// and Linux ARG_MAX are higher (~256K and ~131K) but the same overflow
// is reachable as prompts grow. 8K leaves comfortable headroom under the
// strictest OS limit, including the executable path and any quoting the
// shell adds.
const promptArgvLimit = 8000

// materializePrompt returns the argv argument to hand to `claude` and a
// cleanup func. For prompts under promptArgvLimit it returns the prompt
// verbatim with a no-op cleanup — the historical fast path. Above the
// limit it writes the prompt to a tempfile in dir and returns a short
// bootstrap message instructing the agent to read and delete the file
// (the only viable path on Windows, where the OS argv limit is too low
// for large prompts and the `claude` CLI exposes no --prompt-file flag).
//
// The cleanup func is always safe to call. It removes the tempfile if
// one was created — defensive against the agent forgetting to delete it
// itself, or the run failing before reaching the deletion instruction.
func materializePrompt(dir, prompt string) (string, func(), error) {
	if len(prompt) <= promptArgvLimit {
		return prompt, func() {}, nil
	}
	if dir == "" {
		var err error
		dir, err = os.Getwd()
		if err != nil {
			return "", func() {}, fmt.Errorf("materializePrompt: getwd: %w", err)
		}
	}
	f, err := os.CreateTemp(dir, ".atdd-prompt-*.tmp.md")
	if err != nil {
		return "", func() {}, fmt.Errorf("materializePrompt: create tempfile: %w", err)
	}
	if _, err := f.WriteString(prompt); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", func() {}, fmt.Errorf("materializePrompt: write tempfile: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(f.Name())
		return "", func() {}, fmt.Errorf("materializePrompt: close tempfile: %w", err)
	}
	base := filepath.Base(f.Name())
	bootstrap := fmt.Sprintf(
		"Your full instructions are in `%s` in the working directory. As your very first action, read that file with the Read tool and carry out the instructions exactly. Delete `%s` when you finish.",
		base, base,
	)
	cleanup := func() { os.Remove(f.Name()) }
	return bootstrap, cleanup, nil
}

type execClaude struct{}

// Run invokes the `claude` CLI.
//
// Interactive mode → `claude <prompt>` with stdin/stdout/stderr connected
// directly so the operator sees the full Claude Code UI and can interject.
//
// Autonomous mode → `claude -p <prompt> --output-format json`:
//
//   - The prompt is the embedded agent's full instructions (rendered by
//     renderPrompt). v2 has no host/subagent split — `claude -p` IS the
//     agent and needs the default tool set (Read/Glob/Grep/Edit/Write/Bash)
//     to do real work.
//   - --output-format json buffers the run into a single JSON envelope
//     containing `total_cost_usd` and `usage.{input,output,cache_*}_tokens`
//     so we can surface cost/throughput in the exit banner. The trade-off
//     is no streaming output during the run.
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
	arg, cleanup, err := materializePrompt(opts.Dir, opts.Prompt)
	if err != nil {
		return RunResult{}, err
	}
	defer cleanup()
	cmd := exec.CommandContext(ctx, "claude", arg)
	if opts.Dir != "" {
		cmd.Dir = opts.Dir
	}
	cmd.Stdin = opts.Stdin
	cmd.Stdout = opts.Stdout
	cmd.Stderr = opts.Stderr
	return RunResult{}, cmd.Run()
}

func runAutonomous(ctx context.Context, opts RunOpts) (RunResult, error) {
	arg, cleanup, err := materializePrompt(opts.Dir, opts.Prompt)
	if err != nil {
		return RunResult{}, err
	}
	defer cleanup()
	args := []string{
		"-p", arg,
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
