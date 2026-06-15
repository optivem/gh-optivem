package config

import (
	"testing"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// TestCompilerChecksFor covers the dispatcher in isolation; the underlying
// verifyNpm/verifyDotnet/verifyJava are thin exec.LookPath wrappers (stdlib
// smoke tests, no logic of ours) and are intentionally not exercised here.
func TestCompilerChecksFor(t *testing.T) {
	tests := []struct {
		name      string
		langs     []string
		wantNames []string
	}{
		{
			name:      "empty list yields no checks",
			langs:     nil,
			wantNames: nil,
		},
		{
			name:      "all three langs yields three checks",
			langs:     []string{projectconfig.LangTypescript, projectconfig.LangDotnet, projectconfig.LangJava},
			wantNames: []string{"npm", "dotnet", "java"},
		},
		{
			name:      "duplicates are deduped",
			langs:     []string{projectconfig.LangTypescript, projectconfig.LangTypescript, projectconfig.LangJava},
			wantNames: []string{"npm", "java"},
		},
		{
			name:      "unknown lang is silently skipped",
			langs:     []string{projectconfig.LangTypescript, "rust"},
			wantNames: []string{"npm"},
		},
		{
			name:      "empty string in slice is skipped",
			langs:     []string{"", projectconfig.LangDotnet},
			wantNames: []string{"dotnet"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compilerChecksFor(tt.langs)
			if len(got) != len(tt.wantNames) {
				t.Fatalf("compilerChecksFor(%v) returned %d checks, want %d (names: %v)",
					tt.langs, len(got), len(tt.wantNames), namesOf(got))
			}
			for i, want := range tt.wantNames {
				if got[i].name != want {
					t.Errorf("check[%d].name = %q, want %q", i, got[i].name, want)
				}
				if got[i].fn == nil {
					t.Errorf("check[%d].fn is nil", i)
				}
			}
		})
	}
}

func namesOf(cs []check) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.name
	}
	return out
}

// TestDeployChecksFor mirrors TestCompilerChecksFor: the dispatcher is the
// single source of truth for "what tools does deploy X need", so adding
// cloud-run later (currently gated as in-development at
// projectconfig.IsValidDeploy) means adding one case to the switch and one
// row to this table — nothing else.
func TestDeployChecksFor(t *testing.T) {
	tests := []struct {
		name      string
		deploy    string
		wantNames []string
	}{
		{
			name:      "empty deploy yields no checks",
			deploy:    "",
			wantNames: nil,
		},
		{
			name:      "docker yields one check",
			deploy:    projectconfig.DeployDocker,
			wantNames: []string{"docker"},
		},
		{
			name:      "cloud-run yields no checks today",
			deploy:    projectconfig.DeployCloudRun,
			wantNames: nil,
		},
		{
			name:      "unknown deploy is silently skipped",
			deploy:    "fargate",
			wantNames: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deployChecksFor(tt.deploy)
			if len(got) != len(tt.wantNames) {
				t.Fatalf("deployChecksFor(%q) returned %d checks, want %d (names: %v)",
					tt.deploy, len(got), len(tt.wantNames), namesOf(got))
			}
			for i, want := range tt.wantNames {
				if got[i].name != want {
					t.Errorf("check[%d].name = %q, want %q", i, got[i].name, want)
				}
				if got[i].fn == nil {
					t.Errorf("check[%d].fn is nil", i)
				}
			}
		})
	}
}
