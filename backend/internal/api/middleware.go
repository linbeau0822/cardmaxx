package api

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// requestLogger logs one structured line per request.
//
// This replaces chi's built-in logger so access logs share the application's
// JSON format and carry the request id, which is what makes them correlatable.
func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			defer func() {
				logger.InfoContext(r.Context(), "http request",
					slog.String("request_id", middleware.GetReqID(r.Context())),
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", ww.Status()),
					slog.Int("bytes", ww.BytesWritten()),
					slog.Duration("duration", time.Since(start)),
					slog.String("remote_addr", r.RemoteAddr),
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// cors allows the configured browser origins to call the API.
//
// Written by hand rather than pulled from a dependency: the requirement is one
// allow-list and a preflight response, and credentials are deliberately not
// allowed because the API will authenticate with a bearer token (Phase 8).
func cors(allowedOrigins []string) func(http.Handler) http.Handler {
	allowedMethods := strings.Join([]string{
		http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch,
		http.MethodDelete, http.MethodOptions,
	}, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			// Responses vary by Origin, so caches must not share them.
			w.Header().Add("Vary", "Origin")

			if origin != "" && slices.Contains(allowedOrigins, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", allowedMethods)
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type")
				w.Header().Set("Access-Control-Max-Age", "300")
			}

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
