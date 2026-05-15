package preflight

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// makeFakeRepo creates dir as a fake git repo (empty `.git` directory)
// and returns the absolute path. Used to fabricate a workspace per test.
func makeFakeRepo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("makeFakeRepo: %v", err)
	}
	return dir
}

// makeDir creates dir and any parents.
func makeDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("makeDir: %v", err)
	}
}

func TestRun_NilCfgIsOK(t *testing.T) {
	t.Parallel()
	if err := Run(context.Background(), nil, Options{}); err != nil {
		t.Errorf("nil cfg should pass preflight, got: %v", err)
	}
}

func TestRun_MonoRepoMonolithAllPresent(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, filepath.Join(t.TempDir(), "shop"))
	makeDir(t, filepath.Join(root, "system", "monolith", "java"))
	makeDir(t, filepath.Join(root, "system-test", "java"))

	cfg := &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         "system/monolith/java",
			Repo:         "optivem/shop",
			Lang:         projectconfig.LangJava,
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test/java", Repo: "optivem/shop", Lang: projectconfig.LangJava,
		},
	}
	if err := Run(context.Background(), cfg, Options{Cwd: root}); err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

func TestRun_MissingSystemPath(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, filepath.Join(t.TempDir(), "shop"))
	// system/monolith/java intentionally NOT created.
	makeDir(t, filepath.Join(root, "system-test", "java"))

	cfg := &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         "system/monolith/java",
			Repo:         "optivem/shop",
			Lang:         projectconfig.LangJava,
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test/java", Repo: "optivem/shop", Lang: projectconfig.LangJava,
		},
	}
	err := Run(context.Background(), cfg, Options{Cwd: root})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "system.path") {
		t.Errorf("error should mention system.path, got: %v", err)
	}
}

func TestRun_MissingSystemTestPath(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, filepath.Join(t.TempDir(), "shop"))
	makeDir(t, filepath.Join(root, "system", "monolith", "java"))
	// system-test/java not created.

	cfg := &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         "system/monolith/java",
			Repo:         "optivem/shop",
			Lang:         projectconfig.LangJava,
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test/java", Repo: "optivem/shop", Lang: projectconfig.LangJava,
		},
	}
	err := Run(context.Background(), cfg, Options{Cwd: root})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "system_test.path") {
		t.Errorf("error should mention system_test.path, got: %v", err)
	}
}

func TestRun_MultitierMissingFrontend(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, filepath.Join(t.TempDir(), "shop"))
	makeDir(t, filepath.Join(root, "system", "multitier", "backend-java"))
	makeDir(t, filepath.Join(root, "system-test", "java"))
	// frontend-react not created.

	cfg := &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMultitier,
			Backend: projectconfig.TierSpec{
				Path: "system/multitier/backend-java", Repo: "optivem/shop", Lang: projectconfig.LangJava,
			},
			Frontend: projectconfig.TierSpec{
				Path: "system/multitier/frontend-react", Repo: "optivem/shop", Lang: projectconfig.LangTypescript,
			},
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test/java", Repo: "optivem/shop", Lang: projectconfig.LangJava,
		},
	}
	err := Run(context.Background(), cfg, Options{Cwd: root})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "system.frontend.path") {
		t.Errorf("error should mention system.frontend.path, got: %v", err)
	}
}

func TestRun_MultiRepoSingleRepoNotCloned(t *testing.T) {
	wsRoot := t.TempDir()
	// optivem/shop NOT cloned anywhere under the workspace.

	cfg := &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMultiRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         ".",
			Repo:         "optivem/shop",
			Lang:         projectconfig.LangJava,
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test", Repo: "optivem/shop", Lang: projectconfig.LangJava,
		},
	}
	err := Run(context.Background(), cfg, Options{Workspace: wsRoot, Cwd: t.TempDir()})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "optivem/shop") {
		t.Errorf("error should mention slug, got: %v", err)
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("error should report missing clone, got: %v", err)
	}
}

