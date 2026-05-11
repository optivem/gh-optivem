package configinit

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// monolithAnswers returns the sequence of newline-terminated lines that
// drive Prompt through a full monolith run. Built as a slice and joined
// with "\n" so a test can mutate one position (e.g. inject a bad value
// to exercise the re-ask path) without restating the whole script.
func monolithAnswers() []string {
	return []string{
		"acme",                            // owner
		"page-turner",                     // repo
		"monolith",                        // arch
		"monorepo",                        // repo-strategy
		"java",                            // monolith-lang
		"system-test/java",                // system-test-path
		"external-systems/external-stub",  // stubs-path
		"external-systems/external-real-sim", // simulators-path
		"system/monolith/java",            // system-path
		"https://github.com/orgs/acme/projects/1", // project-url
	}
}

func multitierAnswers() []string {
	return []string{
		"acme",                                       // owner
		"page-turner",                                // repo
		"multitier",                                  // arch
		"multirepo",                                  // repo-strategy
		"dotnet",                                     // backend-lang
		"react",                                      // frontend-lang
		"system-test/typescript",                     // system-test-path
		"external-systems/external-stub",             // stubs-path
		"external-systems/external-real-sim",         // simulators-path
		"system/multitier/backend-dotnet",            // backend-path
		"system/multitier/frontend-react",            // frontend-path
		"",                                           // project-url (skipped)
	}
}

// script joins answers with "\n" and appends a trailing newline so the
// bufio.Reader inside Prompt sees one line per answer.
func script(lines []string) io.Reader {
	return strings.NewReader(strings.Join(lines, "\n") + "\n")
}

func TestPrompt_MonolithHappyPath(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	f, err := Prompt(script(monolithAnswers()), &out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if f.Owner != "acme" {
		t.Errorf("Owner: got %q", f.Owner)
	}
	if f.Arch != "monolith" || f.Lang != "java" {
		t.Errorf("arch/lang: got %q/%q", f.Arch, f.Lang)
	}
	if f.SystemPath != "system/monolith/java" {
		t.Errorf("SystemPath: got %q", f.SystemPath)
	}
	if f.ProjectURL != "https://github.com/orgs/acme/projects/1" {
		t.Errorf("ProjectURL: got %q", f.ProjectURL)
	}
}

func TestPrompt_MultitierHappyPath(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	f, err := Prompt(script(multitierAnswers()), &out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if f.Arch != "multitier" {
		t.Errorf("Arch: got %q", f.Arch)
	}
	if f.BackendLang != "dotnet" || f.FrontendLang != "react" {
		t.Errorf("langs: got backend=%q frontend=%q", f.BackendLang, f.FrontendLang)
	}
	if f.BackendPath != "system/multitier/backend-dotnet" || f.FrontendPath != "system/multitier/frontend-react" {
		t.Errorf("paths: got backend=%q frontend=%q", f.BackendPath, f.FrontendPath)
	}
	if f.ProjectURL != "" {
		t.Errorf("ProjectURL should be empty (Enter to skip), got %q", f.ProjectURL)
	}
}

// TestPrompt_ReAsksOnBadOwner — an invalid GitHub owner format is rejected
// and the prompt re-asks for that field while keeping everything else.
func TestPrompt_ReAsksOnBadOwner(t *testing.T) {
	t.Parallel()
	answers := monolithAnswers()
	// Insert a bad owner ("-bad") before the valid "acme". ValidateOwnerFormat
	// rejects leading-hyphen names.
	lines := append([]string{"-bad"}, answers...)
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
// aborting; arch gates the lang/path branches downstream.
func TestPrompt_ReAsksOnBadArch(t *testing.T) {
	t.Parallel()
	answers := monolithAnswers()
	// Inject a bad arch before the valid "monolith" (position 2).
	lines := append([]string{}, answers[:2]...)
	lines = append(lines, "bogus")
	lines = append(lines, answers[2:]...)
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

// TestPrompt_EmptyProjectURLAccepted — Enter on --project-url leaves the
// field empty without re-asking. Matches the flag's optional semantics.
func TestPrompt_EmptyProjectURLAccepted(t *testing.T) {
	t.Parallel()
	answers := monolithAnswers()
	answers[len(answers)-1] = "" // project-url
	var out bytes.Buffer
	f, err := Prompt(script(answers), &out)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if f.ProjectURL != "" {
		t.Errorf("ProjectURL: got %q, want empty", f.ProjectURL)
	}
}

// TestPrompt_EOFReturnsError — when stdin closes mid-session, Prompt
// surfaces the EOF rather than spinning. EnsureExists's caller treats
// this as "fall back to the terse error".
func TestPrompt_EOFReturnsError(t *testing.T) {
	t.Parallel()
	// Only a partial script: owner + repo + arch, then EOF.
	partial := strings.Join([]string{"acme", "page-turner", "monolith"}, "\n") + "\n"
	var out bytes.Buffer
	_, err := Prompt(strings.NewReader(partial), &out)
	if err == nil {
		t.Fatal("want error on EOF, got nil")
	}
}
