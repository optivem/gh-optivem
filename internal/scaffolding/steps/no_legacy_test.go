package steps

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
)

// --no-legacy must strip every legacy artifact from the scaffolded output;
// without it the legacy artifacts are scaffolded exactly as before.

func TestAddLegacyWorkflow_NoLegacy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		noLegacy bool
		want     bool
	}{
		{"legacy kept", false, true},
		{"--no-legacy", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wfMap := map[string]string{}
			cfg := &config.Config{NoLegacy: tc.noLegacy}
			addLegacyWorkflow(wfMap, Names.MonolithLegacyAcceptWf, map[string]string{"testLang": "java"}, cfg)
			dst, got := wfMap["monolith-java-acceptance-stage-docker-compose-legacy.yml"]
			if got != tc.want {
				t.Errorf("legacy workflow present = %v, want %v (map %v)", got, tc.want, wfMap)
			}
			if got && dst != "acceptance-stage-docker-compose-legacy.yml" {
				t.Errorf("legacy workflow dest = %q", dst)
			}
		})
	}
}

func TestPipelineBadges_NoLegacy(t *testing.T) {
	t.Parallel()
	hasLegacy := func(cfg *config.Config) bool {
		for _, b := range pipelineBadges("base", cfg) {
			if b[1] == acceptanceStageLegacy {
				return true
			}
		}
		return false
	}
	if !hasLegacy(&config.Config{}) {
		t.Error("without --no-legacy: legacy badge missing")
	}
	if hasLegacy(&config.Config{NoLegacy: true}) {
		t.Error("with --no-legacy: legacy badge present")
	}
}

func TestRemoveLegacyTests(t *testing.T) {
	t.Parallel()
	for testLang, paths := range legacyTestPaths {
		t.Run(testLang, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			for _, rel := range paths {
				p := filepath.Join(dir, rel)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			latest := filepath.Join(dir, "tests.yaml")
			if err := os.WriteFile(latest, []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
			removeLegacyTests(dir, testLang)
			for _, rel := range paths {
				if _, err := os.Stat(filepath.Join(dir, rel)); !os.IsNotExist(err) {
					t.Errorf("%s still present after removeLegacyTests", rel)
				}
			}
			if _, err := os.Stat(latest); err != nil {
				t.Errorf("tests.yaml removed: %v", err)
			}
			// Absence is tolerated: a second pass must not fail.
			removeLegacyTests(dir, testLang)
		})
	}
}

func TestWriteOptivemYAML_NoLegacy(t *testing.T) {
	t.Parallel()
	for _, noLegacy := range []bool{false, true} {
		dir := t.TempDir()
		cfg := &config.Config{
			Arch:           "monolith",
			RepoStrategy:   "monorepo",
			Owner:          "x",
			Repo:           "myrepo",
			FullRepo:       "x/myrepo",
			Lang:           "java",
			TestLang:       "java",
			ProjectURL:     "https://github.com/orgs/x/projects/1",
			SystemPath:     "system",
			SystemTestPath: "system-test",
			RepoDir:        dir,
			NoLegacy:       noLegacy,
		}
		WriteOptivemYAML(cfg)
		if _, err := os.Stat(filepath.Join(dir, "gh-optivem.yaml")); err != nil {
			t.Errorf("noLegacy=%v: gh-optivem.yaml missing: %v", noLegacy, err)
		}
		_, err := os.Stat(filepath.Join(dir, scaffoldedConfigFilenameLegacy))
		if present := err == nil; present == noLegacy {
			t.Errorf("noLegacy=%v: gh-optivem.legacy.yaml present = %v", noLegacy, present)
		}
	}
}
