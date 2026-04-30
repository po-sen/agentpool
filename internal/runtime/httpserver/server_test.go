package httpserver_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/po-sen/agentpool/internal/runtime/httpserver"
)

func TestRunReturnsListenError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	server := httpserver.New("127.0.0.1:bad-port", http.NotFoundHandler(), nil)
	err := server.Run(ctx)
	if err == nil {
		t.Fatal("Run() error = nil, want listen error")
	}
	if !strings.Contains(err.Error(), "bad-port") {
		t.Fatalf("Run() error = %v, want error mentioning bad-port", err)
	}
}
