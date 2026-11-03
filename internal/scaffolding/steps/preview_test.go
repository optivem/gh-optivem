package steps

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for p, c := range files {
		full := filepath.Join(dir, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func previewByPath(t *testing.T, scaffold, repo map[string]string) map[string]PreviewEntry {
	t.Helper()
	got, err := BuildPreview(writeTree(t, scaffold), writeTree(t, repo))
	if err != nil {
		t.Fatal(err)
	}
	m := make(map[string]PreviewEntry, len(got))
	for _, e := range got {
		m[e.Path] = e
	}
	return m
}

func TestBuildPreview_Classes(t *testing.T) {
	got := previewByPath(t,
		map[string]string{
			"new.txt":       "x\n",
			"same.txt":      "a\nb\n",
			"changed.txt":   "a\nB\nc\nd\n",
			"sub/deep.txt":  "1\n",
			".git/ignored":  "scaffold git",
			"sub/other.txt": "o\n",
		},
		map[string]string{
			"same.txt":     "a\nb\n",
			"changed.txt":  "a\nb\nc\n",
			"sub/deep.txt": "1\n",
			"unrelated.md": "keep me\n",
			".git/config":  "repo git",
		})

	want := map[string]PathClass{
		"new.txt":       ClassNew,
		"sub/other.txt": ClassNew,
		"same.txt":      ClassIdentical,
		"sub/deep.txt":  ClassIdentical,
		"changed.txt":   ClassOverwrite,
		"unrelated.md":  ClassKept,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries %v, want %d", len(got), got, len(want))
	}
	for p, c := range want {
		if got[p].Class != c {
			t.Errorf("%s: class %s, want %s", p, got[p].Class, c)
		}
	}
	ch := got["changed.txt"]
	if ch.Added != 2 || ch.Removed != 1 {
		t.Errorf("changed.txt counts +%d/-%d, want +2/-1", ch.Added, ch.Removed)
	}
}

func TestBuildPreview_LineEndingsOnlyIsIdentical(t *testing.T) {
	got := previewByPath(t,
		map[string]string{"a.txt": "one\ntwo\n", "b.txt": "one\ntwo"},
		map[string]string{"a.txt": "one\r\ntwo\r\n", "b.txt": "one\r\ntwo\r\n"})
	for _, p := range []string{"a.txt", "b.txt"} {
		if got[p].Class != ClassIdentical {
			t.Errorf("%s: class %s, want IDENTICAL", p, got[p].Class)
		}
	}
}

func TestBuildPreview_Binary(t *testing.T) {
	got := previewByPath(t,
		map[string]string{"same.bin": "\x00\x01\r\n", "diff.bin": "\x00\x02", "text-vs-bin": "\x00x"},
		map[string]string{"same.bin": "\x00\x01\r\n", "diff.bin": "\x00\x01", "text-vs-bin": "plain\n"})
	if c := got["same.bin"]; c.Class != ClassIdentical || !c.Binary {
		t.Errorf("same.bin: %+v", c)
	}
	if c := got["diff.bin"]; c.Class != ClassOverwrite || !c.Binary || c.Added != 0 || c.Removed != 0 {
		t.Errorf("diff.bin: %+v", c)
	}
	if c := got["text-vs-bin"]; c.Class != ClassOverwrite || !c.Binary {
		t.Errorf("text-vs-bin: %+v", c)
	}
}

func TestBuildPreview_BinaryNotLineEndingNormalised(t *testing.T) {
	got := previewByPath(t,
		map[string]string{"x.bin": "\x00a\nb"},
		map[string]string{"x.bin": "\x00a\r\nb"})
	if got["x.bin"].Class != ClassOverwrite {
		t.Errorf("binary CRLF-vs-LF must differ, got %s", got["x.bin"].Class)
	}
}

func TestBuildPreview_RenameIsNewPlusKept(t *testing.T) {
	got := previewByPath(t,
		map[string]string{"docs/new-name.md": "same\n"},
		map[string]string{"docs/old-name.md": "same\n"})
	if got["docs/new-name.md"].Class != ClassNew {
		t.Errorf("new-name: %s", got["docs/new-name.md"].Class)
	}
	if got["docs/old-name.md"].Class != ClassKept {
		t.Errorf("old-name: %s", got["docs/old-name.md"].Class)
	}
}

func TestBuildPreview_CaseOnlyRename(t *testing.T) {
	got := previewByPath(t,
		map[string]string{"README.md": "same\n", "Docs/Guide.md": "new\n"},
		map[string]string{"readme.md": "same\n", "docs/guide.md": "old\n"})

	if len(got) != 2 {
		t.Fatalf("case-only matches must not also yield KEPT/NEW entries, got %v", got)
	}
	r := got["README.md"]
	if r.Class != ClassIdentical || r.ExistingPath != "readme.md" {
		t.Errorf("README.md: %+v", r)
	}
	g := got["Docs/Guide.md"]
	if g.Class != ClassOverwrite || g.ExistingPath != "docs/guide.md" || g.Added != 1 || g.Removed != 1 {
		t.Errorf("Docs/Guide.md: %+v", g)
	}
}

func TestBuildPreview_MissingDirFails(t *testing.T) {
	if _, err := BuildPreview(t.TempDir(), filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected error for missing repo dir")
	}
}

func TestDiffCounts(t *testing.T) {
	cases := []struct {
		name         string
		old, next    []string
		added, remov int
	}{
		{"equal", []string{"a"}, []string{"a"}, 0, 0},
		{"insert middle", []string{"a", "c"}, []string{"a", "b", "c"}, 1, 0},
		{"delete all", []string{"a", "b"}, nil, 0, 2},
		{"replace", []string{"a", "b", "c"}, []string{"a", "x", "c"}, 1, 1},
		{"reorder", []string{"a", "b"}, []string{"b", "a"}, 1, 1},
	}
	for _, c := range cases {
		a, r := diffCounts(c.old, c.next)
		if a != c.added || r != c.remov {
			t.Errorf("%s: +%d/-%d, want +%d/-%d", c.name, a, r, c.added, c.remov)
		}
	}
}
