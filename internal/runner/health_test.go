package runner

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fastPolling tunes the health-check loop down to millisecond granularity so
// tests don't actually sleep for full seconds. Reused across all health tests.
var fastPolling = HealthOptions{
	Attempts: 5,
	Interval: 1 * time.Millisecond,
	Timeout:  1 * time.Second,
}

func newOKServer(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestWaitForURLSucceedsImmediatelyOn200(t *testing.T) {
	ts := newOKServer(t)
	if err := WaitForURL(ts.URL, fastPolling); err != nil {
		t.Errorf("want success, got %v", err)
	}
}

func TestWaitForURLRetriesUntilSuccess(t *testing.T) {
	var attempts int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	if err := WaitForURL(ts.URL, fastPolling); err != nil {
		t.Errorf("want eventual success, got %v after %d attempts", err, atomic.LoadInt32(&attempts))
	}
	if atomic.LoadInt32(&attempts) < 3 {
		t.Errorf("want >=3 attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestWaitForURLFailsAfterMaxAttempts(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	err := WaitForURL(ts.URL, HealthOptions{Attempts: 2, Interval: 1 * time.Millisecond, Timeout: 1 * time.Second})
	if err == nil {
		t.Fatal("want failure after max attempts")
	}
	if !strings.Contains(err.Error(), "not ready") {
		t.Errorf("want 'not ready' error, got %v", err)
	}
}

func TestWaitForURLEmptyURLIsNoOp(t *testing.T) {
	if err := WaitForURL("", fastPolling); err != nil {
		t.Errorf("empty URL should be no-op, got %v", err)
	}
}

func TestWaitForSystemProbesAllUrls(t *testing.T) {
	var hits int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	sys := SystemEntry{
		Components: []Component{
			{Name: "c1", URL: ts.URL + "/c1"},
			{Name: "c2", URL: ""}, // skipped
			{Name: "c3", URL: ts.URL + "/c3"},
		},
		ExternalSystems: []Component{
			{Name: "e1", URL: ts.URL + "/e1"},
		},
	}
	if err := WaitForSystem(sys, fastPolling); err != nil {
		t.Fatalf("want success, got %v", err)
	}
	// 1 external + 2 components with URLs = 3 hits.
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Errorf("want 3 hits, got %d", got)
	}
}

func TestWaitForSystemReturnsFirstFailureWithComponentName(t *testing.T) {
	good := newOKServer(t)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer bad.Close()

	sys := SystemEntry{
		ExternalSystems: []Component{
			{Name: "ERP", URL: bad.URL},
		},
		Components: []Component{
			{Name: "Frontend", URL: good.URL},
		},
	}
	err := WaitForSystem(sys, HealthOptions{Attempts: 1, Interval: 1 * time.Millisecond, Timeout: 1 * time.Second})
	if err == nil {
		t.Fatal("want failure")
	}
	if !strings.Contains(err.Error(), "ERP") {
		t.Errorf("want failure to name 'ERP', got %v", err)
	}
}

func TestIsAnyURLUpTrueWhenAnyResponds(t *testing.T) {
	ts := newOKServer(t)
	sys := SystemEntry{
		ExternalSystems: []Component{
			{Name: "ERP", URL: ts.URL},
		},
	}
	if !IsAnyURLUp(sys, HealthOptions{Timeout: 1 * time.Second}) {
		t.Error("want IsAnyURLUp = true")
	}
}

func TestIsAnyURLUpFalseWhenNoneRespond(t *testing.T) {
	sys := SystemEntry{
		ExternalSystems: []Component{
			{Name: "ERP", URL: "http://127.0.0.1:1"}, // unreachable
		},
		Components: []Component{
			{Name: "F", URL: ""},
		},
	}
	if IsAnyURLUp(sys, HealthOptions{Timeout: 50 * time.Millisecond}) {
		t.Error("want IsAnyURLUp = false")
	}
}
