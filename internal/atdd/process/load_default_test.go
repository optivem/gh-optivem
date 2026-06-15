package process_test

import (
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/process"
)

// TestLoad_ApprovalCategory_ShippedYAMLLoads guards against regressing
// the shipped process-flow.yaml's category coverage.
func TestLoad_ApprovalCategory_ShippedYAMLLoads(t *testing.T) {
	if _, err := process.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
}

// TestLoad_ScopeRationale_ShippedYAML_TestWriters guards that the
// shipped process-flow.yaml carries scope-rationale: on both test-writer
// MIDs (plan 20260528-1038). Regressing this field would silently lose
// the per-MID *why* the dispatcher renders into ${scope-block}.
func TestLoad_ScopeRationale_ShippedYAML_TestWriters(t *testing.T) {
	eng, err := process.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, name := range []string{"write-acceptance-tests", "write-contract-tests"} {
		got, ok := eng.ScopeRationale(name)
		if !ok {
			t.Errorf("Engine.ScopeRationale(%s): want ok=true", name)
			continue
		}
		if !strings.Contains(got, "dsl-core") || !strings.Contains(got, "TODO: DSL") {
			t.Errorf("Engine.ScopeRationale(%s) = %q, want substring about dsl-core / TODO: DSL", name, got)
		}
	}
}
