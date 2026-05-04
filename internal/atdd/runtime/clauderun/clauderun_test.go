// Tests for clauderun.Dispatch.
//
// Strategy: drive Dispatch through fakeClaude / fakeGit so the suite is
// hermetic — no real `claude` or `git` invocations. Each fake captures
// the args / Run call it received and emits canned values, letting us
// assert prompt construction, commit-detection branches, and error paths.
package clauderun

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeClaude records the RunOpts it was called with and returns a canned
// error. headFn (when set) is called inside Run so the test can simulate
// the agent producing a commit during the subprocess — by mutating
// fakeGit.heads in lock-step with the call sequence. stderr (when set)
// is written to opts.Stderr before returning, used by the rate-limit /
// auth classification tests.
type fakeClaude struct {
	calls  []RunOpts
	err    error
	usage  *TokenUsage
	headFn func()
	stderr []byte
}

func (f *fakeClaude) Run(_ context.Context, opts RunOpts) (RunResult, error) {
	f.calls = append(f.calls, opts)
	if f.headFn != nil {
		f.headFn()
	}
	if len(f.stderr) > 0 && opts.Stderr != nil {
		opts.Stderr.Write(f.stderr)
	}
	return RunResult{Usage: f.usage}, f.err
}

// fakeGit serves canned outputs. The HEAD-rev-parse and log calls
// consume the `out` FIFO (existing tests rely on this). Snapshot calls
// (rev-parse --abbrev-ref HEAD, status --porcelain) get sensible
// defaults so tests that don't care about item 2's branch-switch /
// untracked detection don't have to enumerate them. Tests that DO care
// can override via branchPre/branchPost (FIFO, used per call) and
// statusPre/statusPost.
type fakeGit struct {
	out        [][]byte
	err        error
	args       [][]string
	branchPre  string
	branchPost string
	statusPre  []byte
	statusPost []byte

	abbrevCount int
	statusCount int

	// hooksDir is what fakeGit returns for `rev-parse --git-path hooks`
	// — the call installPreCommitHook makes when CLICommits is on. Tests
	// that exercise the install set this to a writable temp dir; tests
	// in legacy mode never trigger the call so leaving it empty is fine.
	hooksDir string

	// gitDir is what fakeGit returns for `rev-parse --absolute-git-dir`
	// — installPreCommitHook drops the per-worktree dispatch marker
	// here. Tests set it to a writable temp dir.
	gitDir string

	// commitEnv captures os.Getenv("CLAUDERUN_CLI_COMMIT") at the moment
	// args[0] == "commit" — lets the hook-env test assert the var is set
	// during the commit call without leaking process state.
	commitEnv    string
	commitEnvSet bool
}

func (f *fakeGit) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	f.args = append(f.args, args)
	if len(args) > 0 && args[0] == "commit" {
		f.commitEnv, f.commitEnvSet = os.LookupEnv("CLAUDERUN_CLI_COMMIT")
	}
	if f.err != nil {
		return nil, f.err
	}
	if len(args) >= 3 && args[0] == "rev-parse" && args[1] == "--git-path" && args[2] == "hooks" {
		return []byte(f.hooksDir + "\n"), nil
	}
	if len(args) >= 2 && args[0] == "rev-parse" && args[1] == "--absolute-git-dir" {
		return []byte(f.gitDir + "\n"), nil
	}
	if len(args) >= 3 && args[0] == "rev-parse" && args[1] == "--abbrev-ref" {
		f.abbrevCount++
		v := f.branchPre
		if f.abbrevCount > 1 {
			v = f.branchPost
		}
		if v == "" {
			v = "main"
		}
		return []byte(v + "\n"), nil
	}
	if len(args) >= 2 && args[0] == "status" && args[1] == "--porcelain" {
		f.statusCount++
		v := f.statusPre
		if f.statusCount > 1 {
			v = f.statusPost
		}
		return v, nil
	}
	if len(f.out) == 0 {
		return nil, errors.New("fakeGit: no canned output left")
	}
	v := f.out[0]
	f.out = f.out[1:]
	return v, nil
}

