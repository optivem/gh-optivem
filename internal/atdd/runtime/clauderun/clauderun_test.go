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
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/assets"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/agents"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/projectconfig"
	"github.com/optivem/gh-optivem/internal/userstate"
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
// branch-switch / dirty-file detection don't have to enumerate them.
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
	// Minimal monolith Java shop-shaped config — production dispatch wires
	// Placeholders from cfg.PlaceholderMap(), so seed both consistently so
	// inline ${key} annotations + ${scope-block} resolve cleanly.
	cfg := &projectconfig.Config{
		System: projectconfig.System{
			Architecture:    projectconfig.ArchMonolith,
			Path:            "system",
			Lang:            "java",
			DbMigrationPath: projectconfig.DefaultDbMigrationPath,
		},
		SystemTest: projectconfig.TierSpec{
			Path:  "system-test",
			Lang:  "java",
			Paths: projectconfig.DefaultPaths(projectconfig.LangJava, "system-test", "shop"),
		},
	}
	return Options{
		Agent:           "acceptance-test-writer",
		NodeDescription: "Write the AT-RED scenario",
		IssueNum:        42,
		IssueTitle:      "Add PUT /carts/{id}/items endpoint",
		// The stripped prompts reference ${language} in the language-
		// equivalents pointer; seed a default so tests don't have to.
		Language: "java",
		// acceptance-test-writer.md references ${acceptance-criteria}; seed
		// a default so dispatch tests using this scaffold render cleanly.
		// Tests that exercise the unset/load-bearing path override this.
		AcceptanceCriteria: "Scenario: placeholder\n  Given x\n  When y\n  Then z",
		// Per-phase scope (plan 20260526-1448 Item 4). Every writing-agent
		// prompt body now references ${scope-block} (load-bearing); seed a
		// minimal acceptance-test-writer-shaped scope so dispatch tests
		// render cleanly. Tests exercising scope-specific behaviour override
		// these.
		ScopeRead:     []string{"at-test", "dsl-port"},
		ScopeWrite:    []string{"at-test", "dsl-port", "dsl-core"},
		ProjectConfig: cfg,
		Placeholders:  cfg.PlaceholderMap(),
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
	mustContain(t, got, "don't summarise")
	mustContain(t, got, "never run `git commit`")
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
		"system-implementer", "system-refactorer",
		"system-driver-adapter-implementer",
		"external-system-driver-adapter-implementer",
		"dsl-implementer",
		"external-system-stub-implementer",
		"acceptance-test-writer", "contract-test-writer",
	} {
		opts := newOpts()
		opts.Agent = name
		// Seed every agent-specific placeholder this loop's iteration
		// might reference. Strict-mode ExpandParams (plan 20260527-0205)
		// rejects unfilled placeholders before render returns; the test's
		// purpose is the legacy-marker scan, so just satisfy the
		// prerequisites:
		//   - ${checklist} — refactor / structural-task agents
		//   - ${touches-system-driver} — dsl-implementer
		opts.Checklist = "- [ ] placeholder checklist item"
		opts.NodeParams = map[string]string{"touches-system-driver": "false"}

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

func TestRenderPrompt_TaskAgentArchitectureAndScopeBlock_ExplicitValues(t *testing.T) {
	opts := newOpts()
	opts.Agent = "system-implementer"
	opts.Architecture = "monolith"
	// Per-phase scope from the BPMN node's read:/write: lists (plan
	// 20260526-1448 Item 4). Production fills these via engine.Scope at
	// dispatch time; for the render test we supply them directly.
	opts.ScopeRead = []string{"system-path"}
	opts.ScopeWrite = []string{"system-path"}
	// ProjectConfig drives the ${scope-block} resolver — it joins each
	// key against cfg.PlaceholderMap(). Seed a minimal monolith config so
	// `system-path` resolves to a concrete path in the rendered block.
	opts.ProjectConfig = &projectconfig.Config{
		System: projectconfig.System{
			Architecture:    projectconfig.ArchMonolith,
			Path:            "system/monolith/java",
			Lang:            "java",
			DbMigrationPath: projectconfig.DefaultDbMigrationPath,
		},
	}
	// implement-system's body references ${checklist}, now load-bearing.
	// Production dispatch fills this from ctx.State["checklist"]
	// (populated by parse-ticket); supply directly so the no-leftover-${...}
	// assertion below doesn't trip.
	opts.Checklist = "- [ ] Refactor X"
	// implement-system now inlines phase-doc placeholders
	// in its body; without these the no-leftover-${...} assertion below
	// would catch the inlined Family B path references. The full canonical
	// key set (CanonicalPathKeys) must appear so the Step 1 read-layer
	// enumeration in system-implementer.md resolves.
	opts.Placeholders = map[string]string{
		"sut-namespace":                  "shop",
		"system-path":                    "system/monolith/java",
		"system-db-migration-path":       projectconfig.DefaultDbMigrationPath,
		"system-test-path":               "system-test/java",
		"driver-port":                    "system-test/src/testkit/driver/port/shop",
		"driver-adapter":                 "system-test/src/testkit/driver/adapter/shop",
		"external-system-driver-port":    "system-test/src/testkit/external/port/shop",
		"external-system-driver-adapter": "system-test/src/testkit/external/adapter/shop",
		"at-test":                        "system-test/src/test/java/shop/latest/acceptance",
		"dsl-port":                       "system-test/src/testkit/dsl/port/shop",
		"dsl-core":                       "system-test/src/testkit/dsl/core/shop",
		"ct-test":                        "system-test/src/test/java/shop/latest/contract",
	}

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	mustContain(t, got, "Architecture: monolith")
	// The ### Scope heading lives in the prompt source (implement-system.md).
	mustContain(t, got, "### Scope")
	mustContain(t, got, "You may **read** files under these paths:")
	mustContain(t, got, "You may **modify** files under these paths:")
	mustContain(t, got, "- `system-path`: system/monolith/java")
	mustContain(t, got, "`scope_exception`")
	// Symmetric read/write — no write \ read entries — so the auto-derived
	// "Write-only paths" annotation must not appear, and with no rationale
	// supplied the "Why:" tail must not appear either.
	if strings.Contains(got, "Write-only paths") {
		t.Errorf("symmetric scope block must not render the write-only annotation:\n%s", got)
	}
	if strings.Contains(got, "Why:") {
		t.Errorf("symmetric scope block with no rationale must not render a Why: tail:\n%s", got)
	}
	if strings.Contains(got, "${") {
		t.Errorf("prompt still contains ${...} placeholder: %s", got)
	}
}

// TestRenderPrompt_ScopeBlock_WriteOnlyWithRationale covers the asymmetric
// scope path (plan 20260528-1038): a writing-agent MID whose `write:` set
// contains keys absent from `read:` (dsl-core is the canonical case for
// the two test-writer MIDs). The dispatcher auto-derives the
// "Write-only paths:" line; the per-MID scope-rationale renders below it
// as a "Why:" tail. Both must appear, alongside the existing scope_exception
// guardrail line.
func TestRenderPrompt_ScopeBlock_WriteOnlyWithRationale(t *testing.T) {
	opts := newOpts()
	// newOpts already seeds acceptance-test-writer + the asymmetric
	// at-test / dsl-port / dsl-core scope shape; only the rationale needs
	// adding here.
	opts.ScopeRationale = "dsl-core is write-only because the test-writer appends `TODO: DSL` stubs there so the project compiles; reading existing dsl-core content would leak implementation context into test design."

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	mustContain(t, got, "Write-only paths (in `write:` but not `read:`): dsl-core")
	mustContain(t, got, "Treat these as append-only or edit-by-location")
	mustContain(t, got, "Why: dsl-core is write-only because")
	mustContain(t, got, "`scope_exception`")
}

func TestRenderPrompt_TaskAgentChecklistInjected(t *testing.T) {
	// update-system is the reshape variant dispatched by
	// redesign-system-structure CYCLE and the agent that consumes
	// ${checklist} on the system side (plan 20260526-1448 Item 10 verb
	// split). The translation-side implement-system no longer carries
	// ${checklist} in its body.
	opts := newOpts()
	opts.Agent = "system-updater"
	opts.Checklist = "- [x] Rename \"New Order\" to \"Place Order\"\n- [x] Rename SKU aria-label"

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	mustContain(t, got, opts.Checklist)
	if strings.Contains(got, "Fetch the issue with `gh`") {
		t.Errorf("update-system prompt should no longer instruct the agent to fetch the issue: %s", got)
	}
	if strings.Contains(got, "${checklist}") {
		t.Errorf("${checklist} placeholder leaked into rendered prompt")
	}
}

func TestRenderPrompt_RefactorSystemAgent_RendersScopeBlock(t *testing.T) {
	// refactor-system's MID-node scope is read=[system-path], write=[system-path]
	// (the broadest single-key writing-agent). Scope rendering replaces the
	// pre-Item-4 ${allowed_roots} mechanism; this test pins the scope_block
	// shape for refactor-system.
	opts := newOpts()
	opts.Agent = "system-refactorer"
	opts.Architecture = "monolith"
	opts.ScopeRead = []string{"system-path"}
	opts.ScopeWrite = []string{"system-path"}
	// refactor-system's body references ${checklist} (load-bearing).
	// Production fills this from ctx.State["checklist"]; supply
	// directly so the no-leftover-${...} assertion below doesn't trip.
	opts.Checklist = "- [ ] Refactor X"
	// The refactor-system prompt inlines phase-doc placeholders that the
	// production dispatcher fills from cfg.PlaceholderMap(); supply them
	// directly so the body renders without ${...} leftovers.
	opts.Placeholders = map[string]string{
		"sut-namespace":    "shop",
		"system-path":      "system",
		"system-test-path": "system-test",
		"driver-port":      "system-test/src/testkit/driver/port/shop",
		"driver-adapter":   "system-test/src/testkit/driver/adapter/shop",
	}

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	mustContain(t, got, "### Scope")
	mustContain(t, got, "- `system-path`: system")
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
// subtype files point at that doc via ${references-root} instead of
// inlining it, so the bodies are now identical except for routing. The
// previous "external subtype includes doctrine inline / system subtype
// excludes it" assertions are obsolete — the doctrine is read on-demand
// from the synced ~/.gh-optivem/references/atdd/architecture/driver-adapter.md
// by both variants. Subtype routing still works via the task-name lookup
// in process-flow.yaml.

// TestRenderPrompt_ReferencesRootSubstitutes covers the placeholder that
// lets prompt bodies reference the per-user synced references root via
// ${references-root}. The substituted value is an absolute path so the
// agent's Read tool resolves it regardless of working directory.
// Uses PromptOverride (which IS expanded, unlike OverrideText) to
// inject a body that references the placeholder.
func TestRenderPrompt_ReferencesRootSubstitutes(t *testing.T) {
	opts := newOpts()
	opts.PromptOverride = "Read ${references-root}/atdd/architecture/dsl-core.md."

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	if strings.Contains(got, "${references-root}") {
		t.Errorf("expected ${references-root} substituted; got: %q", got)
	}
	// references_root substitutes to an OS-native path (Windows uses
	// backslashes); the literal trailing "/atdd/..." in the template
	// stays as-is. Assert the two halves independently so the test
	// is platform-portable.
	if !strings.Contains(got, filepath.Join(".gh-optivem", "references")) {
		t.Errorf("expected ${references-root} resolved to ~/.gh-optivem/references prefix; got: %q", got)
	}
	if !strings.Contains(got, "/atdd/architecture/dsl-core.md") {
		t.Errorf("expected literal suffix preserved; got: %q", got)
	}
}

// TestDispatch_MaterializesProjectReferencesWhenProjectConfigSet covers the
// project-local-references wiring: when Options.ProjectConfig and RepoPath
// are both set, Dispatch calls MaterializeProject before render and the
// rendered prompt's ${references-root} resolves to
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
	opts.PromptOverride = "Read ${references-root}/atdd/architecture/system.md."

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(claudeFake.calls) != 1 {
		t.Fatalf("expected 1 claude call, got %d", len(claudeFake.calls))
	}
	prompt := claudeFake.calls[0].Prompt
	wantPrefix := filepath.Join(repoPath, ".gh-optivem", "references")
	if !strings.Contains(prompt, wantPrefix) {
		t.Errorf("expected ${references-root} resolved to %q in prompt; got: %q", wantPrefix, prompt)
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
// ${references-root} resolved against assetsync.ReferencesRoot() and no
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
	// ProjectConfig must be nil to exercise the legacy fallback path.
	// newOpts() seeds it for the common scope_block case — clear here.
	opts.ProjectConfig = nil
	opts.PromptOverride = "Read ${references-root}/atdd/architecture/system.md."

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
	opts.PromptOverride = "Read ${references-root}/code/language-equivalents/${language}.md."

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
// ${acceptance-criteria} placeholder that lets write-acceptance-tests consume
// the scenarios intake parsed from the ticket body without re-fetching the
// issue via `gh issue view`.
func TestRenderPrompt_AcceptanceCriteriaSubstitutes(t *testing.T) {
	opts := newOpts()
	opts.AcceptanceCriteria = "Scenario: View product list\n  Given the catalog has products\n  When I open the product list page\n  Then I see them"
	opts.PromptOverride = "Scenarios:\n${acceptance-criteria}"

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	if strings.Contains(got, "${acceptance-criteria}") {
		t.Errorf("expected ${acceptance-criteria} substituted; got: %q", got)
	}
	if !strings.Contains(got, "View product list") {
		t.Errorf("expected scenario body in rendered prompt; got: %q", got)
	}
}

// TestRenderPrompt_UnsetAcceptanceCriteriaFailsFast pins the load-bearing
// contract: a prompt that references ${acceptance-criteria} with no value
// set produces a clear render-time error rather than silently substituting
// an empty block. AT - RED - TEST is contractually scoped to implementing
// AC, so an absent value is a wiring bug, not a valid state.
func TestRenderPrompt_UnsetAcceptanceCriteriaFailsFast(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	opts := newOpts()
	opts.AcceptanceCriteria = "" // override the test default
	opts.PromptOverride = "You are the Test Agent. Scenarios:\n${acceptance-criteria}"
	_, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts)
	if err == nil {
		t.Fatalf("expected error for unset ${acceptance-criteria}, got nil")
	}
	if !strings.Contains(err.Error(), "acceptance-criteria") {
		t.Errorf("expected error to name 'acceptance-criteria'; got: %v", err)
	}
}

// TestRenderPrompt_UnsetChecklistFailsFast pins the load-bearing
// contract on ${checklist}: a prompt that references ${checklist} with
// no value set produces a clear render-time error rather than silently
// substituting an empty block. The four Checklist-using ticket-kinds
// (system-redesign, external-system-redesign, system-refactor,
// test-refactor) all dispatch prompts that
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
	// newOpts() seeds Placeholders from cfg.PlaceholderMap(), which
	// includes "language" — clear it so the unfilled-placeholder check
	// has a real ${language} to catch.
	delete(opts.Placeholders, "language")
	// Use a node_replacements-style PromptOverride that references
	// ${language} without setting Language. Dispatch's
	// findUnfilledPlaceholders catches the leftover.
	opts.PromptOverride = "You are the Test Agent. Read ${references-root}/code/language-equivalents/${language}.md."
	_, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts)
	if err == nil {
		t.Fatalf("expected error for unset ${language}, got nil")
	}
	if !strings.Contains(err.Error(), "language") {
		t.Errorf("expected error to name 'language'; got: %v", err)
	}
}

