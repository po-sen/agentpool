package crypto

import (
	"encoding/hex"
	"strings"
	"testing"
)

func TestGeneratorNewRunIDCreatesOpaqueRunID(t *testing.T) {
	generator := NewGenerator()

	first, err := generator.NewRunID()
	if err != nil {
		t.Fatalf("new first run ID: %v", err)
	}
	second, err := generator.NewRunID()
	if err != nil {
		t.Fatalf("new second run ID: %v", err)
	}

	if first == second {
		t.Fatalf("generated duplicate run IDs: %s", first)
	}
	assertRunIDShape(t, string(first))
	assertRunIDShape(t, string(second))
}

func assertRunIDShape(t *testing.T, id string) {
	t.Helper()

	const prefix = "run_"
	if !strings.HasPrefix(id, prefix) {
		t.Fatalf("run ID %q does not start with %q", id, prefix)
	}

	encoded := strings.TrimPrefix(id, prefix)
	if len(encoded) != 32 {
		t.Fatalf("encoded random suffix length = %d, want 32", len(encoded))
	}
	if _, err := hex.DecodeString(encoded); err != nil {
		t.Fatalf("encoded random suffix is not hex: %v", err)
	}
}