func TestRun_MultiRepoNotAGitRepo(t *testing.T) {
	wsRoot := t.TempDir()

	// Create the directory but no .git.
	beDir := filepath.Join(wsRoot, "shop-backend")
	makeDir(t, filepath.Join(beDir, "."))
	feDir := makeFakeRepo(t, filepath.Join(wsRoot, "shop-frontend"))
	mainDir := makeFakeRepo(t, filepath.Join(wsRoot, "shop-main"))
	makeDir(t, filepath.Join(mainDir, "system-test"))
	makeDir(t, filepath.Join(feDir, "."))

	cfg := &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMultiRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMultitier,
			Backend: projectconfig.TierSpec{
				Path: ".", Repo: "optivem/shop-backend", Lang: projectconfig.LangJava,
			},
			Frontend: projectconfig.TierSpec{
				Path: ".", Repo: "optivem/shop-frontend", Lang: projectconfig.LangTypescript,
			},
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test", Repo: "optivem/shop-main", Lang: projectconfig.LangJava,
		},
	}
	err := Run(context.Background(), cfg, Options{Workspace: wsRoot, Cwd: t.TempDir()})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "is not a git repository") {
		t.Errorf("error should call out missing .git, got: %v", err)
	}
	if !strings.Contains(err.Error(), "optivem/shop-backend") {
		t.Errorf("error should name the bad repo, got: %v", err)
	}
}

func TestRun_MultiRepoMultitierAllPresent(t *testing.T) {
	wsRoot := t.TempDir()

	makeFakeRepo(t, filepath.Join(wsRoot, "shop-backend"))
	makeFakeRepo(t, filepath.Join(wsRoot, "shop-frontend"))
	mainDir := makeFakeRepo(t, filepath.Join(wsRoot, "shop-main"))
	makeDir(t, filepath.Join(mainDir, "system-test"))

	cfg := &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMultiRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMultitier,
			Backend: projectconfig.TierSpec{
				Path: ".", Repo: "optivem/shop-backend", Lang: projectconfig.LangJava,
			},
			Frontend: projectconfig.TierSpec{
				Path: ".", Repo: "optivem/shop-frontend", Lang: projectconfig.LangTypescript,
			},
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test", Repo: "optivem/shop-main", Lang: projectconfig.LangJava,
		},
	}
	if err := Run(context.Background(), cfg, Options{Workspace: wsRoot, Cwd: t.TempDir()}); err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

func TestRun_TierPathExistsUnderWrongRepo(t *testing.T) {
	wsRoot := t.TempDir()

	// Set up two repos. Put a "system-test" dir under the FRONTEND repo
	// instead of the main repo where system_test claims to live.
	makeFakeRepo(t, filepath.Join(wsRoot, "shop-backend"))
	feDir := makeFakeRepo(t, filepath.Join(wsRoot, "shop-frontend"))
	mainDir := makeFakeRepo(t, filepath.Join(wsRoot, "shop-main"))
	makeDir(t, filepath.Join(feDir, "system-test"))
	// mainDir does NOT have system-test.
	_ = mainDir

	cfg := &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMultiRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMultitier,
			Backend: projectconfig.TierSpec{
				Path: ".", Repo: "optivem/shop-backend", Lang: projectconfig.LangJava,
			},
			Frontend: projectconfig.TierSpec{
				Path: ".", Repo: "optivem/shop-frontend", Lang: projectconfig.LangTypescript,
			},
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test", Repo: "optivem/shop-main", Lang: projectconfig.LangJava,
		},
	}
	err := Run(context.Background(), cfg, Options{Workspace: wsRoot, Cwd: t.TempDir()})
	if err == nil {
		t.Fatal("expected error — system-test exists under frontend repo, not main")
	}
	if !strings.Contains(err.Error(), "system_test.path") {
		t.Errorf("error should mention system_test.path, got: %v", err)
	}
}

