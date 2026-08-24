package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/config"
	"github.com/optivem/gh-optivem/internal/kernel/log"
	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// TestTemplateManifestLicense_MatchesConfiguredLicense covers Problem 2: the
// scaffolded manifest must declare the same license the LICENSE file grants,
// not whatever the template repo carried (MIT-0, as of shop 6b69c9e6).
func TestTemplateManifestLicense_MatchesConfiguredLicense(t *testing.T) {
	for _, key := range projectconfig.LicenseKeys() {
		t.Run(key, func(t *testing.T) {
			dir := t.TempDir()
			manifest := filepath.Join(dir, "package.json")
			writeFile(t, manifest, `{
  "name": "my-shop-system-test",
  "version": "1.0.0",
  "license": "MIT-0",
  "author": "Optivem"
}
`)

			templateManifestLicense(dir, key)

			want := projectconfig.LicenseSPDXID(key)
			got := readFile(t, manifest)
			if !strings.Contains(got, `"license": "`+want+`"`) {
				t.Errorf("license %q: manifest does not declare %q:\n%s", key, want, got)
			}
			if strings.Contains(got, `"MIT-0"`) && want != "MIT-0" {
				t.Errorf("license %q: template's MIT-0 survived:\n%s", key, got)
			}
			if !strings.Contains(got, `"author": "Optivem"`) {
				t.Errorf("license %q: rewrite disturbed a neighbouring field:\n%s", key, got)
			}
		})
	}
}

// TestTemplateManifestLicense_SkipsVendoredManifests guards against rewriting
// a dependency's own license. shop carries Playwright's package.json (declaring
// Apache-2.0) under system-test/dotnet/**/bin/Debug/**, which belongs to
// Playwright, not to the scaffolded project.
func TestTemplateManifestLicense_SkipsVendoredManifests(t *testing.T) {
	dir := t.TempDir()

	const vendored = `{
  "name": "playwright",
  "license": "Apache-2.0"
}
`
	vendoredPaths := []string{
		filepath.Join(dir, "node_modules", "left-pad", "package.json"),
		filepath.Join(dir, "system-test", "dotnet", "Common", "bin", "Debug", "net8.0", ".playwright", "package", "package.json"),
		filepath.Join(dir, "system", "obj", "package.json"),
		filepath.Join(dir, "frontend", "dist", "package.json"),
	}
	for _, p := range vendoredPaths {
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		writeFile(t, p, vendored)
	}

	ours := filepath.Join(dir, "system-test", "typescript", "package.json")
	if err := os.MkdirAll(filepath.Dir(ours), 0755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, ours, `{
  "name": "my-shop-system-test",
  "license": "MIT-0"
}
`)

	templateManifestLicense(dir, projectconfig.LicenseApache2)

	for _, p := range vendoredPaths {
		if got := readFile(t, p); got != vendored {
			t.Errorf("vendored manifest %s was rewritten:\n%s", p, got)
		}
	}
	if got := readFile(t, ours); !strings.Contains(got, `"license": "Apache-2.0"`) {
		t.Errorf("our own manifest was not rewritten:\n%s", got)
	}
}

// TestTemplateManifestLicense_LeavesManifestsWithoutALicenseAlone — a manifest
// that declares nothing contradicts nothing, and injecting a field would edit
// files the plan never scoped.
func TestTemplateManifestLicense_LeavesManifestsWithoutALicenseAlone(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "package.json")
	const before = `{
  "name": "my-shop-frontend",
  "private": true
}
`
	writeFile(t, manifest, before)

	templateManifestLicense(dir, projectconfig.LicenseMIT)

	if got := readFile(t, manifest); got != before {
		t.Errorf("manifest without a license field was modified:\n%s", got)
	}
}

// TestCopyrightHolder covers the copyright-holder key: the explicit value wins,
// and the GitHub owner handle is the fallback so zero-config stays zero-config.
func TestCopyrightHolder(t *testing.T) {
	tests := []struct {
		name   string
		cfg    *config.Config
		expect string
	}{
		{
			name:   "explicit holder wins",
			cfg:    &config.Config{Owner: "optivem", CopyrightHolder: "Optivem d.o.o."},
			expect: "Optivem d.o.o.",
		},
		{
			name:   "falls back to owner when unset",
			cfg:    &config.Config{Owner: "optivem"},
			expect: "optivem",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := copyrightHolder(tt.cfg); got != tt.expect {
				t.Errorf("copyrightHolder() = %q, want %q", got, tt.expect)
			}
		})
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestWriteLicense_UnknownKeyFailsTheStep is the regression test for Problem 1.
// The fetch-based implementation warned and returned, so an unresolvable
// license produced a repo with no LICENSE while the scaffold still reported
// success. The step must now fail — log.Fatal panics with a *log.StepError the
// step runner catches and turns into a non-zero exit.
func TestWriteLicense_UnknownKeyFailsTheStep(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{Owner: "optivem", License: "not-a-license", RepoDir: dir}

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("WriteLicense returned normally for an unknown license key -- the silent-skip path is back")
		}
		se, ok := r.(*log.StepError)
		if !ok {
			t.Fatalf("panic was %T, want *log.StepError (the step runner only catches that)", r)
		}
		if !strings.Contains(se.Msg, "not-a-license") {
			t.Errorf("failure message does not name the license key: %s", se.Msg)
		}
		if !strings.Contains(se.Msg, "assets/licenses/not-a-license.txt") {
			t.Errorf("failure message does not name the expected asset path: %s", se.Msg)
		}
		if _, err := os.Stat(filepath.Join(dir, "LICENSE")); !os.IsNotExist(err) {
			t.Error("a LICENSE file was written despite the failure")
		}
	}()

	WriteLicense(cfg)
}

// TestWriteLicense_WritesEveryRepoDir pins the multirepo fan-out: each
// companion repo gets its own LICENSE, filled with the same holder.
func TestWriteLicense_WritesEveryRepoDir(t *testing.T) {
	root, backend, frontend := t.TempDir(), t.TempDir(), t.TempDir()
	cfg := &config.Config{
		Owner:           "optivem",
		CopyrightHolder: "Optivem d.o.o.",
		License:         projectconfig.LicenseMIT,
		Arch:            "multitier",
		RepoStrategy:    "multirepo",
		RepoDir:         root,
		BackendRepoDir:  backend,
		FrontendRepoDir: frontend,
	}

	WriteLicense(cfg)

	for _, dir := range []string{root, backend, frontend} {
		got := readFile(t, filepath.Join(dir, "LICENSE"))
		if !strings.Contains(got, "Copyright (c) ") || !strings.Contains(got, "Optivem d.o.o.") {
			t.Errorf("%s: LICENSE notice not filled:\n%s", dir, got)
		}
		if strings.ContainsAny(got, "[<") {
			t.Errorf("%s: LICENSE still carries a placeholder", dir)
		}
	}
}
