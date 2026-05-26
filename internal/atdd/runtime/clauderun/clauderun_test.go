// Tests for clauderun.Dispatch.
//
// Strategy: drive Dispatch through fakeClaude / fakeGit so the suite is
// hermetic — no real `claude` or `git` invocations. Each fake captures
// the args / Run call it received and emits canned values, letting us
// assert prompt construction, working-tree-delta detection, and error
// paths.
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

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// ---------------------------------------------------------------------------
// Fakes
// ---------------------------------------------------------------------------

// fakeClaude records the RunOpts it was called with and returns a canned
// error. stderr (when set) is written to opts.Stderr before returning,
// used by the rate-limit / auth classification tests.
type fakeClaude struct {
	calls  []RunOpts
	err    error
	usage  *TokenUsage
	stderr []byte
}

func (f *fakeClaude) Run(_ context.Context, opts RunOpts) (RunResult, error) {
	f.calls = append(f.calls, opts)
	if len(f.stderr) > 0 && opts.Stderr != nil {
		opts.Stderr.Write(f.stderr)
	}
	return RunResult{Usage: f.usage}, f.err
}

// fakeGit serves canned outputs. The HEAD-rev-parse calls consume the
// `out` FIFO. Snapshot calls (rev-parse --abbrev-ref HEAD, status
// --porcelain) get sensible defaults so tests that don't care about
// branch-switch / untracked detection don't have to enumerate them.
// Tests that DO care can override via branchPre/branchPost (FIFO, used
// per call) and statusPre/statusPost.
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
}

func (f *fakeGit) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	f.args = append(f.args, args)
	if f.err != nil {
		return nil, f.err
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
// argument prefix.
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
		Agent:           "write-acceptance-tests",
		NodeDescription: "Write the AT-RED scenario",
		IssueNum:        42,
		IssueTitle:      "Add PUT /carts/{id}/items endpoint",
		// The stripped prompts reference ${language} in the language-
		// equivalents pointer; seed a default so tests don't have to.
		Language: "java",
		// write-acceptance-tests.md references ${acceptance_criteria}; seed a
		// default so dispatch tests using this scaffold render cleanly.
		// Tests that exercise the unset/load-bearing path override this.
		AcceptanceCriteria: "Scenario: placeholder\n  Given x\n  When y\n  Then z",
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
	opts := newOpts()
	opts.OverrideText = "prefer record types"

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}

	// v2: prompt is the embedded agent body with ${name} placeholders
	// substituted; the parent-claude "Launch the X subagent" wrapper is gone.
	mustContain(t, got, "The Acceptance Criteria below were parsed")
	mustContain(t, got, `#42 "Add PUT /carts/{id}/items endpoint"`)
	mustContain(t, got, "Phase: Write the AT-RED scenario")
	// Phase doc was dropped — every preamble agent now reads from its
	// own embedded prompt body; assert the rendered preamble does NOT
	// reintroduce the dangling line.
	if strings.Contains(got, "Phase doc:") {
		t.Errorf("rendered prompt still has a `Phase doc:` line:\n%s", got)
	}
	mustContain(t, got, "prefer record types")
	mustContain(t, got, "do not summarise")
	mustContain(t, got, "the agent must never run `git commit`")
	// All ${…} placeholders must be expanded — none should leak through.
	if strings.Contains(got, "${") {
		t.Errorf("prompt still contains ${...} placeholder")
	}
}

func TestRenderPrompt_NoLegacyCommitGatingLeaksAcrossAgents(t *testing.T) {
	// The legacy commit-confirmation reference block and its placeholder
	// marker were removed when clauderun stopped owning the commit. Smoke-
	// test every embedded prompt to make sure no agent leaks the marker
	// or the pre-rollout preamble.
	for _, name := range []string{
		"implement-system", "refactor-system",
		"implement-system-driver-adapters",
		"implement-external-system-driver-adapters",
		"implement-dsl",
		"implement-external-system-stubs",
		"write-acceptance-tests", "write-contract-tests",
	} {
		opts := newOpts()
		opts.Agent = name

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
		if strings.Contains(got, "# Commit Confirmation Rule") {
			t.Errorf("%s: legacy commit-confirmation rule block leaked", name)
		}
	}
}