// TestRenderPrompt_InteractiveSuffixAppendedWhenNotHeadless pins the
// operator-facing terminal hint added in plan 20260528-0959 — interactive
// dispatches keep the Claude Code REPL open after the agent finishes, so
// the suffix tells new operators to `/exit` (close) or type feedback to
// redirect. Headless dispatches have no REPL, so the suffix would only
// waste tokens.
func TestRenderPrompt_InteractiveSuffixAppendedWhenNotHeadless(t *testing.T) {
	opts := newOpts()
	opts.Headless = false

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	mustContain(t, got, "type `/exit` to close the session")
	if !strings.HasSuffix(strings.TrimRight(got, "\n"), "the agent will incorporate it.") {
		t.Errorf("interactive suffix should be at the end of the prompt; tail was:\n%s",
			tailLines(got, 5))
	}
}

func TestRenderPrompt_InteractiveSuffixOmittedWhenHeadless(t *testing.T) {
	opts := newOpts()
	opts.Headless = true

	got, err := renderPrompt(opts)
	if err != nil {
		t.Fatalf("renderPrompt: %v", err)
	}
	if strings.Contains(got, "type `/exit` to close the session") {
		t.Errorf("headless prompt must not include the operator /exit suffix:\n%s", got)
	}
}

// TestRenderPrompt_ReEntryPolicySubstitutes pins the shared
// ${re-entry-policy} substitution dedup'd from the three writing-
// implementer prompts (plan 20260528-1045). The constant
// rendererReEntryPolicy carries the "if the previous WRITE didn't
// compile, fix the broken/missing piece minimally" clause; each
// implementer prompt references it once and follows with a per-agent
// appendix. If someone deletes the constant or unregisters the
// substitution, this test fires.
func TestRenderPrompt_ReEntryPolicySubstitutes(t *testing.T) {
	agentsToCheck := []string{
		"dsl-implementer",
		"system-driver-adapter-implementer",
		"external-system-driver-adapter-implementer",
	}
	for _, agent := range agentsToCheck {
		t.Run(agent, func(t *testing.T) {
			opts := newOpts()
			opts.Agent = agent

			got, err := renderPrompt(opts)
			if err != nil {
				t.Fatalf("renderPrompt(%s): %v", agent, err)
			}
			if strings.Contains(got, "${re-entry-policy}") {
				t.Errorf("%s: ${re-entry-policy} survived in rendered prompt", agent)
			}
			if !strings.Contains(got, "If your previous WRITE didn't compile") {
				t.Errorf("%s: rendered prompt missing re-entry-policy clause; got:\n%s", agent, got)
			}
		})
	}
}

