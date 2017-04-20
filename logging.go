package sup

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
func SetLogger(l Logger) { logger = l }

var logger Logger

func log(s Severity, format string, values ...interface{}) {
	if logger == nil {
		return
	}
	logger(s, format, values)
}
