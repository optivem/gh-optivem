package componenttest

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/optivem/gh-optivem/internal/build/runner"
)

// MutationSuiteID is the component-tests.yaml suite id the mutation gate runs.
// The suite is opt-in: keep it out of suiteGroups.all so a bare
// `component-test run` (the CI commit gate) never pays the mutation cost.
const MutationSuiteID = "mutation"

// TargetClassesEnv is the environment variable the mutation suite command
// receives, carrying the comma-separated fully-qualified names of the classes
// to mutate. The suite's build wiring (e.g. the PIT Gradle plugin) reads it to
// scope the run to the changed classes.
const TargetClassesEnv = "PIT_TARGET_CLASSES"

// maxSurvivorsShown caps the survivor list handed back to the fix loop: a
// short, actionable list rather than the full report.
const maxSurvivorsShown = 30

// MutationOptions narrows and configures a mutation gate run.
type MutationOptions struct {
	Components []string
	// Threshold is the minimum mutation score (0-100). 0 means unset: the run
	// reports the score and survivors but does not fail on the score.
	Threshold int
	// All mutates the whole component instead of only the classes changed
	// relative to HEAD (the nightly full-baseline mode).
	All bool
}

// Mutation runs the mutation gate for every selected component: the component's
// `mutation` suite, scoped to the classes changed by the working tree unless
// opts.All, then scores the report and fails when it is below the threshold,
// printing only the surviving mutants.
func Mutation(components []Component, opts MutationOptions) error {
	selected, err := selectComponents(components, opts.Components)
	if err != nil {
		return err
	}
	for _, c := range selected {
		if err := mutateComponent(c, opts); err != nil {
			return fmt.Errorf("component %s: %w", c.Name, err)
		}
	}
	return nil
}

func mutateComponent(c Component, opts MutationOptions) error {
	if c.Lang != "java" {
		return fmt.Errorf("mutation gate supports java components only for now (component language %q)", c.Lang)
	}
	cfg, err := Load(filepath.Join(c.Path, ConfigFileName))
	if err != nil {
		return err
	}
	suite, ok := cfg.suiteByID(MutationSuiteID)
	if !ok {
		return fmt.Errorf("no %q suite in %s: the inner-loop mutation gate needs one", MutationSuiteID, filepath.Join(c.Path, ConfigFileName))
	}
	if suite.TestReportPath == "" {
		return fmt.Errorf("suite %q declares no testReportPath: set it to the PIT XML report (e.g. build/reports/pitest/mutations.xml)", MutationSuiteID)
	}

	env := map[string]string{}
	for k, v := range suite.Env {
		env[k] = v
	}
	if !opts.All {
		changed, err := ChangedJavaClasses(c.Path)
		if err != nil {
			return err
		}
		if len(changed) == 0 {
			fmt.Fprintf(os.Stdout, "Mutation gate: no changed production classes under %s; nothing to mutate.\n", c.Path)
			return nil
		}
		fmt.Fprintf(os.Stdout, "Mutation gate: mutating %d changed class(es): %s\n", len(changed), strings.Join(changed, ", "))
		env[TargetClassesEnv] = strings.Join(changed, ",")
	}

	cwd := c.Path
	if suite.Path != "" && suite.Path != "." {
		cwd = filepath.Join(c.Path, suite.Path)
	}
	report := filepath.Join(cwd, suite.TestReportPath)
	// A stale report from an earlier run must never be scored as this run's.
	if err := os.Remove(report); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale report %s: %w", report, err)
	}

	start := time.Now()
	if err := runner.RunShell(suite.Command, cwd, env); err != nil {
		return fmt.Errorf("mutation suite command failed (component tests red under mutation analysis, or the build broke): %w", err)
	}
	fmt.Fprintf(os.Stdout, "Mutation run took %s\n", formatDur(time.Since(start)))

	data, err := os.ReadFile(report)
	if err != nil {
		return fmt.Errorf("mutation suite finished but wrote no report at %s: %w", report, err)
	}
	score, err := ScorePitReport(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", report, err)
	}
	return score.Judge(opts.Threshold, os.Stdout)
}

func (c *Config) suiteByID(id string) (Suite, bool) {
	for _, s := range c.Suites {
		if s.ID == id {
			return s, true
		}
	}
	return Suite{}, false
}

// MutationScore is a scored PIT report.
type MutationScore struct {
	Total     int // mutants that could be evaluated (non-viable excluded)
	Detected  int // killed, timed out, memory error, run error
	Survivors []Survivor
}

// Survivor is one undetected mutant.
type Survivor struct {
	Class       string
	Line        int
	Mutator     string
	Description string
	Status      string // SURVIVED | NO_COVERAGE
}