func TestRenderPrompt_TaskAgentArchitectureAndAllowedRoots_ExplicitValues(t *testing.T) {
	opts := newOpts()
	opts.Agent = "implement-system"
	opts.Architecture = "monolith"
	opts.AllowedRoots = "- System: system/monolith/java (lang: java)\n- System tests: system-test/java (lang: java)\n"
	// implement-system's body references ${checklist}, now load-bearing.
	// Production dispatch fills this from ctx.State["ticket_checklist"]
	// (populated by parse-ticket); supply directly so the no-leftover-${...}
	// assertion below doesn't trip.
	opts.Checklist = "- [ ] Refactor X"
	// implement-system now inlines phase-doc placeholders
	// in its body; without these the no-leftover-${...} assertion below
	// would catch the inlined Family B path references.
	opts.Placeholders = map[string]string{
		"sut-namespace":    "shop",
		"system-test-path": "system-test/java",
		"driver-port":      "system-test/src/testkit/driver/port/shop",
		"driver-adapter":   "system-test/src/testkit/driver/adapter/shop",
	}

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	mustContain(t, got, "Architecture: monolith")
	mustContain(t, got, "Allowed write roots:")
	mustContain(t, got, "- System: system/monolith/java (lang: java)")
	mustContain(t, got, "- System tests: system-test/java (lang: java)")
	if strings.Contains(got, "${") {
		t.Errorf("prompt still contains ${...} placeholder: %s", got)
	}
}

func TestRenderPrompt_TaskAgentChecklistInjected(t *testing.T) {
	opts := newOpts()
	opts.Agent = "implement-system"
	opts.Checklist = "- [x] Rename \"New Order\" to \"Place Order\"\n- [x] Rename SKU aria-label"

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	mustContain(t, got, opts.Checklist)
	if strings.Contains(got, "Fetch the issue with `gh`") {
		t.Errorf("implement-system prompt should no longer instruct the agent to fetch the issue: %s", got)
	}
	if strings.Contains(got, "${checklist}") {
		t.Errorf("${checklist} placeholder leaked into rendered prompt")
	}
}

func TestRenderPrompt_RefactorSystemAgent_EmptyArchitectureAndRootsRender(t *testing.T) {
	// When architecture and allowed_roots are empty (e.g. fresh config or
	// pre-resolution), the placeholders expand to empty strings — the
	// prompt still renders without leaking ${...}. There is no longer a
	// "broadest defaults" fallback; per-component lang has replaced
	// `Architecture=both`/`Lang=all` semantics.
	opts := newOpts()
	opts.Agent = "refactor-system"
	// refactor-system's body references ${checklist} (load-bearing).
	// Production fills this from ctx.State["ticket_checklist"]; supply
	// directly so the no-leftover-${...} assertion below doesn't trip.
	opts.Checklist = "- [ ] Refactor X"
	// The refactor-system prompt inlines phase-doc
	// placeholders that the production dispatcher fills from
	// cfg.PlaceholderMap(); supply them directly so the body renders
	// without ${...} leftovers.
	opts.Placeholders = map[string]string{
		"sut-namespace":    "shop",
		"system-test-path": "system-test",
		"driver-port":      "system-test/src/testkit/driver/port/shop",
		"driver-adapter":   "system-test/src/testkit/driver/adapter/shop",
	}

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	mustContain(t, got, "Architecture: ")
	mustContain(t, got, "Allowed write roots:")
	if strings.Contains(got, "${") {
		t.Errorf("prompt still contains ${...} placeholder: %s", got)
	}
}

func TestRenderPrompt_ReturnsErrorForUnknownAgent(t *testing.T) {
	opts := newOpts()
	opts.Agent = "atdd-doesnotexist"
	if _, err := renderPrompt(opts); err == nil {
		t.Fatalf("expected error for unknown agent, got nil")
	}
}

// Step 6/D6 stripped the inlined `### Reference: ...` blocks from every
// prompt body. The external-system doctrine that previously distinguished
// the two task subtype prompts was inlined from
// references/atdd/architecture/driver-adapter.md; after the strip, both
// subtype files point at that doc via ${references_root} instead of
// inlining it, so the bodies are now identical except for routing. The
// previous "external subtype includes doctrine inline / system subtype
// excludes it" assertions are obsolete — the doctrine is read on-demand
// from the synced ~/.gh-optivem/references/atdd/architecture/driver-adapter.md
// by both variants. Subtype routing still works via the task-name lookup
// in process-flow.yaml.

