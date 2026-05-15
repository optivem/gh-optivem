// Package markdown implements tracker.Tracker against a local board
// laid out as a tree of `<board>/<status>/<id>.md` files. It is the
// escape hatch for operators who can't reach GitHub — air-gapped
// workshops, GitHub outages, or just exercising the pipeline offline.
//
// Layout:
//
//	<boardDir>/
//	  ready/        # PickReady picks the lexicographically smallest .md
//	  in-progress/
//	  in-acceptance/
//	  done/
//
// The status column an item sits in is determined by its directory.
// SetStatus performs a `git mv` between status dirs; reorderings
// happen in the user's editor by renaming files (filenames sort
// ascending; PickReady returns the topmost name). Verify checks the
// four canonical dirs exist; other dirs are created on demand.
//
// Issue.ID is the filename without `.md`. Issue.Title is the file's
// first `# H1` heading with a filename fallback. Issue.URL is "" —
// markdown items aren't URL-addressable from the public web; the
// adapter routes everything through file paths. Issue.Handle is the
// absolute file path, so SetStatus / Classify / ReadSections /
// MarkChecklistComplete address the file directly.
//
// All git mutations go through a GitRunner — the default shells out to
// the real `git` CLI; tests inject a fake. MarkChecklistComplete
// auto-commits so the working tree stays clean after the call.
package markdown

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker/internal/parse"
)

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

// Tracker is the markdown adapter's implementation of tracker.Tracker.
// Constructed via New with a board root directory and an optional
// GitRunner. The board root may be a relative path; it is resolved to
// an absolute path during New so subsequent calls don't depend on the
// process's working directory.
type Tracker struct {
	boardDir string
	git      GitRunner
}

// GitRunner runs `git` against the board directory. The default
// implementation is execGit, which shells out to the real `git` CLI
// with the board directory as -C. Tests inject fakes to avoid
// requiring git to be installed and to assert argv.
type GitRunner interface {
	// Run executes `git <args...>` with the working directory set to
	// the board root. Output is returned verbatim on success; on
	// failure, the returned error includes the trimmed stderr so
	// callers see "fatal: …" or "error: …" directly.
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ---------------------------------------------------------------------------
// Errors
// ---------------------------------------------------------------------------

// ErrBoardDirMissing is returned by Verify when one of the canonical
// status subdirectories (ready/, in-progress/, in-acceptance/, done/)
// is absent. Surfaced by preflight so the operator sees "create the
// board dirs first" rather than a downstream "no such file" deep in
// the pipeline.
var ErrBoardDirMissing = errors.New("markdown: board directory missing required subdir")

// ---------------------------------------------------------------------------
// Canonical statuses
// ---------------------------------------------------------------------------

// statusDirs are the four canonical status subdirectories Verify
// requires to exist. Other status names are accepted by SetStatus
// (the target dir is created on demand), but these four are the
// minimum a markdown board declares up-front.
var statusDirs = []string{"ready", "in-progress", "in-acceptance", "done"}

// statusDirName normalises a canonical status name ("Ready",
// "In progress", "In acceptance", "Done") into its directory form
// ("ready", "in-progress", "in-acceptance", "done"). The
// normalisation is just `lowercase + spaces→hyphens`; SetStatus
// accepts any status the operator names and routes it through this
// rule so adding a new column means creating a directory and nothing
// else.
func statusDirName(status string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(status)), " ", "-")
}

// ---------------------------------------------------------------------------
// Constructor
// ---------------------------------------------------------------------------

