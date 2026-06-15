package pathx

import (
	"runtime"
	"testing"
)

func TestNormalizeExe(t *testing.T) {
	cases := []struct {
		in        string
		wantUnix  string
		wantWin32 string
	}{
		{`.\gradlew.bat`, "./gradlew", `.\gradlew.bat`},
		{`gradlew.bat`, "gradlew", `gradlew.bat`},
		{`./gradlew`, "./gradlew", "./gradlew"},
		{"npm", "npm", "npm"},
		{"dotnet", "dotnet", "dotnet"},
		{"gh", "gh", "gh"},
	}
	for _, c := range cases {
		want := c.wantUnix
		if runtime.GOOS == "windows" {
			want = c.wantWin32
		}
		if got := NormalizeExe(c.in); got != want {
			t.Errorf("NormalizeExe(%q) = %q, want %q (GOOS=%s)", c.in, got, want, runtime.GOOS)
		}
	}
}
