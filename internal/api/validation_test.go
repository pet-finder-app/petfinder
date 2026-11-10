package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		valid    bool
	}{
		{"strong", "correct horse 42", true},
		{"too short", "horse42", false},
		{"no number", "correct horse battery", false},
		{"bcrypt limit", strings.Repeat("a", 71) + "1", true},
		{"over bcrypt limit", strings.Repeat("a", 72) + "1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fieldErrors{}
			validPassword(err, tt.password)
			if err.empty() != tt.valid {
				t.Fatalf("valid=%v, errors=%v", tt.valid, err)
			}
		})
	}
}

func TestDecodeJSONRejectsTrailingContent(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"valid"} trailing`))
	w := httptest.NewRecorder()
	var body struct {
		Name string `json:"name"`
	}
	if decodeJSON(w, r, &body) {
		t.Fatal("expected trailing content to be rejected")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("got status %d", w.Code)
	}
}

func TestValidEmailNormalizes(t *testing.T) {
	err := fieldErrors{}
	got := validEmail(err, "  PERSON@example.com ")
	if !err.empty() {
		t.Fatalf("unexpected errors: %v", err)
	}
	if got != "person@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestValidImageURLRejectsNonHTTP(t *testing.T) {
	err := fieldErrors{}
	validImageURL(err, "url", "javascript:alert(1)")
	if err.empty() {
		t.Fatal("expected validation error")
	}
}
