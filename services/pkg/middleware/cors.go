package middleware

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"
)

// CORS applies origin allowlist and credential headers for HttpOnly cookie auth.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (slices.Contains(allowedOrigins, "*") || slices.Contains(allowedOrigins, origin)) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Idempotency-Key, X-Admission-Token")
				w.Header().Set("Access-Control-Allow-Methods", strings.Join([]string{
					http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions,
				}, ", "))
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Recover converts panics inside HTTP handlers into clean 500 JSON responses.
func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if logger != nil {
						logger.Error("panic recovered", slog.Any("panic", rec), slog.String("path", r.URL.Path))
					}
					WriteError(w, http.StatusInternalServerError, "internal_error", "unexpected server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
