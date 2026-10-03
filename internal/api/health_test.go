package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type pingStub struct{ err error }

func (p pingStub) Ping(context.Context) error { return p.err }

func TestHealthHandler(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"ready", nil, http.StatusOK},
		{"database unavailable", errors.New("down"), http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			healthHandler(pingStub{err: tt.err}).ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/health", nil))
			if w.Code != tt.want {
				t.Fatalf("got %d, want %d", w.Code, tt.want)
			}
		})
	}
}
