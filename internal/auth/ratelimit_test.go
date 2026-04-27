package auth

import "testing"

// TestLogin_RateLimit_PerIP — The 6th failed login from the same source IP
// inside the rolling 1-minute window returns 429. Earlier attempts succeed
// (or return 401 for bad creds).
// Implementation: Plan 09 (login-ratelimit).
func TestLogin_RateLimit_PerIP(t *testing.T) {
	t.Skip("Plan 09: per-IP rate limit pending")
}

// TestLogin_RateLimit_PerUsername — The 6th failed login for the same username
// across DIFFERENT source IPs inside the rolling 1-minute window returns 429.
// Defends against distributed credential stuffing.
// Implementation: Plan 09 (login-ratelimit).
func TestLogin_RateLimit_PerUsername(t *testing.T) {
	t.Skip("Plan 09: per-username rate limit pending")
}

// TestRateLimit_Cleanup — The cleanup goroutine evicts buckets older than
// the rolling window so memory does not grow unbounded under attack.
// Implementation: Plan 09 (login-ratelimit).
func TestRateLimit_Cleanup(t *testing.T) {
	t.Skip("Plan 09: rate-limit GC pending")
}
