package api

import "net/http"

// handleHealth reports that the process is up and serving.
//
// It intentionally does not check PostgreSQL: this is a liveness probe, and a
// dependency check belongs in a separate readiness endpoint so a transient
// database blip cannot get a healthy process restarted.
//
// TODO(phase-8): add GET /ready that pings the database pool.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// notImplemented builds a handler for a route whose contract exists but whose
// implementation lands in a later phase. Returning 501 keeps the API honest: a
// caller can tell "not built yet" apart from "built and returned no data".
func notImplemented(phase string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotImplemented, CodeNotImplemented,
			"this endpoint is not implemented yet; planned for "+phase)
	}
}
