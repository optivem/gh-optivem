package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseSlugsRejectsBadShape(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"single ok", []string{"optivem/shop"}, false},
		{"multiple ok", []string{"optivem/shop", "optivem/eshop-tests"}, false},
		{"bare repo", []string{"shop"}, true},
		{"extra segment", []string{"optivem/shop/extra"}, true},
		{"mix with bad", []string{"optivem/shop", "shop"}, true},
		{"empty owner", []string{"/shop"}, true},
		{"empty repo", []string{"optivem/"}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseSlugs(tc.args)
			if (err != nil) != tc.wantErr {
				t.Fatalf("parseSlugs(%v) err=%v, wantErr=%v", tc.args, err, tc.wantErr)
			}
		})
	}
}

func TestCommonFlagsToOptions(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		f := commonFlags{DelaySecs: 10}
		opt, err := f.toOptions()
		if err != nil {
			t.Fatalf("toOptions: %v", err)
		}
		if opt.DryRun {
			t.Error("DryRun should default false")
		}
		if opt.DelayBetweenDeletes != 10*time.Second {
			t.Errorf("delay = %v, want 10s", opt.DelayBetweenDeletes)
		}
		if !opt.BeforeDate.IsZero() {
			t.Errorf("BeforeDate should be zero, got %v", opt.BeforeDate)
		}
	})

	t.Run("before-date parses", func(t *testing.T) {
		f := commonFlags{BeforeDate: "2026-01-15"}
		opt, err := f.toOptions()
		if err != nil {
			t.Fatalf("toOptions: %v", err)
		}
		if opt.BeforeDate.Year() != 2026 || opt.BeforeDate.Month() != 1 || opt.BeforeDate.Day() != 15 {
			t.Errorf("BeforeDate = %v, want 2026-01-15", opt.BeforeDate)
		}
	})

	t.Run("before-date rejects bad format", func(t *testing.T) {
		f := commonFlags{BeforeDate: "01/15/2026"}
		_, err := f.toOptions()
		if err == nil {
			t.Fatal("expected error for non-ISO date")
		}
		if !strings.Contains(err.Error(), "YYYY-MM-DD") {
			t.Errorf("error should mention expected format: %v", err)
		}
	})
}

func TestNewCleanupCmdSubcommands(t *testing.T) {
	cmd := newCleanupCmd()
	want := map[string]bool{
		"releases":        false,
		"packages":        false,
		"repos":           false,
		"sonar-projects":  false,
	}
	for _, sub := range cmd.Commands() {
		// Cobra's Name() strips the args portion of Use.
		name := sub.Name()
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for verb, found := range want {
		if !found {
			t.Errorf("cleanup is missing subcommand %q", verb)
		}
	}
}

func TestCleanupReposRejectsMissingSelector(t *testing.T) {
	// Run the RunE directly to assert the validation message without spinning
	// up the network. cobra would otherwise call it from main.
	cmd := newCleanupReposCmd()
	cmd.SetArgs([]string{"valentinajemuovic"}) // no --prefix, no names
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when neither --prefix nor names supplied")
	}
	if !strings.Contains(err.Error(), "--prefix") {
		t.Errorf("error should mention --prefix: %v", err)
	}
}

func TestCleanupSonarProjectsRejectsMissingSelector(t *testing.T) {
	cmd := newCleanupSonarProjectsCmd()
	cmd.SetArgs([]string{"myorg"})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	t.Setenv("SONAR_TOKEN", "dummy") // ensure we hit the selector check, not the token check
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when neither --prefix nor keys supplied")
	}
	if !strings.Contains(err.Error(), "--prefix") {
		t.Errorf("error should mention --prefix: %v", err)
	}
}

func TestCleanupSonarProjectsRejectsMissingToken(t *testing.T) {
	cmd := newCleanupSonarProjectsCmd()
	cmd.SetArgs([]string{"myorg", "myorg_some-project"})
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	t.Setenv("SONAR_TOKEN", "")
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when SONAR_TOKEN is unset")
	}
	if !strings.Contains(err.Error(), "SONAR_TOKEN") {
		t.Errorf("error should mention SONAR_TOKEN: %v", err)
	}
}
