package httpx

import (
	"encoding/json"
	"net/http"
)

const jsonContentType = "application/json; charset=utf-8"

const maxRequestBodyBytes int64 = 8 << 10

type errorDetail struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

// WriteJSON writes payload to w as a JSON body with the given status code.
//
// Marshal happens before the header is written so an encoding failure (for
// example a NaN float) can still fall back to a 500 instead of leaving a
// 200 header already committed with a truncated body. The header is not
// written yet at that point, so the substituted code is still eligible to
// be recorded under the same first-write-wins rule as any other code.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		status = http.StatusInternalServerError
		if recorder, ok := w.(errorCodeRecorder); ok {
			recorder.recordErrorCode(CodeInternal)
		}
		body, _ = json.Marshal(errorEnvelope{Error: errorDetail{Code: CodeInternal, Message: "internal server error"}})
	}
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// WriteError records the code on the writer when it is the package's own
// recorder. The assertion works only because WriteError and statusRecorder
// live in the same package, so no exported type has to carry the code across
// a boundary that does not exist.
func WriteError(w http.ResponseWriter, status int, code Code, message string) {
	if recorder, ok := w.(errorCodeRecorder); ok {
		recorder.recordErrorCode(code)
	}
	WriteJSON(w, status, errorEnvelope{Error: errorDetail{Code: code, Message: message}})
}

// DecodeJSON decodes the request body of r as JSON into dst, capping the
// body at 8KB.
//
// Unknown fields are ignored on purpose: a shipped client binary may keep
// sending a field this server has already stopped reading.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	return DecodeJSONLimit(w, r, dst, maxRequestBodyBytes)
}

// DecodeJSONLimit is DecodeJSON with a caller-supplied body cap, for the rare
// consumer whose legitimate payloads run past the 8KB default.
func DecodeJSONLimit(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	return json.NewDecoder(r.Body).Decode(dst)
}