func TestRun_ExternalSystemsDeclaredAndPresent(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, filepath.Join(t.TempDir(), "shop"))
	makeDir(t, filepath.Join(root, "system", "monolith", "java"))
	makeDir(t, filepath.Join(root, "system-test", "java"))
	makeDir(t, filepath.Join(root, "stubs"))
	makeDir(t, filepath.Join(root, "simulators"))

	cfg := &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         "system/monolith/java",
			Repo:         "optivem/shop",
			Lang:         projectconfig.LangJava,
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test/java", Repo: "optivem/shop", Lang: projectconfig.LangJava,
		},
		ExternalSystems: projectconfig.ExternalSystems{
			Stubs:      projectconfig.ExternalSpec{Path: "stubs", Repo: "optivem/shop"},
			Simulators: projectconfig.ExternalSpec{Path: "simulators", Repo: "optivem/shop"},
		},
	}
	if err := Run(context.Background(), cfg, Options{Cwd: root}); err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

func TestRun_ExternalSystemsMissingPath(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, filepath.Join(t.TempDir(), "shop"))
	makeDir(t, filepath.Join(root, "system", "monolith", "java"))
	makeDir(t, filepath.Join(root, "system-test", "java"))
	// stubs created but simulators NOT.
	makeDir(t, filepath.Join(root, "stubs"))

	cfg := &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         "system/monolith/java",
			Repo:         "optivem/shop",
			Lang:         projectconfig.LangJava,
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test/java", Repo: "optivem/shop", Lang: projectconfig.LangJava,
		},
		ExternalSystems: projectconfig.ExternalSystems{
			Stubs:      projectconfig.ExternalSpec{Path: "stubs", Repo: "optivem/shop"},
			Simulators: projectconfig.ExternalSpec{Path: "simulators", Repo: "optivem/shop"},
		},
	}
	err := Run(context.Background(), cfg, Options{Cwd: root})
	if err == nil {
		t.Fatal("expected error for missing simulators path")
	}
	if !strings.Contains(err.Error(), "external_systems.simulators.path") {
		t.Errorf("error should mention simulators.path, got: %v", err)
	}
}

func TestRun_ExternalSystemsOmittedDoesNotFail(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, filepath.Join(t.TempDir(), "shop"))
	makeDir(t, filepath.Join(root, "system", "monolith", "java"))
	makeDir(t, filepath.Join(root, "system-test", "java"))

	cfg := &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         "system/monolith/java",
			Repo:         "optivem/shop",
			Lang:         projectconfig.LangJava,
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test/java", Repo: "optivem/shop", Lang: projectconfig.LangJava,
		},
		// ExternalSystems omitted entirely.
	}
	if err := Run(context.Background(), cfg, Options{Cwd: root}); err != nil {
		t.Errorf("expected nil with no external_systems, got: %v", err)
	}
}

// monolithCfg returns a populated *projectconfig.Config with one repo +
// the four mandatory tier paths, sonar identities filled in, and a
// project.url set. Used by remote-check tests below — each constructs
// its own fake workspace and overrides the checker fields it cares
// about while the rest of the layout stays uniform.
func monolithCfg() *projectconfig.Config {
	return &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		Project:      projectconfig.Project{Provider: projectconfig.ProviderGitHub, URL: "https://github.com/orgs/acme/projects/1"},
		Sonar:        projectconfig.Sonar{Organization: "acme"},
		System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         "system/monolith/java",
			Repo:         "acme/page-turner",
			Lang:         projectconfig.LangJava,
			SonarProject: "acme_page-turner_system",
		},
		SystemTest: projectconfig.TierSpec{
			Path:         "system-test/java",
			Repo:         "acme/page-turner",
			Lang:         projectconfig.LangJava,
			SonarProject: "acme_page-turner_test",
		},
	}
}

// seedMonolithFS mirrors monolithCfg on disk: creates a fake clone at
// <workspace>/page-turner with .git/, system/monolith/java, and
// system-test/java populated so the local-FS pass is a clean pass.
// Returns repoRoot so the test can pass it as Options.Cwd.
func seedMonolithFS(t *testing.T, workspace string) string {
	t.Helper()
	root := makeFakeRepo(t, filepath.Join(workspace, "page-turner"))
	makeDir(t, filepath.Join(root, "system", "monolith", "java"))
	makeDir(t, filepath.Join(root, "system-test", "java"))
	return root
}

