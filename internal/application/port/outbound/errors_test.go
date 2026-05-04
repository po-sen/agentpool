package outbound

import (
	"errors"
	"testing"
)

func TestOutboundSentinelErrors(t *testing.T) {
	if !errors.Is(ErrRunQueueEmpty, ErrRunQueueEmpty) {
		t.Fatal("ErrRunQueueEmpty should match itself")
	}
}
