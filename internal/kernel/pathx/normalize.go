// Package pathx contains small cross-platform path helpers shared across
// packages that exec subprocesses.
package pathx

import (
	"runtime"
	"strings"
)

// NormalizeExe translates Windows-style wrapper paths (e.g. `.\gradlew.bat`)
// to their Unix equivalents (`./gradlew`) when running on a non-Windows host,
// so a single command literal in source/JSON works on both platforms. On
// Windows the path is returned unchanged.
//
// The translation is intentionally narrow: backslashes are normalized to
// forward slashes and a trailing `.bat` is stripped. It exists for the
// gradle-wrapper convention (gradlew.bat / gradlew) and other scripts that
// follow the same shape; it does not try to translate cmd.exe invocations
// or PowerShell scripts.
func NormalizeExe(exe string) string {
	if runtime.GOOS == "windows" {
		return exe
	}
	exe = strings.ReplaceAll(exe, `\`, `/`)
	exe = strings.TrimSuffix(exe, ".bat")
	return exe
}
