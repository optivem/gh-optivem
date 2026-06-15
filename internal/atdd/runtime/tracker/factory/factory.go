// Package factory wires projectconfig.Project values to concrete
// tracker.Tracker adapters. The dispatch lives in a sibling package
// rather than on the tracker package itself because the github and
// markdown adapters both import tracker for the Issue / Tracker /
// sentinel types — declaring Open inside tracker would re-import the
// adapters and create a build cycle.
//
// Open validates cfg.Provider against the known set, validates
// cfg.URL against the chosen adapter's expected shape (github wants
// an https://github.com/... URL; markdown wants a directory path),
// and returns the adapter ready to use. Provider-empty and unknown-
// provider errors name the field so the operator sees exactly which
// line of gh-optivem.yaml to fix; provider/URL shape mismatches name
// both fields. The empty-provider error also points at
// `gh optivem config migrate`, which idempotently adds the field
// from an existing project.url.
package factory

import (
	"context"
	"fmt"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker/github"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker/markdown"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// Open returns the Tracker adapter named by cfg.Provider, bound to
// cfg.URL. The github adapter parses cfg.URL as a projectV2 URL and
// reaches GitHub via the real `gh` CLI; the markdown adapter resolves
// cfg.URL as a directory and reaches `git` via the real CLI. Tests
// that need to inject a fake runner construct the adapter directly
// via the package-level New (this factory is the production wiring).
func Open(_ context.Context, cfg projectconfig.Project) (tracker.Tracker, error) {
	switch cfg.Provider {
	case projectconfig.ProviderGitHub:
		return github.New(cfg.URL, nil)
	case projectconfig.ProviderMarkdown:
		return markdown.New(cfg.URL, nil)
	case "":
		return nil, fmt.Errorf("tracker: project.provider is required (one of %q, %q); run `gh optivem config migrate` to add it from an existing project.url",
			projectconfig.ProviderGitHub, projectconfig.ProviderMarkdown)
	default:
		return nil, fmt.Errorf("tracker: unknown project.provider=%q (want %q or %q)",
			cfg.Provider, projectconfig.ProviderGitHub, projectconfig.ProviderMarkdown)
	}
}
