// spec_generate_commands.go wires `gh optivem spec generate`: readable
// Gherkin acceptance criteria generated from the tagged acceptance tests
// (plan 20261109-1636). The output is printed or written to --out and is
// never committed.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
	"github.com/optivem/gh-optivem/internal/spec"
	"github.com/optivem/gh-optivem/internal/spec/generate"
)

// atTestPathKey is the gh-optivem.yaml system-test path holding the
// acceptance tests, the default for --tests.
const atTestPathKey = "at-test"

var useCaseIDRe = regexp.MustCompile(`^UC-\d+$`)

type specGenerateOptions struct {
	Config   string
	Tests    []string
	UseCases []string
	Out      string
}

func newSpecGenerateCmd() *cobra.Command {
	opts := specGenerateOptions{}
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate readable Gherkin acceptance criteria from the tagged acceptance tests",
		Long: `Read the acceptance-test sources (.java, .cs, *.spec.ts / *.test.ts), turn
each test's scenario DSL chain into a Gherkin scenario and print one Feature
per use case. Each scenario carries the test's @UC-NNN.M tag; a test with
several IDs appears under each. Parameterised tests become Scenario Outlines,
wip tests carry @wip, and untagged tests go in a trailing block (their count
is noted on stderr).

Use-case titles come from the use cases in spec.yaml's specDir; a criterion
whose use case is not there is an error. The tests are read from --tests
(repeatable), or by default from system-test.paths.at-test in the
gh-optivem.yaml next to spec.yaml. spec.yaml's exclude globs apply.

All or nothing: a test whose chain cannot be parsed fails the run with
file:line and the test name, and nothing is printed or written. The output
is generated, not a source of truth; do not commit it.

Exit codes: 0 ok, 1 unparseable tests or unknown IDs, 2 usage or configuration error.`,
		Example: `  gh optivem spec generate
  gh optivem spec generate --use-case UC-001 --out criteria.feature
  gh optivem spec generate --tests system-test/java/src/test/java/com/acme/latest/acceptance`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runSpecCmd(func() (int, error) { return runSpecGenerate(opts, cmd.OutOrStdout(), cmd.ErrOrStderr()) })
		},
	}
	cmd.Flags().StringVar(&opts.Config, "config", defaultSpecConfig, "Path to spec.yaml")
	cmd.Flags().StringSliceVar(&opts.Tests, "tests", nil, "Acceptance-test directory or file (repeatable); default: system-test.paths.at-test from gh-optivem.yaml")
	cmd.Flags().StringSliceVar(&opts.UseCases, "use-case", nil, "Only generate this use case, e.g. UC-001 (repeatable)")
	cmd.Flags().StringVar(&opts.Out, "out", "", "Write the Gherkin to this file instead of stdout")
	return cmd
}

func runSpecGenerate(opts specGenerateOptions, out, errOut io.Writer) (int, error) {
	for _, id := range opts.UseCases {
		if !useCaseIDRe.MatchString(id) {
			return 0, fmt.Errorf("--use-case %q: want a use-case ID such as UC-001", id)
		}
	}
	cfg, err := spec.LoadConfig(opts.Config)
	if err != nil {
		return 0, err
	}
	base := filepath.Dir(opts.Config)
	useCases, err := spec.LoadDir(resolveUnder(base, cfg.SpecDir))
	if err != nil {
		return 0, err
	}
	dirs := opts.Tests
	if len(dirs) == 0 {
		if dirs, err = defaultTestDirs(base); err != nil {
			return 0, err
		}
	}
	for _, d := range dirs {
		if _, err := os.Stat(d); err != nil {
			return 0, fmt.Errorf("test dir: %w", err)
		}
	}
	res, err := generate.Generate(generate.Options{TestDirs: dirs, UseCases: useCases, Only: opts.UseCases, Exclude: cfg.Exclude})
	if err != nil {
		fmt.Fprintf(errOut, "spec generate: some tests could not be turned into criteria; nothing was written:\n%v\n", err)
		return spec.ExitGateFailed, nil
	}
	if n := len(res.Untagged); n > 0 {
		fmt.Fprintf(errOut, "NOTE %d untagged acceptance test(s), rendered under %q:\n", n, generate.UntaggedFeature)
		for _, u := range res.Untagged {
			fmt.Fprintf(errOut, "  %s\n", u)
		}
	}
	if opts.Out != "" {
		if err := os.WriteFile(opts.Out, []byte(res.Gherkin), 0o644); err != nil {
			return 0, fmt.Errorf("write %s: %w", opts.Out, err)
		}
		return spec.ExitPass, nil
	}
	_, err = io.WriteString(out, res.Gherkin)
	return spec.ExitPass, err
}

// defaultTestDirs reads system-test.paths.at-test from the gh-optivem.yaml in
// base (the spec.yaml directory); its paths are relative to that directory.
func defaultTestDirs(base string) ([]string, error) {
	pc, err := projectconfig.Load(base)
	if err != nil {
		return nil, err
	}
	if pc == nil {
		return nil, fmt.Errorf("no --tests given and no %s in %s to read system-test.paths.%s from", projectconfig.Path, base, atTestPathKey)
	}
	p := pc.SystemTest.Paths[atTestPathKey]
	if p == "" {
		return nil, fmt.Errorf("no --tests given and %s has no system-test.paths.%s", filepath.Join(base, projectconfig.Path), atTestPathKey)
	}
	return []string{resolveUnder(base, p)}, nil
}
