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
