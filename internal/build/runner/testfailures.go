package runner

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FailedTestCase is one executed test case a machine-readable report records
// as failed. Detail carries the failure type, message, and (truncated) stack /
// body text, so a caller can tell an assertion-shaped failure from a crash
// (e.g. Playwright's TargetClosedError) by inspecting it.
type FailedTestCase struct {
	Name   string
	Detail string
}

// ReportFailures is the structured evidence the selected suites' reports
// (TestCountPath) carry about a test run.
//
//   - FreshReports counts reports written at/after the `since` cutoff. Older
//     reports are left over from an earlier run and are ignored, so a run that
//     crashed before writing anything reads as zero fresh reports rather than
//     inheriting a stale verdict.
//   - Failures lists the failed test cases found in fresh reports.
//   - GlobalErrors lists run-level errors not attached to any test case
//     (Playwright's top-level `errors`, e.g. a failing globalSetup).
//   - Complete is true when every selected suite declares a TestCountPath, i.e.
//     the fresh reports can be taken as the whole story of the run.
type ReportFailures struct {
	FreshReports int
	Failures     []FailedTestCase
	GlobalErrors []string
	Complete     bool
}

// reportFreshnessSlack tolerates coarse filesystem mtime resolution (and small
// clock skew on mounted volumes) when deciding whether a report was written by
// the run that started at `since`.
const reportFreshnessSlack = 2 * time.Second

// failureDetailMax bounds each FailedTestCase.Detail so a runaway stack trace
// cannot bloat the caller's state.
const failureDetailMax = 4000

// FailuresInReports reads the TestCountPath report of every selected suite
// that was (re)written since `since` and returns the failed test cases they
// record, WITHOUT running anything. Suite-dir resolution mirrors NamesInReport
// / runOneSuite. Unknown suite ids are skipped. An empty suiteIDs selects
// every declared suite. A fresh report that is malformed IS an error.
func FailuresInReports(tests *TestsConfig, testsCwd string, suiteIDs []string, since time.Time) (ReportFailures, error) {
	out := ReportFailures{Complete: true}
	if tests == nil {
		out.Complete = false
		return out, nil
	}
	if len(suiteIDs) == 0 {
		suiteIDs = tests.SuiteIDs()
	}
	cutoff := since.Add(-reportFreshnessSlack)
	found := 0
	for _, id := range suiteIDs {
		suite := tests.FindSuite(id)
		if suite == nil {
			continue
		}
		found++
		if suite.TestCountPath == "" {
			out.Complete = false
			continue
		}
		suiteDir := testsCwd
		if suite.Path != "" && suite.Path != "." {
			suiteDir = filepath.Join(testsCwd, suite.Path)
		}
		if err := collectReportFailures(filepath.Join(suiteDir, suite.TestCountPath), cutoff, &out); err != nil {
			return ReportFailures{}, fmt.Errorf("read report for suite %s: %w", suite.Name, err)
		}
	}
	if found == 0 {
		out.Complete = false
	}
	return out, nil
}

func collectReportFailures(path string, cutoff time.Time, out *ReportFailures) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat test report %s: %w", path, err)
	}
	if info.IsDir() {
		matches, err := filepath.Glob(filepath.Join(path, "*.xml"))
		if err != nil {
			return fmt.Errorf("glob JUnit reports in %s: %w", path, err)
		}
		for _, m := range matches {
			if err := collectReportFailures(m, cutoff, out); err != nil {
				return err
			}
		}
		return nil
	}
	if info.ModTime().Before(cutoff) {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read test report %s: %w", path, err)
	}
	var fails []FailedTestCase
	var globals []string
	switch strings.ToLower(filepath.Ext(path)) {
	case ".trx":
		fails, err = trxFailures(data)
	case ".xml":
		fails, err = junitFailures(data)
	case ".json":
		fails, globals, err = playwrightFailures(data)
	default:
		return fmt.Errorf("unsupported test report format for failures: %s", path)
	}
	if err != nil {
		return fmt.Errorf("parse test report %s: %w", path, err)
	}
	out.FreshReports++
	out.Failures = append(out.Failures, fails...)
	out.GlobalErrors = append(out.GlobalErrors, globals...)
	return nil
}

func joinDetail(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			kept = append(kept, p)
		}
	}
	s := strings.Join(kept, "\n")
	if len(s) > failureDetailMax {
		s = s[:failureDetailMax]
	}
	return s
}

// ---- JUnit XML (Java) -------------------------------------------------------

type junitProblem struct {
	Message string `xml:"message,attr"`
	Type    string `xml:"type,attr"`
	Body    string `xml:",chardata"`
}

type junitFailCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Failure   *junitProblem `xml:"failure"`
	Error     *junitProblem `xml:"error"`
}

type junitFailSuite struct {
	Cases []junitFailCase `xml:"testcase"`
}

type junitFailSuites struct {
	Suites []junitFailSuite `xml:"testsuite"`
}

func junitFailures(data []byte) ([]FailedTestCase, error) {
	var suites []junitFailSuite
	var multi junitFailSuites
	if err := xml.Unmarshal(data, &multi); err == nil && len(multi.Suites) > 0 {
		suites = multi.Suites
	} else {
		var single junitFailSuite
		if err := xml.Unmarshal(data, &single); err != nil {
			return nil, err
		}
		suites = []junitFailSuite{single}
	}
	var out []FailedTestCase
	for _, s := range suites {
		for _, c := range s.Cases {
			for _, p := range []*junitProblem{c.Failure, c.Error} {
				if p == nil {
					continue
				}
				out = append(out, FailedTestCase{Name: c.Name, Detail: joinDetail(p.Type, p.Message, p.Body)})
			}
		}
	}
	return out, nil
}

// ---- TRX (dotnet) -----------------------------------------------------------

type trxFailErrorInfo struct {
	Message    string `xml:"Message"`
	StackTrace string `xml:"StackTrace"`
}

type trxFailRun struct {
	XMLName xml.Name `xml:"TestRun"`
	Results struct {
		UnitTests []struct {
			TestName string `xml:"testName,attr"`
			Outcome  string `xml:"outcome,attr"`
			Output   struct {
				ErrorInfo trxFailErrorInfo `xml:"ErrorInfo"`
			} `xml:"Output"`
		} `xml:"UnitTestResult"`
	} `xml:"Results"`
}

func trxFailures(data []byte) ([]FailedTestCase, error) {
	var r trxFailRun
	if err := xml.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	var out []FailedTestCase
	for _, ut := range r.Results.UnitTests {
		switch ut.Outcome {
		case "Failed", "Error", "Aborted", "Timeout":
			out = append(out, FailedTestCase{
				Name:   ut.TestName,
				Detail: joinDetail(ut.Outcome, ut.Output.ErrorInfo.Message, ut.Output.ErrorInfo.StackTrace),
			})
		}
	}
	return out, nil
}

// ---- Playwright JSON (TypeScript) ------------------------------------------

type pwFailError struct {
	Message string `json:"message"`
	Stack   string `json:"stack"`
}

type pwFailResult struct {
	Status string        `json:"status"`
	Error  *pwFailError  `json:"error"`
	Errors []pwFailError `json:"errors"`
}

type pwFailSpec struct {
	Title string `json:"title"`
	Tests []struct {
		Results []pwFailResult `json:"results"`
	} `json:"tests"`
}

type pwFailSuite struct {
	Specs  []pwFailSpec  `json:"specs"`
	Suites []pwFailSuite `json:"suites"`
}

type pwFailReport struct {
	Suites []pwFailSuite `json:"suites"`
	Errors []pwFailError `json:"errors"`
}

func playwrightFailures(data []byte) ([]FailedTestCase, []string, error) {
	var r pwFailReport
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, nil, err
	}
	var out []FailedTestCase
	for _, s := range r.Suites {
		out = appendPlaywrightSuiteFailures(out, s)
	}
	var globals []string
	for _, e := range r.Errors {
		globals = append(globals, joinDetail(e.Message, e.Stack))
	}
	return out, globals, nil
}

// appendPlaywrightSuiteFailures walks one suite (its specs, then its nested
// suites recursively) and appends a FailedTestCase per failing result.
func appendPlaywrightSuiteFailures(out []FailedTestCase, s pwFailSuite) []FailedTestCase {
	for _, spec := range s.Specs {
		for _, t := range spec.Tests {
			for _, res := range t.Results {
				if !pwResultFailed(res.Status) {
					continue
				}
				out = append(out, FailedTestCase{Name: spec.Title, Detail: pwResultDetail(res)})
			}
		}
	}
	for _, child := range s.Suites {
		out = appendPlaywrightSuiteFailures(out, child)
	}
	return out
}

// pwResultFailed reports whether a Playwright result status counts as a failure.
func pwResultFailed(status string) bool {
	return status == "failed" || status == "timedOut" || status == "interrupted"
}

// pwResultDetail joins a failing result's status, error and errors into one detail string.
func pwResultDetail(res pwFailResult) string {
	parts := []string{res.Status}
	if res.Error != nil {
		parts = append(parts, res.Error.Message, res.Error.Stack)
	}
	for _, e := range res.Errors {
		parts = append(parts, e.Message)
	}
	return joinDetail(parts...)
}
