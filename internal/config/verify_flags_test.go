package config

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// TestValidateVerifyFlags covers the --lang rejection paths the CLI's Run
// closure used to reach via os.Exit. The function aggregates every bad
// --lang value so a typo in a long comma-separated list surfaces all
// offenders at once.
func TestValidateVerifyFlags(t *testing.T) {
	cases := []struct {
		name        string
		langs       []string
		wantErr     bool
		wantSubstrs []string
	}{
		{
			name:    "all valid",
			langs:   []string{projectconfig.LangTypescript, projectconfig.LangDotnet},
			wantErr: false,
		},
		{
			name:    "empty inputs pass",
			langs:   nil,
			wantErr: false,
		},
		{
			name:        "single bad lang",
			langs:       []string{"rust"},
			wantErr:     true,
			wantSubstrs: []string{"--lang", "rust", "java", "dotnet", "typescript"},
		},
		{
			name:        "multiple bad langs reported together",
			langs:       []string{"rust", projectconfig.LangJava, "perl"},
			wantErr:     true,
			wantSubstrs: []string{"rust", "perl"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateVerifyFlags(tc.langs)
			assertVerifyFlagsErr(t, err, tc.wantErr, tc.wantSubstrs)
		})
	}
}

func assertVerifyFlagsErr(t *testing.T, err error, wantErr bool, wantSubstrs []string) {
	t.Helper()

	if wantErr && err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !wantErr && err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if err == nil {
		return
	}
	assertErrSubstrings(t, err.Error(), wantSubstrs)
}

func assertErrSubstrings(t *testing.T, msg string, wantSubstrs []string) {
	t.Helper()

	for _, s := range wantSubstrs {
		if !strings.Contains(msg, s) {
			t.Errorf("error missing substring %q. Got:\n%s", s, msg)
		}
	}
}