// hasGitArg reports whether any recorded git call started with the given
// argument prefix. Used by tests that previously asserted on call counts
// — the snapshot calls have shifted those counts, so we now assert on
// the substantive calls (`log`, `rev-parse HEAD`) instead.
func (f *fakeGit) hasGitArg(prefix ...string) bool {
	for _, args := range f.args {
		if len(args) < len(prefix) {
			continue
		}
		match := true
		for i, p := range prefix {
			if args[i] != p {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func newOpts() Options {
	return Options{
		Agent:           "atdd-test",
		PhaseDoc:        "docs/atdd/process/at-red-test.md",
		NodeDescription: "Write the AT-RED scenario",
		IssueNum:        42,
		IssueTitle:      "Add PUT /carts/{id}/items endpoint",
		IssueRepo:       "optivem/shop",
		ProjectTitle:    "Shop ATDD",
		ProjectURL:      "https://github.com/orgs/optivem/projects/1",
		// Discard banners so test output stays clean.
		Stdout: io.Discard,
		Stderr: io.Discard,
		Stdin:  strings.NewReader(""),
	}
}

// ---------------------------------------------------------------------------
// Prompt construction
// ---------------------------------------------------------------------------

func TestRenderPrompt_IncludesAllFields(t *testing.T) {
	// Default opts has CLICommits=false (legacy mode), so the rendered
	// prompt swaps the new "do not commit" preamble back to the pre-rollout
	// "your COMMIT must land on HEAD" sentence.
	opts := newOpts()
	opts.OverrideText = "prefer record types"

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}

	// v2: prompt is the embedded agent body with ${name} placeholders
	// substituted; the parent-claude "Launch the X subagent" wrapper is gone.
	mustContain(t, got, "You are the Test Agent")
	mustContain(t, got, `#42 "Add PUT /carts/{id}/items endpoint"`)
	mustContain(t, got, "(optivem/shop)")
	mustContain(t, got, "Shop ATDD (https://github.com/orgs/optivem/projects/1)")
	mustContain(t, got, "Phase: Write the AT-RED scenario")
	mustContain(t, got, "Phase doc: docs/atdd/process/at-red-test.md")
	mustContain(t, got, "prefer record types")
	mustContain(t, got, "your COMMIT must land on HEAD")
	// All ${…} placeholders must be expanded — none should leak through.
	if strings.Contains(got, "${") {
		t.Errorf("prompt still contains ${...} placeholder")
	}
}

func TestRenderPrompt_CLICommits_StripsCommitGating(t *testing.T) {
	// With --cli-commits on, the agent must not be told to commit. The
	// preamble flips to the "do not commit" sentence and the embedded
	// shared-commit-confirmation reference block is gone — neither the
	// rule heading nor the legacy marker should leak into the output.
	opts := newOpts()
	opts.CLICommits = true

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}

	mustContain(t, got, "do not commit and do not summarise")
	mustContain(t, got, "The CLI will stage and commit your changes after you exit")
	if strings.Contains(got, "your COMMIT must land on HEAD") {
		t.Errorf("CLI-commits prompt should not tell the agent to land a commit on HEAD")
	}
	if strings.Contains(got, "# Commit Confirmation Rule") {
		t.Errorf("CLI-commits prompt should not embed the legacy commit-confirmation rule block")
	}
	if strings.Contains(got, "<!-- legacy-block:") {
		t.Errorf("legacy-block marker leaked into rendered prompt: %q", got)
	}
}

func TestRenderPrompt_LegacyMode_RestoresCommitConfirmationBlock(t *testing.T) {
	// CLICommits=false (default) is the rehearsal-compatible mode: the
	// reverse-substitution must put the legacy preamble back AND inject
	// the shared-commit-confirmation reference block where the marker sits.
	opts := newOpts()
	opts.Agent = "atdd-task"

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}

	mustContain(t, got, "your COMMIT must land on HEAD")
	mustContain(t, got, "# Commit Confirmation Rule")
	mustContain(t, got, `### Reference: docs/atdd/process/shared-commit-confirmation.md`)
	if strings.Contains(got, "<!-- legacy-block:") {
		t.Errorf("legacy-block marker leaked into rendered prompt: %q", got)
	}
}

func TestRenderPrompt_CLICommits_NoMarkerLeaksAcrossAgents(t *testing.T) {
	// Smoke-test every embedded prompt: with --cli-commits on, none of
	// them should leak the legacy-block marker or the legacy preamble.
	for _, name := range []string{
		"atdd-backend", "atdd-bug", "atdd-chore", "atdd-driver", "atdd-dsl",
		"atdd-frontend", "atdd-release", "atdd-story", "atdd-stubs",
		"atdd-task", "atdd-test",
	} {
		opts := newOpts()
		opts.Agent = name
		opts.CLICommits = true

		got, err := renderPrompt(opts)
		if err != nil {
			t.Fatalf("%s: renderPrompt: %v", name, err)
		}
		if strings.Contains(got, "<!-- legacy-block:") {
			t.Errorf("%s: legacy-block marker leaked", name)
		}
		if strings.Contains(got, "your COMMIT must land on HEAD") {
			t.Errorf("%s: legacy preamble leaked", name)
		}
	}
}

func TestRenderPrompt_TaskAgentScopeBlock_ExplicitValues(t *testing.T) {
	opts := newOpts()
	opts.Agent = "atdd-task"
	opts.Architecture = "monolith"
	opts.SystemLang = "java"
	opts.TestLang = "dotnet"

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	mustContain(t, got, "Scope: Architecture=monolith, System Lang=java, Test Lang=dotnet")
	if strings.Contains(got, "${") {
		t.Errorf("prompt still contains ${...} placeholder: %s", got)
	}
}

func TestRenderPrompt_TaskAgentScopeBlock_EmptyDefaultsToBroadest(t *testing.T) {
	// When gh-optivem.yaml omits the scope block, all three axes arrive empty.
	// They must render as the broadest option ("both" / "all") so the agent
	// reads a complete Scope line, not "Architecture=, System Lang=, …".
	opts := newOpts()
	opts.Agent = "atdd-chore"

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	mustContain(t, got, "Scope: Architecture=both, System Lang=all, Test Lang=all")
}

func TestRenderPrompt_ReturnsErrorForUnknownAgent(t *testing.T) {
	// agents.Prompt errors when no embedded prompt matches the name; the
	// driver relies on this so a YAML referencing an unembedded agent
	// fails loudly at dispatch time.
	opts := newOpts()
	opts.Agent = "atdd-doesnotexist"
	if _, err := renderPrompt(opts); err == nil {
		t.Fatalf("expected error for unknown agent, got nil")
	}
}

// ---------------------------------------------------------------------------
// Dispatch — happy path
// ---------------------------------------------------------------------------

func TestDispatch_SuccessReturnsCommitInfo(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1111111\n"), // rev-parse before
			[]byte("bbbbbbb2222222\n"), // rev-parse after
			[]byte("AT-RED-TEST: scenario for PUT /carts/{id}/items\n"), // log subject
		},
	}
	claudeFake := &fakeClaude{}

	got, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, newOpts())
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got.SHA != "bbbbbbb2222222" {
		t.Errorf("SHA: got %q, want %q", got.SHA, "bbbbbbb2222222")
	}
	if got.Subject != "AT-RED-TEST: scenario for PUT /carts/{id}/items" {
		t.Errorf("Subject: got %q", got.Subject)
	}
	if len(claudeFake.calls) != 1 {
		t.Fatalf("expected 1 claude call, got %d", len(claudeFake.calls))
	}
	// Prompt is constructed and passed through to the runner.
	if !strings.Contains(claudeFake.calls[0].Prompt, "You are the Test Agent") {
		t.Errorf("prompt missing agent identity line")
	}
}

