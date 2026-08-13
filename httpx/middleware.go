package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// Middleware wraps an http.Handler with additional behavior.
//
// Compose Logging outside Recover (Logging(logger)(Recover(logger)(next)))
// so a panic recovered downstream is written through Logging's own status
// recorder and the request line reports the resulting status.
type Middleware func(http.Handler) http.Handler

const requestIDHeader = "X-Request-Id"

type requestIDContextKey struct{}

// RequestID assigns a random id to the request, sets it on the response's
// X-Request-Id header, and stores it in the request context for
// RequestIDFrom.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		w.Header().Set(requestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDContextKey{}, id)))
	})
}

// RequestIDFrom returns the request id RequestID stored on ctx, or an empty
// string if RequestID never ran on this request.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

func newRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(buffer)
}

type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
	errorCode   Code
}

func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{ResponseWriter: w, status: http.StatusOK}
}

// net/http honors only the first WriteHeader call and logs subsequent ones as
// superfluous, so the recorder keeps the first status and ignores the rest.
func (recorder *statusRecorder) WriteHeader(status int) {
	if recorder.wroteHeader {
		return
	}
	recorder.wroteHeader = true
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func (recorder *statusRecorder) Write(body []byte) (int, error) {
	if !recorder.wroteHeader {
		recorder.WriteHeader(http.StatusOK)
	}
	return recorder.ResponseWriter.Write(body)
}

// The header is first-write-wins (see WriteHeader above), and the log line
// must tell the same story the wire does: once a header has committed, a
// later WriteError describes an envelope the client never received, so the
// code is dropped rather than overwriting the one that matches what shipped.
// Logging and Recover each wrap the ResponseWriter in their own statusRecorder,
// so a recorded code must still walk down to every nested recorder the same
// way WriteHeader's status already does through the embedded ResponseWriter
// call.
func (recorder *statusRecorder) recordErrorCode(code Code) {
	if recorder.wroteHeader {
		return
	}
	recorder.errorCode = code
	if inner, ok := recorder.ResponseWriter.(errorCodeRecorder); ok {
		inner.recordErrorCode(code)
	}
}

type errorCodeRecorder interface {
	recordErrorCode(code Code)
}

// Logging returns a Middleware that logs one line per request through
// logger, including the response status and any error code WriteError
// recorded on it.
func Logging(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := newStatusRecorder(w)

			defer func() {
				attrs := []any{
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", recorder.status),
					slog.Int64("duration_ms", time.Since(started).Milliseconds()),
				}
				if recorder.errorCode != "" {
					attrs = append(attrs, slog.String("error_code", string(recorder.errorCode)))
				}
				logger.InfoContext(r.Context(), "request", attrs...)
			}()

			next.ServeHTTP(recorder, r)
		})
	}
}

// Recover returns a Middleware that recovers a panic from the wrapped
// handler, logs it through logger, and responds with a 500 envelope if the
// response has not already committed a header.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			recorder := newStatusRecorder(w)

			defer func() {
				recovered := recover()
				if recovered == nil {
					return
				}
				logger.ErrorContext(r.Context(), "panic recovered",
					slog.Any("panic", recovered),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.String("stack", string(debug.Stack())),
				)
				if recorder.wroteHeader {
					return
				}
				WriteError(recorder, http.StatusInternalServerError, CodeInternal, "internal server error")
			}()

			next.ServeHTTP(recorder, r)
		})
	}
}
