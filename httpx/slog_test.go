package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRequestIDHandlerCorrelatesLogLinesWithTheResponseHeader proves the slog
// wrapper carries request_id onto any *Context log call reached through
// RequestID's context, correlating it with the same id echoed in the
// response header.
func TestRequestIDHandlerCorrelatesLogLinesWithTheResponseHeader(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(NewRequestIDLogHandler(slog.NewJSONHandler(&logBuffer, nil)))

	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.ErrorContext(r.Context(), "handler level error")
		w.WriteHeader(http.StatusInternalServerError)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	echoed := recorder.Header().Get("X-Request-Id")
	if echoed == "" {
		t.Fatal("X-Request-Id response header is empty")
	}

	var logLine map[string]any
	if err := json.Unmarshal(logBuffer.Bytes(), &logLine); err != nil {
		t.Fatalf("log line is not valid JSON: %v (line %q)", err, logBuffer.String())
	}
	if got := logLine["request_id"]; got != echoed {
		t.Errorf("request_id = %v, want %q", got, echoed)
	}
}

// TestRequestIDHandlerHasNoAttrWhenTheContextCarriesNoRequestID guards the
// "when non-empty" branch: a log call made outside any request (a boot-time
// log line, for instance) must not gain a blank request_id attribute.
func TestRequestIDHandlerHasNoAttrWhenTheContextCarriesNoRequestID(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(NewRequestIDLogHandler(slog.NewJSONHandler(&logBuffer, nil)))

	logger.Info("boot")

	var logLine map[string]any
	if err := json.Unmarshal(logBuffer.Bytes(), &logLine); err != nil {
		t.Fatalf("log line is not valid JSON: %v (line %q)", err, logBuffer.String())
	}
	if _, present := logLine["request_id"]; present {
		t.Errorf("log line carries a request_id attribute outside any request: %v", logLine)
	}
}

// TestRequestIDHandlerSurvivesWith proves WithAttrs and WithGroup re-wrap the
// result in requestIDHandler instead of unwrapping to the bare embedded
// handler, since logger.With is a common way to attach fixed attrs.
func TestRequestIDHandlerSurvivesWith(t *testing.T) {
	base := NewRequestIDLogHandler(slog.NewJSONHandler(&bytes.Buffer{}, nil))

	if _, ok := base.WithAttrs([]slog.Attr{slog.String("component", "test")}).(requestIDHandler); !ok {
		t.Error("WithAttrs must return a requestIDHandler so request_id keeps flowing")
	}
	if _, ok := base.WithGroup("group").(requestIDHandler); !ok {
		t.Error("WithGroup must return a requestIDHandler so request_id keeps flowing")
	}
}