// TestRenderPrompt_ReferencesRootSubstitutes covers the placeholder that
// lets prompt bodies reference the per-user synced references root via
// ${references_root}. The substituted value is an absolute path so the
// agent's Read tool resolves it regardless of working directory.
// Uses PromptOverride (which IS expanded, unlike OverrideText) to
// inject a body that references the placeholder.
func TestRenderPrompt_ReferencesRootSubstitutes(t *testing.T) {
	opts := newOpts()
	opts.PromptOverride = "Read ${references_root}/atdd/architecture/dsl-core.md."

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	if strings.Contains(got, "${references_root}") {
		t.Errorf("expected ${references_root} substituted; got: %q", got)
	}
	// references_root substitutes to an OS-native path (Windows uses
	// backslashes); the literal trailing "/atdd/..." in the template
	// stays as-is. Assert the two halves independently so the test
	// is platform-portable.
	if !strings.Contains(got, filepath.Join(".gh-optivem", "references")) {
		t.Errorf("expected ${references_root} resolved to ~/.gh-optivem/references prefix; got: %q", got)
	}
	if !strings.Contains(got, "/atdd/architecture/dsl-core.md") {
		t.Errorf("expected literal suffix preserved; got: %q", got)
	}
}

// TestDispatch_MaterializesProjectReferencesWhenProjectConfigSet covers the
// project-local-references wiring: when Options.ProjectConfig and RepoPath
// are both set, Dispatch calls MaterializeProject before render and the
// rendered prompt's ${references_root} resolves to
// <RepoPath>/.gh-optivem/references instead of the user-global home path.
//
// To keep the test independent of the current state of the embedded
// `runtime/references/**/*.md` corpus (some teaching docs intentionally
// contain ${name}-shaped meta-references that the substituter can't
// distinguish from real placeholders), the test pre-writes a sidecar
// that matches the PlaceholderMap exactly. MaterializeProject's value-
// based staleness check returns "not stale" and short-circuits without
// walking the embedded tree — exercising the wiring path that matters
// here: Dispatch → MaterializeProject → projectReferencesRoot → renderPrompt.
func TestDispatch_MaterializesProjectReferencesWhenProjectConfigSet(t *testing.T) {
	repoPath := t.TempDir()
	cfg := &projectconfig.Config{
		System: projectconfig.System{
			Architecture: "monolith",
			Path:         "system",
			Repo:         "x/y",
			Lang:         "typescript",
		},
		SystemTest: projectconfig.TierSpec{
			Path:  "system-test",
			Repo:  "x/y",
			Lang:  "typescript",
			Paths: projectconfig.DefaultPaths(projectconfig.LangTypescript, "system-test", "y"),
		},
	}
	preWriteFreshSidecar(t, repoPath, cfg.PlaceholderMap())

	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	claudeFake := &fakeClaude{}

	opts := newOpts()
	opts.RepoPath = repoPath
	opts.ProjectConfig = cfg
	opts.PromptOverride = "Read ${references_root}/atdd/architecture/system.md."

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(claudeFake.calls) != 1 {
		t.Fatalf("expected 1 claude call, got %d", len(claudeFake.calls))
	}
	prompt := claudeFake.calls[0].Prompt
	wantPrefix := filepath.Join(repoPath, ".gh-optivem", "references")
	if !strings.Contains(prompt, wantPrefix) {
		t.Errorf("expected ${references_root} resolved to %q in prompt; got: %q", wantPrefix, prompt)
	}
}

