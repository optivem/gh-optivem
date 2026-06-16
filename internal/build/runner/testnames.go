package runner

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// executedTestNames parses a machine-readable test report and returns the set
// of bare test *method* names that executed at least once. It is the
// presence-check twin of countExecutedTests: same report-format dispatch (the
// path's extension, or directory → folder of JUnit XML), but it collapses the
// per-invocation testcase entries back to method names so the result is
// agnostic to channel and parameterization — one method that ran across two
// channels and five data rows contributes a single name.
//
// The de-dup to a method name is what makes the caller's "every requested
// --test name executed somewhere" check survive the suite partitioning
// (isolated vs non-isolated, API vs UI): a name that ran in any partition
// lands in the union regardless of how many invocations or which sub-suite
// produced it.
//
// A report file that does not exist contributes the empty set, not an error —
// same rationale as countExecutedTests: a sub-suite whose filter matched
// nothing may never write a report, and "absent here" is a normal partition
// outcome, not a failure. A malformed report that does exist IS an error.
func executedTestNames(path string) (map[string]bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("stat test report %s: %w", path, err)
	}
	if info.IsDir() {
		return namesJUnitDir(path)
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".trx":
		return namesTRX(path)
	case ".xml":
		return namesJUnitFile(path)
	case ".json":
		return namesPlaywrightJSON(path)
	default:
		return nil, fmt.Errorf("unsupported test report format for names: %s", path)
	}
}

// leadingIdentRE captures the leading method identifier of a testcase name,
// dropping any trailing channel decoration (" [Channel: UI]"), data-row index
// ("[2]"), or argument list ("(channel: UI)") the runner appends per
// invocation. Java/C#/TS method names are all `[A-Za-z_$][\w$]*`.
var leadingIdentRE = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*`)

func bareMethod(s string) string {
	s = strings.TrimSpace(s)
	if m := leadingIdentRE.FindString(s); m != "" {
		return m
	}
	return s
}

// ---- JUnit XML (Java) -------------------------------------------------------
// Shape: <testcase name="method [Channel: UI]" classname="...">. The method is
// the leading identifier of name; classname is ignored (the requested names
// are bare method names, matching gradle's `--tests *.<method>` semantics).

type junitCase struct {
	Name string `xml:"name,attr"`
}

type junitSuiteCases struct {
	Cases []junitCase `xml:"testcase"`
}

type junitSuitesCases struct {
	Suites []junitSuiteCases `xml:"testsuite"`
}

func namesJUnitFile(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read JUnit report %s: %w", path, err)
	}
	return namesJUnitBytes(data, path)
}

func namesJUnitBytes(data []byte, path string) (map[string]bool, error) {
	out := map[string]bool{}
	// Try the <testsuites> wrapper first; fall back to a bare <testsuite>
	// (gradle writes one bare-root file per class). Mirrors countJUnitBytes.
	var multi junitSuitesCases
	if err := xml.Unmarshal(data, &multi); err == nil && len(multi.Suites) > 0 {
		for _, s := range multi.Suites {
			for _, c := range s.Cases {
				out[bareMethod(c.Name)] = true
			}
		}
		return out, nil
	}
	var single junitSuiteCases
	if err := xml.Unmarshal(data, &single); err != nil {
		return nil, fmt.Errorf("parse JUnit report %s: %w", path, err)
	}
	for _, c := range single.Cases {
		out[bareMethod(c.Name)] = true
	}
	return out, nil
}

func namesJUnitDir(dir string) (map[string]bool, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.xml"))
	if err != nil {
		return nil, fmt.Errorf("glob JUnit reports in %s: %w", dir, err)
	}
	out := map[string]bool{}
	for _, m := range matches {
		names, err := namesJUnitFile(m)
		if err != nil {
			return nil, err
		}
		for n := range names {
			out[n] = true
		}
	}
	return out, nil
}

// ---- TRX (dotnet) -----------------------------------------------------------
// Shape: <Results><UnitTestResult testName="ShouldPlaceOrder(channel: UI)"/>.
// testName may be fully qualified, so strip the argument list then take the
// last dotted segment before reducing to the bare identifier.

type trxResults struct {
	XMLName xml.Name `xml:"TestRun"`
	Results struct {
		UnitTests []struct {
			TestName string `xml:"testName,attr"`
		} `xml:"UnitTestResult"`
	} `xml:"Results"`
}

func bareMethodTRX(testName string) string {
	if i := strings.IndexByte(testName, '('); i >= 0 {
		testName = testName[:i]
	}
	if i := strings.LastIndexByte(testName, '.'); i >= 0 {
		testName = testName[i+1:]
	}
	return bareMethod(testName)
}

func namesTRX(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read TRX report %s: %w", path, err)
	}
	var r trxResults
	if err := xml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse TRX report %s: %w", path, err)
	}
	out := map[string]bool{}
	for _, ut := range r.Results.UnitTests {
		out[bareMethodTRX(ut.TestName)] = true
	}
	return out, nil
}

// ---- Playwright JSON (TypeScript) ------------------------------------------
// Shape: { suites: [ { specs: [ { title: "shouldXxx" } ], suites: [ … ] } ] }.
// Channel is a Playwright project, not part of the spec title, so the title is
// the bare test name; suites nest, so walk recursively.

type pwSuite struct {
	Specs []struct {
		Title string `json:"title"`
	} `json:"specs"`
	Suites []pwSuite `json:"suites"`
}

type pwNamesReport struct {
	Suites []pwSuite `json:"suites"`
}

func collectPwSpecs(s pwSuite, out map[string]bool) {
	for _, spec := range s.Specs {
		out[strings.TrimSpace(spec.Title)] = true
	}
	for _, child := range s.Suites {
		collectPwSpecs(child, out)
	}
}

func namesPlaywrightJSON(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Playwright report %s: %w", path, err)
	}
	var r pwNamesReport
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse Playwright report %s: %w", path, err)
	}
	out := map[string]bool{}
	for _, s := range r.Suites {
		collectPwSpecs(s, out)
	}
	return out, nil
}
