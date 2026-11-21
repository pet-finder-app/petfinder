//go:build e2e

package e2e

import (
	"net/http"
	"testing"
	"time"
)

// Follow-ups can be rescheduled and cancelled by the organization only, and a
// cancelled follow-up accepts no reports.
func TestFollowupRescheduleAndCancel(t *testing.T) {
	s := newAdoptionSetup(t)
	completeAdoption(t, s)
	var task struct {
		ID, Status         string
		DueAt              time.Time `json:"due_at"`
		CancellationReason *string   `json:"cancellation_reason"`
	}
	s.owner.do(http.MethodPost, "/v1/applications/"+s.applicationID+"/follow-ups", map[string]any{"due_at": time.Now().Add(72 * time.Hour), "instructions": "Envie fotos do animal"}, http.StatusCreated, &task)
	path := "/v1/follow-ups/" + task.ID
	later := time.Now().Add(240 * time.Hour).UTC().Truncate(time.Second)
	s.adopter.do(http.MethodPatch, path, map[string]any{"due_at": later}, http.StatusForbidden, nil)
	s.owner.do(http.MethodPatch, path, map[string]any{"due_at": time.Now().Add(-time.Hour)}, http.StatusUnprocessableEntity, nil)
	s.owner.do(http.MethodPatch, path, map[string]any{"due_at": later, "instructions": "Envie fotos e conte como está a adaptação"}, http.StatusOK, &task)
	if !task.DueAt.Equal(later) {
		t.Fatalf("due_at = %v, want %v", task.DueAt, later)
	}
	s.adopter.do(http.MethodPost, path+"/cancellation", map[string]any{"reason": "Não quero mais"}, http.StatusForbidden, nil)
	s.owner.do(http.MethodPost, path+"/cancellation", map[string]any{"reason": "Acompanhamento feito por visita presencial"}, http.StatusOK, &task)
	if task.Status != "cancelled" || task.CancellationReason == nil {
		t.Fatalf("follow-up after cancellation = %+v", task)
	}
	s.adopter.do(http.MethodPost, path+"/reports", map[string]any{"notes": "O animal está ótimo e adaptado à casa", "welfare_status": "good"}, http.StatusConflict, nil)
	s.owner.do(http.MethodPatch, path, map[string]any{"instructions": "Tarde demais para mudar"}, http.StatusConflict, nil)
	s.owner.do(http.MethodPost, path+"/cancellation", map[string]any{"reason": "Cancelar de novo"}, http.StatusConflict, nil)
}

// Completed adoptions may still record an agreement, and whoever handles
// applications can publish the templates they use.
func TestAgreementAfterCompletionAndTemplatePermissions(t *testing.T) {
	s := newAdoptionSetup(t)
	representative := registerUser(t)
	s.owner.do(http.MethodPost, "/v1/organizations/"+s.org.ID+"/members", map[string]any{"email": representative.User.Email, "role": "representative"}, http.StatusCreated, nil)
	representative.do(http.MethodPut, "/v1/me/organizations/"+s.org.ID+"/membership", map[string]any{"decision": "accepted"}, http.StatusOK, nil)
	var template struct{ ID string }
	representative.do(http.MethodPost, "/v1/organizations/"+s.org.ID+"/agreement-templates", map[string]any{"kind": "adoption", "body": "{{user_name}} adota {{animal_name}} junto à {{organization_name}} em {{date}}."}, http.StatusCreated, &template)
	completeAdoption(t, s)
	var agreement struct{ ID string }
	s.owner.do(http.MethodPost, "/v1/applications/"+s.applicationID+"/agreement", map[string]any{"template_id": template.ID, "representative_id": s.owner.User.ID}, http.StatusCreated, &agreement)
	if agreement.ID == "" {
		t.Fatal("agreement not created after completion")
	}
}

