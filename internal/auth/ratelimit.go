// Package auth — login rate limiting (AUTH-04).
//
// LoginLimiter applies a per-IP and per-username token bucket per
// RESEARCH §Pattern 17. Both buckets must allow before login proceeds; either
// bucket exhausting (5 attempts inside the rolling 1-minute window) refuses
// the attempt with a 429 + Retry-After response.
//
// The implementation uses golang.org/x/time/rate (the canonical Go primitive
// for token-bucket rate limiting) with `rate.Every(time.Minute)` refill and a
// burst of 5. A background cleanup goroutine evicts stale buckets every 15
// minutes to keep the map bounded under attack.
//
// Source: https://pkg.go.dev/golang.org/x/time/rate
package auth

import (
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// LoginLimiter enforces AUTH-04: 5 attempts per rolling 1 minute, with
// independent per-IP and per-username buckets. Both must allow before login
// proceeds.
type LoginLimiter struct {
	perIP       *limiterMap
	perUsername *limiterMap
	stop        chan struct{}
}

// limiterMap is a concurrency-safe map from key (IP or lowercase username) to
// a *rate.Limiter + a lastSeen timestamp used by the cleanup pass.
type limiterMap struct {
	sync.Mutex
	m map[string]*entry
}

type entry struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// AUTH-04 token-bucket parameters. These constants are part of the public
// contract — tests assert behavior against them, and any change must update
// the operator-facing 429 Retry-After expectations.
const (
	// loginBurst is the maximum number of attempts inside one window.
	loginBurst = 5
	// loginRefill is the rate.Every interval — one token regenerates per minute.
	loginRefill = time.Minute
	// cleanupAfter is how long an entry can sit idle before the cleanup pass evicts it.
	cleanupAfter = time.Hour
	// cleanupTick is how often the cleanup goroutine runs.
	cleanupTick = 15 * time.Minute
)

// NewLoginLimiter constructs a LoginLimiter and starts its cleanup goroutine.
// Stop the limiter via (*LoginLimiter).Stop when the process exits or the test
// completes.
func NewLoginLimiter() *LoginLimiter {
	ll := &LoginLimiter{
		perIP:       &limiterMap{m: map[string]*entry{}},
		perUsername: &limiterMap{m: map[string]*entry{}},
		stop:        make(chan struct{}),
	}
	go ll.cleanup()
	return ll
}

// Stop ends the cleanup goroutine. Safe to call once; calling twice will panic
// (close of closed channel) — match Go's stdlib semantics for one-shot stop.
func (ll *LoginLimiter) Stop() { close(ll.stop) }

// get returns the *rate.Limiter for `key`, creating one with fresh burst on
// first access. Updates lastSeen so the cleanup pass treats the entry as live.
func (lm *limiterMap) get(key string) *rate.Limiter {
	lm.Lock()
	defer lm.Unlock()
	e, ok := lm.m[key]
	if !ok {
		e = &entry{lim: rate.NewLimiter(rate.Every(loginRefill), loginBurst)}
		lm.m[key] = e
	}
	e.lastSeen = time.Now()
	return e.lim
}

// Allow consults both the per-IP and per-username buckets. Returns
// (allowedIP, allowedUser); both must be true for the login to proceed.
//
// Username comparison is case-insensitive (we lower the key) so attackers
// cannot rotate "Bob@" / "bob@" / "BOB@" to bypass the per-username bucket.
func (ll *LoginLimiter) Allow(ip, username string) (allowedIP, allowedUser bool) {
	allowedIP = ll.perIP.get(ip).Allow()
	allowedUser = ll.perUsername.get(strings.ToLower(username)).Allow()
	return
}

// RetryAfter returns the duration until the next bucket token regenerates for
// the given (ip, username) pair — used to populate the 429 Retry-After header.
// Whichever bucket has the longer wait wins.
//
// Implementation note: Reserve().Delay() returns the wait time for ONE more
// token. We then immediately Cancel() the reservation so it doesn't actually
// consume the slot — RetryAfter is a query, not a request.
func (ll *LoginLimiter) RetryAfter(ip, username string) time.Duration {
	rIP := ll.perIP.get(ip).Reserve()
	rUser := ll.perUsername.get(strings.ToLower(username)).Reserve()
	a := rIP.Delay()
	b := rUser.Delay()
	rIP.Cancel()
	rUser.Cancel()
	if a > b {
		return a
	}
	return b
}

// cleanup runs in a background goroutine, evicting stale entries from both
// maps on every tick.
func (ll *LoginLimiter) cleanup() {
	t := time.NewTicker(cleanupTick)
	defer t.Stop()
	for {
		select {
		case <-ll.stop:
			return
		case <-t.C:
			ll.RunCleanupOnce()
		}
	}
}

// RunCleanupOnce performs a single cleanup pass synchronously. Exported for
// tests; production paths use the goroutine started by NewLoginLimiter.
func (ll *LoginLimiter) RunCleanupOnce() {
	for _, lm := range []*limiterMap{ll.perIP, ll.perUsername} {
		lm.Lock()
		for k, e := range lm.m {
			if time.Since(e.lastSeen) > cleanupAfter {
				delete(lm.m, k)
			}
		}
		lm.Unlock()
	}
}

// ageEntry rewinds an entry's lastSeen by `d` so the next cleanup pass evicts
// it. Test-only helper — no production caller.
func (ll *LoginLimiter) ageEntry(key string, d time.Duration) {
	for _, lm := range []*limiterMap{ll.perIP, ll.perUsername} {
		lm.Lock()
		if e, ok := lm.m[key]; ok {
			e.lastSeen = e.lastSeen.Add(-d)
		}
		lm.Unlock()
	}
}
