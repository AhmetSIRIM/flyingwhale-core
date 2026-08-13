// Package httpx provides the HTTP primitives shared by FlyingWhale servers:
// a JSON error envelope, request id and structured logging middlewares, a
// panic-recovery middleware, and a slog handler that correlates log lines
// with a request. A request-scoped status recorder couples the envelope and
// the middlewares so a recorded error code reaches the access log line that
// matches what the client actually received.
package httpx
