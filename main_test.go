package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// TestLoadProjectConfigForInit_DefaultPathFlagsMissingFileBuildsInMemory
// pins the central guarantee of the YAML-single-write fix: when init is
// invoked with the default path (no --config / GH_OPTIVEM_CONFIG), no
// pre-existing CWD copy, and YAML-affecting flags supplied, the config
// is built in memory and sourcePath is empty — no file appears in the
// CWD. Downstream, steps.WriteOptivemYAML is then the sole on-disk writer.
//
// Regression guard: before this change, configinit.Run would silently
// write `<CWD>/gh-optivem.yaml`, orphaning the file outside the scaffold
// dir on every run (and visibly on failure runs where Cleanup keeps the
// scaffold dir but never touches the CWD copy).
func TestLoadProjectConfigForInit_DefaultPathFlagsMissingFileBuildsInMemory(t *testing.T) {
	stubExistenceChecks(t)

	dir := t.TempDir()
	chdirForTest(t, dir)
	t.Setenv(projectconfig.EnvVar, "")

	f := &config.RawFlags{
		Owner:        "acme",
		Repo:         "page-turner",
		SystemName:   "Page Turner",
		Arch:         "monolith",
		RepoStrategy: "monorepo",
		Lang:         "java",
		TestLang:     "java",
		ProjectURL:   "https://github.com/orgs/acme/projects/1",
		License:      projectconfig.LicenseMIT,
		Deploy:       projectconfig.DeployDocker,
	}

	pc, sourcePath, err := loadProjectConfigForInit("", f)
	if err != nil {
		t.Fatalf("loadProjectConfigForInit: %v", err)
	}
	if pc == nil {
		t.Fatal("want non-nil pc")
	}
	if sourcePath != "" {
		t.Errorf("sourcePath: want empty (in-memory, default-path), got %q", sourcePath)
	}
	if _, statErr := os.Stat(filepath.Join(dir, projectconfig.Path)); !os.IsNotExist(statErr) {
		t.Errorf("gh-optivem.yaml must not be written to CWD on default-path init; stat err: %v", statErr)
	}
}

// TestLoadProjectConfigForInit_DefaultPathPreExistingFileLoaded confirms
// the pre-existing-file branch still loads from disk and reports the
// absolute path as sourcePath. Operator-authored CWD files are respected
// — the fix is "don't create the orphan", not "delete pre-existing files".
func TestLoadProjectConfigForInit_DefaultPathPreExistingFileLoaded(t *testing.T) {
	stubExistenceChecks(t)

	dir := t.TempDir()
	chdirForTest(t, dir)
	t.Setenv(projectconfig.EnvVar, "")

	path := filepath.Join(dir, projectconfig.Path)
	seed := &projectconfig.Config{
		Project: projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/acme/projects/9"},
	}
	if err := projectconfig.WriteToPath(path, seed); err != nil {
		t.Fatalf("seed: %v", err)
	}

	pc, sourcePath, err := loadProjectConfigForInit("", &config.RawFlags{})
	if err != nil {
		t.Fatalf("loadProjectConfigForInit: %v", err)
	}
	if pc == nil || pc.Project.URL != seed.Project.URL {
		t.Errorf("loaded config mismatch: %+v", pc)
	}
	abs, _ := filepath.Abs(path)
	if sourcePath != abs {
		t.Errorf("sourcePath: got %q, want %q", sourcePath, abs)
	}
}

// stubExistenceChecks swaps the package-level GitHub existence probes
// for no-ops so the test stays offline. Restored on t.Cleanup.
func stubExistenceChecks(t *testing.T) {
	t.Helper()
	prevOwner := config.CheckOwnerExistsFn
	prevProject := config.CheckProjectExistsFn
	config.CheckOwnerExistsFn = func(string) error { return nil }
	config.CheckProjectExistsFn = func(string) error { return nil }
	t.Cleanup(func() {
		config.CheckOwnerExistsFn = prevOwner
		config.CheckProjectExistsFn = prevProject
	})
}

// chdirForTest changes the working directory for the duration of the
// test and restores it afterward. Required for tests that exercise
// projectconfig.ResolvePath's default-path branch (which reads os.Getwd).
func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})
}
