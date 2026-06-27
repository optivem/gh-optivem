package config

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// userEnvFileOverride is the env var that, when set, points gh-optivem at an
// explicit absolute .env path instead of the platform default. Ideal for a
// synced folder (Dropbox/OneDrive) so one file carries the credential set
// across machines. Uses the GH_OPTIVEM_ prefix shared by every gh-optivem
// operator env var.
const userEnvFileOverride = "GH_OPTIVEM_ENV_FILE"

// UserEnvFilePath returns the path gh-optivem loads a user-level .env from at
// startup: the GH_OPTIVEM_ENV_FILE override if set, else
// os.UserConfigDir()/gh-optivem/.env (Windows %AppData%\gh-optivem\.env,
// Linux/mac ~/.config/gh-optivem/.env). The file need not exist — LoadEnvFile
// treats a missing path as a no-op. Returns an error only when the platform
// config dir can't be resolved and no override was given.
func UserEnvFilePath() (string, error) {
	if override := os.Getenv(userEnvFileOverride); override != "" {
		return override, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "gh-optivem", ".env"), nil
}

// LoadEnvFile loads key=value pairs from a .env file into the process
// environment and returns how many variables it actually set. A real exported
// env var always wins: a key is set only when os.Getenv(key) is currently
// empty, so the file fills gaps without overriding the shell. This is the
// production promotion of the test-only loadEnvFile (config_system_test.go),
// sharing its "existing env wins" precedence.
//
// Line handling: blank lines and lines beginning with '#' are skipped; an
// optional leading "export " prefix is stripped; the key/value split is the
// first '='; surrounding single or double quotes on the value are trimmed.
// A line without '=' is skipped rather than failing the load. A missing file
// is a silent no-op (returns 0, nil) — not every operator keeps a .env. Other
// open errors (e.g. permissions) are returned so the caller can surface them.
func LoadEnvFile(path string) (loaded int, err error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		v = trimQuotes(strings.TrimSpace(v))
		if os.Getenv(k) == "" && os.Setenv(k, v) == nil {
			loaded++
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		return loaded, scanErr
	}
	return loaded, nil
}

// trimQuotes strips one layer of matching surrounding single or double quotes
// from a value, leaving unquoted or mismatched values untouched.
func trimQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
