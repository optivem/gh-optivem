// Package tracker is the seam between the ATDD pipeline driver and the
// concrete issue-tracker backend (GitHub Projects today, markdown files
// for the local/offline path, Jira tomorrow).
//
// The Tracker interface is the union of every tracker-shaped operation
// the runtime issues — picking the next ready item, moving it through
// status columns, inspecting its body for classification and section
// content, ticking checklists. Each backend implements the same seven
// methods; the runtime never branches on backend type.
//
// Open is the factory: it dispatches on projectconfig.Project.Provider
// ("github" | "markdown") and validates that the configured URL fits the
// chosen adapter's expected shape. Both fields are surfaced in the error
// when the dispatch fails so the operator sees exactly which line of
// gh-optivem.yaml to fix.
//
// Status: scaffold. Open() returns ErrNotImplemented today; the github
// and markdown adapters are wired in subsequent migration steps. The
// interface itself is stable — the per-method semantics are documented
// inline and a fake implementation in tracker_test.go exercises the
// contract at compile time.
package tracker

import (
	"context"
	"errors"

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// Issue is the backend-agnostic representation of a tracker item. Every
// adapter populates ID and Title; URL is populated when the backend
// addresses items by URL (GitHub, Jira) and is empty for the markdown
// adapter (file paths are reachable via FindIssue but not as URLs).
//
// Handle is the opaque per-backend payload that the runtime carries
// between calls without inspecting. The github adapter encodes its
// "projectID:itemID" pair into Handle so SetStatus can pass them back
// to the Projects API; the markdown adapter encodes the absolute file
// path. The runtime threads Handle through Context as a single
// `issue_handle` string.
//
// Repo is intentionally NOT a field. The two flows that historically
// needed an owner/name pair (gh issue view --repo …) are subsumed by
// Tracker.Classify, Tracker.ReadSections, and Tracker.MarkChecklistComplete
// — the adapter knows where the issue lives because it produced the Issue.
type Issue struct {
	ID     string
	Title  string
	URL    string
	Handle string
}

// Tracker is the seven-method interface every backend implements. The
// methods divide into three groups:
//
//   - Workflow:  PickReady, FindIssue, SetStatus, Verify
//   - Inspection: Classify, ReadSections
//   - Mutation:  MarkChecklistComplete
//
// Adapters are constructed via Open (or directly via package
// constructors in tests). All methods accept a context.Context and
// return errors so cancellation and per-call timeouts work uniformly.
type Tracker interface {
	// PickReady returns the topmost item with status "Ready" on the
	// configured project. Returns ErrEmptyReady when the Ready column
	// is empty (a normal "nothing to do" outcome, not an I/O error).
	PickReady(ctx context.Context) (Issue, error)

	// FindIssue resolves an issue by its backend-native ID OR by its
	// URL form. Both shapes are accepted; the adapter chooses the
	// parse path. Returns an error wrapping the input when no item
	// matches.
	FindIssue(ctx context.Context, idOrURL string) (Issue, error)

	// SetStatus moves the item identified by handle to the named
	// status. Status names are the canonical column labels
	// ("Ready", "In progress", "In acceptance", "Done"); adapters
	// map them to backend-native mechanics (single-select option for
	// GitHub Projects, target directory for markdown).
	SetStatus(ctx context.Context, handle, status string) error

	// Verify checks that the configured project / board exists and
	// is reachable. For github, this is a minimal projectV2 GraphQL
	// lookup. For markdown, it stats the configured directory. Used
	// by preflight to surface "you declared a board that doesn't
	// exist or auth can't see it" up-front.
	Verify(ctx context.Context) error

	// Classify returns the issue's ticket kind (e.g. "feature",
	// "bug", "tech debt") and a confidence flag. The github adapter
	// reads the project's Type field plus a label-token table; the
	// markdown adapter reads YAML frontmatter `type:` with a filename
	// heuristic fallback.
	Classify(ctx context.Context, i Issue) (kind string, confident bool, err error)

	// ReadSections parses the issue body and returns the named
	// sections (H2/H3 headings → body text). Section names that are
	// not present in the body map to "" (empty string), not absent
	// keys — callers can distinguish "missing" from "blank" only
	// when they care to.
	ReadSections(ctx context.Context, i Issue, headings []string) (map[string]string, error)

	// MarkChecklistComplete rewrites every `- [ ]` line in the
	// issue body to `- [x]`. The github adapter pushes the rewrite
	// via gh issue edit; the markdown adapter rewrites the file in
	// place AND auto-commits the change so the working tree stays
	// clean afterwards.
	MarkChecklistComplete(ctx context.Context, i Issue) error
}

// ---------------------------------------------------------------------------
// Sentinel errors
// ---------------------------------------------------------------------------

// ErrEmptyReady is returned by PickReady when the Ready column has no
// items. A normal "nothing to do" outcome — callers usually report it
// and stop, not retry.
var ErrEmptyReady = errors.New("tracker: Ready column is empty")

// ErrNotImplemented is returned by stubbed methods and by the Open
// factory until the per-adapter wiring lands. Distinct from "not
// supported by this backend" — that case returns a more specific
// error naming the operation and the backend.
var ErrNotImplemented = errors.New("tracker: not implemented")

// ---------------------------------------------------------------------------
// Factory
// ---------------------------------------------------------------------------

// Open constructs the Tracker matching cfg.Provider. Currently a stub
// that returns ErrNotImplemented for every call; the github and
// markdown adapters are wired in later migration steps. The signature
// is stable so consumers can adopt it before the dispatch lands.
//
// Contract once wired:
//   - cfg.Provider == "github":   github URL required, returns *github.Tracker
//   - cfg.Provider == "markdown": directory path required, returns *markdown.Tracker
//   - cfg.Provider == "":          error pointing at `gh optivem config migrate`
//   - cfg.Provider unknown:       error naming both the value and the field
//   - provider/url shape mismatch: error naming both fields
func Open(_ context.Context, _ projectconfig.Project) (Tracker, error) {
	return nil, ErrNotImplemented
}
