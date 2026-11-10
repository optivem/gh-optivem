package actions

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/engine/statemachine"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// characterizeConfig is a mono-repo monolith Java config with a stub-backed
// external system, the shape the safety-gate tests run against.
func characterizeConfig() *projectconfig.Config {
	return &projectconfig.Config{
		RepoStrategy: projectconfig.RepoStrategyMonoRepo,
		System: projectconfig.System{
			Architecture:    projectconfig.ArchMonolith,
			Path:            "system/monolith",
			Repo:            "acme/shop",
			DbMigrationPath: "system/db/migrations",
		},
		SystemTest: projectconfig.TierSpec{
			Path: "system-test",
			Repo: "acme/shop",
			Lang: projectconfig.LangJava,
			Paths: map[string]string{
				"at-test":                        "system-test/at",
				"ct-test":                        "system-test/ct",
				"dsl-port":                       "system-test/dsl/port",
				"dsl-core":                       "system-test/dsl/core",
				"system-driver-adapter":          "system-test/driver/system",
				"external-system-driver-adapter": "system-test/driver/external",
			},
		},
		ExternalSystems: projectconfig.ExternalSystems{
			"erp": {
				RealKind: projectconfig.RealKindTestInstance,
				Stub:     projectconfig.ExternalSpec{Path: "external-systems/stubs", Repo: "acme/shop"},
			},
		},
	}
}

// gitRepo creates a git repo in a temp dir with the given files committed.
func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "core.autocrlf", "false")
	for p, c := range files {
		writeRepoFile(t, dir, p, c)
	}
	commitAll(t, dir, "initial")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "--allow-empty", "-m", msg)
}

var baseRepoFiles = map[string]string{
	"system/monolith/App.java":                 "class App {}\n",
	"system/db/migrations/V1__init.sql":        "create table t (id int);\n",
	"external-systems/stubs/mappings/erp.json": "{}\n",
	"system-test/at/OrderTest.java":            "class OrderTest {\n  void t() {\n    order.place();\n    assertThat(order.total()).isEqualTo(10);\n  }\n}\n",
	"system-test/dsl/core/OrderDsl.java":       "class OrderDsl {\n  void v() {\n    result.shouldSucceed();\n    log(\"x\");\n  }\n}\n",
}

func recordedCtx(t *testing.T, a actions) *statemachine.Context {
	t.Helper()
	ctx := statemachine.NewContext()
	if out := a.recordStartCommits(ctx); out.Err != nil {
		t.Fatalf("record-start-commits: %v", out.Err)
	}
	return ctx
}