// tailLines returns the last n non-blank-trimmed lines of s, joined with
// "\n", for compact assertion failure output.
func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) <= n {
		return strings.Join(lines, "\n")
	}
	return strings.Join(lines[len(lines)-n:], "\n")
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

func TestDispatch_HeadlessFlagPropagates(t *testing.T) {
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	claudeFake := &fakeClaude{}

	opts := newOpts()
	opts.Headless = true

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	got := claudeFake.calls[0]
	if !got.Headless {
		t.Errorf("Headless: got false, want true")
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
	mustContain(t, got, "[agent]  enter")
	mustContain(t, got, "acceptance-test-writer")
	mustContain(t, got, "[agent]  exit")
	mustContain(t, got, "1 files")
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
	mustContain(t, got, "[agent]  exit   no changes")
	if strings.Contains(got, "committed") {
		t.Errorf("no-op banner must not say 'committed': %s", got)
	}
}

// TestDispatch_EnterBannerListsLogPaths pins the headless inspection
// contract: the [agent] enter banner surfaces each per-dispatch log
// path (prompt, events, outputs) the operator has no other lever to
// discover, but only when that path is actually populated. A
// no-outputs MID, an interactive dispatch with no event stream, or a
// test path that sets none must produce no log lines at all.
func TestDispatch_EnterBannerListsLogPaths(t *testing.T) {
	t.Run("all four paths set → all four lines shown", func(t *testing.T) {
		var buf bytes.Buffer
		gitFake := &fakeGit{
			out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
		}
		opts := newOpts()
		opts.Stdout = &buf
		opts.PromptLogPath = "/tmp/run-1/prompt.md"
		opts.EventsLogPath = "/tmp/run-1/events.ndjson"
		opts.EventsTextLogPath = "/tmp/run-1/events.log"
		opts.OutputFilePath = "/tmp/run-1/outputs.jsonl"

		if _, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts); err != nil {
			t.Fatalf("Dispatch: %v", err)
		}
		got := buf.String()
		mustContain(t, got, "Prompt log:  /tmp/run-1/prompt.md")
		mustContain(t, got, "Events log:  /tmp/run-1/events.ndjson")
		mustContain(t, got, "Events text: /tmp/run-1/events.log")
		mustContain(t, got, "Outputs log: /tmp/run-1/outputs.jsonl")
	})

	t.Run("only prompt set → only prompt line shown", func(t *testing.T) {
		var buf bytes.Buffer
		gitFake := &fakeGit{
			out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
		}
		opts := newOpts()
		opts.Stdout = &buf
		opts.PromptLogPath = "/tmp/run-2/prompt.md"

		if _, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts); err != nil {
			t.Fatalf("Dispatch: %v", err)
		}
		got := buf.String()
		mustContain(t, got, "Prompt log:  /tmp/run-2/prompt.md")
		if strings.Contains(got, "Events log:") {
			t.Errorf("Events log line must be omitted when EventsLogPath is empty: %s", got)
		}
		if strings.Contains(got, "Outputs log:") {
			t.Errorf("Outputs log line must be omitted when OutputFilePath is empty: %s", got)
		}
	})

	t.Run("no paths set → no log lines shown", func(t *testing.T) {
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
		for _, label := range []string{"Prompt log:", "Events log:", "Outputs log:"} {
			if strings.Contains(got, label) {
				t.Errorf("no log lines should be emitted when all paths are empty, got %q in %s", label, got)
			}
		}
	})
}