func TestDispatch_AutonomousFlagPropagates(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"),
			[]byte("bbbb\n"),
			[]byte("subject\n"),
		},
	}
	claudeFake := &fakeClaude{}

	opts := newOpts()
	opts.Autonomous = true

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	got := claudeFake.calls[0]
	if !got.Autonomous {
		t.Errorf("Autonomous: got false, want true")
	}
}

// ---------------------------------------------------------------------------
// Dispatch — failure paths
// ---------------------------------------------------------------------------

func TestDispatch_FailsWhenSubprocessExitsNonZero(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"), // only the pre-snapshot rev-parse HEAD lands
		},
	}
	claudeFake := &fakeClaude{err: errors.New("exit status 1")}

	_, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, newOpts())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "exited non-zero") {
		t.Errorf("error wording: got %q", err.Error())
	}
	// On the non-zero-exit path neither the post-snapshot nor the log
	// call should run — surfacing stderr is the only useful action.
	if gitFake.hasGitArg("log") {
		t.Errorf("git log must not run on subprocess failure: %v", gitFake.args)
	}
	if gitFake.statusCount != 1 {
		t.Errorf("expected exactly 1 status --porcelain (pre-snapshot only), got %d", gitFake.statusCount)
	}
}

func TestDispatch_FailsWhenHEADUnchanged(t *testing.T) {
	// Same HEAD before and after → "subprocess succeeded but produced no
	// commit". Both pre and post snapshots run (so we can compare), but
	// no `git log` since there's no new SHA.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("samesha\n"),
			[]byte("samesha\n"),
		},
	}
	claudeFake := &fakeClaude{}

	_, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, newOpts())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no commit") {
		t.Errorf("error wording: got %q", err.Error())
	}
	if gitFake.hasGitArg("log") {
		t.Errorf("git log must not run when HEAD is unchanged: %v", gitFake.args)
	}
}

func TestDispatch_PropagatesGitFailureBeforeRun(t *testing.T) {
	gitFake := &fakeGit{err: errors.New("not a git repo")}
	claudeFake := &fakeClaude{}

	_, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, newOpts())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "snapshot before dispatch") {
		t.Errorf("error wording: got %q", err.Error())
	}
	if len(claudeFake.calls) != 0 {
		t.Errorf("claude must not run when pre-flight git fails, got %d calls", len(claudeFake.calls))
	}
}

// ---------------------------------------------------------------------------
// Banner output (smoke check — we do not lock in exact ANSI bytes)
// ---------------------------------------------------------------------------

func TestDispatch_WritesEnterAndExitBanners(t *testing.T) {
	var buf bytes.Buffer
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"),
			[]byte("bbbb\n"),
			[]byte("subject\n"),
		},
	}
	opts := newOpts()
	opts.Stdout = &buf

	if _, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	got := buf.String()
	mustContain(t, got, "ENTERING AGENT")
	mustContain(t, got, "atdd-test")
	mustContain(t, got, "EXITED AGENT")
	mustContain(t, got, "bbbb") // short SHA prefix
}

// ---------------------------------------------------------------------------
// Token usage parsing & banner formatting (item 6)
// ---------------------------------------------------------------------------

func TestParseClaudeJSON_ExtractsUsageAndResultText(t *testing.T) {
	// Verbatim shape from a real `claude -p --output-format json` invocation —
	// captured 2026-04-30 against claude CLI. Trimmed to the fields we read.
	envelope := `{"type":"result","subtype":"success","is_error":false,"result":"Hi there friend","total_cost_usd":0.17759875,"usage":{"input_tokens":6,"cache_creation_input_tokens":28307,"cache_read_input_tokens":0,"output_tokens":10}}`

	usage, result := parseClaudeJSON([]byte(envelope))
	if usage == nil {
		t.Fatalf("expected non-nil usage")
	}
	if usage.InputTokens != 6 || usage.OutputTokens != 10 || usage.CacheCreationInputTokens != 28307 {
		t.Errorf("usage fields wrong: %+v", *usage)
	}
	if usage.TotalCostUSD != 0.17759875 {
		t.Errorf("cost: got %v, want 0.17759875", usage.TotalCostUSD)
	}
	if result != "Hi there friend" {
		t.Errorf("result: got %q", result)
	}
}

