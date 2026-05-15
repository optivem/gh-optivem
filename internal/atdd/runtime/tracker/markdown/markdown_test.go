package markdown

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
)

// ---------------------------------------------------------------------------
// Compile-time interface assertion
// ---------------------------------------------------------------------------

var _ tracker.Tracker = (*Tracker)(nil)

// ---------------------------------------------------------------------------
// Fake GitRunner
// ---------------------------------------------------------------------------

// fakeGit records each invocation. For `mv` it performs the rename on
// the real filesystem so subsequent stats/reads see the moved file —
// the rest of the adapter's logic depends on the file actually being
// where it is supposed to be after the move. Tests that need to
// simulate failure can swap in a `failGit`.
type fakeGit struct {
	t        *testing.T
	boardDir string
	calls    [][]string
}

func newFakeGit(t *testing.T, boardDir string) *fakeGit {
	return &fakeGit{t: t, boardDir: boardDir}
}

func (g *fakeGit) Run(_ context.Context, args ...string) ([]byte, error) {
	g.calls = append(g.calls, append([]string(nil), args...))
	if len(args) >= 1 && args[0] == "mv" && len(args) == 3 {
		src := filepath.Join(g.boardDir, args[1])
		dst := filepath.Join(g.boardDir, args[2])
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return nil, err
		}
		if err := os.Rename(src, dst); err != nil {
			return nil, err
		}
		return nil, nil
	}
	// add / commit / anything else: just record.
	return nil, nil
}

// failGit returns an error for every Run call. Used to exercise the
// error path of MarkChecklistComplete and SetStatus.
type failGit struct{}

func (failGit) Run(_ context.Context, args ...string) ([]byte, error) {
	return nil, fmt.Errorf("git %s: simulated failure", strings.Join(args, " "))
}

// ---------------------------------------------------------------------------
// Board fixture
// ---------------------------------------------------------------------------

// newBoard builds a board directory tree under t.TempDir() with the
// four canonical status subdirs and the files declared in items.
// items maps relative paths (under boardDir) to file contents.
func newBoard(t *testing.T, items map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range statusDirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	for rel, body := range items {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	return root
}

// ---------------------------------------------------------------------------
// New
// ---------------------------------------------------------------------------

func TestNew_RejectsEmptyDir(t *testing.T) {
	if _, err := New("", nil); err == nil {
		t.Fatalf("expected error for empty boardDir")
	}
}

func TestNew_RejectsMissingDir(t *testing.T) {
	if _, err := New(filepath.Join(t.TempDir(), "nope"), nil); err == nil {
		t.Fatalf("expected error for missing boardDir")
	}
}

func TestNew_RejectsFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "not-a-dir.md")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := New(f, nil); err == nil {
		t.Fatalf("expected error for file path")
	}
}

func TestNew_AcceptsRelativePath(t *testing.T) {
	root := t.TempDir()
	cwd, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	tr, err := New(".", newFakeGit(t, root))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !filepath.IsAbs(tr.boardDir) {
		t.Errorf("boardDir not absolute: %q", tr.boardDir)
	}
}

// ---------------------------------------------------------------------------
// PickReady
// ---------------------------------------------------------------------------

func TestPickReady_ReturnsLexicographicallyFirst(t *testing.T) {
	root := newBoard(t, map[string]string{
		"ready/003-third.md":  "# Third\n",
		"ready/001-first.md":  "# First story\n",
		"ready/002-second.md": "# Second\n",
		"done/000-done.md":    "# Done\n",
	})
	tr, err := New(root, newFakeGit(t, root))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := tr.PickReady(context.Background())
	if err != nil {
		t.Fatalf("PickReady: %v", err)
	}
	if got.ID != "001-first" {
		t.Errorf("ID = %q, want 001-first", got.ID)
	}
	if got.Title != "First story" {
		t.Errorf("Title = %q, want First story", got.Title)
	}
	if got.URL != "" {
		t.Errorf("URL = %q, want empty", got.URL)
	}
	if !strings.HasSuffix(got.Handle, filepath.Join("ready", "001-first.md")) {
		t.Errorf("Handle = %q, want path ending in ready/001-first.md", got.Handle)
	}
}

func TestPickReady_EmptyReadyReturnsSentinel(t *testing.T) {
	root := newBoard(t, map[string]string{
		"done/finished.md": "# done\n",
	})
	tr, _ := New(root, newFakeGit(t, root))
	_, err := tr.PickReady(context.Background())
	if !errors.Is(err, tracker.ErrEmptyReady) {
		t.Errorf("expected tracker.ErrEmptyReady, got %v", err)
	}
}

