package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/linbeau0822/cardmaxx/backend/internal/config"
)

// newTestRouter builds the real router. Every handler is dependency-free, so
// these tests need no database.
func newTestRouter() http.Handler {
	cfg := config.ServerConfig{AllowedOrigins: []string{"http://localhost:5173"}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(cfg, logger)
}

func do(t *testing.T, method, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	newTestRouter().ServeHTTP(rec, req)
	return rec
}

func TestHealth(t *testing.T) {
	rec := do(t, http.MethodGet, "/health", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body: %s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got, want := rec.Header().Get("Content-Type"), "application/json; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}
	if body["status"] != "ok" {
		t.Errorf(`body = %v, want {"status":"ok"}`, body)
	}
}

func TestUnimplementedEndpointsReturn501(t *testing.T) {
	routes := []struct {
		method, path string
	}{
		{http.MethodPost, "/api/v1/plaid/link-token"},
		{http.MethodPost, "/api/v1/plaid/exchange-token"},
		{http.MethodPost, "/api/v1/plaid/sync"},
		{http.MethodGet, "/api/v1/accounts"},
		{http.MethodGet, "/api/v1/transactions"},
		{http.MethodGet, "/api/v1/cards"},
		{http.MethodGet, "/api/v1/analysis/spending"},
		{http.MethodGet, "/api/v1/recommendations"},
	}

	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			rec := do(t, route.method, route.path, nil)

			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotImplemented)
			}

			var body errorResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("response is not valid JSON: %v (body: %s)", err, rec.Body.String())
			}
			if body.Error.Code != CodeNotImplemented {
				t.Errorf("error.code = %q, want %q", body.Error.Code, CodeNotImplemented)
			}
			if body.Error.Message == "" {
				t.Error("error.message is empty, want an explanation of what is missing")
			}
		})
	}
}

func TestUnknownRouteReturnsJSON404(t *testing.T) {
	rec := do(t, http.MethodGet, "/api/v1/nope", nil)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}

	var body errorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not valid JSON: %v (body: %s)", err, rec.Body.String())
	}
	if body.Error.Code != CodeNotFound {
		t.Errorf("error.code = %q, want %q", body.Error.Code, CodeNotFound)
	}
}

func TestMethodNotAllowedReturnsJSON405(t *testing.T) {
	rec := do(t, http.MethodDelete, "/health", nil)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

func TestCORSAllowsConfiguredOriginOnly(t *testing.T) {
	allowed := do(t, http.MethodGet, "/health", map[string]string{"Origin": "http://localhost:5173"})
	if got := allowed.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Errorf("Access-Control-Allow-Origin = %q, want the request origin", got)
	}

	denied := do(t, http.MethodGet, "/health", map[string]string{"Origin": "https://evil.example"})
	if got := denied.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin = %q for a disallowed origin, want it unset", got)
	}
}

func TestCORSPreflight(t *testing.T) {
	rec := do(t, http.MethodOptions, "/api/v1/transactions", map[string]string{
		"Origin":                        "http://localhost:5173",
		"Access-Control-Request-Method": http.MethodGet,
	})

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if rec.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("Access-Control-Allow-Methods is unset on a preflight response")
	}
}
