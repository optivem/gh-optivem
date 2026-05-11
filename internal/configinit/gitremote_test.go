package configinit

import "testing"

func TestParseGitHubRemote(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		in        string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{"https with .git", "https://github.com/acme/page-turner.git", "acme", "page-turner", true},
		{"https without .git", "https://github.com/acme/page-turner", "acme", "page-turner", true},
		{"http with .git", "http://github.com/acme/page-turner.git", "acme", "page-turner", true},
		{"scp ssh with .git", "git@github.com:acme/page-turner.git", "acme", "page-turner", true},
		{"scp ssh without .git", "git@github.com:acme/page-turner", "acme", "page-turner", true},
		{"ssh:// form", "ssh://git@github.com/acme/page-turner.git", "acme", "page-turner", true},
		{"trailing slash", "https://github.com/acme/page-turner/", "", "", false},
		{"extra path segment", "https://github.com/acme/page-turner/sub", "", "", false},
		{"missing repo", "https://github.com/acme/", "", "", false},
		{"missing owner", "https://github.com//page-turner.git", "", "", false},
		{"non-github host", "https://gitlab.com/acme/page-turner.git", "", "", false},
		{"enterprise host", "https://github.acme.com/acme/page-turner.git", "", "", false},
		{"empty", "", "", "", false},
		{"garbage", "not-a-url", "", "", false},
		{"bad owner format (leading hyphen)", "https://github.com/-bad/page-turner.git", "", "", false},
		{"bad repo format (leading dot)", "https://github.com/acme/.config.git", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			owner, repo, ok := parseGitHubRemote(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ok: got %v, want %v", ok, tc.wantOK)
			}
			if owner != tc.wantOwner {
				t.Errorf("owner: got %q, want %q", owner, tc.wantOwner)
			}
			if repo != tc.wantRepo {
				t.Errorf("repo: got %q, want %q", repo, tc.wantRepo)
			}
		})
	}
}

// TestInferOwnerRepo_UsesRunner verifies that InferOwnerRepo plumbs
// runGitRemote's output through parseGitHubRemote — the only test that
// exercises the public function. Subprocess wiring is patched out so
// the test stays hermetic.
func TestInferOwnerRepo_UsesRunner(t *testing.T) {
	orig := runGitRemote
	t.Cleanup(func() { runGitRemote = orig })

	runGitRemote = func(string) (string, error) {
		return "https://github.com/acme/page-turner.git", nil
	}
	owner, repo, ok := InferOwnerRepo("/tmp")
	if !ok || owner != "acme" || repo != "page-turner" {
		t.Errorf("got owner=%q repo=%q ok=%v; want acme/page-turner/true", owner, repo, ok)
	}
}

// TestInferOwnerRepo_RunnerError surfaces as ok=false. Covers the "no
// git remote" / "not a git repo" path.
func TestInferOwnerRepo_RunnerError(t *testing.T) {
	orig := runGitRemote
	t.Cleanup(func() { runGitRemote = orig })

	runGitRemote = func(string) (string, error) {
		return "", &mockExecErr{}
	}
	_, _, ok := InferOwnerRepo("/tmp")
	if ok {
		t.Errorf("ok: got true, want false on runner error")
	}
}

type mockExecErr struct{}

func (mockExecErr) Error() string { return "exit status 128" }
