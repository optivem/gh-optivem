package runner

import (
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
		expected string
	}{
		{"explicit Test wins over Sample", TestOptions{Test: "explicit", Sample: true}, "explicit"},
		{"only Test", TestOptions{Test: "explicit"}, "explicit"},
		{"only Sample", TestOptions{Sample: true}, "shouldWork"},
		{"neither", TestOptions{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := pickFilterValue(suite, c.opts); got != c.expected {
				t.Errorf("got %q, want %q", got, c.expected)
			}
		})
	}
}

func TestPrepareSystemNilIsNoOp(t *testing.T) {
	if err := prepareSystem(nil, ".", TestOptions{}); err != nil {
		t.Errorf("nil sys should be a no-op, got %v", err)
	}
}

func TestPrepareSystemNoStartProbesWhenDown(t *testing.T) {
	// SystemEntry with no components/external systems → IsAnyURLUp returns
	// false trivially without making any network calls. With NoStart=true,
	// prepareSystem should refuse to proceed and surface the "start it
	// first" message.
	sys := &SystemConfig{Systems: []SystemEntry{{Label: "test-stack"}}}
	err := prepareSystem(sys, ".", TestOptions{NoStart: true, NoBuild: true})
	if err == nil {
		t.Fatal("want error when --no-start and system not running")
	}
	if !strings.Contains(err.Error(), "test-stack") {
		t.Errorf("want error to name the system label, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--no-start") {
		t.Errorf("want error to mention --no-start, got: %v", err)
	}
}

func TestSelectSuitesAllWhenSuiteIDEmpty(t *testing.T) {
	cfg := &TestsConfig{Suites: []Suite{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	got, err := selectSuites(cfg, "")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("want 3 suites, got %d", len(got))
	}
}

func TestSelectSuitesFilterToOne(t *testing.T) {
	cfg := &TestsConfig{Suites: []Suite{{ID: "a"}, {ID: "b"}, {ID: "c"}}}
	got, err := selectSuites(cfg, "b")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(got) != 1 || got[0].ID != "b" {
		t.Errorf("want [b], got %+v", got)
	}
}

func TestSelectSuitesUnknownIDListsAvailable(t *testing.T) {
	cfg := &TestsConfig{Suites: []Suite{{ID: "smoke"}, {ID: "e2e"}}}
	_, err := selectSuites(cfg, "missing")
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
