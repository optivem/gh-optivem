// spec_commands.go wires the `gh optivem spec` command group: the
// spec-coverage gate described in docs/spec-gate/convention.md. It derives a
// lifecycle state per acceptance criterion from the use-case markdown and the
// JUnit XML test results, and gates the pipeline on the outcome.
//
// Exit codes follow the convention: 0 pass, 1 gate failed, 2 usage or
// configuration error. Every configuration, parse or I/O failure is reported
// with its true cause and exits 2; nothing is folded into a benign default.
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/optivem/gh-optivem/internal/spec"
)

const defaultSpecConfig = "spec.yaml"

type specOptions struct {
	Config    string
	Mode      string
	Threshold float64
	JSON      bool
	HTML      string
	PRComment bool
	Flags     []string
}

func newSpecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Spec-coverage gate: derive acceptance-criterion states from tests and gate on them",
		Long: `Spec-coverage gate (see docs/spec-gate/convention.md).

Use-case markdown files define acceptance criteria with stable IDs (UC-001.3).
Acceptance tests carry those IDs as tags that surface as properties in JUnit
XML. The gate reads only the markdown and the XML and derives a lifecycle
state per criterion: Planned, In progress, Verified, Failing or Waived.

Configuration lives in spec.yaml (default: ./spec.yaml, override with --config).
Exit codes: 0 pass, 1 gate failed, 2 usage or configuration error.`,
		Example: `  gh optivem spec validate
  gh optivem spec check --mode lenient
  gh optivem spec check --mode strict --threshold 90 --html spec-report.html
  gh optivem spec adopt`,
	}
	cmd.AddCommand(newSpecCheckCmd(), newSpecStatusCmd(), newSpecValidateCmd(), newSpecAdoptCmd())
	return cmd
}

func addSpecConfigFlag(cmd *cobra.Command, opts *specOptions) {
	cmd.Flags().StringVar(&opts.Config, "config", defaultSpecConfig, "Path to spec.yaml")
}

func addSpecReportFlags(cmd *cobra.Command, opts *specOptions) {
	addSpecConfigFlag(cmd, opts)
	cmd.Flags().StringVar(&opts.Mode, "mode", spec.ModeLenient, "Gate mode: lenient (blocks on validation errors and Failing) or strict (also blocks on unverified in-scope criteria and coverage below the threshold)")
	cmd.Flags().Float64Var(&opts.Threshold, "threshold", 0, "Minimum spec coverage percent (0-100) for strict mode; overrides spec.yaml (default 100)")
	cmd.Flags().BoolVar(&opts.JSON, "json", false, "Print the JSON report (schema v1) instead of the terminal report")
	cmd.Flags().StringVar(&opts.HTML, "html", "", "Also write a self-contained HTML report to this path")
	cmd.Flags().BoolVar(&opts.PRComment, "pr-comment", false, "Print the sticky pull-request comment markdown instead of the terminal report (printed, never posted)")
	cmd.Flags().StringSliceVar(&opts.Flags, "flag", nil, "Feature flag considered on (repeatable); adds to flags: in spec.yaml")
}

// runSpecCmd turns a run function's result into the process exit code:
// an error is a usage or configuration error (2), otherwise the returned code.
func runSpecCmd(fn func() (int, error)) {
	code, err := fn()
	if err != nil {
		fmt.Fprintf(os.Stderr, errorFormat, err)
		os.Exit(spec.ExitUsage)
	}
	if code != 0 {
		os.Exit(code)
	}
}

func newSpecCheckCmd() *cobra.Command {
	opts := specOptions{}
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Evaluate the spec-coverage gate and exit non-zero when it fails",
		Long: `Read spec.yaml, the use-case markdown and the JUnit XML results, derive the
state of every criterion, print the report and apply the gate for --mode.

Also appends a Markdown summary to the file named by GITHUB_STEP_SUMMARY when
that environment variable is set. With --pr-comment the sticky pull-request
comment body (marker <!-- gh-optivem-spec -->) is printed instead of the
terminal report; posting it is left to the workflow.

Exit codes: 0 pass, 1 gate failed, 2 usage or configuration error.`,
		Example: `  gh optivem spec check
  gh optivem spec check --mode strict --threshold 95
  gh optivem spec check --json > spec-report.json
  gh optivem spec check --pr-comment --html spec-report.html`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runSpecCmd(func() (int, error) { return runSpecCheck(opts, cmd, cmd.OutOrStdout(), true) })
		},
	}
	addSpecReportFlags(cmd, &opts)
	return cmd
}

