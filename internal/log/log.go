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
//
// When --log-file is set, every level (including Debug) is mirrored to the
// file as plain text (no ANSI), regardless of --quiet. The file is the full
// audit trail; terminal output is the filtered human view.
package log

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

const prefixedLineFmt = "%s %s\n"

// Plain-text prefixes for file mirror. Kept in sync with the colored prefixes.
const (
	plainDebug   = "DBG"
	plainInfo    = ">"
	plainSuccess = "OK"
	plainWarn    = "WARN"
	plainError   = "FAIL"
	plainFatal   = "FATAL:"
)

var (
	prefixDebug   = color.New(color.FgMagenta).SprintFunc()
	prefixInfo    = color.New(color.FgCyan).SprintFunc()
	prefixSuccess = color.New(color.FgGreen).SprintFunc()
	prefixWarn    = color.New(color.FgYellow).SprintFunc()
	prefixError   = color.New(color.FgRed).SprintFunc()
)

// Package state, set once by Init. Package-level globals are acceptable for a
// single-process CLI.
var (
	verbose bool
	quiet   bool
	logFile *os.File
)

// Init configures verbosity and (optionally) mirrors all output to logFilePath.
// Verbose and quiet are mutually exclusive (config validates that earlier).
// Returns an error if the log file cannot be opened.
func Init(isVerbose, isQuiet bool, logFilePath string) error {
	verbose = isVerbose
	quiet = isQuiet
	if logFilePath == "" {
		return nil
	}
	f, err := os.OpenFile(logFilePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open log file %s: %w", logFilePath, err)
	}
	logFile = f
	return nil
}

// Close flushes and closes the log file, if one was opened. Call from a defer
// in runInit to ensure the file is written before exit.
func Close() {
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}

// writeFile appends a plain-prefixed line to the log file if one is open.
// Failures are ignored (stdout/stderr remain the primary channel).
func writeFile(prefix, msg string) {
	if logFile == nil {
		return
	}
	fmt.Fprintf(logFile, prefixedLineFmt, prefix, msg)
}

// Debug prints a diagnostic line only when --verbose is set. Goes to stdout.
// Always mirrored to the log file.
func Debug(msg string) {
	writeFile(plainDebug, msg)
	if !verbose {
		return
	}
	fmt.Printf(prefixedLineFmt, prefixDebug("DBG"), msg)
}

func Debugf(format string, args ...any) {
	Debug(fmt.Sprintf(format, args...))
}

// Info prints an in-progress status line. Suppressed by --quiet. Goes to stdout.
// Always mirrored to the log file.
func Info(msg string) {
	writeFile(plainInfo, msg)
	if quiet {
		return
	}
	fmt.Printf(prefixedLineFmt, prefixInfo(">"), msg)
}

func Infof(format string, args ...any) {
	Info(fmt.Sprintf(format, args...))
}

// Success prints a positive completion line. Always shown. Goes to stdout.
func Success(msg string) {
	writeFile(plainSuccess, msg)
	fmt.Printf(prefixedLineFmt, prefixSuccess("OK"), msg)
}

func Successf(format string, args ...any) {
	Success(fmt.Sprintf(format, args...))
}

// Warn prints a warning. Always shown. Goes to stderr (Unix convention).
func Warn(msg string) {
	writeFile(plainWarn, msg)
	fmt.Fprintf(os.Stderr, prefixedLineFmt, prefixWarn("WARN"), msg)
}

func Warnf(format string, args ...any) {
	Warn(fmt.Sprintf(format, args...))
}

// Error prints an error that does not stop the program. Always shown. Goes to stderr.
func Error(msg string) {
	writeFile(plainError, msg)
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
	writeFile(plainFatal, msg)
	fmt.Fprintf(os.Stderr, prefixedLineFmt, prefixError(plainFatal), msg)
	panic(&StepError{Msg: msg})
}

func Fatalf(format string, args ...any) {
	Fatal(fmt.Sprintf(format, args...))
}

// FatalExit prints an error and exits immediately. Use for pre-validation failures only.
func FatalExit(msg string) {
	writeFile(plainFatal, msg)
	fmt.Fprintf(os.Stderr, prefixedLineFmt, prefixError(plainFatal), msg)
	Close()
	os.Exit(1)
}
