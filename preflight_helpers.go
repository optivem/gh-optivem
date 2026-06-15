// preflight_helpers.go builds the wired-up preflight.Options the cobra
// layer passes to preflight.Run. Both `gh optivem implement` and
// `gh optivem config preflight` go through this helper so the two
// surfaces share one definition of "what real remote checks does
// preflight run" — adding or removing a remote check class touches one
// place, not two.
//
// Token requirements:
//   - GitHub: relies on whatever auth `gh` CLI is already configured with
//     (`gh auth login` or $GH_TOKEN / $GITHUB_TOKEN). No extra plumbing.
//   - SonarCloud: requires $SONAR_TOKEN whenever cfg.Sonar.Organization
//     is set. Missing token is a hard error so preflight never silently
//     skips a check class the operator's config implies should run.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/preflight"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/statemachine"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker/factory"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
	"github.com/optivem/gh-optivem/internal/kernel/shell"
)

// defaultPreflightOptions returns preflight.Options wired to the real
// remote clients: shell.RepoExists for GitHub repo lookups, a SonarCloud
// client for org + project checks, and tracker.Verify for the project
// board URL — opened via the factory so the github/markdown dispatch
// happens in one place. The closure captures cfg.Project so it can
// pass both provider and url to factory.Open; preflight.Options's
// BoardURLOK signature still receives the URL but it's the same value
// already inside cfg.Project, so the captured copy stays authoritative.
//
// The embedded state machine is wired here too (opts.Engine), so the
// scope-resolution and BPMN-suite-existence sweeps run on both surfaces that
// build their options through this helper — `gh optivem implement` and
// `gh optivem config preflight`. That is the whole point of the shared
// builder: one definition of "ready to implement", validated identically
// wherever it's checked.
//
// Returns an error when cfg declares a SonarCloud setup (sonar.organization
// non-empty) but $SONAR_TOKEN is not in the environment — strict on
// purpose, so preflight cannot pass while silently skipping the Sonar
// remote contract. Also errors if the embedded state machine fails to load.
func defaultPreflightOptions(cfg *projectconfig.Config, workspace, cwd string) (preflight.Options, error) {
	var project projectconfig.Project
	if cfg != nil {
		project = cfg.Project
	}
	opts := preflight.Options{
		Workspace: workspace,
		Cwd:       cwd,
		RepoExists: func(_ context.Context, slug string) (bool, error) {
			// shell.RepoExists currently shells out via the package-level
			// Run (no ctx plumbing); drop ctx here rather than reworking
			// internal/shell to accept a context.Context everywhere.
			return shell.RepoExists(slug)
		},
		BoardURLOK: func(ctx context.Context, _ string) error {
			tr, err := factory.Open(ctx, project)
			if err != nil {
				return err
			}
			return tr.Verify(ctx)
		},
	}
	// Load the embedded state machine so preflight sweeps every writing-agent
	// MID's read/write scope and the BPMN-required test suites against cfg
	// before any agent runs. The driver re-loads internally — process-flow.yaml
	// is small enough that the second read is free.
	eng, err := statemachine.LoadDefault()
	if err != nil {
		return preflight.Options{}, fmt.Errorf("preflight: load state machine: %w", err)
	}
	opts.Engine = eng
	if cfg == nil || cfg.Sonar.Organization == "" {
		return opts, nil
	}
	token := os.Getenv("SONAR_TOKEN")
	if token == "" {
		return preflight.Options{}, fmt.Errorf(
			"preflight: SONAR_TOKEN is not set but sonar.organization=%q is declared in gh-optivem.yaml; export SONAR_TOKEN=<your-token>",
			cfg.Sonar.Organization)
	}
	sc := shell.NewSonarCloud(token, cfg.Sonar.Organization)
	opts.SonarOrgExists = sc.OrgExists
	opts.SonarProjectExists = sc.ProjectExists
	return opts, nil
}
