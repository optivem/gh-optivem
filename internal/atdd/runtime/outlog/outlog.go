// Package outlog provides level-tagged byte-stream multiplexing for the
// ATDD runtime. Every Fprint site in the runtime picks one of two levels
// (Phase or Detail) by choosing Out.Phase or Out.Detail as its writer
// target. Each Sink declares the maximum level it accepts: a terminal
// configured at Phase receives only headline output, while a log-file
// sink at Detail receives the firehose.
//
// Mental model: Phase is the operator-facing headline channel (BPMN
// trace, prompts, errors, top-level agent banners); Detail is the
// firehose (subprocess byte streams, agent body, verbose internal
// banners). A sink turned to "headlines only" misses the firehose but
// never misses headlines, because Phase < Detail numerically and every
// sink with MaxLevel ≥ Phase receives Phase writes.
//
// The package solves a writer-routing problem, not a structured-logging
// problem. Subprocess output (gradle, docker, agent stdout) is byte
// bytes-tee, which io.MultiWriter handles natively; slog would add a
// layer without solving the actual problem.
package outlog

import (
	"fmt"
	"io"
)

// Level orders output importance. Lower values are higher priority and
// always reach every sink; higher values are filtered out by sinks whose
// MaxLevel does not reach them.
type Level int

const (
	// Phase is the operator-facing headline channel: BPMN trace lines,
	// approval / STOP prompts, errors, top-level agent banners.
	Phase Level = iota
	// Detail is the firehose: subprocess byte streams (gradle, docker,
	// gh CLI, agent stdout), prompt-prep summaries, verbose internal
	// banners. Only sinks at MaxLevel ≥ Detail receive these writes.
	Detail
)

// Sink is one output destination with a maximum accepted level.
type Sink struct {
	W        io.Writer
	MaxLevel Level
}

// Out exposes the two writers call sites pick between. Each writer is an
// io.MultiWriter of every Sink whose MaxLevel reaches that level. A
// zero-Sink Out writes both fields to io.Discard, which is safe for
// programmatic / test paths that don't configure sinks.
type Out struct {
	Phase  io.Writer
	Detail io.Writer
}

// New composes an Out from the given sinks. Each level's writer is an
// io.MultiWriter over the sinks that accept that level. Passing no
// sinks yields Phase = Detail = io.Discard.
func New(sinks ...Sink) *Out {
	var phaseWriters, detailWriters []io.Writer
	for _, s := range sinks {
		if s.W == nil {
			continue
		}
		if s.MaxLevel >= Phase {
			phaseWriters = append(phaseWriters, s.W)
		}
		if s.MaxLevel >= Detail {
			detailWriters = append(detailWriters, s.W)
		}
	}
	return &Out{
		Phase:  combine(phaseWriters),
		Detail: combine(detailWriters),
	}
}

// Default constructs a back-compat Out where both Phase and Detail
// write to the supplied writer. Used by package-level zero-value paths
// (e.g. driver.Options.withDefaults when no explicit Out is provided)
// so existing call sites that only carried a single Stdout writer keep
// seeing all output.
func Default(w io.Writer) *Out {
	if w == nil {
		w = io.Discard
	}
	return &Out{Phase: w, Detail: w}
}

func combine(ws []io.Writer) io.Writer {
	switch len(ws) {
	case 0:
		return io.Discard
	case 1:
		return ws[0]
	default:
		return io.MultiWriter(ws...)
	}
}

// ParseLevel maps the string form used by CLI flags to a Level.
// Accepts "phase" and "detail" case-insensitively. Any other value is
// rejected with a clear error so flag parsing fails loud rather than
// silently falling back.
func ParseLevel(s string) (Level, error) {
	switch s {
	case "phase", "PHASE", "Phase":
		return Phase, nil
	case "detail", "DETAIL", "Detail":
		return Detail, nil
	default:
		return 0, fmt.Errorf("outlog: invalid level %q (want phase or detail)", s)
	}
}

// String returns the CLI-form name of the level.
func (l Level) String() string {
	switch l {
	case Phase:
		return "phase"
	case Detail:
		return "detail"
	default:
		return fmt.Sprintf("level(%d)", int(l))
	}
}
