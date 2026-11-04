// kickoff_commands.go wires `gh optivem kickoff`: the step after `init` that
// takes a scaffolded repo from the shop sample to the user's own product. One
// public command resumes at the next incomplete phase (description, prototype,
// use case diagram, use cases, shop removal, stories); the phases are also
// reachable through the hidden `kickoff phase <name>` subcommands for tests and
// debugging.
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/atdd/process/clauderun"
	"github.com/optivem/gh-optivem/internal/kernel/approval"
	"github.com/optivem/gh-optivem/internal/kernel/cmdctx"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
	"github.com/optivem/gh-optivem/internal/kickoff"
)

type kickoffFlags struct {
	description     string
	descriptionFile string
	feedback        string
	approve         bool
	redo            string
	autopilot       bool
	status          bool
}

func newKickoffCmd() *cobra.Command {
	f := &kickoffFlags{}
	cmd := &cobra.Command{
		Use:   "kickoff",
		Short: "Move a scaffolded repo from the shop sample to your own product",
		Long: `Take a freshly scaffolded repo (green on the shop sample) to your own product.

One command, run repeatedly: it reads what is already done and continues at the
next phase. Each phase ends at a checkpoint where you review and approve.

  1. description       docs/product.md: a few informal sentences about your product
  2. prototype         a clickable HTML prototype in docs/prototype/; give feedback until you approve it
  3. use-case-diagram  docs/use-cases.md: screen to use case table, use case diagram, external systems
  4. use-cases         docs/use-cases/UC-NNN-<slug>.md with UC-NNN.M acceptance criteria
  5. remove-sample     deletes the shop examples, keeping the test architecture
  6. stories           one ticket per use case slice, ready for 'gh optivem implement'

Approvals are recorded in gh-optivem-progress.yaml (commit it). Editing approved
content later marks that phase and every later one as needing re-approval.

By default stories are then built one at a time with 'gh optivem implement'
(attended). --autopilot instead runs every story unattended through the ATDD
flow, stopping only on a failure.`,
		Example: `  gh optivem kickoff --description "A booking site for small yoga studios: studios publish classes, members book and cancel."
  gh optivem kickoff                              # resume at the next phase
  gh optivem kickoff --approve                    # approve the phase waiting at its checkpoint
  gh optivem kickoff --feedback "make the booking form a single step"
  gh optivem kickoff --redo prototype             # reopen the prototype; later approvals are dropped
  gh optivem kickoff --autopilot                  # build all stories unattended
  gh optivem kickoff --status                     # show progress only`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			exitIfComponentKind("gh optivem kickoff", "it replaces the shop sample of a system scaffold with the user's own product, and a kind: component project has no sample to replace")
			exitOnError(runKickoff(cmd, f))
		},
	}
	cmd.Flags().StringVar(&f.description, "description", "", "Product description (a few informal sentences); written to docs/product.md")
	cmd.Flags().StringVar(&f.descriptionFile, "description-file", "", "Read the product description from this file; written to docs/product.md")
	cmd.Flags().StringVar(&f.feedback, "feedback", "", "Feedback for the agent of the phase waiting at its checkpoint, in plain language")
	cmd.Flags().BoolVar(&f.approve, "approve", false, "Approve the phase waiting at its checkpoint without prompting")
	cmd.Flags().StringVar(&f.redo, "redo", "", "Reopen an approval phase (prototype, use-case-diagram, use-cases); every later approval is dropped")
	cmd.Flags().BoolVar(&f.autopilot, "autopilot", false, "After the stories exist, run every story through the ATDD flow unattended")
	cmd.Flags().BoolVar(&f.status, "status", false, "Print the progress of each phase and exit")

	phase := &cobra.Command{
		Use:    "phase <name>",
		Short:  "Run a single kickoff phase (debugging and tests)",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			ph, err := kickoff.ParsePhase(args[0])
			exitOnError(err)
			opts, err := buildKickoffOptions(cmd, f)
			exitOnError(err)
			exitOnError(kickoff.RunPhase(cmd.Context(), opts, ph))
		},
	}
	cmd.AddCommand(phase)
	return cmd
}

