package sonar

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/kernel/shell"
)

func TestNewClientDefaults(t *testing.T) {
	c := NewClient("", "tok")
	if c.BaseURL != DefaultBaseURL {
		t.Errorf("empty baseURL → %q, want %q", c.BaseURL, DefaultBaseURL)
	}
	if c.Token != "tok" {
		t.Errorf("token not propagated: %q", c.Token)
	}
	// Trailing slash gets trimmed.
	c2 := NewClient("https://example.test/api/", "tok")
	if c2.BaseURL != "https://example.test/api" {
		t.Errorf("trailing slash not trimmed: %q", c2.BaseURL)
	}
}

func TestSearchProjectsRequiresToken(t *testing.T) {
	c := NewClient("https://example.test/api", "")
	if _, err := c.SearchProjects("org", 1, 100); err == nil {
		t.Error("expected error when token is empty")
	}
}

func TestDeleteProjectRequiresToken(t *testing.T) {
	c := NewClient("https://example.test/api", "")
	if err := c.DeleteProject("org_key"); err == nil {
		t.Error("expected error when token is empty")
	}
}

func TestSearchProjectsHappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/projects/search" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("organization"); got != "myorg" {
			t.Errorf("organization=%q, want myorg", got)
		}
		if got := r.URL.Query().Get("p"); got != "2" {
			t.Errorf("p=%q, want 2", got)
		}
		if got := r.URL.Query().Get("ps"); got != "50" {
			t.Errorf("ps=%q, want 50", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok" {
			t.Errorf("auth=%q, want 'Bearer tok'", got)
		}
		w.WriteHeader(200)
		io.WriteString(w, `{"components":[{"key":"a","name":"A"},{"key":"b","name":"B"}],"paging":{"pageIndex":2,"pageSize":50,"total":120}}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	page, err := c.SearchProjects("myorg", 2, 50)
	if err != nil {
		t.Fatalf("SearchProjects: %v", err)
	}
	if len(page.Components) != 2 || page.Components[0].Key != "a" {
		t.Errorf("unexpected components: %+v", page.Components)
	}
	if page.Paging.Total != 120 {
		t.Errorf("paging.total = %d, want 120", page.Paging.Total)
	}
}

func TestDeleteProjectSendsFormBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method=%q, want POST", r.Method)
		}
		if r.URL.Path != "/projects/delete" {
			t.Errorf("path=%q, want /projects/delete", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("content-type=%q, want form-urlencoded", ct)
		}
		body, _ := io.ReadAll(r.Body)
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("body not form-encoded: %v (%q)", err, body)
		}
		if got := vals.Get("project"); got != "myorg_repo" {
			t.Errorf("project=%q, want myorg_repo", got)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	if err := c.DeleteProject("myorg_repo"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
}

func TestDoSurfacesErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		io.WriteString(w, `{"errors":[{"msg":"forbidden"}]}`)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	_, err := c.SearchProjects("org", 1, 100)
	if err == nil {
		t.Fatal("expected error on 403")
	}
	if !strings.Contains(err.Error(), "HTTP 403") {
		t.Errorf("error missing status: %v", err)
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("error missing body snippet: %v", err)
	}
}

// TestDoRetriesOn5xxThenSucceeds pins the retry contract for Client.do: a
// SearchProjects call that observes 504 twice then 200 must hit the transport
// three times. Sleep is stubbed via shell.SetSleepFnForTest so the test
// finishes immediately instead of waiting 5s+15s between attempts.
func TestDoRetriesOn5xxThenSucceeds(t *testing.T) {
	var sleeps []time.Duration
	restore := shell.SetSleepFnForTest(func(d time.Duration) { sleeps = append(sleeps, d) })
	defer restore()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		switch n {
		case 1, 2:
			w.WriteHeader(504)
			io.WriteString(w, "Gateway Timeout")
		default:
			w.WriteHeader(200)
			io.WriteString(w, `{"components":[],"paging":{"pageIndex":1,"pageSize":100,"total":0}}`)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	page, err := c.SearchProjects("myorg", 1, 100)
	if err != nil {
		t.Fatalf("SearchProjects: unexpected error after retry: %v", err)
	}
	if page == nil {
		t.Fatal("SearchProjects returned nil page after retry success")
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("transport calls = %d, want 3 (504, 504, 200)", got)
	}
	if len(sleeps) != 2 {
		t.Fatalf("sleeps = %d, want 2 backoffs", len(sleeps))
	}
}

// TestDoHardFailOn4xxNoRetry pins the hard-fail contract: a 401 must surface
// immediately, with no retries.
func TestDoHardFailOn4xxNoRetry(t *testing.T) {
	var sleeps []time.Duration
	restore := shell.SetSleepFnForTest(func(d time.Duration) { sleeps = append(sleeps, d) })
	defer restore()

	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(401)
		io.WriteString(w, "Unauthorized")
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	_, err := c.SearchProjects("myorg", 1, 100)
	if err == nil {
		t.Fatal("SearchProjects: expected error on 401, got nil")
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("transport calls = %d, want 1 (no retry on 4xx)", got)
	}
	if len(sleeps) != 0 {
		t.Fatalf("sleeps = %d, want 0 — hard-fail must not sleep", len(sleeps))
	}
	if !strings.Contains(err.Error(), "HTTP 401") {
		t.Errorf("error missing HTTP 401: %v", err)
	}
}

// TestDeleteProjectBodyResentOnRetry confirms the form body is rebuilt each
// retry attempt — without the buffer-once-then-rebuild logic in do(), the
// second attempt would send an empty body.
func TestDeleteProjectBodyResentOnRetry(t *testing.T) {
	var sleeps []time.Duration
	restore := shell.SetSleepFnForTest(func(d time.Duration) { sleeps = append(sleeps, d) })
	defer restore()

	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(buf))
		if len(bodies) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "tok")
	if err := c.DeleteProject("myorg_repo"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("got %d bodies, want 2", len(bodies))
	}
	for i, b := range bodies {
		if !strings.Contains(b, "project=myorg_repo") {
			t.Errorf("attempt %d body missing project=myorg_repo: %q", i+1, b)
		}
	}
}

func TestMaxPage(t *testing.T) {
	tests := []struct {
		total, pageSize, want int
	}{
		{0, 100, 0},
		{1, 100, 1},
		{100, 100, 1},
		{101, 100, 2},
		{250, 100, 3},
		{120, 50, 3},
		{-5, 100, 0},
		{100, 0, 0},
	}
	for _, tc := range tests {
		if got := MaxPage(tc.total, tc.pageSize); got != tc.want {
			t.Errorf("MaxPage(%d, %d) = %d, want %d", tc.total, tc.pageSize, got, tc.want)
		}
	}
}
