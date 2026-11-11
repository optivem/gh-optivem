package runner

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/spec"
)

func TestParseUseCaseIDsReducesAndDedupes(t *testing.T) {
	got, err := ParseUseCaseIDs([]string{"UC-003.2", " UC-001 ", "", "UC-003", "UC-001.4", "UC-010.1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"UC-003", "UC-001", "UC-010"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestParseUseCaseIDsEmptyIsWholeSuite(t *testing.T) {
	got, err := ParseUseCaseIDs([]string{"", " "})
	if err != nil || len(got) != 0 {
		t.Fatalf("want empty selection, got %v, %v", got, err)
	}
	sel, err := ResolveUseCaseSelection(nil, "java", nil, "")
	if err != nil || sel.Active() {
		t.Fatalf("empty --use-case must be inactive, got %+v, %v", sel, err)
	}
}

func TestParseUseCaseIDsRejectsMalformed(t *testing.T) {
	_, err := ParseUseCaseIDs([]string{"UC-001", "uc-2", "UC-003.x"})
	if err == nil || !strings.Contains(err.Error(), "uc-2, UC-003.x") {
		t.Fatalf("want error naming the bad tokens, got %v", err)
	}
}

func specUseCases() []spec.UseCase {
	return []spec.UseCase{
		{ID: "UC-001", Criteria: []spec.Criterion{{ID: "UC-001.1"}, {ID: "UC-001.2"}}},
		{ID: "UC-003"}, // done use case: criteria deleted from the spec
	}
}

func TestResolveUseCaseSelectionUnknownUseCaseErrors(t *testing.T) {
	_, err := ResolveUseCaseSelection([]string{"UC-001", "UC-099.1"}, "dotnet", specUseCases(), "")
	if err == nil || !strings.Contains(err.Error(), "UC-099") {
		t.Fatalf("want error naming UC-099, got %v", err)
	}
}

func TestResolveUseCaseSelectionUnknownLangErrors(t *testing.T) {
	_, err := ResolveUseCaseSelection([]string{"UC-001"}, "python", specUseCases(), "")
	if err == nil || !strings.Contains(err.Error(), "python") {
		t.Fatalf("want unsupported-lang error, got %v", err)
	}
}

func TestResolveUseCaseSelectionJavaUnionsSpecAndSourceTags(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src", "test", "java")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	java := `@Tag("UC-001.2") @Tag("UC-001.10") void a(){} @Tag("UC-003.1") void b(){} @Tag("UC-0031.1") void c(){}`
	if err := os.WriteFile(filepath.Join(src, "ATest.java"), []byte(java), 0o644); err != nil {
		t.Fatal(err)
	}
	// build output must be ignored
	if err := os.MkdirAll(filepath.Join(dir, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "build", "Gen.java"), []byte(`"UC-001.99"`), 0o644); err != nil {
		t.Fatal(err)
	}
	sel, err := ResolveUseCaseSelection([]string{"UC-003.4", "UC-001"}, "java", specUseCases(), dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"UC-003": {"UC-003.1"},
		"UC-001": {"UC-001.1", "UC-001.2", "UC-001.10"},
	}
	if !reflect.DeepEqual(sel.Criteria, want) {
		t.Fatalf("criteria: got %v, want %v", sel.Criteria, want)
	}
	if !reflect.DeepEqual(sel.UseCases, []string{"UC-003", "UC-001"}) {
		t.Fatalf("use cases: got %v", sel.UseCases)
	}
}

func TestResolveUseCaseSelectionJavaNoCriteriaErrors(t *testing.T) {
	_, err := ResolveUseCaseSelection([]string{"UC-003"}, "java", specUseCases(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "UC-003") {
		t.Fatalf("want error naming UC-003 (no criterion IDs), got %v", err)
	}
}

func TestApplyUseCaseFilterDotnet(t *testing.T) {
	sel := UseCaseSelection{Lang: "dotnet", UseCases: []string{"UC-001", "UC-003"}}
	cases := []struct{ name, in, want string }{
		{"inject into existing filter",
			`dotnet test --filter 'FullyQualifiedName~.Latest.AcceptanceTests&Category!=isolated' -e CHANNEL=API`,
			`dotnet test --filter 'FullyQualifiedName~.Latest.AcceptanceTests&Category!=isolated&(tag~UC-001.|tag~UC-003.)' -e CHANNEL=API`},
		{"existing top-level OR is parenthesised",
			`dotnet test --filter 'A~x|B~y'`,
			`dotnet test --filter '(A~x|B~y)&(tag~UC-001.|tag~UC-003.)'`},
		{"no filter appends one",
			`dotnet test --logger trx`,
			`dotnet test --logger trx --filter '(tag~UC-001.|tag~UC-003.)'`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := applyUseCaseFilter(c.in, sel)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("\n got:  %q\n want: %q", got, c.want)
			}
		})
	}
	if _, err := applyUseCaseFilter(`dotnet test --filter "A~x"`, sel); err == nil {
		t.Error("double-quoted --filter must fail loud rather than run unfiltered")
	}
}

func TestApplyUseCaseFilterJava(t *testing.T) {
	sel := UseCaseSelection{Lang: "java", UseCases: []string{"UC-001", "UC-003"},
		Criteria: map[string][]string{"UC-001": {"UC-001.1", "UC-001.2"}, "UC-003": {"UC-003.1"}}}
	cases := []struct{ name, in, want string }{
		{"no include appends",
			`.\gradlew.bat test -Dtype=acceptance -DexcludeTags=isolated -Dchannel=API`,
			`.\gradlew.bat test -Dtype=acceptance -DexcludeTags=isolated -Dchannel=API -DincludeTags=UC-001.1|UC-001.2|UC-003.1`},
		{"existing include is ANDed",
			`.\gradlew.bat test -DincludeTags=isolated -Dchannel=API`,
			`.\gradlew.bat test -DincludeTags=isolated&(UC-001.1|UC-001.2|UC-003.1) -Dchannel=API`},
		{"comma include list folded",
			`gradlew test -DincludeTags=a,b`,
			`gradlew test -DincludeTags=(a|b)&(UC-001.1|UC-001.2|UC-003.1)`},
		{"quoted include keeps its quotes",
			`gradlew test '-DincludeTags=isolated' -x`,
			`gradlew test '-DincludeTags=isolated&(UC-001.1|UC-001.2|UC-003.1)' -x`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := applyUseCaseFilter(c.in, sel)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("\n got:  %q\n want: %q", got, c.want)
			}
		})
	}
	// The rewritten token survives splitCommand as one argv entry.
	got, _ := applyUseCaseFilter(`gradlew test -DincludeTags=isolated`, sel)
	parts, err := splitCommand(got)
	if err != nil || parts[len(parts)-1] != "-DincludeTags=isolated&(UC-001.1|UC-001.2|UC-003.1)" {
		t.Fatalf("argv: %v, %v", parts, err)
	}
}

