// Package release owns the end-of-cycle release mechanics for the ATDD
// pipeline driver: regex-remove `@Disabled`-style markers from in-scope test
// files, commit, and close the GitHub issue. It replaces the legacy
// `atdd-release` agent.
//
// The package is deliberately mechanical and small. It exposes three
// primitives:
//
//   - RemoveDisabledMarkers walks a set of roots, applies one regex per
//     language, and rewrites or deletes lines that match. Each language has
//     a built-in default Pattern; callers can override the full set via
//     RemoveOptions.Patterns.
//   - Commit shells out to `git add -A` + `git commit -m <msg>`. It REQUIRES
//     a non-nil Confirmer — there is no way to silently commit. Callers
//     that want non-interactive use must pass a Confirmer that auto-returns
//     true; the explicit handshake makes "skip the gate" visible at the
//     call site rather than buried in a flag.
//   - CloseIssue shells out to `gh issue close <N>`.
//
// Scope explicitly excluded in this package: Cobra wiring, auto-detection of
// in-scope test files from issue commits, and any driver-loop integration.
// Callers pass Roots explicitly.
package release

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/optivem/gh-optivem/internal/promptio"
)

// RemoveOptions controls a RemoveDisabledMarkers run.
type RemoveOptions struct {
	// Roots are directories to walk recursively, e.g.
	// ["system-test/java", "system-test/dotnet", "system-test/typescript"].
	// Each root is walked independently; missing roots are skipped (not an
	// error) so callers can pass all three language roots unconditionally.
	Roots []string

	// Patterns optionally overrides the built-in default patterns. Nil
	// (the common case) uses DefaultPatterns(). Callers passing a custom
	// slice replace the defaults entirely — there is no merge.
	Patterns []Pattern
}

// Pattern describes how to find and remove a disabled-marker in one
// language. A file is matched when its path satisfies Glob; once matched,
// every line matching Line is removed from the file. If LineRewrite is
// non-nil, matched lines are passed through it instead of removed (used by
// the .NET `[Fact(Skip = "…")]` rewrite, which keeps the attribute and
// drops only the Skip parameter).
//
// Imports is optional: if non-nil and the file has no remaining matches
// after removal, lines matching Imports are also removed (used to clean up
// `import org.junit.jupiter.api.Disabled;` once the last `@Disabled` is
// gone).
type Pattern struct {
	Name        string
	Glob        string
	Line        *regexp.Regexp
	LineRewrite func(string) string
	Imports     *regexp.Regexp
}

// FileChange records the per-file outcome of a RemoveDisabledMarkers pass.
type FileChange struct {
	Path         string
	LinesRemoved []int    // 1-indexed line numbers fully removed
	LinesEdited  []int    // 1-indexed line numbers rewritten in place (e.g. .NET Skip drop)
	PatternName  string
}

// Changes is the return type of RemoveDisabledMarkers.
type Changes struct {
	Files []FileChange
}

// Confirmer asks the user to approve a destructive action. Implementations
// return (true, nil) to proceed, (false, nil) to abort cleanly, or
// (false, err) on I/O failure. A nil Confirmer is rejected by Commit; the
// "ask before every commit" gate is firm user policy and the only way to
// opt out is to pass a Confirmer that auto-returns true.
type Confirmer func(prompt string) (bool, error)

// CommitOptions controls a Commit call.
type CommitOptions struct {
	Message   string
	Confirm   Confirmer // mandatory; nil → error.
	GitRunner GitRunner // optional injection point for tests; nil → real exec.
	Stdout    io.Writer // optional; nil → os.Stdout. Diff summary goes here.
}

// GitRunner is the test seam for `git` invocations. Tests pass a fake;
// production callers pass nil and Commit uses the real `exec` runner.
type GitRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// GhRunner is the test seam for `gh` invocations.
type GhRunner interface {
	Run(ctx context.Context, args ...string) ([]byte, error)
}

// ErrCommitDeclined is returned by Commit when the Confirmer returns false.
var ErrCommitDeclined = errors.New("release: commit declined by user")

// ErrConfirmerRequired is returned by Commit when CommitOptions.Confirm is
// nil. This guards the "ask before commit" policy at the type-system level.
var ErrConfirmerRequired = errors.New("release: Commit requires a Confirmer (no silent commits)")

