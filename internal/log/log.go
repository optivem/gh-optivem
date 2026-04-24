// Package log provides colored logging helpers.
//
// Colors are produced via github.com/fatih/color, which auto-honors NO_COLOR
// and FORCE_COLOR, detects TTY (so piped/redirected output is clean text),
// and handles Windows terminals via go-colorable.
package log

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

const prefixedLineFmt = "%s %s\n"

var (
	prefixLog  = color.New(color.FgCyan).SprintFunc()
	prefixOK   = color.New(color.FgGreen).SprintFunc()
	prefixWarn = color.New(color.FgYellow).SprintFunc()
	prefixFail = color.New(color.FgRed).SprintFunc()
)

func Log(msg string)                   { fmt.Printf(prefixedLineFmt, prefixLog(">"), msg) }
func Logf(format string, args ...any)  { Log(fmt.Sprintf(format, args...)) }
func OK(msg string)                    { fmt.Printf(prefixedLineFmt, prefixOK("OK"), msg) }
func OKf(format string, args ...any)   { OK(fmt.Sprintf(format, args...)) }
func Warn(msg string)                  { fmt.Printf(prefixedLineFmt, prefixWarn("WARN"), msg) }
func Warnf(format string, args ...any) { Warn(fmt.Sprintf(format, args...)) }
func Fail(msg string)                  { fmt.Printf(prefixedLineFmt, prefixFail("FAIL"), msg) }
func Failf(format string, args ...any) { Fail(fmt.Sprintf(format, args...)) }

// StepError is a sentinel type used by Fatal to allow the step runner to catch failures.
type StepError struct {
	Msg string
}

func (e *StepError) Error() string { return e.Msg }

// Fatal prints an error and panics with a StepError (caught by the step runner).
// For use during step execution. For pre-validation failures, use FatalExit.
func Fatal(msg string) {
	fmt.Fprintf(os.Stderr, prefixedLineFmt, prefixFail("FATAL:"), msg)
	panic(&StepError{Msg: msg})
}

func Fatalf(format string, args ...any) {
	Fatal(fmt.Sprintf(format, args...))
}

// FatalExit prints an error and exits immediately. Use for pre-validation failures only.
func FatalExit(msg string) {
	fmt.Fprintf(os.Stderr, prefixedLineFmt, prefixFail("FATAL:"), msg)
	os.Exit(1)
}
