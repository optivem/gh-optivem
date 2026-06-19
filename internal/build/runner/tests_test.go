package runner

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAppendTestFilterFullFlagAppendsAsArg(t *testing.T) {
	got := appendTestFilter("npx playwright test smoke", "--grep 'shouldWork'")
	want := "npx playwright test smoke --grep 'shouldWork'"
	if got != want {
		t.Errorf("\n got:  %q\n want: %q", got, want)
	}
}

func TestAppendTestFilterFragmentInjectsIntoExistingFilter(t *testing.T) {
	cmd := "dotnet test --filter 'FullyQualifiedName~Smoke' -e ENV=local"
	got := appendTestFilter(cmd, "&DisplayName~ShouldWork")
	want := "dotnet test --filter 'FullyQualifiedName~Smoke&DisplayName~ShouldWork' -e ENV=local"
	if got != want {
		t.Errorf("\n got:  %q\n want: %q", got, want)
	}
}

func TestAppendTestFilterFragmentNoExistingFilterIsNoOp(t *testing.T) {
	cmd := "dotnet test"
	// Mirrors PS1 behavior: no --filter present, so the fragment is silently
	// dropped. Documented here so any future change is intentional.
	got := appendTestFilter(cmd, "&DisplayName~ShouldWork")
	if got != cmd {
		t.Errorf("want command unchanged when no --filter present, got %q", got)
	}
}

func TestPickFilterValue(t *testing.T) {
	suite := Suite{SampleTest: "shouldWork"}
	cases := []struct {
		name     string
		opts     TestOptions
		expected []string
	}{
		{"explicit Test wins over Sample", TestOptions{Test: []string{"explicit"}, Sample: true}, []string{"explicit"}},
		{"only Test, single value", TestOptions{Test: []string{"explicit"}}, []string{"explicit"}},
		{"only Test, multiple values", TestOptions{Test: []string{"T1", "T2"}}, []string{"T1", "T2"}},
		{"only Sample", TestOptions{Sample: true}, []string{"shouldWork"}},
		{"sample on suite without sampleTest", TestOptions{Sample: true}, []string{"shouldWork"}},
		{"neither", TestOptions{}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pickFilterValue(suite, c.opts); !reflect.DeepEqual(got, c.expected) {
				t.Errorf("got %v, want %v", got, c.expected)
			}
		})
	}
}

func TestPickFilterValueSampleNoSampleTestReturnsNil(t *testing.T) {
	// Regression: --sample on a suite that hasn't declared sampleTest must
	// not force an empty <test> substitution; behaves like "no filter".
	got := pickFilterValue(Suite{}, TestOptions{Sample: true})
	if got != nil {
		t.Errorf("got %v, want nil", got)
	}
}

func TestApplyTestFilterSingleNameRegression(t *testing.T) {
	// One test name + default join must produce the exact same command as
	// the pre-fan-out implementation: substitute <test> once, append once.
	cases := []struct {
		name       string
		command    string
		testFilter string
		want       string
	}{
		{
			name:       "playwright --grep full flag",
			command:    "npx playwright test smoke",
			testFilter: "--grep '<test>'",
			want:       "npx playwright test smoke --grep 'shouldWork'",
		},
		{
			name:       "dotnet &-fragment injected into existing --filter",
			command:    "dotnet test --filter 'FullyQualifiedName~Smoke'",
			testFilter: "&DisplayName~<test>",
			want:       "dotnet test --filter 'FullyQualifiedName~Smoke&DisplayName~shouldWork'",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := applyTestFilter(c.command, c.testFilter, "", []string{"shouldWork"})
			if got != c.want {
				t.Errorf("\n got:  %q\n want: %q", got, c.want)
			}
		})
	}
}