// RemoveDisabledMarkers walks each root, matches files against each
// pattern's glob, and removes (or rewrites) marker lines per the pattern.
// Returns a Changes slice describing every file modified.
//
// Ordering: roots are walked in the order given; within each root files
// are visited in lexical order (the default of filepath.WalkDir). Patterns
// are applied in the order given; within a single file every pattern that
// matches the glob runs, in order, so a file matching multiple patterns
// (rare) accumulates the union of edits.
func RemoveDisabledMarkers(ctx context.Context, opts RemoveOptions) (Changes, error) {
	patterns := opts.Patterns
	if patterns == nil {
		patterns = DefaultPatterns()
	}
	var out Changes
	for _, root := range opts.Roots {
		// Missing root is a soft skip — callers commonly pass all three
		// language roots even when only one language is in use.
		info, err := os.Stat(root)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return Changes{}, fmt.Errorf("release: stat %s: %w", root, err)
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			for _, p := range patterns {
				ok, err := matchGlob(p.Glob, path)
				if err != nil {
					return fmt.Errorf("release: glob %q: %w", p.Glob, err)
				}
				if !ok {
					continue
				}
				change, err := applyPattern(path, p)
				if err != nil {
					return err
				}
				if change != nil {
					out.Files = append(out.Files, *change)
				}
			}
			return nil
		})
		if err != nil {
			return Changes{}, err
		}
		_ = ctx // ctx is reserved for future cancellation; walkDir is fast and local.
	}
	return out, nil
}

// applyPattern reads path, removes/rewrites marker lines per p, and writes
// the result back when there are changes. Returns nil when the file has no
// matches.
func applyPattern(path string, p Pattern) (*FileChange, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("release: read %s: %w", path, err)
	}
	// Preserve whether the file ended with a newline. After splitting on
	// '\n' a trailing newline produces a final empty element — track it so
	// we can re-emit identically on write-back.
	hadTrailingNewline := len(data) > 0 && data[len(data)-1] == '\n'
	body := string(data)
	if hadTrailingNewline {
		body = body[:len(body)-1]
	}
	lines := strings.Split(body, "\n")
	var (
		removed     []int
		edited      []int
		kept        = make([]string, 0, len(lines))
		matchCount  int
	)
	for i, line := range lines {
		if p.Line.MatchString(line) {
			matchCount++
			if p.LineRewrite != nil {
				rewritten := p.LineRewrite(line)
				if rewritten == line {
					// The pattern matched but the rewriter returned the
					// line unchanged (e.g. Skip parameter not actually
					// present). Treat as a no-op for this line.
					kept = append(kept, line)
					continue
				}
				if rewritten == "" {
					removed = append(removed, i+1)
					continue
				}
				edited = append(edited, i+1)
				kept = append(kept, rewritten)
				continue
			}
			removed = append(removed, i+1)
			continue
		}
		kept = append(kept, line)
	}

	// Import-line cleanup: only when at least one marker was removed AND
	// no Line matches remain in `kept`. matchCount > 0 above already means
	// the file had markers; we re-check kept to handle patterns that leave
	// markers behind (rare, but defensive).
	if p.Imports != nil && matchCount > 0 {
		stillHasMarker := false
		for _, line := range kept {
			if p.Line.MatchString(line) {
				stillHasMarker = true
				break
			}
		}
		if !stillHasMarker {
			cleaned := make([]string, 0, len(kept))
			for i, line := range kept {
				if p.Imports.MatchString(line) {
					removed = append(removed, originalLineNumber(lines, line, i, removed))
					continue
				}
				cleaned = append(cleaned, line)
			}
			kept = cleaned
		}
	}

	if len(removed) == 0 && len(edited) == 0 {
		return nil, nil
	}

	joined := strings.Join(kept, "\n")
	if hadTrailingNewline && len(kept) > 0 {
		joined += "\n"
	}
	if err := os.WriteFile(path, []byte(joined), 0o644); err != nil {
		return nil, fmt.Errorf("release: write %s: %w", path, err)
	}

	return &FileChange{
		Path:         path,
		LinesRemoved: removed,
		LinesEdited:  edited,
		PatternName:  p.Name,
	}, nil
}

// originalLineNumber best-effort maps a kept-array index back to the
// original-file line number. Imports cleanup is rare and the exact line
// number is informational, so this scans the original `lines` for the same
// text. If multiple originals match (unlikely for an import line), the
// first un-removed index wins.
func originalLineNumber(orig []string, target string, keptIdx int, alreadyRemoved []int) int {
	skip := make(map[int]bool, len(alreadyRemoved))
	for _, n := range alreadyRemoved {
		skip[n] = true
	}
	for i, line := range orig {
		if skip[i+1] {
			continue
		}
		if line == target {
			return i + 1
		}
	}
	return keptIdx + 1
}