// TestDispatch_EnterBannerSurfacesTuning pins the headless inspection
// contract for --model / --effort: the [agent] enter banner must show
// the tuning the dispatcher passed through, including an explicit
// "(claude session default)" when neither is set, so the operator can
// never confuse silent inheritance with an applied override.
func TestDispatch_EnterBannerSurfacesTuning(t *testing.T) {
	cases := []struct {
		name   string
		model  string
		effort string
		want   string
	}{
		{"both set", "sonnet", "high", "Tuning: model=sonnet, effort=high"},
		{"only model", "haiku", "", "Tuning: model=haiku"},
		{"only effort", "", "low", "Tuning: effort=low"},
		{"neither set", "", "", "Tuning: (claude session default)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			gitFake := &fakeGit{
				out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
			}
			opts := newOpts()
			opts.Stdout = &buf
			opts.Model = tc.model
			opts.Effort = tc.effort

			if _, err := Dispatch(context.Background(), Deps{Claude: &fakeClaude{}, Git: gitFake}, opts); err != nil {
				t.Fatalf("Dispatch: %v", err)
			}
			mustContain(t, buf.String(), tc.want)
		})
	}
}

// ---------------------------------------------------------------------------
// Token usage parsing & banner formatting
// ---------------------------------------------------------------------------

func TestParseClaudeStreamJSON_ExtractsUsageAndResultFromTerminalEvent(t *testing.T) {
	// Multi-line shape from `claude -p --output-format stream-json --verbose`:
	// leading system/assistant/user events, terminating in a `type:"result"`
	// line. parseClaudeStreamJSON should ignore everything up to the result
	// line, then decode usage + result + cost from it.
	stream := strings.Join([]string{
		`{"type":"system","subtype":"init","session_id":"abc"}`,
		`{"type":"assistant","message":{"id":"msg_01","content":[{"type":"text","text":"thinking..."}]}}`,
		`{"type":"user","message":{"id":"msg_02","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"ok"}]}}`,
		`{"type":"result","subtype":"success","is_error":false,"result":"Hi there friend","total_cost_usd":0.17759875,"usage":{"input_tokens":6,"cache_creation_input_tokens":28307,"cache_read_input_tokens":0,"output_tokens":10}}`,
	}, "\n") + "\n"

	usage, result := parseClaudeStreamJSON([]byte(stream))
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

func TestParseClaudeStreamJSON_SkipsMalformedMidStreamLines(t *testing.T) {
	// A garbage line in the middle of the stream must not prevent the
	// terminal result event from being parsed — a single CLI hiccup
	// shouldn't poison the whole audit. The raw bytes still land in the
	// events log; the parser just keeps walking until it hits result.
	stream := strings.Join([]string{
		`{"type":"system","subtype":"init"}`,
		`not-json-at-all`,
		`{"type":"assistant","message":{"id":"msg_01"}}`,
		`{"type":"result","subtype":"success","result":"final","total_cost_usd":0.05,"usage":{"input_tokens":3,"output_tokens":4}}`,
	}, "\n") + "\n"

	usage, result := parseClaudeStreamJSON([]byte(stream))
	if usage == nil {
		t.Fatalf("expected non-nil usage despite mid-stream garbage")
	}
	if result != "final" {
		t.Errorf("result: got %q, want %q", result, "final")
	}
	if usage.InputTokens != 3 || usage.OutputTokens != 4 || usage.TotalCostUSD != 0.05 {
		t.Errorf("usage fields wrong: %+v", *usage)
	}
}

func TestParseClaudeStreamJSON_GracefulOnEmptyOrNoResultEvent(t *testing.T) {
	// Empty stream (CLI died before emitting anything) → zero values; the
	// surrounding non-zero-exit error path surfaces the failure.
	usage, result := parseClaudeStreamJSON(nil)
	if usage != nil || result != "" {
		t.Errorf("expected (nil, \"\") on empty input, got (%+v, %q)", usage, result)
	}

	usage, result = parseClaudeStreamJSON([]byte("claude: command failed\n"))
	if usage != nil || result != "" {
		t.Errorf("expected (nil, \"\") on non-JSON input, got (%+v, %q)", usage, result)
	}

	// Stream with assistant/system events but no terminal result line —
	// e.g. CLI killed mid-run. Same fallback.
	stream := `{"type":"system","subtype":"init"}` + "\n" + `{"type":"assistant","message":{"id":"x"}}` + "\n"
	usage, result = parseClaudeStreamJSON([]byte(stream))
	if usage != nil || result != "" {
		t.Errorf("expected (nil, \"\") when no result event present, got (%+v, %q)", usage, result)
	}
}

// ---------------------------------------------------------------------------
// openEventsLog — the per-dispatch stream-json audit log helper that
// runHeadless tees stdout into. Extracted from runHeadless so the
// path-handling and error-recovery branches are unit-testable without
// spinning up a real `claude` subprocess.
// ---------------------------------------------------------------------------

func TestOpenEventsLog_WritesStreamToFileWhenPathSet(t *testing.T) {
	dir := t.TempDir()
	// Nested path: helper must mkdir parents (mirrors the runState layout
	// .gh-optivem/runs/<ts>/<seq>-<agent>.events.jsonl).
	path := filepath.Join(dir, "runs", "20260528-1045", "001-system-implementer.events.jsonl")
	var stderr bytes.Buffer

	w, closeFn := openEventsLog(path, &stderr)
	if _, err := io.WriteString(w, `{"type":"system"}`+"\n"+`{"type":"result","result":"done"}`+"\n"); err != nil {
		t.Fatalf("write events: %v", err)
	}
	closeFn()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events log: %v", err)
	}
	if !strings.Contains(string(body), `"type":"result"`) {
		t.Errorf("events log missing result event: %q", string(body))
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr warning on success path, got: %s", stderr.String())
	}
}

func TestOpenEventsLog_EmptyPathReturnsDiscardSink(t *testing.T) {
	var stderr bytes.Buffer
	w, closeFn := openEventsLog("", &stderr)
	defer closeFn()

	if w != io.Discard {
		t.Errorf("empty path must return io.Discard sink")
	}
	if stderr.Len() != 0 {
		t.Errorf("empty path must not emit a warning, got: %s", stderr.String())
	}
}

