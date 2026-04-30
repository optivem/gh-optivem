//go:build system

package config

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var (
	binaryPath string
	// runPrefix scopes this test run's repos + SonarCloud projects so the
	// post-run sweep only deletes what this run created — not orphans from
	// prior failed runs that share the bare "test-app-" prefix.
	runPrefix string
)

func testOwner() string {
	owner := os.Getenv("TEST_OWNER")
	if owner == "" {
		panic("TEST_OWNER environment variable is required")
	}
	return owner
}

var baseArgs = []string{
	"--system-name", "Sky Travel",
}

// randomRepoName returns "<runPrefix><16 hex>" for a fresh repo per subtest, so
// repeated/parallel runs don't collide on GitHub. runPrefix is a per-run value
// like "test-app-<runid>-" that scopes post-run cleanup to this run's repos.
func randomRepoName() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return runPrefix + hex.EncodeToString(b)
}

func cleanupFlags() []string {
	if os.Getenv("TEST_NO_CLEANUP") == "1" {
		return []string{"--keep-local"}
	}
	return nil
}

func verifyFlags() []string {
	var flags []string
	if level := os.Getenv("TEST_VERIFY_LEVEL"); level != "" {
		flags = append(flags, "--verify-level", level)
	}
	if os.Getenv("TEST_NO_LEGACY") == "true" {
		flags = append(flags, "--no-legacy")
	}
	if os.Getenv("TEST_NO_LOCAL_TESTS") == "true" {
		flags = append(flags, "--no-local-tests")
	}
	if os.Getenv("TEST_NO_LOCAL_SONAR") == "true" {
		flags = append(flags, "--no-local-sonar")
	}
	if tag := os.Getenv("TEST_SHOP_TAG"); tag != "" {
		flags = append(flags, "--shop-ref", tag)
	}
	return flags
}

func withBase(extra ...string) []string {
	args := []string{"--owner", testOwner()}
	args = append(args, baseArgs...)
	args = append(args, "--repo", randomRepoName())
	args = append(args, extra...)
	args = append(args, cleanupFlags()...)
	args = append(args, verifyFlags()...)
	return args
}

// loadEnvFile loads key=value pairs from a .env file into the environment.
// Existing environment variables take precedence (are not overwritten).
func loadEnvFile(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		if os.Getenv(k) == "" {
			os.Setenv(k, v)
		}
	}
}

func TestMain(m *testing.M) {
	// Find the module root (2 levels up from internal/config/)
	modRoot, _ := filepath.Abs(filepath.Join("..", ".."))

	// Load .env defaults (env vars take precedence)
	loadEnvFile(filepath.Join(modRoot, ".env"))

	// Per-run prefix: every repo + SonarCloud project created during this run
	// starts with "test-app-<runid>-". Post-run cleanup filters on exactly this
	// prefix so it only deletes what this run created.
	rid := make([]byte, 4)
	if _, err := rand.Read(rid); err != nil {
		panic("cannot generate run id: " + err.Error())
	}
	runPrefix = "test-app-" + hex.EncodeToString(rid) + "-"

	// Build the binary once before all system tests
	dir, err := os.MkdirTemp("", "gh-optivem-test-*")
	if err != nil {
		panic("cannot create temp dir: " + err.Error())
	}
	defer os.RemoveAll(dir)

	bin := filepath.Join(dir, "gh-optivem")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = modRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic("failed to build binary: " + err.Error() + "\n" + string(out))
	}

	binaryPath = bin
	code := m.Run()

	// Valid-config tests leave behind GitHub repos + SonarCloud projects; the CLI
	// no longer deletes those itself. Sweep them via cleanup-orphans.sh, scoped
	// to runPrefix so the sweep only touches what this run created. Skip the
	// sweep when any test failed so the scaffolded repo is preserved for
	// debugging — orphans from failed runs can be cleared manually by running
	// cleanup-orphans.sh with its default (bare "test-app-") prefix. Opt out
	// entirely with TEST_NO_CLEANUP=1.
	if code == 0 && os.Getenv("TEST_NO_CLEANUP") != "1" {
		cmd := exec.Command("bash", "scripts/cleanup-orphans.sh",
			"--owner", testOwner(),
			"--repos", "--sonar",
			"--prefixes", runPrefix,
			"--delete",
		)
		cmd.Dir = modRoot
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: post-test cleanup failed: %v\n", err)
		}
	}

	os.Exit(code)
}

