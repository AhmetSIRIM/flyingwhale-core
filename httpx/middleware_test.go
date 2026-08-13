package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// RequestID always mints its own identifier; a client-supplied X-Request-Id is
// deliberately not reused, so the value in the log line can never be forged.
func TestRequestIDEchoesAFreshHexIdentifier(t *testing.T) {
	tests := []struct {
		name            string
		inboundHeader   string
		wantHeaderIsSet bool
	}{
		{name: "no inbound header", inboundHeader: "", wantHeaderIsSet: true},
		{name: "inbound header is not reused", inboundHeader: "client-supplied-id", wantHeaderIsSet: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var seenInHandler string
			handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seenInHandler = RequestIDFrom(r.Context())
				w.WriteHeader(http.StatusOK)
			}))

			request := httptest.NewRequest(http.MethodGet, "/v1/example", nil)
			if testCase.inboundHeader != "" {
				request.Header.Set("X-Request-Id", testCase.inboundHeader)
			}
			recorder := httptest.NewRecorder()

			handler.ServeHTTP(recorder, request)

			echoed := recorder.Header().Get("X-Request-Id")
			if testCase.wantHeaderIsSet && echoed == "" {
				t.Fatal("X-Request-Id response header is empty")
			}
			if len(echoed) != 32 {
				t.Errorf("X-Request-Id = %q (len %d), want 32 hex characters", echoed, len(echoed))
			}
			if echoed == testCase.inboundHeader {
				t.Errorf("X-Request-Id reused the inbound value %q", testCase.inboundHeader)
			}
			if seenInHandler != echoed {
				t.Errorf("context id = %q, echoed header = %q, want equal", seenInHandler, echoed)
			}
		})
	}
}

// The id comes from crypto/rand, so a collision across a run of requests
// would point at a broken random source rather than an expected coincidence.
func TestRequestIDIsUniquePerRequest(t *testing.T) {
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	seen := make(map[string]bool, 50)
	for attempt := 0; attempt < 50; attempt++ {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/example", nil))
		id := recorder.Header().Get("X-Request-Id")
		if seen[id] {
			t.Fatalf("duplicate request id %q on attempt %d", id, attempt)
		}
		seen[id] = true
	}
}

// Logging emits exactly one structured line per request carrying the fields
// a log consumer greps for: method, path, status, duration_ms, error_code.
// request_id correlation is NewRequestIDLogHandler's job, covered separately
// in slog_test.go rather than through a bare handler here. The handler runs
// through Logging(Recover(handler)), matching the production composition,
// because error_code has to walk down through both middlewares' own
// statusRecorder, not just the one closest to Logging.
func TestLoggingEmitsStructuredRequestLine(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		path          string
		handler       http.HandlerFunc
		wantStatus    float64
		wantErrorCode string
	}{
		{
			name:   "explicit status",
			method: http.MethodPost,
			path:   "/v1/example",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusCreated)
			},
			wantStatus: 201,
		},
		{
			name:   "implicit 200 when the handler only writes a body",
			method: http.MethodGet,
			path:   "/v1/example",
			handler: func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(`{}`))
			},
			wantStatus: 200,
		},
		{
			name:   "error envelope logs the exact code WriteError wrote",
			method: http.MethodPost,
			path:   "/v1/example",
			handler: func(w http.ResponseWriter, r *http.Request) {
				WriteError(w, http.StatusForbidden, Code("forbidden"), "forbidden")
			},
			wantStatus:    403,
			wantErrorCode: "forbidden",
		},
		{
			name:   "WriteError after an earlier WriteHeader logs no error_code because the header already committed",
			method: http.MethodPost,
			path:   "/v1/example",
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				WriteError(w, http.StatusForbidden, Code("forbidden"), "forbidden")
			},
			wantStatus: 200,
		},
		{
			name:   "a second WriteError does not override the first error_code, the only envelope the client decoded",
			method: http.MethodPost,
			path:   "/v1/example",
			handler: func(w http.ResponseWriter, r *http.Request) {
				WriteError(w, http.StatusForbidden, Code("forbidden"), "forbidden")
				WriteError(w, http.StatusInternalServerError, CodeInternal, "internal error")
			},
			wantStatus:    403,
			wantErrorCode: "forbidden",
		},
		{
			name:   "WriteJSON's marshal failure fallback logs the internal code it substituted",
			method: http.MethodGet,
			path:   "/v1/example",
			handler: func(w http.ResponseWriter, r *http.Request) {
				WriteJSON(w, http.StatusOK, map[string]any{"value": math.NaN()})
			},
			wantStatus:    500,
			wantErrorCode: string(CodeInternal),
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))

			handler := RequestID(Logging(logger)(Recover(logger)(testCase.handler)))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(testCase.method, testCase.path, nil))

			var logLine map[string]any
			if err := json.Unmarshal(logBuffer.Bytes(), &logLine); err != nil {
				t.Fatalf("log line is not valid JSON: %v (line %q)", err, logBuffer.String())
			}

			got := map[string]any{
				"method": logLine["method"],
				"path":   logLine["path"],
				"status": logLine["status"],
			}
			want := map[string]any{
				"method": testCase.method,
				"path":   testCase.path,
				"status": testCase.wantStatus,
			}
			if errorCode, present := logLine["error_code"]; present {
				got["error_code"] = errorCode
			}
			if testCase.wantErrorCode != "" {
				want["error_code"] = testCase.wantErrorCode
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("unexpected log fields (-want +got):\n%s", diff)
			}
			if _, present := logLine["duration_ms"]; !present {
				t.Error("log line has no duration_ms field")
			}
		})
	}
}

