// Package sync writes the embedded global/ asset tree to per-user paths so
// methodology docs are reachable on the user's filesystem without
// per-repo install ceremony.
//
// On every gh-optivem invocation, EnsureSynced reads a per-user stamp file
// and compares it to the binary's version. On mismatch (first run after
// install or upgrade), the embedded global/ subtree is walked and written
// to:
//
//   - global/docs/    → ~/.gh-optivem/docs/   (owned: docs/atdd/)
//
// The atdd/ subdirectory under that root is entirely owned by gh-optivem —
// sync wipes-and-replaces it so files removed in a new release disappear
// from the user's disk. Anything outside the owned subtree
// (~/.gh-optivem/docs/notes/) is never touched.
//
// Concurrency: per-file atomic temp + rename means a killed sync never
// leaves a half-written file in place. Concurrent gh-optivem invocations
// write the same content (the embedded FS is identical across processes
// of the same binary), so the wipe-then-write race is benign — the
// final state is the same regardless of order. The stamp file is written
// last so a killed sync leaves no stale stamp.
//
// Escape hatch: setting GH_OPTIVEM_NO_AUTO_SYNC=1 (or any truthy value)
// disables sync. Callers that need fresh assets should consult Stale()
// under the escape hatch and fail fast with EscapeHatchHint.
package sync

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/optivem/gh-optivem/internal/assets"
)

// syncMu serialises ensureSyncedAt across goroutines in the same process.
// Without it, two callers can both observe a stale stamp and race the wipe
// + write — on Windows that surfaces as "Access is denied" errors on
// Rename when one goroutine is replacing a file the other is still writing.
// Cross-process serialisation is unnecessary in practice (only one
// gh-optivem invocation per terminal session) and would require
// platform-specific file locks we don't otherwise need.
var syncMu sync.Mutex

const (
	EscapeHatchEnv = "GH_OPTIVEM_NO_AUTO_SYNC"

	// EscapeHatchHint is the stable error phrasing for ATDD-consuming
	// commands that detect stale assets while the escape hatch is set.
	EscapeHatchHint = "gh-optivem assets out of date or missing. Run `gh optivem asset sync` or unset GH_OPTIVEM_NO_AUTO_SYNC."

	stampFileName      = ".version"
	embeddedGlobalRoot = "global"

	dirGhOptivem    = ".gh-optivem"
	embeddedDocsDir = "docs"
)

// ownedSubdirs lists the destination paths (relative to the user's home dir)
// that gh-optivem owns wholesale and wipes on every re-sync. Anything
// outside these paths is preserved across syncs.
var ownedSubdirs = []string{
	filepath.Join(dirGhOptivem, "docs", "atdd"),
}

// Result reports what EnsureSynced did.
type Result struct {
	// Synced is true only when EnsureSynced wrote files (stamp was missing
	// or mismatched). Subsequent invocations return false.
	Synced bool
	// Skipped is true when the escape hatch env var was set.
	Skipped bool
	// Version is the version recorded in the stamp file after sync (or
	// the version passed in when no sync was needed).
	Version string
	// Notice is a one-line message suitable for printing when Synced is
	// true. Empty otherwise.
	Notice string
}

// EnsureSynced is the entry point called at every gh-optivem invocation.
// Returns Result.Synced == false when the stamp matches; Result.Skipped
// == true when GH_OPTIVEM_NO_AUTO_SYNC is set; otherwise writes the
// embedded global/ subtree to per-user paths and updates the stamp.
func EnsureSynced(binaryVersion string) (Result, error) {
	if IsEscapeHatchSet() {
		return Result{Skipped: true, Version: binaryVersion}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Result{}, fmt.Errorf("locate home dir: %w", err)
	}
	return ensureSyncedAt(home, binaryVersion)
}

// ForceSync writes the embedded global/ subtree to per-user paths
// regardless of the stamp file and regardless of the escape hatch.
// Used by `gh optivem asset sync` — the explicit-force form a user
// invokes when the auto-sync gate has been bypassed or when they
// want a clean re-write.
func ForceSync(binaryVersion string) (Result, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Result{}, fmt.Errorf("locate home dir: %w", err)
	}
	syncMu.Lock()
	defer syncMu.Unlock()
	if err := syncAllAt(home, binaryVersion); err != nil {
		return Result{}, err
	}
	return Result{
		Synced:  true,
		Version: binaryVersion,
		Notice:  fmt.Sprintf("Synced gh-optivem assets to ~/.gh-optivem (%s).", binaryVersion),
	}, nil
}