func TestApplyTestFilterOrJoinsWithPipe(t *testing.T) {
	cases := []struct {
		name       string
		command    string
		testFilter string
		join       string
		names      []string
		want       string
	}{
		{
			name:       "playwright --grep with two names",
			command:    "npx playwright test smoke",
			testFilter: "--grep '<test>'",
			join:       "or",
			names:      []string{"T1", "T2"},
			want:       "npx playwright test smoke --grep 'T1|T2'",
		},
		{
			name:       "dotnet fragment with two names",
			command:    "dotnet test --filter 'FullyQualifiedName~Smoke'",
			testFilter: "&DisplayName~<test>",
			join:       "or",
			names:      []string{"T1", "T2"},
			want:       "dotnet test --filter 'FullyQualifiedName~Smoke&DisplayName~T1|T2'",
		},
		{
			name:       "empty join string defaults to or",
			command:    "npx playwright test smoke",
			testFilter: "--grep '<test>'",
			join:       "",
			names:      []string{"T1", "T2", "T3"},
			want:       "npx playwright test smoke --grep 'T1|T2|T3'",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := applyTestFilter(c.command, c.testFilter, c.join, c.names)
			if got != c.want {
				t.Errorf("\n got:  %q\n want: %q", got, c.want)
			}
		})
	}
}

func TestApplyTestFilterFragmentOrWrapsInParensWithPipe(t *testing.T) {
	// dotnet `--filter` ORs full property terms (`DisplayName~T1|DisplayName~T2`),
	// not bare values — so multi-name fragments have to substitute per name,
	// pipe-join the substituted fragments, and wrap in `( ... )` before being
	// injected into the existing --filter '...'.
	cases := []struct {
		name       string
		command    string
		testFilter string
		names      []string
		want       string
	}{
		{
			name:       "single name still wraps in parens",
			command:    "dotnet test --filter 'FullyQualifiedName~Smoke'",
			testFilter: "&DisplayName~<test>",
			names:      []string{"T1"},
			want:       "dotnet test --filter 'FullyQualifiedName~Smoke&(DisplayName~T1)'",
		},
		{
			name:       "two names",
			command:    "dotnet test --filter 'FullyQualifiedName~Smoke'",
			testFilter: "&DisplayName~<test>",
			names:      []string{"T1", "T2"},
			want:       "dotnet test --filter 'FullyQualifiedName~Smoke&(DisplayName~T1|DisplayName~T2)'",
		},
		{
			name:       "three names",
			command:    "dotnet test --filter 'FullyQualifiedName~Smoke'",
			testFilter: "&DisplayName~<test>",
			names:      []string{"T1", "T2", "T3"},
			want:       "dotnet test --filter 'FullyQualifiedName~Smoke&(DisplayName~T1|DisplayName~T2|DisplayName~T3)'",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := applyTestFilter(c.command, c.testFilter, "fragment-or", c.names)
			if got != c.want {
				t.Errorf("\n got:  %q\n want: %q", got, c.want)
			}
		})
	}
}

func TestApplyTestFilterFragmentOrMissingAmpersandLeavesCommandUnchanged(t *testing.T) {
	// fragment-or only makes sense for `&`-prefixed injection templates.
	// A misconfigured tests.yaml (`testFilter: --grep '<test>'` with
	// `testFilterJoin: fragment-or`) must fail loudly rather than silently
	// emit a malformed expression.
	cmd := "npx playwright test smoke"
	got := applyTestFilter(cmd, "--grep '<test>'", "fragment-or", []string{"T1", "T2"})
	if got != cmd {
		t.Errorf("want command unchanged when fragment-or template lacks '&' prefix, got %q", got)
	}
}

func TestApplyTestFilterRepeatAppendsPerName(t *testing.T) {
	// gradle's --tests is Ant-glob, no OR — multi-value requires the whole
	// flag to repeat once per name.
	got := applyTestFilter(
		"./gradlew acceptanceTest",
		"--tests <test>",
		"repeat",
		[]string{"T1", "T2"},
	)
	want := "./gradlew acceptanceTest --tests T1 --tests T2"
	if got != want {
		t.Errorf("\n got:  %q\n want: %q", got, want)
	}
}

