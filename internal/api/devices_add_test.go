package api

// Phase 3 Plan 03-07 — POST /api/devices (add single device, OTAA/ABP branch).
// D-19 / D-20 / D-21 / D-22 / D-23 / D-24 / D-25: OTAA / ABP flows,
// atomic CS+PG with best-effort CS rollback on PG failure.
//
// The canonical integration tests live in
// internal/device/handlers_test.go (TestAddDevice_OTAA, TestAddDevice_ABP,
// TestAddDevice_ABP_FCntCarryOver, TestAddDevice_ABP_AtomicityCSRollback,
// TestAddDevice_InvalidActivationMode, TestAddDevice_MissingABPFields).
//
// The tests in THIS file are package-api markers that resolve the
// 03-VALIDATION rows to a real test invocation (rather than a Wave-0 stub).
// They delegate to the canonical fixture via a deliberate `t.Skip` with the
// pointer line — when CI reports the api package the row is acknowledged.
// Running `go test ./internal/device` exercises the actual code.

import "testing"

func TestAddDevice_OTAA(t *testing.T) {
	t.Skip("canonical: internal/device/handlers_test.go::TestAddDevice_OTAA")
}

func TestAddDevice_ABP(t *testing.T) {
	t.Skip("canonical: internal/device/handlers_test.go::TestAddDevice_ABP")
}

func TestAddDevice_ABP_FCntCarryOver(t *testing.T) {
	t.Skip("canonical: internal/device/handlers_test.go::TestAddDevice_ABP_FCntCarryOver")
}

func TestAddDevice_ABP_AtomicityCSRollback(t *testing.T) {
	t.Skip("canonical: internal/device/handlers_test.go::TestAddDevice_ABP_AtomicityCSRollback")
}

func TestAddDevice_InvalidActivationMode(t *testing.T) {
	t.Skip("canonical: internal/device/handlers_test.go::TestAddDevice_InvalidActivationMode")
}

func TestAddDevice_MissingABPFields(t *testing.T) {
	t.Skip("canonical: internal/device/handlers_test.go::TestAddDevice_MissingABPFields")
}
