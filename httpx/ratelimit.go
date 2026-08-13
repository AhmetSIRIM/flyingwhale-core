package httpx

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Exactly one trusted reverse proxy fronts requests reaching this handler,
// running on loopback, and it appends the connecting peer's address to any
// inbound X-Forwarded-For rather than replacing it. So when the direct peer
// is loopback, the rightmost entry is the address the proxy itself observed,
// while every entry to its left is client-supplied and forgeable. Trusting
// the leftmost entry would let any client forge a fresh rate-limit bucket
// per request; ignoring the header from a loopback peer would put the whole
// world in one bucket.
func clientIP(r *http.Request) string {
	peerHost := remoteHost(r.RemoteAddr)
	if !isLoopbackHost(peerHost) {
		return peerHost
	}
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded == "" {
		return peerHost
	}
	parts := strings.Split(forwarded, ",")
	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return peerHost
	}
	return last
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func isLoopbackHost(host string) bool {
	parsed := net.ParseIP(host)
	return parsed != nil && parsed.IsLoopback()
}

const (
	rateLimitWindow        = time.Minute
	rateLimitIdleTTL       = 10 * time.Minute
	rateLimitSweepInterval = time.Minute
	rateLimitMaxEntries    = 10000
)

type tokenBucket struct {
	tokens   float64
	lastSeen time.Time
}

type rateLimiter struct {
	mu            sync.Mutex
	buckets       map[string]*tokenBucket
	capacity      float64
	window        time.Duration
	idleTTL       time.Duration
	sweepInterval time.Duration
	maxEntries    int
	lastSweep     time.Time
	now           func() time.Time
}

// newRateLimiter panics if requestsPerMinute is not positive: capacity has
// no library-owned default, so a non-positive value can only be a wiring
// mistake in the caller.
func newRateLimiter(requestsPerMinute int, now func() time.Time) *rateLimiter {
	if requestsPerMinute <= 0 {
		panic(fmt.Sprintf("httpx: RateLimit requires a positive requestsPerMinute, got %d", requestsPerMinute))
	}
	if now == nil {
		now = time.Now
	}
	return &rateLimiter{
		buckets:       make(map[string]*tokenBucket),
		capacity:      float64(requestsPerMinute),
		window:        rateLimitWindow,
		idleTTL:       rateLimitIdleTTL,
		sweepInterval: rateLimitSweepInterval,
		maxEntries:    rateLimitMaxEntries,
		lastSweep:     now(),
		now:           now,
	}
}

func (limiter *rateLimiter) Allow(key string) bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	now := limiter.now()
	limiter.sweepLocked(now)

	bucket, found := limiter.buckets[key]
	if !found {
		// A full map fails closed on new keys instead of evicting a tracked
		// client: growing past the cap risks unbounded memory, while evicting
		// would let a flood of unseen IPs bump already-tracked clients out.
		if len(limiter.buckets) >= limiter.maxEntries {
			return false
		}
		bucket = &tokenBucket{tokens: limiter.capacity}
		limiter.buckets[key] = bucket
	} else if elapsed := now.Sub(bucket.lastSeen); elapsed > 0 {
		// Ratio form, not tokens-per-second: capacity * (elapsed/window) keeps
		// exact window fractions exact instead of landing a hair below one token.
		refilled := bucket.tokens + limiter.capacity*(elapsed.Seconds()/limiter.window.Seconds())
		if refilled > limiter.capacity {
			refilled = limiter.capacity
		}
		bucket.tokens = refilled
	}
	bucket.lastSeen = now

	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

// RateLimit rate-limits by client IP using a token bucket refilled at
// requestsPerMinute tokens per minute. Capacity is required and has no
// library-owned default: RateLimit panics if requestsPerMinute is not
// positive, so a missing or misconfigured limit fails at wiring time rather
// than silently falling back to a value the caller never chose. It keys on
// the peer address, trusting X-Forwarded-For only from a loopback peer and
// only its rightmost entry, which suits a single trusted reverse proxy on
// loopback.
func RateLimit(requestsPerMinute int, now func() time.Time) Middleware {
	limiter := newRateLimiter(requestsPerMinute, now)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow(clientIP(r)) {
				WriteError(w, http.StatusTooManyRequests, CodeTooManyRequests, "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (limiter *rateLimiter) sweepLocked(now time.Time) {
	if now.Sub(limiter.lastSweep) < limiter.sweepInterval {
		return
	}
	limiter.lastSweep = now
	for key, bucket := range limiter.buckets {
		if now.Sub(bucket.lastSeen) >= limiter.idleTTL {
			delete(limiter.buckets, key)
		}
	}
}