func TestApplyTestFilterEmptyNamesPassesThrough(t *testing.T) {
	// No --test / no --sample → no filter applied, no <test> substitution.
	cases := []struct {
		name       string
		testFilter string
		join       string
	}{
		{"or, full flag template", "--grep '<test>'", "or"},
		{"or, &-fragment template", "&DisplayName~<test>", "or"},
		{"repeat, full flag template", "--tests <test>", "repeat"},
	}
	const command = "npx playwright test smoke"
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := applyTestFilter(command, c.testFilter, c.join, nil); got != command {
				t.Errorf("want command unchanged, got %q", got)
			}
		})
	}
}

func TestApplyTestFilterEmptyTestFilterPassesThrough(t *testing.T) {
	// A suite with no testFilter template configured can't substitute names —
	// command must come back unchanged regardless of how many names we pass.
	got := applyTestFilter("./gradlew test", "", "or", []string{"T1", "T2"})
	if got != "./gradlew test" {
		t.Errorf("want command unchanged, got %q", got)
	}
}

// TestRunTestsProbesWhenSystemDown asserts the test-only-by-default precheck:
// RunTests refuses to proceed against a system whose health probe fails and
// surfaces the "start it first" message naming the offending system label.
func TestRunTestsProbesWhenSystemDown(t *testing.T) {
	// SystemEntry with no components/external systems → IsAnyURLUp returns
	// false trivially without making any network calls.
	sys := &SystemConfig{Systems: []SystemEntry{{Label: "test-stack"}}}
	tests := &TestsConfig{Suites: []Suite{{ID: "noop", Name: "noop", Command: "go version", Path: "."}}}
	err := RunTests(sys, tests, ".", ".", TestOptions{})
	if err == nil {
		t.Fatal("want error when system not running")
	}
	if !strings.Contains(err.Error(), "test-stack") {
		t.Errorf("want error to name the system label, got: %v", err)
	}
	if !strings.Contains(err.Error(), "gh optivem system start") {
		t.Errorf("want error to suggest `gh optivem system start`, got: %v", err)
	}
}

// TestRunTestsNilSystemSkipsProbe asserts that passing sys=nil bypasses the
// probe — the test-only verb can still be driven without system orchestration
// (e.g. when the SUT is started by other means).
func TestRunTestsNilSystemSkipsProbe(t *testing.T) {
	tests := &TestsConfig{Suites: []Suite{{ID: "noop", Name: "noop", Command: "go version", Path: "."}}}
	if err := RunTests(nil, tests, ".", ".", TestOptions{}); err != nil {
		t.Fatalf("nil sys should skip the probe, got %v", err)
	}
}

