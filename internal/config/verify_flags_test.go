package config

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// TestValidateVerifyFlags covers the --lang / --deploy rejection paths the
// CLI's Run closure used to reach via os.Exit. The function aggregates every
// bad --lang value so a typo in a long comma-separated list surfaces all
// offenders at once.
func TestValidateVerifyFlags(t *testing.T) {
	cases := []struct {
		name        string
		langs       []string
		deploy      string
		wantErr     bool
		wantSubstrs []string
	}{
		{
			name:    "all valid",
			langs:   []string{projectconfig.LangTypescript, projectconfig.LangDotnet},
			deploy:  projectconfig.DeployDocker,
			wantErr: false,
		},
		{
			name:    "empty inputs pass",
			langs:   nil,
			deploy:  "",
			wantErr: false,
		},
		{
			name:        "single bad lang",
			langs:       []string{"rust"},
			deploy:      "",
			wantErr:     true,
			wantSubstrs: []string{"--lang", "rust", "java", "dotnet", "typescript"},
		},
		{
			name:        "multiple bad langs reported together",
			langs:       []string{"rust", projectconfig.LangJava, "perl"},
			deploy:      "",
			wantErr:     true,
			wantSubstrs: []string{"rust", "perl"},
		},
		{
			name:        "bad deploy",
			langs:       nil,
			deploy:      "fargate",
			wantErr:     true,
			wantSubstrs: []string{"--deploy", "fargate", projectconfig.DeployDocker, projectconfig.DeployCloudRun},
		},
		{
			name:        "bad lang short-circuits before deploy check",
			langs:       []string{"rust"},
			deploy:      "fargate",
			wantErr:     true,
			wantSubstrs: []string{"--lang", "rust"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateVerifyFlags(tc.langs, tc.deploy)
			if tc.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected nil error, got %v", err)
			}
			if err == nil {
				return
			}
			msg := err.Error()
			for _, s := range tc.wantSubstrs {
				if !strings.Contains(msg, s) {
					t.Errorf("error missing substring %q. Got:\n%s", s, msg)
				}
			}
		})
	}
}