func TestParseClaudeJSON_GracefulOnMalformed(t *testing.T) {
	// Non-JSON output (e.g. a CLI error message printed before any envelope)
	// must not panic and must not produce phantom usage.
	usage, result := parseClaudeJSON([]byte("claude: command failed\n"))
	if usage != nil || result != "" {
		t.Errorf("expected (nil, \"\") on malformed input, got (%+v, %q)", usage, result)
	}

	// Empty input — same expectation.
	usage, result = parseClaudeJSON(nil)
	if usage != nil || result != "" {
		t.Errorf("expected (nil, \"\") on empty input, got (%+v, %q)", usage, result)
	}
}

func TestExitBanner_IncludesUsageWhenPresent(t *testing.T) {
	var buf bytes.Buffer
	opts := newOpts()
	opts.Stdout = &buf

	usage := &TokenUsage{
		InputTokens:              6,
		OutputTokens:             1800,
		CacheCreationInputTokens: 10000,
		CacheReadInputTokens:     2400,
		TotalCostUSD:             0.18,
	}
	writeExitBanner(opts, "abc1234567", "atdd: red phase", 47*time.Second, usage, nil)

	got := buf.String()
	mustContain(t, got, "EXITED AGENT: committed abc1234")
	mustContain(t, got, "47s")
	mustContain(t, got, "12.4k in") // 6 + 10000 + 2400 = 12406 → 12.4k
	mustContain(t, got, "1.8k out")
	mustContain(t, got, "$0.18")
}

func TestExitBanner_OmitsUsageSuffixWhenNil(t *testing.T) {
	// Interactive mode has no JSON envelope — usage is nil and the banner
	// must degrade to elapsed-time-only without the trailing token line.
	var buf bytes.Buffer
	opts := newOpts()
	opts.Stdout = &buf
	writeExitBanner(opts, "abc1234", "subj", 5*time.Second, nil, nil)

	got := buf.String()
	mustContain(t, got, "EXITED AGENT")
	if strings.Contains(got, "$") || strings.Contains(got, " in ") {
		t.Errorf("expected no token suffix when usage is nil, got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Stderr classification (item 1: rate limit / auth)
// ---------------------------------------------------------------------------

func TestClassifyRunError(t *testing.T) {
	tests := []struct {
		name     string
		stderr   string
		wantSub  string // "" → expect nil
		wantNone bool
	}{
		{name: "empty", stderr: "", wantNone: true},
		{name: "unrelated", stderr: "panic: something else", wantNone: true},
		{name: "rate_limit_error", stderr: "Error: rate_limit_error: weekly cap reached", wantSub: "rate limit"},
		{name: "weekly limit", stderr: "weekly limit hit. Try again later.", wantSub: "rate limit"},
		{name: "5-hour limit", stderr: "you've hit your 5-hour limit", wantSub: "rate limit"},
		{name: "too many requests", stderr: "HTTP 429: Too many requests", wantSub: "rate limit"},
		{name: "not authenticated", stderr: "Error: not authenticated", wantSub: "claude /login"},
		{name: "please login", stderr: "Please log in to continue", wantSub: "claude /login"},
		{name: "invalid api key", stderr: "Error: invalid api key", wantSub: "claude /login"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyRunError([]byte(tt.stderr))
			if tt.wantNone {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantSub)
			}
			if !strings.Contains(got.Error(), tt.wantSub) {
				t.Errorf("error %q does not contain %q", got.Error(), tt.wantSub)
			}
		})
	}
}

func TestClassifyRunError_OnlyScansLastLines(t *testing.T) {
	// A noisy runner that prints a wall of progress before failing must
	// not have unrelated lines suppress the trailing rate-limit signature.
	// The classifier scans the last ~20 lines, which is more than enough
	// to catch a typical CLI failure.
	var noisy strings.Builder
	for range 100 {
		noisy.WriteString("progress chatter\n")
	}
	noisy.WriteString("Error: rate_limit_error: weekly cap reached\n")

	got := classifyRunError([]byte(noisy.String()))
	if got == nil {
		t.Fatalf("expected rate-limit classification, got nil")
	}
	if !strings.Contains(got.Error(), "rate limit") {
		t.Errorf("got %q", got.Error())
	}
}

func TestDispatch_ClassifiesRateLimitInStderr(t *testing.T) {
	gitFake := &fakeGit{out: [][]byte{[]byte("aaaa\n")}}
	claudeFake := &fakeClaude{
		err:    errors.New("exit status 1"),
		stderr: []byte("Error: rate_limit_error: weekly limit reached.\n"),
	}

	_, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, newOpts())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("expected rate-limit specific message, got %q", err.Error())
	}
	// Should NOT fall through to the generic wrapper when classified.
	if strings.Contains(err.Error(), "exited non-zero") {
		t.Errorf("classified error must not also carry generic wrapper: %q", err.Error())
	}
}