func TestPickReady_IgnoresHiddenFiles(t *testing.T) {
	root := newBoard(t, map[string]string{
		"ready/.DS_Store.md":    "# hidden\n",
		"ready/001-visible.md":  "# visible one\n",
	})
	tr, _ := New(root, newFakeGit(t, root))
	got, err := tr.PickReady(context.Background())
	if err != nil {
		t.Fatalf("PickReady: %v", err)
	}
	if got.ID != "001-visible" {
		t.Errorf("ID = %q, want 001-visible", got.ID)
	}
}

func TestPickReady_FallsBackToFilenameTitle(t *testing.T) {
	// File with no H1 — title falls back to the ID.
	root := newBoard(t, map[string]string{
		"ready/no-h1.md": "Body only, no heading.\n",
	})
	tr, _ := New(root, newFakeGit(t, root))
	got, err := tr.PickReady(context.Background())
	if err != nil {
		t.Fatalf("PickReady: %v", err)
	}
	if got.Title != "no-h1" {
		t.Errorf("Title = %q, want filename fallback no-h1", got.Title)
	}
}

// ---------------------------------------------------------------------------
// FindIssue
// ---------------------------------------------------------------------------

func TestFindIssue_ByID(t *testing.T) {
	root := newBoard(t, map[string]string{
		"in-progress/SHOP-7.md": "# Shop seven\n",
	})
	tr, _ := New(root, newFakeGit(t, root))
	got, err := tr.FindIssue(context.Background(), "SHOP-7")
	if err != nil {
		t.Fatalf("FindIssue: %v", err)
	}
	if got.ID != "SHOP-7" || got.Title != "Shop seven" {
		t.Errorf("got (%q, %q), want (SHOP-7, Shop seven)", got.ID, got.Title)
	}
}

func TestFindIssue_ByRelPath(t *testing.T) {
	root := newBoard(t, map[string]string{
		"ready/001-x.md": "# X\n",
	})
	tr, _ := New(root, newFakeGit(t, root))
	got, err := tr.FindIssue(context.Background(), filepath.Join("ready", "001-x.md"))
	if err != nil {
		t.Fatalf("FindIssue: %v", err)
	}
	if got.ID != "001-x" {
		t.Errorf("ID = %q, want 001-x", got.ID)
	}
}

func TestFindIssue_ByAbsolutePath(t *testing.T) {
	root := newBoard(t, map[string]string{
		"ready/abc.md": "# Abc\n",
	})
	tr, _ := New(root, newFakeGit(t, root))
	abs := filepath.Join(root, "ready", "abc.md")
	got, err := tr.FindIssue(context.Background(), abs)
	if err != nil {
		t.Fatalf("FindIssue: %v", err)
	}
	if got.ID != "abc" {
		t.Errorf("ID = %q", got.ID)
	}
}

func TestFindIssue_NotFound(t *testing.T) {
	root := newBoard(t, map[string]string{
		"ready/exists.md": "# x\n",
	})
	tr, _ := New(root, newFakeGit(t, root))
	if _, err := tr.FindIssue(context.Background(), "nope"); err == nil {
		t.Fatalf("expected error for missing ID")
	}
}

func TestFindIssue_RejectsEmptyInput(t *testing.T) {
	root := newBoard(t, nil)
	tr, _ := New(root, newFakeGit(t, root))
	if _, err := tr.FindIssue(context.Background(), ""); err == nil {
		t.Fatalf("expected error for empty input")
	}
}

// ---------------------------------------------------------------------------
// SetStatus
// ---------------------------------------------------------------------------