// New constructs a markdown.Tracker bound to the given board root.
// boardDir must point at a directory that exists; the canonical
// status subdirectories underneath it are checked separately by
// Verify (or auto-created by SetStatus on first use). git==nil falls
// back to the real `git` CLI via execGit.
func New(boardDir string, git GitRunner) (*Tracker, error) {
	if boardDir == "" {
		return nil, fmt.Errorf("markdown: boardDir is required")
	}
	abs, err := filepath.Abs(boardDir)
	if err != nil {
		return nil, fmt.Errorf("markdown: resolve boardDir: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("markdown: boardDir %q: %w", abs, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("markdown: boardDir %q is not a directory", abs)
	}
	if git == nil {
		git = execGit{dir: abs}
	}
	return &Tracker{boardDir: abs, git: git}, nil
}

// ---------------------------------------------------------------------------
// Tracker interface — workflow
// ---------------------------------------------------------------------------

// PickReady returns the topmost item in the ready/ directory, sorted
// by filename ascending. Files whose name starts with `.` are ignored.
// Returns tracker.ErrEmptyReady when the ready/ directory exists but
// contains no .md files.
func (t *Tracker) PickReady(_ context.Context) (tracker.Issue, error) {
	readyDir := filepath.Join(t.boardDir, "ready")
	matches, err := filepath.Glob(filepath.Join(readyDir, "*.md"))
	if err != nil {
		return tracker.Issue{}, fmt.Errorf("markdown: glob ready: %w", err)
	}
	matches = filterVisible(matches)
	if len(matches) == 0 {
		return tracker.Issue{}, tracker.ErrEmptyReady
	}
	slices.Sort(matches)
	return t.issueFromFile(matches[0])
}

// FindIssue accepts either an ID (filename sans `.md`, e.g.
// "001-add-cart") or a file path (absolute or relative to the board
// root). When an ID is supplied, every status directory is walked
// looking for a matching file. Returns an error wrapping the input
// when no file matches.
func (t *Tracker) FindIssue(_ context.Context, idOrPath string) (tracker.Issue, error) {
	if idOrPath == "" {
		return tracker.Issue{}, fmt.Errorf("markdown: FindIssue requires an ID or path")
	}
	// Path form: contains a separator or ends in .md.
	if strings.ContainsAny(idOrPath, `/\`) || strings.HasSuffix(idOrPath, ".md") {
		abs := idOrPath
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(t.boardDir, idOrPath)
		}
		abs = filepath.Clean(abs)
		if _, err := os.Stat(abs); err != nil {
			return tracker.Issue{}, fmt.Errorf("markdown: file %q: %w", idOrPath, err)
		}
		return t.issueFromFile(abs)
	}
	// ID form: walk the board for a matching filename.
	target := idOrPath + ".md"
	entries, err := os.ReadDir(t.boardDir)
	if err != nil {
		return tracker.Issue{}, fmt.Errorf("markdown: read board dir: %w", err)
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		candidate := filepath.Join(t.boardDir, e.Name(), target)
		if _, err := os.Stat(candidate); err == nil {
			return t.issueFromFile(candidate)
		}
	}
	return tracker.Issue{}, fmt.Errorf("markdown: issue %q not found under %s", idOrPath, t.boardDir)
}

// SetStatus moves the file identified by handle into the directory
// matching the requested status. The destination directory is created
// on demand. The move uses `git mv` so the rename is recorded as a
// rename (preserving history) and lands in the staged tree; the
// operator commits at their cadence.
func (t *Tracker) SetStatus(ctx context.Context, handle, status string) error {
	if handle == "" {
		return fmt.Errorf("markdown: handle is required")
	}
	if status == "" {
		return fmt.Errorf("markdown: status is required")
	}
	dir := statusDirName(status)
	if dir == "" {
		return fmt.Errorf("markdown: status %q normalises to empty directory name", status)
	}
	src, err := filepath.Abs(handle)
	if err != nil {
		return fmt.Errorf("markdown: resolve handle: %w", err)
	}
	dstDir := filepath.Join(t.boardDir, dir)
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return fmt.Errorf("markdown: mkdir %s: %w", dstDir, err)
	}
	dst := filepath.Join(dstDir, filepath.Base(src))
	if src == dst {
		return nil
	}
	srcRel, err := filepath.Rel(t.boardDir, src)
	if err != nil {
		return fmt.Errorf("markdown: relpath src: %w", err)
	}
	dstRel, err := filepath.Rel(t.boardDir, dst)
	if err != nil {
		return fmt.Errorf("markdown: relpath dst: %w", err)
	}
	if _, err := t.git.Run(ctx, "mv", srcRel, dstRel); err != nil {
		return fmt.Errorf("markdown: git mv %s %s: %w", srcRel, dstRel, err)
	}
	return nil
}

// Verify checks that the board root contains the four canonical
// status subdirectories. Each missing dir is reported with its full
// path so the operator can mkdir the exact thing the adapter wants.
func (t *Tracker) Verify(_ context.Context) error {
	var missing []string
	for _, d := range statusDirs {
		p := filepath.Join(t.boardDir, d)
		info, err := os.Stat(p)
		if err != nil || !info.IsDir() {
			missing = append(missing, d)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: %s (under %s)",
			ErrBoardDirMissing,
			strings.Join(missing, ", "),
			t.boardDir,
		)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Tracker interface — inspection / mutation
// ---------------------------------------------------------------------------

// Classify reads the file at i.Handle and returns its declared ticket
// kind. Source of truth is a YAML-ish frontmatter `type: <value>`
// field at the top of the file; the lowercased value is returned with
// confident=true. When no frontmatter type is present, a filename
// heuristic checks the prefix before the first `-` against a known
// set (feature, bug, story, task, chore, techdebt) — matches return
// (kind, false); non-matches return ("", false).
func (t *Tracker) Classify(_ context.Context, i tracker.Issue) (string, bool, error) {
	body, err := os.ReadFile(i.Handle)
	if err != nil {
		return "", false, fmt.Errorf("markdown: read %q: %w", i.Handle, err)
	}
	if k := frontmatterType(string(body)); k != "" {
		return strings.ToLower(k), true, nil
	}
	if k := filenameKindHeuristic(i.Handle); k != "" {
		return k, false, nil
	}
	return "", false, nil
}

// ReadSections reads the file at i.Handle and returns its H2/H3
// sections matching headings, via the shared parse.ExtractSection
// walker. Absent headings map to "" so callers see a stable key set.
func (t *Tracker) ReadSections(_ context.Context, i tracker.Issue, headings []string) (map[string]string, error) {
	body, err := os.ReadFile(i.Handle)
	if err != nil {
		return nil, fmt.Errorf("markdown: read %q: %w", i.Handle, err)
	}
	s := string(body)
	out := make(map[string]string, len(headings))
	for _, h := range headings {
		out[h] = parse.ExtractSection(s, h)
	}
	return out, nil
}

// MarkChecklistComplete rewrites every `- [ ]` / `* [ ]` line in the
// file at i.Handle to its checked equivalent, then `git add` + `git
// commit` the file. Idempotent: a file with no unchecked items leaves
// the working tree untouched and skips the commit.
func (t *Tracker) MarkChecklistComplete(ctx context.Context, i tracker.Issue) error {
	body, err := os.ReadFile(i.Handle)
	if err != nil {
		return fmt.Errorf("markdown: read %q: %w", i.Handle, err)
	}
	s := string(body)
	if !parse.HasUnchecked(s) {
		return nil
	}
	updated := parse.TickCheckboxes(s)
	if err := os.WriteFile(i.Handle, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("markdown: write %q: %w", i.Handle, err)
	}
	rel, err := filepath.Rel(t.boardDir, i.Handle)
	if err != nil {
		return fmt.Errorf("markdown: relpath %q: %w", i.Handle, err)
	}
	if _, err := t.git.Run(ctx, "add", rel); err != nil {
		return fmt.Errorf("markdown: git add %s: %w", rel, err)
	}
	msg := fmt.Sprintf("checklist: tick remaining items for %s", i.ID)
	if _, err := t.git.Run(ctx, "commit", "-m", msg, "--", rel); err != nil {
		return fmt.Errorf("markdown: git commit %s: %w", rel, err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// issueFromFile builds a tracker.Issue from a file path. Title is the
// first H1 in the file; on absence (or read error) the filename sans
// .md is used so callers always see a non-empty title.
func (t *Tracker) issueFromFile(absPath string) (tracker.Issue, error) {
	id := strings.TrimSuffix(filepath.Base(absPath), ".md")
	title := id
	if body, err := os.ReadFile(absPath); err == nil {
		if h1 := parse.FirstH1(string(body)); h1 != "" {
			title = h1
		}
	}
	return tracker.Issue{
		ID:     id,
		Title:  title,
		URL:    "",
		Handle: absPath,
	}, nil
}

// filterVisible drops paths whose basename starts with a dot —
// hidden files (e.g. `.DS_Store`) and editor swap files shouldn't
// surface as pickable items.
func filterVisible(paths []string) []string {
	out := paths[:0]
	for _, p := range paths {
		if strings.HasPrefix(filepath.Base(p), ".") {
			continue
		}
		out = append(out, p)
	}
	return out
}

// frontmatterTypePattern matches a `type: <value>` line inside a
// `---`-fenced YAML frontmatter block at the top of the file. Only
// the first occurrence is consulted; the regex is intentionally
// strict (anchored to start of body) so a `type:` mention later in
// the document doesn't fool Classify.
var frontmatterTypePattern = regexp.MustCompile(`(?s)\A---\s*\n(.*?)\n---\s*\n`)
var frontmatterTypeLine = regexp.MustCompile(`(?m)^type:\s*(\S+)\s*$`)

// frontmatterType extracts the value of a `type:` field from a YAML
// frontmatter block at the top of body. Returns "" when no
// frontmatter block is present or no `type:` line is inside it.
func frontmatterType(body string) string {
	fm := frontmatterTypePattern.FindStringSubmatch(body)
	if fm == nil {
		return ""
	}
	m := frontmatterTypeLine.FindStringSubmatch(fm[1])
	if m == nil {
		return ""
	}
	return m[1]
}

// knownKinds is the closed set of ticket kinds the filename heuristic
// will accept. Keeping this closed prevents an arbitrary prefix
// ("notes-..", "wip-..") from masquerading as a recognised kind.
var knownKinds = map[string]struct{}{
	"feature":  {},
	"bug":      {},
	"story":    {},
	"task":     {},
	"chore":    {},
	"techdebt": {},
}

// filenameKindHeuristic returns the prefix before the first `-` in
// the file's basename if it matches knownKinds, else "". Used by
// Classify as a fallback when no frontmatter type is set.
func filenameKindHeuristic(absPath string) string {
	base := strings.TrimSuffix(filepath.Base(absPath), ".md")
	idx := strings.Index(base, "-")
	if idx <= 0 {
		return ""
	}
	prefix := strings.ToLower(base[:idx])
	if _, ok := knownKinds[prefix]; ok {
		return prefix
	}
	return ""
}

// ---------------------------------------------------------------------------
// Default runner
// ---------------------------------------------------------------------------

type execGit struct {
	dir string
}

func (g execGit) Run(ctx context.Context, args ...string) ([]byte, error) {
	full := append([]string{"-C", g.dir}, args...)
	cmd := exec.CommandContext(ctx, "git", full...)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}
