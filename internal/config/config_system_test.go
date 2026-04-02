//go:build system

package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

var binaryPath string

var baseArgs = []string{
	"--owner", "testuser",
	"--system-name", "Test System",
	"--repo", "test-app",
	"--random-suffix",
}

func withBase(extra ...string) []string {
	args := make([]string, len(baseArgs))
	copy(args, baseArgs)
	return append(args, extra...)
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

func runCLI(t *testing.T, args ...string) (string, int) {
	t.Helper()
	// Always add --dry-run so no real repos are created
	args = append(args, "--dry-run")

	cmd := exec.Command(binaryPath, args...)
	// Set OPTIVEM_STARTER_PATH to avoid "Cannot find VERSION file" error
	cmd.Env = append(os.Environ(), "OPTIVEM_STARTER_PATH="+filepath.Join("..", ".."))
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
		name string
		args []string
	}{
		{
			name: "monolith java",
			args: withBase("--arch", "monolith", "--lang", "java"),
		},
		{
			name: "monolith dotnet",
			args: withBase("--arch", "monolith", "--lang", "dotnet"),
		},
		{
			name: "monolith typescript",
			args: withBase("--arch", "monolith", "--lang", "typescript"),
		},
		{
			name: "monolith java with explicit test-lang",
			args: withBase("--arch", "monolith", "--lang", "java", "--test-lang", "typescript"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, exitCode := runCLI(t, tt.args...)
			if exitCode != 0 {
				t.Errorf("expected exit code 0, got %d\noutput: %s", exitCode, out)
			}
		})
	}
}

func TestValidMultitierConfigurations(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "multitier java backend react frontend",
			args: withBase("--arch", "multitier", "--backend-lang", "java", "--frontend-lang", "react"),
		},
		{
			name: "multitier dotnet backend react frontend",
			args: withBase("--arch", "multitier", "--backend-lang", "dotnet", "--frontend-lang", "react"),
		},
		{
			name: "multitier typescript backend react frontend",
			args: withBase("--arch", "multitier", "--backend-lang", "typescript", "--frontend-lang", "react"),
		},
		{
			name: "multitier java backend react frontend with test-lang",
			args: withBase("--arch", "multitier", "--backend-lang", "java", "--frontend-lang", "react", "--test-lang", "java"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, exitCode := runCLI(t, tt.args...)
			if exitCode != 0 {
				t.Errorf("expected exit code 0, got %d\noutput: %s", exitCode, out)
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
				"--lang", "java",
			},
		},
		{
			name: "missing system-name",
			args: []string{
				"--owner", "testuser",
				"--repo", "test-app",
				"--arch", "monolith",
				"--lang", "java",
			},
		},
		{
			name: "missing repo",
			args: []string{
				"--owner", "testuser",
				"--system-name", "Test System",
				"--arch", "monolith",
				"--lang", "java",
			},
		},
		{
			name: "missing arch",
			args: withBase("--lang", "java"),
		},
		{
			name: "invalid arch",
			args: withBase("--arch", "invalid", "--lang", "java"),
		},
		{
			name: "monolith missing lang",
			args: withBase("--arch", "monolith"),
		},
		{
			name: "monolith invalid lang",
			args: withBase("--arch", "monolith", "--lang", "python"),
		},
		{
			name: "multitier missing backend-lang",
			args: withBase("--arch", "multitier", "--frontend-lang", "react"),
		},
		{
			name: "multitier missing frontend-lang",
			args: withBase("--arch", "multitier", "--backend-lang", "java"),
		},
		{
			name: "multitier invalid frontend-lang",
			args: withBase("--arch", "multitier", "--backend-lang", "java", "--frontend-lang", "angular"),
		},
		{
			name: "multitier invalid backend-lang",
			args: withBase("--arch", "multitier", "--backend-lang", "python", "--frontend-lang", "react"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, exitCode := runCLI(t, tt.args...)
			if exitCode == 0 {
				t.Errorf("expected non-zero exit code for invalid config")
			}
		})
	}
}
