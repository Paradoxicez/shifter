// Package testharness — DATA-06 synthetic-data harness.
//
// The harness is a shared library + dual entry point per CONTEXT D-27:
//
//   - Go integration tests in scenarios_test.go drive 5 named scenarios
//     (clean_swap, swap_with_inflight_uplink, rollover, swap_then_rollover,
//     overlapping_uplinks_during_swap) plus a vendor-E2E scenario for
//     DATA-10 against testcontainer Postgres+TimescaleDB + Mosquitto.
//
//   - The CLI subcommand `shifter test-harness <scenario>` (internal/cli/
//     testharness.go) publishes the same scenarios to a deployed install's
//     MQTT broker so an operator can validate post-install without a dev
//     environment. Same library code; different transport configuration.
//
// Vendor uplink publishers (vendor_axioma_w1.go, vendor_acrel_adw300.go)
// synthesize the v4 event JSON shape — `object` keys match the codec_js
// result.* keys EXACTLY (axioma_w1.js / acrel_family.js — pinned in
// 02-13-PLAN.md `<codec_keys_pinned>` table). Mapping rows seeded by
// the harness reference these same pointer keys so normalize.go can
// produce canonical values without needing the actual codec_js to run.
//
// W3 sync barrier: when scenarios are driven through MQTT (in.Publisher !=
// nil) the helper publishOrInline polls q.CountMeasurementsByMP every 100ms
// (10s ceiling) to ensure the deployed serve.go's ingest pipeline has
// persisted each row before the scenario advances. Broker-down or
// ingest-down failures surface as a clear operator-facing error message.
package testharness