func TestApplyUseCaseFilterTypescript(t *testing.T) {
	sel := UseCaseSelection{Lang: "typescript", UseCases: []string{"UC-001", "UC-003"}}
	cases := []struct{ name, in, want string }{
		{"no grep appends",
			`npx playwright test --pass-with-no-tests --project=acceptance-test tests/latest/acceptance`,
			`npx playwright test --pass-with-no-tests --project=acceptance-test tests/latest/acceptance --grep '@UC-001\.|@UC-003\.'`},
		{"grep-invert is not a grep",
			`npx playwright test --grep-invert '@isolated' tests`,
			`npx playwright test --grep-invert '@isolated' tests --grep '@UC-001\.|@UC-003\.'`},
		{"existing grep becomes lookahead AND",
			`npx playwright test --workers=1 --grep '@isolated' tests/latest/acceptance`,
			`npx playwright test --workers=1 --grep '^(?=.*@isolated)(?=.*(@UC-001\.|@UC-003\.))' tests/latest/acceptance`},
		{"grep= form",
			`npx playwright test --grep=@isolated`,
			`npx playwright test --grep '^(?=.*@isolated)(?=.*(@UC-001\.|@UC-003\.))'`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := applyUseCaseFilter(c.in, sel)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("\n got:  %q\n want: %q", got, c.want)
			}
		})
	}
}

