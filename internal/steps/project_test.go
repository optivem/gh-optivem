package steps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// runRecord captures one shell invocation observed by a test.
type runRecord struct {
	cmd   string
	stdin string
	via   string // "Run" / "RunCapture" / "RunStdin"
}

// stubRunner records every shell call and returns canned responses keyed by
// substring. It replaces the package-level shell hooks for the duration of
// each test.
type stubRunner struct {
	t           *testing.T
	calls       []runRecord
	captureResp map[string]string
	captureErr  map[string]error
	runErr      map[string]error
	runOutput   map[string]string
}

func newStubRunner(t *testing.T) *stubRunner {
	return &stubRunner{
		t:           t,
		captureResp: map[string]string{},
		captureErr:  map[string]error{},
		runErr:      map[string]error{},
		runOutput:   map[string]string{},
	}
}

// install replaces the package-level shell hooks with this stub for the
// remainder of t. Cleans up automatically via t.Cleanup.
func (s *stubRunner) install() {
	prevCapture := projectRunCapture
	prevStdin := projectRunStdin
	prevRun := projectRun
	prevConfirm := projectConfirmFn
	prevLoad := projectLoadSourceConfig
	prevWrite := projectWriteSourceConfig
	projectRunCapture = func(cmd, _ string) (string, error) {
		s.calls = append(s.calls, runRecord{cmd: cmd, via: "RunCapture"})
		for k, v := range s.captureResp {
			if strings.Contains(cmd, k) {
				if err := s.captureErr[k]; err != nil {
					return v, err
				}
				return v, nil
			}
		}
		return "", fmt.Errorf("stub: unmatched RunCapture %q", cmd)
	}
	projectRunStdin = func(cmd, stdin string, _ bool, _ string) (string, error) {
		s.calls = append(s.calls, runRecord{cmd: cmd, stdin: stdin, via: "RunStdin"})
		return "", nil
	}
	projectRun = func(cmd string, _ bool, _ string) (string, error) {
		s.calls = append(s.calls, runRecord{cmd: cmd, via: "Run"})
		for k, v := range s.runOutput {
			if strings.Contains(cmd, k) {
				if err := s.runErr[k]; err != nil {
					return v, err
				}
				return v, nil
			}
		}
		return "", nil
	}
	s.t.Cleanup(func() {
		projectRunCapture = prevCapture
		projectRunStdin = prevStdin
		projectRun = prevRun
		projectConfirmFn = prevConfirm
		projectLoadSourceConfig = prevLoad
		projectWriteSourceConfig = prevWrite
	})
}

func (s *stubRunner) calledViaContaining(via, fragment string) []runRecord {
	var out []runRecord
	for _, r := range s.calls {
		if r.via == via && strings.Contains(r.cmd, fragment) {
			out = append(out, r)
		}
	}
	return out
}

// captureGraphqlOptions parses the JSON body sent to gh api graphql --input -
// and returns the option names the mutation passed.
func captureGraphqlOptions(t *testing.T, stdin string) []string {
	t.Helper()
	var req struct {
		Variables struct {
			Options []struct {
				Name string `json:"name"`
			} `json:"options"`
		} `json:"variables"`
	}
	if err := json.Unmarshal([]byte(stdin), &req); err != nil {
		t.Fatalf("graphql body unparseable: %v\nbody=%s", err, stdin)
	}
	names := make([]string, 0, len(req.Variables.Options))
	for _, o := range req.Variables.Options {
		names = append(names, o.Name)
	}
	return names
}

// containsAll asserts every needle is somewhere in haystack (case-insensitive).
func containsAll(t *testing.T, haystack []string, needles ...string) {
	t.Helper()
	have := make(map[string]bool, len(haystack))
	for _, h := range haystack {
		have[strings.ToLower(h)] = true
	}
	for _, n := range needles {
		if !have[strings.ToLower(n)] {
			t.Errorf("expected %q in %v", n, haystack)
		}
	}
}

func TestParseProjectURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in        string
		wantOwner string
		wantNum   int
		wantErr   bool
	}{
		{"https://github.com/users/acme/projects/5", "acme", 5, false},
		{"https://github.com/orgs/acme/projects/12", "acme", 12, false},
		{"https://github.com/orgs/acme/projects/5/", "acme", 5, false},
		{"https://github.com/acme/projects/5", "", 0, true},                    // missing users/orgs prefix
		{"https://gitlab.com/users/acme/projects/5", "", 0, true},              // wrong host
		{"https://github.com/users/acme/projects/abc", "", 0, true},            // non-numeric
		{"https://github.com/users/acme/projects/0", "", 0, true},              // zero
		{"not a url", "", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			owner, n, err := parseProjectURL(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got owner=%s n=%d", owner, n)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if owner != tc.wantOwner || n != tc.wantNum {
				t.Errorf("got owner=%s n=%d, want owner=%s n=%d", owner, n, tc.wantOwner, tc.wantNum)
			}
		})
	}
}

func TestMissingOptions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		current  []string
		required []string
		want     []string
	}{
		{"all present", []string{"Ready", "In Progress", "In Acceptance", "In QA"}, atddRequiredStatusOptions, nil},
		{"all missing", []string{"Todo", "Done"}, atddRequiredStatusOptions, atddRequiredStatusOptions},
		{"some missing", []string{"Ready", "In Progress", "Done"}, atddRequiredStatusOptions, []string{"In Acceptance", "In QA"}},
		{"case-insensitive match", []string{"ready", "IN PROGRESS", "in acceptance", "in qa"}, atddRequiredStatusOptions, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := missingOptions(tc.current, tc.required)
			if !equalStringSlices(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBuildSetStatusOptionsRequest(t *testing.T) {
	t.Parallel()
	body, err := buildSetStatusOptionsRequest("FIELD_ID", []string{"Ready", "In Progress", "In Acceptance", "In QA"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var req struct {
		Query     string `json:"query"`
		Variables struct {
			FieldID string `json:"fieldId"`
			Options []struct {
				Name        string `json:"name"`
				Color       string `json:"color"`
				Description string `json:"description"`
			} `json:"options"`
		} `json:"variables"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("body unparseable: %v\n%s", err, body)
	}
	if !strings.Contains(req.Query, "updateProjectV2Field") {
		t.Errorf("query missing mutation: %s", req.Query)
	}
	if req.Variables.FieldID != "FIELD_ID" {
		t.Errorf("fieldId: got %q, want FIELD_ID", req.Variables.FieldID)
	}
	if len(req.Variables.Options) != 4 {
		t.Fatalf("option count: got %d, want 4", len(req.Variables.Options))
	}
	for _, o := range req.Variables.Options {
		if o.Color == "" {
			t.Errorf("option %s missing color", o.Name)
		}
	}
}

func TestBuildSetStatusOptionsRequest_Dedup(t *testing.T) {
	t.Parallel()
	body, err := buildSetStatusOptionsRequest("F", []string{"Ready", "ready", "READY"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	names := captureGraphqlOptions(t, body)
	if len(names) != 1 {
		t.Errorf("expected dedup to 1 option, got %v", names)
	}
}

// TestEnsureProjectBoard_PathA_FirstRun: no existing project, create one,
// set canonical Status options, link the repo.
func TestEnsureProjectBoard_PathA_FirstRun(t *testing.T) {
	stub := newStubRunner(t)
	stub.captureResp["project list"] = `{"projects":[]}`
	stub.captureResp["project create"] = `{"id":"PVT_X","number":7,"title":"Page Turner","url":"https://github.com/users/acme/projects/7"}`
	stub.captureResp["project field-list"] = `{"fields":[{"id":"PVTSSF_S","name":"Status","type":"ProjectV2SingleSelectField","options":[{"id":"o1","name":"Todo"},{"id":"o2","name":"In Progress"},{"id":"o3","name":"Done"}]}]}`
	stub.install()

	cfg := &config.Config{
		Owner:        "acme",
		FullRepo:     "acme/page-turner",
		SystemName:   "Page Turner",
		RepoStrategy: "monorepo",
		Arch:         "monolith",
	}
	EnsureProjectBoard(cfg, nil)

	if cfg.ProjectURL != "https://github.com/users/acme/projects/7" {
		t.Errorf("ProjectURL not set: %q", cfg.ProjectURL)
	}
	if got := stub.calledViaContaining("RunCapture", "project list"); len(got) != 1 {
		t.Errorf("expected 1 project list call, got %d", len(got))
	}
	if got := stub.calledViaContaining("RunCapture", "project create"); len(got) != 1 {
		t.Errorf("expected 1 project create call, got %d", len(got))
	}
	mutCalls := stub.calledViaContaining("RunStdin", "gh api graphql")
	if len(mutCalls) != 1 {
		t.Fatalf("expected 1 graphql mutation call, got %d", len(mutCalls))
	}
	names := captureGraphqlOptions(t, mutCalls[0].stdin)
	containsAll(t, names, canonicalStatusOptions...)
	// Default Todo must be replaced (not in canonical set).
	for _, n := range names {
		if strings.EqualFold(n, "Todo") {
			t.Errorf("canonical replace must drop Todo, got names=%v", names)
		}
	}
	if got := stub.calledViaContaining("Run", "project link"); len(got) != 1 {
		t.Errorf("expected 1 project link call, got %d", len(got))
	}
}

// TestEnsureProjectBoard_PathA_Reuse: an existing project with the same title
// is reused — no create call, but the canonical Status set is still applied
// and the repo is (re)linked.
func TestEnsureProjectBoard_PathA_Reuse(t *testing.T) {
	stub := newStubRunner(t)
	stub.captureResp["project list"] = `{"projects":[{"id":"PVT_E","number":3,"title":"Page Turner","url":"https://github.com/users/acme/projects/3"}]}`
	stub.captureResp["project field-list"] = `{"fields":[{"id":"PVTSSF_S","name":"Status","type":"ProjectV2SingleSelectField","options":[{"id":"o1","name":"Backlog"},{"id":"o2","name":"Ready"},{"id":"o3","name":"In Progress"},{"id":"o4","name":"In Acceptance"},{"id":"o5","name":"In QA"},{"id":"o6","name":"Done"}]}]}`
	stub.runOutput["project link"] = "Linked"
	stub.install()

	cfg := &config.Config{
		Owner:        "acme",
		FullRepo:     "acme/page-turner",
		SystemName:   "Page Turner",
		RepoStrategy: "monorepo",
		Arch:         "monolith",
	}
	EnsureProjectBoard(cfg, nil)

	if cfg.ProjectURL != "https://github.com/users/acme/projects/3" {
		t.Errorf("ProjectURL not set: %q", cfg.ProjectURL)
	}
	if got := stub.calledViaContaining("RunCapture", "project create"); len(got) != 0 {
		t.Errorf("expected 0 project create calls on reuse, got %d", len(got))
	}
	if got := stub.calledViaContaining("RunStdin", "gh api graphql"); len(got) != 1 {
		t.Errorf("expected 1 graphql mutation call (canonical re-apply), got %d", len(got))
	}
	if got := stub.calledViaContaining("Run", "project link"); len(got) != 1 {
		t.Errorf("expected 1 project link call on reuse, got %d", len(got))
	}
}

// TestEnsureProjectBoard_PathA_Multirepo: multitier multirepo links both
// component repos.
func TestEnsureProjectBoard_PathA_Multirepo_Multitier(t *testing.T) {
	stub := newStubRunner(t)
	stub.captureResp["project list"] = `{"projects":[]}`
	stub.captureResp["project create"] = `{"id":"PVT_X","number":1,"title":"X","url":"https://github.com/orgs/acme/projects/1"}`
	stub.captureResp["project field-list"] = `{"fields":[{"id":"PVTSSF_S","name":"Status","type":"ProjectV2SingleSelectField","options":[{"id":"o1","name":"Todo"}]}]}`
	stub.install()

	cfg := &config.Config{
		Owner:            "acme",
		FullRepo:         "acme/page-turner",
		SystemName:       "X",
		RepoStrategy:     "multirepo",
		Arch:             "multitier",
		BackendFullRepo:  "acme/page-turner-backend",
		FrontendFullRepo: "acme/page-turner-frontend",
	}
	EnsureProjectBoard(cfg, nil)

	links := stub.calledViaContaining("Run", "project link")
	if len(links) != 2 {
		t.Fatalf("expected 2 link calls (backend + frontend), got %d", len(links))
	}
	var sawBackend, sawFrontend bool
	for _, l := range links {
		if strings.Contains(l.cmd, "acme/page-turner-backend") {
			sawBackend = true
		}
		if strings.Contains(l.cmd, "acme/page-turner-frontend") {
			sawFrontend = true
		}
	}
	if !sawBackend || !sawFrontend {
		t.Errorf("expected both component repos linked; calls=%v", links)
	}
}

// TestEnsureProjectBoard_PathA_LinkAlreadyExists: tolerates the "already
// linked" CLI response on reuse — repo association is preserved.
func TestEnsureProjectBoard_PathA_LinkAlreadyExists(t *testing.T) {
	stub := newStubRunner(t)
	stub.captureResp["project list"] = `{"projects":[{"id":"PVT_E","number":3,"title":"X","url":"https://github.com/users/acme/projects/3"}]}`
	stub.captureResp["project field-list"] = `{"fields":[{"id":"PVTSSF_S","name":"Status","type":"ProjectV2SingleSelectField","options":[]}]}`
	stub.runOutput["project link"] = "Repository already linked to this project"
	stub.runErr["project link"] = fmt.Errorf("exit status 1")
	stub.install()

	cfg := &config.Config{
		Owner:        "acme",
		FullRepo:     "acme/page-turner",
		SystemName:   "X",
		RepoStrategy: "monorepo",
		Arch:         "monolith",
	}
	// Should not panic / Fatalf.
	EnsureProjectBoard(cfg, nil)
}

// TestEnsureProjectBoard_PathB_AdditiveMerge: supplied URL with default
// columns gets the missing ATDD options added; existing names preserved.
func TestEnsureProjectBoard_PathB_AdditiveMerge(t *testing.T) {
	stub := newStubRunner(t)
	stub.captureResp["project field-list"] = `{"fields":[{"id":"PVTSSF_S","name":"Status","type":"ProjectV2SingleSelectField","options":[{"id":"o1","name":"Todo"},{"id":"o2","name":"In Progress"},{"id":"o3","name":"Done"}]}]}`
	stub.install()
	projectConfirmFn = func(_ *config.Config, _ []string) bool { return true }

	cfg := &config.Config{
		Owner:        "acme",
		FullRepo:     "acme/x",
		ProjectURL:   "https://github.com/users/acme/projects/9",
		RepoStrategy: "monorepo",
		Arch:         "monolith",
	}
	EnsureProjectBoard(cfg, nil)

	mut := stub.calledViaContaining("RunStdin", "gh api graphql")
	if len(mut) != 1 {
		t.Fatalf("expected 1 graphql mutation call, got %d", len(mut))
	}
	names := captureGraphqlOptions(t, mut[0].stdin)
	// All required ATDD options present.
	containsAll(t, names, atddRequiredStatusOptions...)
	// Existing Todo / Done preserved (case-insensitive match).
	containsAll(t, names, "Todo", "Done")

	// Path B must NOT auto-create, link, or list projects.
	if got := stub.calledViaContaining("RunCapture", "project create"); len(got) != 0 {
		t.Errorf("Path B must not call project create; got %d", len(got))
	}
	if got := stub.calledViaContaining("RunCapture", "project list"); len(got) != 0 {
		t.Errorf("Path B must not call project list; got %d", len(got))
	}
	if got := stub.calledViaContaining("Run", "project link"); len(got) != 0 {
		t.Errorf("Path B must not call project link; got %d", len(got))
	}
}

// TestEnsureProjectBoard_PathB_AlreadyComplete: all required options already
// present (mixed casing) — no mutation.
func TestEnsureProjectBoard_PathB_AlreadyComplete(t *testing.T) {
	stub := newStubRunner(t)
	stub.captureResp["project field-list"] = `{"fields":[{"id":"PVTSSF_S","name":"Status","type":"ProjectV2SingleSelectField","options":[{"id":"o1","name":"ready"},{"id":"o2","name":"IN PROGRESS"},{"id":"o3","name":"in acceptance"},{"id":"o4","name":"in qa"}]}]}`
	stub.install()
	confirmCalls := 0
	projectConfirmFn = func(_ *config.Config, _ []string) bool {
		confirmCalls++
		return true
	}

	cfg := &config.Config{
		Owner:        "acme",
		FullRepo:     "acme/x",
		ProjectURL:   "https://github.com/users/acme/projects/9",
		RepoStrategy: "monorepo",
		Arch:         "monolith",
	}
	EnsureProjectBoard(cfg, nil)

	if got := stub.calledViaContaining("RunStdin", "gh api graphql"); len(got) != 0 {
		t.Errorf("expected 0 graphql mutation calls when complete, got %d", len(got))
	}
	if confirmCalls != 0 {
		t.Errorf("expected no confirmation prompt when complete, got %d", confirmCalls)
	}
}

// TestEnsureProjectBoard_NoProject: --no-project short-circuits both paths,
// no shell calls regardless of cfg.ProjectURL.
func TestEnsureProjectBoard_NoProject(t *testing.T) {
	t.Run("auto-create path", func(t *testing.T) {
		stub := newStubRunner(t)
		stub.install()
		cfg := &config.Config{Owner: "acme", SystemName: "X", NoProject: true}
		EnsureProjectBoard(cfg, nil)
		if len(stub.calls) != 0 {
			t.Errorf("--no-project must not invoke shell; got %d", len(stub.calls))
		}
	})
	t.Run("supplied URL path", func(t *testing.T) {
		stub := newStubRunner(t)
		stub.install()
		cfg := &config.Config{
			Owner:      "acme",
			ProjectURL: "https://github.com/users/acme/projects/1",
			NoProject:  true,
		}
		EnsureProjectBoard(cfg, nil)
		if len(stub.calls) != 0 {
			t.Errorf("--no-project must not invoke shell; got %d", len(stub.calls))
		}
	})
}

func TestProjectAddDeclinedMessage(t *testing.T) {
	t.Parallel()
	t.Run("runtime-critical only", func(t *testing.T) {
		msg := projectAddDeclinedMessage([]string{"In Acceptance"})
		if !strings.Contains(msg, "runtime-critical") {
			t.Errorf("missing runtime-critical phrasing: %s", msg)
		}
		if strings.Contains(msg, "workflow column") {
			t.Errorf("should not mention workflow column: %s", msg)
		}
	})
	t.Run("QA only", func(t *testing.T) {
		msg := projectAddDeclinedMessage([]string{"In QA"})
		if !strings.Contains(msg, "workflow column") {
			t.Errorf("missing workflow column phrasing: %s", msg)
		}
		if strings.Contains(msg, "runtime-critical") {
			t.Errorf("should not mention runtime-critical: %s", msg)
		}
	})
	t.Run("mixed", func(t *testing.T) {
		msg := projectAddDeclinedMessage([]string{"In Acceptance", "In QA"})
		if !strings.Contains(msg, "runtime-critical") {
			t.Errorf("missing runtime-critical phrasing: %s", msg)
		}
		if !strings.Contains(msg, "workflow column") {
			t.Errorf("missing workflow column phrasing: %s", msg)
		}
	})
}

// TestReposToLink: link targets per arch + strategy.
func TestReposToLink(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		cfg  *config.Config
		want []string
	}{
		{"monolith monorepo", &config.Config{
			RepoStrategy: "monorepo", Arch: "monolith", FullRepo: "a/b",
		}, []string{"a/b"}},
		{"monolith multirepo", &config.Config{
			RepoStrategy: "multirepo", Arch: "monolith", SystemFullRepo: "a/b-system",
		}, []string{"a/b-system"}},
		{"multitier multirepo", &config.Config{
			RepoStrategy: "multirepo", Arch: "multitier",
			BackendFullRepo: "a/b-backend", FrontendFullRepo: "a/b-frontend",
		}, []string{"a/b-backend", "a/b-frontend"}},
		{"multitier monorepo", &config.Config{
			RepoStrategy: "monorepo", Arch: "multitier", FullRepo: "a/b",
		}, []string{"a/b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := reposToLink(tc.cfg)
			if !equalStringSlices(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// writeSourceYAML seeds a minimal gh-optivem.yaml at path with the given
// project URL. Returns the file's bytes after seeding so callers can
// compare them later for "untouched" assertions.
func writeSourceYAML(t *testing.T, path, projectURL string) []byte {
	t.Helper()
	pc := &projectconfig.Config{Project: projectconfig.Project{URL: projectURL}}
	if err := projectconfig.WriteToPath(path, pc); err != nil {
		t.Fatalf("seed source yaml: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seeded yaml: %v", err)
	}
	return got
}

// stubAutoCreatePathA seeds the stub runner with the canned responses Path A
// expects (project list → empty, create → URL, field-list → Status with
// default options).
func stubAutoCreatePathA(s *stubRunner, projectURL string) {
	s.captureResp["project list"] = `{"projects":[]}`
	s.captureResp["project create"] = fmt.Sprintf(
		`{"id":"PVT_X","number":7,"title":"Page Turner","url":%q}`, projectURL)
	s.captureResp["project field-list"] = `{"fields":[{"id":"PVTSSF_S","name":"Status","type":"ProjectV2SingleSelectField","options":[{"id":"o1","name":"Todo"}]}]}`
}

// TestEnsureProjectBoard_PathA_PersistsURLBackToSourceConfig: Path A with
// an empty project.url in the source gh-optivem.yaml writes the
// auto-created URL back into the same file.
func TestEnsureProjectBoard_PathA_PersistsURLBackToSourceConfig(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "gh-optivem.yaml")
	writeSourceYAML(t, src, "")

	stub := newStubRunner(t)
	stubAutoCreatePathA(stub, "https://github.com/users/acme/projects/7")
	stub.install()

	cfg := &config.Config{
		Owner:                    "acme",
		FullRepo:                 "acme/page-turner",
		SystemName:               "Page Turner",
		RepoStrategy:             "monorepo",
		Arch:                     "monolith",
		SourceConfigPath:         src,
		SourceProjectURLWasEmpty: true,
	}
	EnsureProjectBoard(cfg, nil)

	pc, err := projectconfig.LoadFromPath(src)
	if err != nil {
		t.Fatalf("reload source yaml: %v", err)
	}
	if pc.Project.URL != "https://github.com/users/acme/projects/7" {
		t.Errorf("source yaml project.url not persisted: got %q", pc.Project.URL)
	}
	if cfg.ProjectURL != pc.Project.URL {
		t.Errorf("cfg.ProjectURL %q != source url %q", cfg.ProjectURL, pc.Project.URL)
	}
}

// TestEnsureProjectBoard_PathA_LeavesSourceUntouched_WhenURLAlreadyPresent:
// reused-by-title runs (SourceProjectURLWasEmpty=false) must not rewrite
// the source file — no churn, no marshalling round-trip.
func TestEnsureProjectBoard_PathA_LeavesSourceUntouched_WhenURLAlreadyPresent(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "gh-optivem.yaml")
	originalBytes := writeSourceYAML(t, src, "https://github.com/users/acme/projects/3")

	stub := newStubRunner(t)
	// Reuse path: project list returns the existing project, no create call.
	stub.captureResp["project list"] = `{"projects":[{"id":"PVT_E","number":3,"title":"Page Turner","url":"https://github.com/users/acme/projects/3"}]}`
	stub.captureResp["project field-list"] = `{"fields":[{"id":"PVTSSF_S","name":"Status","type":"ProjectV2SingleSelectField","options":[]}]}`
	stub.install()

	cfg := &config.Config{
		Owner:                    "acme",
		FullRepo:                 "acme/page-turner",
		SystemName:               "Page Turner",
		RepoStrategy:             "monorepo",
		Arch:                     "monolith",
		SourceConfigPath:         src,
		SourceProjectURLWasEmpty: false,
	}
	EnsureProjectBoard(cfg, nil)

	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("re-read source yaml: %v", err)
	}
	if string(got) != string(originalBytes) {
		t.Errorf("source yaml mutated when SourceProjectURLWasEmpty=false:\noriginal:\n%s\ngot:\n%s", originalBytes, got)
	}
}

// TestEnsureProjectBoard_PathA_WriteErrorAborts: when the write seam
// returns an error, the step calls log.Fatalf (panics with *log.StepError).
func TestEnsureProjectBoard_PathA_WriteErrorAborts(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "gh-optivem.yaml")
	writeSourceYAML(t, src, "")

	stub := newStubRunner(t)
	stubAutoCreatePathA(stub, "https://github.com/users/acme/projects/7")
	stub.install()
	projectWriteSourceConfig = func(_ string, _ *projectconfig.Config) error {
		return fmt.Errorf("simulated write failure")
	}

	cfg := &config.Config{
		Owner:                    "acme",
		FullRepo:                 "acme/page-turner",
		SystemName:               "Page Turner",
		RepoStrategy:             "monorepo",
		Arch:                     "monolith",
		SourceConfigPath:         src,
		SourceProjectURLWasEmpty: true,
	}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected log.Fatalf panic, got none")
		}
		se, ok := r.(*log.StepError)
		if !ok {
			t.Fatalf("expected *log.StepError, got %T: %v", r, r)
		}
		if !strings.Contains(se.Msg, "persist project URL") {
			t.Errorf("unexpected panic message: %q", se.Msg)
		}
	}()
	EnsureProjectBoard(cfg, nil)
}

// TestEnsureProjectBoard_PathB_LeavesSourceUntouched: Path B never touches
// the source file regardless of SourceProjectURLWasEmpty — the URL came
// from the source config, nothing to persist.
func TestEnsureProjectBoard_PathB_LeavesSourceUntouched(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "gh-optivem.yaml")
	originalBytes := writeSourceYAML(t, src, "https://github.com/users/acme/projects/9")

	stub := newStubRunner(t)
	stub.captureResp["project field-list"] = `{"fields":[{"id":"PVTSSF_S","name":"Status","type":"ProjectV2SingleSelectField","options":[{"id":"o1","name":"Ready"},{"id":"o2","name":"In Progress"},{"id":"o3","name":"In Acceptance"},{"id":"o4","name":"In QA"}]}]}`
	stub.install()

	cfg := &config.Config{
		Owner:                    "acme",
		FullRepo:                 "acme/x",
		ProjectURL:               "https://github.com/users/acme/projects/9",
		RepoStrategy:             "monorepo",
		Arch:                     "monolith",
		SourceConfigPath:         src,
		SourceProjectURLWasEmpty: false,
	}
	EnsureProjectBoard(cfg, nil)

	got, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("re-read source yaml: %v", err)
	}
	if string(got) != string(originalBytes) {
		t.Errorf("Path B mutated source yaml")
	}
}