func TestRun_RepoExistsFalse_FailsWithSlug(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	cwd := seedMonolithFS(t, ws)
	cfg := monolithCfg()
	opts := Options{
		Workspace: ws,
		Cwd:       cwd,
		RepoExists: func(_ context.Context, _ string) (bool, error) {
			return false, nil
		},
	}
	err := Run(context.Background(), cfg, opts)
	if err == nil {
		t.Fatal("want failure when RepoExists returns false, got nil")
	}
	if !strings.Contains(err.Error(), "acme/page-turner") {
		t.Errorf("error should name the missing slug, got: %v", err)
	}
	if !strings.Contains(err.Error(), "does not exist on GitHub") {
		t.Errorf("error should call out GitHub, got: %v", err)
	}
}

func TestRun_SonarOrgMissing_SkipsPerProjectChecks(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	cwd := seedMonolithFS(t, ws)
	cfg := monolithCfg()
	projectCalls := 0
	opts := Options{
		Workspace: ws,
		Cwd:       cwd,
		SonarOrgExists: func(_ context.Context, _ string) (bool, error) {
			return false, nil
		},
		SonarProjectExists: func(_ context.Context, _ string) (bool, error) {
			projectCalls++
			return true, nil
		},
	}
	err := Run(context.Background(), cfg, opts)
	if err == nil {
		t.Fatal("want failure when sonar org missing, got nil")
	}
	if !strings.Contains(err.Error(), "sonar.organization") {
		t.Errorf("error should mention sonar.organization, got: %v", err)
	}
	if projectCalls != 0 {
		t.Errorf("per-project checks should not run when org is missing; got %d call(s)", projectCalls)
	}
}

func TestRun_SonarProjectMissing_NamesField(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	cwd := seedMonolithFS(t, ws)
	cfg := monolithCfg()
	opts := Options{
		Workspace: ws,
		Cwd:       cwd,
		SonarOrgExists: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
		SonarProjectExists: func(_ context.Context, key string) (bool, error) {
			// system_test project missing, others present.
			return key != "acme_page-turner_test", nil
		},
	}
	err := Run(context.Background(), cfg, opts)
	if err == nil {
		t.Fatal("want failure when a sonar project is missing, got nil")
	}
	if !strings.Contains(err.Error(), "system_test.sonar_project") {
		t.Errorf("error should name the missing project field, got: %v", err)
	}
	if !strings.Contains(err.Error(), "acme_page-turner_test") {
		t.Errorf("error should include the missing key, got: %v", err)
	}
}

func TestRun_BoardURLOK_ErrorIsSurfaced(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	cwd := seedMonolithFS(t, ws)
	cfg := monolithCfg()
	opts := Options{
		Workspace: ws,
		Cwd:       cwd,
		BoardURLOK: func(_ context.Context, url string) error {
			return fmt.Errorf("project view: HTTP 404 (project not found)")
		},
	}
	err := Run(context.Background(), cfg, opts)
	if err == nil {
		t.Fatal("want failure when BoardURLOK returns error, got nil")
	}
	if !strings.Contains(err.Error(), "project.url") {
		t.Errorf("error should mention project.url, got: %v", err)
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Errorf("error should propagate the underlying message, got: %v", err)
	}
}

func TestRun_BoardURLOK_SkippedWhenURLEmpty(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	cwd := seedMonolithFS(t, ws)
	cfg := monolithCfg()
	cfg.Project.URL = ""
	called := false
	opts := Options{
		Workspace: ws,
		Cwd:       cwd,
		BoardURLOK: func(_ context.Context, _ string) error {
			called = true
			return nil
		},
	}
	if err := Run(context.Background(), cfg, opts); err != nil {
		t.Errorf("expected nil with empty project.url, got: %v", err)
	}
	if called {
		t.Error("BoardURLOK should not be invoked when project.url is empty")
	}
}

func TestRun_AllRemoteChecksPass(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	cwd := seedMonolithFS(t, ws)
	cfg := monolithCfg()
	opts := Options{
		Workspace: ws,
		Cwd:       cwd,
		RepoExists: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
		SonarOrgExists: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
		SonarProjectExists: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
		BoardURLOK: func(_ context.Context, _ string) error {
			return nil
		},
	}
	if err := Run(context.Background(), cfg, opts); err != nil {
		t.Errorf("expected nil with every remote check returning OK, got: %v", err)
	}
}
