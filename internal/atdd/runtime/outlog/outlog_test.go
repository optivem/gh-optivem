package outlog_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/outlog"
)

func TestPhaseSinkReceivesOnlyPhase(t *testing.T) {
	var buf bytes.Buffer
	out := outlog.New(outlog.Sink{W: &buf, MaxLevel: outlog.Phase})

	fmt.Fprintln(out.Phase, "headline")
	fmt.Fprintln(out.Detail, "firehose")

	got := buf.String()
	if !strings.Contains(got, "headline") {
		t.Errorf("phase sink missed Phase write; got %q", got)
	}
	if strings.Contains(got, "firehose") {
		t.Errorf("phase sink leaked Detail write; got %q", got)
	}
}

func TestDetailSinkReceivesBoth(t *testing.T) {
	var buf bytes.Buffer
	out := outlog.New(outlog.Sink{W: &buf, MaxLevel: outlog.Detail})

	fmt.Fprintln(out.Phase, "headline")
	fmt.Fprintln(out.Detail, "firehose")

	got := buf.String()
	if !strings.Contains(got, "headline") || !strings.Contains(got, "firehose") {
		t.Errorf("detail sink missed a write; got %q", got)
	}
}

func TestTwoSinksAtDifferentLevels(t *testing.T) {
	var terminal, file bytes.Buffer
	out := outlog.New(
		outlog.Sink{W: &terminal, MaxLevel: outlog.Phase},
		outlog.Sink{W: &file, MaxLevel: outlog.Detail},
	)

	fmt.Fprintln(out.Phase, "headline")
	fmt.Fprintln(out.Detail, "firehose")

	if !strings.Contains(terminal.String(), "headline") || strings.Contains(terminal.String(), "firehose") {
		t.Errorf("terminal sink: want phase-only; got %q", terminal.String())
	}
	if !strings.Contains(file.String(), "headline") || !strings.Contains(file.String(), "firehose") {
		t.Errorf("file sink: want both; got %q", file.String())
	}
}

func TestEmptyConstructorWritesToDiscard(t *testing.T) {
	out := outlog.New()

	// Should not panic, and writes should be no-ops (Discard).
	n, err := fmt.Fprintln(out.Phase, "ignored")
	if err != nil || n == 0 {
		t.Errorf("Phase write to empty Out should succeed silently; n=%d err=%v", n, err)
	}
	n, err = fmt.Fprintln(out.Detail, "ignored")
	if err != nil || n == 0 {
		t.Errorf("Detail write to empty Out should succeed silently; n=%d err=%v", n, err)
	}
}

func TestDefaultMirrorsAllLevelsToOneWriter(t *testing.T) {
	var buf bytes.Buffer
	out := outlog.Default(&buf)

	fmt.Fprintln(out.Phase, "headline")
	fmt.Fprintln(out.Detail, "firehose")

	got := buf.String()
	if !strings.Contains(got, "headline") || !strings.Contains(got, "firehose") {
		t.Errorf("Default should mirror both levels to the same writer; got %q", got)
	}
}

func TestDefaultNilFallsBackToDiscard(t *testing.T) {
	out := outlog.Default(nil)
	if out.Phase != io.Discard || out.Detail != io.Discard {
		t.Errorf("Default(nil) should yield io.Discard writers; got %v / %v", out.Phase, out.Detail)
	}
}

func TestNilWriterSinkIgnored(t *testing.T) {
	var buf bytes.Buffer
	out := outlog.New(
		outlog.Sink{W: nil, MaxLevel: outlog.Detail},
		outlog.Sink{W: &buf, MaxLevel: outlog.Detail},
	)

	fmt.Fprintln(out.Phase, "headline")

	if !strings.Contains(buf.String(), "headline") {
		t.Errorf("non-nil sink should still receive writes when paired with nil sink; got %q", buf.String())
	}
}

func TestParseLevel(t *testing.T) {
	cases := map[string]outlog.Level{
		"phase":  outlog.Phase,
		"Phase":  outlog.Phase,
		"PHASE":  outlog.Phase,
		"detail": outlog.Detail,
		"Detail": outlog.Detail,
		"DETAIL": outlog.Detail,
	}
	for in, want := range cases {
		got, err := outlog.ParseLevel(in)
		if err != nil {
			t.Errorf("ParseLevel(%q) returned error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestParseLevelRejectsUnknown(t *testing.T) {
	if _, err := outlog.ParseLevel("warn"); err == nil {
		t.Errorf("ParseLevel(warn) should reject; got nil error")
	}
	if _, err := outlog.ParseLevel(""); err == nil {
		t.Errorf("ParseLevel(empty) should reject; got nil error")
	}
}

func TestLevelString(t *testing.T) {
	if outlog.Phase.String() != "phase" {
		t.Errorf("Phase.String() = %q, want phase", outlog.Phase.String())
	}
	if outlog.Detail.String() != "detail" {
		t.Errorf("Detail.String() = %q, want detail", outlog.Detail.String())
	}
}
