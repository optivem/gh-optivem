package runner

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestPrintEndpoints_skipsEmptyURLs asserts that components or external
// systems with an empty URL produce no line — matches WaitForSystem's
// skip-empty-URL behavior so the two views of the config stay consistent.
func TestPrintEndpoints_skipsEmptyURLs(t *testing.T) {
	t.Parallel()
	sys := &SystemConfig{Systems: []SystemEntry{
		{
			Label: "real",
			Components: []Component{
				{Name: "api", URL: "http://localhost:1234"},
				{Name: "worker", URL: ""}, // no URL — must not appear
			},
			ExternalSystems: []Component{
				{Name: "stub-ext", URL: ""}, // also no URL
			},
		},
	}}
	var buf bytes.Buffer
	PrintEndpoints(&buf, sys)
	got := buf.String()
	if !strings.Contains(got, "api: http://localhost:1234") {
		t.Errorf("expected api line, got: %q", got)
	}
	if strings.Contains(got, "worker") {
		t.Errorf("worker has empty URL, should be skipped; got: %q", got)
	}
	if strings.Contains(got, "stub-ext") {
		t.Errorf("stub-ext has empty URL, should be skipped; got: %q", got)
	}
}

// TestPrintEndpoints_writesAllSystems asserts that every system in the config
// contributes its components and external systems to the output, in order.
func TestPrintEndpoints_writesAllSystems(t *testing.T) {
	t.Parallel()
	sys := &SystemConfig{Systems: []SystemEntry{
		{
			Label: "real",
			Components: []Component{
				{Name: "real-api", URL: "http://localhost:1111"},
				{Name: "real-web", URL: "http://localhost:1112"},
			},
		},
		{
			Label: "stub",
			Components: []Component{
				{Name: "stub-api", URL: "http://localhost:2221"},
				{Name: "stub-web", URL: "http://localhost:2222"},
			},
		},
	}}
	var buf bytes.Buffer
	PrintEndpoints(&buf, sys)
	got := buf.String()
	for _, want := range []string{
		"real-api: http://localhost:1111",
		"real-web: http://localhost:1112",
		"stub-api: http://localhost:2221",
		"stub-web: http://localhost:2222",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing line %q in output:\n%s", want, got)
		}
	}
	// Order: real before stub, components in declaration order.
	ridx := strings.Index(got, "real-api")
	sidx := strings.Index(got, "stub-api")
	if ridx < 0 || sidx < 0 || ridx > sidx {
		t.Errorf("expected real-api before stub-api in output:\n%s", got)
	}
}

// TestStatus_returnsDownCount asserts that Status returns the number of DOWN
// components, prints OK for live URLs, and DOWN (with the URL) for dead ones.
// Uses httptest for OK URLs and a reserved-port-style unreachable URL for the
// DOWN case so the test doesn't depend on network reachability.
func TestStatus_returnsDownCount(t *testing.T) {
	t.Parallel()
	okSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer okSrv.Close()
	// 127.0.0.1:1 is reserved; connection refuses fast. Backed by a 100ms
	// status timeout so the test stays snappy even on slow CI.
	downURL := "http://127.0.0.1:1/"

	sys := &SystemConfig{Systems: []SystemEntry{
		{
			Label: "real",
			Components: []Component{
				{Name: "api", URL: okSrv.URL},
				{Name: "worker", URL: downURL},
				{Name: "empty", URL: ""}, // skipped entirely
			},
			ExternalSystems: []Component{
				{Name: "ext-down", URL: downURL},
			},
		},
	}}
	var buf bytes.Buffer
	down := Status(&buf, sys, StatusOptions{Timeout: 100 * time.Millisecond})
	if down != 2 {
		t.Errorf("down count: got %d, want 2 (worker + ext-down)", down)
	}
	got := buf.String()
	if !strings.Contains(got, "OK api: "+okSrv.URL) {
		t.Errorf("expected 'OK api: %s' line, got:\n%s", okSrv.URL, got)
	}
	if !strings.Contains(got, "DOWN worker: "+downURL) {
		t.Errorf("expected 'DOWN worker' line, got:\n%s", got)
	}
	if !strings.Contains(got, "DOWN ext-down: "+downURL) {
		t.Errorf("expected 'DOWN ext-down' line, got:\n%s", got)
	}
	if strings.Contains(got, "empty") {
		t.Errorf("empty-URL component must be skipped, got:\n%s", got)
	}
}

// TestStatus_zeroDownAllOK asserts the happy path: every URL responds 200,
// Status returns 0, every line is OK.
func TestStatus_zeroDownAllOK(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	sys := &SystemConfig{Systems: []SystemEntry{{
		Label:      "real",
		Components: []Component{{Name: "api", URL: srv.URL}},
	}}}
	var buf bytes.Buffer
	down := Status(&buf, sys, StatusOptions{Timeout: 500 * time.Millisecond})
	if down != 0 {
		t.Errorf("down: got %d, want 0", down)
	}
	if strings.Contains(buf.String(), "DOWN") {
		t.Errorf("unexpected DOWN line in:\n%s", buf.String())
	}
}

// TestStatus_emitsPerSystemHeader asserts that Status writes one
// "=== System connected to <desc> ===" header per SystemEntry, prefers
// Description over Label, and preserves declaration order (stub before real
// when stub is declared first). Uses an unreachable URL so the probe lines
// resolve fast and deterministically; only header presence and ordering are
// under test.
func TestStatus_emitsPerSystemHeader(t *testing.T) {
	t.Parallel()
	downURL := "http://127.0.0.1:1/"
	sys := &SystemConfig{Systems: []SystemEntry{
		{
			Label:       "stub",
			Description: "External System Stubs",
			Components:  []Component{{Name: "api", URL: downURL}},
		},
		{
			Label:      "real", // no Description — header falls back to label
			Components: []Component{{Name: "api", URL: downURL}},
		},
	}}
	var buf bytes.Buffer
	_ = Status(&buf, sys, StatusOptions{Timeout: 50 * time.Millisecond})
	got := buf.String()
	stubHeader := "=== System connected to External System Stubs ==="
	realHeader := "=== System connected to real ==="
	stubIdx := strings.Index(got, stubHeader)
	realIdx := strings.Index(got, realHeader)
	if stubIdx < 0 {
		t.Errorf("missing description-driven header %q in:\n%s", stubHeader, got)
	}
	if realIdx < 0 {
		t.Errorf("missing label-fallback header %q in:\n%s", realHeader, got)
	}
	if stubIdx >= 0 && realIdx >= 0 && stubIdx > realIdx {
		t.Errorf("expected stub header before real header (declaration order), got indices %d / %d in:\n%s", stubIdx, realIdx, got)
	}
}

// TestStatus_non200IsDown asserts that a 500 response counts as DOWN
// (matches the snapshot semantics: anything that is not a healthy 200 is
// reported as DOWN, including server errors).
func TestStatus_non200IsDown(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	sys := &SystemConfig{Systems: []SystemEntry{{
		Label:      "real",
		Components: []Component{{Name: "broken", URL: srv.URL}},
	}}}
	var buf bytes.Buffer
	down := Status(&buf, sys, StatusOptions{Timeout: 500 * time.Millisecond})
	if down != 1 {
		t.Errorf("down: got %d, want 1", down)
	}
	if !strings.Contains(buf.String(), "DOWN broken:") {
		t.Errorf("expected DOWN broken line, got:\n%s", buf.String())
	}
}
