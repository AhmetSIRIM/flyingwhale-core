package httpx

import (
	"context"
	"log/slog"
)

// NewRequestIDLogHandler wraps inner so every log record whose context
// carries a request id (set by RequestID) gains a request_id attribute,
// correlating any *Context log call anywhere in the call graph with the
// access log line for the same request, without repeating the attribute at
// each call site.
func NewRequestIDLogHandler(inner slog.Handler) slog.Handler {
	return requestIDHandler{inner}
}

type requestIDHandler struct {
	slog.Handler
}

func (h requestIDHandler) Handle(ctx context.Context, record slog.Record) error {
	if id := RequestIDFrom(ctx); id != "" {
		record.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, record)
}

// WithAttrs and WithGroup re-wrap the result in requestIDHandler rather than
// unwrapping to the bare inner handler, so request_id keeps propagating
// through a logger derived via With or WithGroup.
func (h requestIDHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return requestIDHandler{h.Handler.WithAttrs(attrs)}
}

func (h requestIDHandler) WithGroup(name string) slog.Handler {
	return requestIDHandler{h.Handler.WithGroup(name)}
}