// A panic must never reach the client as a dropped connection: the envelope is
// the only thing a client's error mapper can read.
func TestRecoverTurnsPanicsIntoInternalEnvelope(t *testing.T) {
	tests := []struct {
		name         string
		panicValue   any
		wantRecovery bool
		wantStatus   int
	}{
		{name: "string panic", panicValue: "boom", wantRecovery: true, wantStatus: http.StatusInternalServerError},
		{name: "error panic", panicValue: errors.New("nil map write"), wantRecovery: true, wantStatus: http.StatusInternalServerError},
		{name: "no panic passes through", panicValue: nil, wantRecovery: false, wantStatus: http.StatusNoContent},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))

			handler := RequestID(Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if testCase.panicValue != nil {
					panic(testCase.panicValue)
				}
				w.WriteHeader(http.StatusNoContent)
			})))

			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/example", nil))

			if recorder.Code != testCase.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, testCase.wantStatus)
			}
			if !testCase.wantRecovery {
				return
			}

			var decoded wireEnvelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
				t.Fatalf("body is not valid JSON: %v (body %q)", err, recorder.Body.String())
			}
			want := wireEnvelope{Error: wireError{Code: string(CodeInternal), Message: "internal server error"}}
			if diff := cmp.Diff(want, decoded); diff != "" {
				t.Errorf("unexpected envelope (-want +got):\n%s", diff)
			}
			if !strings.Contains(logBuffer.String(), "panic recovered") {
				t.Errorf("log does not mention the recovered panic: %q", logBuffer.String())
			}
		})
	}
}

// A handler that has already sent a header (and possibly part of a body)
// before panicking must not have an error envelope appended on top: the
// client would see a 200 with a corrupted, unparseable body. Recover must
// still log the panic even though it cannot write the envelope.
func TestRecoverSkipsWriteErrorWhenHeaderAlreadySent(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))

	handler := RequestID(Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic("boom after a partial write")
	})))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/example", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (Recover must not overwrite an already-sent header)", recorder.Code, http.StatusOK)
	}
	if got := recorder.Body.String(); got != "partial" {
		t.Errorf("body = %q, want %q (Recover must not append an error envelope to a started body)", got, "partial")
	}
	if !strings.Contains(logBuffer.String(), "panic recovered") {
		t.Errorf("log does not mention the recovered panic: %q", logBuffer.String())
	}
}

// net/http honors only the first WriteHeader call on a request; a recorder
// that captured the last call instead of the first would misreport the
// status in every access log line following a handler that (incorrectly,
// but not uncommonly) calls WriteHeader more than once.
func TestStatusRecorderKeepsTheFirstWriteHeaderCall(t *testing.T) {
	underlying := httptest.NewRecorder()
	recorder := newStatusRecorder(underlying)

	recorder.WriteHeader(http.StatusCreated)
	recorder.WriteHeader(http.StatusInternalServerError)

	if recorder.status != http.StatusCreated {
		t.Errorf("recorder.status = %d, want %d (first WriteHeader call wins)", recorder.status, http.StatusCreated)
	}
	if underlying.Code != http.StatusCreated {
		t.Errorf("underlying status = %d, want %d", underlying.Code, http.StatusCreated)
	}
}

// A statusRecorder wraps but does not hide the underlying ResponseWriter:
// Unwrap lets http.ResponseController reach a method statusRecorder does
// not implement itself, such as Flush.
func TestStatusRecorderUnwrapsForResponseController(t *testing.T) {
	underlying := httptest.NewRecorder()
	recorder := newStatusRecorder(underlying)

	controller := http.NewResponseController(recorder)
	if err := controller.Flush(); err != nil {
		t.Fatalf("Flush() through the wrapped recorder returned %v, want nil", err)
	}
	if !underlying.Flushed {
		t.Error("underlying httptest.ResponseRecorder was not flushed")
	}
}

// Logging must emit its request line even when a downstream panic is
// recovered rather than propagated, and the recommended composition order
// (Logging outside Recover) means the line reports the status Recover
// actually wrote, not the pre-panic default.
func TestLoggingSurvivesADownstreamPanicWhenComposedOutsideRecover(t *testing.T) {
	var logBuffer bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logBuffer, nil))

	panicking := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	})
	handler := RequestID(Logging(logger)(Recover(logger)(panicking)))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/v1/example", nil))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}

	requestLine := findLogLineByMessage(t, logBuffer.Bytes(), "request")
	if got := requestLine["status"]; got != float64(http.StatusInternalServerError) {
		t.Errorf("request log status = %v, want %v", got, http.StatusInternalServerError)
	}
}

// findLogLineByMessage scans newline-delimited JSON log records for the one
// whose "msg" field matches, since Logging and Recover may share a logger and
// therefore a buffer.
func findLogLineByMessage(t *testing.T, logOutput []byte, message string) map[string]any {
	t.Helper()

	for _, line := range bytes.Split(bytes.TrimSpace(logOutput), []byte("\n")) {
		var decoded map[string]any
		if err := json.Unmarshal(line, &decoded); err != nil {
			t.Fatalf("log line is not valid JSON: %v (line %q)", err, line)
		}
		if decoded["msg"] == message {
			return decoded
		}
	}

	t.Fatalf("no log line with msg %q found in %q", message, logOutput)
	return nil
}
