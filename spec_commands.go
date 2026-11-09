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

// specFlagUsage is the --flag usage text shared by the spec subcommands.
const specFlagUsage = "Feature flag considered on (repeatable); adds to flags: in spec.yaml"

const defaultSpecConfig = "spec.yaml"

type specOptions struct {
	Config    string
	Mode      string
	Threshold float64
	JSON      bool
	HTML      string
	PRComment bool
	Flags     []string
	Refresh   []string
	Criteria  []string
	Create    bool
}

func newSpecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Spec-coverage gate: derive acceptance-criterion states from tests and gate on them",
		Long: `Spec-coverage gate (see docs/spec-gate/convention.md).

Use-case markdown files define acceptance criteria with stable IDs (UC-001.3).
Acceptance tests carry those IDs as tags that surface as properties in JUnit
XML. The gate reads only the markdown and the XML and derives a lifecycle
state per criterion: Planned, In progress, Verified, Failing, Waived or
Changed (tests pass but the criterion text changed since it was verified).

Configuration lives in spec.yaml (default: ./spec.yaml, override with --config).
Exit codes: 0 pass, 1 gate failed, 2 usage or configuration error.`,
		Example: `  gh optivem spec validate
  gh optivem spec check --mode lenient
  gh optivem spec check --mode strict --threshold 90 --html spec-report.html
  gh optivem spec adopt
  gh optivem spec record`,
	}
	cmd.AddCommand(newSpecCheckCmd(), newSpecStatusCmd(), newSpecValidateCmd(), newSpecAdoptCmd(), newSpecRecordCmd())
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
	cmd.Flags().StringSliceVar(&opts.Flags, "flag", nil, specFlagUsage)
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
		Long: `Write the spec state file (spec.yaml 'baseline', default
<specDir>/.spec-baseline.json) listing every criterion that is not Verified
now, plus the text fingerprint of every Verified criterion. In strict mode,
baselined criteria stop blocking; a Verified criterion that regresses, or any
new unverified criterion, still blocks.`,
		Example: `  gh optivem spec adopt
  gh optivem spec adopt --config path/to/spec.yaml`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runSpecCmd(func() (int, error) { return runSpecAdopt(opts, cmd.OutOrStdout()) })
		},
	}
	addSpecConfigFlag(cmd, &opts)
	cmd.Flags().StringSliceVar(&opts.Flags, "flag", nil, specFlagUsage)
	return cmd
}

