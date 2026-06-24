package testselect

import (
	"reflect"
	"testing"
)

// TestExpandSuiteGroups exercises the alias-expansion contract that
// `gh optivem system-test run --suite=…` relies on so the BPMN literal
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
			want: []string{"acceptance-parallel-api", "acceptance-isolated-api", "acceptance-parallel-ui", "acceptance-isolated-ui"},
		},
		{
			// The correctness-by-construction guard: the bare per-channel id is a
			// GROUP that fans out to both partitions, so the per-channel verify
			// (statemachine/channels.go binds `acceptance-<ch>`) can never silently
			// drop the isolated partition — the #76 coverage hole is unreintroducible.
			name: "per-channel alias expands to both partitions (default registry)",
			in:   []string{"acceptance-api"},
			want: []string{"acceptance-parallel-api", "acceptance-isolated-api"},
		},
		{
			name: "concrete partition suite passes through unchanged",
			in:   []string{"acceptance-parallel-api"},
			want: []string{"acceptance-parallel-api"},
		},
		{
			name: "unknown name passes through (downstream catches typos)",
			in:   []string{"acceptance-typo"},
			want: []string{"acceptance-typo"},
		},
		{
			name: "alias + overlapping per-channel alias is de-duped, first-seen order preserved",
			in:   []string{"acceptance", "acceptance-api"},
			want: []string{"acceptance-parallel-api", "acceptance-isolated-api", "acceptance-parallel-ui", "acceptance-isolated-ui"},
		},
		{
			name: "per-channel alias before top-level alias is de-duped, first-seen order preserved",
			in:   []string{"acceptance-ui", "acceptance"},
			want: []string{"acceptance-parallel-ui", "acceptance-isolated-ui", "acceptance-parallel-api", "acceptance-isolated-api"},
		},
		{
			name: "alias mixed with unrelated suite",
			in:   []string{"acceptance", "contract-stub"},
			want: []string{"acceptance-parallel-api", "acceptance-isolated-api", "acceptance-parallel-ui", "acceptance-isolated-ui", "contract-stub"},
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
			want:          []string{"acceptance-parallel-api", "acceptance-isolated-api", "acceptance-parallel-ui", "acceptance-isolated-ui"},
		},
		{
			name:     "channel-aware: api-only project expands acceptance to api ids only",
			in:       []string{"acceptance"},
			channels: []string{"api"},
			want:     []string{"acceptance-parallel-api", "acceptance-isolated-api"},
		},
		{
			name:     "channel-aware: non-{api,ui} channel set drives the default group",
			in:       []string{"acceptance"},
			channels: []string{"mobile", "api"},
			want:     []string{"acceptance-parallel-mobile", "acceptance-isolated-mobile", "acceptance-parallel-api", "acceptance-isolated-api"},
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
