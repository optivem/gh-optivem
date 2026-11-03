// environment_commands.go wires the `gh optivem environment` subtree. The
// `environment` noun owns the local shell environment the gh-optivem CLI reads
// from — credentials and other inputs alike (e.g.
// DOCKERHUB_USERNAME is an account name, not a token, but is still required
// from the environment).
//
//	gh optivem environment show   — print each env var, with token values
//	                                 masked, so you can confirm what your
//	                                 shell is exporting without leaking the
//	                                 secret to terminal scrollback.
//	gh optivem environment verify — check every credential and required
//	                                 local tool (gh CLI auth, actionlint),
//	                                 returning non-zero on any missing or
//	                                 rejected value.
//	gh optivem environment setup  — install the missing tools and fix PATH.
//	                                 The only subcommand that writes.
//
// `verify` is designed to be invoked from a CI preflight job so a single
// broken input surfaces once, before a scaffolding matrix fans out and burns
// runner minutes failing the same way N times. `show` is the local
// counterpart — judgment-free output for humans debugging their setup.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/kernel/log"
)

// langFlagUsage is the --lang help shared by verify and setup.
const langFlagUsage = "Languages whose toolchains to check or install: java, dotnet, typescript " +
	"(comma-separated or repeated). Omit for the language-agnostic tools only."

