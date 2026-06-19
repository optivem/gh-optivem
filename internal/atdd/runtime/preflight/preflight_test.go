package preflight

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/process"
	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
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

// TestRun_MissingEnvVarsAggregate pins that opts.MissingEnvVars folds every
// missing credential into the one aggregated failure block — and that it
// runs even on a nil cfg, since credential presence is independent of
// project layout. This is the "fix everything in one shell restart" contract.
func TestRun_MissingEnvVarsAggregate(t *testing.T) {
	t.Parallel()
	opts := Options{
		MissingEnvVars: func() []string { return []string{"SONAR_TOKEN", "GHCR_TOKEN"} },
	}
	err := Run(context.Background(), nil, opts)
	if err == nil {
		t.Fatal("want error listing missing env vars, got nil")
	}
	for _, name := range []string{"SONAR_TOKEN", "GHCR_TOKEN"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("aggregated error should name %s, got:\n%s", name, err)
		}
	}
}

// TestRun_NoMissingEnvVarsIsOK confirms an empty missing-list is a no-op:
// a fully-provisioned environment adds no failure lines.
func TestRun_NoMissingEnvVarsIsOK(t *testing.T) {
	t.Parallel()
	opts := Options{MissingEnvVars: func() []string { return nil }}
	if err := Run(context.Background(), nil, opts); err != nil {
		t.Errorf("no missing env vars should pass, got: %v", err)
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
	if !strings.Contains(err.Error(), "system-test.path") {
		t.Errorf("error should mention system-test.path, got: %v", err)
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
	// instead of the main repo where system-test claims to live.
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
	if !strings.Contains(err.Error(), "system-test.path") {
		t.Errorf("error should mention system-test.path, got: %v", err)
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
			"warehouse": {
				RealKind:  projectconfig.RealKindSimulator,
				Stub:      projectconfig.ExternalSpec{Path: "stubs", Repo: "optivem/shop"},
				Simulator: projectconfig.ExternalSpec{Path: "simulators", Repo: "optivem/shop"},
			},
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
			"warehouse": {
				RealKind:  projectconfig.RealKindSimulator,
				Stub:      projectconfig.ExternalSpec{Path: "stubs", Repo: "optivem/shop"},
				Simulator: projectconfig.ExternalSpec{Path: "simulators", Repo: "optivem/shop"},
			},
		},
	}
	err := Run(context.Background(), cfg, Options{Cwd: root})
	if err == nil {
		t.Fatal("expected error for missing simulator path")
	}
	if !strings.Contains(err.Error(), "external-systems.warehouse.simulator.path") {
		t.Errorf("error should mention the warehouse simulator path, got: %v", err)
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
		t.Errorf("expected nil with no external-systems, got: %v", err)
	}
}

// canonicalTSPaths returns a fully-populated `system-test.paths:` block in
// canonical order, mirroring the shape `DefaultPaths("typescript", "system-test/typescript", "")`
// would emit for a flat (no-SUT-leaf) TS layout. Used by the
// SystemTestPaths tests below.
func canonicalTSPaths() map[string]string {
	return map[string]string{
		"system-driver-port":             "system-test/typescript/src/testkit/driver/port",
		"system-driver-adapter":          "system-test/typescript/src/testkit/driver/adapter",
		"external-system-driver-port":    "system-test/typescript/src/testkit/external/port",
		"external-system-driver-adapter": "system-test/typescript/src/testkit/external/adapter",
		"at-test":                        "system-test/typescript/tests/latest/acceptance",
		"dsl-port":                       "system-test/typescript/src/testkit/dsl/port",
		"dsl-core":                       "system-test/typescript/src/testkit/dsl/core",
		"ct-test":                        "system-test/typescript/tests/latest/contract",
	}
}

func TestRun_SystemTestPathsAllPresent(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, filepath.Join(t.TempDir(), "shop"))
	makeDir(t, filepath.Join(root, "system", "monolith", "typescript"))
	paths := canonicalTSPaths()
	for _, p := range paths {
		makeDir(t, filepath.Join(root, p))
	}

	cfg := &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         "system/monolith/typescript",
			Repo:         "optivem/shop",
			Lang:         projectconfig.LangTypescript,
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test/typescript", Repo: "optivem/shop", Lang: projectconfig.LangTypescript,
			Paths: paths,
		},
	}
	if err := Run(context.Background(), cfg, Options{Cwd: root}); err != nil {
		t.Errorf("expected nil with all paths present, got: %v", err)
	}
}