func TestDispatch_ClassifiesAuthErrorInStderr(t *testing.T) {
	gitFake := &fakeGit{out: [][]byte{[]byte("aaaa\n")}}
	claudeFake := &fakeClaude{
		err:    errors.New("exit status 1"),
		stderr: []byte("Error: not authenticated. Run /login.\n"),
	}

	_, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, newOpts())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "claude /login") {
		t.Errorf("expected auth-specific message, got %q", err.Error())
	}
}

func TestDispatch_FallsThroughToGenericOnUnknownStderr(t *testing.T) {
	// Arbitrary stderr that doesn't match any signature → keep the
	// existing "exited non-zero" wrapper so the operator still sees the
	// underlying exec error.
	gitFake := &fakeGit{out: [][]byte{[]byte("aaaa\n")}}
	claudeFake := &fakeClaude{
		err:    errors.New("exit status 1"),
		stderr: []byte("panic: nil pointer dereference\n"),
	}

	_, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, newOpts())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "exited non-zero") {
		t.Errorf("expected generic wrapper, got %q", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Repo snapshot (item 2: branch-switch / stranded untracked detection)
// ---------------------------------------------------------------------------

func TestParseUntracked(t *testing.T) {
	porcelain := []byte(" M foo.go\n?? new1.txt\n?? new2.go\nA  staged.go\n")
	got := parseUntracked(porcelain)
	if len(got) != 2 || !got["new1.txt"] || !got["new2.go"] {
		t.Errorf("parseUntracked: got %v, want {new1.txt, new2.go}", got)
	}
}

func TestDiffUntracked_SortedNew(t *testing.T) {
	pre := map[string]bool{"existing.txt": true}
	post := map[string]bool{"existing.txt": true, "zeta.go": true, "alpha.go": true}
	got := diffUntracked(pre, post)
	if len(got) != 2 || got[0] != "alpha.go" || got[1] != "zeta.go" {
		t.Errorf("diffUntracked: got %v, want [alpha.go zeta.go]", got)
	}
}

func TestDispatch_HaltsOnBranchSwitch(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"), // pre HEAD
			[]byte("bbbb\n"), // post HEAD (also new — but branch switched, so we never reach the log)
		},
		branchPre:  "main",
		branchPost: "feature/foo",
	}
	claudeFake := &fakeClaude{}

	_, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, newOpts())
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "switched branches") {
		t.Errorf("expected branch-switch wording, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "main") || !strings.Contains(err.Error(), "feature/foo") {
		t.Errorf("expected both branch names in error, got %q", err.Error())
	}
	if gitFake.hasGitArg("log") {
		t.Errorf("log must not run when branches diverged: %v", gitFake.args)
	}
}

func TestDispatch_WarnsOnStrandedUntracked(t *testing.T) {
	var buf bytes.Buffer
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"),
			[]byte("bbbb\n"),
			[]byte("subject\n"),
		},
		statusPre:  []byte(""),
		statusPost: []byte("?? scratch/notes.txt\n?? new_file.go\n"),
	}
	opts := newOpts()
	opts.Stdout = &buf

	got, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts)
	if err != nil {
		t.Fatalf("Dispatch should succeed (warning is non-fatal): %v", err)
	}
	if got.SHA != "bbbb" {
		t.Errorf("SHA: got %q, want bbbb", got.SHA)
	}
	output := buf.String()
	mustContain(t, output, "untracked file")
	mustContain(t, output, "scratch/notes.txt")
	mustContain(t, output, "new_file.go")
}

func TestDispatch_DoesNotWarnWhenNoNewUntracked(t *testing.T) {
	// Files that were already untracked before dispatch should NOT be
	// reported as "left … outside the commit" — the operator already
	// knew about them.
	var buf bytes.Buffer
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"),
			[]byte("bbbb\n"),
			[]byte("subject\n"),
		},
		statusPre:  []byte("?? pre-existing.txt\n"),
		statusPost: []byte("?? pre-existing.txt\n"),
	}
	opts := newOpts()
	opts.Stdout = &buf

	if _, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if strings.Contains(buf.String(), "untracked file") {
		t.Errorf("must not warn about pre-existing untracked file:\n%s", buf.String())
	}
}

// ---------------------------------------------------------------------------
// CLI-commits mode (items 1–3 of cli-owns-commit-not-agent.md)
// ---------------------------------------------------------------------------

func TestRenderCommitMessage_FormatShape(t *testing.T) {
	opts := newOpts()
	opts.Agent = "atdd-task"
	opts.IssueNum = 61
	opts.IssueTitle = "Redesigning New Order UI"
	opts.NodeDescription = "SYSTEM UI TASK - WRITE"

	got := renderCommitMessage(opts, "monolith-typescript/.../home.tsx | 2 +-\n 1 file changed, 1 insertion(+), 1 deletion(-)")

	want := "atdd-task(#61): Redesigning New Order UI\n" +
		"\nPhase: SYSTEM UI TASK - WRITE\n" +
		"\nmonolith-typescript/.../home.tsx | 2 +-\n 1 file changed, 1 insertion(+), 1 deletion(-)\n"
	if got != want {
		t.Errorf("renderCommitMessage:\n  got:\n%s\n  want:\n%s", got, want)
	}
}

