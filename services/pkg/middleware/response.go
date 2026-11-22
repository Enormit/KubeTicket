package middleware

import (
	"encoding/json"
	"net/http"
)

// ErrorBody defines a consistent JSON error response structure.
type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// WriteJSON serializes payload with the specified HTTP status code.
func WriteJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// WriteError returns a structured JSON error payload.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	WriteJSON(w, status, ErrorBody{
		Error:   code,
		Message: message,
	})
}