// TestRunTestsZeroExecutedFailsRun asserts the empty-selection guard: a suite
// that exits cleanly but whose TestCountPath reports zero executed tests fails
// the run with the marker the verify classifier matches (plan 20260608-1502).
func TestRunTestsZeroExecutedFailsRun(t *testing.T) {
	dir := t.TempDir()
	trxZero := `<TestRun><ResultSummary><Counters total="0" executed="0"/></ResultSummary></TestRun>`
	if err := os.WriteFile(filepath.Join(dir, "results.trx"), []byte(trxZero), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := &TestsConfig{Suites: []Suite{{
		ID: "noop", Name: "noop", Command: "go version", Path: ".",
		TestCountPath: "results.trx",
	}}}
	err := RunTests(nil, tests, dir, dir, TestOptions{})
	if err == nil {
		t.Fatal("want error when zero tests executed")
	}
	if !strings.Contains(err.Error(), "0 tests executed for the given selection") {
		t.Fatalf("want the empty-selection marker, got: %v", err)
	}
}

// TestRunTestsNonZeroExecutedSucceeds is the negative-space guard: a counted
// suite that actually executed tests must not trip the empty-selection marker.
func TestRunTestsNonZeroExecutedSucceeds(t *testing.T) {
	dir := t.TempDir()
	trxOne := `<TestRun><ResultSummary><Counters total="1" executed="1"/></ResultSummary></TestRun>`
	if err := os.WriteFile(filepath.Join(dir, "results.trx"), []byte(trxOne), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := &TestsConfig{Suites: []Suite{{
		ID: "noop", Name: "noop", Command: "go version", Path: ".",
		TestCountPath: "results.trx",
	}}}
	if err := RunTests(nil, tests, dir, dir, TestOptions{}); err != nil {
		t.Fatalf("a suite that executed tests must succeed, got: %v", err)
	}
}

// TestRunTestsUnnamedMissingCountReportFails is Layer 2 of the portable-path
// guard: a full (unnamed) run whose suite declares a testCountPath that the
// run never produced must fail loud naming the path — not fold into a silent 0
// that the empty-selection marker would misreport as "filter matched nothing".
func TestRunTestsUnnamedMissingCountReportFails(t *testing.T) {
	dir := t.TempDir()
	tests := &TestsConfig{Suites: []Suite{{
		ID: "noop", Name: "noop", Command: "go version", Path: ".",
		TestCountPath: "absent.xml", // declared, never written
	}}}
	err := RunTests(nil, tests, dir, dir, TestOptions{})
	if err == nil {
		t.Fatal("want error when an unnamed run's declared count report is missing")
	}
	if !strings.Contains(err.Error(), "test count report not found") {
		t.Fatalf("want 'test count report not found' naming the path, got: %v", err)
	}
}

// TestRunTestsNamedMissingCountReportTolerated is the named-run counterpart: a
// requested test lives in one partition; the other legitimately runs nothing
// and writes no report. Layer 2 must NOT fire for named runs — the presence
// check already guarantees the name ran somewhere.
func TestRunTestsNamedMissingCountReportTolerated(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "iso.xml"),
		[]byte(`<testsuite name="isolated" tests="1"><testcase name="onlyHere [Channel: API]"/></testsuite>`), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := &TestsConfig{Suites: []Suite{
		{ID: "acc-noniso", Name: "acc-noniso", Command: "go version", Path: ".", TestCountPath: "noniso-absent.xml"},
		{ID: "acc-iso", Name: "acc-iso", Command: "go version", Path: ".", TestCountPath: "iso.xml"},
	}}
	if err := RunTests(nil, tests, dir, dir, TestOptions{Test: []string{"onlyHere"}}); err != nil {
		t.Fatalf("named run must tolerate a missing report in an empty partition, got: %v", err)
	}
}

// twoPartitionSuites builds the partitioned shape of a fanned-out category: a
// non-isolated sub-suite (its report holds whatever ran there) and an isolated
// one, each reading its own report file. Commands are benign (`go version`,
// exits 0) — the pre-staged reports stand in for what the runner produced.
func twoPartitionSuites(t *testing.T, nonIsoReport, isoReport string) (*TestsConfig, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "noniso.xml"), []byte(nonIsoReport), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "iso.xml"), []byte(isoReport), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := &TestsConfig{Suites: []Suite{
		{ID: "acc-noniso", Name: "acc-noniso", Command: "go version", Path: ".", TestCountPath: "noniso.xml"},
		{ID: "acc-iso", Name: "acc-iso", Command: "go version", Path: ".", TestCountPath: "iso.xml"},
	}}
	return tests, dir
}

// TestRunTestsNamedPresentInOnePartitionPasses is the core fix: an isolated
// named test runs only in the isolated sub-suite; the non-isolated sub-suite
// legitimately runs zero. The presence union must still pass — the per-suite
// empty is no longer a failure.
func TestRunTestsNamedPresentInOnePartitionPasses(t *testing.T) {
	tests, dir := twoPartitionSuites(t,
		`<testsuite name="non-isolated" tests="0" skipped="0"></testsuite>`,
		`<testsuite name="isolated" tests="2"><testcase name="cannotCancelAnOrderAt2245OnDec31 [Channel: API]"/><testcase name="cannotCancelAnOrderAt2245OnDec31 [Channel: UI]"/></testsuite>`,
	)
	err := RunTests(nil, tests, dir, dir, TestOptions{Test: []string{"cannotCancelAnOrderAt2245OnDec31"}})
	if err != nil {
		t.Fatalf("named isolated test present in one partition must pass, got: %v", err)
	}
}

