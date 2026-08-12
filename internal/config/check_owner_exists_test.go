package config

import (
	"strings"
	"testing"
)

// TestRealCheckOwnerExists_NotFound pins the one case that licenses the
// "no such owner" verdict: both the users/ and orgs/ probes answer HTTP 404.
func TestRealCheckOwnerExists_NotFound(t *testing.T) {
	dir := mkPathDir(t)
	// The message is quoted because a stub body must parse as both /bin/sh and
	// cmd.exe: unquoted `(` is a POSIX-shell syntax error. cmd.exe echoes the
	// quotes literally, which is harmless — every assertion below is a
	// substring check on `(HTTP 404)` / `HTTP 401`.
	writeStub(t, dir, "gh", "echo \"gh: Not Found (HTTP 404)\" >&2\nexit 1")

	err := realCheckOwnerExists("ghost-owner")
	if err == nil {
		t.Fatal("expected an error when both probes 404, got nil")
	}
	if !strings.Contains(err.Error(), `no GitHub user or organization named "ghost-owner"`) {
		t.Fatalf("expected the definitive not-found verdict, got:\n%s", err)
	}
}

// TestRealCheckOwnerExists_AuthFailureIsNotAVerdict is the regression guard
// for the misdiagnosis this function used to produce: an unauthenticated or
// expired gh token made every real owner look nonexistent. A 401 is not an
// answer to "does this owner exist" — the error must say so, echo gh's own
// stderr, and point at `gh auth status`.
func TestRealCheckOwnerExists_AuthFailureIsNotAVerdict(t *testing.T) {
	dir := mkPathDir(t)
	writeStub(t, dir, "gh", "echo \"gh: Requires authentication (HTTP 401)\" >&2\nexit 1")

	err := realCheckOwnerExists("real-owner")
	if err == nil {
		t.Fatal("expected an error when the probe cannot reach GitHub, got nil")
	}
	msg := err.Error()
	if strings.Contains(msg, "no GitHub user or organization named") {
		t.Fatalf("a 401 must not be reported as a missing owner, got:\n%s", msg)
	}
	for _, want := range []string{"could not verify", "HTTP 401", "gh auth status"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected %q in the error, got:\n%s", want, msg)
		}
	}
}

// TestRealCheckOwnerExists_GhMissingIsNotAVerdict covers the other
// indeterminate shape: gh is not on PATH at all, so exec fails before any
// HTTP status exists. Still not a "no such owner" answer.
func TestRealCheckOwnerExists_GhMissingIsNotAVerdict(t *testing.T) {
	mkPathDir(t) // empty PATH — no gh stub planted

	err := realCheckOwnerExists("real-owner")
	if err == nil {
		t.Fatal("expected an error when gh is missing from PATH, got nil")
	}
	if strings.Contains(err.Error(), "no GitHub user or organization named") {
		t.Fatalf("a missing gh binary must not be reported as a missing owner, got:\n%s", err)
	}
	if !strings.Contains(err.Error(), "could not verify") {
		t.Fatalf("expected the undecided-probe wording, got:\n%s", err)
	}
}