func TestApplyUseCaseFilterInactiveUnchanged(t *testing.T) {
	got, err := applyUseCaseFilter("dotnet test", UseCaseSelection{Lang: "dotnet"})
	if err != nil || got != "dotnet test" {
		t.Fatalf("inactive selection must not touch the command, got %q, %v", got, err)
	}
}

// tsSel is a use-case selection whose rewrite (`--grep '...'` appended) keeps
// the `go env GOOS` stand-in command exiting 0.
var tsSel = UseCaseSelection{Lang: "typescript", UseCases: []string{"UC-001"}}

func TestRunTestsUseCaseZeroExecutedFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "r.json"), []byte(`{"suites":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := &TestsConfig{Suites: []Suite{{ID: "acc", Name: "acc", Command: "go env GOOS", Path: ".", TestCountPath: "r.json"}}}
	err := RunTests(nil, tests, dir, dir, TestOptions{UseCase: tsSel})
	if err == nil || !strings.Contains(err.Error(), "0 tests executed for the given selection") || !strings.Contains(err.Error(), "UC-001") {
		t.Fatalf("want the empty-selection marker naming the use case, got %v", err)
	}
}

func TestRunTestsUseCaseMissingPartitionReportTolerated(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "iso.xml"),
		[]byte(`<testsuite name="x" tests="1"><testcase name="t1"/></testsuite>`), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := &TestsConfig{Suites: []Suite{
		{ID: "a", Name: "a", Command: "go env GOOS", Path: ".", TestCountPath: "absent.xml"},
		{ID: "b", Name: "b", Command: "go env GOOS", Path: ".", TestCountPath: "iso.xml"},
	}}
	if err := RunTests(nil, tests, dir, dir, TestOptions{UseCase: tsSel}); err != nil {
		t.Fatalf("use-case run must tolerate an empty partition when tests ran elsewhere, got %v", err)
	}
}

func TestRunTestsUseCaseNeedsCountedSuite(t *testing.T) {
	dir := t.TempDir()
	tests := &TestsConfig{Suites: []Suite{{ID: "e2e", Name: "e2e", Command: "go env GOOS", Path: "."}}}
	err := RunTests(nil, tests, dir, dir, TestOptions{UseCase: tsSel})
	if err == nil || !strings.Contains(err.Error(), "testCountPath") {
		t.Fatalf("want error: no suite can guard the selection, got %v", err)
	}
}

func TestRunTestsUseCaseRejectsTestAndSample(t *testing.T) {
	dir := t.TempDir()
	tests := &TestsConfig{Suites: []Suite{{ID: "a", Name: "a", Command: "go env GOOS", Path: ".", TestCountPath: "r.json"}}}
	for _, opts := range []TestOptions{{UseCase: tsSel, Test: []string{"x"}}, {UseCase: tsSel, Sample: true}} {
		if err := RunTests(nil, tests, dir, dir, opts); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
			t.Fatalf("want mutual-exclusion error, got %v", err)
		}
	}
}

func TestRunTestsUseCaseDoesNotMutateConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "iso.xml"),
		[]byte(`<testsuite name="x" tests="1"><testcase name="t1"/></testsuite>`), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := &TestsConfig{Suites: []Suite{{ID: "b", Name: "b", Command: "go env GOOS", Path: ".", TestCountPath: "iso.xml"}}}
	if err := RunTests(nil, tests, dir, dir, TestOptions{UseCase: tsSel}); err != nil {
		t.Fatal(err)
	}
	if tests.Suites[0].Command != "go env GOOS" {
		t.Fatalf("tests config mutated: %q", tests.Suites[0].Command)
	}
}
