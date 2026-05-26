package testselect

import (
	"reflect"
	"testing"
)

// TestExpandSuiteGroups exercises the alias-expansion contract that
// `gh optivem test run --suite=…` relies on so the BPMN literal
// `suite: acceptance` (process-flow.yaml:761,769) resolves to the
// canonical acceptance suite ids without burning a yaml schema slot.
//
// Cases with a nil projectGroups exercise the Go-side defaultSuiteGroups
// fallback (the behavior when a project's tests.yaml omits a
// `suiteGroups:` block). Cases with a non-nil projectGroups exercise the
// per-project override and new-group registration.
func TestExpandSuiteGroups(t *testing.T) {
	cases := []struct {
		name          string
		in            []string
		projectGroups map[string][]string
		want          []string
	}{
		{
			name: "empty input passes through",
			in:   nil,
			want: nil,
		},
		{
			name: "pure alias expands via default registry",
			in:   []string{"acceptance"},
			want: []string{"acceptance-api", "acceptance-ui"},
		},
		{
			name: "non-alias name passes through unchanged",
			in:   []string{"acceptance-api"},
			want: []string{"acceptance-api"},
		},
		{
			name: "unknown name passes through (downstream catches typos)",
			in:   []string{"acceptance-typo"},
			want: []string{"acceptance-typo"},
		},
		{
			name: "alias + overlapping explicit is de-duped, first-seen order preserved",
			in:   []string{"acceptance", "acceptance-api"},
			want: []string{"acceptance-api", "acceptance-ui"},
		},
		{
			name: "explicit before alias is de-duped, first-seen order preserved",
			in:   []string{"acceptance-ui", "acceptance"},
			want: []string{"acceptance-ui", "acceptance-api"},
		},
		{
			name: "alias mixed with unrelated suite",
			in:   []string{"acceptance", "contract-stub"},
			want: []string{"acceptance-api", "acceptance-ui", "contract-stub"},
		},
		{
			name:          "project override replaces default acceptance group",
			in:            []string{"acceptance"},
			projectGroups: map[string][]string{"acceptance": {"acceptance-api"}},
			want:          []string{"acceptance-api"},
		},
		{
			name:          "project-defined new group expands",
			in:            []string{"contract"},
			projectGroups: map[string][]string{"contract": {"contract-stub", "contract-real"}},
			want:          []string{"contract-stub", "contract-real"},
		},
		{
			name:          "default group still resolves when project declares unrelated group",
			in:            []string{"acceptance"},
			projectGroups: map[string][]string{"contract": {"contract-stub"}},
			want:          []string{"acceptance-api", "acceptance-ui"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExpandSuiteGroups(c.in, c.projectGroups)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ExpandSuiteGroups(%v, %v) = %v, want %v", c.in, c.projectGroups, got, c.want)
			}
		})
	}
}
