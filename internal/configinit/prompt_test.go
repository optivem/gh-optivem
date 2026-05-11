package configinit

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// stubInference replaces InferOwnerRepo's underlying git shell-out with
// a deterministic value for the duration of one test. Returns a cleanup
// closure registered via t.Cleanup so the original is restored even on
// test failure.
func stubInference(t *testing.T, raw string, ok bool) {
	t.Helper()
	orig := runGitRemote
	t.Cleanup(func() { runGitRemote = orig })
	runGitRemote = func(string) (string, error) {
		if !ok {
			return "", io.EOF // any non-nil error triggers the ok=false path
		}
		return raw, nil
	}
}

// monolithAnswers returns the slim interactive script for a monolith run
// when InferOwnerRepo succeeds (owner/repo do not prompt). The order
// follows Prompt's question sequence: system-name → arch → repo-strategy
// → lang → project-url → license.
func monolithAnswers() []string {
	return []string{
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
		"Page Turner",                             // system-name
		"multitier",                               // arch
		"multirepo",                               // repo-strategy
		"dotnet",                                  // backend-lang
		"https://github.com/orgs/acme/projects/2", // project-url
		"mit",                                     // license
	}
}

// noRemoteAnswers is the script when inference fails — the prompt asks
// owner and repo before the rest.
func noRemoteAnswers() []string {
	return append([]string{
		"acme",        // owner
		"page-turner", // repo
	}, monolithAnswers()...)
}

// script joins answers with "\n" and appends a trailing newline so the
// bufio.Reader inside Prompt sees one line per answer.
func script(lines []string) io.Reader {
	return strings.NewReader(strings.Join(lines, "\n") + "\n")
}

func TestPrompt_MonolithHappyPath(t *testing.T) {
	stubInference(t, "https://github.com/acme/page-turner.git", true)
	var out bytes.Buffer
	f, err := Prompt(script(monolithAnswers()), &out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if f.Owner != "acme" || f.Repo != "page-turner" {
		t.Errorf("owner/repo: got %q/%q; expected inferred acme/page-turner", f.Owner, f.Repo)
	}
	if f.Arch != "monolith" || f.Lang != "java" {
		t.Errorf("arch/lang: got %q/%q", f.Arch, f.Lang)
	}
	if f.SystemPath != defaultSystemPath {
		t.Errorf("SystemPath: got %q, want default %q", f.SystemPath, defaultSystemPath)
	}
	if f.SystemTestPath != defaultSystemTestPath || f.StubsPath != defaultStubsPath || f.SimulatorsPath != defaultSimulatorsPath {
		t.Errorf("path defaults: got system-test=%q stubs=%q sims=%q", f.SystemTestPath, f.StubsPath, f.SimulatorsPath)
	}
	if f.ProjectURL != "https://github.com/orgs/acme/projects/1" {
		t.Errorf("ProjectURL: got %q", f.ProjectURL)
	}
	if !strings.Contains(out.String(), "Inferred owner=acme, repo=page-turner from git remote origin") {
		t.Errorf("output should announce inferred values, got:\n%s", out.String())
	}
}

func TestPrompt_MultitierHappyPath(t *testing.T) {
	stubInference(t, "https://github.com/acme/page-turner.git", true)
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
	if f.FrontendLang != "react" {
		t.Errorf("FrontendLang should be hardcoded to react, got %q", f.FrontendLang)
	}
	if f.BackendPath != defaultBackendPath || f.FrontendPath != defaultFrontendPath {
		t.Errorf("multitier path defaults: got backend=%q frontend=%q", f.BackendPath, f.FrontendPath)
	}
	if f.ProjectURL != "https://github.com/orgs/acme/projects/2" {
		t.Errorf("ProjectURL: got %q", f.ProjectURL)
	}
}

// TestPrompt_FallsBackToPromptWhenNoRemote — inference returns ok=false,
// owner+repo are prompted using the existing validators.
func TestPrompt_FallsBackToPromptWhenNoRemote(t *testing.T) {
	stubInference(t, "", false)
	var out bytes.Buffer
	f, err := Prompt(script(noRemoteAnswers()), &out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if f.Owner != "acme" || f.Repo != "page-turner" {
		t.Errorf("owner/repo: got %q/%q; want prompted acme/page-turner", f.Owner, f.Repo)
	}
	if strings.Contains(out.String(), "Inferred owner") {
		t.Errorf("output should NOT announce inferred values on no-remote path, got:\n%s", out.String())
	}
}

// TestPrompt_ReAsksOnBadOwner — invalid owner format on the no-remote
// fallback path is rejected and the prompt re-asks for that field.
func TestPrompt_ReAsksOnBadOwner(t *testing.T) {
	stubInference(t, "", false)
	// Prepend "-bad" before the valid owner; everything else is the
	// happy-path no-remote script.
	lines := append([]string{"-bad"}, noRemoteAnswers()...)
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

// TestPrompt_ReAsksOnBadArch — invalid arch value re-asks rather than
// aborting; arch gates the lang branch downstream. Arch sits at index 1
// in the script (after system-name).
func TestPrompt_ReAsksOnBadArch(t *testing.T) {
	stubInference(t, "https://github.com/acme/page-turner.git", true)
	answers := monolithAnswers()
	// Inject a bad arch between system-name (index 0) and the valid "monolith" (index 1).
	lines := append([]string{}, answers[:1]...)
	lines = append(lines, "bogus")
	lines = append(lines, answers[1:]...)
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

// TestPrompt_ReAsksOnBadProjectURL — empty input re-asks (URL is
// mandatory), and a non-https://github.com URL is also rejected.
// Mirrors the Validate rule in projectconfig: the pipeline cannot
// operate without a project board. project-url sits at index 4 (after
// system-name, arch, repo-strategy, lang); license follows at index 5.
func TestPrompt_ReAsksOnBadProjectURL(t *testing.T) {
	stubInference(t, "https://github.com/acme/page-turner.git", true)
	answers := monolithAnswers()
	const urlPos = 4
	lines := append([]string{}, answers[:urlPos]...)
	lines = append(lines, "", "https://gitlab.com/orgs/acme/projects/1")
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
	if !strings.Contains(body, "value cannot be empty") {
		t.Errorf("output should re-ask on empty URL, got:\n%s", body)
	}
	if !strings.Contains(body, "must be a https://github.com/") {
		t.Errorf("output should reject non-github URL, got:\n%s", body)
	}
}

// TestPrompt_EOFReturnsError — when stdin closes mid-session, Prompt
// surfaces the EOF rather than spinning. EnsureExists's caller treats
// this as "fall back to the terse error".
func TestPrompt_EOFReturnsError(t *testing.T) {
	stubInference(t, "https://github.com/acme/page-turner.git", true)
	// Only a partial script: arch + repo-strategy, then EOF.
	partial := strings.Join([]string{"monolith", "monorepo"}, "\n") + "\n"
	var out bytes.Buffer
	_, err := Prompt(strings.NewReader(partial), &out)
	if err == nil {
		t.Fatal("want error on EOF, got nil")
	}
}
