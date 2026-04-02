//go:build system

package config

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

var binaryPath string

func testOwner() string {
	owner := os.Getenv("TEST_OWNER")
	if owner == "" {
		panic("TEST_OWNER environment variable is required")
	}
	return owner
}

var baseArgs = []string{
	"--system-name", "Test System",
	"--repo", "test-app",
	"--random-suffix",
}

func cleanupFlags() []string {
	if os.Getenv("TEST_NO_CLEANUP") == "1" {
		return []string{"--no-cleanup"}
	}
	flags := []string{"--cleanup"}
	if os.Getenv("TEST_FORCE_CLEANUP") == "1" {
		flags = append(flags, "--force-cleanup")
	}
	return flags
}

func withBase(extra ...string) []string {
	args := []string{"--owner", testOwner()}
	args = append(args, baseArgs...)
	args = append(args, extra...)
	args = append(args, cleanupFlags()...)
	return args
}

func TestMain(m *testing.M) {
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

	// Find the module root (3 levels up from internal/config/)
	modRoot := filepath.Join("..", "..")

	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = modRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic("failed to build binary: " + err.Error() + "\n" + string(out))
	}

	binaryPath = bin
	os.Exit(m.Run())
}

// runCLI runs the binary and returns output + exit code.
// Valid config tests run with --test --cleanup for full scaffolding + automatic cleanup.
func runCLI(t *testing.T, args ...string) (string, int) {
	t.Helper()

	cmd := exec.Command(binaryPath, args...)
	// Use OPTIVEM_STARTER_PATH if set (e.g. in CI), otherwise resolve from workspace layout
	starterPath := os.Getenv("OPTIVEM_STARTER_PATH")
	if starterPath == "" {
		modRoot, _ := filepath.Abs(filepath.Join("..", ".."))
		starterPath = filepath.Join(filepath.Dir(modRoot), "starter")
	}
	cmd.Env = append(os.Environ(), "OPTIVEM_STARTER_PATH="+starterPath)
	out, err := cmd.CombinedOutput()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error running CLI: %v", err)
		}
	}
	return string(out), exitCode
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
				"--lang", tt.monolithLang,
				"--test-lang", tt.testLang,
				"--test",
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
				"--test",
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
				"--system-name", "Test System",
				"--repo", "test-app",
				"--arch", "monolith",
				"--repo-strategy", "monorepo",
				"--lang", "java",
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
				"--lang", "java",
				"--dry-run",
			},
		},
		{
			name: "missing repo",
			args: []string{
				"--owner", "testuser",
				"--system-name", "Test System",
				"--arch", "monolith",
				"--repo-strategy", "monorepo",
				"--lang", "java",
				"--dry-run",
			},
		},
		{
			name: "missing arch",
			args: append(withBase("--repo-strategy", "monorepo", "--lang", "java"), "--dry-run"),
		},
		{
			name: "invalid arch",
			args: append(withBase("--arch", "invalid", "--repo-strategy", "monorepo", "--lang", "java"), "--dry-run"),
		},
		{
			name: "missing repo-strategy",
			args: append(withBase("--arch", "monolith", "--lang", "java"), "--dry-run"),
		},
		{
			name: "invalid repo-strategy",
			args: append(withBase("--arch", "monolith", "--repo-strategy", "invalid", "--lang", "java"), "--dry-run"),
		},
		{
			name: "monolith missing lang",
			args: append(withBase("--arch", "monolith", "--repo-strategy", "monorepo"), "--dry-run"),
		},
		{
			name: "monolith invalid lang",
			args: append(withBase("--arch", "monolith", "--repo-strategy", "monorepo", "--lang", "python"), "--dry-run"),
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