func TestSetStatus_GitMvBetweenStatusDirs(t *testing.T) {
	root := newBoard(t, map[string]string{
		"ready/123.md": "# story\n",
	})
	gg := newFakeGit(t, root)
	tr, _ := New(root, gg)

	src := filepath.Join(root, "ready", "123.md")
	if err := tr.SetStatus(context.Background(), src, "In progress"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if len(gg.calls) != 1 {
		t.Fatalf("expected 1 git call, got %d: %v", len(gg.calls), gg.calls)
	}
	wantArgs := []string{"mv",
		filepath.Join("ready", "123.md"),
		filepath.Join("in-progress", "123.md"),
	}
	if strings.Join(gg.calls[0], "\x00") != strings.Join(wantArgs, "\x00") {
		t.Errorf("git argv = %v\nwant %v", gg.calls[0], wantArgs)
	}
	// Side-effect: the file ended up where we asked.
	if _, err := os.Stat(filepath.Join(root, "in-progress", "123.md")); err != nil {
		t.Errorf("file not at destination: %v", err)
	}
}

func TestSetStatus_CreatesNewStatusDirOnDemand(t *testing.T) {
	root := newBoard(t, map[string]string{
		"ready/x.md": "# x\n",
	})
	gg := newFakeGit(t, root)
	tr, _ := New(root, gg)
	src := filepath.Join(root, "ready", "x.md")
	if err := tr.SetStatus(context.Background(), src, "In review"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	dst := filepath.Join(root, "in-review", "x.md")
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("expected file at %s: %v", dst, err)
	}
}

func TestSetStatus_NoopWhenAlreadyInTargetStatus(t *testing.T) {
	root := newBoard(t, map[string]string{
		"in-progress/y.md": "# y\n",
	})
	gg := newFakeGit(t, root)
	tr, _ := New(root, gg)
	src := filepath.Join(root, "in-progress", "y.md")
	if err := tr.SetStatus(context.Background(), src, "In progress"); err != nil {
		t.Fatalf("SetStatus: %v", err)
	}
	if len(gg.calls) != 0 {
		t.Errorf("expected 0 git calls when src==dst, got %d: %v", len(gg.calls), gg.calls)
	}
}

func TestSetStatus_RejectsEmptyInputs(t *testing.T) {
	root := newBoard(t, map[string]string{"ready/x.md": "# x\n"})
	tr, _ := New(root, newFakeGit(t, root))
	if err := tr.SetStatus(context.Background(), "", "In progress"); err == nil {
		t.Errorf("expected error for empty handle")
	}
	if err := tr.SetStatus(context.Background(), filepath.Join(root, "ready", "x.md"), ""); err == nil {
		t.Errorf("expected error for empty status")
	}
}

func TestSetStatus_GitFailureWraps(t *testing.T) {
	root := newBoard(t, map[string]string{"ready/x.md": "# x\n"})
	tr, _ := New(root, failGit{})
	err := tr.SetStatus(context.Background(),
		filepath.Join(root, "ready", "x.md"), "Done")
	if err == nil {
		t.Fatalf("expected wrapped git error")
	}
	if !strings.Contains(err.Error(), "git mv") {
		t.Errorf("error did not wrap git mv: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Verify
// ---------------------------------------------------------------------------

func TestVerify_HappyPath(t *testing.T) {
	root := newBoard(t, nil) // newBoard creates all four canonical dirs
	tr, _ := New(root, newFakeGit(t, root))
	if err := tr.Verify(context.Background()); err != nil {
		t.Errorf("Verify: %v", err)
	}
}

func TestVerify_ReportsMissingDirs(t *testing.T) {
	// Bare empty dir — none of the canonical subdirs exist.
	root := t.TempDir()
	tr, _ := New(root, newFakeGit(t, root))
	err := tr.Verify(context.Background())
	if !errors.Is(err, ErrBoardDirMissing) {
		t.Fatalf("expected ErrBoardDirMissing, got %v", err)
	}
	for _, want := range []string{"ready", "in-progress", "in-acceptance", "done"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error did not name %q: %v", want, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Classify
// ---------------------------------------------------------------------------

func TestClassify_FrontmatterTypeWins(t *testing.T) {
	root := newBoard(t, map[string]string{
		"ready/x.md": "---\ntype: bug\nowner: alice\n---\n\n# Title\n",
	})
	tr, _ := New(root, newFakeGit(t, root))
	got, err := tr.FindIssue(context.Background(), "x")
	if err != nil {
		t.Fatalf("FindIssue: %v", err)
	}
	kind, ok, err := tr.Classify(context.Background(), got)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if kind != "bug" || !ok {
		t.Errorf("got (%q, %v), want (bug, true)", kind, ok)
	}
}

func TestClassify_FilenameHeuristic(t *testing.T) {
	root := newBoard(t, map[string]string{
		"ready/feature-add-cart.md": "# Add cart\n",
		"ready/bug-login.md":        "# Login\n",
		"ready/random-thing.md":     "# Random\n",
	})
	tr, _ := New(root, newFakeGit(t, root))
	cases := []struct {
		id     string
		wantK  string
		wantOK bool
	}{
		{"feature-add-cart", "feature", false},
		{"bug-login", "bug", false},
		{"random-thing", "", false}, // not in knownKinds
	}
	for _, c := range cases {
		t.Run(c.id, func(t *testing.T) {
			issue, err := tr.FindIssue(context.Background(), c.id)
			if err != nil {
				t.Fatalf("FindIssue: %v", err)
			}
			k, ok, err := tr.Classify(context.Background(), issue)
			if err != nil {
				t.Fatalf("Classify: %v", err)
			}
			if k != c.wantK || ok != c.wantOK {
				t.Errorf("got (%q, %v), want (%q, %v)", k, ok, c.wantK, c.wantOK)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ReadSections
// ---------------------------------------------------------------------------

func TestReadSections_StableKeySet(t *testing.T) {
	body := "# Title\n\n" +
		"## Description\n\nthe description\n\n" +
		"## Acceptance Criteria\n\n- AC1\n- AC2\n\n" +
		"## Checklist\n\n- [ ] step\n"
	root := newBoard(t, map[string]string{"ready/x.md": body})
	tr, _ := New(root, newFakeGit(t, root))
	issue, err := tr.FindIssue(context.Background(), "x")
	if err != nil {
		t.Fatalf("FindIssue: %v", err)
	}
	got, err := tr.ReadSections(context.Background(), issue,
		[]string{"Acceptance Criteria", "Checklist", "Missing"})
	if err != nil {
		t.Fatalf("ReadSections: %v", err)
	}
	if got["Acceptance Criteria"] != "- AC1\n- AC2" {
		t.Errorf("Acceptance Criteria = %q", got["Acceptance Criteria"])
	}
	if got["Checklist"] != "- [ ] step" {
		t.Errorf("Checklist = %q", got["Checklist"])
	}
	if got["Missing"] != "" {
		t.Errorf("Missing = %q, want empty", got["Missing"])
	}
}

// ---------------------------------------------------------------------------
// MarkChecklistComplete
// ---------------------------------------------------------------------------

func TestMarkChecklistComplete_RewritesAndCommits(t *testing.T) {
	body := "# T\n\n## Checklist\n\n- [ ] One\n- [x] Two\n- [ ] Three\n"
	root := newBoard(t, map[string]string{"ready/x.md": body})
	gg := newFakeGit(t, root)
	tr, _ := New(root, gg)
	issue, _ := tr.FindIssue(context.Background(), "x")
	if err := tr.MarkChecklistComplete(context.Background(), issue); err != nil {
		t.Fatalf("MarkChecklistComplete: %v", err)
	}
	// File rewritten in place.
	got, err := os.ReadFile(issue.Handle)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	want := "# T\n\n## Checklist\n\n- [x] One\n- [x] Two\n- [x] Three\n"
	if string(got) != want {
		t.Errorf("file body:\ngot:  %q\nwant: %q", got, want)
	}
	// Two git calls: add + commit -m … -- <rel>.
	if len(gg.calls) != 2 {
		t.Fatalf("expected 2 git calls (add, commit), got %d: %v", len(gg.calls), gg.calls)
	}
	if gg.calls[0][0] != "add" {
		t.Errorf("first call: got %v, want add", gg.calls[0])
	}
	if gg.calls[1][0] != "commit" {
		t.Errorf("second call: got %v, want commit", gg.calls[1])
	}
	// Commit message names the issue ID.
	joined := strings.Join(gg.calls[1], " ")
	if !strings.Contains(joined, "x") {
		t.Errorf("commit message should name issue ID; got %q", joined)
	}
}

func TestMarkChecklistComplete_NoUncheckedIsNoop(t *testing.T) {
	body := "## Checklist\n\n- [x] Done one\n- [x] Done two\n"
	root := newBoard(t, map[string]string{"ready/y.md": body})
	gg := newFakeGit(t, root)
	tr, _ := New(root, gg)
	issue, _ := tr.FindIssue(context.Background(), "y")
	if err := tr.MarkChecklistComplete(context.Background(), issue); err != nil {
		t.Fatalf("MarkChecklistComplete: %v", err)
	}
	if len(gg.calls) != 0 {
		t.Errorf("expected 0 git calls on no-op, got %d: %v", len(gg.calls), gg.calls)
	}
}

func TestMarkChecklistComplete_GitFailureWraps(t *testing.T) {
	body := "## Checklist\n\n- [ ] One\n"
	root := newBoard(t, map[string]string{"ready/z.md": body})
	tr, _ := New(root, failGit{})
	issue := tracker.Issue{
		ID:     "z",
		Handle: filepath.Join(root, "ready", "z.md"),
	}
	if err := tr.MarkChecklistComplete(context.Background(), issue); err == nil {
		t.Fatalf("expected wrapped git error")
	}
}

// ---------------------------------------------------------------------------
// Lower-level helpers
// ---------------------------------------------------------------------------

func TestStatusDirName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Ready", "ready"},
		{"In progress", "in-progress"},
		{"In acceptance", "in-acceptance"},
		{"Done", "done"},
		{"  Mixed CASE Name  ", "mixed-case-name"},
	}
	for _, c := range cases {
		if got := statusDirName(c.in); got != c.want {
			t.Errorf("statusDirName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFrontmatterType(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"present", "---\ntype: feature\n---\n\nbody", "feature"},
		{"with other fields", "---\nowner: bob\ntype: bug\nweight: 3\n---\n", "bug"},
		{"no frontmatter", "# Title\ntype: bug\nbody\n", ""},
		{"frontmatter without type", "---\nowner: bob\n---\nbody\n", ""},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := frontmatterType(c.in); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