// TestRun_SystemTestPathsMissingEntries is the regression test for the gap
// the 2026-05-26 shop-template review surfaced: a `*-typescript-legacy.yaml`
// that named four phantom `myShop`-suffixed testkit dirs and still passed
// preflight because preflight only stat'd the top-level `system-test.path:`
// parent dir, never the inner `paths:` map.
func TestRun_SystemTestPathsMissingEntries(t *testing.T) {
	t.Parallel()
	root := makeFakeRepo(t, filepath.Join(t.TempDir(), "shop"))
	makeDir(t, filepath.Join(root, "system", "monolith", "typescript"))
	// Materialise only the four `external` / `tests/latest/*` entries that
	// match the shop template's actual TS layout. Leave the four
	// `myShop`-suffixed entries (system-driver-port, system-driver-adapter, dsl-port,
	// dsl-core) phantom.
	paths := map[string]string{
		"system-driver-port":             "system-test/typescript/src/testkit/driver/port/myShop",
		"system-driver-adapter":          "system-test/typescript/src/testkit/driver/adapter/myShop",
		"external-system-driver-port":    "system-test/typescript/src/testkit/driver/port/external",
		"external-system-driver-adapter": "system-test/typescript/src/testkit/driver/adapter/external",
		"at-test":                        "system-test/typescript/tests/latest/acceptance",
		"dsl-port":                       "system-test/typescript/src/testkit/dsl/port/myShop",
		"dsl-core":                       "system-test/typescript/src/testkit/dsl/core/usecase/myShop",
		"ct-test":                        "system-test/typescript/tests/latest/contract",
	}
	for _, key := range []string{"external-system-driver-port", "external-system-driver-adapter", "at-test", "ct-test"} {
		makeDir(t, filepath.Join(root, paths[key]))
	}

	cfg := &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		System: projectconfig.System{
			Architecture: projectconfig.ArchMonolith,
			Path:         "system/monolith/typescript",
			Repo:         "optivem/shop",
			Lang:         projectconfig.LangTypescript,
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test/typescript", Repo: "optivem/shop", Lang: projectconfig.LangTypescript,
			Paths: paths,
		},
	}
	err := Run(context.Background(), cfg, Options{Cwd: root})
	if err == nil {
		t.Fatal("expected error — four myShop-suffixed paths are missing")
	}
	for _, key := range []string{"system-driver-port", "system-driver-adapter", "dsl-port", "dsl-core"} {
		want := "system-test.paths." + key
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %s, got: %v", want, err)
		}
	}
	// Sanity: the four present entries must NOT appear in the failure list.
	for _, key := range []string{"external-system-driver-port", "external-system-driver-adapter", "at-test", "ct-test"} {
		bad := "system-test.paths." + key
		if strings.Contains(err.Error(), bad) {
			t.Errorf("error should not mention %s (it exists on disk), got: %v", bad, err)
		}
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
			// system-test project missing, others present.
			return key != "acme_page-turner_test", nil
		},
	}
	err := Run(context.Background(), cfg, opts)
	if err == nil {
		t.Fatal("want failure when a sonar project is missing, got nil")
	}
	if !strings.Contains(err.Error(), "system-test.sonar-project") {
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

// TestRun_ScopeSweepCatchesBlankDbMigrationPath proves the layer-resolver
// sweep surfaces a missing system.db-migration-path at preflight time
// — the same condition that, pre-this-plan, only blew up 4+ minutes
// into a ticket run inside validate-outputs-and-scopes. Every other check
// passes; the Engine being wired and DbMigrationPath being blank is the
// only contributor to the failure.
func TestRun_ScopeSweepCatchesBlankDbMigrationPath(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	cwd := seedMonolithFS(t, ws)
	cfg := monolithCfg()
	// Architecture is set but DbMigrationPath is intentionally blank.
	// (Validate Rule 22b would normally reject this on full schema
	// validation, but preflight is structurally independent — and the
	// resolver sweep is the layer this test exercises.)
	cfg.System.DbMigrationPath = ""

	eng, err := process.Load()
	if err != nil {
		t.Fatalf("load state machine: %v", err)
	}

	opts := Options{
		Workspace: ws,
		Cwd:       cwd,
		Engine:    eng,
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
	err = Run(context.Background(), cfg, opts)
	if err == nil {
		t.Fatal("expected scope-sweep failure for blank system.db-migration-path, got nil")
	}
	if !strings.Contains(err.Error(), "system.db-migration-path") {
		t.Errorf("error should name system.db-migration-path, got: %v", err)
	}
	if !strings.Contains(err.Error(), "implement-system") {
		t.Errorf("error should name the implement-system MID (process-flow.yaml's implement-system write list references system-db-migration-path), got: %v", err)
	}
}

// TestRun_ScopeSweepSkippedWhenEngineNil pins the function-level nil-Engine
// contract: with no Engine wired, the engine-derived sweeps are silent and
// Run returns nil even on a cfg that would otherwise fail the scope sweep
// (blank DbMigrationPath). Production callers (`implement`, `config
// preflight`) now both wire the engine via defaultPreflightOptions, so this
// guards the API contract that other callers can opt out, not a specific
// command's behaviour.
func TestRun_ScopeSweepSkippedWhenEngineNil(t *testing.T) {
	t.Parallel()
	ws := t.TempDir()
	cwd := seedMonolithFS(t, ws)
	cfg := monolithCfg()
	cfg.System.DbMigrationPath = ""

	opts := Options{
		Workspace: ws,
		Cwd:       cwd,
		// Engine deliberately not set.
		RepoExists:         func(_ context.Context, _ string) (bool, error) { return true, nil },
		SonarOrgExists:     func(_ context.Context, _ string) (bool, error) { return true, nil },
		SonarProjectExists: func(_ context.Context, _ string) (bool, error) { return true, nil },
		BoardURLOK:         func(_ context.Context, _ string) error { return nil },
	}
	if err := Run(context.Background(), cfg, opts); err != nil {
		t.Errorf("expected nil with Engine unset (sweep skipped), got: %v", err)
	}
}

// --- suite-existence sweep -------------------------------------------------
//
// These exercise runSuiteExistenceChecks directly against the real default
// engine, so the literal sweep runs over the actual process-flow.yaml
// `suite:` values (acceptance, contract-real, contract-stub). Engine-wiring
// into Run itself is already covered by the scope-sweep tests above (same
// opts.Engine gate); these isolate the suite-resolution logic from the FS,
// remote, and scope-sweep passes.

// loadDefaultEngine loads the embedded process-flow.yaml, failing the test on
// error. Shared by the suite-existence cases below.
func loadDefaultEngine(t *testing.T) *statemachine.Engine {
	t.Helper()
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("load state machine: %v", err)
	}
	return eng
}

// writeTestsYAML writes a minimal-but-valid tests.yaml declaring one suite per
// id (LoadTests requires id/name/command on each), returning the file path.
func writeTestsYAML(t *testing.T, suiteIDs ...string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("suites:\n")
	for _, id := range suiteIDs {
		fmt.Fprintf(&b, "  - id: %s\n    name: %q\n    command: echo run\n", id, id)
	}
	path := filepath.Join(t.TempDir(), "tests.yaml")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write tests.yaml: %v", err)
	}
	return path
}