func newSpecStatusCmd() *cobra.Command {
	opts := specOptions{}
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show criterion states and coverage without failing on the gate verdict",
		Long: `Same report as 'check', but the exit code only reflects usage or
configuration errors (2). A failing gate verdict is shown, not enforced, so
this is safe for local use and dashboards.`,
		Example: `  gh optivem spec status
  gh optivem spec status --mode strict --html spec-report.html`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runSpecCmd(func() (int, error) { return runSpecCheck(opts, cmd, cmd.OutOrStdout(), false) })
		},
	}
	addSpecReportFlags(cmd, &opts)
	return cmd
}

func newSpecValidateCmd() *cobra.Command {
	opts := specOptions{}
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the use-case markdown only (no test results needed)",
		Long: `Parse the use-case markdown in specDir and fail on duplicate criterion IDs,
IDs whose prefix does not match the use case id, and missing or malformed
front matter. Does not read test results.

Exit codes: 0 valid, 1 validation errors, 2 usage or configuration error.`,
		Example: `  gh optivem spec validate
  gh optivem spec validate --config path/to/spec.yaml`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runSpecCmd(func() (int, error) { return runSpecValidate(opts, cmd.OutOrStdout()) })
		},
	}
	addSpecConfigFlag(cmd, &opts)
	return cmd
}

func newSpecAdoptCmd() *cobra.Command {
	opts := specOptions{}
	cmd := &cobra.Command{
		Use:   "adopt",
		Short: "Write the adoption baseline of currently unverified criteria",
		Long: `Write the baseline file (spec.yaml 'baseline', default
<specDir>/.spec-baseline.json) listing every criterion that is not Verified
now. In strict mode, baselined criteria stop blocking; a Verified criterion
that regresses, or any new unverified criterion, still blocks.`,
		Example: `  gh optivem spec adopt
  gh optivem spec adopt --config path/to/spec.yaml`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runSpecCmd(func() (int, error) { return runSpecAdopt(opts, cmd.OutOrStdout()) })
		},
	}
	addSpecConfigFlag(cmd, &opts)
	cmd.Flags().StringSliceVar(&opts.Flags, "flag", nil, "Feature flag considered on (repeatable); adds to flags: in spec.yaml")
	return cmd
}

// specInputs is everything loaded from spec.yaml, the markdown and the XML.
type specInputs struct {
	cfg      spec.Config
	base     string
	useCases []spec.UseCase
	tests    []spec.Test
}

func resolveUnder(base, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(base, p)
}

func loadSpecInputs(configFile string, needResults bool) (specInputs, error) {
	cfg, err := spec.LoadConfig(configFile)
	if err != nil {
		return specInputs{}, err
	}
	base := filepath.Dir(configFile)
	useCases, err := spec.LoadDir(resolveUnder(base, cfg.SpecDir))
	if err != nil {
		return specInputs{}, err
	}
	in := specInputs{cfg: cfg, base: base, useCases: useCases}
	if !needResults {
		return in, nil
	}
	if err := cfg.RequireResults(); err != nil {
		return specInputs{}, err
	}
	files, err := expandResultGlobs(base, cfg.Results, cfg.Exclude)
	if err != nil {
		return specInputs{}, err
	}
	if len(files) == 0 {
		return specInputs{}, fmt.Errorf("no JUnit XML files matched results %v (excluding %v) under %s; run the tests first or fix spec.yaml", []string(cfg.Results), []string(cfg.Exclude), base)
	}
	in.tests, err = spec.ReadJUnitFiles(files)
	if err != nil {
		return specInputs{}, err
	}
	return in, nil
}

func (o specOptions) reportOptions(cmd *cobra.Command) spec.Options {
	ro := spec.Options{Mode: o.Mode, Flags: o.Flags, Now: time.Now()}
	if cmd != nil && cmd.Flags().Changed("threshold") {
		t := o.Threshold
		ro.Threshold = &t
	}
	return ro
}

func buildGatedReport(in specInputs, ro spec.Options) (spec.Report, error) {
	if ro.Threshold != nil && (*ro.Threshold < 0 || *ro.Threshold > 100) {
		return spec.Report{}, fmt.Errorf("--threshold %v must be between 0 and 100", *ro.Threshold)
	}
	rep := spec.BuildReport(in.useCases, in.tests, in.cfg, ro)
	var baseline *spec.Baseline
	b, err := spec.LoadBaseline(resolveUnder(in.base, in.cfg.Baseline))
	switch {
	case err == nil:
		baseline = &b
	case errors.Is(err, fs.ErrNotExist):
		// No baseline file means adopt was never run; that is a valid state.
	default:
		return spec.Report{}, err
	}
	if err := spec.ApplyGate(&rep, baseline); err != nil {
		return spec.Report{}, err
	}
	return rep, nil
}