func TestRecordStartCommits_RecordsCommitsAndPaths(t *testing.T) {
	repo := gitRepo(t, baseRepoFiles)
	a := newActions(Deps{Config: characterizeConfig(), RepoPath: repo, Stdout: &bytes.Buffer{}})
	ctx := recordedCtx(t, a)

	head := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	repos, err := recordedSystemRepos(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Commit != head || strings.Join(repos[0].Paths, ",") != "system/monolith,system/db/migrations" {
		t.Fatalf("system-start-commits = %+v, want one entry at %s with surface + migrations", repos, head)
	}
	if got := ctx.GetString(CtxKeyStandInPaths); got != "external-systems/stubs" {
		t.Errorf("stand-in-paths = %q", got)
	}
	if got := ctx.GetString(CtxKeyAssertionCheckedCommit); got != head {
		t.Errorf("assertion-checked-commit = %q, want %s", got, head)
	}
}

func TestRecordStartCommits_ResumeKeepsStartCommit(t *testing.T) {
	repo := gitRepo(t, baseRepoFiles)
	a := newActions(Deps{Config: characterizeConfig(), RepoPath: repo, Stdout: &bytes.Buffer{}})
	ctx := recordedCtx(t, a)
	before := ctx.GetString(CtxKeySystemStartCommits)
	checked := ctx.GetString(CtxKeyAssertionCheckedCommit)

	writeRepoFile(t, repo, "system-test/at/New.java", "class New {}\n")
	commitAll(t, repo, "layer")
	if out := a.recordStartCommits(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if ctx.GetString(CtxKeySystemStartCommits) != before || ctx.GetString(CtxKeyAssertionCheckedCommit) != checked {
		t.Fatal("a resumed record-start-commits overwrote the recorded commits")
	}
}

func TestRecordStartCommits_FailsHard(t *testing.T) {
	cases := map[string]func(*projectconfig.Config){
		"microservices": func(c *projectconfig.Config) { c.System.Architecture = projectconfig.ArchMicroservices },
		"unset":         func(c *projectconfig.Config) { c.System.Architecture = "" },
		"stand-in equals system": func(c *projectconfig.Config) {
			c.ExternalSystems["erp"] = projectconfig.ExternalSystem{Stub: projectconfig.ExternalSpec{Path: "system/monolith"}}
		},
		"stand-in inside system": func(c *projectconfig.Config) {
			c.ExternalSystems["erp"] = projectconfig.ExternalSystem{Stub: projectconfig.ExternalSpec{Path: "system/monolith/stubs"}}
		},
		"stand-in contains system": func(c *projectconfig.Config) {
			c.ExternalSystems["erp"] = projectconfig.ExternalSystem{Stub: projectconfig.ExternalSpec{Path: "system/"}}
		},
		"simulator overlaps migrations": func(c *projectconfig.Config) {
			c.ExternalSystems["erp"] = projectconfig.ExternalSystem{
				RealKind:  projectconfig.RealKindSimulator,
				Stub:      projectconfig.ExternalSpec{Path: "external-systems/stubs"},
				Simulator: projectconfig.ExternalSpec{Path: "system/db"},
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := characterizeConfig()
			mutate(cfg)
			// No git repo: the config check must fail before any git call.
			a := newActions(Deps{Config: cfg, Git: newFakeRunner(t, "git"), Stdout: &bytes.Buffer{}})
			ctx := statemachine.NewContext()
			if out := a.recordStartCommits(ctx); out.Err == nil {
				t.Fatal("want a hard error")
			}
			if ctx.GetString(CtxKeySystemStartCommits) != "" {
				t.Fatal("start commits recorded despite the error")
			}
		})
	}
}

func TestRecordStartCommits_SiblingPathIsNotOverlap(t *testing.T) {
	cfg := characterizeConfig()
	cfg.ExternalSystems["erp"] = projectconfig.ExternalSystem{Stub: projectconfig.ExternalSpec{Path: "system/monolith2"}}
	repo := gitRepo(t, baseRepoFiles)
	a := newActions(Deps{Config: cfg, RepoPath: repo, Stdout: &bytes.Buffer{}})
	recordedCtx(t, a)
}

func TestRecordStartCommits_MultitierRecordsBothTiers(t *testing.T) {
	cfg := characterizeConfig()
	cfg.System = projectconfig.System{
		Architecture:    projectconfig.ArchMultitier,
		Backend:         projectconfig.TierSpec{Path: "system/backend", Repo: "acme/shop"},
		Frontend:        projectconfig.TierSpec{Path: "system/frontend", Repo: "acme/shop"},
		DbMigrationPath: "system/db/migrations",
	}
	repo := gitRepo(t, baseRepoFiles)
	a := newActions(Deps{Config: cfg, RepoPath: repo, Stdout: &bytes.Buffer{}})
	repos, err := recordedSystemRepos(recordedCtx(t, a))
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || strings.Join(repos[0].Paths, ",") != "system/backend,system/frontend,system/db/migrations" {
		t.Fatalf("repos = %+v", repos)
	}
}

func TestCheckSystemUnchanged(t *testing.T) {
	cases := []struct {
		name    string
		change  func(t *testing.T, repo string)
		want    bool
		offends string
	}{
		{"untouched", func(*testing.T, string) {}, true, ""},
		{"committed system edit", func(t *testing.T, repo string) {
			writeRepoFile(t, repo, "system/monolith/App.java", "class App { int x; }\n")
			commitAll(t, repo, "edit")
		}, false, "system/monolith/App.java"},
		{"uncommitted system edit", func(t *testing.T, repo string) {
			writeRepoFile(t, repo, "system/monolith/App.java", "class App { int y; }\n")
		}, false, "system/monolith/App.java"},
		{"untracked system file", func(t *testing.T, repo string) {
			writeRepoFile(t, repo, "system/monolith/pkg/New.java", "class New {}\n")
		}, false, "system/monolith/pkg/New.java"},
		{"migration added and committed", func(t *testing.T, repo string) {
			writeRepoFile(t, repo, "system/db/migrations/V2__more.sql", "alter table t add c int;\n")
			commitAll(t, repo, "migration")
		}, false, "system/db/migrations/V2__more.sql"},
		{"stand-in and tests edited", func(t *testing.T, repo string) {
			writeRepoFile(t, repo, "external-systems/stubs/mappings/erp.json", "{\"x\":1}\n")
			writeRepoFile(t, repo, "system-test/at/Another.java", "class Another {}\n")
			commitAll(t, repo, "stubs")
			writeRepoFile(t, repo, "external-systems/stubs/mappings/new.json", "{}\n")
		}, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := gitRepo(t, baseRepoFiles)
			a := newActions(Deps{Config: characterizeConfig(), RepoPath: repo, Stdout: &bytes.Buffer{}})
			ctx := recordedCtx(t, a)
			tc.change(t, repo)
			if out := a.checkSystemUnchanged(ctx); out.Err != nil {
				t.Fatal(out.Err)
			}
			if got := ctx.Get(CtxKeySystemUnchanged); got != tc.want {
				t.Fatalf("system-unchanged = %v, want %v (files %q)", got, tc.want, ctx.GetString(CtxKeySystemChangedFiles))
			}
			if got := ctx.GetString(CtxKeySystemChangedFiles); got != tc.offends {
				t.Errorf("system-changed-files = %q, want %q", got, tc.offends)
			}
		})
	}
}

func TestCheckSystemUnchanged_GitFailureIsHardError(t *testing.T) {
	repo := gitRepo(t, baseRepoFiles)
	a := newActions(Deps{Config: characterizeConfig(), RepoPath: repo, Stdout: &bytes.Buffer{}})
	ctx := recordedCtx(t, a)
	ctx.Set(CtxKeySystemStartCommits, `[{"repo":"acme/shop","commit":"0000000000000000000000000000000000000000","paths":["system/monolith"]}]`)
	if out := a.checkSystemUnchanged(ctx); out.Err == nil {
		t.Fatal("an unknown start commit must be a hard error, not unchanged")
	}
	if _, ok := ctx.State[CtxKeySystemUnchanged]; ok {
		t.Fatal("system-unchanged stamped despite the git failure")
	}
}

func TestCheckSystemUnchanged_NotRecordedIsHardError(t *testing.T) {
	a := newActions(Deps{Config: characterizeConfig(), Git: newFakeRunner(t, "git"), Stdout: &bytes.Buffer{}})
	if out := a.checkSystemUnchanged(statemachine.NewContext()); out.Err == nil {
		t.Fatal("want an error without recorded start commits")
	}
}

func TestCheckNoAssertionRemoved(t *testing.T) {
	cases := []struct {
		name   string
		change func(t *testing.T, repo string)
		want   bool
		hit    string
	}{
		{"assertion line removed", func(t *testing.T, repo string) {
			writeRepoFile(t, repo, "system-test/at/OrderTest.java", "class OrderTest {\n  void t() {\n    order.place();\n  }\n}\n")
			commitAll(t, repo, "weaken")
		}, false, "system-test/at/OrderTest.java:4: assertThat(order.total()).isEqualTo(10);"},
		{"uncommitted DSL should removed", func(t *testing.T, repo string) {
			writeRepoFile(t, repo, "system-test/dsl/core/OrderDsl.java", "class OrderDsl {\n  void v() {\n    log(\"x\");\n  }\n}\n")
		}, false, "system-test/dsl/core/OrderDsl.java:3: result.shouldSucceed();"},
		{"non-assertion removed", func(t *testing.T, repo string) {
			writeRepoFile(t, repo, "system-test/dsl/core/OrderDsl.java", "class OrderDsl {\n  void v() {\n    result.shouldSucceed();\n  }\n}\n")
			commitAll(t, repo, "tidy")
		}, true, ""},
		{"assertion moved to another file", func(t *testing.T, repo string) {
			writeRepoFile(t, repo, "system-test/at/OrderTest.java", "class OrderTest {\n  void t() {\n    order.place();\n  }\n}\n")
			writeRepoFile(t, repo, "system-test/at/OrderTotalTest.java", "class OrderTotalTest {\n    assertThat(order.total()).isEqualTo(10);\n}\n")
			commitAll(t, repo, "split")
		}, true, ""},
		{"assertion removed outside the test paths", func(t *testing.T, repo string) {
			writeRepoFile(t, repo, "system/monolith/App.java", "class App {}\n")
		}, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := gitRepo(t, baseRepoFiles)
			a := newActions(Deps{Config: characterizeConfig(), RepoPath: repo, Stdout: &bytes.Buffer{}})
			ctx := recordedCtx(t, a)
			start := ctx.GetString(CtxKeyAssertionCheckedCommit)
			tc.change(t, repo)
			if out := a.checkNoAssertionRemoved(ctx); out.Err != nil {
				t.Fatal(out.Err)
			}
			if got := ctx.Get(CtxKeyNoAssertionRemoved); got != tc.want {
				t.Fatalf("no-assertion-removed = %v, want %v (hits %q)", got, tc.want, ctx.GetString(CtxKeyRemovedAssertions))
			}
			if got := ctx.GetString(CtxKeyRemovedAssertions); got != tc.hit {
				t.Errorf("removed-assertions = %q, want %q", got, tc.hit)
			}
			head := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
			wantChecked := start
			if tc.want {
				wantChecked = head
			}
			if got := ctx.GetString(CtxKeyAssertionCheckedCommit); got != wantChecked {
				t.Errorf("assertion-checked-commit = %q, want %q", got, wantChecked)
			}
		})
	}
}

