package sync

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestEnsureSynced_FirstRunWritesTreeAndStamp covers the happy path: a
// freshly-installed binary with no stamp file syncs the full global/
// embed and stamps the version.
func TestEnsureSynced_FirstRunWritesTreeAndStamp(t *testing.T) {
	home := t.TempDir()

	res, err := ensureSyncedAt(home, "v1.2.3")
	if err != nil {
		t.Fatalf("ensureSyncedAt: %v", err)
	}
	if !res.Synced {
		t.Errorf("expected Synced=true on first run")
	}
	if res.Version != "v1.2.3" {
		t.Errorf("Version = %q, want v1.2.3", res.Version)
	}

	stamp, err := os.ReadFile(filepath.Join(home, ".gh-optivem", ".version"))
	if err != nil {
		t.Fatalf("read stamp: %v", err)
	}
	if got := strings.TrimSpace(string(stamp)); got != "v1.2.3" {
		t.Errorf("stamp = %q, want v1.2.3", got)
	}

	// Spot-check that at least one known doc landed at the expected
	// mapped path. The exact file set comes from the embedded global/
	// subtree.
	for _, mustExist := range []string{
		filepath.Join(home, ".gh-optivem", "docs", "atdd", "architecture", "system.md"),
	} {
		if _, err := os.Stat(mustExist); err != nil {
			t.Errorf("expected synced file %s: %v", mustExist, err)
		}
	}
}

// TestEnsureSynced_StampMatchIsNoOp covers the steady-state path: a
// second invocation with the same binary version reads the stamp, sees a
// match, and returns Synced=false without touching the filesystem.
func TestEnsureSynced_StampMatchIsNoOp(t *testing.T) {
	home := t.TempDir()

	if _, err := ensureSyncedAt(home, "v1.0.0"); err != nil {
		t.Fatalf("first sync: %v", err)
	}

	// Mutate one of the synced files. A no-op call must NOT restore it.
	target := filepath.Join(home, ".gh-optivem", "docs", "atdd", "architecture", "system.md")
	if err := os.WriteFile(target, []byte("tampered"), 0o644); err != nil {
		t.Fatalf("tamper: %v", err)
	}

	res, err := ensureSyncedAt(home, "v1.0.0")
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if res.Synced {
		t.Errorf("expected Synced=false on matching stamp")
	}

	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(got) != "tampered" {
		t.Errorf("no-op sync overwrote the file; want tampered content preserved")
	}
}

// TestEnsureSynced_VersionBumpReSyncsAndWipesOwnedSubtree covers the
// upgrade path: a stamp from an older version triggers a fresh sync,
// and stale files inside the owned subtree are wiped (so removed-in-
// release content disappears from the user's disk).
func TestEnsureSynced_VersionBumpReSyncsAndWipesOwnedSubtree(t *testing.T) {
	home := t.TempDir()

	if _, err := ensureSyncedAt(home, "v1.0.0"); err != nil {
		t.Fatalf("v1.0.0 sync: %v", err)
	}

	// Plant a stray file inside an owned subtree. After re-sync it must
	// be gone (gh-optivem owns the subtree wholesale).
	stray := filepath.Join(home, ".gh-optivem", "docs", "atdd", "stray-file.md")
	if err := os.WriteFile(stray, []byte("stale"), 0o644); err != nil {
		t.Fatalf("plant stray: %v", err)
	}

	// Plant a non-owned file outside the owned subtree but adjacent to it.
	// After re-sync it must STILL be there (we promise the user we don't
	// touch anything outside ~/.gh-optivem/docs/atdd/).
	preserved := filepath.Join(home, ".gh-optivem", "notes", "private.md")
	if err := os.MkdirAll(filepath.Dir(preserved), 0o755); err != nil {
		t.Fatalf("mkdir preserved: %v", err)
	}
	if err := os.WriteFile(preserved, []byte("mine"), 0o644); err != nil {
		t.Fatalf("plant preserved: %v", err)
	}

	res, err := ensureSyncedAt(home, "v2.0.0")
	if err != nil {
		t.Fatalf("v2.0.0 sync: %v", err)
	}
	if !res.Synced {
		t.Errorf("expected Synced=true on version mismatch")
	}

	if _, err := os.Stat(stray); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stray file under owned subtree must be wiped on re-sync (err: %v)", err)
	}
	if _, err := os.Stat(preserved); err != nil {
		t.Errorf("non-owned file outside owned subtrees must survive re-sync: %v", err)
	}
}

