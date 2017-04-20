package sup

import (
	"fmt"
	"log"
)

// Severity of the message to be logged
type Severity int

const (
	Error = iota
	Info
	Debug
)

// Logger is a function that accepts a severity or level, a format string and a list of values
type Logger func(s Severity, format string, values ...interface{})

// SetLogger configures the logging function to use for all logging.
// The default logger is nil and no logging is performed at all.
func SetLogger(l Logger) { logFunction = l }

func DefaultLogger(s Severity, format string, values ...interface{}) {
	if s == Debug {
		return
	}
	severity := "ERROR"
	if s == Info {
		severity = "info"
	}
	f := fmt.Sprintf("[%s] (supervisor) %s", severity, format)
	log.Printf(f, values...)
}

var logFunction Logger

func log_(s Severity, format string, values ...interface{}) {
	if logFunction == nil {
		return
	}
	logFunction(s, format, values...)
}
