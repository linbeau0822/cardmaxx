package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// ErrorCode is a stable, machine-readable identifier for a failure. The
// frontend switches on these, never on the human-readable message.
type ErrorCode string

const (
	CodeNotFound         ErrorCode = "not_found"
	CodeMethodNotAllowed ErrorCode = "method_not_allowed"
	CodeNotImplemented   ErrorCode = "not_implemented"
	CodeBadRequest       ErrorCode = "bad_request"
	CodeInternal         ErrorCode = "internal_error"
	CodeUnavailable      ErrorCode = "service_unavailable"
)

// errorResponse is the single error envelope used by every endpoint.
type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

// writeJSON serialises v as the response body.
//
// The payload is encoded before any header is written so an encoding failure
// still produces a 500 instead of a truncated 200 body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		slog.Error("encoding response failed", slog.Any("error", err))
		http.Error(w, `{"error":{"code":"`+string(CodeInternal)+`","message":"failed to encode response"}}`,
			http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	// A failed write means the client went away; there is nothing left to send.
	_, _ = w.Write(body)
}

// writeError returns the standard error envelope.
func writeError(w http.ResponseWriter, status int, code ErrorCode, message string) {
	writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message}})
}
