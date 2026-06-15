package statemachine

import (
	"strings"
	"testing"
)

// minimalWithOutputs returns a one-process YAML doc whose `outputs:` block
// is replaced by the caller-provided fragment (indented two spaces under
// the process). Used to exercise the OutputSpec parser/validator in
// isolation from the rest of the schema.
func minimalWithOutputs(outputsFragment string) string {
	return `
processes:
  sample-mid:
    name: "Sample MID"
    start: END
` + outputsFragment + `
    nodes:
      - id: END
        type: end-event
        name: "Done"
    sequence-flows: []
`
}

func TestLoadBytes_OutputSpec_ListOfObjectsParses(t *testing.T) {
	yaml := minimalWithOutputs(`    outputs:
      - key: dsl-port-changed
        type: bool
      - key: test-names
        type: string-list
        optional: true
      - key: scope-exception-reason
        type: string
        optional: true`)

	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	got, ok := eng.Outputs("sample-mid")
	if !ok {
		t.Fatalf("Engine.Outputs(sample-mid): not found")
	}
	if len(got) != 3 {
		t.Fatalf("len(outputs) = %d, want 3", len(got))
	}
	if got[0].Key != "dsl-port-changed" || got[0].Type != OutputTypeBool || got[0].Optional {
		t.Errorf("outputs[0] = %+v, want {dsl-port-changed bool false}", got[0])
	}
	if got[1].Key != "test-names" || got[1].Type != OutputTypeStringList || !got[1].Optional {
		t.Errorf("outputs[1] = %+v, want {test-names string-list true}", got[1])
	}
	if got[2].Key != "scope-exception-reason" || got[2].Type != OutputTypeString || !got[2].Optional {
		t.Errorf("outputs[2] = %+v, want {scope-exception-reason string true}", got[2])
	}
}

func TestLoadBytes_OutputSpec_EmptyListAllowed(t *testing.T) {
	yaml := minimalWithOutputs(``)
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	got, ok := eng.Outputs("sample-mid")
	if !ok {
		t.Fatalf("Engine.Outputs(sample-mid): not found")
	}
	if len(got) != 0 {
		t.Errorf("outputs = %v, want empty", got)
	}
}

func TestLoadBytes_OutputSpec_LegacyStringCSVRejected(t *testing.T) {
	yaml := minimalWithOutputs(`    outputs: "dsl-port-changed"`)
	_, err := LoadBytes([]byte(yaml))
	if err == nil {
		t.Fatalf("LoadBytes: expected error for legacy string-CSV form, got nil")
	}
	// yaml.v3's natural "cannot unmarshal !!str into ..." surfaces from
	// the parser; we don't require an exact match, but the failure must
	// be loud enough that an operator sees the form is wrong.
	if !strings.Contains(err.Error(), "outputs") && !strings.Contains(err.Error(), "OutputSpec") {
		t.Errorf("error %q does not mention outputs/OutputSpec", err)
	}
}

func TestLoadBytes_OutputSpec_MissingKeyRejected(t *testing.T) {
	yaml := minimalWithOutputs(`    outputs:
      - type: bool`)
	_, err := LoadBytes([]byte(yaml))
	if err == nil {
		t.Fatalf("LoadBytes: expected error for missing key, got nil")
	}
	if !strings.Contains(err.Error(), "missing `key:`") {
		t.Errorf("error %q does not mention missing key", err)
	}
}

func TestLoadBytes_OutputSpec_MissingTypeRejected(t *testing.T) {
	yaml := minimalWithOutputs(`    outputs:
      - key: dsl-port-changed`)
	_, err := LoadBytes([]byte(yaml))
	if err == nil {
		t.Fatalf("LoadBytes: expected error for missing type, got nil")
	}
	if !strings.Contains(err.Error(), "missing `type:`") {
		t.Errorf("error %q does not mention missing type", err)
	}
}

func TestLoadBytes_OutputSpec_UnknownTypeRejected(t *testing.T) {
	yaml := minimalWithOutputs(`    outputs:
      - key: dsl-port-changed
        type: int`)
	_, err := LoadBytes([]byte(yaml))
	if err == nil {
		t.Fatalf("LoadBytes: expected error for unknown type, got nil")
	}
	if !strings.Contains(err.Error(), "not one of") {
		t.Errorf("error %q does not mention the allow-list", err)
	}
}