func TestRenderCommitMessage_OmitsEmptyOptionalSections(t *testing.T) {
	// NodeDescription empty → no Phase: line. Empty diffStat → no body.
	opts := newOpts()
	opts.Agent = "atdd-chore"
	opts.IssueNum = 7
	opts.IssueTitle = "tidy"
	opts.NodeDescription = ""

	got := renderCommitMessage(opts, "")
	want := "atdd-chore(#7): tidy\n"
	if got != want {
		t.Errorf("renderCommitMessage: got %q, want %q", got, want)
	}
}

func TestParseDirty_AllStatusKinds(t *testing.T) {
	porcelain := []byte(" M modified.go\n?? new.txt\n D deleted.go\nA  staged.go\nR  old.go -> renamed.go\n")
	got := parseDirty(porcelain)
	want := []string{"modified.go", "new.txt", "deleted.go", "staged.go", "renamed.go"}
	for _, p := range want {
		if !got[p] {
			t.Errorf("parseDirty: missing %q in %v", p, got)
		}
	}
	if len(got) != len(want) {
		t.Errorf("parseDirty: got %d entries, want %d (%v)", len(got), len(want), got)
	}
}

func TestDispatch_CLICommits_StagesAndCommitsModifiedFile(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"),                                // pre rev-parse HEAD
			[]byte("aaaa\n"),                                // post rev-parse HEAD (same — agent didn't commit)
			[]byte(""),                                      // git add -A -- foo.go
			[]byte("foo.go | 2 +-\n 1 file changed\n"),      // git diff --cached --stat
			[]byte(""),                                      // git commit -m
			[]byte("bbbb\n"),                                // git rev-parse HEAD (new commit SHA)
		},
		statusPre:  []byte(""),
		statusPost: []byte(" M foo.go\n"),
		hooksDir:   t.TempDir(),
		gitDir:     t.TempDir(),
	}
	opts := newOpts()
	opts.CLICommits = true

	got, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got.SHA != "bbbb" {
		t.Errorf("SHA: got %q, want bbbb", got.SHA)
	}
	if !strings.Contains(got.Subject, "atdd-test(#42)") {
		t.Errorf("Subject: got %q", got.Subject)
	}
	if !gitFake.hasGitArg("add", "-A", "--", "foo.go") {
		t.Errorf("expected `git add -A -- foo.go`, got calls: %v", gitFake.args)
	}
	if !gitFake.hasGitArg("commit", "-m") {
		t.Errorf("expected `git commit -m ...`, got calls: %v", gitFake.args)
	}
}

func TestDispatch_CLICommits_StagesUntrackedFile(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"), []byte("aaaa\n"),
			[]byte(""), []byte("new.txt | 1 +\n"), []byte(""), []byte("bbbb\n"),
		},
		statusPre:  []byte(""),
		statusPost: []byte("?? new.txt\n"),
		hooksDir:   t.TempDir(),
		gitDir:     t.TempDir(),
	}
	opts := newOpts()
	opts.CLICommits = true

	if _, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !gitFake.hasGitArg("add", "-A", "--", "new.txt") {
		t.Errorf("expected `git add -A -- new.txt`, got: %v", gitFake.args)
	}
}

func TestDispatch_CLICommits_StagesDeletedFile(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"), []byte("aaaa\n"),
			[]byte(""), []byte("gone.go | 5 -----\n"), []byte(""), []byte("bbbb\n"),
		},
		statusPre:  []byte(""),
		statusPost: []byte(" D gone.go\n"),
		hooksDir:   t.TempDir(),
		gitDir:     t.TempDir(),
	}
	opts := newOpts()
	opts.CLICommits = true

	if _, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !gitFake.hasGitArg("add", "-A", "--", "gone.go") {
		t.Errorf("expected `git add -A -- gone.go`, got: %v", gitFake.args)
	}
}

func TestDispatch_CLICommits_SkipsPreExistingDirty(t *testing.T) {
	// Pre-existing `M dirty.go` is already in the operator's working
	// tree — must NOT be picked up by the CLI commit. Only the new
	// untracked file shows up in the staging delta.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"), []byte("aaaa\n"),
			[]byte(""), []byte("new.txt | 1 +\n"), []byte(""), []byte("bbbb\n"),
		},
		statusPre:  []byte(" M dirty.go\n"),
		statusPost: []byte(" M dirty.go\n?? new.txt\n"),
		hooksDir:   t.TempDir(),
		gitDir:     t.TempDir(),
	}
	opts := newOpts()
	opts.CLICommits = true

	if _, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if !gitFake.hasGitArg("add", "-A", "--", "new.txt") {
		t.Errorf("expected `git add` to include only new.txt, got: %v", gitFake.args)
	}
	for _, args := range gitFake.args {
		if len(args) >= 1 && args[0] == "add" {
			for _, a := range args {
				if a == "dirty.go" {
					t.Errorf("`git add` must not include pre-existing dirty path, got: %v", args)
				}
			}
		}
	}
}