// preWriteFreshSidecar writes a materialize sidecar that matches
// `placeholders` so the next MaterializeProject call against repoPath
// short-circuits via projectStale=false. Used to test the Dispatch →
// materialize wiring without exercising the embedded-doc walk.
func preWriteFreshSidecar(t *testing.T, repoPath string, placeholders map[string]string) {
	t.Helper()
	sidecarDir := filepath.Join(repoPath, ".gh-optivem")
	if err := os.MkdirAll(sidecarDir, 0o755); err != nil {
		t.Fatalf("mkdir sidecar dir: %v", err)
	}
	var b strings.Builder
	b.WriteString("binary_version: \"\"\nplaceholders:\n")
	for k, v := range placeholders {
		b.WriteString("  ")
		b.WriteString(k)
		b.WriteString(": ")
		b.WriteString(v)
		b.WriteString("\n")
	}
	if err := os.WriteFile(filepath.Join(sidecarDir, ".materialized.yaml"), []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
}

// TestDispatch_FallsBackToUserGlobalReferencesRootWhenProjectConfigNil covers
// the legacy / scaffold-flow path: callers without a ProjectConfig get
// ${references_root} resolved against assetsync.ReferencesRoot() and no
// project-local materialization happens. Regression guard so a future
// "always materialize" change doesn't break CLI utilities and tests that
// legitimately have no project context.
func TestDispatch_FallsBackToUserGlobalReferencesRootWhenProjectConfigNil(t *testing.T) {
	repoPath := t.TempDir()
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	claudeFake := &fakeClaude{}

	opts := newOpts()
	opts.RepoPath = repoPath
	// ProjectConfig left nil — should fall back to the user-global root.
	opts.PromptOverride = "Read ${references_root}/atdd/architecture/system.md."

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	prompt := claudeFake.calls[0].Prompt
	// The user-global root never lives under the test's tempdir.
	projectLocal := filepath.Join(repoPath, ".gh-optivem", "references")
	if strings.Contains(prompt, projectLocal) {
		t.Errorf("expected NOT to substitute project-local references root; got: %q", prompt)
	}
	// The sidecar must NOT exist — materialize should not have fired.
	if _, err := os.Stat(filepath.Join(repoPath, ".gh-optivem", ".materialized.yaml")); err == nil {
		t.Errorf("expected no sidecar when ProjectConfig is nil")
	}
}

// TestRenderPrompt_LanguageSubstitutes covers the D10 placeholder that
// lets prompt bodies select a per-language reference doc slice via
// ${language}. The driver picks the value per phase from project config.
func TestRenderPrompt_LanguageSubstitutes(t *testing.T) {
	opts := newOpts()
	opts.Language = "go"
	opts.PromptOverride = "Read ${references_root}/code/language-equivalents/${language}.md."

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	if strings.Contains(got, "${language}") {
		t.Errorf("expected ${language} substituted; got: %q", got)
	}
	if !strings.Contains(got, "language-equivalents/go.md") {
		t.Errorf("expected ${language} resolved to 'go'; got: %q", got)
	}
}

// TestRenderPrompt_AcceptanceCriteriaSubstitutes covers the
// ${acceptance_criteria} placeholder that lets write-acceptance-tests consume
// the scenarios intake parsed from the ticket body without re-fetching the
// issue via `gh issue view`.
func TestRenderPrompt_AcceptanceCriteriaSubstitutes(t *testing.T) {
	opts := newOpts()
	opts.AcceptanceCriteria = "Scenario: View product list\n  Given the catalog has products\n  When I open the product list page\n  Then I see them"
	opts.PromptOverride = "Scenarios:\n${acceptance_criteria}"

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	if strings.Contains(got, "${acceptance_criteria}") {
		t.Errorf("expected ${acceptance_criteria} substituted; got: %q", got)
	}
	if !strings.Contains(got, "View product list") {
		t.Errorf("expected scenario body in rendered prompt; got: %q", got)
	}
}

// TestRenderPrompt_UnsetAcceptanceCriteriaFailsFast pins the load-bearing
// contract: a prompt that references ${acceptance_criteria} with no value
// set produces a clear render-time error rather than silently substituting
// an empty block. AT - RED - TEST is contractually scoped to implementing
// AC, so an absent value is a wiring bug, not a valid state.
func TestRenderPrompt_UnsetAcceptanceCriteriaFailsFast(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	opts := newOpts()
	opts.AcceptanceCriteria = "" // override the test default
	opts.PromptOverride = "You are the Test Agent. Scenarios:\n${acceptance_criteria}"
	_, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts)
	if err == nil {
		t.Fatalf("expected error for unset ${acceptance_criteria}, got nil")
	}
	if !strings.Contains(err.Error(), "acceptance_criteria") {
		t.Errorf("expected error to name 'acceptance_criteria'; got: %v", err)
	}
}

// TestRenderPrompt_UnsetChecklistFailsFast pins the load-bearing
// contract on ${checklist}: a prompt that references ${checklist} with
// no value set produces a clear render-time error rather than silently
// substituting an empty block. The five Checklist-using ticket-kinds
// (system-redesign, external-system-redesign, system-refactor,
// test-refactor, external-system-onboarding) all dispatch prompts that
// read ${checklist}; an absent value means parse-ticket didn't populate
// or the ticket-kind/body declaration drifted.
func TestRenderPrompt_UnsetChecklistFailsFast(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	opts := newOpts()
	opts.Checklist = "" // explicit override of default (already empty in newOpts)
	opts.PromptOverride = "You are the Structural Agent. Checklist:\n${checklist}"
	_, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts)
	if err == nil {
		t.Fatalf("expected error for unset ${checklist}, got nil")
	}
	if !strings.Contains(err.Error(), "checklist") {
		t.Errorf("expected error to name 'checklist'; got: %v", err)
	}
}

