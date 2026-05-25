package agents

import (
	"strings"
	"testing"
)

func TestSplitFrontmatter(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantFM  string
		wantOut string
	}{
		{
			name:    "no frontmatter",
			in:      "You are an agent.\n",
			wantFM:  "",
			wantOut: "You are an agent.\n",
		},
		{
			name:    "well-formed",
			in:      "---\nmodel: sonnet\neffort: medium\n---\nYou are an agent.\n",
			wantFM:  "model: sonnet\neffort: medium\n",
			wantOut: "You are an agent.\n",
		},
		{
			name:    "frontmatter with comments",
			in:      "---\n# tuning notes\nmodel: haiku\n---\nbody",
			wantFM:  "# tuning notes\nmodel: haiku\n",
			wantOut: "body",
		},
		{
			name: "CRLF line endings",
			// Windows operators may save the file with CRLF — splitter must cope.
			in:      "---\r\nmodel: sonnet\r\n---\r\nBody line.\r\n",
			wantFM:  "model: sonnet\r\n",
			wantOut: "Body line.\r\n",
		},
		{
			name: "missing closing marker",
			// Without a closing `---`, treat the whole file as body so the
			// agent prompt still loads (degraded → no tuning) instead of
			// silently swallowing content.
			in:      "---\nmodel: sonnet\nYou are an agent.\n",
			wantFM:  "",
			wantOut: "---\nmodel: sonnet\nYou are an agent.\n",
		},
		{
			name:    "leading whitespace before opening marker",
			in:      "  ---\nmodel: sonnet\n---\nbody",
			wantFM:  "",
			wantOut: "  ---\nmodel: sonnet\n---\nbody",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotFM, gotOut := splitFrontmatter(tt.in)
			if gotFM != tt.wantFM {
				t.Errorf("frontmatter:\n got: %q\nwant: %q", gotFM, tt.wantFM)
			}
			if gotOut != tt.wantOut {
				t.Errorf("body:\n got: %q\nwant: %q", gotOut, tt.wantOut)
			}
		})
	}
}

func TestLoadTuning_WriteAcceptanceTests(t *testing.T) {
	// Pins the write-acceptance-tests frontmatter so a careless edit
	// doesn't silently drop the per-agent tuning back to session
	// defaults (Opus + max effort) — which would re-introduce the cost
	// spike the frontmatter was added to fix.
	got, err := LoadTuning("write-acceptance-tests")
	if err != nil {
		t.Fatalf("LoadTuning: %v", err)
	}
	if got.Model != "sonnet" {
		t.Errorf("Model = %q, want %q", got.Model, "sonnet")
	}
	if got.Effort != "medium" {
		t.Errorf("Effort = %q, want %q", got.Effort, "medium")
	}
}

func TestLoadTuning_EveryAgentDeclaresTuning(t *testing.T) {
	// Mandatory-frontmatter contract: every embedded agent must declare
	// model + effort. A new agent added without frontmatter would
	// silently inherit the operator's session default (typically Opus +
	// max) — this test catches that at build time instead of at the
	// first surprising bill.
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			tuning, err := LoadTuning(name)
			if err != nil {
				t.Fatalf("LoadTuning(%q): %v", name, err)
			}
			if tuning.Model == "" {
				t.Errorf("Model is empty")
			}
			if tuning.Effort == "" {
				t.Errorf("Effort is empty")
			}
		})
	}
}

func TestParseTuningFrontmatter_Rejections(t *testing.T) {
	// Validator regression coverage — keeps the contract decoupled from
	// which embedded agents happen to exist today.
	tests := []struct {
		name    string
		fm      string
		wantSub string
	}{
		{"empty", "", "no frontmatter"},
		{"missing both", "# only a comment\n", "missing required `model:`"},
		{"missing effort", "model: sonnet\n", "missing required `effort:`"},
		{"missing model", "effort: medium\n", "missing required `model:`"},
		{"malformed yaml", "model: : :\n", "parse frontmatter"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseTuningFrontmatter(tt.fm)
			if err == nil {
				t.Fatalf("got nil error, want substring %q", tt.wantSub)
			}
			if !strings.Contains(err.Error(), tt.wantSub) {
				t.Errorf("got err %q, want substring %q", err.Error(), tt.wantSub)
			}
		})
	}
}

func TestParseTuningFrontmatter_Accepts(t *testing.T) {
	got, err := parseTuningFrontmatter("model: sonnet\neffort: medium\n")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.Model != "sonnet" || got.Effort != "medium" {
		t.Errorf("got %+v, want {sonnet medium}", got)
	}
}

func TestFixKindPromptsExist(t *testing.T) {
	// Pins the closed set of fix-* failure-kinds the YAML's
	// `task-name: "fix-${failure-kind}"` placeholder (process-flow.yaml,
	// `fix` MID) resolves against. Every kind here must have a matching
	// prompt embedded under internal/assets/runtime/prompts/atdd/.
	//
	// Phase D will wire the binding that emits `failure-kind`; until
	// then this test guards the convention end-to-end at the prompt
	// level — adding a new kind requires adding a fix-<kind>.md, and a
	// prompt rename will fail this test before runtime sees an unknown
	// task-name.
	wantKinds := []string{
		"unexpected-passing-tests",
		"unexpected-failing-tests",
	}
	names := map[string]bool{}
	for _, n := range Names() {
		names[n] = true
	}
	for _, kind := range wantKinds {
		taskName := "fix-" + kind
		if !names[taskName] {
			t.Errorf("missing prompt for failure-kind %q (expected agents.Names() to include %q)", kind, taskName)
		}
	}
}

func TestPrompt_StripsFrontmatter(t *testing.T) {
	// The dispatched prompt must NOT contain the frontmatter — it
	// would confuse the agent (and waste tokens). Walk every embedded
	// agent so a future prompt that adopts frontmatter is covered for
	// free.
	for _, name := range Names() {
		t.Run(name, func(t *testing.T) {
			body, err := Prompt(name)
			if err != nil {
				t.Fatalf("Prompt: %v", err)
			}
			// The shared preamble is prepended, so search past it.
			afterPreamble := strings.TrimPrefix(body, sharedPreamble+"\n\n")
			if strings.HasPrefix(afterPreamble, "---") {
				t.Errorf("prompt body still starts with frontmatter marker:\n%s", afterPreamble[:120])
			}
		})
	}
}