func TestDispatch_CLICommits_NoChangesIsNoOp(t *testing.T) {
	// Clean before, clean after, HEAD unchanged → legitimate no-op
	// phase. Return zero CommitInfo without erroring; do not call
	// `git add` / `git commit`.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"), []byte("aaaa\n"),
		},
		statusPre:  []byte(""),
		statusPost: []byte(""),
		hooksDir:   t.TempDir(),
		gitDir:     t.TempDir(),
	}
	opts := newOpts()
	opts.CLICommits = true

	got, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got.SHA != "" {
		t.Errorf("expected empty SHA on no-op, got %q", got.SHA)
	}
	if gitFake.hasGitArg("add") {
		t.Errorf("`git add` must not run when delta is empty: %v", gitFake.args)
	}
	if gitFake.hasGitArg("commit") {
		t.Errorf("`git commit` must not run when delta is empty: %v", gitFake.args)
	}
}

func TestDispatch_CLICommits_HonorsAgentCommitWhenHeadMovesAndDeltaIsEmpty(t *testing.T) {
	// Migration window: prompts may still tell the agent to commit. If
	// HEAD moved during the subprocess and there's no leftover dirt,
	// honor the agent's commit instead of double-committing.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"),                  // pre rev-parse HEAD
			[]byte("bbbb\n"),                  // post rev-parse HEAD (agent committed)
			[]byte("atdd-task: agent did it\n"), // git log -1 --format=%s
		},
		statusPre:  []byte(""),
		statusPost: []byte(""),
		hooksDir:   t.TempDir(),
		gitDir:     t.TempDir(),
	}
	opts := newOpts()
	opts.CLICommits = true

	got, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts)
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got.SHA != "bbbb" {
		t.Errorf("SHA: got %q, want bbbb (agent's commit)", got.SHA)
	}
	if got.Subject != "atdd-task: agent did it" {
		t.Errorf("Subject: got %q", got.Subject)
	}
	if gitFake.hasGitArg("add") || gitFake.hasGitArg("commit") {
		t.Errorf("must not stage/commit on top of agent's commit: %v", gitFake.args)
	}
}

func TestDispatch_CLICommits_NoOpBannerSaysNoChanges(t *testing.T) {
	var buf bytes.Buffer
	gitFake := &fakeGit{
		out:      [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
		hooksDir: t.TempDir(),
		gitDir:   t.TempDir(),
	}
	opts := newOpts()
	opts.CLICommits = true
	opts.Stdout = &buf

	if _, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	got := buf.String()
	mustContain(t, got, "EXITED AGENT: no changes")
	if strings.Contains(got, "committed") {
		t.Errorf("no-op banner must not say 'committed': %s", got)
	}
}

func TestDispatch_CLICommits_BranchSwitchStillHalts(t *testing.T) {
	// Branch-switch detection applies in both modes — even with CLI
	// commits, we don't try to commit on whatever branch the agent
	// jumped to.
	gitFake := &fakeGit{
		out:        [][]byte{[]byte("aaaa\n"), []byte("bbbb\n")},
		branchPre:  "main",
		branchPost: "feature/foo",
		hooksDir:   t.TempDir(),
		gitDir:     t.TempDir(),
	}
	opts := newOpts()
	opts.CLICommits = true

	_, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts)
	if err == nil {
		t.Fatalf("expected branch-switch error, got nil")
	}
	if !strings.Contains(err.Error(), "switched branches") {
		t.Errorf("got %q", err.Error())
	}
	if gitFake.hasGitArg("add") || gitFake.hasGitArg("commit") {
		t.Errorf("must not commit when branch switched: %v", gitFake.args)
	}
}

