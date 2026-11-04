package kickoff

import "path/filepath"

// Repo-relative locations the kickoff flow reads and writes, in slash form.
const (
	ProductFile  = "docs/product.md"
	PrototypeDir = "docs/prototype"
	DiagramFile  = "docs/use-cases.md"
	UseCasesDir  = "docs/use-cases"
	ProgressFile = "gh-optivem-progress.yaml"
	// StoriesDraft is gitignored local run data: the story-writer agent drops
	// its drafts here and the command validates them before creating issues.
	StoriesDraft = ".gh-optivem/kickoff/stories"
	// StoryLabel marks every issue kickoff creates, so "stories created" can be
	// derived from the tracker and autopilot knows which issues to run.
	StoryLabel = "kickoff"

	// sampleUseCaseFile is the shop sample's use case narrative. Its id
	// (UC-001) would collide with the user's first use case, so the use-cases
	// phase deletes it before the narratives are written.
	sampleUseCaseFile = "docs/use-cases/UC-001-place-order.md"
)

// Layout resolves repo-relative kickoff paths against a repo root.
type Layout struct{ Root string }

// Abs joins a slash-form repo-relative path onto the repo root.
func (l Layout) Abs(rel string) string { return filepath.Join(l.Root, filepath.FromSlash(rel)) }
