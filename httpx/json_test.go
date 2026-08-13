package httpx

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

// wireError and wireEnvelope decode the response independently of the
// production types, so a rename of an unexported field cannot hide a
// contract break behind a shared struct.
type wireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type wireEnvelope struct {
	Error wireError `json:"error"`
}

func TestWriteErrorEnvelope(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		code    Code
		message string
	}{
		{name: "bad request", status: http.StatusBadRequest, code: Code("invalid_request"), message: "the request body is invalid"},
		{name: "rate limited", status: http.StatusTooManyRequests, code: CodeTooManyRequests, message: "too many requests"},
		{name: "forbidden", status: http.StatusForbidden, code: Code("forbidden"), message: "forbidden"},
		{name: "internal error", status: http.StatusInternalServerError, code: CodeInternal, message: "internal server error"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()

			WriteError(recorder, testCase.status, testCase.code, testCase.message)

			if recorder.Code != testCase.status {
				t.Errorf("status = %d, want %d", recorder.Code, testCase.status)
			}
			if got := recorder.Header().Get("Content-Type"); got != jsonContentType {
				t.Errorf("Content-Type = %q, want %q", got, jsonContentType)
			}

			var decoded wireEnvelope
			if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
				t.Fatalf("body is not valid JSON: %v (body %q)", err, recorder.Body.String())
			}

			want := wireEnvelope{Error: wireError{Code: string(testCase.code), Message: testCase.message}}
			if diff := cmp.Diff(want, decoded); diff != "" {
				t.Errorf("unexpected envelope (-want +got):\n%s", diff)
			}
		})
	}
}

func TestWriteJSONPayload(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		payload any
		want    map[string]any
	}{
		{
			name:    "object payload",
			status:  http.StatusOK,
			payload: map[string]any{"enabled": true},
			want:    map[string]any{"enabled": true},
		},
		{
			name:    "empty slice payload",
			status:  http.StatusOK,
			payload: map[string]any{"items": []any{}},
			want:    map[string]any{"items": []any{}},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()

			WriteJSON(recorder, testCase.status, testCase.payload)

			if recorder.Code != testCase.status {
				t.Errorf("status = %d, want %d", recorder.Code, testCase.status)
			}
			if got := recorder.Header().Get("Content-Type"); got != jsonContentType {
				t.Errorf("Content-Type = %q, want %q", got, jsonContentType)
			}

			var decoded map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
				t.Fatalf("body is not valid JSON: %v (body %q)", err, recorder.Body.String())
			}
			if diff := cmp.Diff(testCase.want, decoded); diff != "" {
				t.Errorf("unexpected body (-want +got):\n%s", diff)
			}
		})
	}
}

// A payload that fails to marshal (a NaN float has no JSON representation)
// must fall back to a 500 internal envelope rather than committing a 200
// header ahead of the encode and then writing a truncated or empty body.
func TestWriteJSONMarshalFailureFallsBackToInternalEnvelope(t *testing.T) {
	recorder := httptest.NewRecorder()

	WriteJSON(recorder, http.StatusOK, map[string]any{"value": math.NaN()})

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if got := recorder.Header().Get("Content-Type"); got != jsonContentType {
		t.Errorf("Content-Type = %q, want %q", got, jsonContentType)
	}

	var decoded wireEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("body is not valid JSON: %v (body %q)", err, recorder.Body.String())
	}
	want := wireEnvelope{Error: wireError{Code: string(CodeInternal), Message: "internal server error"}}
	if diff := cmp.Diff(want, decoded); diff != "" {
		t.Errorf("unexpected envelope (-want +got):\n%s", diff)
	}
}

// decodeTarget stands in for a real request body: one known field, so an
// unknown field in the payload proves the ignore policy rather than the
// absence of validation.
type decodeTarget struct {
	Text string `json:"text"`
}

func TestDecodeJSON(t *testing.T) {
	oversized := `{"text":"` + strings.Repeat("a", 9000) + `"}`

	tests := []struct {
		name      string
		body      string
		wantText  string
		wantError bool
	}{
		{name: "known field decodes", body: `{"text":"hello"}`, wantText: "hello"},
		{name: "unknown field is ignored", body: `{"text":"hello","rating":5,"nested":{"a":1}}`, wantText: "hello"},
		{name: "missing field stays zero", body: `{}`, wantText: ""},
		{name: "malformed json errors", body: `{"text":`, wantError: true},
		{name: "body over 8 KB errors", body: oversized, wantError: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/example", strings.NewReader(testCase.body))
			recorder := httptest.NewRecorder()

			var decoded decodeTarget
			err := DecodeJSON(recorder, request, &decoded)

			if testCase.wantError {
				if err == nil {
					t.Fatalf("DecodeJSON() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeJSON() error = %v, want nil", err)
			}
			if diff := cmp.Diff(testCase.wantText, decoded.Text); diff != "" {
				t.Errorf("unexpected text (-want +got):\n%s", diff)
			}
		})
	}
}

func TestDecodeJSONBodyLimitIsEightKilobytes(t *testing.T) {
	if diff := cmp.Diff(int64(8192), maxRequestBodyBytes); diff != "" {
		t.Errorf("unexpected body limit (-want +got):\n%s", diff)
	}
}

// TestDecodeJSONLimit proves the caller-supplied cap is honored independently
// of DecodeJSON's own 8KB default, so a consumer with a legitimately larger
// body can opt into a bigger ceiling.
func TestDecodeJSONLimit(t *testing.T) {
	const customLimit int64 = 64 << 10

	withinLimit := `{"text":"` + strings.Repeat("a", int(customLimit)-20) + `"}`
	overLimit := `{"text":"` + strings.Repeat("a", int(customLimit)+1000) + `"}`

	tests := []struct {
		name      string
		body      string
		wantError bool
	}{
		{name: "body within the custom limit decodes", body: withinLimit, wantError: false},
		{name: "body over the custom limit errors", body: overLimit, wantError: true},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/example", strings.NewReader(testCase.body))
			recorder := httptest.NewRecorder()

			var decoded decodeTarget
			err := DecodeJSONLimit(recorder, request, &decoded, customLimit)

			if testCase.wantError {
				if err == nil {
					t.Fatalf("DecodeJSONLimit() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("DecodeJSONLimit() error = %v, want nil", err)
			}
		})
	}
}
