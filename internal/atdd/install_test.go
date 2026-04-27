package atdd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSubstituteRepoName(t *testing.T) {
	cases := []struct {
		name string
		in   string
		repo string
		want string
	}{
		{
			name: "bare shop is replaced",
			in:   "the `shop` repo",
			repo: "page-turner",
			want: "the `page-turner` repo",
		},
		{
			name: "shop with trailing slash is preserved (doctrine package)",
			in:   "files under `shop/api`",
			repo: "page-turner",
			want: "files under `shop/api`",
		},
		{
			name: "mixed shop and shop/ in same line",
			in:   "the `shop/` package and the `shop` repo",
			repo: "page-turner",
			want: "the `shop/` package and the `page-turner` repo",
		},
		{
			name: "Shop (capitalized) is left alone",
			in:   "ShopApiClient composes controllers",
			repo: "page-turner",
			want: "ShopApiClient composes controllers",
		},
		{
			name: "shop-suffix becomes <repo>-suffix",
			in:   "the `shop-system` repo and the `shop-backend` repo",
			repo: "page-turner",
			want: "the `page-turner-system` repo and the `page-turner-backend` repo",
		},
		{
			name: "multiple occurrences",
			in:   "shop, shop, shop",
			repo: "x",
			want: "x, x, x",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := substituteRepoName(c.in, c.repo)
			if got != c.want {
				t.Errorf("substituteRepoName(%q, %q) = %q; want %q", c.in, c.repo, got, c.want)
			}
		})
	}
}

func TestStripMultirepoTODOs(t *testing.T) {
	in := `before
     <!-- TODO(gh-optivem): multirepo support — for ` + "`<repo>`" + ` + ` + "`<repo>-backend`" + ` (multitier) scaffolds, the install-time substitution needs to expand ` + "`shop`" + `. v1 install is monorepo-only. -->
after
`
	want := `before
after
`
	got := stripMultirepoTODOs(in)
	if got != want {
		t.Errorf("stripMultirepoTODOs:\n got: %q\nwant: %q", got, want)
	}
}

func TestStripMultirepoTODOs_PreservesUnrelatedComments(t *testing.T) {
	in := "<!-- some other comment -->\nkeep me\n"
	got := stripMultirepoTODOs(in)
	if got != in {
		t.Errorf("expected unrelated comment preserved; got %q", got)
	}
}

func TestTransformMonorepo_BulkSubstitutionOnly(t *testing.T) {
	opts := Options{
		ShopPath:     "irrelevant",
		DestDir:      "irrelevant",
		Repo:         "page-turner",
		Arch:         "monolith",
		RepoStrategy: "monorepo",
	}
	in := managerSourceBlock + "\n     <!-- TODO(gh-optivem): multirepo support — placeholder. v1 install is monorepo-only. -->\n"
	got := Transform(in, opts)

	wantContains := []string{
		"`page-turner` (system tests live in the same monorepo as the system).",
		"Known system repositories: `page-turner`.",
		"default to the `page-turner` repository.",
	}
	for _, w := range wantContains {
		if !strings.Contains(got, w) {
			t.Errorf("monorepo Transform missing %q\ngot:\n%s", w, got)
		}
	}
	if strings.Contains(got, "TODO(gh-optivem)") {
		t.Errorf("monorepo Transform did not strip TODO comment\ngot:\n%s", got)
	}
}

func TestTransformMultirepoMonolith_BlockRewriteAndSubstitution(t *testing.T) {
	opts := Options{
		ShopPath:     "irrelevant",
		DestDir:      "irrelevant",
		Repo:         "page-turner",
		Arch:         "monolith",
		RepoStrategy: "multirepo",
	}
	got := Transform(managerSourceBlock, opts)

	wantContains := []string{
		"Known test repositories: `page-turner` (the orchestration root repo, which hosts the system tests).",
		"Known system repositories: `page-turner-system`.",
		"default to `page-turner` for tests and `page-turner-system` for system code.",
	}
	for _, w := range wantContains {
		if !strings.Contains(got, w) {
			t.Errorf("multirepo monolith Transform missing %q\ngot:\n%s", w, got)
		}
	}
}

func TestTransformMultirepoMultitier_BlockRewriteAndSubstitution(t *testing.T) {
	opts := Options{
		ShopPath:     "irrelevant",
		DestDir:      "irrelevant",
		Repo:         "page-turner",
		Arch:         "multitier",
		RepoStrategy: "multirepo",
	}
	got := Transform(managerSourceBlock, opts)

	wantContains := []string{
		"Known system repositories: `page-turner-backend`, `page-turner-frontend`.",
		"pick `page-turner-backend` or `page-turner-frontend`",
	}
	for _, w := range wantContains {
		if !strings.Contains(got, w) {
			t.Errorf("multitier Transform missing %q\ngot:\n%s", w, got)
		}
	}
}

func TestTransformAcceptanceCommit_MultirepoMultitier_SplitCommitBlock(t *testing.T) {
	opts := Options{
		ShopPath:     "irrelevant",
		DestDir:      "irrelevant",
		Repo:         "page-turner",
		Arch:         "multitier",
		RepoStrategy: "multirepo",
	}
	got := Transform(acceptanceCommitSourceBlock, opts)

	wantContains := []string{
		"In the `page-turner-backend` repository: COMMIT backend changes",
		"In the `page-turner-frontend` repository: COMMIT frontend changes",
		"COMMIT in the `page-turner` repository",
	}
	for _, w := range wantContains {
		if !strings.Contains(got, w) {
			t.Errorf("acceptance-commit multitier Transform missing %q\ngot:\n%s", w, got)
		}
	}
}

