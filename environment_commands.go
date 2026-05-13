// environment_commands.go wires the `gh optivem environment <verb>` subtree.
// The `environment` noun owns operations on the real-world environment
// variables gh-optivem consumes — credentials and other inputs alike (e.g.
// DOCKERHUB_USERNAME is an account name, not a token, but is still required
// from the environment). Today the only verb is `verify`; future verbs (e.g.
// `environment list` to show which env vars are configured) would slot in as
// siblings.
//
//	gh optivem environment verify — live auth-check every credential the CLI
//	                                consumes from the environment, returning
//	                                non-zero on any missing or rejected value.
//
// Designed to be invoked from a CI preflight job so a single broken input
// surfaces once, before a scaffolding matrix fans out and burns runner
// minutes failing the same way N times.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/log"
)

// newEnvironmentCmd builds the `gh optivem environment` parent. The parent
// has no Run, so invoking it without a subcommand prints help (Cobra default).
func newEnvironmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "environment",
		Short: "Operate on the environment variables gh-optivem consumes",
		Long: `Operate on the real-world environment variables gh-optivem consumes.
Subcommands run read-only validation against the current environment and
exit non-zero on any failure. No repos, secrets, or releases are mutated.`,
	}
	cmd.AddCommand(
		newEnvironmentVerifyCmd(),
	)
	return cmd
}

// newEnvironmentVerifyCmd implements `gh optivem environment verify`. Reads
// every input the gh-acceptance pipeline uses from the environment, checks
// presence, and runs a live auth call against each provider in parallel.
// Exits 0 on full success; exits 1 with an aggregated error listing every
// missing or rejected value.
func newEnvironmentVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Verify every gh-acceptance pipeline environment variable is valid",
		Long: `Verify every environment variable the gh-optivem CLI consumes is present
and (for credentials) accepted by its provider:

  DOCKERHUB_USERNAME  — read from env, used for the Docker Hub login call
  DOCKERHUB_TOKEN     — POST hub.docker.com/v2/users/login
  SONAR_TOKEN         — GET sonarcloud.io/api/authentication/validate
  GHCR_TOKEN          — GET api.github.com/user (and read:packages scope)
  WORKFLOW_TOKEN      — GET api.github.com/user (and repo scope)
  REPO_TOKEN          — GET api.github.com/user (and repo scope)

All checks run in parallel; on any failure the command prints every broken
variable and exits non-zero, so the user fixes them in one pass instead of
fix-one-retry-discover-next.`,
		Example: `  gh optivem environment verify`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			// Initialize logging with sane defaults so the auth-check helpers'
			// log.Info / log.Successf calls produce output. No log file — this
			// is a short-lived CI preflight, not a scaffolding run.
			if err := log.Init(false, false, ""); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			defer log.Close()

			if err := config.VerifyEnvironment(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			log.Successf("All environment variables valid.")
		},
	}
}