func TestOpenEventsLog_OpenFailureWarnsAndReturnsDiscardSink(t *testing.T) {
	// Force open failure by pointing the path at a child of an existing
	// regular file — MkdirAll fails because the parent isn't a directory.
	// Cross-platform: works on both Windows and POSIX without root.
	tmp := t.TempDir()
	regularFile := filepath.Join(tmp, "blocking-file")
	if err := os.WriteFile(regularFile, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	badPath := filepath.Join(regularFile, "nested", "events.jsonl")

	var stderr bytes.Buffer
	w, closeFn := openEventsLog(badPath, &stderr)
	defer closeFn()

	// Diagnostics must downgrade to a stderr warning, never break the
	// dispatch — same policy as writePromptLog. Writes still succeed
	// (silently) because the sink is io.Discard.
	if _, err := io.WriteString(w, "doesn't matter\n"); err != nil {
		t.Errorf("writes against the fallback sink must not error, got %v", err)
	}
	if stderr.Len() == 0 {
		t.Errorf("expected stderr warning when open fails")
	}
	if !strings.Contains(stderr.String(), "events log") {
		t.Errorf("warning should mention 'events log', got: %s", stderr.String())
	}
}

// ---------------------------------------------------------------------------
// openEventsTextLog + streamTextWriter — the human-readable sibling of
// openEventsLog. runHeadless tees the same stream-json stdout through
// this writer so operators get a transcript-style .events.log next to
// the raw .events.jsonl.
// ---------------------------------------------------------------------------

func TestOpenEventsTextLog_RendersAssistantTextToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs", "20260528-1400", "001-system-implementer.events.log")
	var stderr bytes.Buffer

	w, closeFn := openEventsTextLog(path, &stderr)
	stream := `{"type":"system","subtype":"init"}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"hello operator"}]}}` + "\n" +
		`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Read","input":{"file_path":"/tmp/x.go"}}]}}` + "\n" +
		`{"type":"result","result":"done"}` + "\n"
	if _, err := io.WriteString(w, stream); err != nil {
		t.Fatalf("write events: %v", err)
	}
	closeFn()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events text log: %v", err)
	}
	got := string(body)
	for _, want := range []string{
		"session started",
		"hello operator",
		"[tool Read] file_path=/tmp/x.go",
		"─── result ───",
		"done",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("events text log missing %q, got:\n%s", want, got)
		}
	}
	// No raw JSON should leak through for well-formed events.
	if strings.Contains(got, `"type":"assistant"`) {
		t.Errorf("events text log should not contain raw JSON, got:\n%s", got)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr warning on success path, got: %s", stderr.String())
	}
}

func TestOpenEventsTextLog_EmptyPathReturnsDiscardSink(t *testing.T) {
	var stderr bytes.Buffer
	w, closeFn := openEventsTextLog("", &stderr)
	defer closeFn()

	if w != io.Discard {
		t.Errorf("empty path must return io.Discard sink")
	}
	if stderr.Len() != 0 {
		t.Errorf("empty path must not emit a warning, got: %s", stderr.String())
	}
}

func TestOpenEventsTextLog_OpenFailureWarnsAndReturnsDiscardSink(t *testing.T) {
	tmp := t.TempDir()
	regularFile := filepath.Join(tmp, "blocking-file")
	if err := os.WriteFile(regularFile, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	badPath := filepath.Join(regularFile, "nested", "events.log")

	var stderr bytes.Buffer
	w, closeFn := openEventsTextLog(badPath, &stderr)
	defer closeFn()

	if _, err := io.WriteString(w, "doesn't matter\n"); err != nil {
		t.Errorf("writes against the fallback sink must not error, got %v", err)
	}
	if stderr.Len() == 0 {
		t.Errorf("expected stderr warning when open fails")
	}
	if !strings.Contains(stderr.String(), "events text log") {
		t.Errorf("warning should mention 'events text log', got: %s", stderr.String())
	}
}

// TestStreamTextWriter_HandlesSplitChunks pins the partial-line
// buffering contract: the OS pipe can split a JSON event mid-line, and
// the writer must accumulate bytes until it sees the next newline.
func TestStreamTextWriter_HandlesSplitChunks(t *testing.T) {
	var out bytes.Buffer
	tw := &streamTextWriter{w: &out}

	full := `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"split me"}]}}` + "\n"
	split := len(full) / 2
	if _, err := tw.Write([]byte(full[:split])); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("must buffer until newline, got premature flush: %q", out.String())
	}
	if _, err := tw.Write([]byte(full[split:])); err != nil {
		t.Fatalf("second write: %v", err)
	}
	tw.Flush()
	if !strings.Contains(out.String(), "split me") {
		t.Errorf("expected combined output to contain 'split me', got: %q", out.String())
	}
}

