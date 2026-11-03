package steps

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
)

var previewRemoteBase = map[string]string{
	"/environments": "", "gh label list": "", "/actions/variables": "", "/actions/secrets": "",
}

// previewFixture lays out a scaffold dir plus its baseline and returns a cfg
// pointing at them.
func previewFixture(t *testing.T, scaffold, existing map[string]string) *config.Config {
	t.Helper()
	installRemoteStub(t, previewRemoteBase, nil)
	dir := writeTree(t, scaffold)
	base := baselineDir(dir)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	for p, c := range existing {
		full := filepath.Join(base, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := existingCfg()
	cfg.RepoDir = dir
	return cfg
}

var previewScaffold = map[string]string{"a.txt": "one\ntwo\n", "new.txt": "n\n", "same.txt": "s\n"}
var previewExisting = map[string]string{"a.txt": "one\nTWO\nthree\n", "same.txt": "s\n", "mine.txt": "keep\n"}

func TestRunPreview_InactiveWithoutPreExistingRepos(t *testing.T) {
	cfg := existingCfg()
	cfg.PreExistingRepos = nil
	var out bytes.Buffer
	got, err := RunPreview(cfg, nil, nil, PreviewIO{Out: &out})
	if err != nil || got != PreviewInactive || out.Len() != 0 {
		t.Fatalf("got %v %v, output %q", got, err, out.String())
	}
}

func TestRunPreview_RendersSummaryOverwriteAndRemote(t *testing.T) {
	cfg := previewFixture(t, previewScaffold, previewExisting)
	cfg.DryRun = true
	var out bytes.Buffer
	got, err := RunPreview(cfg, nil, nil, PreviewIO{Out: &out})
	if err != nil || got != PreviewDryRun {
		t.Fatalf("got %v %v", got, err)
	}
	s := out.String()
	for _, want := range []string{
		"acme/shop",
		"1 new, 1 identical, 1 OVERWRITE, 1 kept",
		"1 file(s) will be OVERWRITTEN",
		"OVERWRITE  a.txt  (+1 -2)",
		"Remote resources",
		"create",
		"--dry-run: nothing was written",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "SECRET-VALUE") {
		t.Errorf("secret value leaked:\n%s", s)
	}
	if strings.Contains(s, "--- diff:") {
		t.Errorf("full diff must be verbose-only:\n%s", s)
	}
}

func TestRunPreview_VerboseAddsDiffAndNewList(t *testing.T) {
	cfg := previewFixture(t, previewScaffold, previewExisting)
	cfg.DryRun = true
	cfg.Verbose = true
	prev := diffRunner
	diffRunner = func(_, _ string) string { return "DIFFBODY" }
	t.Cleanup(func() { diffRunner = prev })
	var out bytes.Buffer
	if _, err := RunPreview(cfg, nil, nil, PreviewIO{Out: &out}); err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "--- diff: a.txt ---") || !strings.Contains(s, "DIFFBODY") || !strings.Contains(s, "NEW        new.txt") {
		t.Errorf("verbose output incomplete:\n%s", s)
	}
}

func TestRunPreview_DryRunNeverRestoresOrPrompts(t *testing.T) {
	cfg := previewFixture(t, previewScaffold, previewExisting)
	cfg.DryRun = true
	cfg.AssumeYes = true
	got, err := RunPreview(cfg, nil, nil, PreviewIO{In: strings.NewReader("y\n"), Out: &bytes.Buffer{}, Interactive: true})
	if err != nil || got != PreviewDryRun {
		t.Fatalf("got %v %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(cfg.RepoDir, "mine.txt")); err == nil {
		t.Error("dry-run must not restore kept files")
	}
}

func TestRunPreview_YesPrintsPreviewAndProceeds(t *testing.T) {
	cfg := previewFixture(t, previewScaffold, previewExisting)
	cfg.AssumeYes = true
	var out bytes.Buffer
	got, err := RunPreview(cfg, nil, nil, PreviewIO{Out: &out})
	if err != nil || got != PreviewProceed {
		t.Fatalf("got %v %v", got, err)
	}
	if !strings.Contains(out.String(), "OVERWRITE  a.txt") || !strings.Contains(out.String(), "--yes: proceeding") {
		t.Errorf("preview must print under --yes:\n%s", out.String())
	}
	if b, err := os.ReadFile(filepath.Join(cfg.RepoDir, "mine.txt")); err != nil || string(b) != "keep\n" {
		t.Errorf("kept file not restored: %v %q", err, b)
	}
	if b, _ := os.ReadFile(filepath.Join(cfg.RepoDir, "a.txt")); string(b) != "one\ntwo\n" {
		t.Errorf("scaffold file must win over the existing one, got %q", b)
	}
}

func TestRunPreview_NonTTYWithoutYesFailsActionably(t *testing.T) {
	cfg := previewFixture(t, previewScaffold, previewExisting)
	got, err := RunPreview(cfg, nil, nil, PreviewIO{In: strings.NewReader("y\n"), Out: &bytes.Buffer{}, Interactive: false})
	if err == nil || got == PreviewProceed {
		t.Fatalf("must not proceed, got %v %v", got, err)
	}
	if !strings.Contains(err.Error(), "--yes") || !strings.Contains(err.Error(), "--dry-run") {
		t.Errorf("message should name --yes and --dry-run: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(cfg.RepoDir, "mine.txt")); statErr == nil {
		t.Error("nothing may be restored when not confirmed")
	}
}

func TestRunPreview_PromptHasNoDefault(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  PreviewOutcome
	}{
		{"yes", "y\n", PreviewProceed},
		{"no", "n\n", PreviewDeclined},
		{"bare enter then no", "\nn\n", PreviewDeclined},
		{"eof", "", PreviewDeclined},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := previewFixture(t, previewScaffold, previewExisting)
			var out bytes.Buffer
			got, err := RunPreview(cfg, nil, nil, PreviewIO{In: strings.NewReader(tc.input), Out: &out, Interactive: true})
			if err != nil || got != tc.want {
				t.Fatalf("got %v %v, want %v", got, err, tc.want)
			}
			if tc.name == "bare enter then no" && !strings.Contains(out.String(), "Please answer y or n.") {
				t.Errorf("bare Enter must re-prompt:\n%s", out.String())
			}
			_, statErr := os.Stat(filepath.Join(cfg.RepoDir, "mine.txt"))
			if (tc.want == PreviewProceed) != (statErr == nil) {
				t.Errorf("kept file restored=%v for outcome %v", statErr == nil, tc.want)
			}
		})
	}
}

