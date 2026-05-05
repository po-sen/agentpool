package shell

import (
	"strings"
	"testing"
	"time"
)

func TestParseTimeoutUsesDefaultWhenArgumentIsMissing(t *testing.T) {
	runner := &Runner{
		defaultTimeout: 7 * time.Second,
		maxTimeout:     30 * time.Second,
	}

	timeout, err := runner.parseTimeout(nil)
	if err != nil {
		t.Fatalf("parseTimeout() error = %v", err)
	}
	if timeout != 7*time.Second {
		t.Fatalf("timeout = %s, want 7s", timeout)
	}
}

func TestParseTimeoutAcceptsPositiveIntegerSeconds(t *testing.T) {
	runner := &Runner{
		defaultTimeout: 10 * time.Second,
		maxTimeout:     30 * time.Second,
	}

	timeout, err := runner.parseTimeout(map[string]string{argumentTimeoutSeconds: "5"})
	if err != nil {
		t.Fatalf("parseTimeout() error = %v", err)
	}
	if timeout != 5*time.Second {
		t.Fatalf("timeout = %s, want 5s", timeout)
	}
}

func TestParseTimeoutRejectsInvalidValues(t *testing.T) {
	runner := &Runner{
		defaultTimeout: 10 * time.Second,
		maxTimeout:     30 * time.Second,
	}

	tests := []string{"0", "-1", "abc"}
	for _, test := range tests {
		_, err := runner.parseTimeout(map[string]string{argumentTimeoutSeconds: test})
		if err == nil {
			t.Fatalf("parseTimeout(%q) error = nil, want error", test)
		}
		if !strings.Contains(err.Error(), "positive integer") {
			t.Fatalf("parseTimeout(%q) error = %v, want positive integer error", test, err)
		}
	}
}

func TestParseTimeoutRejectsValuesAboveMax(t *testing.T) {
	runner := &Runner{
		defaultTimeout: 10 * time.Second,
		maxTimeout:     30 * time.Second,
	}

	_, err := runner.parseTimeout(map[string]string{argumentTimeoutSeconds: "31"})
	if err == nil {
		t.Fatal("parseTimeout() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "timeout_seconds exceeds maximum 30") {
		t.Fatalf("parseTimeout() error = %v, want max timeout error", err)
	}
}
