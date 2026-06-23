package componenttest

import (
	"reflect"
	"strings"
	"testing"
)

func TestComponentLabel(t *testing.T) {
	if got := (Component{Name: "backend", Lang: "java"}).label(); got != "backend (java)" {
		t.Errorf("label = %q", got)
	}
	if got := (Component{Name: "monolith"}).label(); got != "monolith" {
		t.Errorf("label without lang = %q", got)
	}
}

func TestPickFilterValue(t *testing.T) {
	s := Suite{SampleTest: "the sample"}
	cases := []struct {
		name string
		opts Options
		want []string
	}{
		{"test wins over sample", Options{Test: []string{"T1"}, Sample: true}, []string{"T1"}},
		{"sample when set", Options{Sample: true}, []string{"the sample"}},
		{"nothing", Options{}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickFilterValue(s, tc.opts); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("pickFilterValue = %v, want %v", got, tc.want)
			}
		})
	}
	t.Run("sample empty when suite has no sampleTest", func(t *testing.T) {
		if got := pickFilterValue(Suite{}, Options{Sample: true}); got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})
}

func TestSelectComponents(t *testing.T) {
	all := []Component{
		{Name: "backend", Path: "be"},
		{Name: "frontend", Path: "fe"},
	}

	t.Run("empty selects all", func(t *testing.T) {
		got, err := selectComponents(all, nil)
		if err != nil || !reflect.DeepEqual(got, all) {
			t.Fatalf("got %v, err %v", got, err)
		}
	})

	t.Run("filters by name", func(t *testing.T) {
		got, err := selectComponents(all, []string{"frontend"})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 1 || got[0].Name != "frontend" {
			t.Errorf("got %v", got)
		}
	})

	t.Run("dedupes repeated request", func(t *testing.T) {
		got, err := selectComponents(all, []string{"backend", "backend"})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 1 {
			t.Errorf("expected 1 component, got %v", got)
		}
	})

	t.Run("unknown name fails loud with available", func(t *testing.T) {
		_, err := selectComponents(all, []string{"bogus"})
		if err == nil || !strings.Contains(err.Error(), "component(s) not found: bogus") {
			t.Fatalf("expected not-found, got %v", err)
		}
		if !strings.Contains(err.Error(), "backend") || !strings.Contains(err.Error(), "frontend") {
			t.Errorf("error should list available, got %v", err)
		}
	})

	t.Run("no components at all", func(t *testing.T) {
		_, err := selectComponents(nil, nil)
		if err == nil || !strings.Contains(err.Error(), "no components found") {
			t.Fatalf("expected no-components error, got %v", err)
		}
	})
}

// TestRunSuite_Pending verifies a pending suite is never executed: it returns a
// PENDING result with no error and runs no command (the bogus command would
// fail if it were run).
func TestRunSuite_Pending(t *testing.T) {
	s := Suite{ID: "integration", Name: "Narrow Integration", Pending: true, Command: "this-command-does-not-exist"}
	r, err := runSuite(Component{Name: "backend", Path: "."}, s, Options{})
	if err != nil {
		t.Fatalf("pending suite should not error, got: %v", err)
	}
	if r.status != "PENDING" {
		t.Errorf("status = %q, want PENDING", r.status)
	}
}

// TestRunSuite_RequiresDockerFailsFastWhenAbsent only asserts the fail-fast
// message when Docker is genuinely unavailable. When a daemon IS reachable the
// branch can't be exercised without actually running the suite command, so the
// test skips rather than executing an arbitrary command.
func TestRunSuite_RequiresDockerFailsFastWhenAbsent(t *testing.T) {
	if dockerAvailable() {
		t.Skip("docker daemon reachable; cannot exercise the absent-daemon branch in isolation")
	}
	s := Suite{ID: "component", Name: "Component", Command: "echo should-not-run", RequiresDocker: true}
	_, err := runSuite(Component{Name: "backend", Path: "."}, s, Options{})
	if err == nil || !strings.Contains(err.Error(), "requires Docker") {
		t.Fatalf("expected docker-required error, got: %v", err)
	}
}