func TestRunPreview_RemoteProbeErrorAborts(t *testing.T) {
	cfg := previewFixture(t, previewScaffold, previewExisting)
	installRemoteStub(t, previewRemoteBase, map[string]error{"/environments": errors.New("HTTP 403")})
	cfg.AssumeYes = true
	if got, err := RunPreview(cfg, nil, nil, PreviewIO{Out: &bytes.Buffer{}}); err == nil || got == PreviewProceed {
		t.Fatalf("indeterminate probe must abort, got %v %v", got, err)
	}
}

func TestRunPreview_MissingBaselineFails(t *testing.T) {
	cfg := existingCfg()
	cfg.RepoDir = t.TempDir()
	cfg.AssumeYes = true
	if _, err := RunPreview(cfg, nil, nil, PreviewIO{Out: &bytes.Buffer{}}); err == nil || !strings.Contains(err.Error(), "baseline") {
		t.Fatalf("expected baseline error, got %v", err)
	}
}

func TestCheckRepoAccess_DistinctCauses(t *testing.T) {
	prev := repoAccessProbe
	t.Cleanup(func() { repoAccessProbe = prev })
	cases := []struct {
		name string
		resp string
		err  error
		want string // empty means success
	}{
		{"ok", "true\tmain\n", nil, ""},
		{"404", "", errors.New("gh: Not Found (HTTP 404)"), "HTTP 404"},
		{"403", "", errors.New("gh: Forbidden (HTTP 403)"), "HTTP 403"},
		{"401", "", errors.New("gh: Bad credentials (HTTP 401)"), "HTTP 401"},
		{"other", "", errors.New("connection reset"), "cannot determine"},
		{"no push", "false\tmain\n", nil, "no push access"},
		{"no default branch", "true\t\n", nil, "no default branch"},
		{"non-main default", "true\tmaster\n", nil, "default branch \"master\""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repoAccessProbe = func(string) (string, error) { return tc.resp, tc.err }
			err := checkRepoAccess("acme/shop")
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "acme/shop") {
				t.Fatalf("want error containing %q and the repo, got %v", tc.want, err)
			}
		})
	}
}

func TestCopyTree_SkipsGitAndKeepsContent(t *testing.T) {
	src := writeTree(t, map[string]string{"a/b.txt": "x", ".git/config": "g", "c.txt": "y"})
	dst := t.TempDir()
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dst, "a", "b.txt")); err != nil || string(b) != "x" {
		t.Errorf("a/b.txt: %v %q", err, b)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); err == nil {
		t.Error(".git must be skipped")
	}
}
