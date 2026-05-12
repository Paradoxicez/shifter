package resolver

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// ErrNoActiveBinding is returned by Resolver.Lookup when the loader reports
// no row for (devEUI, at). Callers in the ingest pipeline (Plan 02-09) treat
// this as "drop the uplink with quality=missing_canonical" rather than a
// retryable error — there is no binding for an unprovisioned device, ever.
var ErrNoActiveBinding = errors.New("resolver: no active binding for dev_eui at requested time")

// Binding is the resolved per-uplink context: which MP this dev_eui maps to,
// the binding's reading offset and last raw value, the device profile's
// counter modulus (for DATA-05 rollover math), and the binding's open
// window. Returned by Lookup; consumed by the ingest pipeline.
type Binding struct {
	BindingID       uuid.UUID
	MeteringPointID uuid.UUID
	DeviceID        uuid.UUID
	DeviceProfileID uuid.UUID

	ReadingOffset  *big.Float // never nil — defaults to 0 if missing
	LastRawValue   *big.Float // nil on first uplink for a freshly-opened binding (DATA-05 boundary signal)
	CounterModulus int64      // device_profile.counter_modulus (e.g. 4294967296 for 32-bit)

	ValidFrom time.Time // half-open [valid_from, valid_to)
	ValidTo   time.Time // zero value means "still active"

	// BatteryCurve is device_profile.battery_curve (D-44 enum). Carried here so
	// the ingest pipeline can pass it to NormalizeMeasurement without an extra
	// DB round-trip. "" is safe — NormalizeMeasurement treats it as passthrough.
	BatteryCurve string
}

// Loader is the interface Resolver uses to fetch a binding from Postgres on
// cache miss. The production implementation wraps sqlc.GetActiveBindingByDevEUI;
// tests substitute an in-memory fake.
//
// Loaders MUST normalize devEUI (lowercase) before querying — the schema
// stores lowercase per 0012 CHECK and the resolver always passes lowercase
// in. A loader that didn't normalize would silently miss every uplink whose
// CS payload uses upper-case hex.
type Loader interface {
	LoadActive(ctx context.Context, devEUI string, at time.Time) (Binding, error)
}

// Resolver is the in-memory cache + loader combo. Methods are safe to call
// from any goroutine concurrently. Cache is bounded by the count of active
// dev_eui values for the install (single-tenant, typically <500); no
// eviction policy is needed at v1 scale (T-02-07-04 disposition).
type Resolver struct {
	loader Loader

	mu    sync.RWMutex
	cache map[string]Binding // key = lowercase dev_eui

	hits     atomic.Int64
	misses   atomic.Int64
	invalids atomic.Int64
}

// New constructs a Resolver around the given Loader. Loader must be non-nil
// for Lookup to succeed; pass a fake loader in tests.
func New(loader Loader) *Resolver {
	return &Resolver{
		loader: loader,
		cache:  make(map[string]Binding),
	}
}

// Lookup returns the active binding for devEUI at time `at`. Cache hit is
// O(1) under an RLock; cache miss falls through to the loader and the
// result is stored under a write lock.
//
// Cache-window check: a cached entry whose ValidFrom > at is treated as a
// miss. This handles the rare race where Lookup is called for a historical
// timestamp (Phase 4 reports) and the cache holds a NEWER binding for the
// same dev_eui — we don't want to attribute a 10-day-old uplink to today's
// binding.
func (r *Resolver) Lookup(ctx context.Context, devEUI string, at time.Time) (Binding, error) {
	devEUI = strings.ToLower(devEUI)

	r.mu.RLock()
	if b, ok := r.cache[devEUI]; ok && !at.Before(b.ValidFrom) {
		// Also reject the cached entry if it's CLOSED and `at` falls outside
		// the closed window (zero-value ValidTo means "still active").
		if b.ValidTo.IsZero() || at.Before(b.ValidTo) {
			r.mu.RUnlock()
			r.hits.Add(1)
			return b, nil
		}
	}
	r.mu.RUnlock()

	// Miss path: hit the loader.
	if r.loader == nil {
		r.misses.Add(1)
		return Binding{}, errors.New("resolver: nil loader")
	}
	b, err := r.loader.LoadActive(ctx, devEUI, at)
	if err != nil {
		r.misses.Add(1)
		return Binding{}, err
	}

	r.mu.Lock()
	r.cache[devEUI] = b
	r.mu.Unlock()
	r.misses.Add(1)
	return b, nil
}

// Set pre-warms the cache for devEUI. Used by ingest's UpdateBindingLastRaw
// path (Plan 02-09) to avoid a re-load on the next uplink, and by tests
// that want to seed the cache without going through Lookup.
//
// Set normalizes devEUI to lowercase to match Lookup/Invalidate.
func (r *Resolver) Set(devEUI string, b Binding) {
	devEUI = strings.ToLower(devEUI)
	r.mu.Lock()
	r.cache[devEUI] = b
	r.mu.Unlock()
}

// Invalidate removes devEUI from the cache. Implements swap.Invalidator —
// CommitSwap calls this defensively post-commit alongside the 0017 NOTIFY
// trigger. Idempotent: invalidating a missing key is a no-op.
func (r *Resolver) Invalidate(devEUI string) {
	devEUI = strings.ToLower(devEUI)
	r.mu.Lock()
	delete(r.cache, devEUI)
	r.mu.Unlock()
	r.invalids.Add(1)
}

// Stats is a read-only snapshot of the resolver's hit/miss/invalidation
// counters. Used by /health/detailed (Phase 6) and by tests.
type Stats struct {
	Hits          int64
	Misses        int64
	Invalidations int64
	Size          int
}

// Stats returns a snapshot of the current counters and cache size.
func (r *Resolver) Stats() Stats {
	r.mu.RLock()
	size := len(r.cache)
	r.mu.RUnlock()
	return Stats{
		Hits:          r.hits.Load(),
		Misses:        r.misses.Load(),
		Invalidations: r.invalids.Load(),
		Size:          size,
	}
}
