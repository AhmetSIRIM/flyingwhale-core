// Package httpx provides the HTTP primitives shared by FlyingWhale servers:
// a JSON error envelope with a shared code vocabulary, request id and
// structured logging middlewares, a panic-recovery middleware, a rate
// limiter, a router with prefixed and middleware-chained route groups,
// Accept-Language negotiation, and a slog handler that correlates log lines
// with a request. A request-scoped status recorder couples the envelope and
// the middlewares so a recorded error code reaches the access log line that
// matches what the client actually received. Policy defaults (rate limit
// capacity, supported languages, a fallback locale) come from the caller.
// The package keeps its own safety caps: the request body limit, which
// DecodeJSONLimit overrides, and the rate limiter's bucket-map cap, which
// fails closed on new keys once the map is full.
package httpx