func TestMaterializePrompt_BelowLimitReturnsVerbatim(t *testing.T) {
	dir := t.TempDir()
	prompt := strings.Repeat("x", promptArgvLimit)

	arg, cleanup, err := materializePrompt(dir, prompt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	if arg != prompt {
		t.Errorf("expected verbatim prompt; got %d-byte arg", len(arg))
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("no tempfile expected below limit; found %d entries", len(entries))
	}
}

func TestMaterializePrompt_AboveLimitSpillsToFile(t *testing.T) {
	dir := t.TempDir()
	prompt := strings.Repeat("y", promptArgvLimit+1)

	arg, cleanup, err := materializePrompt(dir, prompt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	if len(arg) >= len(prompt) {
		t.Errorf("expected short bootstrap arg; got %d bytes (prompt was %d)", len(arg), len(prompt))
	}
	if !strings.Contains(arg, ".atdd-prompt-") || !strings.Contains(arg, ".tmp.md") {
		t.Errorf("bootstrap arg missing tempfile reference: %q", arg)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read tempdir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected exactly 1 tempfile; got %d", len(entries))
	}
	body, err := os.ReadFile(dir + string(os.PathSeparator) + entries[0].Name())
	if err != nil {
		t.Fatalf("read tempfile: %v", err)
	}
	if string(body) != prompt {
		t.Errorf("tempfile content mismatch: got %d bytes, want %d", len(body), len(prompt))
	}
}

func TestMaterializePrompt_CleanupRemovesTempfile(t *testing.T) {
	dir := t.TempDir()
	prompt := strings.Repeat("z", promptArgvLimit+1)

	_, cleanup, err := materializePrompt(dir, prompt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cleanup()

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("cleanup must remove tempfile; %d entries remain", len(entries))
	}
}

func TestCappedBuffer_TruncatesPastCap(t *testing.T) {
	var b cappedBuffer
	b.cap = 10
	b.Write([]byte("0123456789"))
	b.Write([]byte("ABCDEFG")) // dropped
	if got := string(b.Bytes()); got != "0123456789" {
		t.Errorf("cappedBuffer: got %q, want %q", got, "0123456789")
	}
}

// ---------------------------------------------------------------------------
// Pre-commit hook installer (plans/20260502-200525-pre-commit-hook-blocks-agent-commits.md)
// ---------------------------------------------------------------------------

func TestDispatch_CLICommits_InstallsPreCommitHook(t *testing.T) {
	hooksDir := t.TempDir()
	gitDir := t.TempDir()
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"), []byte("aaaa\n"),
		},
		hooksDir: hooksDir,
		gitDir:   gitDir,
	}
	opts := newOpts()
	opts.CLICommits = true

	if _, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	hookPath := filepath.Join(hooksDir, "pre-commit")
	body, err := os.ReadFile(hookPath)
	if err != nil {
		t.Fatalf("read installed hook: %v", err)
	}
	if string(body) != preCommitHookBody {
		t.Errorf("hook body mismatch:\n  got:\n%s\n  want:\n%s", body, preCommitHookBody)
	}
	markerPath := filepath.Join(gitDir, clauderunMarkerName)
	if _, err := os.Stat(markerPath); err != nil {
		t.Errorf("dispatch marker missing at %s: %v", markerPath, err)
	}
}

func TestDispatch_LegacyMode_DoesNotInstallHook(t *testing.T) {
	// CLICommits=false — the hook must not be written, so legacy
	// rehearsals where the agent commits keep working.
	hooksDir := t.TempDir()
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"), []byte("bbbb\n"), []byte("subject\n"),
		},
		hooksDir: hooksDir,
		gitDir:   t.TempDir(),
	}
	opts := newOpts()

	if _, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	if _, err := os.Stat(filepath.Join(hooksDir, "pre-commit")); !os.IsNotExist(err) {
		t.Errorf("expected no pre-commit hook in legacy mode; got err=%v", err)
	}
	for _, args := range gitFake.args {
		if len(args) >= 3 && args[0] == "rev-parse" && args[1] == "--git-path" {
			t.Errorf("install resolver must not run in legacy mode, got args: %v", args)
		}
	}
}

func TestDispatch_HookConflict_RefusesToOverwrite(t *testing.T) {
	// Pre-existing pre-commit hook with different content — Dispatch
	// must surface a clear error rather than silently clobber the
	// operator's hook.
	hooksDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(hooksDir, "pre-commit"), []byte("#!/bin/sh\necho operator hook\n"), 0o755); err != nil {
		t.Fatalf("seed conflicting hook: %v", err)
	}

	gitFake := &fakeGit{
		out:      [][]byte{[]byte("aaaa\n")},
		hooksDir: hooksDir,
		gitDir:   t.TempDir(),
	}
	opts := newOpts()
	opts.CLICommits = true

	_, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts)
	if err == nil {
		t.Fatalf("expected hook-conflict error, got nil")
	}
	if !strings.Contains(err.Error(), "refusing to overwrite") {
		t.Errorf("error wording: got %q", err.Error())
	}
}

func TestCommitChanges_SetsEnvVarForCommit(t *testing.T) {
	// commitChanges must set CLAUDERUN_CLI_COMMIT=1 in the process env
	// for the duration of the `git commit` call so the installed hook
	// recognizes the CLI as the authorized committer. The fake captures
	// os.Getenv at call time; this assertion is what links the env-var
	// plumbing (item 2) to the hook (item 1).
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte(""),                      // git add -A -- foo.go
			[]byte("foo.go | 2 +-\n"),       // git diff --cached --stat
			[]byte(""),                      // git commit -m
			[]byte("bbbb\n"),                // git rev-parse HEAD
		},
	}
	opts := newOpts()
	opts.RepoPath = t.TempDir()

	pre := repoState{head: "aaaa", branch: "main", dirty: map[string]bool{}}
	post := repoState{head: "aaaa", branch: "main", dirty: map[string]bool{"foo.go": true}}

	// Make sure the var is unset going in so we can verify defer-restore
	// actually clears it after the call.
	os.Unsetenv("CLAUDERUN_CLI_COMMIT")

	if _, err := commitChanges(context.Background(), gitFake, opts, pre, post); err != nil {
		t.Fatalf("commitChanges: %v", err)
	}

	if !gitFake.commitEnvSet || gitFake.commitEnv != "1" {
		t.Errorf("CLAUDERUN_CLI_COMMIT during commit: got set=%v value=%q, want set=true value=\"1\"",
			gitFake.commitEnvSet, gitFake.commitEnv)
	}
	if _, stillSet := os.LookupEnv("CLAUDERUN_CLI_COMMIT"); stillSet {
		t.Errorf("CLAUDERUN_CLI_COMMIT must be unset after commitChanges returns")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("missing %q in:\n%s", needle, haystack)
	}
}
