package resolver

import (
	"context"
	"errors"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// fakeLoader records every LoadActive call and returns a configured Binding
// (or error). Used by the cache unit tests to count loader invocations.
type fakeLoader struct {
	mu       sync.Mutex
	calls    int64
	bindings map[string]Binding
	err      error
}

func newFakeLoader() *fakeLoader {
	return &fakeLoader{bindings: map[string]Binding{}}
}

func (f *fakeLoader) LoadActive(_ context.Context, devEUI string, _ time.Time) (Binding, error) {
	atomic.AddInt64(&f.calls, 1)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return Binding{}, f.err
	}
	if b, ok := f.bindings[strings.ToLower(devEUI)]; ok {
		return b, nil
	}
	return Binding{}, ErrNoActiveBinding
}

func (f *fakeLoader) callCount() int64 {
	return atomic.LoadInt64(&f.calls)
}

func (f *fakeLoader) put(devEUI string, b Binding) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bindings[strings.ToLower(devEUI)] = b
}

// makeBinding returns a Binding with sensible defaults: ValidFrom in the
// past, ValidTo zero (still active), reading_offset=0.
func makeBinding(suffix string) Binding {
	return Binding{
		BindingID:       uuid.New(),
		MeteringPointID: uuid.New(),
		DeviceID:        uuid.New(),
		DeviceProfileID: uuid.New(),
		ReadingOffset:   big.NewFloat(0),
		LastRawValue:    nil, // first uplink boundary signal
		CounterModulus:  4294967296,
		ValidFrom:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// TestLookup_Miss_LoadsFromLoader — cache miss falls through to loader; the
// loaded binding is cached so a second Lookup with the same devEUI is a hit.
func TestLookup_Miss_LoadsFromLoader(t *testing.T) {
	loader := newFakeLoader()
	want := makeBinding("alpha")
	loader.put("aabbccddeeff0011", want)

	r := New(loader)

	got, err := r.Lookup(context.Background(), "aabbccddeeff0011", time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, want.BindingID, got.BindingID, "loader-returned binding")
	require.Equal(t, int64(1), loader.callCount(), "first call hits loader")

	stats := r.Stats()
	require.Equal(t, int64(0), stats.Hits)
	require.Equal(t, int64(1), stats.Misses)

	// Second lookup is a hit — no new loader call.
	got2, err := r.Lookup(context.Background(), "aabbccddeeff0011", time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, want.BindingID, got2.BindingID)
	require.Equal(t, int64(1), loader.callCount(), "second call must NOT hit loader")
	stats = r.Stats()
	require.Equal(t, int64(1), stats.Hits)
}

// TestLookup_NormalizesCase — uppercase dev_eui in the input must hit the
// same cache slot as lowercase. Resolver guarantees lowercase keys.
func TestLookup_NormalizesCase(t *testing.T) {
	loader := newFakeLoader()
	loader.put("aabbccddeeff0011", makeBinding("alpha"))
	r := New(loader)

	_, err := r.Lookup(context.Background(), "AABBCCDDEEFF0011", time.Now().UTC())
	require.NoError(t, err)

	// Cache must contain the lowercase key only.
	r.mu.RLock()
	_, hasLower := r.cache["aabbccddeeff0011"]
	_, hasUpper := r.cache["AABBCCDDEEFF0011"]
	r.mu.RUnlock()
	require.True(t, hasLower, "cache stores lowercase")
	require.False(t, hasUpper, "cache must not store mixed case")
}

// TestInvalidate_DropsCache — Invalidate(devEUI) removes the entry; next
// Lookup re-loads from the loader.
func TestInvalidate_DropsCache(t *testing.T) {
	loader := newFakeLoader()
	loader.put("aabbccddeeff0011", makeBinding("alpha"))
	r := New(loader)

	// Populate.
	_, err := r.Lookup(context.Background(), "aabbccddeeff0011", time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, int64(1), loader.callCount())

	// Invalidate (case-insensitive).
	r.Invalidate("AABBCCDDEEFF0011")

	// Next lookup re-loads.
	_, err = r.Lookup(context.Background(), "aabbccddeeff0011", time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, int64(2), loader.callCount(), "post-invalidate lookup must hit loader again")

	stats := r.Stats()
	require.Equal(t, int64(1), stats.Invalidations)
}

// TestInvalidate_MissingKeyIsNoOp — invalidating a key that was never cached
// must not error (idempotent contract — the swap commit path calls
// Invalidate even when the resolver hasn't seen the dev_eui yet).
func TestInvalidate_MissingKeyIsNoOp(t *testing.T) {
	r := New(newFakeLoader())

	r.Invalidate("never-cached")
	r.Invalidate("never-cached")

	stats := r.Stats()
	require.Equal(t, int64(2), stats.Invalidations, "Invalidate counts every call")
	require.Equal(t, 0, stats.Size, "cache stays empty")
}

// TestSet_PreWarms — Set populates the cache; the next Lookup is a hit.
func TestSet_PreWarms(t *testing.T) {
	loader := newFakeLoader()
	r := New(loader)

	want := makeBinding("alpha")
	r.Set("aabbccddeeff0011", want)

	got, err := r.Lookup(context.Background(), "aabbccddeeff0011", time.Now().UTC())
	require.NoError(t, err)
	require.Equal(t, want.BindingID, got.BindingID)
	require.Equal(t, int64(0), loader.callCount(), "Set must populate cache without loader call")

	stats := r.Stats()
	require.Equal(t, int64(1), stats.Hits)
}

// TestStats_Counts — sequence of Lookup → Invalidate → Lookup must produce
// (hits=0, misses=2, invalids=1).
func TestStats_Counts(t *testing.T) {
	loader := newFakeLoader()
	loader.put("aabbccddeeff0011", makeBinding("alpha"))
	r := New(loader)

	_, _ = r.Lookup(context.Background(), "aabbccddeeff0011", time.Now().UTC())
	r.Invalidate("aabbccddeeff0011")
	_, _ = r.Lookup(context.Background(), "aabbccddeeff0011", time.Now().UTC())

	stats := r.Stats()
	require.Equal(t, int64(0), stats.Hits, "no cache-hit path exercised")
	require.Equal(t, int64(2), stats.Misses, "two loader fall-throughs")
	require.Equal(t, int64(1), stats.Invalidations)
}

// TestLookup_ConcurrentAccess — race-detector smoke. 100 goroutines × 100
// lookups on a shared cache. Run with `go test -race`.
func TestLookup_ConcurrentAccess(t *testing.T) {
	loader := newFakeLoader()
	for i := 0; i < 10; i++ {
		eui := makeEUI(i)
		loader.put(eui, makeBinding(eui))
	}
	r := New(loader)

	var wg sync.WaitGroup
	for g := 0; g < 100; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				eui := makeEUI((gid + i) % 10)
				_, err := r.Lookup(context.Background(), eui, time.Now().UTC())
				if err != nil {
					t.Errorf("concurrent lookup error: %v", err)
				}
				if (gid+i)%17 == 0 {
					r.Invalidate(eui)
				}
			}
		}(g)
	}
	wg.Wait()

	// At least every key was loaded at least once; cache size <= 10.
	stats := r.Stats()
	require.LessOrEqual(t, stats.Size, 10)
	require.Greater(t, stats.Hits+stats.Misses, int64(0))
}

