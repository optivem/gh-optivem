package kickoff

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Fingerprint returns the approval fingerprint of the content ph produces:
// the prototype directory, the use case diagram file, or the use case
// narratives. It is a SHA-256 over the sorted repo-relative paths and file
// contents, with line endings normalised so a checkout with autocrlf does not
// invalidate an approval.
func Fingerprint(l Layout, ph Phase) (string, error) {
	var files []string
	switch ph {
	case PhasePrototype:
		walked, err := walkFiles(l.Abs(PrototypeDir))
		if err != nil {
			return "", err
		}
		files = walked
	case PhaseDiagram:
		if _, err := os.Stat(l.Abs(DiagramFile)); err == nil {
			files = []string{l.Abs(DiagramFile)}
		}
	case PhaseUseCases:
		entries, err := filepath.Glob(filepath.Join(l.Abs(UseCasesDir), "*.md"))
		if err != nil {
			return "", err
		}
		files = entries
	default:
		return "", fmt.Errorf("phase %s has no approval fingerprint", ph)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("nothing to fingerprint for phase %s", ph)
	}
	sort.Strings(files)
	h := sha256.New()
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return "", fmt.Errorf("fingerprint %s: %w", ph, err)
		}
		rel, err := filepath.Rel(l.Root, f)
		if err != nil {
			return "", err
		}
		h.Write([]byte(filepath.ToSlash(rel)))
		h.Write([]byte{0})
		h.Write([]byte(strings.ReplaceAll(string(data), "\r\n", "\n")))
		h.Write([]byte{0})
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

func walkFiles(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) && p == dir {
				return nil
			}
			return err
		}
		if !d.IsDir() {
			out = append(out, p)
		}
		return nil
	})
	return out, err
}