func runKickoff(cmd *cobra.Command, f *kickoffFlags) error {
	opts, err := buildKickoffOptions(cmd, f)
	if err != nil {
		return err
	}
	if f.redo != "" {
		ph, err := kickoff.ParseRedoPhase(f.redo)
		if err != nil {
			return err
		}
		opts.Redo = ph
	}
	if f.status {
		prog, err := kickoff.LoadProgress(opts.Layout)
		if err != nil {
			return err
		}
		st, err := kickoff.Detect(opts.Layout, prog, kickoff.Probes{})
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), st.Describe())
		return nil
	}
	return kickoff.Run(cmd.Context(), opts)
}

func buildKickoffOptions(cmd *cobra.Command, f *kickoffFlags) (kickoff.Options, error) {
	cfg, cfgPath, err := loadProjectConfigForRunner()
	if err != nil {
		return kickoff.Options{}, err
	}
	root := filepath.Dir(cfgPath)
	layout := kickoff.Layout{Root: root}

	pub, err := kickoffPublisher(cfg, root)
	if err != nil {
		return kickoff.Options{}, err
	}
	user, err := kickoffUser(root)
	if err != nil {
		return kickoff.Options{}, err
	}
	resolved := cmdctx.Approval(cmd)
	return kickoff.Options{
		Layout:      layout,
		Stdin:       os.Stdin,
		Stdout:      cmd.OutOrStdout(),
		Interactive: isatty.IsTerminal(os.Stdin.Fd()),
		Feedback:    f.feedback,
		Approve:     f.approve,
		Autopilot:   f.autopilot,
		Description: kickoff.DescriptionInput{Text: f.description, File: f.descriptionFile},
		Agents: kickoff.AgentRunner{
			Layout: layout,
			Claude: clauderun.NewClaudeRunner(),
			Stdout: cmd.OutOrStdout(),
			Stderr: cmd.ErrOrStderr(),
		},
		Remove:    kickoff.DefaultRemove,
		Publisher: pub,
		Implement: kickoff.ExecImplement(root, cmd.OutOrStdout(), cmd.ErrOrStderr()),
		Confirm: func(prompt string) (bool, error) {
			return approval.Confirm(resolved, approval.CategoryHuman, os.Stdin, cmd.OutOrStdout(), prompt)
		},
		User: user,
	}, nil
}

// kickoffPublisher picks the tracker adapter for the project. Stories are
// created in the acceptance-test repo (the repo `implement` works in).
func kickoffPublisher(cfg *projectconfig.Config, root string) (kickoff.Publisher, error) {
	switch cfg.Project.Provider {
	case projectconfig.ProviderGitHub:
		repo := cfg.SystemTest.Repo
		if repo == "" {
			repos := cfg.Repos()
			if len(repos) == 0 {
				return nil, fmt.Errorf("gh-optivem.yaml declares no repo to create stories in (system-test.repo)")
			}
			repo = repos[0]
		}
		return kickoff.GitHubPublisher{GH: kickoff.ExecGH{}, Repo: repo, ProjectURL: cfg.Project.URL}, nil
	case projectconfig.ProviderMarkdown:
		board := cfg.Project.URL
		if !filepath.IsAbs(board) {
			board = filepath.Join(root, board)
		}
		return kickoff.MarkdownPublisher{BoardDir: board}, nil
	}
	return nil, fmt.Errorf("project.provider %q is not supported by kickoff (want %q or %q)", cfg.Project.Provider, projectconfig.ProviderGitHub, projectconfig.ProviderMarkdown)
}

// kickoffUser names the approver recorded in gh-optivem-progress.yaml.
func kickoffUser(root string) (string, error) {
	out, err := exec.Command("git", "-C", root, "config", "user.name").Output()
	if name := strings.TrimSpace(string(out)); err == nil && name != "" {
		return name, nil
	}
	return "", fmt.Errorf("git config user.name is not set: kickoff records who approved each phase")
}
