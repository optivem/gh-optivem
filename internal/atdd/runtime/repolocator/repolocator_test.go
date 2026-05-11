package repolocator

import (
	"path/filepath"
	"testing"

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

func newMonoRepoConfig() *projectconfig.Config {
	return &projectconfig.Config{
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
}

func newMultiRepoConfig() *projectconfig.Config {
	return &projectconfig.Config{
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
}

// TestResolve_MonoRepoDefaultsToParentOfCWD verifies the mono-repo case
// resolves to <parent(cwd)>/<repo-name> — which equals CWD when CWD is
// <workspace>/<repo-name>.
func TestResolve_MonoRepoDefaultsToParentOfCWD(t *testing.T) {
	cwd := filepath.Join("/", "tmp", "workspace", "shop")

	got, err := Resolve(newMonoRepoConfig(), "", cwd)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Local) != 1 {
		t.Fatalf("expected 1 entry, got %v", got.Local)
	}
	want := filepath.Join(filepath.Dir(cwd), "shop")
	if got.Local["optivem/shop"] != want {
		t.Errorf("optivem/shop: got %q, want %q", got.Local["optivem/shop"], want)
	}
}

// TestResolve_MultiRepoDefaultsToParentOfCWD verifies the multi-repo
// case places every clone under parent(cwd), keyed by repo-name.
func TestResolve_MultiRepoDefaultsToParentOfCWD(t *testing.T) {
	cwd := filepath.Join("/", "tmp", "workspace", "calling-repo")
	parent := filepath.Dir(cwd)

	got, err := Resolve(newMultiRepoConfig(), "", cwd)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantBackend := filepath.Join(parent, "shop-backend")
	wantFrontend := filepath.Join(parent, "shop-frontend")
	wantMain := filepath.Join(parent, "shop-main")

	if got.Local["optivem/shop-backend"] != wantBackend {
		t.Errorf("backend: got %q, want %q", got.Local["optivem/shop-backend"], wantBackend)
	}
	if got.Local["optivem/shop-frontend"] != wantFrontend {
		t.Errorf("frontend: got %q, want %q", got.Local["optivem/shop-frontend"], wantFrontend)
	}
	if got.Local["optivem/shop-main"] != wantMain {
		t.Errorf("main: got %q, want %q", got.Local["optivem/shop-main"], wantMain)
	}
}

// TestResolve_WorkspaceOverridesDefault verifies an explicit --workspace
// argument beats the parent(cwd) default.
func TestResolve_WorkspaceOverridesDefault(t *testing.T) {
	ws := filepath.Join("/", "opt", "workspace")
	cwd := filepath.Join("/", "tmp", "anywhere")

	got, err := Resolve(newMultiRepoConfig(), ws, cwd)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Local["optivem/shop-backend"] != filepath.Join(ws, "shop-backend") {
		t.Errorf("backend should resolve under --workspace, got %q",
			got.Local["optivem/shop-backend"])
	}
	if got.Local["optivem/shop-frontend"] != filepath.Join(ws, "shop-frontend") {
		t.Errorf("frontend should resolve under --workspace, got %q",
			got.Local["optivem/shop-frontend"])
	}
}

func TestResolve_NilCfgReturnsEmpty(t *testing.T) {
	got, err := Resolve(nil, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Local) != 0 {
		t.Errorf("expected empty Local on nil cfg, got %v", got.Local)
	}
}

func TestResolve_NoReposReturnsEmpty(t *testing.T) {
	got, err := Resolve(&projectconfig.Config{}, "", "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Local) != 0 {
		t.Errorf("expected empty Local on no-repos config, got %v", got.Local)
	}
}