// TestRenderPrompt_UnsetLanguageFailsFast pins the D10 "load-bearing"
// contract: a prompt that references ${language} with no Language set
// produces a clear render-time error rather than silently substituting
// an empty path that would resolve to a missing doc at agent run time.
func TestRenderPrompt_UnsetLanguageFailsFast(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	opts := newOpts()
	opts.Language = "" // override the test default to exercise the load-bearing path
	// Use a node_replacements-style PromptOverride that references
	// ${language} without setting Language. Dispatch's
	// findUnfilledPlaceholders catches the leftover.
	opts.PromptOverride = "You are the Test Agent. Read ${references_root}/code/language-equivalents/${language}.md."
	_, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts)
	if err == nil {
		t.Fatalf("expected error for unset ${language}, got nil")
	}
	if !strings.Contains(err.Error(), "language") {
		t.Errorf("expected error to name 'language'; got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Dispatch — happy path
// ---------------------------------------------------------------------------

func TestDispatch_SuccessReturnsNilOnCleanExit(t *testing.T) {
	// Subprocess exits zero, working tree is dirty (one new file). HEAD
	// stays put — clauderun no longer commits, so an unchanged HEAD is
	// the expected shape, not an error.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaaaaa1111111\n"), // pre rev-parse HEAD
			[]byte("aaaaaaa1111111\n"), // post rev-parse HEAD (same)
		},
		statusPre:  []byte(""),
		statusPost: []byte(" M foo.go\n"),
	}
	claudeFake := &fakeClaude{}

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, newOpts()); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(claudeFake.calls) != 1 {
		t.Fatalf("expected 1 claude call, got %d", len(claudeFake.calls))
	}
	if !strings.Contains(claudeFake.calls[0].Prompt, "The Acceptance Criteria below were parsed") {
		t.Errorf("prompt missing expected write-acceptance-tests body marker")
	}
	if gitFake.hasGitArg("add") || gitFake.hasGitArg("commit") {
		t.Errorf("clauderun must not stage or commit: %v", gitFake.args)
	}
}

func TestDispatch_NoChangesIsNotAnError(t *testing.T) {
	// Clean before, clean after, HEAD unchanged. Previously this was the
	// "no commit produced" failure path; now it's a legitimate no-op.
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("samesha\n"),
			[]byte("samesha\n"),
		},
	}
	claudeFake := &fakeClaude{}

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, newOpts()); err != nil {
		t.Fatalf("expected no error on clean no-op, got %v", err)
	}
}

func TestDispatch_AutonomousFlagPropagates(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
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
	// On the non-zero-exit path the post-snapshot must not run.
	if gitFake.statusCount != 1 {
		t.Errorf("expected exactly 1 status --porcelain (pre-snapshot only), got %d", gitFake.statusCount)
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
			[]byte("aaaa\n"),
		},
		statusPre:  []byte(""),
		statusPost: []byte(" M foo.go\n"),
	}
	opts := newOpts()
	opts.Stdout = &buf

	if _, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	got := buf.String()
	mustContain(t, got, "ENTERING AGENT")
	mustContain(t, got, "write-acceptance-tests")
	mustContain(t, got, "EXITED AGENT")
	mustContain(t, got, "1 file(s) changed")
}

func TestDispatch_BannerSaysNoChangesOnCleanExit(t *testing.T) {
	var buf bytes.Buffer
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	opts := newOpts()
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

// ---------------------------------------------------------------------------
// Token usage parsing & banner formatting
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
	usage, result := parseClaudeJSON([]byte("claude: command failed\n"))
	if usage != nil || result != "" {
		t.Errorf("expected (nil, \"\") on malformed input, got (%+v, %q)", usage, result)
	}

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
	writeExitBanner(opts, 7, 47*time.Second, usage, nil)

	got := buf.String()
	mustContain(t, got, "EXITED AGENT: 7 file(s) changed")
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
	writeExitBanner(opts, 1, 5*time.Second, nil, nil)

	got := buf.String()
	mustContain(t, got, "EXITED AGENT")
	if strings.Contains(got, "$") || strings.Contains(got, " in ") {
		t.Errorf("expected no token suffix when usage is nil, got:\n%s", got)
	}
}

// ---------------------------------------------------------------------------
// Stderr classification (rate limit / auth)
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
// Repo snapshot (branch-switch / stranded untracked detection)
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

func TestDispatch_HaltsOnBranchSwitch(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"), // pre HEAD
			[]byte("bbbb\n"), // post HEAD
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
}

