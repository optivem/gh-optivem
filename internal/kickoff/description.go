package kickoff

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	minDescriptionChars = 20
	minDescriptionWords = 4
	maxDescriptionChars = 20000
)

// ValidateDescription is the single check applied to the product description
// whichever way it arrives (flag, file, interactive prompt, or hand-written
// docs/product.md).
func ValidateDescription(text string) error {
	t := strings.TrimSpace(text)
	if t == "" {
		return errors.New("the product description is empty")
	}
	if len(t) < minDescriptionChars || len(strings.Fields(t)) < minDescriptionWords {
		return fmt.Errorf("the product description is too short: write at least a few sentences (minimum %d words, %d characters)", minDescriptionWords, minDescriptionChars)
	}
	if len(t) > maxDescriptionChars {
		return fmt.Errorf("the product description is too long (%d characters, maximum %d): keep it to a short informal description", len(t), maxDescriptionChars)
	}
	return nil
}

// DescriptionInput is where a new description comes from. At most one of Text
// and File may be set; with neither, Prompt (when non-nil) reads it
// interactively.
type DescriptionInput struct {
	Text   string
	File   string
	Prompt func() (string, error)
}

// CaptureDescription validates the description from the chosen source and
// writes it to docs/product.md. It refuses to overwrite an existing file: the
// description is edited in place.
func CaptureDescription(l Layout, in DescriptionInput) error {
	if in.Text != "" && in.File != "" {
		return errors.New("pass either --description or --description-file, not both")
	}
	path := l.Abs(ProductFile)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists: edit it directly instead of passing a new description", ProductFile)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("check %s: %w", ProductFile, err)
	}
	var text string
	switch {
	case in.Text != "":
		text = in.Text
	case in.File != "":
		data, err := os.ReadFile(in.File)
		if err != nil {
			return fmt.Errorf("read --description-file: %w", err)
		}
		text = string(data)
	case in.Prompt != nil:
		t, err := in.Prompt()
		if err != nil {
			return err
		}
		text = t
	default:
		return fmt.Errorf("no product description: write %s, or pass --description / --description-file, or run on a terminal to be prompted", ProductFile)
	}
	if err := ValidateDescription(text); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(text)+"\n"), 0o644)
}

// PromptDescription reads a multi-line description from r, ended by a blank
// line or end of input.
func PromptDescription(r io.Reader, w io.Writer) (string, error) {
	fmt.Fprintln(w, "Describe your product in a few sentences (finish with an empty line):")
	sc := bufio.NewScanner(r)
	var lines []string
	for sc.Scan() {
		line := sc.Text()
		if strings.TrimSpace(line) == "" && len(lines) > 0 {
			break
		}
		lines = append(lines, line)
	}
	if err := sc.Err(); err != nil {
		return "", fmt.Errorf("read description: %w", err)
	}
	return strings.Join(lines, "\n"), nil
}
