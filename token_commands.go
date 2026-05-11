// token_commands.go wires the `gh optivem token <verb>` subtree. The `token`
// noun owns operations on the real-world credentials gh-optivem consumes
// from the environment. Today the only verb is `verify`; future verbs (e.g.
// `token list` to show which credential env vars are configured) would slot
// in as siblings.
//
//	gh optivem token verify — live auth-check every credential the CLI
//	                          consumes from the environment, returning
//	                          non-zero on any missing or rejected token.
//
// Designed to be invoked from a CI preflight job so a single broken token
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

// newTokenCmd builds the `gh optivem token` parent. The parent has no Run,
// so invoking it without a subcommand prints help (Cobra default).
func newTokenCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "token",
		Short: "Operate on the credentials gh-optivem consumes from the environment",
		Long: `Operate on the real-world credentials gh-optivem consumes from the
environment. Subcommands run read-only validation against the current
environment and exit non-zero on any failure. No repos, secrets, or releases
are mutated.`,
	}
	cmd.AddCommand(
		newTokenVerifyCmd(),
	)
	return cmd
}

// newTokenVerifyCmd implements `gh optivem token verify`. Reads every
// credential the gh-acceptance pipeline uses from the environment, checks
// presence, and runs a live auth call against each provider in parallel.
// Exits 0 on full success; exits 1 with an aggregated error listing every
// missing or rejected token.
func newTokenVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Verify every gh-acceptance pipeline token is valid",
		Long: `Verify every credential the gh-optivem CLI consumes from the environment
is present and accepted by its provider:

  DOCKERHUB_USERNAME  — read from env, used for the Docker Hub login call
  DOCKERHUB_TOKEN     — POST hub.docker.com/v2/users/login
  SONAR_TOKEN         — GET sonarcloud.io/api/authentication/validate
  GHCR_TOKEN          — GET api.github.com/user (and read:packages scope)
  WORKFLOW_TOKEN      — GET api.github.com/user (and repo scope)
  REPO_TOKEN          — GET api.github.com/user (and repo scope)

All checks run in parallel; on any failure the command prints every broken
credential and exits non-zero, so the user fixes them in one pass instead
of fix-one-retry-discover-next.`,
		Example: `  gh optivem token verify`,
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

			if err := config.VerifyTokens(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			log.Successf("All tokens valid.")
		},
	}
}