// Responses carry the display names clients show, and notifications carry
// enough context to link to their case.
func TestDisplayNamesAndNotificationContext(t *testing.T) {
	s := newAdoptionSetup(t)
	var post struct {
		OrganizationName string `json:"organization_name"`
	}
	s.owner.do(http.MethodPost, "/v1/organizations/"+s.org.ID+"/posts", map[string]any{"body": "Feira de adoção"}, http.StatusCreated, &post)
	if post.OrganizationName != s.org.Name {
		t.Fatalf("post organization_name = %q", post.OrganizationName)
	}
	s.owner.do(http.MethodPut, "/v1/organizations/"+s.org.ID+"/payment-details", map[string]any{"method": "pix", "instructions": "Doe pelo PIX", "pix_key": "doe@example.com", "account_holder": "ONG", "published": true}, http.StatusOK, nil)
	var donation struct {
		ID               string
		OrganizationName string `json:"organization_name"`
	}
	s.adopter.do(http.MethodPost, "/v1/organizations/"+s.org.ID+"/donations", map[string]any{"amount_cents": 1000, "transferred_at": time.Now().Add(-time.Minute), "anonymous": false}, http.StatusCreated, &donation)
	if donation.OrganizationName != s.org.Name {
		t.Fatalf("donation organization_name = %q", donation.OrganizationName)
	}

	var notifications struct {
		Items []struct {
			Kind          string  `json:"kind"`
			SubjectID     string  `json:"subject_id"`
			ApplicationID *string `json:"application_id"`
			Audience      string  `json:"audience"`
			ReadAt        *string `json:"read_at"`
		} `json:"items"`
	}
	s.owner.do(http.MethodGet, "/v1/notifications?page_size=100", nil, http.StatusOK, &notifications)
	sawApplication, sawDonation := false, false
	for _, n := range notifications.Items {
		if n.Kind == "application" {
			sawApplication = n.ApplicationID != nil && *n.ApplicationID == s.applicationID && n.Audience == "organization"
		}
		if n.Kind == "donation" {
			sawDonation = n.Audience == "organization"
		}
	}
	if !sawApplication || !sawDonation {
		t.Fatalf("organization notifications lack context: %+v", notifications.Items)
	}

	var sent struct {
		Message struct{ ID string } `json:"message"`
	}
	s.owner.do(http.MethodPost, "/v1/conversations/"+s.conversationID+"/messages", map[string]any{"body": "Olá! Vamos conversar."}, http.StatusCreated, &sent)
	unreadMessages := func() int {
		s.adopter.do(http.MethodGet, "/v1/notifications?page_size=100", nil, http.StatusOK, &notifications)
		count := 0
		for _, n := range notifications.Items {
			if n.Kind == "message" && n.SubjectID == s.conversationID && n.ReadAt == nil {
				count++
			}
		}
		return count
	}
	if unreadMessages() == 0 {
		t.Fatal("message notification missing")
	}
	s.adopter.do(http.MethodPut, "/v1/conversations/"+s.conversationID+"/read-state", map[string]any{"message_id": sent.Message.ID}, http.StatusOK, nil)
	if got := unreadMessages(); got != 0 {
		t.Fatalf("%d message notifications still unread after reading the conversation", got)
	}
}

// Surrender responses name both parties and the actor of each history entry.
func TestSurrenderDisplayNames(t *testing.T) {
	s := newAdoptionSetup(t)
	completeAdoption(t, s)
	var surrender struct {
		RequesterName    string `json:"requester_name"`
		OrganizationName string `json:"organization_name"`
		History          []struct {
			ActorName string `json:"actor_name"`
		} `json:"history"`
	}
	s.adopter.do(http.MethodPost, "/v1/surrenders", map[string]any{"organization_id": s.org.ID, "kind": "return", "application_id": s.applicationID, "reason": "Mudança de cidade impede continuar os cuidados"}, http.StatusCreated, &surrender)
	if surrender.RequesterName == "" || surrender.OrganizationName != s.org.Name {
		t.Fatalf("surrender names = %q / %q", surrender.RequesterName, surrender.OrganizationName)
	}
	for _, h := range surrender.History {
		if h.ActorName == "" {
			t.Fatal("history entry without actor_name")
		}
	}
}
