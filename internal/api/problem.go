package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"
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
	// Validate the original JSON so presence, enum, range and length constraints
	// match the generated schema. The strict decoder below still rejects unknown
	// fields and trailing content, including when used without a route contract.
	if contract, ok := r.Context().Value(requestContractKey{}).(*requestContract); ok {
		if reflect.TypeOf(dst) != reflect.PointerTo(contract.typeOf) {
			writeProblem(w, http.StatusInternalServerError, "Internal server error", "request contract mismatch", nil)
			return false
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, "Invalid request body", "unable to read request body", nil)
			return false
		}
		var value any
		if err := json.Unmarshal(body, &value); err != nil {
			writeProblem(w, http.StatusBadRequest, "Invalid request body", err.Error(), nil)
			return false
		}
		result := &huma.ValidateResult{}
		huma.Validate(contract.registry, contract.schema, huma.NewPathBuffer(nil, 0), huma.ModeWriteToServer, value, result)
		if len(result.Errors) > 0 {
			fields := map[string]string{}
			for _, err := range result.Errors {
				if detail, ok := err.(*huma.ErrorDetail); ok {
					fields[detail.Location] = detail.Message
				} else {
					fields["body"] = err.Error()
				}
			}
			writeProblem(w, http.StatusUnprocessableEntity, "Validation failed", "review the invalid fields", fields)
			return false
		}
		r.Body = io.NopCloser(strings.NewReader(string(body)))
	}
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
