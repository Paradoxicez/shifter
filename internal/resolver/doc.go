// Package resolver implements DATA-01 (dev_eui → metering_point_id at uplink
// time, via the active binding) and D-25 (eventless cache invalidation) for
// the ingest hot path.
//
// Architecture (RESEARCH §"Pattern 7"):
//
//   - cache.go — concurrent-safe in-memory map keyed by lowercase dev_eui.
//     Lookup is RWMutex-protected for the cache-hit path. On miss, the
//     loader.LoadActive function (typically a sqlc.GetActiveBindingByDevEUI
//     wrapper) fetches the active binding from Postgres and the result is
//     cached. NO TTL: invalidation arrives via NOTIFY binding_changed
//     emitted by the migration 0017 trigger when bindings are opened or
//     closed (PITFALLS §2 — TTL-based invalidation creates a swap-then-
//     uplink race window).
//
//   - listener.go — runs a dedicated pgx connection holding LISTEN
//     binding_changed for the process lifetime. Outer reconnect loop with
//     2s backoff defends against connection drops (Pitfall 12). The
//     defer conn.Release() inside runOnce returns the conn to the pool
//     even if the connection itself is dead — pgxpool replaces it on next
//     Acquire.
//
//   - invalidate.go — small helper for emitting an out-of-band
//     pg_notify('binding_changed', ...) from a tx, used by tests and as a
//     defensive belt-and-suspenders for code paths that must invalidate
//     without touching the binding table directly.
//
// Resolver implements swap.Invalidator (Invalidate(string)) so the swap
// commit path can call deps.Resolver.Invalidate(devEUI) defensively after
// a successful tx commit; the 0017 trigger normally beats us to it but
// the direct call closes a network-flap-shaped window.
package resolver
