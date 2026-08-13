package httpx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

// Exactly one trusted reverse proxy fronts requests reaching this handler,
// running on loopback, and it appends the connecting peer's address to any
// inbound X-Forwarded-For rather than replacing it. So when the direct peer
// is loopback, the rightmost entry is the address the proxy itself observed,
// while every entry to its left is client-supplied and forgeable. Trusting
// the leftmost entry would let any client forge a fresh rate-limit bucket
// per request; ignoring the header entirely behind that proxy would put the
// whole world in one bucket.
func TestClientIPTrustsForwardedHeaderOnlyFromLoopback(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		forwarded  string
		want       string
	}{
		{name: "ipv4 loopback peer takes the rightmost forwarded value", remoteAddr: "127.0.0.1:53124", forwarded: "203.0.113.7, 70.41.3.18, 150.172.238.178", want: "150.172.238.178"},
		{name: "ipv6 loopback peer resolves a single forwarded value", remoteAddr: "[::1]:9000", forwarded: "203.0.113.9", want: "203.0.113.9"},
		{name: "loopback peer trims whitespace around the rightmost value", remoteAddr: "127.0.0.1:53124", forwarded: "10.0.0.1,   198.51.100.4   ", want: "198.51.100.4"},
		{name: "loopback peer without the header falls back to the peer", remoteAddr: "127.0.0.1:53124", forwarded: "", want: "127.0.0.1"},
		{name: "loopback peer with an empty rightmost value falls back to the peer", remoteAddr: "127.0.0.1:53124", forwarded: "203.0.113.7, ", want: "127.0.0.1"},
		{name: "a forged leftmost value is ignored in favor of the proxy-appended one", remoteAddr: "127.0.0.1:53124", forwarded: "203.0.113.7, 198.51.100.42", want: "198.51.100.42"},
		{name: "public peer never trusts the header", remoteAddr: "203.0.113.55:41234", forwarded: "10.0.0.1", want: "203.0.113.55"},
		{name: "private lan peer never trusts the header", remoteAddr: "192.168.1.20:41234", forwarded: "10.0.0.1", want: "192.168.1.20"},
		{name: "remote address without a port is used as is", remoteAddr: "203.0.113.99", forwarded: "10.0.0.1", want: "203.0.113.99"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations", nil)
			request.RemoteAddr = testCase.remoteAddr
			if testCase.forwarded != "" {
				request.Header.Set("X-Forwarded-For", testCase.forwarded)
			}

			if diff := cmp.Diff(testCase.want, clientIP(request)); diff != "" {
				t.Errorf("unexpected client ip (-want +got):\n%s", diff)
			}
		})
	}
}

// FakeClock is a hand-advanced clock.
// Matches: monotonic forward movement, stable readings between advances.
// Divergences: never advances on its own, not safe for concurrent use.
type FakeClock struct {
	current time.Time
}

func newFakeClock() *FakeClock {
	return &FakeClock{current: time.Date(2026, time.July, 29, 10, 0, 0, 0, time.UTC)}
}

func (clock *FakeClock) Now() time.Time {
	return clock.current
}

func (clock *FakeClock) Advance(delta time.Duration) {
	clock.current = clock.current.Add(delta)
}

// assertPanics runs fn and fails the test unless fn panics.
func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("expected a panic, got none")
		}
	}()
	fn()
}

// The bucket starts full, drains per request and refills continuously at
// capacity per window, capped at capacity. Half a window on a capacity-2 bucket
// must yield exactly one token, so the arithmetic cannot lose the boundary case
// to floating point.
func TestRateLimiterRefillsOverAFakeClock(t *testing.T) {
	clock := newFakeClock()
	limiter := newRateLimiter(2, clock.Now)

	steps := []struct {
		name    string
		advance time.Duration
		want    bool
	}{
		{name: "first request spends a token", advance: 0, want: true},
		{name: "second request drains the bucket", advance: 0, want: true},
		{name: "third request is denied", advance: 0, want: false},
		{name: "half a window refills exactly one token", advance: 30 * time.Second, want: true},
		{name: "and the bucket is empty again", advance: 0, want: false},
		{name: "a full window refills to capacity", advance: time.Minute, want: true},
		{name: "capacity is capped so only one spare token remains", advance: 0, want: true},
		{name: "then denied again", advance: 0, want: false},
		{name: "ten idle windows still cap at capacity", advance: 10 * time.Minute, want: true},
		{name: "second token of the capped bucket", advance: 0, want: true},
		{name: "third is denied", advance: 0, want: false},
	}

	for _, step := range steps {
		clock.Advance(step.advance)
		if got := limiter.Allow("203.0.113.7"); got != step.want {
			t.Errorf("%s: Allow() = %t, want %t", step.name, got, step.want)
		}
	}
}

func TestRateLimiterKeysAreIndependent(t *testing.T) {
	clock := newFakeClock()
	limiter := newRateLimiter(1, clock.Now)

	if !limiter.Allow("203.0.113.7") {
		t.Fatal("first key was denied its only token")
	}
	if limiter.Allow("203.0.113.7") {
		t.Error("first key was allowed a second token")
	}
	if !limiter.Allow("198.51.100.4") {
		t.Error("second key was denied its own token")
	}
}

