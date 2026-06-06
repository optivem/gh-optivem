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
		channels      []string
		want          []string
	}{
		{
			name: "empty input passes through",
			in:   nil,
			want: nil,
		},
		{
			name: "pure alias expands via default registry (nil channels = {api,ui} fallback)",
			in:   []string{"acceptance"},
			want: []string{"acceptance-api", "acceptance-isolated-api", "acceptance-ui", "acceptance-isolated-ui"},
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
			want: []string{"acceptance-api", "acceptance-isolated-api", "acceptance-ui", "acceptance-isolated-ui"},
		},
		{
			name: "explicit before alias is de-duped, first-seen order preserved",
			in:   []string{"acceptance-ui", "acceptance"},
			want: []string{"acceptance-ui", "acceptance-api", "acceptance-isolated-api", "acceptance-isolated-ui"},
		},
		{
			name: "alias mixed with unrelated suite",
			in:   []string{"acceptance", "contract-stub"},
			want: []string{"acceptance-api", "acceptance-isolated-api", "acceptance-ui", "acceptance-isolated-ui", "contract-stub"},
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
			want:          []string{"acceptance-api", "acceptance-isolated-api", "acceptance-ui", "acceptance-isolated-ui"},
		},
		{
			name:     "channel-aware: api-only project expands acceptance to api ids only",
			in:       []string{"acceptance"},
			channels: []string{"api"},
			want:     []string{"acceptance-api", "acceptance-isolated-api"},
		},
		{
			name:     "channel-aware: non-{api,ui} channel set drives the default group",
			in:       []string{"acceptance"},
			channels: []string{"mobile", "api"},
			want:     []string{"acceptance-mobile", "acceptance-isolated-mobile", "acceptance-api", "acceptance-isolated-api"},
		},
		{
			name:          "project override wins over channel-derived default",
			in:            []string{"acceptance"},
			channels:      []string{"api"},
			projectGroups: map[string][]string{"acceptance": {"acceptance-api", "acceptance-ui"}},
			want:          []string{"acceptance-api", "acceptance-ui"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExpandSuiteGroups(c.in, c.projectGroups, c.channels)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ExpandSuiteGroups(%v, %v, %v) = %v, want %v", c.in, c.projectGroups, c.channels, got, c.want)
			}
		})
	}
}
