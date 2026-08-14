package httpx

// Code identifies a machine-readable error category carried in an error
// envelope's body.
type Code string

// CodeInternal and CodeTooManyRequests are append-only: a client maps these
// wire strings against known cases, so an existing value never changes and
// an existing constant is never renamed once released. Any other code is
// defined by the application that owns it.
const (
	CodeInternal        Code = "internal"
	CodeTooManyRequests Code = "too_many_requests"
)
