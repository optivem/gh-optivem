package preflight

import (
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
	if err := Run(nil, nil, ""); err != nil {
		t.Errorf("nil cfg should pass preflight, got: %v", err)
	}
}

func TestRun_MonoRepoMonolithAllPresent(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, t.TempDir())
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
	if err := Run(cfg, nil, root); err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

func TestRun_MissingSystemPath(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, t.TempDir())
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
	err := Run(cfg, nil, root)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "system.path") {
		t.Errorf("error should mention system.path, got: %v", err)
	}
}

func TestRun_MissingSystemTestPath(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, t.TempDir())
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
	err := Run(cfg, nil, root)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "system_test.path") {
		t.Errorf("error should mention system_test.path, got: %v", err)
	}
}

func TestRun_MultitierMissingFrontend(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, t.TempDir())
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
	err := Run(cfg, nil, root)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "system.frontend.path") {
		t.Errorf("error should mention system.frontend.path, got: %v", err)
	}
}

func TestRun_MultiRepoSingleRepoNotCloned(t *testing.T) {
	wsRoot := t.TempDir()
	// optivem/shop NOT cloned anywhere — neither sibling nor under env var.
	t.Setenv("GH_OPTIVEM_WORKSPACE", wsRoot)

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
	err := Run(cfg, nil, t.TempDir())
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
	t.Setenv("GH_OPTIVEM_WORKSPACE", wsRoot)

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
	err := Run(cfg, nil, t.TempDir())
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
	t.Setenv("GH_OPTIVEM_WORKSPACE", wsRoot)

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
	if err := Run(cfg, nil, t.TempDir()); err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

func TestRun_TierPathExistsUnderWrongRepo(t *testing.T) {
	wsRoot := t.TempDir()
	t.Setenv("GH_OPTIVEM_WORKSPACE", wsRoot)

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
	err := Run(cfg, nil, t.TempDir())
	if err == nil {
		t.Fatal("expected error — system-test exists under frontend repo, not main")
	}
	if !strings.Contains(err.Error(), "system_test.path") {
		t.Errorf("error should mention system_test.path, got: %v", err)
	}
}

func TestRun_ExternalSystemsDeclaredAndPresent(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, t.TempDir())
	makeDir(t, filepath.Join(root, "system", "monolith", "java"))
	makeDir(t, filepath.Join(root, "system-test", "java"))
	makeDir(t, filepath.Join(root, "external-stub"))
	makeDir(t, filepath.Join(root, "external-real-sim"))

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
			Stubs:      projectconfig.ExternalSpec{Path: "external-stub", Repo: "optivem/shop"},
			Simulators: projectconfig.ExternalSpec{Path: "external-real-sim", Repo: "optivem/shop"},
		},
	}
	if err := Run(cfg, nil, root); err != nil {
		t.Errorf("expected nil, got: %v", err)
	}
}

func TestRun_ExternalSystemsMissingPath(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, t.TempDir())
	makeDir(t, filepath.Join(root, "system", "monolith", "java"))
	makeDir(t, filepath.Join(root, "system-test", "java"))
	// external-stub created but external-real-sim NOT.
	makeDir(t, filepath.Join(root, "external-stub"))

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
			Stubs:      projectconfig.ExternalSpec{Path: "external-stub", Repo: "optivem/shop"},
			Simulators: projectconfig.ExternalSpec{Path: "external-real-sim", Repo: "optivem/shop"},
		},
	}
	err := Run(cfg, nil, root)
	if err == nil {
		t.Fatal("expected error for missing simulators path")
	}
	if !strings.Contains(err.Error(), "external_systems.simulators.path") {
		t.Errorf("error should mention simulators.path, got: %v", err)
	}
}

func TestRun_ExternalSystemsOmittedDoesNotFail(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, t.TempDir())
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
	if err := Run(cfg, nil, root); err != nil {
		t.Errorf("expected nil with no external_systems, got: %v", err)
	}
}
