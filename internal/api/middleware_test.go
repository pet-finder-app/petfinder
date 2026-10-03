package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCommonMiddleware(t *testing.T) {
	h := commonMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Fatalf("got status %d", w.Code)
	}
	if w.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing request id")
	}
	if w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing security header")
	}
}

func TestCommonMiddlewareRecoversPanics(t *testing.T) {
	h := commonMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("got status %d", w.Code)
	}
}