func TestDispatch_WarnsOnStrandedUntracked(t *testing.T) {
	var buf bytes.Buffer
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"),
			[]byte("aaaa\n"),
		},
		statusPre:  []byte(""),
		statusPost: []byte("?? scratch/notes.txt\n?? new_file.go\n"),
	}
	opts := newOpts()
	opts.Stdout = &buf

	if _, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch should succeed (warning is non-fatal): %v", err)
	}
	output := buf.String()
	mustContain(t, output, "untracked file")
	mustContain(t, output, "scratch/notes.txt")
	mustContain(t, output, "new_file.go")
}

func TestDispatch_DoesNotWarnWhenNoNewUntracked(t *testing.T) {
	var buf bytes.Buffer
	gitFake := &fakeGit{
		out: [][]byte{
			[]byte("aaaa\n"),
			[]byte("aaaa\n"),
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
// materializePrompt (argv overflow handling)
// ---------------------------------------------------------------------------

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

// Multi-line prompts must spill to a tempfile even when their byte count
// is under promptArgvLimit. Windows' `.cmd` shim for the `claude` CLI
// truncates argv at the first newline, so handing a small multi-line
// prompt to exec.Command would deliver only the first line to the agent.
func TestMaterializePrompt_MultiLineBelowLimitSpillsToFile(t *testing.T) {
	dir := t.TempDir()
	prompt := "first line\nsecond line\nthird line\n"
	if len(prompt) > promptArgvLimit {
		t.Fatalf("test precondition: prompt %d bytes should be under limit %d", len(prompt), promptArgvLimit)
	}

	arg, cleanup, err := materializePrompt(dir, prompt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	if arg == prompt {
		t.Fatal("multi-line prompt must not be returned verbatim — Windows cmd.exe would truncate at first newline")
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
	body, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read tempfile: %v", err)
	}
	if string(body) != prompt {
		t.Errorf("tempfile content mismatch: got %q, want %q", string(body), prompt)
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

// ---------------------------------------------------------------------------
// Unfilled-placeholder guard (item 3)
// ---------------------------------------------------------------------------

func TestDispatch_HaltsOnUnfilledPlaceholder(t *testing.T) {
	// PromptOverride is the simplest way to inject a known-missing
	// ${...} key into the rendered prompt without modifying an embedded
	// agent body. ${unmapped_field} has no entry in renderPrompt's
	// params map, so ExpandParams leaves it untouched and the guardrail
	// catches it. RawPrompt would skip the check entirely (documented
	// escape hatch); PromptOverride still goes through ExpandParams +
	// the guard.
	gitFake := &fakeGit{}
	claudeFake := &fakeClaude{}
	opts := newOpts()
	opts.PromptOverride = "Imagine: ${unmapped_field}\nDo the thing."

	_, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts)
	if err == nil {
		t.Fatalf("expected error when ${unmapped_field} is unfilled, got nil")
	}
	if !strings.Contains(err.Error(), "unfilled placeholders") {
		t.Errorf("error should mention unfilled placeholders, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "${unmapped_field}") {
		t.Errorf("error should name the leftover placeholder, got %q", err.Error())
	}
	if len(claudeFake.calls) != 0 {
		t.Errorf("claude must not run when placeholders are unfilled, got %d calls", len(claudeFake.calls))
	}
}

func TestDispatch_HaltsOnUnfilledCommandPlaceholder(t *testing.T) {
	// fix-command-failed's prompt references ${command} — the trio
	// ${command} / ${command_exit_code} / ${command_stderr_tail} is
	// registered into params only when Options.CommandLine is non-empty
	// (mirroring Checklist / AcceptanceCriteria). Empty CommandLine plus
	// a prompt that references ${command} must trigger the same fast
	// failure the unfilled-placeholder guard provides for other
	// load-bearing fields, so a regression in the wiring (runCommand
	// silently skipping its state stash, ExpandParams losing the
	// state-fallback path, dispatcher not reading from ctx.State, …)
	// surfaces before the agent reads a blank prompt.
	gitFake := &fakeGit{}
	claudeFake := &fakeClaude{}
	opts := newOpts()
	opts.PromptOverride = "Diagnose the failure of: ${command}\nexit=${command_exit_code}\nstderr=${command_stderr_tail}"
	// CommandLine intentionally left empty.

	_, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts)
	if err == nil {
		t.Fatalf("expected error when ${command} is unfilled, got nil")
	}
	if !strings.Contains(err.Error(), "unfilled placeholders") {
		t.Errorf("error should mention unfilled placeholders, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "${command}") {
		t.Errorf("error should name the leftover placeholder, got %q", err.Error())
	}
	if len(claudeFake.calls) != 0 {
		t.Errorf("claude must not run when placeholders are unfilled, got %d calls", len(claudeFake.calls))
	}
}

func TestDispatch_CommandPlaceholdersResolveWhenPopulated(t *testing.T) {
	// Counterpart to TestDispatch_HaltsOnUnfilledCommandPlaceholder:
	// when CommandLine is non-empty, the full trio registers and the
	// prompt resolves. Locks in the failure-payload propagation path
	// runCommand → ctx.State → driver → clauderun for fix-command-failed.
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	claudeFake := &fakeClaude{}
	opts := newOpts()
	opts.PromptOverride = "cmd=${command} exit=${command_exit_code} tail=${command_stderr_tail}"
	opts.CommandLine = "gh optivem system build"
	opts.CommandExitCode = 2
	opts.CommandStderrTail = "boom: missing config"

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(claudeFake.calls) != 1 {
		t.Fatalf("expected 1 claude call, got %d", len(claudeFake.calls))
	}
	want := "cmd=gh optivem system build exit=2 tail=boom: missing config"
	if got := claudeFake.calls[0].Prompt; got != want {
		t.Errorf("prompt:\n got: %q\nwant: %q", got, want)
	}
}

func TestDispatch_RawPromptSkipsPlaceholderCheck(t *testing.T) {
	// node_replacements: short-circuits the substitution path entirely;
	// an operator who deliberately writes ${foo} in their replacement
	// file body should not be blocked by the guardrail.
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	claudeFake := &fakeClaude{}
	opts := newOpts()
	opts.RawPrompt = "Literal ${unfilled} text — RawPrompt mode."

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch with RawPrompt: %v", err)
	}
	if len(claudeFake.calls) != 1 {
		t.Fatalf("expected 1 claude call, got %d", len(claudeFake.calls))
	}
	if claudeFake.calls[0].Prompt != opts.RawPrompt {
		t.Errorf("RawPrompt did not pass through verbatim:\n got: %q\nwant: %q",
			claudeFake.calls[0].Prompt, opts.RawPrompt)
	}
}

func TestFindUnfilledPlaceholders_ReturnsDistinctOrdered(t *testing.T) {
	got := findUnfilledPlaceholders("first ${foo}, then ${bar}, then ${foo} again, ${baz_quux}")
	want := []string{"${foo}", "${bar}", "${baz_quux}"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFindUnfilledPlaceholders_NoMatchesReturnsNil(t *testing.T) {
	if got := findUnfilledPlaceholders("nothing to see here, $literal $$double"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// Prompt log (item 2)
// ---------------------------------------------------------------------------

func TestDispatch_WritesPromptLogWhenPathSet(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "runs", "001-write-acceptance-tests.prompt.md")

	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	claudeFake := &fakeClaude{}
	opts := newOpts()
	opts.PromptLogPath = logPath

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	body, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read prompt log: %v", err)
	}
	if string(body) != claudeFake.calls[0].Prompt {
		t.Errorf("prompt log does not match captured prompt byte-for-byte\n got len=%d\nwant len=%d",
			len(body), len(claudeFake.calls[0].Prompt))
	}
}

func TestDispatch_PromptLogFailureIsNonFatal(t *testing.T) {
	// Pointing PromptLogPath at a file path that exists *as a directory*
	// makes os.WriteFile fail. Dispatch should warn to stderr and continue.
	dir := t.TempDir()
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	claudeFake := &fakeClaude{}
	var stderr bytes.Buffer
	opts := newOpts()
	opts.Stderr = &stderr
	opts.PromptLogPath = dir // path is a directory → WriteFile fails

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("expected log failure to be non-fatal, got %v", err)
	}
	if !strings.Contains(stderr.String(), "failed to write prompt log") {
		t.Errorf("expected stderr warning, got %q", stderr.String())
	}
}

func TestDispatch_NoLogWhenPathEmpty(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	claudeFake := &fakeClaude{}
	opts := newOpts() // PromptLogPath is "" by default

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Prepared-prompt summary banner (item 5)
// ---------------------------------------------------------------------------

func TestDispatch_PreparedPromptBannerReflectsOptions(t *testing.T) {
	var buf bytes.Buffer
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	claudeFake := &fakeClaude{}
	opts := newOpts()
	opts.Stdout = &buf
	opts.Agent = "implement-system"
	opts.Architecture = "monolith"
	opts.AllowedRoots = "- System: system/monolith/typescript (lang: typescript)\n- System tests: system-test/typescript (lang: typescript)\n"
	opts.Checklist = "- [x] One done\n- [ ] Two pending"
	opts.PromptLogPath = "/tmp/runs/001-implement-system.prompt.md"
	// implement-system's inlined phase-doc body now references
	// ${sut-namespace}, ${driver-adapter}, ${driver-port}, ${system-test-path};
	// the production dispatcher fills these from cfg.PlaceholderMap(). With
	// no ProjectConfig in this test, supply them directly so renderPrompt
	// has values to substitute.
	opts.Placeholders = map[string]string{
		"sut-namespace":    "shop",
		"system-test-path": "system-test/typescript",
		"driver-port":      "system-test/src/testkit/driver/port/shop",
		"driver-adapter":   "system-test/src/testkit/driver/adapter/shop",
	}

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	got := buf.String()
	mustContain(t, got, "PREPARED PROMPT for implement-system")
	mustContain(t, got, "architecture:")
	mustContain(t, got, "monolith")
	mustContain(t, got, "allowed roots:")
	mustContain(t, got, "2 path(s)")
	mustContain(t, got, "- System: system/monolith/typescript (lang: typescript)")
	mustContain(t, got, "- System tests: system-test/typescript (lang: typescript)")
	mustContain(t, got, "checklist:")
	mustContain(t, got, "2 item(s) (1 already [x])")
	mustContain(t, got, "- [x] One done")
	mustContain(t, got, "- [ ] Two pending")
	mustContain(t, got, "/tmp/runs/001-implement-system.prompt.md")
}

func TestDispatch_PreparedPromptBannerUsesPlaceholdersForEmpties(t *testing.T) {
	// The bug we're guarding against shows up here: when Architecture and
	// AllowedRoots are empty (the seedScopeParams → seedScopeState bug), the
	// banner makes that visible at a glance.
	var buf bytes.Buffer
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	claudeFake := &fakeClaude{}
	opts := newOpts()
	opts.Stdout = &buf
	// Architecture and AllowedRoots default to "".

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	got := buf.String()
	mustContain(t, got, "(empty)")
	mustContain(t, got, "(none)") // override text + log
}

func TestDispatch_PreparedPromptBanner_RawPromptShowsOverrideMode(t *testing.T) {
	var buf bytes.Buffer
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	claudeFake := &fakeClaude{}
	opts := newOpts()
	opts.Stdout = &buf
	opts.RawPrompt = "operator-supplied prompt"

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	got := buf.String()
	mustContain(t, got, "override mode")
	if strings.Contains(got, "architecture:") {
		t.Errorf("override-mode banner must not list introspection fields:\n%s", got)
	}
}

func TestDispatch_ShowPromptDumpsFullPrompt(t *testing.T) {
	var buf bytes.Buffer
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	claudeFake := &fakeClaude{}
	opts := newOpts()
	opts.Stdout = &buf
	opts.ShowPrompt = true

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	got := buf.String()
	mustContain(t, got, "The Acceptance Criteria below were parsed") // the embedded body is dumped
}

func TestDispatch_ShowPromptOffByDefault(t *testing.T) {
	var buf bytes.Buffer
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	claudeFake := &fakeClaude{}
	opts := newOpts()
	opts.Stdout = &buf

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if strings.Contains(buf.String(), "The Acceptance Criteria below were parsed") {
		t.Errorf("--show-prompt off must not dump the prompt body:\n%s", buf.String())
	}
}

func TestSummarizeAllowedRoots(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "(empty)"},
		{"only system", "- System: a (lang: java)\n- System tests: b (lang: java)\n", "2 path(s)"},
		{"with externals",
			"- System: a\n- System tests: b\n\nExternal-system roots (note):\n- Stubs: c\n- Simulators: d\n",
			"2 path(s), 2 external"},
		{"only externals",
			"\nExternal-system roots:\n- Stubs: c\n",
			"0 path(s), 1 external"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarizeAllowedRoots(tt.in); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSummarizeChecklist(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", "(empty)"},
		{"all done", "- [x] One\n- [X] Two", "2 item(s) (2 already [x])"},
		{"mixed", "- [x] One\n- [ ] Two\n- [ ] Three", "3 item(s) (1 already [x])"},
		{"non-checklist text", "Just some prose", "(empty)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarizeChecklist(tt.in); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
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
// Helpers
// ---------------------------------------------------------------------------

func mustContain(t *testing.T, haystack, needle string) {
	t.Helper()
	if !strings.Contains(haystack, needle) {
		t.Fatalf("missing %q in:\n%s", needle, haystack)
	}
}