// newEnvironmentCmd builds the `gh optivem environment` parent. The parent
// has no Run, so invoking it without a subcommand prints help (Cobra default).
func newEnvironmentCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "environment",
		Short: "Inspect or set up the local environment gh-optivem needs",
		Long: `Inspect or set up the local environment gh-optivem needs.

  show    read-only: print each credential variable (masked)
  verify  read-only: check tools and credentials, exit non-zero on any failure
  setup   WRITES: install missing tools and fix PATH on this machine

No repos, secrets, or releases are mutated by any of them.`,
	}
	cmd.AddCommand(
		newEnvironmentShowCmd(),
		newEnvironmentVerifyCmd(),
		newEnvironmentSetupCmd(),
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
accepted by its provider.

Provide these via your shell, or via the user-level .env file gh-optivem loads
at startup ($GH_OPTIVEM_ENV_FILE if set, else <user-config-dir>/gh-optivem/.env).
A real exported variable always wins; the file only fills ones that are unset,
so editing it takes effect on the next run with no terminal restart.`,
		Example: `  gh optivem environment show`,
		Args:    cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			config.ShowEnvironment()
		},
	}
}

// newEnvironmentVerifyCmd implements `gh optivem environment verify`. Reads
// every input the gh-acceptance pipeline uses from the environment, checks
// presence, and runs a live auth call against each provider in parallel.
// Exits 0 on full success; exits 1 with an aggregated error listing every
// missing or rejected value.
//
// The --lang flag opts in to per-language compiler-presence checks
// (npm / dotnet / java). It's opt-in (not auto-detected from gh-optivem.yaml)
// so a CI preflight job can pin one matrix combo without coupling this
// command to the project-config schema or cwd state. Without --lang, only
// the language-agnostic tools (gh, actionlint) and tokens are checked.
//
// The bash and docker checks are NOT gated on anything: every scaffold shells
// out through bash and runs its system locally through `docker compose`,
// whatever it deploys to, so both sit in the always-run set alongside gh and
// actionlint. There is no --deploy flag here; when a target-specific tool
// lands (gcloud for cloud-run), it gets its own opt-in flag then.
func newEnvironmentVerifyCmd() *cobra.Command {
	var langs []string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify the local environment is ready (read-only; never installs anything)",
		Long: `Verify the local environment is ready to run the gh-acceptance pipeline.
This only reads - it changes nothing on your machine. To install what is
missing, run 'gh optivem environment setup'.

Checks that
required local tools are installed and authenticated, and every credential
the CLI consumes is present and accepted by its provider.

  gh CLI auth         — gh CLI installed and ` + "`gh auth status`" + ` succeeds
  actionlint          — actionlint binary on PATH
  bash                — bash binary on PATH (run-sonar.sh, and every shell
                        command the system / test runners issue)
  docker              — docker binary on PATH (the local system runs on
                        ` + "`docker compose`" + `, whatever you deploy to)
  claude              — claude binary on PATH (every ATDD agent runs as a
                        ` + "`claude`" + ` subprocess). Presence only — sign-in is
                        not probeable. Set GH_OPTIVEM_SKIP_CLAUDE_CHECK=1 on
                        CI runners that scaffold but never run ` + "`implement`" + `.
  DOCKERHUB_USERNAME  — read from env, used for the Docker Hub login call
  DOCKERHUB_TOKEN     — POST hub.docker.com/v2/users/login
  SONAR_TOKEN         — GET sonarcloud.io/api/authentication/validate
  GHCR_TOKEN          — GET api.github.com/user (and read:packages scope),
                        then the real ghcr.io OCI token exchange
  WORKFLOW_TOKEN      — GET api.github.com/user (and repo + workflow scopes)
  REPO_TOKEN          — GET api.github.com/user (and repo scope)
  npm                 — required when --lang includes typescript
  dotnet              — required when --lang includes dotnet
  java                — required when --lang includes java

All checks run in parallel; on any failure the command prints every broken
check and exits non-zero, so the user fixes them in one pass instead of
fix-one-retry-discover-next.

The npm / dotnet / java compiler checks only run when --lang is passed. Omit
it to check just the language-agnostic tools and tokens.`,
		Example: `  gh optivem environment verify
  gh optivem environment verify --lang typescript
  gh optivem environment verify --lang typescript,dotnet,java`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			// Initialize logging with sane defaults so the auth-check helpers'
			// log.Info / log.Successf calls produce output. No log file — this
			// is a short-lived CI preflight, not a scaffolding run.
			if err := log.Init(false, false, ""); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			defer log.Close()

			// Validate --lang up front. Aggregated rejection (every bad
			// value surfaced at once) lives in config.ValidateVerifyFlags
			// so the rejection paths can be exercised by unit tests; this
			// surface stays responsible only for printing + exit.
			if err := config.ValidateVerifyFlags(langs); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			if err := config.VerifyEnvironment(langs); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			log.Successf("All environment variables valid.")
		},
	}
	cmd.Flags().StringSliceVar(&langs, "lang", nil, langFlagUsage)
	return cmd
}

// newEnvironmentSetupCmd implements `gh optivem environment setup`. The writing
// half of the verify/setup pair: installs what is installable, edits the user
// PATH where upstream installers decline to, then reports what is left.
//
// --lang is parsed and validated exactly as `verify` does (same flag text,
// same config.ValidateVerifyFlags), so the two commands cannot disagree about
// which toolchains a given --lang value names.
func newEnvironmentSetupCmd() *cobra.Command {
	var langs []string
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Install the missing local tools (writes to your machine)",
		Long: `Install the local tools gh-optivem needs and fix your PATH, then report what
is left for you to do. Unlike 'verify', this command WRITES to your machine.

Installs, when missing: Git Bash, Go, actionlint, Claude Code, and (with
--lang) Node.js, the .NET SDK and a Java JDK. Where an installer declines to
edit PATH (Claude Code's %USERPROFILE%\.local\bin, Go's GOPATH/bin), setup adds
the directory to your user PATH and prints exactly what it changed.

Automated on Windows via winget. On other platforms setup installs nothing and
prints the manual install pointer for each missing tool.

It finishes by running 'verify' and listing only what a human must still do:
Docker Desktop, 'gh auth login', signing in to Claude Code, and credentials.
Open a new shell afterwards so every program sees the updated PATH.`,
		Example: `  gh optivem environment setup
  gh optivem environment setup --lang typescript
  gh optivem environment setup --lang typescript,dotnet,java`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if err := log.Init(false, false, ""); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			defer log.Close()

			if err := config.ValidateVerifyFlags(langs); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}

			if err := config.SetupEnvironment(langs); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		},
	}
	cmd.Flags().StringSliceVar(&langs, "lang", nil, langFlagUsage)
	return cmd
}