func TestLoadBytes_OutputSpec_DuplicateKeyRejected(t *testing.T) {
	yaml := minimalWithOutputs(`    outputs:
      - key: dsl-port-changed
        type: bool
      - key: dsl-port-changed
        type: string`)
	_, err := LoadBytes([]byte(yaml))
	if err == nil {
		t.Fatalf("LoadBytes: expected error for duplicate key, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate key") {
		t.Errorf("error %q does not mention duplicate key", err)
	}
}

func TestLoadBytes_OutputSpec_OptionalDefaultsToFalse(t *testing.T) {
	yaml := minimalWithOutputs(`    outputs:
      - key: dsl-port-changed
        type: bool`)
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	got, _ := eng.Outputs("sample-mid")
	if len(got) != 1 || got[0].Optional {
		t.Errorf("outputs[0].Optional = true, want false (default)")
	}
}

func TestEngine_Outputs_UnknownProcess(t *testing.T) {
	yaml := minimalWithOutputs(``)
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if _, ok := eng.Outputs("nonexistent"); ok {
		t.Errorf("Engine.Outputs(nonexistent): want ok=false")
	}
}

// TestLoadBytes_ApprovalCategory_MissingErrors checks that a writing-agent
// MID whose EXECUTE_AGENT call-activity omits `category:` is rejected at
// load with the offending node named and the valid-set listed.
func TestLoadBytes_ApprovalCategory_MissingErrors(t *testing.T) {
	yaml := `
processes:
  some-mid:
    name: "Some MID"
    start: EXECUTE_AGENT
    nodes:
      - id: EXECUTE_AGENT
        type: call-activity
        process: execute-agent
        name: "Dispatch the Agent"
        params:
          task-name: some-task
          agent: some-agent
      - id: MID_END
        type: end-event
        name: "Done"
    sequence-flows:
      - {from: EXECUTE_AGENT, to: MID_END}
  execute-agent:
    name: "Execute Agent"
    start: RUN_AGENT
    nodes:
      - id: RUN_AGENT
        type: user-task
        agent: ${agent}
        name: "Run agent"
      - id: PRIM_END
        type: end-event
        name: "Done"
    sequence-flows:
      - {from: RUN_AGENT, to: PRIM_END}
`
	_, err := LoadBytes([]byte(yaml))
	if err == nil {
		t.Fatal("expected load error for missing category on EXECUTE_AGENT, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "some-mid") || !strings.Contains(msg, "EXECUTE_AGENT") {
		t.Errorf("error should name the offending process/node, got %q", msg)
	}
	if !strings.Contains(msg, "prod-agent") || !strings.Contains(msg, "human") {
		t.Errorf("error should list valid set, got %q", msg)
	}
}

// TestLoadBytes_ApprovalCategory_InvalidTokenErrors checks that an unknown
// category token (`category: foo`) is rejected at load.
func TestLoadBytes_ApprovalCategory_InvalidTokenErrors(t *testing.T) {
	yaml := `
processes:
  some-mid:
    name: "Some MID"
    start: EXECUTE_AGENT
    nodes:
      - id: EXECUTE_AGENT
        type: call-activity
        process: execute-agent
        name: "Dispatch the Agent"
        params:
          task-name: some-task
          agent: some-agent
          category: foo
      - id: MID_END
        type: end-event
        name: "Done"
    sequence-flows:
      - {from: EXECUTE_AGENT, to: MID_END}
  execute-agent:
    name: "Execute Agent"
    start: RUN_AGENT
    nodes:
      - id: RUN_AGENT
        type: user-task
        agent: ${agent}
        name: "Run agent"
      - id: PRIM_END
        type: end-event
        name: "Done"
    sequence-flows:
      - {from: RUN_AGENT, to: PRIM_END}
`
	_, err := LoadBytes([]byte(yaml))
	if err == nil {
		t.Fatal("expected load error for invalid category, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "foo") {
		t.Errorf("error should mention the offending token, got %q", msg)
	}
	if !strings.Contains(msg, "human") || !strings.Contains(msg, "command") {
		t.Errorf("error should list valid set, got %q", msg)
	}
}

// midWithScope returns a one-process YAML doc whose EXECUTE_AGENT
// call-activity carries the caller-provided read/write/rationale lines
// (each may be empty). Used to exercise Engine.ScopeRationale in
// isolation from the rest of the schema.
func midWithScope(scopeFragment string) string {
	return `
processes:
  sample-mid:
    name: "Sample MID"
    start: EXECUTE_AGENT
    nodes:
      - id: EXECUTE_AGENT
        type: call-activity
        process: execute-agent
        name: "Dispatch the Agent"
        params:
          task-name: sample-task
          agent: sample-agent
          category: test-agent
` + scopeFragment + `
      - id: MID_END
        type: end-event
        name: "Done"
    sequence-flows:
      - {from: EXECUTE_AGENT, to: MID_END}
  execute-agent:
    name: "Execute Agent"
    start: RUN_AGENT
    nodes:
      - id: RUN_AGENT
        type: user-task
        agent: ${agent}
        name: "Run agent"
      - id: PRIM_END
        type: end-event
        name: "Done"
    sequence-flows:
      - {from: RUN_AGENT, to: PRIM_END}
`
}

// TestEngine_ScopeRationale_Present covers the rationale-present path:
// a writing-agent MID declares scope-rationale: alongside read:/write:,
// and Engine.ScopeRationale returns the string with ok=true.
func TestEngine_ScopeRationale_Present(t *testing.T) {
	yaml := midWithScope(`        read:  [at-test, dsl-port]
        write: [at-test, dsl-port, dsl-core]
        scope-rationale: |
          dsl-core is write-only because reading impl would leak context.`)
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	got, ok := eng.ScopeRationale("sample-mid")
	if !ok {
		t.Fatalf("Engine.ScopeRationale(sample-mid): want ok=true, got false")
	}
	if !strings.Contains(got, "write-only because reading impl would leak context") {
		t.Errorf("rationale = %q, want substring about leaking context", got)
	}
}

// TestEngine_ScopeRationale_Absent covers the common case: read:/write:
// declared but no scope-rationale: — Engine.ScopeRationale returns
// "" / ok=false.
func TestEngine_ScopeRationale_Absent(t *testing.T) {
	yaml := midWithScope(`        read:  [at-test, dsl-port]
        write: [at-test, dsl-port]`)
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if got, ok := eng.ScopeRationale("sample-mid"); ok || got != "" {
		t.Errorf("Engine.ScopeRationale(sample-mid) = (%q, %v), want (\"\", false)", got, ok)
	}
}

// TestEngine_ScopeRationale_UnknownProcess mirrors
// TestEngine_Outputs_UnknownProcess: an unknown process name returns
// ok=false.
func TestEngine_ScopeRationale_UnknownProcess(t *testing.T) {
	yaml := midWithScope(`        read:  [at-test]
        write: [at-test]`)
	eng, err := LoadBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if _, ok := eng.ScopeRationale("nonexistent"); ok {
		t.Errorf("Engine.ScopeRationale(nonexistent): want ok=false")
	}
}
