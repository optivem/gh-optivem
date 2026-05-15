package shell

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRoundTripper routes every outgoing request to a single test handler,
// regardless of host/scheme. Lets us point SonarCloud's hardcoded
// https://sonarcloud.io URL at a test handler without exposing a baseURL.
type fakeRoundTripper struct {
	handler http.HandlerFunc
}

func (f *fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	f.handler(rec, req)
	return rec.Result(), nil
}

func newSonarTestClient(t *testing.T, handler http.HandlerFunc) *SonarCloud {
	t.Helper()
	s := NewSonarCloud("tok", "myorg")
	s.client = &http.Client{Transport: &fakeRoundTripper{handler: handler}}
	return s
}

// TestSonarCloud_api_RetriesOn5xxThenSucceeds pins the retry contract: a
// single logical api() call that observes 504 twice then 200 must hit the
// transport three times and ultimately surface the 200 result.
func TestSonarCloud_api_RetriesOn5xxThenSucceeds(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	var calls int32
	handler := func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		switch n {
		case 1, 2:
			w.WriteHeader(504)
			_, _ = w.Write([]byte(`{"error":"Gateway Timeout"}`))
		default:
			w.WriteHeader(200)
			_, _ = w.Write([]byte(`{"organizations":[{"key":"myorg"}]}`))
		}
	}
	s := newSonarTestClient(t, handler)

	result, err := s.api(context.Background(), "GET", "/organizations/search?organizations=myorg", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("transport calls = %d, want 3 (504, 504, 200)", got)
	}
	if _, hasErr := result["error"]; hasErr {
		t.Fatalf("result unexpectedly carries error after success: %v", result)
	}
	if _, ok := result["organizations"]; !ok {
		t.Fatalf("result missing organizations field: %v", result)
	}
	if len(sleeps) != 2 {
		t.Fatalf("sleeps = %d, want 2 backoffs between 3 attempts", len(sleeps))
	}
}

// TestSonarCloud_api_HardFailOn4xxNoRetry pins the hard-fail contract: a 401
// must surface immediately as result["error"]=true with status=401, with no
// retries attempted.
func TestSonarCloud_api_HardFailOn4xxNoRetry(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	var calls int32
	handler := func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"errors":[{"msg":"Unauthorized"}]}`))
	}
	s := newSonarTestClient(t, handler)

	result, err := s.api(context.Background(), "GET", "/organizations/search?organizations=myorg", nil)
	if err != nil {
		t.Fatalf("unexpected transport error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("transport calls = %d, want 1 (no retry on 4xx)", got)
	}
	if e, _ := result["error"].(bool); !e {
		t.Fatalf("result missing error=true: %v", result)
	}
	if status, _ := result["status"].(float64); int(status) != 401 {
		t.Fatalf("result status = %v, want 401", result["status"])
	}
	if len(sleeps) != 0 {
		t.Fatalf("sleeps = %d, want 0 — hard-fail must not sleep", len(sleeps))
	}
}

// TestSonarCloud_api_PostBodyResentOnRetry confirms the form body is rebuilt
// each attempt — without that, the second attempt would send an empty body
// (the strings.Reader from the first attempt is exhausted).
func TestSonarCloud_api_PostBodyResentOnRetry(t *testing.T) {
	var sleeps []time.Duration
	withFakeSleep(t, &sleeps)

	var bodies []string
	handler := func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		bodies = append(bodies, string(buf))
		if len(bodies) == 1 {
			w.WriteHeader(503)
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}
	s := newSonarTestClient(t, handler)

	_, err := s.api(context.Background(), "POST", "/projects/create", map[string]string{
		"organization": "myorg", "project": "k", "name": "k",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("got %d bodies, want 2", len(bodies))
	}
	for i, b := range bodies {
		if !strings.Contains(b, "project=k") {
			t.Errorf("attempt %d body missing project=k: %q", i+1, b)
		}
	}
}