func TestSuiteExistence_RenamedAcceptanceSuite(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{Channels: []string{"api", "ui"}}
	// acceptance-parallel-api renamed to the bare "acceptance" alias; the other
	// three partition ids kept. Expected per-channel ids are
	// acceptance-parallel-<ch> plus acceptance-isolated-<ch>, so
	// acceptance-parallel-api is the only missing one.
	path := writeTestsYAML(t, "acceptance", "acceptance-parallel-ui", "acceptance-isolated-api", "acceptance-isolated-ui")
	got := runSuiteExistenceChecks(cfg, loadDefaultEngine(t), path)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, `"acceptance-parallel-api"`) {
		t.Errorf("expected failure naming acceptance-parallel-api, got: %v", got)
	}
	if strings.Contains(joined, `"acceptance-parallel-ui"`) {
		t.Errorf("acceptance-parallel-ui is present and must not be flagged, got: %v", got)
	}
}

func TestSuiteExistence_RenamedContractSuite_WithExternalSystems(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{
		Channels: []string{"api", "ui"},
		ExternalSystems: projectconfig.ExternalSystems{
			"warehouse": {
				RealKind: projectconfig.RealKindTestInstance,
				Stub:     projectconfig.ExternalSpec{Path: "stubs", Repo: "acme/externals"},
			},
		},
	}
	// contract-real renamed away; everything else present. External systems
	// are configured, so the contract suites are required.
	path := writeTestsYAML(t, "acceptance-parallel-api", "acceptance-parallel-ui", "acceptance-isolated-api", "acceptance-isolated-ui", "contract-stub")
	got := runSuiteExistenceChecks(cfg, loadDefaultEngine(t), path)
	if !strings.Contains(strings.Join(got, "\n"), `"contract-real"`) {
		t.Errorf("expected failure naming contract-real, got: %v", got)
	}
}