// DocsRoot returns the absolute path of the synced docs root
// (~/.gh-optivem/docs/). Rendered prompts substitute this into the
// ${docs_root} placeholder so agent Read-tool calls resolve to the
// per-user synced copy regardless of working directory.
func DocsRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate home dir: %w", err)
	}
	return filepath.Join(home, dirGhOptivem, "docs"), nil
}

// Stale reports whether the on-disk stamp matches the binary version.
// True when the stamp file is missing or mismatched. Used by ATDD-
// consuming commands under the escape hatch.
func Stale(binaryVersion string) (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return true, fmt.Errorf("locate home dir: %w", err)
	}
	return staleAt(home, binaryVersion)
}

// IsEscapeHatchSet returns true when the GH_OPTIVEM_NO_AUTO_SYNC env var
// is set to a truthy value (1, true, yes — case-insensitive). Empty,
// "0", "false", "no" all read as unset.
func IsEscapeHatchSet() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(EscapeHatchEnv)))
	if v == "" {
		return false
	}
	if b, err := strconv.ParseBool(v); err == nil {
		return b
	}
	return v != "false" && v != "0" && v != "no"
}

func ensureSyncedAt(home, binaryVersion string) (Result, error) {
	stale, err := staleAt(home, binaryVersion)
	if err != nil {
		return Result{}, err
	}
	if !stale {
		return Result{Version: binaryVersion}, nil
	}

	// Double-checked locking: re-read the stamp under the mutex so a
	// concurrent goroutine that already synced lets us short-circuit
	// without a redundant wipe-and-write.
	syncMu.Lock()
	defer syncMu.Unlock()
	stale, err = staleAt(home, binaryVersion)
	if err != nil {
		return Result{}, err
	}
	if !stale {
		return Result{Version: binaryVersion}, nil
	}

	if err := syncAllAt(home, binaryVersion); err != nil {
		return Result{}, err
	}
	return Result{
		Synced:  true,
		Version: binaryVersion,
		Notice:  fmt.Sprintf("Synced gh-optivem assets to ~/.gh-optivem (%s).", binaryVersion),
	}, nil
}

func staleAt(home, binaryVersion string) (bool, error) {
	data, err := os.ReadFile(stampPathAt(home))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return true, nil
		}
		return true, fmt.Errorf("read %s: %w", stampPathAt(home), err)
	}
	return strings.TrimSpace(string(data)) != binaryVersion, nil
}

func stampPathAt(home string) string {
	return filepath.Join(home, dirGhOptivem, stampFileName)
}

func syncAllAt(home, binaryVersion string) error {
	for _, rel := range ownedSubdirs {
		owned := filepath.Join(home, rel)
		if err := os.RemoveAll(owned); err != nil {
			return fmt.Errorf("wipe %s: %w", owned, err)
		}
	}

	walkErr := fs.WalkDir(assets.FS, embeddedGlobalRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, embeddedGlobalRoot+"/")
		dest, err := mapDest(home, rel)
		if err != nil {
			return err
		}
		data, err := assets.FS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read embedded %s: %w", path, err)
		}
		return atomicWriteFile(dest, data)
	})
	if walkErr != nil {
		return walkErr
	}

	return atomicWriteFile(stampPathAt(home), []byte(binaryVersion+"\n"))
}

// mapDest converts an embedded global/-relative path to its on-disk
// destination under home. Returns an error for paths outside the known
// prefix (docs/) — a guard against schema drift in the embedded tree
// leaking files to unexpected locations.
func mapDest(home, rel string) (string, error) {
	switch {
	case strings.HasPrefix(rel, embeddedDocsDir+"/"):
		return filepath.Join(home, dirGhOptivem, rel), nil
	default:
		return "", fmt.Errorf("sync: unmapped embedded path %q (expected docs/*)", rel)
	}
}

// atomicWriteFile writes data to path via temp + rename so a killed sync
// never leaves a half-written file in place. Parent directories are
// created as needed.
func atomicWriteFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create dir for %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp.*")
	if err != nil {
		return fmt.Errorf("create temp for %s: %w", path, err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp for %s: %w", path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp for %s: %w", path, err)
	}
	return nil
}