// TestRunTestsNamedAbsentEverywhereFails: a requested name that ran in no
// partition is the genuine fault the presence check exists to catch, surfaced
// with the prefix the verify classifier routes to the infra halt.
func TestRunTestsNamedAbsentEverywhereFails(t *testing.T) {
	tests, dir := twoPartitionSuites(t,
		`<testsuite name="non-isolated" tests="1"><testcase name="placeOrder [Channel: API]"/></testsuite>`,
		`<testsuite name="isolated" tests="1"><testcase name="someIsolatedTest [Channel: API]"/></testsuite>`,
	)
	err := RunTests(nil, tests, dir, dir, TestOptions{Test: []string{"ghostTest"}})
	if err == nil {
		t.Fatal("want error when the requested test ran nowhere")
	}
	if !strings.Contains(err.Error(), "requested test(s) never executed: ghostTest") {
		t.Fatalf("want the presence marker naming ghostTest, got: %v", err)
	}
}

// TestRunTestsMultiNamePartialMissFails is the case count>0 could not catch:
// two names requested, one ran, one didn't — the run must fail and name only
// the missing one.
func TestRunTestsMultiNamePartialMissFails(t *testing.T) {
	tests, dir := twoPartitionSuites(t,
		`<testsuite name="non-isolated" tests="1"><testcase name="present [Channel: API]"/></testsuite>`,
		`<testsuite name="isolated" tests="0" skipped="0"></testsuite>`,
	)
	err := RunTests(nil, tests, dir, dir, TestOptions{Test: []string{"present", "missing"}})
	if err == nil {
		t.Fatal("want error when one of two requested names ran nowhere")
	}
	if !strings.Contains(err.Error(), "requested test(s) never executed: missing") {
		t.Fatalf("want the presence marker naming only 'missing', got: %v", err)
	}
	if strings.Contains(err.Error(), "present") {
		t.Fatalf("the present name must not be reported missing, got: %v", err)
	}
}

