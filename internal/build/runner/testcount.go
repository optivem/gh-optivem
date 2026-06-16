package runner

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// countExecutedTests parses a machine-readable test report and returns the
// number of tests that actually executed. The report shape is chosen by the
// path's extension (or, for a directory, treated as a folder of JUnit XML):
//
//   - .trx          dotnet's <ResultSummary><Counters executed="N"/></>
//   - .xml / dir    JUnit XML — sum <testsuite tests=.. skipped=..>, where
//                   executed = tests - skipped. Gradle drops one file per test
//                   class into a directory, so a directory is globbed for *.xml.
//   - .json         Playwright JSON report (stats.expected+unexpected+flaky).
//
// A report file that does not exist counts as 0 executed, not an error: a
// runner that exits 0 having matched nothing may never write a report at all
// (dotnet on a zero-match --filter), and "ran, produced no report" is itself
// the empty-selection signal this guard exists to catch. A malformed report
// that does exist IS an error — silently treating a parse failure as 0 would
// turn every real run into a false halt.
//
// This silent-0 is scoped to named runs by runOneSuite: a full (unnamed) run
// expects every selected suite to produce its report, so a missing path there
// is failed loud at the call site before reaching here. Only named --test runs,
// whose per-partition empties are legitimate, rely on the 0 below.
func countExecutedTests(path string) (int, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("stat test report %s: %w", path, err)
	}
	if info.IsDir() {
		return countJUnitDir(path)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".trx":
		return countTRX(path)
	case ".xml":
		return countJUnitFile(path)
	case ".json":
		return countPlaywrightJSON(path)
	default:
		return 0, fmt.Errorf("unsupported test report format for count: %s", path)
	}
}

// trxReport is the minimal slice of dotnet's TRX schema we need: the single
// <ResultSummary><Counters .../></ResultSummary> block. `executed` is dotnet's
// own tally of tests that ran (passed+failed+…, excluding not-executed), which
// is exactly the number this guard cares about.
type trxReport struct {
	XMLName       xml.Name `xml:"TestRun"`
	ResultSummary struct {
		Counters struct {
			Executed int `xml:"executed,attr"`
		} `xml:"Counters"`
	} `xml:"ResultSummary"`
}

func countTRX(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read TRX report %s: %w", path, err)
	}
	var r trxReport
	if err := xml.Unmarshal(data, &r); err != nil {
		return 0, fmt.Errorf("parse TRX report %s: %w", path, err)
	}
	return r.ResultSummary.Counters.Executed, nil
}

// junitTestsuite is one <testsuite> element. JUnit reports either wrap the
// suites in a <testsuites> root or emit a bare <testsuite> (gradle writes one
// bare-root file per class), so both shapes are decoded.
type junitTestsuite struct {
	Tests   int `xml:"tests,attr"`
	Skipped int `xml:"skipped,attr"`
}

type junitTestsuites struct {
	Suites []junitTestsuite `xml:"testsuite"`
}

func countJUnitFile(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read JUnit report %s: %w", path, err)
	}
	return countJUnitBytes(data, path)
}

func countJUnitBytes(data []byte, path string) (int, error) {
	// Try the <testsuites> wrapper first; fall back to a bare <testsuite>.
	var multi junitTestsuites
	if err := xml.Unmarshal(data, &multi); err == nil && len(multi.Suites) > 0 {
		total := 0
		for _, s := range multi.Suites {
			total += s.Tests - s.Skipped
		}
		return total, nil
	}
	var single junitTestsuite
	if err := xml.Unmarshal(data, &single); err != nil {
		return 0, fmt.Errorf("parse JUnit report %s: %w", path, err)
	}
	return single.Tests - single.Skipped, nil
}

func countJUnitDir(dir string) (int, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.xml"))
	if err != nil {
		return 0, fmt.Errorf("glob JUnit reports in %s: %w", dir, err)
	}
	total := 0
	for _, m := range matches {
		n, err := countJUnitFile(m)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// playwrightReport is the minimal slice of Playwright's JSON reporter output.
// stats.expected/unexpected/flaky are the executed outcomes; skipped tests are
// not counted as executed, matching the JUnit/TRX semantics above.
type playwrightReport struct {
	Stats struct {
		Expected   int `json:"expected"`
		Unexpected int `json:"unexpected"`
		Flaky      int `json:"flaky"`
	} `json:"stats"`
}

func countPlaywrightJSON(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read Playwright report %s: %w", path, err)
	}
	var r playwrightReport
	if err := json.Unmarshal(data, &r); err != nil {
		return 0, fmt.Errorf("parse Playwright report %s: %w", path, err)
	}
	return r.Stats.Expected + r.Stats.Unexpected + r.Stats.Flaky, nil
}
