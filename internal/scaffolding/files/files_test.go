package files

import (
	"os"
	"path/filepath"
	"strings"
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

// TestRenameDirsInTreeRenamesMatchingDirs is the happy-path baseline: a single
// unambiguous rename succeeds and reports one directory renamed.
func TestRenameDirsInTreeRenamesMatchingDirs(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "foo-old")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}

	n, err := RenameDirsInTree(root, "old", "new")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 1 {
		t.Fatalf("renamed = %d, want 1", n)
	}
	if _, err := os.Stat(filepath.Join(root, "foo-new")); err != nil {
		t.Fatalf("expected renamed dir foo-new to exist: %v", err)
	}
}

// TestRenameDirsInTreeFailsLoudOnCollision is the regression test for issue
// #60: a repeat scaffold run leaves a stale "foo-old" tree from the first
// run alongside a freshly-copied "foo-new" tree from the second. Renaming
// foo-old -> foo-new then collides with the existing destination. Before
// this fix, os.Rename's error was discarded (`if err == nil { count++ }`)
// and the corrupted, un-renamed tree silently persisted alongside the new
// one, producing duplicate classes downstream. Now the collision must
// surface as an error naming both paths.
func TestRenameDirsInTreeFailsLoudOnCollision(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "foo-old")
	dst := filepath.Join(root, "foo-new")
	if err := os.MkdirAll(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		t.Fatal(err)
	}
	// Non-empty destination guarantees os.Rename fails cross-platform.
	if err := os.WriteFile(filepath.Join(dst, "marker.txt"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := RenameDirsInTree(root, "old", "new")
	if err == nil {
		t.Fatal("RenameDirsInTree: want an error on destination collision, got nil")
	}
	for _, want := range []string{src, dst} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention path %q", err, want)
		}
	}
}