func TestStreamTextWriter_MalformedLineFallsBackToRawPassthrough(t *testing.T) {
	var out bytes.Buffer
	tw := &streamTextWriter{w: &out}
	if _, err := tw.Write([]byte("this is not json\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.Contains(out.String(), "this is not json") {
		t.Errorf("malformed line should pass through raw, got: %q", out.String())
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
	mustContain(t, got, "[agent]  exit   7 files")
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
	mustContain(t, got, "[agent]  exit")
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
// Repo snapshot (branch-switch detection, dirty-file delta)
// ---------------------------------------------------------------------------

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
	if !strings.Contains(err.Error(), "placeholder") {
		t.Errorf("error should mention an unresolved placeholder, got %q", err.Error())
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
	// ${command} / ${command-exit-code} / ${command-stderr-tail} is
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
	opts.PromptOverride = "Diagnose the failure of: ${command}\nexit=${command-exit-code}\nstderr=${command-stderr-tail}"
	// CommandLine intentionally left empty.

	_, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts)
	if err == nil {
		t.Fatalf("expected error when ${command} is unfilled, got nil")
	}
	if !strings.Contains(err.Error(), "placeholder") {
		t.Errorf("error should mention an unresolved placeholder, got %q", err.Error())
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
	// Run headless so the interactive operator-suffix doesn't append —
	// this test asserts exact prompt equality on the substitution path,
	// not the suffix behaviour (covered separately by
	// TestRenderPrompt_InteractiveSuffix*).
	opts.Headless = true
	opts.PromptOverride = "cmd=${command} exit=${command-exit-code} tail=${command-stderr-tail}"
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
	opts.Agent = "system-implementer"
	opts.Architecture = "monolith"
	opts.ScopeRead = []string{"system-path"}
	opts.ScopeWrite = []string{"system-path"}
	opts.ProjectConfig = &projectconfig.Config{
		System: projectconfig.System{
			Architecture:    projectconfig.ArchMonolith,
			Path:            "system/monolith/typescript",
			Lang:            "typescript",
			DbMigrationPath: projectconfig.DefaultDbMigrationPath,
		},
	}
	opts.Checklist = "- [x] One done\n- [ ] Two pending"
	opts.PromptLogPath = "/tmp/runs/001-system-implementer.prompt.md"
	// implement-system's inlined phase-doc body now references the full
	// CanonicalPathKeys set (driver-{port,adapter}, external-system-driver-
	// {port,adapter}, at-test, dsl-{port,core}, ct-test) plus sut-namespace
	// / system-test-path. The production dispatcher fills these from
	// cfg.PlaceholderMap(); with no SystemTest.Paths in this test's
	// ProjectConfig, supply them directly so renderPrompt has values to
	// substitute.
	opts.Placeholders = map[string]string{
		"sut-namespace":                  "shop",
		"system-path":                    "system/monolith/typescript",
		"system-db-migration-path":       projectconfig.DefaultDbMigrationPath,
		"system-test-path":               "system-test/typescript",
		"driver-port":                    "system-test/src/testkit/driver/port/shop",
		"driver-adapter":                 "system-test/src/testkit/driver/adapter/shop",
		"external-system-driver-port":    "system-test/src/testkit/external/port/shop",
		"external-system-driver-adapter": "system-test/src/testkit/external/adapter/shop",
		"at-test":                        "system-test/tests/latest/acceptance",
		"dsl-port":                       "system-test/src/testkit/dsl/port/shop",
		"dsl-core":                       "system-test/src/testkit/dsl/core/shop",
		"ct-test":                        "system-test/tests/latest/contract",
	}

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	got := buf.String()
	mustContain(t, got, "[agent]  prep   system-implementer")
	mustContain(t, got, "architecture:")
	mustContain(t, got, "monolith")
	mustContain(t, got, "scope:")
	mustContain(t, got, "1 read / 1 write")
	mustContain(t, got, "checklist:")
	mustContain(t, got, "2 item(s) (1 already [x])")
	mustContain(t, got, "- [x] One done")
	mustContain(t, got, "- [ ] Two pending")
	mustContain(t, got, "/tmp/runs/001-system-implementer.prompt.md")
}

func TestDispatch_PreparedPromptBannerUsesPlaceholdersForEmpties(t *testing.T) {
	// The bug we're guarding against shows up here: when Architecture and
	// scope are empty (the seedScopeParams → seedScopeState bug, or a phase
	// with no read:/write: lists), the banner makes that visible at a glance.
	var buf bytes.Buffer
	gitFake := &fakeGit{
		out: [][]byte{[]byte("aaaa\n"), []byte("aaaa\n")},
	}
	claudeFake := &fakeClaude{}
	opts := newOpts()
	opts.Stdout = &buf
	// Architecture defaults to ""; scope is unset by zeroing the slices
	// so the banner exercises the empty path.
	opts.Architecture = ""
	opts.ScopeRead = nil
	opts.ScopeWrite = nil
	// PromptOverride avoids the load-bearing-${scope-block} failure mode
	// — the real write-acceptance-tests body now references ${scope-block},
	// which would fail dispatch with empty scope. This test only cares
	// about the banner output for unset fields.
	opts.PromptOverride = "Synthetic body — no scope references."

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

func TestSummarizeScope(t *testing.T) {
	tests := []struct {
		name      string
		read      []string
		writeKeys []string
		want      string
	}{
		{"empty", nil, nil, "(empty)"},
		{"read-only", []string{"system-path"}, nil, "1 read / 0 write"},
		{"symmetric", []string{"at-test", "dsl-port"}, []string{"at-test", "dsl-port", "dsl-core"}, "2 read / 3 write"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarizeScope(tt.read, tt.writeKeys); got != tt.want {
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

// ---------------------------------------------------------------------------
// Output channel env wiring (plan 20260526-2118)
// ---------------------------------------------------------------------------

func TestRenderExpectedOutputs_EmptySpec_ReturnsEmpty(t *testing.T) {
	if got := renderExpectedOutputs(nil); got != "" {
		t.Errorf("nil spec: got %q, want empty", got)
	}
	if got := renderExpectedOutputs([]statemachine.OutputSpec{}); got != "" {
		t.Errorf("empty spec: got %q, want empty", got)
	}
}

func TestRenderExpectedOutputs_AllRequired_NoOptionalBlock(t *testing.T) {
	got := renderExpectedOutputs([]statemachine.OutputSpec{
		{Key: "dsl-port-changed", Type: "bool"},
		{Key: "system-driver-port-changed", Type: "bool"},
	})
	mustContain(t, got, "Required outputs:")
	mustContain(t, got, "  dsl-port-changed: bool")
	mustContain(t, got, "  system-driver-port-changed: bool")
	if strings.Contains(got, "Optional outputs:") {
		t.Errorf("Optional outputs block should be omitted when no optional keys:\n%s", got)
	}
	mustContain(t, got, "Emit: gh optivem output write KEY=VAL")
}

func TestRenderExpectedOutputs_AllOptional_NoRequiredBlock(t *testing.T) {
	got := renderExpectedOutputs([]statemachine.OutputSpec{
		{Key: "test-names", Type: "string-list", Optional: true},
	})
	if strings.Contains(got, "Required outputs:") {
		t.Errorf("Required outputs block should be omitted when no required keys:\n%s", got)
	}
	mustContain(t, got, "Optional outputs:")
	mustContain(t, got, "  test-names: string-list")
	mustContain(t, got, "Emit: gh optivem output write KEY=VAL")
}

func TestRenderExpectedOutputs_MixedRequiredAndOptional(t *testing.T) {
	got := renderExpectedOutputs([]statemachine.OutputSpec{
		{Key: "dsl-port-changed", Type: "bool"},
		{Key: "test-names", Type: "string-list", Optional: true},
		{Key: "scope-exception-reason", Type: "string", Optional: true},
	})
	mustContain(t, got, "Required outputs:\n  dsl-port-changed: bool")
	mustContain(t, got, "Optional outputs:\n  test-names: string-list\n  scope-exception-reason: string")
	mustContain(t, got, "Emit: gh optivem output write KEY=VAL")
	// Required must precede Optional.
	if strings.Index(got, "Required outputs:") > strings.Index(got, "Optional outputs:") {
		t.Errorf("Required must come before Optional:\n%s", got)
	}
}

func TestRenderPrompt_ExpectedOutputsSubstitutes(t *testing.T) {
	opts := newOpts()
	opts.PromptOverride = "Body. ${expected-outputs}"
	opts.ExpectedOutputs = []statemachine.OutputSpec{
		{Key: "dsl-port-changed", Type: "bool"},
		{Key: "test-names", Type: "string-list", Optional: true},
	}
	got, err := RenderPrompt(opts)
	if err != nil {
		t.Fatalf("RenderPrompt: %v", err)
	}
	mustContain(t, got, "Required outputs:")
	mustContain(t, got, "Optional outputs:")
	mustContain(t, got, "dsl-port-changed: bool")
	mustContain(t, got, "test-names: string-list")
}

func TestOutputChannelEnv_BothUnset_NilEnv(t *testing.T) {
	got := subprocessEnv(RunOpts{})
	if got != nil {
		t.Errorf("want nil (inherit parent env), got %v", got)
	}
}

func TestOutputChannelEnv_BothSet_AppendedToParentEnv(t *testing.T) {
	got := subprocessEnv(RunOpts{
		OutputFilePath: "/tmp/agent-001.outputs.jsonl",
		OutputKeysSpec: "dsl-port-changed:bool,test-names:string-list",
	})
	if got == nil {
		t.Fatalf("want env slice, got nil")
	}
	// Parent env should be present (PATH or similar) plus our two
	// trailing GH_OPTIVEM_OUTPUT_* entries. We don't assert the parent
	// shape — just the trailing entries.
	last := got[len(got)-2:]
	wantFile := "GH_OPTIVEM_OUTPUT_FILE=/tmp/agent-001.outputs.jsonl"
	wantKeys := "GH_OPTIVEM_OUTPUT_KEYS=dsl-port-changed:bool,test-names:string-list"
	if last[0] != wantFile {
		t.Errorf("got[-2] = %q, want %q", last[0], wantFile)
	}
	if last[1] != wantKeys {
		t.Errorf("got[-1] = %q, want %q", last[1], wantKeys)
	}
}

func TestOutputChannelEnv_OnlyFileSet_ExportsFileOnly(t *testing.T) {
	got := subprocessEnv(RunOpts{OutputFilePath: "/tmp/x.jsonl"})
	if got == nil {
		t.Fatalf("want env slice, got nil")
	}
	if got[len(got)-1] != "GH_OPTIVEM_OUTPUT_FILE=/tmp/x.jsonl" {
		t.Errorf("got[-1] = %q, want file export", got[len(got)-1])
	}
	for _, e := range got {
		if strings.HasPrefix(e, "GH_OPTIVEM_OUTPUT_KEYS=") {
			t.Errorf("OUTPUT_KEYS unexpectedly set: %q", e)
		}
	}
}

func TestDispatch_ForwardsOutputChannelToRunner(t *testing.T) {
	claudeFake := &fakeClaude{}
	gitFake := &fakeGit{
		out:       [][]byte{[]byte("abc123"), []byte("abc123")},
		branchPre: "main", branchPost: "main",
	}
	opts := newOpts()
	opts.OutputFilePath = "/tmp/runs/001-acceptance-test-writer.outputs.jsonl"
	opts.OutputKeysSpec = "dsl-port-changed:bool,test-names:string-list"

	if _, err := Dispatch(context.Background(), Deps{Claude: claudeFake, Git: gitFake}, opts); err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if len(claudeFake.calls) != 1 {
		t.Fatalf("want 1 runner call, got %d", len(claudeFake.calls))
	}
	got := claudeFake.calls[0]
	if got.OutputFilePath != opts.OutputFilePath {
		t.Errorf("OutputFilePath: runner saw %q, want %q", got.OutputFilePath, opts.OutputFilePath)
	}
	if got.OutputKeysSpec != opts.OutputKeysSpec {
		t.Errorf("OutputKeysSpec: runner saw %q, want %q", got.OutputKeysSpec, opts.OutputKeysSpec)
	}
}

// snakePlaceholderRe matches a `${foo_bar}` shaped token — underscore
// as word separator. Single-word names (${language}, ${command},
// ${phase}, ${checklist}) deliberately do not match: with no separator
// there is no kebab-vs-snake convention question. Allows trailing
// underscore-separated segments (${foo_bar_baz}) and digits inside
// segments are tolerated by the broader pattern but kept ASCII-lowercase
// to mirror the project convention.
var snakePlaceholderRe = regexp.MustCompile(`\$\{[a-z]+_[a-z]+[a-z_]*\}`)

// TestNoSnakeCasePlaceholdersInPromptBodies walks every embedded prompt
// asset under internal/assets/runtime/{agents,shared} and fails if any
// snake-cased `${foo_bar}` placeholder survives. Path B aligned the
// renderer registry and prompt bodies on kebab; this test is the drift
// alarm that prevents a future agent file from sneaking a snake
// placeholder back in. Walks runtime/shared/ in addition to
// runtime/agents/atdd/ because the shared preamble is concatenated to
// every dispatched prompt — a snake placeholder there has the same
// load-bearing failure mode as one in an agent body.
func TestNoSnakeCasePlaceholdersInPromptBodies(t *testing.T) {
	roots := []string{"runtime/agents/atdd", "runtime/shared"}
	var offenders []string
	for _, root := range roots {
		err := fs.WalkDir(assets.FS, root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || !strings.HasSuffix(path, ".md") {
				return nil
			}
			data, readErr := assets.FS.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for _, m := range snakePlaceholderRe.FindAllString(string(data), -1) {
				offenders = append(offenders, path+": "+m)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("snake-cased placeholders survive in embedded prompts; rename to kebab:\n  %s",
			strings.Join(offenders, "\n  "))
	}
}

// TestRenderPrompt_NoSnakePlaceholdersInRenderedOutput renders the
// canonical prompts for a writer/fixer/refactorer triple and asserts
// no `${foo_bar}` token survives in the rendered output. Complements
// TestNoSnakeCasePlaceholdersInPromptBodies: that test pins the .md
// source, this one pins the renderer's substitution layer — catches a
// regression where the renderer registers a kebab key but the prompt
// body (post-rendering, including any inline shared content) still
// names it with snake.
func TestRenderPrompt_NoSnakePlaceholdersInRenderedOutput(t *testing.T) {
	for _, agent := range []string{
		"acceptance-test-writer",  // writer
		"command-failed-fixer",    // fixer
		"system-refactorer",       // refactorer
	} {
		t.Run(agent, func(t *testing.T) {
			if _, err := agents.Prompt(agent); err != nil {
				t.Skipf("agent %q not embedded: %v", agent, err)
			}
			opts := newOpts()
			opts.Agent = agent
			// Seed every load-bearing field so renderPrompt resolves
			// cleanly across all three agent shapes.
			opts.AcceptanceCriteria = "Scenario: x\n  Given a\n  When b\n  Then c"
			opts.Checklist = "- [ ] item"
			opts.ParsedConcepts = "/tmp/parsed-concepts.md"
			opts.VerifyResults = "results"
			opts.ChangedFiles = "M foo.go"
			opts.CommandLine = "go test ./..."
			opts.CommandExitCode = 1
			opts.CommandStderrTail = "stderr tail"
			opts.TicketID = "42"
			rendered, err := renderPrompt(opts)
			if err != nil {
				t.Fatalf("renderPrompt(%s): %v", agent, err)
			}
			if matches := snakePlaceholderRe.FindAllString(rendered, -1); len(matches) > 0 {
				t.Fatalf("snake-cased placeholders survive in rendered prompt for %q: %v", agent, matches)
			}
		})
	}
}

// TestRenderGateMarkerExample pins the per-language WIP-gate snippet
// rendered into the acceptance-test-writer prompt. The gate is
// permanent and ticket-independent: it keys on the
// GH_OPTIVEM_RUN_WIP_TESTS env var, so the snippet carries no
// per-ticket reason string. Regression guard: the rendered output must
// NOT contain the old per-ticket `#<id>`-style disable markers.
func TestRenderGateMarkerExample(t *testing.T) {
	cases := []struct {
		lang        string
		wantSubstrs []string
	}{
		{
			lang: "java",
			wantSubstrs: []string{
				`@EnabledIfEnvironmentVariable(named = "GH_OPTIVEM_RUN_WIP_TESTS", matches = "1"`,
				`import org.junit.jupiter.api.condition.EnabledIfEnvironmentVariable;`,
			},
		},
		{
			lang: "csharp",
			wantSubstrs: []string{
				`[SkippableFact]`,
				`Skip.IfNot(Environment.GetEnvironmentVariable("GH_OPTIVEM_RUN_WIP_TESTS") == "1"`,
			},
		},
		{
			lang: "typescript",
			wantSubstrs: []string{
				`test.skip(process.env.GH_OPTIVEM_RUN_WIP_TESTS !== "1"`,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			got := renderGateMarkerExample(tc.lang)
			if got == "" {
				t.Fatalf("renderGateMarkerExample(%q): got empty string", tc.lang)
			}
			for _, want := range tc.wantSubstrs {
				if !strings.Contains(got, want) {
					t.Errorf("renderGateMarkerExample(%q): missing substring %q\nrendered:\n%s", tc.lang, want, got)
				}
			}
			// Drift-back regression guard: no per-ticket disable marker.
			for _, forbid := range []string{"@Disabled", "Fact(Skip", "#71"} {
				if strings.Contains(got, forbid) {
					t.Errorf("renderGateMarkerExample(%q): rendered output mentions %q — drift back to per-ticket disable marker?\nrendered:\n%s", tc.lang, forbid, got)
				}
			}
		})
	}
}

// TestRenderGateMarkerExample_FailFast pins the empty-string contract:
// an empty or unrecognised language produces "" so the dispatcher's
// findUnfilledPlaceholders surfaces the gap rather than silently
// substituting an empty placeholder.
func TestRenderGateMarkerExample_FailFast(t *testing.T) {
	for _, lang := range []string{"", "rust"} {
		t.Run("lang="+lang, func(t *testing.T) {
			if got := renderGateMarkerExample(lang); got != "" {
				t.Errorf("renderGateMarkerExample(%q): want empty, got %q", lang, got)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// PID marker file — writePidFile / removePidFile / dispatchCwd helpers
// that runHeadless / runInteractive use to record the spawned claude PID
// for orphan recovery (see plans/20260528-1309-orphan-recovery.md).
// ---------------------------------------------------------------------------

// TestWritePidFile_WritesParseableMarker pins the marker file's JSON
// shape: child_pid / parent_pid / cwd snake-case keys, matched by
// `gh optivem doctor --orphans`. Drift between writer and reader is
// the kind of silent breakage the schema test catches early.
func TestWritePidFile_WritesParseableMarker(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "001-system-implementer.pid")
	var stderr bytes.Buffer

	marker := userstate.PidMarker{ChildPid: 12345, ParentPid: 6789, Cwd: `C:\worktrees\rehearsal-20260528`}
	writePidFile(path, marker, &stderr)

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pid file: %v", err)
	}
	// Snake-case keys are the contract with doctor --orphans; pin them
	// as substrings before parsing so a rename failure is unambiguous.
	for _, want := range []string{`"child_pid":12345`, `"parent_pid":6789`, `"cwd":"C:\\worktrees\\rehearsal-20260528"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("pid file missing %q\nbody: %s", want, body)
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr warning on success path, got: %s", stderr.String())
	}
}

// TestWritePidFile_CreatesMissingParentDirs mirrors openEventsLog's
// mkdir-parents behaviour: the driver composes a nested path
// (<userStateDir>/runs/<ts>-<pid>/<seq>-<agent>.pid) and the helper
// is responsible for ensuring the intermediate dirs exist.
func TestWritePidFile_CreatesMissingParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "runs", "20260528-103900-12345", "001-system-implementer.pid")
	var stderr bytes.Buffer

	writePidFile(path, userstate.PidMarker{ChildPid: 1, ParentPid: 2, Cwd: dir}, &stderr)

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("pid file not created in nested path: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr warning when mkdir succeeds, got: %s", stderr.String())
	}
}

// TestWritePidFile_FailSoftOnUnwritablePath pins the fail-soft policy:
// a path whose parent isn't a directory downgrades to a stderr warning
// rather than panicking or returning an error — diagnostics must not
// break the dispatch. Same policy as openEventsLog (line 1796).
func TestWritePidFile_FailSoftOnUnwritablePath(t *testing.T) {
	tmp := t.TempDir()
	regularFile := filepath.Join(tmp, "blocking-file")
	if err := os.WriteFile(regularFile, []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	badPath := filepath.Join(regularFile, "nested", "marker.pid")
	var stderr bytes.Buffer

	writePidFile(badPath, userstate.PidMarker{ChildPid: 1, ParentPid: 2, Cwd: tmp}, &stderr)

	if stderr.Len() == 0 {
		t.Errorf("expected stderr warning when mkdir fails")
	}
	// Any Stat error means the file was not created. We cannot use
	// os.IsNotExist here because on Linux, traversing through a regular
	// file produces ENOTDIR, which os.IsNotExist does not recognize.
	if _, err := os.Stat(badPath); err == nil {
		t.Errorf("expected pid file not to exist when mkdir fails, but it does")
	}
}

// TestRemovePidFile_DeletesExisting pins the clean-exit path: when
// runHeadless / runInteractive's cmd.Wait() returns nil, the marker is
// removed so doctor --orphans sees only files from crashed runs.
func TestRemovePidFile_DeletesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "marker.pid")
	if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	var stderr bytes.Buffer

	removePidFile(path, &stderr)

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected pid file removed, stat err: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("expected no stderr warning on happy-path remove, got: %s", stderr.String())
	}
}

// TestRemovePidFile_SilentOnMissing pins the "missing file is vacuous
// success" branch: writePidFile may have skipped (fail-soft path), so
// removePidFile must not warn about a file it never created.
func TestRemovePidFile_SilentOnMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "never-existed.pid")
	var stderr bytes.Buffer

	removePidFile(path, &stderr)

	if stderr.Len() != 0 {
		t.Errorf("expected no stderr warning on missing-file remove, got: %s", stderr.String())
	}
}

// TestDispatchCwd_PrefersOptsDir pins the cwd-recording precedence:
// explicit opts.Dir (the working dir cmd.Dir is set from) always wins
// over os.Getwd. The marker's cwd field is what doctor --orphans shows
// the operator to identify "which project was this orphan for".
func TestDispatchCwd_PrefersOptsDir(t *testing.T) {
	want := filepath.Join(t.TempDir(), "explicit-dispatch-dir")
	if err := os.MkdirAll(want, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if got := dispatchCwd(want); got != want {
		t.Errorf("dispatchCwd(%q) = %q, want %q", want, got, want)
	}
}

// TestDispatchCwd_FallsBackToOsGetwd pins the fallback: when a caller
// passes Dir="" (utility runs, ad-hoc invocations), the recorded cwd
// should be the process's own working dir rather than blank.
func TestDispatchCwd_FallsBackToOsGetwd(t *testing.T) {
	want, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if got := dispatchCwd(""); got != want {
		t.Errorf("dispatchCwd(\"\") = %q, want %q (os.Getwd)", got, want)
	}
}
