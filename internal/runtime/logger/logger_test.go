package logger

import (
	"bytes"
	"strings"
	"testing"
)

func TestLoggerWritesInfoAndErrorPrefixes(t *testing.T) {
	var output bytes.Buffer
	log := New(&output)

	log.Infof("hello %s", "world")
	log.Errorf("failed %d", 1)

	text := output.String()
	if !strings.Contains(text, "INFO hello world") {
		t.Fatalf("log output %q does not contain info message", text)
	}
	if !strings.Contains(text, "ERROR failed 1") {
		t.Fatalf("log output %q does not contain error message", text)
	}
}

func TestLoggerAcceptsNilWriter(_ *testing.T) {
	log := New(nil)

	log.Infof("discarded")
	log.Errorf("discarded")
}