// TestLookup_LoaderError_NotCached — a loader error MUST NOT poison the
// cache. The next Lookup retries the loader.
func TestLookup_LoaderError_NotCached(t *testing.T) {
	loader := newFakeLoader()
	loader.err = errors.New("simulated DB error")
	r := New(loader)

	_, err := r.Lookup(context.Background(), "aabbccddeeff0011", time.Now().UTC())
	require.Error(t, err)
	require.Equal(t, int64(1), loader.callCount())

	// Cache is empty — next lookup retries the loader.
	require.Equal(t, 0, r.Stats().Size, "errors must not be cached")

	_, err = r.Lookup(context.Background(), "aabbccddeeff0011", time.Now().UTC())
	require.Error(t, err)
	require.Equal(t, int64(2), loader.callCount(), "error path retries loader")
}

// TestLookup_HistoricalAtBeforeValidFrom — Phase 4 reports may probe
// historical time. A cached binding whose ValidFrom > at must NOT be
// returned (would attribute an old uplink to a newer binding).
func TestLookup_HistoricalAtBeforeValidFrom(t *testing.T) {
	loader := newFakeLoader()
	b := makeBinding("alpha")
	b.ValidFrom = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	loader.put("aabbccddeeff0011", b)
	r := New(loader)

	// Pre-warm the cache.
	_, err := r.Lookup(context.Background(), "aabbccddeeff0011", time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	cacheCalls := loader.callCount()

	// Now probe a time BEFORE the binding's ValidFrom — must miss the cache
	// and fall through to the loader (which will return the same row, but
	// the cache shouldn't pretend the cached entry covers this older time).
	at := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	_, err = r.Lookup(context.Background(), "aabbccddeeff0011", at)
	require.NoError(t, err)
	require.Equal(t, cacheCalls+1, loader.callCount(),
		"historical-time lookup must not hit the cache hit path")
}

// makeEUI builds a deterministic 16-char lowercase hex dev_eui from an int.
func makeEUI(n int) string {
	const hexChars = "0123456789abcdef"
	out := make([]byte, 16)
	for i := 0; i < 16; i++ {
		out[15-i] = hexChars[n&0xf]
		n >>= 4
	}
	return string(out)
}
