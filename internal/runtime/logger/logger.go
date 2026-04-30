package logger

import (
	"io"
	"log"
)

// Logger is the minimal application logger.
type Logger struct {
	logger *log.Logger
}

// New creates a logger that writes to the provided output.
func New(out io.Writer) *Logger {
	if out == nil {
		out = io.Discard
	}

	return &Logger{
		logger: log.New(out, "", log.LstdFlags),
	}
}

// Infof writes an informational message.
func (l *Logger) Infof(format string, args ...any) {
	l.logger.Printf("INFO "+format, args...)
}

// Errorf writes an error message.
func (l *Logger) Errorf(format string, args ...any) {
	l.logger.Printf("ERROR "+format, args...)
}
