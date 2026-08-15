package console

import (
	"fmt"
	"os"
)

var defaultStdPrinter Printer = &StdPrinter{
	StdoutPrint: func(s string) { _, _ = fmt.Fprintln(os.Stdout, s) },
	StderrPrint: func(s string) { _, _ = fmt.Fprintln(os.Stderr, s) },
}

// StdPrinter implements the console.Printer interface
// that prints to the stdout or stderr.
type StdPrinter struct {
	StdoutPrint func(s string)
	StderrPrint func(s string)
}

// Log prints s to the stdout.
func (p StdPrinter) Log(s string) {
	p.StdoutPrint(s)
}

// Warn prints s to the stderr.
func (p StdPrinter) Warn(s string) {
	p.StderrPrint(s)
}

// Error prints s to the stderr.
func (p StdPrinter) Error(s string) {
	p.StderrPrint(s)
}
