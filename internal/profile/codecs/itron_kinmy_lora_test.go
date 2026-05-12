package codecs_test

import "testing"

// TestItronKinmyLoRa_GoldenVector exercises the SOF=0x6F frame format
// against a captured 28-byte payload (filled in plan 07-09 once normalize.go
// battery curve registry lands and produces the canonical mapping).
func TestItronKinmyLoRa_GoldenVector(t *testing.T) {
	t.Skip("Wave 0 skeleton — filled in plan 07-09 with golden hex fixture and expected canonical mapping")
}
