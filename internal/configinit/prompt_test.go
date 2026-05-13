package configinit

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
)

// stubExistenceChecks replaces config.CheckOwnerExistsFn and
// config.CheckProjectExistsFn with the supplied functions for the
// duration of the test. Defaults that pass everything are used when nil
// is passed. Returned restore func reverses the swap on t.Cleanup.
func stubExistenceChecks(t *testing.T, owner func(string) error, project func(string) error) {
	t.Helper()
	prevOwner := config.CheckOwnerExistsFn
	prevProject := config.CheckProjectExistsFn
	if owner == nil {
		owner = func(string) error { return nil }
	}
	if project == nil {
		project = func(string) error { return nil }
	}
	config.CheckOwnerExistsFn = owner
	config.CheckProjectExistsFn = project
	t.Cleanup(func() {
		config.CheckOwnerExistsFn = prevOwner
		config.CheckProjectExistsFn = prevProject
	})
}

// monolithAnswers returns the slim interactive script for a monolith run.
// The order follows Prompt's question sequence: owner → repo →
// system-name → arch → repo-strategy → lang → project-url → license.
func monolithAnswers() []string {
	return []string{
		"acme",                                    // owner
		"page-turner",                             // repo
		"Page Turner",                             // system-name
		"monolith",                                // arch
		"monorepo",                                // repo-strategy
		"java",                                    // monolith-lang
		"https://github.com/orgs/acme/projects/1", // project-url
		"mit",                                     // license
	}
}

func multitierAnswers() []string {
	return []string{
		"acme",                                    // owner
		"page-turner",                             // repo
		"Page Turner",                             // system-name
		"multitier",                               // arch
		"multirepo",                               // repo-strategy
		"dotnet",                                  // backend-lang
		"https://github.com/orgs/acme/projects/2", // project-url
		"mit",                                     // license
	}
}

// script joins answers with "\n" and appends a trailing newline so the
// bufio.Reader inside Prompt sees one line per answer.
func script(lines []string) io.Reader {
	return strings.NewReader(strings.Join(lines, "\n") + "\n")
}