// matchGlob applies a glob in the form "**/*.java" or "*.cs" against a
// file path. The "**" segment matches any depth; everything else is
// delegated to filepath.Match on the basename.
//
// Supported forms (sufficient for default patterns):
//
//	"**/*.java"    → any .java file at any depth
//	"**/*.cs"      → any .cs file at any depth
//	"**/*.spec.ts" → any .spec.ts file at any depth
//	"*.go"         → any .go file (basename only)
//
// Anything more elaborate (literal directory prefixes, multi-segment
// glob bodies) would require a real glob library; we deliberately stay
// simple and let callers pass exact globs.
func matchGlob(glob, path string) (bool, error) {
	// Normalize separators so windows paths work with forward-slash globs.
	p := filepath.ToSlash(path)
	g := strings.TrimSpace(glob)
	if strings.HasPrefix(g, "**/") {
		g = g[len("**/"):]
		base := filepath.Base(p)
		return filepath.Match(g, base)
	}
	return filepath.Match(g, filepath.Base(p))
}

// -------------------------------------------------------------------------
// Commit
// -------------------------------------------------------------------------

// Commit stages all working-tree changes (`git add -A`) and creates a
// commit with the given message — but only after the user explicitly
// approves via the supplied Confirmer.
//
// The "ask before every commit" gate is firm user policy across the
// academy; nil Confirm returns ErrConfirmerRequired so a forgetful caller
// can't accidentally bypass it.
func Commit(ctx context.Context, opts CommitOptions) error {
	if opts.Confirm == nil {
		return ErrConfirmerRequired
	}
	if strings.TrimSpace(opts.Message) == "" {
		return errors.New("release: Commit requires a non-empty Message")
	}
	runner := opts.GitRunner
	if runner == nil {
		runner = realGit{}
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = os.Stdout
	}

	// Stage everything. We deliberately use `git add -A` (full working-tree
	// stage) — the academy convention forbids slicing commits with --paths,
	// see CLAUDE.md / the "no --paths flag on commit script" memory.
	if _, err := runner.Run(ctx, "add", "-A"); err != nil {
		return fmt.Errorf("release: git add -A: %w", err)
	}

	// Show the user what they're about to commit. `git status --short`
	// gives a one-line-per-file summary that fits the Confirmer prompt.
	statusOut, err := runner.Run(ctx, "status", "--short")
	if err != nil {
		return fmt.Errorf("release: git status: %w", err)
	}
	fmt.Fprintln(stdout, "Staged changes:")
	if len(bytes.TrimSpace(statusOut)) == 0 {
		fmt.Fprintln(stdout, "  (none)")
	} else {
		fmt.Fprintln(stdout, string(statusOut))
	}
	fmt.Fprintf(stdout, "Commit message: %s\n", opts.Message)

	ok, err := opts.Confirm("Commit these changes?")
	if err != nil {
		return fmt.Errorf("release: confirmer: %w", err)
	}
	if !ok {
		return ErrCommitDeclined
	}

	if _, err := runner.Run(ctx, "commit", "-m", opts.Message); err != nil {
		return fmt.Errorf("release: git commit: %w", err)
	}
	return nil
}

// CloseIssue shells out to `gh issue close <N>`. The Confirmer policy is
// applied at the Commit boundary, not here — by convention closing an
// issue happens immediately after an already-approved final commit.
func CloseIssue(ctx context.Context, issueNum int, gh GhRunner) error {
	if issueNum <= 0 {
		return fmt.Errorf("release: CloseIssue requires a positive issue number, got %d", issueNum)
	}
	if gh == nil {
		gh = realGh{}
	}
	if _, err := gh.Run(ctx, "issue", "close", fmt.Sprintf("%d", issueNum)); err != nil {
		return fmt.Errorf("release: gh issue close %d: %w", issueNum, err)
	}
	return nil
}

// -------------------------------------------------------------------------
// Confirmer helper
// -------------------------------------------------------------------------

// InteractiveConfirmer returns a Confirmer backed by the shared promptio
// helper: the prompt is suffixed with " [y/n]: ", input is case-insensitive,
// unrecognised answers (including bare Enter) re-prompt, EOF returns false.
// See internal/promptio for the canonical semantics — every human y/n
// decision in the CLI funnels through that package.
func InteractiveConfirmer(stdin io.Reader, stdout io.Writer) Confirmer {
	if stdin == nil {
		stdin = os.Stdin
	}
	if stdout == nil {
		stdout = os.Stdout
	}
	return func(prompt string) (bool, error) {
		return promptio.ConfirmYN(stdin, stdout, prompt)
	}
}

// -------------------------------------------------------------------------
// Real exec runners
// -------------------------------------------------------------------------

type realGit struct{}

func (realGit) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("git %s: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

type realGh struct{}

func (realGh) Run(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("gh %s: %w (stderr: %s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
