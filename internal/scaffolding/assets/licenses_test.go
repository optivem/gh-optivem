package assets

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/kernel/projectconfig"
)

// TestEveryOfferedKeyHasAsset pins the offered set and the bundled tree to
// each other in both directions. A key added to projectconfig without a text
// would fail WriteLicense at scaffold time on a user's machine; a text with no
// key is dead weight nobody can select.
func TestEveryOfferedKeyHasAsset(t *testing.T) {
	keys := projectconfig.LicenseKeys()
	if len(keys) == 0 {
		t.Fatal("projectconfig.LicenseKeys() is empty")
	}

	for _, key := range keys {
		text, err := LicenseText(key)
		if err != nil {
			t.Errorf("offered license %q has no bundled asset: %v", key, err)
			continue
		}
		if strings.TrimSpace(text) == "" {
			t.Errorf("bundled asset for %q is blank", key)
		}
	}

	entries, err := licenseFS.ReadDir("licenses")
	if err != nil {
		t.Fatalf("read embedded licenses dir: %v", err)
	}
	offered := map[string]bool{}
	for _, key := range keys {
		offered[key] = true
	}
	for _, e := range entries {
		key := strings.TrimSuffix(e.Name(), ".txt")
		if !offered[key] {
			t.Errorf("bundled asset %q is not in projectconfig.LicenseKeys() -- nobody can select it", e.Name())
		}
	}
}

// TestEveryOfferedKeyHasSPDXID guards the manifest writer: an offered key with
// no SPDX id makes templateManifestLicense fatal mid-scaffold.
func TestEveryOfferedKeyHasSPDXID(t *testing.T) {
	for _, key := range projectconfig.LicenseKeys() {
		if projectconfig.LicenseSPDXID(key) == "" {
			t.Errorf("offered license %q has no SPDX identifier", key)
		}
	}
}

// TestRenderLicense_FillableKeysLeaveNoPlaceholder covers Problem 3's "yes"
// column: the operative copyright notice must name the holder, not ship the
// literal "[year] [fullname]" the template carries.
func TestRenderLicense_FillableKeysLeaveNoPlaceholder(t *testing.T) {
	const year, holder = "2026", "Optivem d.o.o."

	for _, key := range projectconfig.LicenseKeys() {
		if !HasFillableNotice(key) {
			continue
		}
		got, err := RenderLicense(key, year, holder)
		if err != nil {
			t.Fatalf("RenderLicense(%q): %v", key, err)
		}
		if strings.ContainsAny(got, "[<") {
			t.Errorf("%s: rendered text still carries a placeholder delimiter:\n%s", key, firstLineWith(got, "[<"))
		}
		if !strings.Contains(got, year) {
			t.Errorf("%s: rendered text does not carry the year %q", key, year)
		}
		if !strings.Contains(got, holder) {
			t.Errorf("%s: rendered text does not carry the holder %q", key, holder)
		}
	}
}

// TestRenderLicense_NonFillableKeysAreVerbatim covers Problem 3's "no" column.
// Apache-2.0's and GPL-3.0's placeholders sit in an instructional appendix
// about heading your own source files; substituting there would corrupt the
// document, and GPL-3.0 forbids modifying it at all.
func TestRenderLicense_NonFillableKeysAreVerbatim(t *testing.T) {
	for _, key := range projectconfig.LicenseKeys() {
		if HasFillableNotice(key) {
			continue
		}
		want, err := LicenseText(key)
		if err != nil {
			t.Fatalf("LicenseText(%q): %v", key, err)
		}
		got, err := RenderLicense(key, "2026", "Optivem d.o.o.")
		if err != nil {
			t.Fatalf("RenderLicense(%q): %v", key, err)
		}
		if got != want {
			t.Errorf("%s: RenderLicense modified a license that must ship verbatim", key)
		}
	}
}

// TestNonFillableKeysAreTheExpectedOnes pins the classification itself, so a
// future key cannot silently join the "ship verbatim" side by omission.
func TestNonFillableKeysAreTheExpectedOnes(t *testing.T) {
	want := map[string]bool{
		projectconfig.LicenseApache2:   true,
		projectconfig.LicenseGPL3:      true,
		projectconfig.LicenseUnlicense: true,
	}
	for _, key := range projectconfig.LicenseKeys() {
		if HasFillableNotice(key) == want[key] {
			t.Errorf("license %q: fillable=%t, expected %t", key, HasFillableNotice(key), !want[key])
		}
	}
}

// TestLicenseText_UnknownKeyIsAnError covers Problem 1: an unresolvable key
// must surface as an error the caller turns into a failed scaffold, never as
// an empty body that writes a zero-byte LICENSE.
func TestLicenseText_UnknownKeyIsAnError(t *testing.T) {
	for _, key := range []string{"", "not-a-license", "MIT", "../licenses/mit"} {
		text, err := LicenseText(key)
		if err == nil {
			t.Errorf("LicenseText(%q) returned no error", key)
		}
		if text != "" {
			t.Errorf("LicenseText(%q) returned text alongside the failure", key)
		}
		if _, err := RenderLicense(key, "2026", "Optivem"); err == nil {
			t.Errorf("RenderLicense(%q) returned no error", key)
		}
	}
}

func firstLineWith(s, chars string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.ContainsAny(line, chars) {
			return line
		}
	}
	return ""
}
