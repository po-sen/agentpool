package outbound_test

import (
	"errors"
	"testing"

	"github.com/po-sen/agentpool/internal/application/port/outbound"
)

func TestOutboundSentinelErrors(t *testing.T) {
	if !errors.Is(outbound.ErrRunQueueEmpty, outbound.ErrRunQueueEmpty) {
		t.Fatal("ErrRunQueueEmpty should match itself")
	}
}