func TestPrompt_MonolithHappyPath(t *testing.T) {
	stubExistenceChecks(t, nil, nil)
	var out bytes.Buffer
	f, err := Prompt(script(monolithAnswers()), &out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if f.Owner != "acme" || f.Repo != "page-turner" {
		t.Errorf("owner/repo: got %q/%q; want acme/page-turner", f.Owner, f.Repo)
	}
	if f.Arch != "monolith" || f.Lang != "java" {
		t.Errorf("arch/lang: got %q/%q", f.Arch, f.Lang)
	}
	// Tier paths are intentionally left empty by Prompt — defaulting
	// happens downstream in config.resolvePathFlagsForYAML so the
	// flag-driven and interactive paths share one definition of "flat
	// layout". End-to-end YAML output is covered by ensure_test.go.
	if f.SystemPath != "" || f.SystemTestPath != "" || f.StubsPath != "" || f.SimulatorsPath != "" {
		t.Errorf("path fields should be empty post-Prompt; got system=%q system-test=%q stubs=%q sims=%q",
			f.SystemPath, f.SystemTestPath, f.StubsPath, f.SimulatorsPath)
	}
	if f.ProjectURL != "https://github.com/orgs/acme/projects/1" {
		t.Errorf("ProjectURL: got %q", f.ProjectURL)
	}
}

func TestPrompt_MultitierHappyPath(t *testing.T) {
	stubExistenceChecks(t, nil, nil)
	var out bytes.Buffer
	f, err := Prompt(script(multitierAnswers()), &out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if f.Arch != "multitier" {
		t.Errorf("Arch: got %q", f.Arch)
	}
	if f.BackendLang != "dotnet" {
		t.Errorf("BackendLang: got %q", f.BackendLang)
	}
	if f.FrontendLang != "typescript" {
		t.Errorf("FrontendLang should be hardcoded to typescript, got %q", f.FrontendLang)
	}
	// Tier paths are intentionally left empty by Prompt — defaulting
	// happens in config.resolvePathFlagsForYAML.
	if f.BackendPath != "" || f.FrontendPath != "" {
		t.Errorf("multitier path fields should be empty post-Prompt; got backend=%q frontend=%q", f.BackendPath, f.FrontendPath)
	}
	if f.ProjectURL != "https://github.com/orgs/acme/projects/2" {
		t.Errorf("ProjectURL: got %q", f.ProjectURL)
	}
}

// TestPrompt_ReAsksOnBadOwner — invalid owner format is rejected and the
// prompt re-asks for that field.
func TestPrompt_ReAsksOnBadOwner(t *testing.T) {
	stubExistenceChecks(t, nil, nil)
	// Prepend "-bad" before the valid owner; everything else is the
	// happy-path script.
	lines := append([]string{"-bad"}, monolithAnswers()...)
	var out bytes.Buffer
	f, err := Prompt(script(lines), &out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if f.Owner != "acme" {
		t.Errorf("Owner: got %q (should have re-asked past the bad value)", f.Owner)
	}
	if !strings.Contains(out.String(), "owner cannot start or end with a hyphen") {
		t.Errorf("output should surface the validator error, got:\n%s", out.String())
	}
}

// TestPrompt_ReAsksOnMissingOwner — owner passes format validation but
// the GitHub existence check fails. Mirrors the user-reported "sdgsgd
// went through" case; the prompt re-asks just the owner field instead
// of letting the bad value land in the YAML.
func TestPrompt_ReAsksOnMissingOwner(t *testing.T) {
	stubExistenceChecks(t,
		func(o string) error {
			if o == "ghost" {
				return fmt.Errorf("no GitHub user or organization named %q", o)
			}
			return nil
		},
		nil,
	)
	lines := append([]string{"ghost"}, monolithAnswers()...)
	var out bytes.Buffer
	f, err := Prompt(script(lines), &out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if f.Owner != "acme" {
		t.Errorf("Owner: got %q (should have re-asked past the missing owner)", f.Owner)
	}
	if !strings.Contains(out.String(), `no GitHub user or organization named "ghost"`) {
		t.Errorf("output should surface the existence error, got:\n%s", out.String())
	}
}

// TestPrompt_ReAsksOnBadArch — invalid arch value re-asks rather than
// aborting; arch gates the lang branch downstream. Arch sits at index 3
// in the script (after owner, repo, system-name).
func TestPrompt_ReAsksOnBadArch(t *testing.T) {
	stubExistenceChecks(t, nil, nil)
	answers := monolithAnswers()
	const archPos = 3
	lines := append([]string{}, answers[:archPos]...)
	lines = append(lines, "bogus")
	lines = append(lines, answers[archPos:]...)
	var out bytes.Buffer
	f, err := Prompt(script(lines), &out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if f.Arch != "monolith" {
		t.Errorf("Arch: got %q", f.Arch)
	}
	if !strings.Contains(out.String(), "must be 'monolith' or 'multitier'") {
		t.Errorf("output should surface the arch validator error, got:\n%s", out.String())
	}
}

// TestPrompt_ReAsksOnBadProjectURL — a non-https://github.com URL is
// rejected by ValidateProjectURLFormat; a syntactically-valid URL that
// fails the existence check is rejected by CheckProjectExists; both
// cases re-ask just the project-url field. project-url sits at index 6
// (after owner, repo, system-name, arch, repo-strategy, lang); license
// follows at index 7.
func TestPrompt_ReAsksOnBadProjectURL(t *testing.T) {
	stubExistenceChecks(t,
		nil,
		func(u string) error {
			if u == "https://github.com/orgs/acme/projects/999" {
				return fmt.Errorf("project acme/999 not found or not accessible")
			}
			return nil
		},
	)
	answers := monolithAnswers()
	const urlPos = 6
	lines := append([]string{}, answers[:urlPos]...)
	lines = append(lines,
		"https://gitlab.com/orgs/acme/projects/1",   // wrong host → format error
		"https://github.com/orgs/acme/projects/999", // format OK, existence fails via stub
	)
	lines = append(lines, answers[urlPos:]...)
	var out bytes.Buffer
	f, err := Prompt(script(lines), &out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if f.ProjectURL != "https://github.com/orgs/acme/projects/1" {
		t.Errorf("ProjectURL: got %q (should have re-asked past the bad values)", f.ProjectURL)
	}
	body := out.String()
	if !strings.Contains(body, "must be a https://github.com/") {
		t.Errorf("output should reject non-github URL, got:\n%s", body)
	}
	if !strings.Contains(body, "project acme/999 not found") {
		t.Errorf("output should reject non-existent project, got:\n%s", body)
	}
}

// TestPrompt_AllowsEmptyProjectURL — empty project URL is accepted and
// stored as empty in RawFlags; `gh optivem init` Path A will then
// auto-create the project board on first run. Empty input does NOT
// trigger CheckProjectExists.
func TestPrompt_AllowsEmptyProjectURL(t *testing.T) {
	checkProjectCalls := 0
	stubExistenceChecks(t,
		nil,
		func(u string) error {
			checkProjectCalls++
			return fmt.Errorf("stub should not have been called: %q", u)
		},
	)
	answers := monolithAnswers()
	const urlPos = 6
	lines := append([]string{}, answers[:urlPos]...)
	lines = append(lines, "") // empty project URL
	lines = append(lines, answers[urlPos+1:]...)
	var out bytes.Buffer
	f, err := Prompt(script(lines), &out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if f.ProjectURL != "" {
		t.Errorf("ProjectURL: got %q, want empty (auto-create path)", f.ProjectURL)
	}
	if checkProjectCalls != 0 {
		t.Errorf("CheckProjectExists should not have been called on empty URL; got %d call(s)", checkProjectCalls)
	}
}

// TestPrompt_EOFReturnsError — when stdin closes mid-session, Prompt
// surfaces the EOF rather than spinning. EnsureExists's caller treats
// this as "fall back to the terse error".
func TestPrompt_EOFReturnsError(t *testing.T) {
	stubExistenceChecks(t, nil, nil)
	// Only a partial script: owner + repo, then EOF.
	partial := strings.Join([]string{"acme", "page-turner"}, "\n") + "\n"
	var out bytes.Buffer
	_, err := Prompt(strings.NewReader(partial), &out)
	if err == nil {
		t.Fatal("want error on EOF, got nil")
	}
}