func TestFindBareShop_DoctrinePackagePathsAreNotFlagged(t *testing.T) {
	in := "files under `shop/api` are doctrine package members"
	hits := findBareShop(in)
	if len(hits) != 0 {
		t.Errorf("shop/api should not flag; got %v", hits)
	}
}

func TestFindBareShop_BareShopIsFlagged(t *testing.T) {
	in := "the shop repo is here"
	hits := findBareShop(in)
	if len(hits) != 1 {
		t.Errorf("expected 1 hit; got %v", hits)
	}
}

func TestOptionsValidate(t *testing.T) {
	cases := []struct {
		name    string
		opts    Options
		wantErr bool
	}{
		{
			name:    "valid monorepo monolith",
			opts:    Options{ShopPath: "/x", DestDir: "/y", Repo: "r", Arch: "monolith", RepoStrategy: "monorepo"},
			wantErr: false,
		},
		{
			name:    "missing repo",
			opts:    Options{ShopPath: "/x", DestDir: "/y", Arch: "monolith", RepoStrategy: "monorepo"},
			wantErr: true,
		},
		{
			name:    "bad arch",
			opts:    Options{ShopPath: "/x", DestDir: "/y", Repo: "r", Arch: "wat", RepoStrategy: "monorepo"},
			wantErr: true,
		},
		{
			name:    "bad strategy",
			opts:    Options{ShopPath: "/x", DestDir: "/y", Repo: "r", Arch: "monolith", RepoStrategy: "wat"},
			wantErr: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.opts.Validate()
			if (err != nil) != c.wantErr {
				t.Errorf("Validate err = %v; wantErr %v", err, c.wantErr)
			}
		})
	}
}

// TestInstallEndToEnd builds a minimal fake-shop tree, installs from it into
// a tempdir, and asserts the resulting files are present and substituted.
// Round-trips Plan + Install + Validate.
func TestInstallEndToEnd(t *testing.T) {
	shop := t.TempDir()
	dest := t.TempDir()

	// Fake shop: one agent file, one command file, one prompt file in each subdir.
	mustWrite := func(rel, content string) {
		t.Helper()
		full := filepath.Join(shop, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite(".claude/agents/atdd-test.md", "agent content using `shop` as repo and `shop/` as package")
	mustWrite(".claude/commands/atdd-thing.md", "command using `shop` repo")
	mustWrite("docs/prompts/atdd/thing.md", "prompt for `shop`")
	mustWrite("docs/prompts/architecture/thing.md", "arch prompt")
	mustWrite("docs/prompts/code/thing.md", "code prompt")

	// Pre-existing student-authored file that should NOT be wiped.
	studentFile := filepath.Join(dest, ".claude", "agents", "my-custom.md")
	if err := os.MkdirAll(filepath.Dir(studentFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(studentFile, []byte("student content"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := Options{
		ShopPath:     shop,
		DestDir:      dest,
		Repo:         "page-turner",
		Arch:         "monolith",
		RepoStrategy: "monorepo",
	}
	if err := Install(opts); err != nil {
		t.Fatalf("Install: %v", err)
	}

	// Managed file substituted.
	got, _ := os.ReadFile(filepath.Join(dest, ".claude/agents/atdd-test.md"))
	want := "agent content using `page-turner` as repo and `shop/` as package"
	if string(got) != want {
		t.Errorf("agent content:\n got: %q\nwant: %q", got, want)
	}

	// Student-authored file preserved.
	if _, err := os.Stat(studentFile); err != nil {
		t.Errorf("student file was wiped: %v", err)
	}

	// Idempotence: second install should succeed and produce identical output.
	if err := Install(opts); err != nil {
		t.Fatalf("second Install: %v", err)
	}
	got2, _ := os.ReadFile(filepath.Join(dest, ".claude/agents/atdd-test.md"))
	if string(got2) != want {
		t.Errorf("second-install non-idempotent:\n got: %q\nwant: %q", got2, want)
	}
}

// TestInstallPreflightAbortsOnLocalEdits verifies that without --force, an
// install aborts when an existing managed file's content has been edited.
func TestInstallPreflightAbortsOnLocalEdits(t *testing.T) {
	shop := t.TempDir()
	dest := t.TempDir()

	srcAgent := filepath.Join(shop, ".claude", "agents", "atdd-x.md")
	if err := os.MkdirAll(filepath.Dir(srcAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(srcAgent, []byte("`shop` content"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, sub := range managedPromptSubdirs {
		if err := os.MkdirAll(filepath.Join(shop, "docs", "prompts", sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Pre-populate dest with an edited copy.
	destAgent := filepath.Join(dest, ".claude", "agents", "atdd-x.md")
	if err := os.MkdirAll(filepath.Dir(destAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(destAgent, []byte("LOCAL EDITS"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := Options{
		ShopPath:     shop,
		DestDir:      dest,
		Repo:         "page-turner",
		Arch:         "monolith",
		RepoStrategy: "monorepo",
	}

	err := Install(opts)
	if err == nil {
		t.Fatal("expected pre-flight to abort; got nil error")
	}
	if !strings.Contains(err.Error(), "local edits") {
		t.Errorf("expected 'local edits' in error; got %v", err)
	}

	// With --force, install proceeds.
	opts.Force = true
	if err := Install(opts); err != nil {
		t.Fatalf("Install with --force: %v", err)
	}
	got, _ := os.ReadFile(destAgent)
	if !strings.Contains(string(got), "page-turner") {
		t.Errorf("expected --force overwrite; got %q", got)
	}
}
