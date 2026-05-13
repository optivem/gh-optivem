// environment_commands.go wires the `gh optivem environment` subtree. The
// `environment` noun owns read-only operations on the local shell environment
// the gh-optivem CLI reads from — currently the six credential env vars the
// scaffolded pipeline needs as Actions variables/secrets.
//
//	gh optivem environment show   — print each credential env var, with token
//	                                 values masked, so you can confirm what
//	                                 your shell is exporting without leaking
//	                                 the secret to terminal scrollback.
//	gh optivem environment verify — live auth-check every credential against
//	                                 its provider, returning non-zero on any
//	                                 missing or rejected token.
//
// `verify` is designed to be invoked from a CI preflight job so a single
// broken token surfaces once, before a scaffolding matrix fans out and burns
// runner minutes failing the same way N times. `show` is the local
// counterpart — judgment-free output for humans debugging their setup.
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
		Short: "Inspect the local shell environment gh-optivem reads from",
		Long: `Inspect the local shell environment gh-optivem reads from. Subcommands run
read-only checks against the current process environment and exit non-zero
on any failure. No repos, secrets, or releases are mutated.`,
	}
	cmd.AddCommand(
		newEnvironmentShowCmd(),
		newEnvironmentVerifyCmd(),
	)
	return cmd
}

// newEnvironmentShowCmd implements `gh optivem environment show`. Prints the
// six credential env vars with token values masked to first-4/last-4 chars,
// USERNAME printed in full, and "NOT SET" for any that are empty. Always
// exits 0 — this is informational, not enforcement.
func newEnvironmentShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Print every credential env var (token values masked)",
		Long: `Print each credential environment variable gh-optivem reads. Token values
are masked to first-4/last-4 characters so the output is safe to paste or
screenshare; DOCKERHUB_USERNAME is printed in full. Variables that are not
set are reported as "NOT SET".

This is purely informational — no live calls are made and the command always
exits 0. Use 'gh optivem environment verify' to confirm each token is also
accepted by its provider.`,
		Example: `  gh optivem environment show`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			config.ShowEnvironment()
		},
	}
}

// newEnvironmentVerifyCmd implements `gh optivem environment verify`. Reads
// every credential the gh-acceptance pipeline uses from the environment,
// checks presence, and runs a live auth call against each provider in
// parallel. Exits 0 on full success; exits 1 with an aggregated error
// listing every missing or rejected token.
func newEnvironmentVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "verify",
		Short: "Verify every gh-acceptance pipeline credential is valid",
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

			if err := config.VerifyTokens(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			log.Successf("All tokens valid.")
		},
	}
}
