package license

import (
	"sort"
	"strings"
	"testing"
)

// TestKeys_AreTheBundledTexts pins the offered set to the texts/ directory:
// GitHub's license list plus mit-0 and 0bsd (plan 20261102-1714). A key
// listed here with no .txt fails; a .txt not listed here fails. Adding a
// license is a .txt file, a licenseMeta row, and a line here.
func TestKeys_AreTheBundledTexts(t *testing.T) {
	want := []string{
		"0bsd", "agpl-3.0", "apache-2.0", "bsd-2-clause", "bsd-3-clause",
		"bsl-1.0", "cc0-1.0", "epl-2.0", "gpl-2.0", "gpl-3.0", "lgpl-2.1",
		"lgpl-3.0", "mit", "mit-0", "mpl-2.0", "unlicense",
	}
	got := Keys()
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Keys() = %v\nwant     %v", got, want)
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("Keys() is not sorted: %v", got)
	}
}

func TestEveryKeyIsValid(t *testing.T) {
	for _, key := range Keys() {
		if !IsValid(key) {
			t.Errorf("IsValid(%q) = false for an offered key", key)
		}
	}
}

// TestEveryKeyHasMetadata guards README/banner rendering and the manifest
// writer: an offered key with no SPDX id makes templateManifestLicense fatal
// mid-scaffold, and a metadata row with no text is dead weight.
func TestEveryKeyHasMetadata(t *testing.T) {
	for _, key := range Keys() {
		if Name(key) == key {
			t.Errorf("offered license %q has no human-readable name", key)
		}
		if SPDXID(key) == "" {
			t.Errorf("offered license %q has no SPDX identifier", key)
		}
	}
	for key := range licenseMeta {
		if !IsValid(key) {
			t.Errorf("licenseMeta has %q but there is no texts/%s.txt", key, key)
		}
	}
}

func TestEveryKeyRendersNonEmpty(t *testing.T) {
	for _, key := range Keys() {
		got, err := Render(key, "2026", "Optivem d.o.o.")
		if err != nil {
			t.Errorf("Render(%q): %v", key, err)
			continue
		}
		if strings.TrimSpace(got) == "" {
			t.Errorf("Render(%q) returned a blank LICENSE", key)
		}
	}
}

// TestRender_FillableKeysLeaveNoPlaceholder: the operative copyright notice
// must name the holder, not ship the literal "[year] [fullname]".
func TestRender_FillableKeysLeaveNoPlaceholder(t *testing.T) {
	const year, holder = "2026", "Optivem d.o.o."

	for _, key := range Keys() {
		if !HasFillableNotice(key) {
			continue
		}
		got, err := Render(key, year, holder)
		if err != nil {
			t.Fatalf("Render(%q): %v", key, err)
		}
		if strings.ContainsAny(got, "[<") {
			t.Errorf("%s: rendered text still carries a placeholder delimiter:\n%s", key, firstLineWith(got, "[<"))
		}
		if !strings.Contains(got, "Copyright (c) "+year) {
			t.Errorf("%s: rendered notice does not carry the year %q", key, year)
		}
		if !strings.Contains(got, holder) {
			t.Errorf("%s: rendered text does not carry the holder %q", key, holder)
		}
	}
}

// TestRender_NonFillableKeysAreVerbatim: Apache's and the GNU licenses'
// placeholders sit in an instructional appendix about heading your own
// source files; substituting there would corrupt the document. None of them
// may carry the repo's fill placeholders, or a user would get an unfilled
// "[fullname]" in their LICENSE.
func TestRender_NonFillableKeysAreVerbatim(t *testing.T) {
	for _, key := range Keys() {
		if HasFillableNotice(key) {
			continue
		}
		want, err := Text(key)
		if err != nil {
			t.Fatalf("Text(%q): %v", key, err)
		}
		got, err := Render(key, "2026", "Optivem d.o.o.")
		if err != nil {
			t.Fatalf("Render(%q): %v", key, err)
		}
		if got != want {
			t.Errorf("%s: Render modified a license that must ship verbatim", key)
		}
		if strings.Contains(got, yearPlaceholder) || strings.Contains(got, holderPlaceholder) {
			t.Errorf("%s: verbatim text carries a fill placeholder that will never be filled", key)
		}
	}
}

// TestFillableKeysAreTheExpectedOnes pins the classification derived from
// the texts, so a refresh that changes a text's placeholders cannot silently
// move a license between "filled" and "verbatim".
func TestFillableKeysAreTheExpectedOnes(t *testing.T) {
	want := map[string]bool{
		"mit": true, "mit-0": true, "bsd-2-clause": true, "bsd-3-clause": true, "0bsd": true,
	}
	for _, key := range Keys() {
		if HasFillableNotice(key) != want[key] {
			t.Errorf("license %q: fillable=%t, expected %t", key, HasFillableNotice(key), want[key])
		}
	}
}

// TestText_UnknownKeyIsAnError: an unresolvable key must surface as an error
// the caller turns into a failed scaffold, never as an empty body that writes
// a zero-byte LICENSE. The error names every valid key.
func TestText_UnknownKeyIsAnError(t *testing.T) {
	for _, key := range []string{"", "not-a-license", "MIT", "AGPL-3.0", "../texts/mit", "texts/mit"} {
		text, err := Text(key)
		if err == nil {
			t.Errorf("Text(%q) returned no error", key)
			continue
		}
		if text != "" {
			t.Errorf("Text(%q) returned text alongside the failure", key)
		}
		for _, k := range Keys() {
			if !strings.Contains(err.Error(), k) {
				t.Errorf("Text(%q) error does not list valid key %q: %v", key, k, err)
			}
		}
		if _, err := Render(key, "2026", "Optivem"); err == nil {
			t.Errorf("Render(%q) returned no error", key)
		}
		if IsValid(key) {
			t.Errorf("IsValid(%q) = true", key)
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
