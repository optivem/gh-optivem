package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseConfigFull(t *testing.T) {
	c, err := ParseConfig([]byte(`specDir: specs
results: "**/build/test-results/**/*.xml"
threshold: 90
exclude: ["**/legacy/**"]
flags: [dark]
waivers:
  - id: UC-001.14
    reason: ERP sandbox unavailable
    approver: jane.doe
    until: 2026-12-31
`))
	if err != nil {
		t.Fatal(err)
	}
	if c.SpecDir != "specs" || len(c.Results) != 1 || c.ThresholdValue() != 90 || c.Baseline != "specs/.spec-baseline.json" {
		t.Fatalf("%+v", c)
	}
	if len(c.Waivers) != 1 || c.Waivers[0].Until != "2026-12-31" || len(c.Exclude) != 1 || c.Flags[0] != "dark" {
		t.Fatalf("%+v", c)
	}
}

func TestParseConfigDefaultsAndList(t *testing.T) {
	c, err := ParseConfig([]byte("results: [a.xml, b.xml]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.SpecDir != DefaultSpecDir || c.ThresholdValue() != 100 || len(c.Results) != 2 || c.RequireResults() != nil {
		t.Fatalf("%+v", c)
	}
	empty, err := ParseConfig(nil)
	if err != nil || empty.RequireResults() == nil {
		t.Fatalf("empty: %+v %v", empty, err)
	}
}

func TestParseConfigErrors(t *testing.T) {
	cases := map[string]string{
		"unknown key":   "bogus: 1\n",
		"threshold":     "threshold: 120\n",
		"missing field": "waivers:\n  - id: UC-001.1\n    reason: r\n    until: 2026-01-01\n",
		"bad date":      "waivers:\n  - id: UC-001.1\n    reason: r\n    approver: a\n    until: soon\n",
		"duplicate":     "waivers:\n  - {id: UC-001.1, reason: r, approver: a, until: 2026-01-01}\n  - {id: UC-001.1, reason: r, approver: a, until: 2026-01-01}\n",
		"bad results":   "results: {a: b}\n",
	}
	for name, in := range cases {
		if _, err := ParseConfig([]byte(in)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestLoadConfig(t *testing.T) {
	p := filepath.Join(t.TempDir(), "spec.yaml")
	if err := os.WriteFile(p, []byte("results: x.xml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(p); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfig(p + ".missing"); err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("err: %v", err)
	}
}

func TestWaiverExpired(t *testing.T) {
	w := Waiver{Until: "2026-10-28"}
	if w.Expired(time.Date(2026, 10, 28, 23, 59, 0, 0, time.UTC)) {
		t.Error("valid through the until date inclusive")
	}
	if !w.Expired(time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)) {
		t.Error("expired the day after")
	}
}