func TestSuiteExistence_RenamedContractSuite_NoExternalSystems(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{Channels: []string{"api", "ui"}}
	// Same missing contract suites, but no external-systems: the contract
	// branch is unreachable, so requiring them would be a false positive.
	path := writeTestsYAML(t, "acceptance-parallel-api", "acceptance-parallel-ui", "acceptance-isolated-api", "acceptance-isolated-ui")
	got := runSuiteExistenceChecks(cfg, loadDefaultEngine(t), path)
	if len(got) != 0 {
		t.Errorf("expected no failures with no external systems, got: %v", got)
	}
}

func TestSuiteExistence_APIOnlyProject_NoUIFalsePositive(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{Channels: []string{"api"}}
	// An api-only project's tests.yaml declares its api acceptance suites
	// (parallel + isolated); the acceptance alias must expand per cfg.Channels,
	// not to the static [api, ui] group — so the ui ids (acceptance-parallel-ui,
	// acceptance-isolated-ui) must NOT be required.
	path := writeTestsYAML(t, "acceptance-parallel-api", "acceptance-isolated-api")
	got := runSuiteExistenceChecks(cfg, loadDefaultEngine(t), path)
	if len(got) != 0 {
		t.Errorf("api-only project should not require ui acceptance suites, got: %v", got)
	}
}

func TestSuiteExistence_AllSuitesPresent(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{
		Channels: []string{"api", "ui"},
		ExternalSystems: projectconfig.ExternalSystems{
			"warehouse": {
				RealKind: projectconfig.RealKindTestInstance,
				Stub:     projectconfig.ExternalSpec{Path: "stubs", Repo: "acme/externals"},
			},
		},
	}
	path := writeTestsYAML(t, "acceptance-parallel-api", "acceptance-parallel-ui", "acceptance-isolated-api", "acceptance-isolated-ui", "contract-real", "contract-stub")
	got := runSuiteExistenceChecks(cfg, loadDefaultEngine(t), path)
	if len(got) != 0 {
		t.Errorf("expected no failures when every required suite is declared, got: %v", got)
	}
}

func TestSuiteExistence_MissingTestsFile_NamesPath(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{Channels: []string{"api", "ui"}}
	missing := filepath.Join(t.TempDir(), "does-not-exist.yaml")
	got := runSuiteExistenceChecks(cfg, loadDefaultEngine(t), missing)
	if len(got) != 1 {
		t.Fatalf("expected exactly one failure for a missing tests.yaml, got: %v", got)
	}
	if !strings.Contains(got[0], missing) {
		t.Errorf("failure should name the tests.yaml path %q, got: %v", missing, got[0])
	}
}

func TestSuiteExistence_SkippedWhenEngineNilOrPathEmpty(t *testing.T) {
	t.Parallel()
	cfg := &projectconfig.Config{Channels: []string{"api", "ui"}}
	if got := runSuiteExistenceChecks(cfg, nil, "tests.yaml"); got != nil {
		t.Errorf("nil engine should skip the sweep, got: %v", got)
	}
	if got := runSuiteExistenceChecks(cfg, loadDefaultEngine(t), ""); got != nil {
		t.Errorf("empty tests path should skip the sweep, got: %v", got)
	}
}