// Percent is the mutation score, 100 when nothing could be mutated.
func (s MutationScore) Percent() int {
	if s.Total == 0 {
		return 100
	}
	return s.Detected * 100 / s.Total
}

type pitMutation struct {
	Status       string `xml:"status,attr"`
	MutatedClass string `xml:"mutatedClass"`
	LineNumber   int    `xml:"lineNumber"`
	Mutator      string `xml:"mutator"`
	Description  string `xml:"description"`
}

type pitReport struct {
	Mutations []pitMutation `xml:"mutation"`
}

// ScorePitReport scores a PIT XML report (mutations.xml). Non-viable mutants are
// excluded; SURVIVED and NO_COVERAGE count against the score and are returned
// as survivors, every other status counts as detected.
func ScorePitReport(xmlData []byte) (MutationScore, error) {
	var rep pitReport
	if err := xml.Unmarshal(xmlData, &rep); err != nil {
		return MutationScore{}, err
	}
	var s MutationScore
	for _, m := range rep.Mutations {
		switch m.Status {
		case "NON_VIABLE":
			continue
		case "SURVIVED", "NO_COVERAGE":
			s.Total++
			s.Survivors = append(s.Survivors, Survivor{m.MutatedClass, m.LineNumber, shortMutator(m.Mutator), m.Description, m.Status})
		default:
			s.Total++
			s.Detected++
		}
	}
	sort.SliceStable(s.Survivors, func(i, j int) bool {
		if s.Survivors[i].Class != s.Survivors[j].Class {
			return s.Survivors[i].Class < s.Survivors[j].Class
		}
		return s.Survivors[i].Line < s.Survivors[j].Line
	})
	return s, nil
}

// shortMutator trims PIT's fully-qualified mutator name to its last segment.
func shortMutator(m string) string {
	if i := strings.LastIndex(m, "."); i >= 0 {
		return m[i+1:]
	}
	return m
}

// Judge prints the score and the (capped) survivor list to w and returns an
// error when the score is below threshold. A zero threshold only reports.
func (s MutationScore) Judge(threshold int, w io.Writer) error {
	pct := s.Percent()
	fmt.Fprintf(w, "Mutation score: %d%% (%d of %d mutants detected)\n", pct, s.Detected, s.Total)
	if len(s.Survivors) > 0 {
		fmt.Fprintf(w, "Surviving mutants (%d) - add or strengthen component tests that fail when this behaviour changes:\n", len(s.Survivors))
		for i, sv := range s.Survivors {
			if i == maxSurvivorsShown {
				fmt.Fprintf(w, "  ... and %d more\n", len(s.Survivors)-maxSurvivorsShown)
				break
			}
			fmt.Fprintf(w, "  %s:%d [%s, %s] %s\n", sv.Class, sv.Line, sv.Mutator, sv.Status, sv.Description)
		}
	}
	if threshold == 0 {
		fmt.Fprintln(w, "No mutation-threshold configured: report only, not gating.")
		return nil
	}
	if pct < threshold {
		return fmt.Errorf("mutation score %d%% is below the configured threshold %d%%", pct, threshold)
	}
	fmt.Fprintf(w, "Mutation gate passed (threshold %d%%).\n", threshold)
	return nil
}

// ChangedJavaClasses returns the fully-qualified names of the production Java
// classes under <componentPath>/src/main/java that differ from HEAD (modified,
// added, or untracked). Test sources are never mutation targets.
func ChangedJavaClasses(componentPath string) ([]string, error) {
	prefix := filepath.ToSlash(filepath.Join(componentPath, "src", "main", "java")) + "/"
	seen := map[string]bool{}
	for _, args := range [][]string{
		{"diff", "--name-only", "HEAD", "--", componentPath},
		{"ls-files", "--others", "--exclude-standard", "--", componentPath},
	} {
		out, err := exec.Command("git", args...).Output()
		if err != nil {
			return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
		}
		for _, line := range strings.Split(string(out), "\n") {
			if fqcn, ok := javaClassFromPath(strings.TrimSpace(line), prefix); ok {
				seen[fqcn] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// javaClassFromPath maps a repo-relative source path under prefix to its
// fully-qualified class name; package-info / module-info are skipped.
func javaClassFromPath(path, prefix string) (string, bool) {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, ".java") {
		return "", false
	}
	rel := strings.TrimSuffix(strings.TrimPrefix(path, prefix), ".java")
	base := rel[strings.LastIndex(rel, "/")+1:]
	if base == "package-info" || base == "module-info" {
		return "", false
	}
	return strings.ReplaceAll(rel, "/", "."), true
}