// stageSuite writes a JUnit report file and returns a Suite reading it. The
// command is benign (`go version`, exits 0) — the staged report stands in for
// what the runner produced, the idiom twoPartitionSuites uses.
func stageSuite(t *testing.T, dir, id, report string) Suite {
	t.Helper()
	file := id + ".xml"
	if err := os.WriteFile(filepath.Join(dir, file), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	return Suite{ID: id, Name: id, Command: "go version", Path: ".", TestCountPath: file}
}

// TestNamesExecutedInUnionsAcrossSuites is the run-based membership primitive
// (Step 1): running two suites filtered to the names returns the union of the
// bare method names each report shows executed.
func TestNamesExecutedInUnionsAcrossSuites(t *testing.T) {
	dir := t.TempDir()
	tests := &TestsConfig{Suites: []Suite{
		stageSuite(t, dir, "acc-api", `<testsuite tests="1"><testcase name="apiOnly [Channel: API]"/></testsuite>`),
		stageSuite(t, dir, "acc-ui", `<testsuite tests="1"><testcase name="uiOnly [Channel: UI]"/></testsuite>`),
	}}
	got, err := NamesExecutedIn(tests, dir, []string{"acc-api", "acc-ui"}, []string{"apiOnly", "uiOnly"})
	if err != nil {
		t.Fatalf("NamesExecutedIn: %v", err)
	}
	if !got["apiOnly"] || !got["uiOnly"] {
		t.Fatalf("want union {apiOnly, uiOnly}, got %v", got)
	}
}

// TestNamesInReport_ReadsExistingReportsWithoutRunning is the artifact-read
// primitive (plan 20260619-1139, decision #6): NamesInReport returns the union
// of method names recorded in the on-disk reports for the named suites, reading
// the files staged here without running any command. A suite whose report is
// absent (the api-only ticket's UI partition) contributes nothing, and an
// unknown suite id is skipped rather than erroring.
func TestNamesInReport_ReadsExistingReportsWithoutRunning(t *testing.T) {
	dir := t.TempDir()
	tests := &TestsConfig{Suites: []Suite{
		// API partition has the ticket's test on disk.
		stageSuite(t, dir, "acceptance-parallel-api", `<testsuite tests="1"><testcase name="shouldRejectQty100 [Channel: API]"/></testsuite>`),
		// UI partition declared but its report never written (the RED filtered run
		// matched nothing in UI) — declare it with a TestCountPath that does not
		// exist on disk.
		{ID: "acceptance-parallel-ui", Name: "acceptance-parallel-ui", Path: ".", TestCountPath: "acceptance-parallel-ui.xml"},
	}}

	api, err := NamesInReport(tests, dir, []string{"acceptance-parallel-api"})
	if err != nil {
		t.Fatalf("NamesInReport(api): %v", err)
	}
	if !api["shouldRejectQty100"] {
		t.Errorf("api report: want shouldRejectQty100 present, got %v", api)
	}

	ui, err := NamesInReport(tests, dir, []string{"acceptance-parallel-ui"})
	if err != nil {
		t.Fatalf("NamesInReport(ui): %v", err)
	}
	if len(ui) != 0 {
		t.Errorf("ui report absent → want empty set, got %v", ui)
	}

	// An unknown suite id is skipped, not an error.
	none, err := NamesInReport(tests, dir, []string{"ghost-suite"})
	if err != nil {
		t.Fatalf("NamesInReport(ghost): unexpected err %v", err)
	}
	if len(none) != 0 {
		t.Errorf("unknown suite → want empty set, got %v", none)
	}
}

// TestNamesExecutedInEmptyNamesRunsNothing: empty names short-circuit before
// suite resolution, so even a nonexistent suite id can't surface an error.
func TestNamesExecutedInEmptyNamesRunsNothing(t *testing.T) {
	got, err := NamesExecutedIn(&TestsConfig{}, ".", []string{"ghost"}, nil)
	if err != nil {
		t.Fatalf("empty names must not run or error, got: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty set, got %v", got)
	}
}

// TestRunTestsNamedElsewhereSkips is the rehearsal-#76 fix: an API-only test
// reached by the UI verify executes in none of the selected UI suites, but the
// membership probe finds it in the complementary acceptance-api suite — so the
// run is a clean skip (nil), not the infra-halt error.
func TestRunTestsNamedElsewhereSkips(t *testing.T) {
	dir := t.TempDir()
	tests := &TestsConfig{Suites: []Suite{
		stageSuite(t, dir, "acc-ui", `<testsuite tests="0"></testsuite>`),
		stageSuite(t, dir, "acc-api", `<testsuite tests="1"><testcase name="apiOnly [Channel: API]"/></testsuite>`),
	}}
	err := RunTests(nil, tests, dir, dir, TestOptions{
		Suite:                 []string{"acc-ui"},
		Test:                  []string{"apiOnly"},
		MembershipProbeSuites: []string{"acc-ui", "acc-api"},
	})
	if err != nil {
		t.Fatalf("a named test registered for another channel must skip cleanly, got: %v", err)
	}
}

// TestRunTestsNamedAbsentEverywhereWithProbeFails: even with a probe universe, a
// name that ran nowhere — neither the selected suites nor the probed complement
// — still fails loud with the presence marker (a genuine typo).
func TestRunTestsNamedAbsentEverywhereWithProbeFails(t *testing.T) {
	dir := t.TempDir()
	tests := &TestsConfig{Suites: []Suite{
		stageSuite(t, dir, "acc-ui", `<testsuite tests="0"></testsuite>`),
		stageSuite(t, dir, "acc-api", `<testsuite tests="1"><testcase name="apiOnly [Channel: API]"/></testsuite>`),
	}}
	err := RunTests(nil, tests, dir, dir, TestOptions{
		Suite:                 []string{"acc-ui"},
		Test:                  []string{"ghostTest"},
		MembershipProbeSuites: []string{"acc-ui", "acc-api"},
	})
	if err == nil {
		t.Fatal("want fail-loud when the name exists nowhere, even with a probe universe")
	}
	if !strings.Contains(err.Error(), "requested test(s) never executed: ghostTest") {
		t.Fatalf("want the presence marker naming ghostTest, got: %v", err)
	}
}

// TestRunTestsNamedNonAcceptanceSelectionFailsLoud: the net is scoped to
// channel runs — a named run whose selection does NOT overlap the acceptance
// probe universe keeps the original fail-loud behaviour, so a smoke typo is not
// masked even when the name happens to exist in an acceptance suite.
func TestRunTestsNamedNonAcceptanceSelectionFailsLoud(t *testing.T) {
	dir := t.TempDir()
	tests := &TestsConfig{Suites: []Suite{
		stageSuite(t, dir, "smoke", `<testsuite tests="0"></testsuite>`),
		stageSuite(t, dir, "acc-api", `<testsuite tests="1"><testcase name="apiOnly [Channel: API]"/></testsuite>`),
	}}
	err := RunTests(nil, tests, dir, dir, TestOptions{
		Suite:                 []string{"smoke"},
		Test:                  []string{"apiOnly"},
		MembershipProbeSuites: []string{"acc-api"},
	})
	if err == nil {
		t.Fatal("want fail-loud for a non-acceptance selection even if the name exists in acceptance")
	}
	if !strings.Contains(err.Error(), "requested test(s) never executed: apiOnly") {
		t.Fatalf("want the presence marker, got: %v", err)
	}
}

// TestRunSetupExecutesSetupCommandsInOrder asserts that RunSetup runs every
// setupCommand in declaration order and wraps a failure with the failing
// command's Name.
func TestRunSetupExecutesSetupCommandsInOrder(t *testing.T) {
	t.Run("all pass", func(t *testing.T) {
		tests := &TestsConfig{SetupCommands: []SetupCommand{
			{Name: "first", Command: "go version"},
		}}
		if err := RunSetup(tests, "."); err != nil {
			t.Errorf("RunSetup: %v", err)
		}
	})
	t.Run("failure wraps with command name", func(t *testing.T) {
		tests := &TestsConfig{SetupCommands: []SetupCommand{
			{Name: "boom", Command: "go nonexistent-subcommand-xyz"},
		}}
		err := RunSetup(tests, ".")
		if err == nil {
			t.Fatal("want error from failing setup command")
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Errorf("want error to name the setup command, got: %v", err)
		}
	})
}

func TestSelectSuitesAllWhenSuiteIDEmpty(t *testing.T) {
	cfg := &TestsConfig{Suites: []Suite{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	got, err := selectSuites(cfg, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("want 3 suites, got %d", len(got))
	}
}

func TestSelectSuitesFilterToOne(t *testing.T) {
	cfg := &TestsConfig{Suites: []Suite{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	got, err := selectSuites(cfg, []string{"b"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 1 || got[0].ID != "b" {
		t.Errorf("want [b], got %+v", got)
	}
}

func TestSelectSuitesUnknownIDListsAvailable(t *testing.T) {
	cfg := &TestsConfig{Suites: []Suite{{ID: "smoke"}, {ID: "e2e"}}}
	_, err := selectSuites(cfg, []string{"missing"})
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "missing") {
		t.Errorf("want error to mention 'missing', got: %v", err)
	}
	if !strings.Contains(msg, "smoke") || !strings.Contains(msg, "e2e") {
		t.Errorf("want error to list available ids, got: %v", err)
	}
}

func TestSelectSuitesMultiPreservesDeclarationOrder(t *testing.T) {
	// User typed --suite C --suite A but declaration order is A, B, C —
	// so the result must come back as [A, C], regardless of arg order.
	cfg := &TestsConfig{Suites: []Suite{{ID: "A"}, {ID: "B"}, {ID: "C"}}}
	got, err := selectSuites(cfg, []string{"C", "A"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	gotIDs := []string{}
	for _, s := range got {
		gotIDs = append(gotIDs, s.ID)
	}
	want := []string{"A", "C"}
	if !reflect.DeepEqual(gotIDs, want) {
		t.Errorf("got %v, want %v", gotIDs, want)
	}
}

func TestSelectSuitesPartialUnknownReportsThatID(t *testing.T) {
	cfg := &TestsConfig{Suites: []Suite{{ID: "A"}, {ID: "B"}, {ID: "C"}}}
	_, err := selectSuites(cfg, []string{"A", "Z"})
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Z") {
		t.Errorf("want error to mention 'Z', got: %v", err)
	}
	for _, id := range []string{"A", "B", "C"} {
		if !strings.Contains(msg, id) {
			t.Errorf("want error to list available id %q, got: %v", id, err)
		}
	}
}

func TestSelectSuitesAllUnknownReportsAll(t *testing.T) {
	// Both unknown ids must show up in the error, not just the first —
	// users typoing a multi-suite invocation should see every miss.
	cfg := &TestsConfig{Suites: []Suite{{ID: "A"}, {ID: "B"}, {ID: "C"}}}
	_, err := selectSuites(cfg, []string{"X", "Y"})
	if err == nil {
		t.Fatal("want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "X") || !strings.Contains(msg, "Y") {
		t.Errorf("want error to mention both 'X' and 'Y', got: %v", err)
	}
}

func TestMergeEnvOverlayOverridesAndAppends(t *testing.T) {
	base := []string{"PATH=/bin", "FOO=oldfoo", "BAR=bar"}
	overlay := map[string]string{"FOO": "newfoo", "NEW": "newval"}
	got := mergeEnv(base, overlay)

	asMap := make(map[string]string, len(got))
	for _, kv := range got {
		eq := strings.IndexByte(kv, '=')
		asMap[kv[:eq]] = kv[eq+1:]
	}
	want := map[string]string{
		"PATH": "/bin",
		"FOO":  "newfoo",
		"BAR":  "bar",
		"NEW":  "newval",
	}
	if !reflect.DeepEqual(asMap, want) {
		t.Errorf("got %v, want %v", asMap, want)
	}
}

func TestSplitCommandRespectsQuotes(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`npx playwright test --grep 'shouldWork'`, []string{"npx", "playwright", "test", "--grep", "shouldWork"}},
		{`dotnet test --filter "FullyQualifiedName~Smoke"`, []string{"dotnet", "test", "--filter", "FullyQualifiedName~Smoke"}},
		{`echo hello   world`, []string{"echo", "hello", "world"}},
	}
	for _, c := range cases {
		got, err := splitCommand(c.in)
		if err != nil {
			t.Errorf("splitCommand(%q): unexpected error %v", c.in, err)
			continue
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("splitCommand(%q):\n  got:  %v\n  want: %v", c.in, got, c.want)
		}
	}
}

func TestSplitCommandUnterminatedQuoteErrors(t *testing.T) {
	_, err := splitCommand(`npx test --grep 'hello`)
	if err == nil {
		t.Fatal("want error for unterminated quote")
	}
	if !strings.Contains(err.Error(), "unterminated") {
		t.Errorf("want 'unterminated' in error, got %v", err)
	}
}

func TestFormatDur(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{1234 * time.Millisecond, "00:01.234"},
		{61 * time.Second, "01:01.000"},
		{0, "00:00.000"},
	}
	for _, c := range cases {
		if got := formatDur(c.in); got != c.want {
			t.Errorf("formatDur(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
