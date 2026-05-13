// preflight_helpers.go builds the wired-up preflight.Options the cobra
// layer passes to preflight.Run. Both `gh optivem atdd implement-ticket`
// and `gh optivem config preflight` go through this helper so the two
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

	"github.com/optivem/gh-optivem/internal/atdd/runtime/board"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/preflight"
	"github.com/optivem/gh-optivem/internal/projectconfig"
	"github.com/optivem/gh-optivem/internal/shell"
)

// defaultPreflightOptions returns preflight.Options wired to the real
// remote clients: shell.RepoExists for GitHub repo lookups, a SonarCloud
// client for org + project checks, and board.VerifyProjectURL for the
// project board URL.
//
// Returns an error when cfg declares a SonarCloud setup (sonar.organization
// non-empty) but $SONAR_TOKEN is not in the environment — strict on
// purpose, so preflight cannot pass while silently skipping the Sonar
// remote contract.
func defaultPreflightOptions(cfg *projectconfig.Config, workspace, cwd string) (preflight.Options, error) {
	opts := preflight.Options{
		Workspace: workspace,
		Cwd:       cwd,
		RepoExists: func(_ context.Context, slug string) (bool, error) {
			// shell.RepoExists currently shells out via the package-level
			// Run (no ctx plumbing); drop ctx here rather than reworking
			// internal/shell to accept a context.Context everywhere.
			return shell.RepoExists(slug)
		},
		BoardURLOK: func(ctx context.Context, projectURL string) error {
			return board.VerifyProjectURL(ctx, projectURL, nil)
		},
	}
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
