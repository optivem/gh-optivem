// Package log provides colored logging helpers.
//
// Colors are produced via github.com/fatih/color, which auto-honors NO_COLOR
// and FORCE_COLOR, detects TTY (so piped/redirected output is clean text),
// and handles Windows terminals via go-colorable.
//
// Levels:
//
//	Debug   — only shown with --verbose. Use for retry/wait chatter etc.
//	Info    — suppressed with --quiet. Normal in-progress status ("> …").
//	Success — always shown. Step/operation success ("OK …").
//	Warn    — always shown on stderr. Recoverable issue.
//	Error   — always shown on stderr. Operation failed but program continues.
//	Fatal   — always shown on stderr; panics/exits.
package log

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

const prefixedLineFmt = "%s %s\n"

var (
	prefixDebug   = color.New(color.FgMagenta).SprintFunc()
	prefixInfo    = color.New(color.FgCyan).SprintFunc()
	prefixSuccess = color.New(color.FgGreen).SprintFunc()
	prefixWarn    = color.New(color.FgYellow).SprintFunc()
	prefixError   = color.New(color.FgRed).SprintFunc()
)

// Verbosity state, set once by Init from the parsed flags. Package-level
// globals are acceptable for a single-process CLI.
var (
	verbose bool
	quiet   bool
)

// Init configures verbosity. Call once from runInit after ParseAndValidate.
// Verbose and quiet are mutually exclusive; config validates that earlier.
func Init(isVerbose, isQuiet bool) {
	verbose = isVerbose
	quiet = isQuiet
}

// Debug prints a diagnostic line only when --verbose is set. Goes to stdout.
func Debug(msg string) {
	if !verbose {
		return
	}
	fmt.Printf(prefixedLineFmt, prefixDebug(">"), msg)
}

func Debugf(format string, args ...any) {
	if !verbose {
		return
	}
	Debug(fmt.Sprintf(format, args...))
}

// Info prints an in-progress status line. Suppressed by --quiet. Goes to stdout.
func Info(msg string) {
	if quiet {
		return
	}
	fmt.Printf(prefixedLineFmt, prefixInfo(">"), msg)
}

func Infof(format string, args ...any) {
	if quiet {
		return
	}
	Info(fmt.Sprintf(format, args...))
}

// Success prints a positive completion line. Always shown. Goes to stdout.
func Success(msg string) {
	fmt.Printf(prefixedLineFmt, prefixSuccess("OK"), msg)
}

func Successf(format string, args ...any) {
	Success(fmt.Sprintf(format, args...))
}

// Warn prints a warning. Always shown. Goes to stderr (Unix convention).
func Warn(msg string) {
	fmt.Fprintf(os.Stderr, prefixedLineFmt, prefixWarn("WARN"), msg)
}

func Warnf(format string, args ...any) {
	Warn(fmt.Sprintf(format, args...))
}

// Error prints an error that does not stop the program. Always shown. Goes to stderr.
func Error(msg string) {
	fmt.Fprintf(os.Stderr, prefixedLineFmt, prefixError("FAIL"), msg)
}

func Errorf(format string, args ...any) {
	Error(fmt.Sprintf(format, args...))
}

// StepError is a sentinel type used by Fatal to allow the step runner to catch failures.
type StepError struct {
	Msg string
}

func (e *StepError) Error() string { return e.Msg }

// Fatal prints an error and panics with a StepError (caught by the step runner).
// For use during step execution. For pre-validation failures, use FatalExit.
func Fatal(msg string) {
	fmt.Fprintf(os.Stderr, prefixedLineFmt, prefixError("FATAL:"), msg)
	panic(&StepError{Msg: msg})
}

func Fatalf(format string, args ...any) {
	Fatal(fmt.Sprintf(format, args...))
}

// FatalExit prints an error and exits immediately. Use for pre-validation failures only.
func FatalExit(msg string) {
	fmt.Fprintf(os.Stderr, prefixedLineFmt, prefixError("FATAL:"), msg)
	os.Exit(1)
}