func runSpecCheck(opts specOptions, cmd *cobra.Command, out io.Writer, enforce bool) (int, error) {
	if opts.JSON && opts.PRComment {
		return 0, errors.New("--json and --pr-comment are mutually exclusive")
	}
	in, err := loadSpecInputs(opts.Config, true)
	if err != nil {
		return 0, err
	}
	rep, err := buildGatedReport(in, opts.reportOptions(cmd))
	if err != nil {
		return 0, err
	}
	switch {
	case opts.JSON:
		data, err := rep.JSON()
		if err != nil {
			return 0, fmt.Errorf("render JSON report: %w", err)
		}
		fmt.Fprintln(out, string(data))
	case opts.PRComment:
		fmt.Fprintln(out, spec.RenderPRComment(rep))
	default:
		fmt.Fprint(out, spec.RenderText(rep))
	}
	if opts.HTML != "" {
		if err := os.WriteFile(opts.HTML, []byte(spec.RenderHTML(rep, time.Now())), 0o644); err != nil {
			return 0, fmt.Errorf("write HTML report %s: %w", opts.HTML, err)
		}
	}
	if err := appendStepSummary(rep); err != nil {
		return 0, err
	}
	if enforce {
		return rep.ExitCode(), nil
	}
	return 0, nil
}

// appendStepSummary appends the Markdown summary to GITHUB_STEP_SUMMARY when set.
func appendStepSummary(rep spec.Report) error {
	file := os.Getenv("GITHUB_STEP_SUMMARY")
	if file == "" {
		return nil
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open GITHUB_STEP_SUMMARY %s: %w", file, err)
	}
	if _, err := f.WriteString(spec.RenderSummaryMarkdown(rep) + "\n"); err != nil {
		f.Close()
		return fmt.Errorf("write GITHUB_STEP_SUMMARY %s: %w", file, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close GITHUB_STEP_SUMMARY %s: %w", file, err)
	}
	return nil
}

func runSpecValidate(opts specOptions, out io.Writer) (int, error) {
	in, err := loadSpecInputs(opts.Config, false)
	if err != nil {
		return 0, err
	}
	errs := spec.Validate(in.useCases)
	if len(errs) == 0 {
		fmt.Fprintf(out, "OK: %d use case(s) valid\n", len(in.useCases))
		return 0, nil
	}
	for _, e := range errs {
		fmt.Fprintf(out, "ERROR [%s] %s\n", e.Code, e.Message)
	}
	fmt.Fprintf(out, "%d validation error(s)\n", len(errs))
	return spec.ExitGateFailed, nil
}

func runSpecAdopt(opts specOptions, out io.Writer) (int, error) {
	in, err := loadSpecInputs(opts.Config, true)
	if err != nil {
		return 0, err
	}
	rep := spec.BuildReport(in.useCases, in.tests, in.cfg, spec.Options{Mode: spec.ModeLenient, Flags: opts.Flags, Now: time.Now()})
	if n := len(rep.Validation.Errors); n > 0 {
		return 0, fmt.Errorf("cannot adopt: %d validation error(s); run 'gh optivem spec validate' and fix them first", n)
	}
	b := spec.NewBaseline(rep)
	file := resolveUnder(in.base, in.cfg.Baseline)
	if err := spec.WriteBaseline(file, b); err != nil {
		return 0, err
	}
	fmt.Fprintf(out, "Baseline written to %s (%d unverified criteria)\n", file, len(b.Unverified))
	return 0, nil
}

// expandResultGlobs resolves glob patterns (with ** for any number of
// directories) relative to base, drops files matching an exclude pattern and
// returns a sorted, de-duplicated list.
func expandResultGlobs(base string, patterns, excludes []string) ([]string, error) {
	for _, p := range append(append([]string{}, patterns...), excludes...) {
		if _, err := path.Match(strings.ReplaceAll(p, "**", "*"), ""); err != nil {
			return nil, fmt.Errorf("spec.yaml: invalid glob %q: %w", p, err)
		}
	}
	var files []string
	err := filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if n := d.Name(); n == ".git" || n == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(base, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if matchesAny(patterns, rel) && !matchesAny(excludes, rel) {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s for results: %w", base, err)
	}
	sort.Strings(files)
	return files, nil
}

func matchesAny(patterns []string, rel string) bool {
	segs := strings.Split(rel, "/")
	for _, p := range patterns {
		if globMatch(strings.Split(strings.TrimPrefix(filepath.ToSlash(p), "./"), "/"), segs) {
			return true
		}
	}
	return false
}

// globMatch matches slash-separated path segments; a "**" segment matches zero
// or more directories.
func globMatch(pat, segs []string) bool {
	if len(pat) == 0 {
		return len(segs) == 0
	}
	if pat[0] == "**" {
		for i := 0; i <= len(segs); i++ {
			if globMatch(pat[1:], segs[i:]) {
				return true
			}
		}
		return false
	}
	if len(segs) == 0 {
		return false
	}
	ok, err := path.Match(pat[0], segs[0])
	if err != nil || !ok {
		return false
	}
	return globMatch(pat[1:], segs[1:])
}