// Capacity has no library-owned default: a non-positive requestsPerMinute
// can only be a wiring mistake in the caller, so newRateLimiter panics
// instead of silently substituting a value the caller never chose.
func TestNewRateLimiterPanicsOnNonPositiveCapacity(t *testing.T) {
	clock := newFakeClock()

	tests := []struct {
		name              string
		requestsPerMinute int
		wantPanic         bool
	}{
		{name: "zero panics", requestsPerMinute: 0, wantPanic: true},
		{name: "negative panics", requestsPerMinute: -5, wantPanic: true},
		{name: "positive value does not panic", requestsPerMinute: 45, wantPanic: false},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			construct := func() { newRateLimiter(testCase.requestsPerMinute, clock.Now) }
			if testCase.wantPanic {
				assertPanics(t, construct)
				return
			}
			construct()
		})
	}
}

// The bucket map is unbounded input: one entry per source IP. Idle entries are
// swept on a timer so a churn of short-lived clients does not pin memory
// forever.
func TestRateLimiterSweepsIdleBuckets(t *testing.T) {
	clock := newFakeClock()
	limiter := newRateLimiter(20, clock.Now)

	if !limiter.Allow("203.0.113.7") {
		t.Fatal("first request was denied")
	}
	if got := len(limiter.buckets); got != 1 {
		t.Fatalf("bucket count = %d, want 1", got)
	}

	clock.Advance(rateLimitIdleTTL + time.Minute)
	if !limiter.Allow("198.51.100.4") {
		t.Fatal("fresh key was denied")
	}

	if got := len(limiter.buckets); got != 1 {
		t.Errorf("bucket count after sweep = %d, want 1", got)
	}
	if _, present := limiter.buckets["203.0.113.7"]; present {
		t.Error("idle bucket survived the sweep")
	}
}

// A scanner walking through IPs must not be able to grow the map without
// bound and must not be able to evict already-tracked clients by flooding in
// new ones: once the map is at capacity, a brand new key is turned away with
// 429 and the existing entries are left untouched.
func TestRateLimiterRejectsUnknownIPsWhenMapIsFull(t *testing.T) {
	clock := newFakeClock()
	limiter := newRateLimiter(20, clock.Now)
	limiter.maxEntries = 2

	if !limiter.Allow("first") {
		t.Fatal("first key was denied")
	}
	clock.Advance(time.Second)
	if !limiter.Allow("second") {
		t.Fatal("second key was denied")
	}
	clock.Advance(time.Second)
	if limiter.Allow("third") {
		t.Error("a fresh key was allowed once the map was at capacity")
	}

	if got := len(limiter.buckets); got != 2 {
		t.Errorf("bucket count = %d, want 2", got)
	}
	if _, present := limiter.buckets["third"]; present {
		t.Error("the rejected key should not have been added to the map")
	}
	for _, key := range []string{"first", "second"} {
		if _, present := limiter.buckets[key]; !present {
			t.Errorf("bucket %q was evicted even though the map was still within capacity", key)
		}
	}

	if !limiter.Allow("first") {
		t.Error("an already-tracked key was denied service while the map was full")
	}
}

// The middleware keys on the resolved client IP and answers 429 with the
// code too_many_requests, letting a client's error mapper distinguish a
// transient rate limit from any other error code the application defines.
func TestRateLimitMiddlewareKeysOnClientIPAndWritesEnvelope(t *testing.T) {
	clock := newFakeClock()
	handled := 0
	handler := RateLimit(1, clock.Now)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handled++
		w.WriteHeader(http.StatusOK)
	}))

	call := func(forwarded string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/recommendations", nil)
		request.RemoteAddr = "127.0.0.1:53124"
		request.Header.Set("X-Forwarded-For", forwarded)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		return recorder
	}

	if got := call("203.0.113.7").Code; got != http.StatusOK {
		t.Fatalf("first request status = %d, want %d", got, http.StatusOK)
	}

	blocked := call("203.0.113.7")
	if blocked.Code != http.StatusTooManyRequests {
		t.Fatalf("second request status = %d, want %d", blocked.Code, http.StatusTooManyRequests)
	}
	var decoded wireEnvelope
	if err := json.Unmarshal(blocked.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("body is not valid JSON: %v (body %q)", err, blocked.Body.String())
	}
	want := wireEnvelope{Error: wireError{Code: string(CodeTooManyRequests), Message: "too many requests"}}
	if diff := cmp.Diff(want, decoded); diff != "" {
		t.Errorf("unexpected envelope (-want +got):\n%s", diff)
	}

	if got := call("198.51.100.4").Code; got != http.StatusOK {
		t.Errorf("other client status = %d, want %d", got, http.StatusOK)
	}
	if handled != 2 {
		t.Errorf("handler calls = %d, want 2", handled)
	}
}
