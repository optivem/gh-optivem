package config

import (
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// reposExistStubBody builds a `gh` stub that answers `gh api repos/<repo>
// --silent` with exit 0 for every repo in exists (found) and exit 1 (404)
// for everything else — the same shape confirmReposExist/reposThatExist
// probes with. Branches on the second argument ("repos/<repo>"), so the
// body must be built per-OS: POSIX sh uses `case`, cmd.exe uses `if`.
func reposExistStubBody(t *testing.T, exists []string) string {
	t.Helper()
	var body strings.Builder
	if runtime.GOOS == "windows" {
		for _, r := range exists {
			body.WriteString("if \"%2\"==\"repos/" + r + "\" (\nexit /b 0\n)\n")
		}
		body.WriteString("exit /b 1")
		return body.String()
	}
	body.WriteString("case \"$2\" in\n")
	for _, r := range exists {
		body.WriteString("\"repos/" + r + "\") exit 0 ;;\n")
	}
	body.WriteString("*) exit 1 ;;\nesac\n")
	return body.String()
}

// TestReposThatExist_NoneExist covers the common case: every probed repo
// answers 404, so confirmReposExist has nothing to fail the run over.
func TestReposThatExist_NoneExist(t *testing.T) {
	dir := mkPathDir(t)
	writeStubOSSpecific(t, dir, "gh", reposExistStubBody(t, nil))

	got := reposThatExist([]string{"acme/repo-one", "acme/repo-two"})
	if len(got) != 0 {
		t.Fatalf("expected no existing repos, got %v", got)
	}
}

// TestReposThatExist_SomeExist pins the multirepo shape: only the repos
// that actually exist on GitHub are reported, the rest are silently
// skipped as not-found.
func TestReposThatExist_SomeExist(t *testing.T) {
	dir := mkPathDir(t)
	writeStubOSSpecific(t, dir, "gh", reposExistStubBody(t, []string{"acme/backend"}))

	got := reposThatExist([]string{"acme/backend", "acme/frontend", "acme/system"})
	want := []string{"acme/backend"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected only the pre-existing repo, got %v want %v", got, want)
	}
}

// TestReposThatExist_SkipsEmptyEntries covers the ""-entry case: not every
// arch/repo-strategy combination populates every multirepo slot, and an
// empty full-repo string must never be probed.
func TestReposThatExist_SkipsEmptyEntries(t *testing.T) {
	dir := mkPathDir(t)
	writeStubOSSpecific(t, dir, "gh", reposExistStubBody(t, nil))

	got := reposThatExist([]string{"acme/repo-one", "", ""})
	if len(got) != 0 {
		t.Fatalf("expected no existing repos, got %v", got)
	}
}
