package main

import (
	"reflect"
	"testing"
)

// TestNewTestSystemCmdRepeatableTestFlag verifies cobra's StringSliceVar
// wiring on --test: the flag is repeatable AND comma-separated, and an
// absent flag yields an empty slice (no filter).
func TestNewTestSystemCmdRepeatableTestFlag(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{"repeated --test", []string{"--test", "T1", "--test", "T2"}, []string{"T1", "T2"}},
		{"comma-separated value", []string{"--test", "T1,T2"}, []string{"T1", "T2"}},
		{"mixed repeat + comma", []string{"--test", "T1,T2", "--test", "T3"}, []string{"T1", "T2", "T3"}},
		{"single value", []string{"--test", "Only"}, []string{"Only"}},
		{"flag absent", []string{}, []string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cmd := newTestSystemCmd()
			if err := cmd.ParseFlags(c.args); err != nil {
				t.Fatalf("ParseFlags(%v): %v", c.args, err)
			}
			got, err := cmd.Flags().GetStringSlice("test")
			if err != nil {
				t.Fatalf("GetStringSlice: %v", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}
