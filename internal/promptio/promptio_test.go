package promptio

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestConfirmYN_AcceptsYesVariants(t *testing.T) {
	for _, in := range []string{"y\n", "Y\n", "yes\n", "YES\n", "  yes  \n"} {
		var out bytes.Buffer
		ok, err := ConfirmYN(strings.NewReader(in), &out, "Approve?")
		if err != nil {
			t.Errorf("input %q: unexpected err %v", in, err)
		}
		if !ok {
			t.Errorf("input %q: expected true, got false", in)
		}
	}
}

func TestConfirmYN_AcceptsNoVariants(t *testing.T) {
	for _, in := range []string{"n\n", "N\n", "no\n", "NO\n", "  no  \n"} {
		var out bytes.Buffer
		ok, err := ConfirmYN(strings.NewReader(in), &out, "Approve?")
		if err != nil {
			t.Errorf("input %q: unexpected err %v", in, err)
		}
		if ok {
			t.Errorf("input %q: expected false, got true", in)
		}
	}
}

func TestConfirmYN_BareEnterReprompts(t *testing.T) {
	// Empty line is not a valid answer — bare Enter must re-prompt rather
	// than be coerced to either default. This is the property that
	// distinguishes the no-defaults design from the legacy [y/N]/[Y/n]
	// behaviour and the originating incident (Enter accidentally declined
	// a commit that was supposed to land).
	var out bytes.Buffer
	ok, err := ConfirmYN(strings.NewReader("\n\ny\n"), &out, "Approve?")
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if !ok {
		t.Fatalf("expected true after two reprompts, got false")
	}
	if got := strings.Count(out.String(), "Approve? [y/n]: "); got != 3 {
		t.Errorf("expected prompt reprinted 3 times, got %d: %q", got, out.String())
	}
	if got := strings.Count(out.String(), "Please answer y or n."); got != 2 {
		t.Errorf("expected please-answer reprinted 2 times, got %d: %q", got, out.String())
	}
}

func TestConfirmYN_UnrecognisedReprompts(t *testing.T) {
	var out bytes.Buffer
	ok, err := ConfirmYN(strings.NewReader("maybe\nhuh\nn\n"), &out, "Approve?")
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if ok {
		t.Fatalf("expected false after reprompt, got true")
	}
	if got := strings.Count(out.String(), "Please answer y or n."); got != 2 {
		t.Errorf("expected please-answer reprinted 2 times, got %d: %q", got, out.String())
	}
}

func TestConfirmYN_EOFReturnsFalse(t *testing.T) {
	// Stream ends without a valid answer → return false rather than spin
	// forever reading empty lines from a closed stdin. Non-interactive
	// callers rely on this "silence = no" terminator.
	var out bytes.Buffer
	ok, err := ConfirmYN(strings.NewReader("maybe\n"), &out, "Approve?")
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if ok {
		t.Fatalf("expected false on EOF, got true")
	}
}

func TestConfirmYN_AppendsHint(t *testing.T) {
	var out bytes.Buffer
	if _, err := ConfirmYN(strings.NewReader("y\n"), &out, "Approve?"); err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if !strings.Contains(out.String(), "Approve? [y/n]: ") {
		t.Errorf("expected helper to append [y/n] hint, got: %q", out.String())
	}
}

// ---------------------------------------------------------------------------
// ConfirmYNVia
// ---------------------------------------------------------------------------

// scriptedAsker replays a fixed slice of (answer, error) pairs in order,
// failing the test if Ask is called more times than scripted.
type scriptedAsker struct {
	t      *testing.T
	calls  []scriptedCall
	index  int
	asked  []string
}

type scriptedCall struct {
	answer string
	err    error
}

func (s *scriptedAsker) Ask(prompt string) (string, error) {
	s.asked = append(s.asked, prompt)
	if s.index >= len(s.calls) {
		s.t.Fatalf("Ask called %d times, scripted only %d", s.index+1, len(s.calls))
	}
	c := s.calls[s.index]
	s.index++
	return c.answer, c.err
}

func TestConfirmYNVia_AcceptsYes(t *testing.T) {
	asker := &scriptedAsker{t: t, calls: []scriptedCall{{answer: "y\n"}}}
	var out bytes.Buffer
	ok, err := ConfirmYNVia(asker, &out, "Approve?")
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if !ok {
		t.Fatalf("expected true, got false")
	}
	if asker.asked[0] != "Approve? [y/n]: " {
		t.Errorf("expected hint suffix, got: %q", asker.asked[0])
	}
}

func TestConfirmYNVia_RepromptsOnUnrecognised(t *testing.T) {
	asker := &scriptedAsker{t: t, calls: []scriptedCall{
		{answer: "maybe\n"},
		{answer: "n\n"},
	}}
	var out bytes.Buffer
	ok, err := ConfirmYNVia(asker, &out, "Approve?")
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if ok {
		t.Fatalf("expected false, got true")
	}
	if got := strings.Count(out.String(), "Please answer y or n."); got != 1 {
		t.Errorf("expected please-answer once, got %d: %q", got, out.String())
	}
}

func TestConfirmYNVia_EmptyReplyReturnsFalse(t *testing.T) {
	// Prompter contract: an empty reply with no error is how the
	// stdinPrompter implementations signal EOF. Treat it as decline so
	// non-interactive runs don't loop.
	asker := &scriptedAsker{t: t, calls: []scriptedCall{{answer: ""}}}
	var out bytes.Buffer
	ok, err := ConfirmYNVia(asker, &out, "Approve?")
	if err != nil {
		t.Fatalf("unexpected err %v", err)
	}
	if ok {
		t.Fatalf("expected false, got true")
	}
}

func TestConfirmYNVia_AskerErrorPropagates(t *testing.T) {
	want := errors.New("asker broke")
	asker := &scriptedAsker{t: t, calls: []scriptedCall{{err: want}}}
	ok, err := ConfirmYNVia(asker, io.Discard, "Approve?")
	if !errors.Is(err, want) {
		t.Fatalf("expected wrapped err, got %v", err)
	}
	if ok {
		t.Fatalf("expected false on error, got true")
	}
}

// ---------------------------------------------------------------------------
// ParseYN — coercion used outside the prompt loops
// ---------------------------------------------------------------------------

func TestParseYN(t *testing.T) {
	cases := []struct {
		in    string
		value bool
		ok    bool
	}{
		{"y", true, true}, {"Y", true, true}, {"yes", true, true},
		{"YES", true, true}, {"true", true, true}, {"1", true, true},
		{"n", false, true}, {"N", false, true}, {"no", false, true},
		{"NO", false, true}, {"false", false, true}, {"0", false, true},
		{"", false, true},
		{"maybe", false, false}, {"sure", false, false}, {"2", false, false},
	}
	for _, c := range cases {
		v, ok := ParseYN(c.in)
		if v != c.value || ok != c.ok {
			t.Errorf("ParseYN(%q) = (%v, %v), want (%v, %v)", c.in, v, ok, c.value, c.ok)
		}
	}
}
