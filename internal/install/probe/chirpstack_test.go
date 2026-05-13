package probe_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/shifter-io/shifter/internal/doctor"
)

// TestProbeChirpStack_NoAPIKeyInErrorMessage — checker B-3 (T-07-14-03 verification).
// Asserts that ProbeChirpStack NEVER leaks the configured API key in any
// ProbeResult.Message or ProbeResult.Status string under an error condition.
func TestProbeChirpStack_NoAPIKeyInErrorMessage(t *testing.T) {
	const sentinel = "SENTINEL_API_KEY_8f2c93"

	// Use an unreachable host:port to force an error path.
	unreachable := "127.0.0.1:1" // port 1 is reserved; dial will fail fast.

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result := doctor.ProbeChirpStack(ctx, unreachable, sentinel)

	if strings.Contains(result.Message, sentinel) {
		t.Fatalf("ProbeResult.Message leaked API key sentinel: %q", result.Message)
	}
	if strings.Contains(result.Status, sentinel) {
		t.Fatalf("ProbeResult.Status leaked API key sentinel: %q", result.Status)
	}
	// Sanity: we DID hit an error path (this test would be vacuous if status=ok).
	if result.Status != "error" {
		t.Fatalf("expected error status for unreachable host, got %q (msg=%q)", result.Status, result.Message)
	}
}
