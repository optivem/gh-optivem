package main

import (
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
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

// TestLoadSystemMissingFileHintsAtFlag verifies that `gh optivem build/run/...`
// commands surface a --system-config hint (not just "file not found") when the
// default ./system.json is absent — the case a new user runs into first.
func TestLoadSystemMissingFileHintsAtFlag(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "system.json")
	_, err := loadSystem(missing)
	if err == nil {
		t.Fatalf("loadSystem(%q): want error, got nil", missing)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("loadSystem: want errors.Is(err, fs.ErrNotExist), got %v", err)
	}
	if !strings.Contains(err.Error(), "--system-config") {
		t.Errorf("loadSystem error missing --system-config hint: %v", err)
	}
}

// TestLoadTestsMissingFileHintsAtFlag mirrors the system-config hint check for
// --test-config (the second flag a `gh optivem test system` user typically
// needs to learn about).
func TestLoadTestsMissingFileHintsAtFlag(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "tests.json")
	_, err := loadTests(missing)
	if err == nil {
		t.Fatalf("loadTests(%q): want error, got nil", missing)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("loadTests: want errors.Is(err, fs.ErrNotExist), got %v", err)
	}
	if !strings.Contains(err.Error(), "--test-config") {
		t.Errorf("loadTests error missing --test-config hint: %v", err)
	}
}

// TestHintIfMissingPassesThrough ensures non-ENOENT errors come through
// unchanged (e.g. JSON parse errors keep their original message and don't
// gain a misleading "pass --... " suggestion).
func TestHintIfMissingPassesThrough(t *testing.T) {
	want := errors.New("parse system config: bad json")
	got := hintIfMissing(want, "--system-config", defaultSystemConfig)
	if got != want {
		t.Errorf("hintIfMissing rewrote a non-ENOENT error: got %v, want %v", got, want)
	}
	if hintIfMissing(nil, "--system-config", defaultSystemConfig) != nil {
		t.Errorf("hintIfMissing(nil) returned non-nil")
	}
}