// --- effective-suite resolution sweep --------------------------------------

// TestEffectiveSuiteResolution_RealEngineAllResolvable is the no-false-positive
// guard: every `${suite}` placeholder in the shipped process-flow.yaml must
// resolve to a concrete value by propagating concrete `suite:` call-site params
// down the call graph. A failure here would mean the new sweep halts the real
// `gh optivem implement` preflight — so this asserts the live flow is clean
// (and catches any future edit that introduces a state-sourced suite).
func TestEffectiveSuiteResolution_RealEngineAllResolvable(t *testing.T) {
	t.Parallel()
	if got := runEffectiveSuiteResolutionChecks(loadDefaultEngine(t)); len(got) != 0 {
		t.Errorf("real engine must have no unresolvable suites, got:\n%s", strings.Join(got, "\n"))
	}
}

func TestEffectiveSuiteResolution_NilEngineSkips(t *testing.T) {
	t.Parallel()
	if got := runEffectiveSuiteResolutionChecks(nil); got != nil {
		t.Errorf("nil engine should skip the sweep, got: %v", got)
	}
}

// TestEffectiveSuiteResolution_StateSourcedSuiteFlagged is the regression for
// the plan's root cause: a verify node whose `${suite}` is never bound by a
// static call-site param (it would resolve only from runtime state) must fail
// preflight, naming the node — so a bad suite can never silently reach the
// runner. A sibling flow that DOES bind a concrete suite at the call site must
// pass, proving the sweep only flags the genuinely unresolvable case.
// fullMultitierCfg returns a multitier config whose system-test.paths carries
// every canonical Family B key, so every writing-agent MID's read:/write: list
// in the real engine resolves. Backend/Frontend paths and the shared migration
// path are set, so the system-path surface resolves non-empty.
func fullMultitierCfg() *projectconfig.Config {
	paths := map[string]string{}
	for _, k := range projectconfig.CanonicalPathKeys() {
		paths[k] = "system-test/java/" + k
	}
	cfg := &projectconfig.Config{}
	cfg.System.Architecture = projectconfig.ArchMultitier
	cfg.System.DbMigrationPath = "system/db/migrations"
	cfg.System.Backend = projectconfig.TierSpec{Path: "system/multitier/backend-java"}
	cfg.System.Frontend = projectconfig.TierSpec{Path: "system/multitier/frontend-typescript"}
	cfg.SystemTest.Paths = paths
	return cfg
}

// TestRun_ScopeSweepCatchesCollapsedMultitierSurface covers the per-layer
// non-empty gate (this plan's Item 3): a writing-agent MID whose write list
// resolves to no real path on a multitier config — the bug class where the
// system-path surface silently collapses — is flagged statically, while a
// well-formed multitier config passes. A synthetic single-layer write:
// [system-path] is used because the real implement-system pairs system-path
// with the always-present system-db-migration-path, so its list never fully
// collapses; the gate's value is catching a layer that DOES collapse to
// nothing.
func TestRun_ScopeSweepCatchesCollapsedMultitierSurface(t *testing.T) {
	t.Parallel()

	const flow = `
processes:
  root:
    name: "Root"
    start: CALL_IMPL
    nodes:
      - id: CALL_IMPL
        type: call-activity
        process: impl-system
        name: "Call Impl"
      - id: ROOT_END
        type: end-event
        name: "End"
    sequence-flows:
      - {from: CALL_IMPL, to: ROOT_END}
  impl-system:
    name: "Implement System"
    start: EXECUTE_AGENT
    nodes:
      - id: EXECUTE_AGENT
        type: call-activity
        process: execute-agent
        name: "Dispatch the Agent"
        params:
          task-name: implement-system
          agent: system-implementer
          category: prod-agent
        write: [system-path]
      - id: IMPL_END
        type: end-event
        name: "End"
    sequence-flows:
      - {from: EXECUTE_AGENT, to: IMPL_END}
`
	eng, err := statemachine.LoadBytes([]byte(flow))
	if err != nil {
		t.Fatalf("load synthetic flow: %v", err)
	}

	t.Run("collapsed multitier surface (empty tiers) is flagged", func(t *testing.T) {
		cfg := &projectconfig.Config{}
		cfg.System.Architecture = projectconfig.ArchMultitier // backend/frontend paths left empty
		got := runScopeResolutionChecks(cfg, eng)
		joined := strings.Join(got, "\n")
		if !strings.Contains(joined, "impl-system") || !strings.Contains(joined, "layer collapsed") {
			t.Errorf("expected a layer-collapsed failure naming impl-system, got: %v", got)
		}
	})

	t.Run("well-formed multitier surface passes", func(t *testing.T) {
		cfg := &projectconfig.Config{}
		cfg.System.Architecture = projectconfig.ArchMultitier
		cfg.System.Backend = projectconfig.TierSpec{Path: "system/multitier/backend-java"}
		cfg.System.Frontend = projectconfig.TierSpec{Path: "system/multitier/frontend-typescript"}
		if got := runScopeResolutionChecks(cfg, eng); len(got) != 0 {
			t.Errorf("well-formed multitier write: [system-path] must resolve non-empty, got: %v", got)
		}
	})
}