func TestCheckNoAssertionRemoved_OnlySinceLastCheck(t *testing.T) {
	repo := gitRepo(t, baseRepoFiles)
	a := newActions(Deps{Config: characterizeConfig(), RepoPath: repo, Stdout: &bytes.Buffer{}})
	ctx := recordedCtx(t, a)
	// A layer adds an assertion, passes the check, and a later fix of that
	// same layer's work is then diffed from the advanced commit.
	writeRepoFile(t, repo, "system-test/at/Extra.java", "class Extra {\n    assertEquals(1, x);\n}\n")
	commitAll(t, repo, "layer 1")
	if out := a.checkNoAssertionRemoved(ctx); out.Err != nil || ctx.Get(CtxKeyNoAssertionRemoved) != true {
		t.Fatalf("first check: err=%v pass=%v", out.Err, ctx.Get(CtxKeyNoAssertionRemoved))
	}
	writeRepoFile(t, repo, "system-test/at/Extra.java", "class Extra {\n}\n")
	commitAll(t, repo, "layer 2")
	if out := a.checkNoAssertionRemoved(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if ctx.Get(CtxKeyNoAssertionRemoved) != false || !strings.Contains(ctx.GetString(CtxKeyRemovedAssertions), "Extra.java:2: assertEquals(1, x);") {
		t.Fatalf("want the layer-2 removal reported, got %q", ctx.GetString(CtxKeyRemovedAssertions))
	}
}

func TestRemovedAssertions_PerLanguage(t *testing.T) {
	diff := func(line string) string {
		return "diff --git a/T b/T\nindex 1..2 100644\n--- a/T\n+++ b/T\n@@ -7 +6,0 @@\n-" + line + "\n"
	}
	cases := []struct {
		lang, line string
		hit        bool
	}{
		{projectconfig.LangJava, "    assertEquals(1, x);", true},
		{projectconfig.LangJava, "    assert x > 0;", true},
		{projectconfig.LangJava, "    verify(mock).call();", true},
		{projectconfig.LangJava, "    // assertThat(x)", false},
		{projectconfig.LangDotnet, "        Assert.Equal(1, x);", true},
		{projectconfig.LangDotnet, "        x.Should().Be(1);", true},
		{projectconfig.LangDotnet, "        result.ShouldSucceed();", true},
		{projectconfig.LangDotnet, "        var y = x;", false},
		{projectconfig.LangTypescript, "  expect(x).toBe(1);", true},
		{projectconfig.LangTypescript, "  assert.equal(x, 1);", true},
		{projectconfig.LangTypescript, "  await result.shouldFail();", true},
		{projectconfig.LangTypescript, "  const y = x;", false},
		{"", "  expect(x).toBe(1);", true}, // unknown language: union of patterns
		{"", "        Assert.True(x);", true},
	}
	for _, tc := range cases {
		hits := RemovedAssertions(tc.lang, diff(tc.line))
		if got := len(hits) > 0; got != tc.hit {
			t.Errorf("lang %q line %q: hit=%v, want %v", tc.lang, tc.line, got, tc.hit)
		}
		if tc.hit && len(hits) == 1 && !strings.HasPrefix(hits[0], "T:7: ") {
			t.Errorf("hit %q, want T:7: prefix", hits[0])
		}
	}
}

func TestRecordStartCommits_RecordsSystemTestStartCommit(t *testing.T) {
	repo := gitRepo(t, baseRepoFiles)
	a := newActions(Deps{Config: characterizeConfig(), RepoPath: repo, Stdout: &bytes.Buffer{}})
	ctx := recordedCtx(t, a)
	head := strings.TrimSpace(runGit(t, repo, "rev-parse", "HEAD"))
	if got := ctx.GetString(CtxKeySystemTestStartCommit); got != head {
		t.Fatalf("system-test-start-commit = %q, want %s", got, head)
	}
	// Unlike assertion-checked-commit it never advances, and a resume keeps it.
	writeRepoFile(t, repo, "system-test/at/New.java", "class New {}\n")
	commitAll(t, repo, "layer")
	if out := a.checkNoAssertionRemoved(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if out := a.recordStartCommits(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if got := ctx.GetString(CtxKeySystemTestStartCommit); got != head {
		t.Fatalf("system-test-start-commit moved to %q, want %s", got, head)
	}
}

func standInCtx(t *testing.T, a actions) *statemachine.Context {
	t.Helper()
	ctx := recordedCtx(t, a)
	if out := a.snapshotStandIns(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	return ctx
}

func TestCheckStandInsChanged(t *testing.T) {
	repo := gitRepo(t, baseRepoFiles)
	a := newActions(Deps{Config: characterizeConfig(), RepoPath: repo, Stdout: &bytes.Buffer{}})

	// A test-only fix changes no stand-in.
	ctx := standInCtx(t, a)
	writeRepoFile(t, repo, "system-test/at/OrderTest.java", "class OrderTest {}\n")
	if out := a.checkStandInsChanged(ctx); out.Err != nil || ctx.Get(CtxKeyStandInsChanged) != false {
		t.Fatalf("test-only fix: err=%v changed=%v", out.Err, ctx.Get(CtxKeyStandInsChanged))
	}

	ctx = standInCtx(t, a)
	writeRepoFile(t, repo, "external-systems/stubs/mappings/erp.json", "{\"changed\": true}\n")
	if out := a.checkStandInsChanged(ctx); out.Err != nil || ctx.Get(CtxKeyStandInsChanged) != true {
		t.Fatalf("stub fix: err=%v changed=%v", out.Err, ctx.Get(CtxKeyStandInsChanged))
	}
}

// Gap 5: the fixer edits a stub, its output fails validation, and the nested
// fix dispatch re-snapshots the working tree (execute-agent's
// pre-agent-fingerprint) before fixing the output. The stub edit predates
// that snapshot but must still trigger the restart.
func TestCheckStandInsChanged_SurvivesNestedReSnapshot(t *testing.T) {
	repo := gitRepo(t, baseRepoFiles)
	a := newActions(Deps{Config: characterizeConfig(), RepoPath: repo, Stdout: &bytes.Buffer{}})
	ctx := standInCtx(t, a)
	if out := a.snapshotWorkingTree(ctx); out.Err != nil { // outer execute-agent
		t.Fatal(out.Err)
	}
	writeRepoFile(t, repo, "external-systems/stubs/mappings/erp.json", "{\"changed\": true}\n")
	if out := a.snapshotWorkingTree(ctx); out.Err != nil { // nested fix re-snapshot
		t.Fatal(out.Err)
	}
	writeRepoFile(t, repo, "system-test/at/OrderTest.java", "class OrderTest {}\n")
	if out := a.checkStandInsChanged(ctx); out.Err != nil || ctx.Get(CtxKeyStandInsChanged) != true {
		t.Fatalf("err=%v changed=%v, want the earlier stub edit counted", out.Err, ctx.Get(CtxKeyStandInsChanged))
	}
}

func TestCheckStandInsChanged_NoBaselineIsWiringError(t *testing.T) {
	repo := gitRepo(t, baseRepoFiles)
	a := newActions(Deps{Config: characterizeConfig(), RepoPath: repo, Stdout: &bytes.Buffer{}})
	ctx := recordedCtx(t, a)
	if out := a.snapshotWorkingTree(ctx); out.Err != nil {
		t.Fatal(out.Err)
	}
	if out := a.checkStandInsChanged(ctx); out.Err == nil || !strings.Contains(out.Err.Error(), CtxKeyStandInBaseline) {
		t.Fatalf("want a missing-baseline error, got %v", out.Err)
	}
	// A declined dispatch or no stand-ins: nothing to check, no baseline needed.
	ctx.Set("approval-outcome", "rejected")
	if out := a.checkStandInsChanged(ctx); out.Err != nil || ctx.Get(CtxKeyStandInsChanged) != false {
		t.Fatalf("rejected: err=%v changed=%v", out.Err, ctx.Get(CtxKeyStandInsChanged))
	}
}
