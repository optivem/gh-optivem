// Package tracker is the seam between the ATDD pipeline driver and the
// concrete issue-tracker backend (GitHub Projects today, markdown files
// for the local/offline path, Jira tomorrow).
//
// The Tracker interface is the union of every tracker-shaped operation
// the runtime issues — resolving a ticket by ID/URL, moving it through
// status columns, inspecting its body for classification and section
// content. Each backend implements the same six methods; the runtime
// never branches on backend type.
//
// Construction goes through the sibling factory package
// (internal/atdd/runtime/tracker/factory): factory.Open dispatches on
// projectconfig.Project.Provider ("github" | "markdown"), validates
// the configured URL against the adapter's expected shape, and
// returns the adapter. The factory lives in a sibling package because
// the github and markdown adapters both import this one for the
// Issue / Tracker types — declaring Open here would re-import the
// adapters and create a build cycle.
//
// Tests can construct adapters directly via the per-package New
// constructors with an injected runner; this package exposes only the
// interface and shared types, so it has no runtime behavior to mock.
package tracker

import "context"

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
// `issue-handle` string.
//
// Repo is intentionally NOT a field. The two flows that historically
// needed an owner/name pair (gh issue view --repo …) are subsumed by
// Tracker.Classify and Tracker.ReadSections — the adapter knows where
// the issue lives because it produced the Issue.
type Issue struct {
	ID     string
	Title  string
	URL    string
	Handle string
}

// Tracker is the six-method interface every backend implements. The
// methods divide into two groups:
//
//   - Workflow:   FindIssue, SetStatus, Verify
//   - Inspection: Classify, Subtypes, ReadSections
//
// Adapters are constructed via Open (or directly via package
// constructors in tests). All methods accept a context.Context and
// return errors so cancellation and per-call timeouts work uniformly.
type Tracker interface {
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

	// Subtypes returns every subtype value declared on the issue, in
	// declaration order. Used by the intake flow's subtype gate, which
	// treats count==1 as "unambiguous", 0 as "operator must declare",
	// and 2+ as "operator must reconcile". The github adapter reads
	// `subtype:<value>` labels and strips the prefix; the markdown
	// adapter reads a `subtype:` field from the file's YAML
	// frontmatter (single-element slice or empty).
	Subtypes(ctx context.Context, i Issue) ([]string, error)

	// ReadSections parses the issue body and returns the named
	// sections (H2/H3 headings → body text). Section names that are
	// not present in the body map to "" (empty string), not absent
	// keys — callers can distinguish "missing" from "blank" only
	// when they care to.
	ReadSections(ctx context.Context, i Issue, headings []string) (map[string]string, error)
}
