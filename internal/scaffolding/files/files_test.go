package files

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReplaceInTreeRespectsBinaryExts confirms that a nil-extensions
// ReplaceInTree call processes text files and skips binaries.
func TestReplaceInTreeRespectsBinaryExts(t *testing.T) {
	const body = "hello world"
	dir := t.TempDir()
	text := filepath.Join(dir, "a.yml")
	binary := filepath.Join(dir, "img.png")
	if err := os.WriteFile(text, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	n := ReplaceInTree(dir, "hello", "hi", nil)
	if n != 1 {
		t.Fatalf("expected 1 file replaced, got %d", n)
	}
	got, _ := os.ReadFile(binary)
	if string(got) != body {
		t.Fatalf("binary file was modified: %q", string(got))
	}
}

func TestEnsureGitignoreLine_AppendsToExistingFile(t *testing.T) {
	dir := t.TempDir()
	gitignore := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("# OS\n.DS_Store\nThumbs.db\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGitignoreLine(dir, ".gh-optivem/"); err != nil {
		t.Fatalf("EnsureGitignoreLine: %v", err)
	}
	body, _ := os.ReadFile(gitignore)
	want := "# OS\n.DS_Store\nThumbs.db\n.gh-optivem/\n"
	if string(body) != want {
		t.Errorf("got %q, want %q", string(body), want)
	}
}

func TestEnsureGitignoreLine_CreatesFileWhenMissing(t *testing.T) {
	dir := t.TempDir()
	if err := EnsureGitignoreLine(dir, ".gh-optivem/"); err != nil {
		t.Fatalf("EnsureGitignoreLine: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read created .gitignore: %v", err)
	}
	if string(body) != ".gh-optivem/\n" {
		t.Errorf("got %q, want %q", string(body), ".gh-optivem/\n")
	}
}

func TestEnsureGitignoreLine_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	for range 3 {
		if err := EnsureGitignoreLine(dir, ".gh-optivem/"); err != nil {
			t.Fatalf("EnsureGitignoreLine: %v", err)
		}
	}
	body, _ := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if string(body) != ".gh-optivem/\n" {
		t.Errorf("got %q, want a single .gh-optivem/ line", string(body))
	}
}

func TestEnsureGitignoreLine_RecognisesEquivalentSpellings(t *testing.T) {
	// The canonical spelling is `.gh-optivem/`. Existing `.gh-optivem`,
	// `/.gh-optivem`, and `/.gh-optivem/` all already-ignore the same
	// directory, so EnsureGitignoreLine treats them as no-ops.
	tests := []string{".gh-optivem", "/.gh-optivem", "/.gh-optivem/"}
	for _, existing := range tests {
		t.Run(existing, func(t *testing.T) {
			dir := t.TempDir()
			gitignore := filepath.Join(dir, ".gitignore")
			if err := os.WriteFile(gitignore, []byte("# preamble\n"+existing+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := EnsureGitignoreLine(dir, ".gh-optivem/"); err != nil {
				t.Fatalf("EnsureGitignoreLine: %v", err)
			}
			body, _ := os.ReadFile(gitignore)
			if got := string(body); got != "# preamble\n"+existing+"\n" {
				t.Errorf("file should be unchanged when %q already present, got %q", existing, got)
			}
		})
	}
}

func TestEnsureGitignoreLine_AppendsNewlineWhenMissing(t *testing.T) {
	dir := t.TempDir()
	gitignore := filepath.Join(dir, ".gitignore")
	// Existing file with no trailing newline — append must add one before
	// the new entry so we don't glue lines together.
	if err := os.WriteFile(gitignore, []byte(".DS_Store"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGitignoreLine(dir, ".gh-optivem/"); err != nil {
		t.Fatalf("EnsureGitignoreLine: %v", err)
	}
	body, _ := os.ReadFile(gitignore)
	want := ".DS_Store\n.gh-optivem/\n"
	if string(body) != want {
		t.Errorf("got %q, want %q", string(body), want)
	}
}

func TestEnsureGitignoreLine_IgnoresCommentLines(t *testing.T) {
	// A `# .gh-optivem/` comment is not the same as the entry itself —
	// the helper must still append the active rule.
	dir := t.TempDir()
	gitignore := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(gitignore, []byte("# .gh-optivem/ — TODO enable\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureGitignoreLine(dir, ".gh-optivem/"); err != nil {
		t.Fatalf("EnsureGitignoreLine: %v", err)
	}
	body, _ := os.ReadFile(gitignore)
	want := "# .gh-optivem/ — TODO enable\n.gh-optivem/\n"
	if string(body) != want {
		t.Errorf("got %q, want %q", string(body), want)
	}
}
