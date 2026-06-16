package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeEnvFile writes content to a fresh .env in a temp dir and returns its path.
func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	return path
}

func TestLoadEnvFileFillsGapsAndCounts(t *testing.T) {
	path := writeEnvFile(t, "ENVF_A=alpha\nENVF_B=beta\n")
	t.Cleanup(func() { os.Unsetenv("ENVF_A"); os.Unsetenv("ENVF_B") })

	loaded, err := LoadEnvFile(path)
	if err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}
	if loaded != 2 {
		t.Errorf("loaded = %d, want 2", loaded)
	}
	if got := os.Getenv("ENVF_A"); got != "alpha" {
		t.Errorf("ENVF_A = %q, want alpha", got)
	}
	if got := os.Getenv("ENVF_B"); got != "beta" {
		t.Errorf("ENVF_B = %q, want beta", got)
	}
}

func TestLoadEnvFileExistingEnvWins(t *testing.T) {
	// A real exported env var must never be overridden by the file.
	t.Setenv("ENVF_PRECEDENCE", "from-shell")
	path := writeEnvFile(t, "ENVF_PRECEDENCE=from-file\nENVF_NEW=from-file\n")
	t.Cleanup(func() { os.Unsetenv("ENVF_NEW") })

	loaded, err := LoadEnvFile(path)
	if err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}
	if loaded != 1 {
		t.Errorf("loaded = %d, want 1 (only the gap-filling var)", loaded)
	}
	if got := os.Getenv("ENVF_PRECEDENCE"); got != "from-shell" {
		t.Errorf("ENVF_PRECEDENCE = %q, want from-shell (exported value must win)", got)
	}
	if got := os.Getenv("ENVF_NEW"); got != "from-file" {
		t.Errorf("ENVF_NEW = %q, want from-file", got)
	}
}

func TestLoadEnvFileQuotesCommentsAndExport(t *testing.T) {
	content := "" +
		"# a comment line\n" +
		"\n" +
		"   # indented comment\n" +
		"ENVF_DQUOTE=\"double quoted\"\n" +
		"ENVF_SQUOTE='single quoted'\n" +
		"export ENVF_EXPORTED=exported-value\n" +
		"ENVF_SPACED =  spaced value \n" +
		"ENVF_EQUALS=a=b=c\n" +
		"no_equals_sign_line\n"
	path := writeEnvFile(t, content)
	t.Cleanup(func() {
		for _, k := range []string{"ENVF_DQUOTE", "ENVF_SQUOTE", "ENVF_EXPORTED", "ENVF_SPACED", "ENVF_EQUALS"} {
			os.Unsetenv(k)
		}
	})

	loaded, err := LoadEnvFile(path)
	if err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}
	if loaded != 5 {
		t.Errorf("loaded = %d, want 5", loaded)
	}
	cases := map[string]string{
		"ENVF_DQUOTE":   "double quoted",
		"ENVF_SQUOTE":   "single quoted",
		"ENVF_EXPORTED": "exported-value",
		"ENVF_SPACED":   "spaced value", // surrounding whitespace trimmed
		"ENVF_EQUALS":   "a=b=c",        // only the first '=' splits
	}
	for k, want := range cases {
		if got := os.Getenv(k); got != want {
			t.Errorf("%s = %q, want %q", k, got, want)
		}
	}
}

func TestLoadEnvFileMissingIsNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.env")
	loaded, err := LoadEnvFile(path)
	if err != nil {
		t.Errorf("missing file should be a silent no-op, got err: %v", err)
	}
	if loaded != 0 {
		t.Errorf("loaded = %d, want 0", loaded)
	}
}

func TestUserEnvFilePathOverride(t *testing.T) {
	t.Setenv(userEnvFileOverride, "/synced/folder/gh-optivem.env")
	got, err := UserEnvFilePath()
	if err != nil {
		t.Fatalf("UserEnvFilePath: %v", err)
	}
	if got != "/synced/folder/gh-optivem.env" {
		t.Errorf("UserEnvFilePath = %q, want the override value", got)
	}
}

func TestUserEnvFilePathDefault(t *testing.T) {
	t.Setenv(userEnvFileOverride, "")
	got, err := UserEnvFilePath()
	if err != nil {
		t.Fatalf("UserEnvFilePath: %v", err)
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Skipf("no user config dir on this platform: %v", err)
	}
	want := filepath.Join(dir, "gh-optivem", ".env")
	if got != want {
		t.Errorf("UserEnvFilePath = %q, want %q", got, want)
	}
}