// TestRun_ScopeSweepRealEngineMultitierPasses guards that the real embedded
// engine's full set of writing-agent MIDs resolves cleanly on a well-formed
// multitier config — the system-path layers now expand to the tier surface
// instead of collapsing to nothing.
func TestRun_ScopeSweepRealEngineMultitierPasses(t *testing.T) {
	t.Parallel()
	if got := runScopeResolutionChecks(fullMultitierCfg(), loadDefaultEngine(t)); len(got) != 0 {
		t.Errorf("real engine on a well-formed multitier config must pass the scope sweep, got: %v", got)
	}
}

func TestEffectiveSuiteResolution_StateSourcedSuiteFlagged(t *testing.T) {
	t.Parallel()

	const unboundFlow = `
processes:
  root:
    name: "Root"
    start: CALL_VERIFY
    nodes:
      - id: CALL_VERIFY
        type: call-activity
        process: verify
        name: "Call Verify"
      - id: ROOT_END
        type: end-event
        name: "End"
    sequence-flows:
      - {from: CALL_VERIFY, to: ROOT_END}
  verify:
    name: "Verify"
    start: RUN_TESTS
    nodes:
      - id: RUN_TESTS
        type: service-task
        action: run-command
        name: "Run Tests"
        params:
          suite: ${suite}
      - id: VERIFY_END
        type: end-event
        name: "End"
    sequence-flows:
      - {from: RUN_TESTS, to: VERIFY_END}
`
	engUnbound, err := statemachine.LoadBytes([]byte(unboundFlow))
	if err != nil {
		t.Fatalf("load unbound flow: %v", err)
	}
	got := runEffectiveSuiteResolutionChecks(engUnbound)
	if len(got) == 0 {
		t.Fatal("expected a failure for the state-sourced suite, got none")
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "RUN_TESTS") || !strings.Contains(joined, `"verify"`) {
		t.Errorf("failure should name node RUN_TESTS in process verify, got: %v", got)
	}

	// Same shape, but the caller binds a concrete suite at the call site.
	const boundFlow = `
processes:
  root:
    name: "Root"
    start: CALL_VERIFY
    nodes:
      - id: CALL_VERIFY
        type: call-activity
        process: verify
        name: "Call Verify"
        params:
          suite: acceptance
      - id: ROOT_END
        type: end-event
        name: "End"
    sequence-flows:
      - {from: CALL_VERIFY, to: ROOT_END}
  verify:
    name: "Verify"
    start: RUN_TESTS
    nodes:
      - id: RUN_TESTS
        type: service-task
        action: run-command
        name: "Run Tests"
        params:
          suite: ${suite}
      - id: VERIFY_END
        type: end-event
        name: "End"
    sequence-flows:
      - {from: RUN_TESTS, to: VERIFY_END}
`
	engBound, err := statemachine.LoadBytes([]byte(boundFlow))
	if err != nil {
		t.Fatalf("load bound flow: %v", err)
	}
	if got := runEffectiveSuiteResolutionChecks(engBound); len(got) != 0 {
		t.Errorf("a concretely-bound suite must not be flagged, got: %v", got)
	}
}
