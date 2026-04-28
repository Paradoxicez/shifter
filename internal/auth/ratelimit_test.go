package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestLogin_RateLimit_PerIP — The 6th login attempt from the same source IP
// inside the rolling 1-minute window returns false for the IP bucket. Earlier
// attempts return true. AUTH-04.
func TestLogin_RateLimit_PerIP(t *testing.T) {
	ll := NewLoginLimiter()
	defer ll.Stop()

	ip := "10.0.0.1"
	user := "alice@example.com"
	for i := 0; i < loginBurst; i++ {
		ipOK, userOK := ll.Allow(ip, user)
		require.True(t, ipOK, "ip allow %d", i)
		require.True(t, userOK)
	}
	// 6th attempt — IP bucket should now refuse.
	ipOK, _ := ll.Allow(ip, user)
	require.False(t, ipOK, "AUTH-04: 6th login from same IP must be rate-limited")
}

// TestLogin_RateLimit_PerUsername — The 6th login attempt for the same
// username across DIFFERENT source IPs inside the rolling 1-minute window
// returns false for the username bucket. Defends against distributed
// credential stuffing. AUTH-04.
func TestLogin_RateLimit_PerUsername(t *testing.T) {
	ll := NewLoginLimiter()
	defer ll.Stop()

	user := "bob@example.com"
	for i := 0; i < loginBurst; i++ {
		ipOK, userOK := ll.Allow("ip-"+string(rune('a'+i)), user) // each request from a different IP
		require.True(t, ipOK)
		require.True(t, userOK, "username allow %d", i)
	}
	_, userOK := ll.Allow("ip-z", user)
	require.False(t, userOK, "AUTH-04: 6th login for same username (any IP) must be rate-limited")
}

// TestRateLimit_Cleanup — Stale entries (lastSeen older than cleanupAfter) are
// removed when the cleanup pass runs.
func TestRateLimit_Cleanup(t *testing.T) {
	ll := NewLoginLimiter()
	defer ll.Stop()
	_, _ = ll.Allow("a", "u@x")
	ll.ageEntry("a", 2*time.Hour)
	ll.RunCleanupOnce()
	ll.perIP.Lock()
	_, exists := ll.perIP.m["a"]
	ll.perIP.Unlock()
	require.False(t, exists, "stale entry should be cleaned up")
}