func newSpecRecordCmd() *cobra.Command {
	opts := specOptions{}
	cmd := &cobra.Command{
		Use:   "record",
		Short: "Record text fingerprints of newly verified criteria in the spec state file",
		Long: `Store the text fingerprint of every Verified criterion that has none yet in
the spec state file (spec.yaml 'baseline', default <specDir>/.spec-baseline.json),
leaving the unverified baseline list untouched. A criterion whose text changed
since its fingerprint was recorded keeps the old fingerprint, so 'check' keeps
reporting it as Changed until you re-verify it and name it with --refresh.
Fingerprints of criteria no longer in the spec are dropped.

Requires an existing state file (run 'spec adopt' first) unless --create is
given: then a missing state file is created with an empty unverified list, so
no existing gap is baselined. --criteria limits recording to the named
criteria; a named criterion that is not Verified is reported and skipped.
Run it after the acceptance tests for a story are green; 'gh optivem
implement' does this for every story (--create --criteria <merged criteria>).`,
		Example: `  gh optivem spec record
  gh optivem spec record --refresh UC-001.4
  gh optivem spec record --create --criteria UC-001.2,UC-001.3`,
		Args: cobra.NoArgs,
		Run: func(cmd *cobra.Command, args []string) {
			runSpecCmd(func() (int, error) { return runSpecRecord(opts, cmd.OutOrStdout()) })
		},
	}
	addSpecConfigFlag(cmd, &opts)
	cmd.Flags().StringSliceVar(&opts.Flags, "flag", nil, specFlagUsage)
	cmd.Flags().StringSliceVar(&opts.Refresh, "refresh", nil, "Criterion id whose changed text is accepted as re-verified (repeatable)")
	cmd.Flags().StringSliceVar(&opts.Criteria, "criteria", nil, "Only record fingerprints for these criterion ids (repeatable or comma-separated); default all")
	cmd.Flags().BoolVar(&opts.Create, "create", false, "Create the state file (empty unverified list) when it does not exist instead of failing")
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
	var baseline *spec.Baseline
	b, err := spec.LoadBaseline(resolveUnder(in.base, in.cfg.Baseline))
	switch {
	case err == nil:
		baseline = &b
		ro.Fingerprints = b.Fingerprints
	case errors.Is(err, fs.ErrNotExist):
		// No baseline file means adopt was never run; that is a valid state.
	default:
		return spec.Report{}, err
	}
	rep := spec.BuildReport(in.useCases, in.tests, in.cfg, ro)
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
	for _, w := range spec.Warnings(in.useCases) {
		fmt.Fprintf(out, "WARN %s\n", w)
	}
	if len(errs) == 0 {
		criteria, planned := 0, 0
		for _, uc := range in.useCases {
			if uc.Planned() {
				planned++
				continue
			}
			criteria += len(uc.Criteria)
		}
		fmt.Fprintf(out, "OK: %d use case(s) valid, %d criteria gated", len(in.useCases), criteria)
		if planned > 0 {
			fmt.Fprintf(out, " (%d planned use case(s) not gated)", planned)
		}
		fmt.Fprintln(out)
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
	fmt.Fprintf(out, "Baseline written to %s (%d unverified criteria, %d fingerprints)\n", file, len(b.Unverified), len(b.Fingerprints))
	return 0, nil
}

// runSpecRecord stores fingerprints of newly verified criteria. Without
// --create it requires an existing state file (run 'spec adopt' first). With
// --create a missing file starts with an empty unverified list, which gates
// exactly like no file, so no unverified criterion is ever silently baselined.
func runSpecRecord(opts specOptions, out io.Writer) (int, error) {
	in, err := loadSpecInputs(opts.Config, true)
	if err != nil {
		return 0, err
	}
	file := resolveUnder(in.base, in.cfg.Baseline)
	b, err := loadRecordBaseline(file, opts.Create)
	if err != nil {
		return 0, err
	}
	rep := spec.BuildReport(in.useCases, in.tests, in.cfg, spec.Options{Mode: spec.ModeLenient, Flags: opts.Flags, Now: time.Now()})
	if n := len(rep.Validation.Errors); n > 0 {
		return 0, fmt.Errorf("cannot record: %d validation error(s); run 'gh optivem spec validate' and fix them first", n)
	}
	scope, err := recordScope(rep, opts, in.cfg.SpecDir)
	if err != nil {
		return 0, err
	}
	rec := spec.RecordFingerprints(rep, b.Fingerprints, opts.Refresh, scope)
	b.Fingerprints = rec.Fingerprints
	if err := spec.WriteBaseline(file, b); err != nil {
		return 0, err
	}
	fmt.Fprintf(out, "Fingerprints recorded in %s: %d added, %d refreshed, %d pruned\n", file, len(rec.Added), len(rec.Refreshed), len(rec.Pruned))
	for _, id := range rec.StillChanged {
		fmt.Fprintf(out, "  %s changed since it was last verified; re-run with --refresh %s once its test is re-checked\n", id, id)
	}
	for _, id := range rec.NotVerified {
		fmt.Fprintf(out, "  %s is not Verified by the test results; no fingerprint recorded\n", id)
	}
	return 0, nil
}

// loadRecordBaseline loads the spec state file for 'spec record'. A missing
// file is an error unless create is set, in which case it starts empty.
func loadRecordBaseline(file string, create bool) (spec.Baseline, error) {
	b, err := spec.LoadBaseline(file)
	if err == nil {
		return b, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return spec.Baseline{}, err
	}
	if !create {
		return spec.Baseline{}, fmt.Errorf("no spec state file at %s; run 'gh optivem spec adopt' first (or pass --create)", file)
	}
	return spec.Baseline{SchemaVersion: spec.BaselineSchemaVersion, Unverified: []string{}}, nil
}

// recordScope checks that every --refresh and --criteria id names a known
// criterion (gated or planned) and returns the trimmed --criteria scope.
func recordScope(rep spec.Report, opts specOptions, specDir string) ([]string, error) {
	known := map[string]bool{}
	for _, uc := range rep.AllUseCases() {
		for _, c := range uc.Criteria {
			known[c.ID] = true
		}
	}
	for _, id := range opts.Refresh {
		if !known[id] {
			return nil, fmt.Errorf("--refresh %s: no such criterion in %s", id, specDir)
		}
	}
	var scope []string
	for _, id := range opts.Criteria {
		if id = strings.TrimSpace(id); id == "" {
			continue
		}
		if !known[id] {
			return nil, fmt.Errorf("--criteria %s: no such criterion in %s", id, specDir)
		}
		scope = append(scope, id)
	}
	if len(opts.Criteria) > 0 && len(scope) == 0 {
		return nil, errors.New("--criteria names no criterion id")
	}
	return scope, nil
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