// runCLI runs the binary and returns output + exit code.
// Valid config tests leave the remote repos + SonarCloud projects behind; on a
// green suite, TestMain runs scripts/cleanup-orphans.sh scoped to this run's
// prefix (test-app-<runid>-) to delete only what this run created. On failure
// nothing is deleted so the scaffolded repo is available for debugging.
// The local scaffold dir is deleted by the CLI on success.
// Streams the subprocess's stdout/stderr to os.Stderr as it runs so the created
// repo name (and other progress) appears live in the log instead of only after
// the subtest finishes. Still returns the captured output for assertions.
func runCLI(t *testing.T, args ...string) (string, int) {
	t.Helper()

	// Prepend "init" subcommand
	fullArgs := append([]string{"init"}, args...)
	cmd := exec.Command(binaryPath, fullArgs...)

	var buf bytes.Buffer
	tee := io.MultiWriter(&buf, os.Stderr)
	cmd.Stdout = tee
	cmd.Stderr = tee
	err := cmd.Run()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error running CLI: %v", err)
		}
	}
	return buf.String(), exitCode
}

func TestValidMonolithConfigurations(t *testing.T) {
	tests := []struct {
		name, arch, repoStrategy, monolithLang, testLang string
	}{
		//             arch        repoStrategy  monolithLang  testLang
		{"monolith monorepo java java",         "monolith", "monorepo",  "java",       "java"},
		{"monolith monorepo java dotnet",       "monolith", "monorepo",  "java",       "dotnet"},
		{"monolith monorepo java typescript",   "monolith", "monorepo",  "java",       "typescript"},
		{"monolith monorepo dotnet java",       "monolith", "monorepo",  "dotnet",     "java"},
		{"monolith monorepo dotnet dotnet",     "monolith", "monorepo",  "dotnet",     "dotnet"},
		{"monolith monorepo dotnet typescript", "monolith", "monorepo",  "dotnet",     "typescript"},
		{"monolith monorepo ts java",           "monolith", "monorepo",  "typescript", "java"},
		{"monolith monorepo ts dotnet",         "monolith", "monorepo",  "typescript", "dotnet"},
		{"monolith monorepo ts typescript",     "monolith", "monorepo",  "typescript", "typescript"},
		{"monolith multirepo java java",         "monolith", "multirepo", "java",       "java"},
		{"monolith multirepo java dotnet",       "monolith", "multirepo", "java",       "dotnet"},
		{"monolith multirepo java typescript",   "monolith", "multirepo", "java",       "typescript"},
		{"monolith multirepo dotnet java",       "monolith", "multirepo", "dotnet",     "java"},
		{"monolith multirepo dotnet dotnet",     "monolith", "multirepo", "dotnet",     "dotnet"},
		{"monolith multirepo dotnet typescript", "monolith", "multirepo", "dotnet",     "typescript"},
		{"monolith multirepo ts java",           "monolith", "multirepo", "typescript", "java"},
		{"monolith multirepo ts dotnet",         "monolith", "multirepo", "typescript", "dotnet"},
		{"monolith multirepo ts typescript",     "monolith", "multirepo", "typescript", "typescript"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := withBase(
				"--arch", tt.arch,
				"--repo-strategy", tt.repoStrategy,
				"--monolith-lang", tt.monolithLang,
				"--test-lang", tt.testLang,
			)
			out, exitCode := runCLI(t, args...)
			t.Log(out)
			if exitCode != 0 {
				t.Errorf("expected exit code 0, got %d", exitCode)
			}
		})
	}
}

