//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

type report struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

func TestReportModeration(t *testing.T) {
	s := newAdoptionSetup(t)
	admin, outsider := registerAdmin(t), registerUser(t)

	var created report
	s.adopter.do(http.MethodPost, "/v1/reports", map[string]any{
		"category": "payment_request", "description": "Pediram uma taxa antes da entrega.",
		"subject_type": "animal", "subject_id": s.animal.ID,
	}, http.StatusCreated, &created)
	if created.Status != "open" {
		t.Fatalf("new report status = %q", created.Status)
	}
	reportPath := "/v1/reports/" + created.ID
	s.adopter.do(http.MethodGet, reportPath, nil, http.StatusOK, nil)
	outsider.do(http.MethodGet, reportPath, nil, http.StatusForbidden, nil)
	s.adopter.do(http.MethodGet, "/v1/reports", nil, http.StatusForbidden, nil)

	var queue struct {
		Items []report `json:"items"`
	}
	admin.do(http.MethodGet, "/v1/reports?page_size=100", nil, http.StatusOK, &queue)
	if !containsReport(queue.Items, created.ID) {
		t.Fatalf("open report %s missing from the admin queue", created.ID)
	}

	s.adopter.do(http.MethodPatch, reportPath, map[string]any{"status": "investigating"}, http.StatusForbidden, nil)
	admin.do(http.MethodPatch, reportPath, map[string]any{"status": "resolved"}, http.StatusUnprocessableEntity, nil)
	var resolved report
	admin.do(http.MethodPatch, reportPath, map[string]any{"status": "resolved", "resolution": "Anúncio removido e organização advertida."}, http.StatusOK, &resolved)
	if resolved.Status != "resolved" {
		t.Fatalf("status = %q after resolving", resolved.Status)
	}
	admin.do(http.MethodGet, "/v1/reports?page_size=100", nil, http.StatusOK, &queue)
	if containsReport(queue.Items, created.ID) {
		t.Fatalf("resolved report %s still in the open queue", created.ID)
	}
}

func containsReport(items []report, id string) bool {
	for _, item := range items {
		if item.ID == id {
			return true
		}
	}
	return false
}
