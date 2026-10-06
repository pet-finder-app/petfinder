package api

import (
	"context"
	"net/http"
	"time"
)

type pinger interface {
	Ping(context.Context) error
}

func healthHandler(db pinger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			writeProblem(w, http.StatusServiceUnavailable, "Service unavailable", "database is not ready", nil)
			return
		}
		writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
	}
}
