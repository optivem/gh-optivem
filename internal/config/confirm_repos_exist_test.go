package config

import (
	"os"
	"os/exec"
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

// TestConfirmReposExist_OverwriteReturnsPreExistingSet pins the opt-in path:
// with overwrite, pre-existing repos are returned (not fatal) so CreateRepos
// can skip them.
func TestConfirmReposExist_OverwriteReturnsPreExistingSet(t *testing.T) {
	dir := mkPathDir(t)
	writeStubOSSpecific(t, dir, "gh", reposExistStubBody(t, []string{"acme/backend", "acme/system"}))

	got := confirmReposExist([]string{"acme/backend", "acme/frontend", "acme/system"}, true)
	want := []string{"acme/backend", "acme/system"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// TestConfirmReposExist_NoneExistWithoutOverwrite covers the greenfield path:
// nothing exists, so no flag is needed and nothing is returned.
func TestConfirmReposExist_NoneExistWithoutOverwrite(t *testing.T) {
	dir := mkPathDir(t)
	writeStubOSSpecific(t, dir, "gh", reposExistStubBody(t, nil))

	if got := confirmReposExist([]string{"acme/repo-one"}, false); len(got) != 0 {
		t.Fatalf("expected no existing repos, got %v", got)
	}
}

// runFatalHelper re-executes the test binary running only the named helper
// test, because log.FatalExit calls os.Exit. It returns the combined output
// and whether the child exited non-zero.
func runFatalHelper(t *testing.T, helper string) (string, bool) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+helper+"$")
	cmd.Env = append(os.Environ(), "OPTIVEM_FATAL_HELPER=1")
	out, err := cmd.CombinedOutput()
	return string(out), err != nil
}

func TestConfirmReposExist_ExistingWithoutOverwriteFailsFast(t *testing.T) {
	if os.Getenv("OPTIVEM_FATAL_HELPER") == "1" {
		dir := mkPathDir(t)
		writeStubOSSpecific(t, dir, "gh", reposExistStubBody(t, []string{"acme/backend"}))
		confirmReposExist([]string{"acme/backend", "acme/frontend"}, false)
		return
	}
	out, failed := runFatalHelper(t, "TestConfirmReposExist_ExistingWithoutOverwriteFailsFast")
	if !failed {
		t.Fatalf("expected non-zero exit without --overwrite, got success: %s", out)
	}
	if !strings.Contains(out, "acme/backend") || !strings.Contains(out, "--overwrite") {
		t.Fatalf("error must name the repo and point at --overwrite, got: %s", out)
	}
}

func TestRequireAllReposExistForDryRun_NeedsOverwrite(t *testing.T) {
	if os.Getenv("OPTIVEM_FATAL_HELPER") == "1" {
		requireAllReposExistForDryRun([]string{"acme/a"}, false, map[string]bool{"acme/a": true})
		return
	}
	out, failed := runFatalHelper(t, "TestRequireAllReposExistForDryRun_NeedsOverwrite")
	if !failed || !strings.Contains(out, "requires --overwrite") {
		t.Fatalf("expected --overwrite requirement failure, failed=%v out=%s", failed, out)
	}
}

func TestRequireAllReposExistForDryRun_MissingRepoFails(t *testing.T) {
	if os.Getenv("OPTIVEM_FATAL_HELPER") == "1" {
		requireAllReposExistForDryRun([]string{"acme/a", "acme/b"}, true, map[string]bool{"acme/a": true})
		return
	}
	out, failed := runFatalHelper(t, "TestRequireAllReposExistForDryRun_MissingRepoFails")
	if !failed || !strings.Contains(out, "acme/b") {
		t.Fatalf("expected failure naming acme/b, failed=%v out=%s", failed, out)
	}
}

func TestRequireAllReposExistForDryRun_AllExistPasses(t *testing.T) {
	requireAllReposExistForDryRun([]string{"acme/a", "", "acme/b"}, true, map[string]bool{"acme/a": true, "acme/b": true})
}
