package testselect

import (
	"reflect"
	"testing"
)

// TestExpandSuiteGroups exercises the alias-expansion contract that
// `gh optivem test run --suite=…` relies on so the BPMN literal
// `suite: acceptance` (process-flow.yaml:761,769) resolves to the
// canonical acceptance suite ids without burning a yaml schema slot.
func TestExpandSuiteGroups(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "empty input passes through",
			in:   nil,
			want: nil,
		},
		{
			name: "pure alias expands",
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
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExpandSuiteGroups(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ExpandSuiteGroups(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
