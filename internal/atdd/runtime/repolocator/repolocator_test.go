package repolocator

import (
	"path/filepath"
	"strings"
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

func TestResolve_MonoRepoSiblingConvention_UsesCWD(t *testing.T) {
	t.Setenv(EnvWorkspace, "")
	cwd := filepath.Join("/", "tmp", "shop")

	got, err := Resolve(newMonoRepoConfig(), nil, cwd)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Local) != 1 {
		t.Fatalf("expected 1 entry, got %v", got.Local)
	}
	if got.Local["optivem/shop"] != cwd {
		t.Errorf("optivem/shop: got %q, want %q", got.Local["optivem/shop"], cwd)
	}
}

func TestResolve_MultiRepoSiblingConvention(t *testing.T) {
	t.Setenv(EnvWorkspace, "")
	cwd := filepath.Join("/", "tmp", "workspace", "calling-repo")
	parent := filepath.Dir(cwd)

	got, err := Resolve(newMultiRepoConfig(), nil, cwd)
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

func TestResolve_EnvVarOverridesSiblingConvention(t *testing.T) {
	ws := filepath.Join("/", "opt", "workspace")
	t.Setenv(EnvWorkspace, ws)
	cwd := filepath.Join("/", "tmp", "anywhere")

	got, err := Resolve(newMultiRepoConfig(), nil, cwd)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Local["optivem/shop-backend"] != filepath.Join(ws, "shop-backend") {
		t.Errorf("backend should resolve under env var, got %q",
			got.Local["optivem/shop-backend"])
	}
	if got.Local["optivem/shop-frontend"] != filepath.Join(ws, "shop-frontend") {
		t.Errorf("frontend should resolve under env var, got %q",
			got.Local["optivem/shop-frontend"])
	}
}

func TestResolve_RepoDirFlagWinsOverEnvAndConvention(t *testing.T) {
	t.Setenv(EnvWorkspace, filepath.Join("/", "opt", "workspace"))
	cwd := filepath.Join("/", "tmp", "calling-repo")

	flagPath := filepath.Join("/", "custom", "backend-clone")
	repoDirs := map[string]string{"optivem/shop-backend": flagPath}

	got, err := Resolve(newMultiRepoConfig(), repoDirs, cwd)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Local["optivem/shop-backend"] != flagPath {
		t.Errorf("backend should use --repo-dir, got %q", got.Local["optivem/shop-backend"])
	}
	// The other slugs fall through to env-var.
	if !strings.HasPrefix(got.Local["optivem/shop-frontend"], filepath.Join("/", "opt", "workspace")) {
		t.Errorf("frontend should fall through to env-var, got %q",
			got.Local["optivem/shop-frontend"])
	}
}

func TestResolve_NilCfgReturnsEmpty(t *testing.T) {
	got, err := Resolve(nil, nil, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Local) != 0 {
		t.Errorf("expected empty Local on nil cfg, got %v", got.Local)
	}
}

func TestResolve_NoReposReturnsEmpty(t *testing.T) {
	got, err := Resolve(&projectconfig.Config{}, nil, "")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if len(got.Local) != 0 {
		t.Errorf("expected empty Local on no-repos config, got %v", got.Local)
	}
}

func TestParseRepoDirFlag_OK(t *testing.T) {
	got, err := ParseRepoDirFlag([]string{
		"optivem/shop=/abs/path/shop",
		"optivem/shop-backend=/abs/path/be",
	})
	if err != nil {
		t.Fatalf("ParseRepoDirFlag: %v", err)
	}
	if got["optivem/shop"] != "/abs/path/shop" {
		t.Errorf("shop: got %q", got["optivem/shop"])
	}
	if got["optivem/shop-backend"] != "/abs/path/be" {
		t.Errorf("be: got %q", got["optivem/shop-backend"])
	}
}

func TestParseRepoDirFlag_RejectsMalformed(t *testing.T) {
	cases := []string{
		"slug-only-no-equals",
		"=/path/no/slug",
		"slug=",
		"",
	}
	for _, c := range cases {
		if _, err := ParseRepoDirFlag([]string{c}); err == nil {
			t.Errorf("ParseRepoDirFlag(%q): expected error, got nil", c)
		}
	}
}

func TestParseRepoDirFlag_EmptyInputYieldsEmptyMap(t *testing.T) {
	got, err := ParseRepoDirFlag(nil)
	if err != nil {
		t.Fatalf("ParseRepoDirFlag: %v", err)
	}
	if got == nil {
		t.Error("expected non-nil empty map (not nil)")
	}
	if len(got) != 0 {
		t.Errorf("expected empty map, got %v", got)
	}
}