func TestValidMultitierConfigurations(t *testing.T) {
	tests := []struct {
		name, arch, repoStrategy, backendLang, frontendLang, testLang string
	}{
		//              arch         repoStrategy  backendLang   frontendLang  testLang
		{"multitier monorepo java react java",         "multitier", "monorepo",  "java",       "react", "java"},
		{"multitier monorepo java react dotnet",       "multitier", "monorepo",  "java",       "react", "dotnet"},
		{"multitier monorepo java react typescript",   "multitier", "monorepo",  "java",       "react", "typescript"},
		{"multitier monorepo dotnet react java",       "multitier", "monorepo",  "dotnet",     "react", "java"},
		{"multitier monorepo dotnet react dotnet",     "multitier", "monorepo",  "dotnet",     "react", "dotnet"},
		{"multitier monorepo dotnet react typescript", "multitier", "monorepo",  "dotnet",     "react", "typescript"},
		{"multitier monorepo ts react java",           "multitier", "monorepo",  "typescript", "react", "java"},
		{"multitier monorepo ts react dotnet",         "multitier", "monorepo",  "typescript", "react", "dotnet"},
		{"multitier monorepo ts react typescript",     "multitier", "monorepo",  "typescript", "react", "typescript"},
		{"multitier multirepo java react java",         "multitier", "multirepo", "java",       "react", "java"},
		{"multitier multirepo java react dotnet",       "multitier", "multirepo", "java",       "react", "dotnet"},
		{"multitier multirepo java react typescript",   "multitier", "multirepo", "java",       "react", "typescript"},
		{"multitier multirepo dotnet react java",       "multitier", "multirepo", "dotnet",     "react", "java"},
		{"multitier multirepo dotnet react dotnet",     "multitier", "multirepo", "dotnet",     "react", "dotnet"},
		{"multitier multirepo dotnet react typescript", "multitier", "multirepo", "dotnet",     "react", "typescript"},
		{"multitier multirepo ts react java",           "multitier", "multirepo", "typescript", "react", "java"},
		{"multitier multirepo ts react dotnet",         "multitier", "multirepo", "typescript", "react", "dotnet"},
		{"multitier multirepo ts react typescript",     "multitier", "multirepo", "typescript", "react", "typescript"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := withBase(
				"--arch", tt.arch,
				"--repo-strategy", tt.repoStrategy,
				"--backend-lang", tt.backendLang,
				"--frontend-lang", tt.frontendLang,
				"--test-lang", tt.testLang,
			)
			out, exitCode := runCLI(t, args...)
			t.Log(out)
			if exitCode != 0 {
				t.Errorf("expected exit code 0, got %d", exitCode)
			}
		})
	}
}

func TestInvalidConfigurations(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "missing owner",
			args: []string{
				"--system-name", "Sky Travel",
				"--repo", "test-app",
				"--arch", "monolith",
				"--repo-strategy", "monorepo",
				"--monolith-lang", "java",
				"--dry-run",
			},
		},
		{
			name: "missing system-name",
			args: []string{
				"--owner", "testuser",
				"--repo", "test-app",
				"--arch", "monolith",
				"--repo-strategy", "monorepo",
				"--monolith-lang", "java",
				"--dry-run",
			},
		},
		{
			name: "missing repo",
			args: []string{
				"--owner", "testuser",
				"--system-name", "Sky Travel",
				"--arch", "monolith",
				"--repo-strategy", "monorepo",
				"--monolith-lang", "java",
				"--dry-run",
			},
		},
		{
			name: "missing arch",
			args: append(withBase("--repo-strategy", "monorepo", "--monolith-lang", "java"), "--dry-run"),
		},
		{
			name: "invalid arch",
			args: append(withBase("--arch", "invalid", "--repo-strategy", "monorepo", "--monolith-lang", "java"), "--dry-run"),
		},
		{
			name: "missing repo-strategy",
			args: append(withBase("--arch", "monolith", "--monolith-lang", "java"), "--dry-run"),
		},
		{
			name: "invalid repo-strategy",
			args: append(withBase("--arch", "monolith", "--repo-strategy", "invalid", "--monolith-lang", "java"), "--dry-run"),
		},
		{
			name: "monolith missing lang",
			args: append(withBase("--arch", "monolith", "--repo-strategy", "monorepo"), "--dry-run"),
		},
		{
			name: "monolith invalid lang",
			args: append(withBase("--arch", "monolith", "--repo-strategy", "monorepo", "--monolith-lang", "python"), "--dry-run"),
		},
		{
			name: "multitier missing backend-lang",
			args: append(withBase("--arch", "multitier", "--repo-strategy", "multirepo", "--frontend-lang", "react"), "--dry-run"),
		},
		{
			name: "multitier missing frontend-lang",
			args: append(withBase("--arch", "multitier", "--repo-strategy", "multirepo", "--backend-lang", "java"), "--dry-run"),
		},
		{
			name: "multitier invalid frontend-lang",
			args: append(withBase("--arch", "multitier", "--repo-strategy", "multirepo", "--backend-lang", "java", "--frontend-lang", "angular"), "--dry-run"),
		},
		{
			name: "multitier invalid backend-lang",
			args: append(withBase("--arch", "multitier", "--repo-strategy", "multirepo", "--backend-lang", "python", "--frontend-lang", "react"), "--dry-run"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, exitCode := runCLI(t, tt.args...)
			if exitCode == 0 {
				t.Errorf("expected non-zero exit code for invalid config: %s", fmt.Sprintf("%v", tt.args))
			}
		})
	}
}
