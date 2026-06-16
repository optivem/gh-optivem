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
//   - SonarCloud: the live org/project checks need $SONAR_TOKEN; they are
//     wired only when it (and cfg.Sonar.Organization) is present.
//   - The full required credential set is presence-checked via
//     opts.MissingEnvVars, so every missing var (SONAR_TOKEN included)
//     surfaces together in preflight.Run's aggregated failure block rather
//     than one-at-a-time.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/optivem/gh-optivem/internal/atdd/process"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/preflight"
	"github.com/optivem/gh-optivem/internal/atdd/runtime/tracker/factory"
	"github.com/optivem/gh-optivem/internal/config"
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
// The full required credential set (SONAR_TOKEN, DOCKERHUB_USERNAME/_TOKEN,
// GHCR_TOKEN, WORKFLOW_TOKEN, REPO_TOKEN) is checked for presence via
// opts.MissingEnvVars, so every missing var surfaces together in
// preflight.Run's aggregated block — one shell restart fixes them all. A
// missing $SONAR_TOKEN therefore no longer hard-fails here; it simply
// leaves the Sonar remote checks unwired (the missing-var line is the
// single signal) so preflight never claims a Sonar check ran without a
// token. Returns an error only if the embedded state machine fails to load.
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
		// Presence-only check of the full required credential set. Folding it
		// into preflight.Run's aggregated block means a missing token lists
		// alongside any missing repo/tier/suite, so the operator fixes every
		// gap with a single shell restart instead of fix-one-restart-discover-next.
		MissingEnvVars: config.MissingRequiredEnvVars,
	}
	// Load the embedded state machine so preflight sweeps every writing-agent
	// MID's read/write scope and the BPMN-required test suites against cfg
	// before any agent runs. The driver re-loads internally — process-flow.yaml
	// is small enough that the second read is free.
	eng, err := process.Load()
	if err != nil {
		return preflight.Options{}, fmt.Errorf("preflight: load state machine: %w", err)
	}
	opts.Engine = eng

	// Wire the SonarCloud remote checks only when the config declares an org
	// AND $SONAR_TOKEN is present. A missing token no longer hard-fails here:
	// it is reported by opts.MissingEnvVars as one line in preflight.Run's
	// aggregated block. Skipping the remote wiring (rather than erroring)
	// keeps preflight from claiming the Sonar org/project checks ran when
	// there was no token to run them with — the missing-var line is the
	// single, honest signal.
	if cfg == nil || cfg.Sonar.Organization == "" {
		return opts, nil
	}
	if token := os.Getenv("SONAR_TOKEN"); token != "" {
		sc := shell.NewSonarCloud(token, cfg.Sonar.Organization)
		opts.SonarOrgExists = sc.OrgExists
		opts.SonarProjectExists = sc.ProjectExists
	}
	return opts, nil
}