// TestEnsureSynced_EscapeHatchSkipsSync verifies the escape-hatch env
// var disables the auto-sync. The Result reports Skipped=true and no
// stamp file is written.
func TestEnsureSynced_EscapeHatchSkipsSync(t *testing.T) {
	t.Setenv(EscapeHatchEnv, "1")

	if !IsEscapeHatchSet() {
		t.Fatalf("escape hatch should be detected when env=1")
	}

	res, err := EnsureSynced("v1.0.0")
	if err != nil {
		t.Fatalf("EnsureSynced: %v", err)
	}
	if !res.Skipped {
		t.Errorf("expected Skipped=true under escape hatch")
	}
	if res.Synced {
		t.Errorf("expected Synced=false under escape hatch")
	}
}

// TestStale_MissingStampIsStale covers the "no stamp file yet" case the
// fail-fast path under the escape hatch must handle: callers should
// treat the binary as stale when nothing is on disk.
func TestStale_MissingStampIsStale(t *testing.T) {
	home := t.TempDir()
	stale, err := staleAt(home, "v1.0.0")
	if err != nil {
		t.Fatalf("staleAt: %v", err)
	}
	if !stale {
		t.Errorf("expected Stale=true when stamp file is missing")
	}
}

// TestStale_MatchingStampIsNotStale verifies the steady-state path of
// the Stale() check used by fail-fast callers.
func TestStale_MatchingStampIsNotStale(t *testing.T) {
	home := t.TempDir()
	if _, err := ensureSyncedAt(home, "v1.0.0"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	stale, err := staleAt(home, "v1.0.0")
	if err != nil {
		t.Fatalf("staleAt: %v", err)
	}
	if stale {
		t.Errorf("expected Stale=false when stamp matches")
	}
}

// TestEnsureSynced_ConcurrentInvocationsConverge covers the concurrency
// promise: two processes both detecting a stale stamp can race the
// sync. Because the embedded FS is identical across processes and each
// file write is atomic, the final state is well-defined and identical
// to a single sync.
func TestEnsureSynced_ConcurrentInvocationsConverge(t *testing.T) {
	home := t.TempDir()

	const concurrency = 4
	errs := make(chan error, concurrency)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Go(func() {
			_, err := ensureSyncedAt(home, "v1.0.0")
			errs <- err
		})
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Errorf("concurrent sync errored: %v", err)
		}
	}

	stamp, err := os.ReadFile(filepath.Join(home, ".gh-optivem", ".version"))
	if err != nil {
		t.Fatalf("read stamp after concurrent: %v", err)
	}
	if got := strings.TrimSpace(string(stamp)); got != "v1.0.0" {
		t.Errorf("stamp after concurrent sync = %q, want v1.0.0", got)
	}

	// A known file must have ended up in place — proves at least one
	// race winner completed the full write sequence.
	if _, err := os.Stat(filepath.Join(home, ".gh-optivem", "docs", "atdd", "architecture", "system.md")); err != nil {
		t.Errorf("expected synced file after concurrent runs: %v", err)
	}
}

// TestMapDest_UnknownPrefixErrors guards against silent schema drift:
// any new top-level dir under embedded global/ that doesn't map to one
// of the documented destinations must fail loudly rather than land in
// an unexpected location on the user's disk.
func TestMapDest_UnknownPrefixErrors(t *testing.T) {
	home := t.TempDir()
	if _, err := mapDest(home, "unmapped/foo.md"); err == nil {
		t.Errorf("expected error for unmapped prefix")
	}
}

// TestIsEscapeHatchSet_Truthiness pins the env-var parsing so future
// edits don't accidentally regress acceptable truthy values.
func TestIsEscapeHatchSet_Truthiness(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"FALSE", false},
		{"no", false},
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"yes", true},
	}
	for _, tc := range cases {
		t.Run(tc.val, func(t *testing.T) {
			t.Setenv(EscapeHatchEnv, tc.val)
			if got := IsEscapeHatchSet(); got != tc.want {
				t.Errorf("IsEscapeHatchSet(%q) = %v, want %v", tc.val, got, tc.want)
			}
		})
	}
}
