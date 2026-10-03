package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

type Problem struct {
	Type   string            `json:"type"`
	Title  string            `json:"title"`
	Status int               `json:"status"`
	Detail string            `json:"detail,omitempty"`
	Errors map[string]string `json:"errors,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Error("write response", "error", err)
	}
}

func writeProblem(w http.ResponseWriter, status int, title, detail string, fields map[string]string) {
	writeJSON(w, status, Problem{
		Type:   "https://petfinder.local/problems/" + strings.ReplaceAll(strings.ToLower(http.StatusText(status)), " ", "-"),
		Title:  title,
		Status: status,
		Detail: detail,
		Errors: fields,
	})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request body", err.Error(), nil)
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(w, http.StatusBadRequest, "Invalid request body", "the body must contain one JSON object", nil)
		return false
	}
	return true
}

var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrForbidden = errors.New("forbidden")
	ErrInvalid   = errors.New("invalid")
)

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Resource not found", "the requested resource does not exist", nil)
	case errors.Is(err, ErrConflict):
		writeProblem(w, http.StatusConflict, "Conflict", "the request conflicts with the current resource state", nil)
	case errors.Is(err, ErrForbidden):
		writeProblem(w, http.StatusForbidden, "Forbidden", "you are not allowed to perform this action", nil)
	case errors.Is(err, ErrInvalid):
		writeProblem(w, http.StatusUnprocessableEntity, "Invalid operation", "the requested operation is not valid", nil)
	default:
		slog.Error("request failed", "error", err)
		writeProblem(w, http.StatusInternalServerError, "Internal server error", "an unexpected error occurred", nil)
	}
}
