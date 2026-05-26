package repolocator

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/optivem/gh-optivem/internal/projectconfig"
)

// makeGitDir creates dir/.git as a directory — the canonical layout of a
// non-worktree clone.
func makeGitDir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir %s/.git: %v", dir, err)
	}
}

// makeGitFile creates dir/.git as a file containing a gitdir pointer —
// the layout git uses for a linked worktree.
func makeGitFile(t *testing.T, dir, target string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	body := "gitdir: " + target + "\n"
	if err := os.WriteFile(filepath.Join(dir, ".git"), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s/.git: %v", dir, err)
	}
}

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

// TestResolve_MonoRepoWalksUpFromCwdToGitRoot pins the mono-repo branch
// of Resolve: when cwd is inside a real git repo, the locator returns
// the repo root regardless of the directory's name. Closes the gap
// where the old parent(cwd)/<repo-name> formula required cwd to be the
// clone root with the exact repo-name folder name.
func TestResolve_MonoRepoWalksUpFromCwdToGitRoot(t *testing.T) {
	repo := t.TempDir()
	makeGitDir(t, repo)

	got, err := Resolve(newMonoRepoConfig(), "", repo)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantRoot, _ := filepath.Abs(repo)
	if got.Local["optivem/shop"] != wantRoot {
		t.Errorf("optivem/shop: got %q, want %q (git-resolved root)",
			got.Local["optivem/shop"], wantRoot)
	}
}

// TestResolve_MonoRepoWalksUpFromSubdir confirms the mono-repo branch
// walks up from a nested subdirectory to find the clone root — same
// semantics as workspace.Resolve's git-repo walk-up.
func TestResolve_MonoRepoWalksUpFromSubdir(t *testing.T) {
	repo := t.TempDir()
	makeGitDir(t, repo)
	deep := filepath.Join(repo, "system", "monolith", "java")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir deep: %v", err)
	}

	got, err := Resolve(newMonoRepoConfig(), "", deep)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantRoot, _ := filepath.Abs(repo)
	if got.Local["optivem/shop"] != wantRoot {
		t.Errorf("optivem/shop: got %q, want %q (walk-up from subdir)",
			got.Local["optivem/shop"], wantRoot)
	}
}

// TestResolve_MonoRepoWorktreeFile confirms the mono-repo branch treats
// a .git *file* (git worktree pointer) the same as a .git directory.
// This is the rehearsal-worktree case: the worktree directory is named
// rehearsal-<id>, not the repo-name, so the old formula resolved to a
// non-existent sibling path.
func TestResolve_MonoRepoWorktreeFile(t *testing.T) {
	wt := filepath.Join(t.TempDir(), "rehearsal-20260527")
	makeGitFile(t, wt, "/main/repo/.git/worktrees/rehearsal-20260527")

	got, err := Resolve(newMonoRepoConfig(), "", wt)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantRoot, _ := filepath.Abs(wt)
	if got.Local["optivem/shop"] != wantRoot {
		t.Errorf("optivem/shop: got %q, want %q (worktree root via .git file)",
			got.Local["optivem/shop"], wantRoot)
	}
}

// TestResolve_MonoRepoNoGitRepoFallsBackToSiblingFormula confirms the
// fall-through path: when the mono-repo branch finds no .git on the way
// to the filesystem root, the locator drops back to the old
// parent(cwd)/<repo-name> formula. Preserves today's behaviour for
// invocations from outside a clone — preflight still surfaces the
// missing-clone failure with a clear message.
func TestResolve_MonoRepoNoGitRepoFallsBackToSiblingFormula(t *testing.T) {
	bare := t.TempDir()

	got, err := Resolve(newMonoRepoConfig(), "", bare)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join(filepath.Dir(bare), "shop")
	if got.Local["optivem/shop"] != want {
		t.Errorf("optivem/shop: got %q, want %q (sibling-formula fallback)",
			got.Local["optivem/shop"], want)
	}
}

// TestResolve_MonoRepoExplicitWorkspaceSkipsGitWalkup confirms that an
// operator-supplied --workspace pins the sibling formula even for
// mono-repo configs. The mono-repo branch only kicks in when no
// workspace was supplied — back-compat for any caller that today
// passes --workspace=… expecting the parent/<repo-name> resolution.
func TestResolve_MonoRepoExplicitWorkspaceSkipsGitWalkup(t *testing.T) {
	repo := t.TempDir()
	makeGitDir(t, repo)
	ws := filepath.Join("/", "opt", "workspace")

	got, err := Resolve(newMonoRepoConfig(), ws, repo)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join(ws, "shop")
	if got.Local["optivem/shop"] != want {
		t.Errorf("optivem/shop: got %q, want %q (explicit --workspace wins)",
			got.Local["optivem/shop"], want)
	}
}

// TestResolve_MultiRepoIgnoresGitWalkup confirms the mono-repo branch
// is gated on RepoStrategy: a multi-repo config keeps the sibling
// formula even when cwd is inside a git repo.
func TestResolve_MultiRepoIgnoresGitWalkup(t *testing.T) {
	repo := t.TempDir()
	makeGitDir(t, repo)

	got, err := Resolve(newMultiRepoConfig(), "", repo)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	parent := filepath.Dir(repo)
	if got.Local["optivem/shop-backend"] != filepath.Join(parent, "shop-backend") {
		t.Errorf("backend: got %q, want %q (sibling formula for multi-repo)",
			got.Local["optivem/shop-backend"], filepath.Join(parent, "shop-backend"))
	}
}
