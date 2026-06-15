package statemachine

import (
	"path/filepath"
	"testing"
)

// loadSnapshot loads the concrete ATDD process-flow document for the engine
// tests that exercise generic engine machinery (the channel-unroll transform,
// the graph-invariant checker) against the real flow as a fixture.
//
// The engine package embeds no process and cannot import the process package
// (that would be a cycle: process imports the engine). Instead the helper
// reads the YAML off disk via LoadFile, at the canonical relative path from
// this package directory — so there is no second copy of the document and no
// re-embed of a concrete process into the generic engine.
func loadSnapshot(t *testing.T) *Engine {
	t.Helper()
	path := filepath.Join("..", "..", "atdd", "process", "process-flow.yaml")
	eng, err := LoadFile(path)
	if err != nil {
		t.Fatalf("load process-flow snapshot from %s: %v", path, err)
	}
	return eng
}
