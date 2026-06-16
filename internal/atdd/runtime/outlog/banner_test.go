package outlog_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/optivem/gh-optivem/internal/atdd/runtime/outlog"
)

// The banner wraps its text in color escapes, but the escapes sit at the
// boundaries — the "[phase]  …" core is a contiguous substring either way,
// so Contains is robust whether or not color is enabled. The Fprintln
// newlines that bracket each block are emitted separately and carry no
// color codes, so the leading/trailing "\n" assertions are exact.

func TestWritePhaseBoundaryStart(t *testing.T) {
	var buf bytes.Buffer
	outlog.WritePhaseBoundary(&buf, "start", "my-phase", 0)

	got := buf.String()
	if !strings.HasPrefix(got, "\n") {
		t.Errorf("start banner should be preceded by a blank line; got %q", got)
	}
	if !strings.Contains(got, "[phase]  start  my-phase") {
		t.Errorf("start banner missing core text; got %q", got)
	}
}

func TestWritePhaseBoundaryEnd(t *testing.T) {
	var buf bytes.Buffer
	outlog.WritePhaseBoundary(&buf, "end", "my-phase", 3*time.Second)

	got := buf.String()
	if !strings.Contains(got, "[phase]  end    my-phase  3s") {
		t.Errorf("end banner missing core text / elapsed; got %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Errorf("end banner should be followed by a blank line; got %q", got)
	}
}

func TestWritePhaseBoundaryNilWriterIsNoOp(t *testing.T) {
	// Must not panic when the writer is nil (test fixtures nil out stdout).
	outlog.WritePhaseBoundary(nil, "start", "my-phase", 0)
	outlog.WritePhaseBoundary(nil, "end", "my-phase", time.Second)
}

func TestWritePhaseBoundaryUnknownEdgeWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	outlog.WritePhaseBoundary(&buf, "sideways", "my-phase", 0)
	if buf.Len() != 0 {
		t.Errorf("unknown edge should emit nothing; got %q", buf.String())
	}
}
