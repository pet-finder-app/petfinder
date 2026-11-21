//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestMembershipAndInventoryDiscovery(t *testing.T) {
	owner, stranger := registerUser(t), registerUser(t)
	org := createOrganization(t, owner, "Cidade "+unique())
	var memberships struct {
		Items []struct {
			Organization organization `json:"organization"`
			Status       string       `json:"membership_status"`
		} `json:"items"`
	}
	owner.do("GET", "/v1/me/organizations", nil, 200, &memberships)
	if len(memberships.Items) != 1 || memberships.Items[0].Organization.ID != org.ID || memberships.Items[0].Organization.VerificationStatus != "pending" {
		t.Fatalf("pending membership missing: %+v", memberships)
	}
	stranger.do("GET", "/v1/me/organizations", nil, 200, &memberships)
	if len(memberships.Items) != 0 {
		t.Fatal("memberships leaked to stranger")
	}
	owner.do("POST", "/v1/organizations/"+org.ID+"/members", map[string]any{"email": stranger.User.Email, "role": "representative"}, 201, nil)
	stranger.do("GET", "/v1/me/organizations", nil, 200, &memberships)
	if len(memberships.Items) != 1 {
		t.Fatal("invited organization missing")
	}
	verifyOrganization(t, registerAdmin(t), org.ID)
	var a animal
	owner.do("POST", "/v1/animals", animalRequest(org.ID, "Cidade "+unique()), 201, &a)
	owner.do("PATCH", "/v1/animals/"+a.ID+"/status", map[string]any{"status": "unavailable", "reason": "Temporary shelter closure"}, 200, nil)
	path := "/v1/organizations/" + org.ID + "/animals"
	var inventory animalPage
	owner.do("GET", path+"?status=unavailable", nil, 200, &inventory)
	if len(inventory.Items) != 1 || inventory.Items[0].ID != a.ID {
		t.Fatalf("inventory missing animal: %+v", inventory)
	}
	owner.do("GET", path+"?status=available", nil, 200, &inventory)
	if len(inventory.Items) != 0 {
		t.Fatal("inventory status filter ignored")
	}
	owner.do("GET", path+"?status=invalid", nil, 422, nil)
	// Invited users have no inventory access until membership is active.
	stranger.do("GET", path, nil, 403, nil)
	owner.do("GET", "/v1/me/animals", nil, 200, &inventory)
	if len(inventory.Items) != 1 {
		t.Fatal("submitter inventory missing animal")
	}
	stranger.do("GET", "/v1/me/animals", nil, 200, &inventory)
	if len(inventory.Items) != 0 {
		t.Fatal("submitter inventory leaked")
	}
	client{t: t}.do("GET", "/v1/me/organizations", nil, 401, nil)
}

func TestClearPreferenceRestoresDiscovery(t *testing.T) {
	s := newAdoptionSetup(t)
	path := "/v1/animals/" + s.animal.ID + "/preference"
	s.adopter.do("PUT", path, map[string]any{"value": "not_interested"}, 200, nil)
	if len(discover(s.adopter.client, s.city)) != 0 {
		t.Fatal("dismissal ignored")
	}
	s.owner.do("DELETE", path, nil, 204, nil)
	if len(discover(s.adopter.client, s.city)) != 0 {
		t.Fatal("other user cleared dismissal")
	}
	s.adopter.do("DELETE", path, nil, 204, nil)
	s.adopter.do("DELETE", path, nil, 204, nil)
	if len(discover(s.adopter.client, s.city)) != 1 {
		t.Fatal("clear did not restore discovery")
	}
	s.adopter.do("GET", "/v1/preferences", nil, 200, &struct {
		Items []animal `json:"items"`
	}{})
}

func TestApplicationWithdrawalAndConversationLookup(t *testing.T) {
	s := newAdoptionSetup(t)
	stranger := registerUser(t)
	path := "/v1/applications/" + s.applicationID
	var detail struct {
		Conversations []struct{ ID string } `json:"conversations"`
		History       []struct {
			ToStatus string `json:"to_status"`
		} `json:"history"`
	}
	s.adopter.do("GET", path, nil, 200, &detail)
	if len(detail.Conversations) != 1 || detail.Conversations[0].ID != s.conversationID {
		t.Fatalf("conversation reference missing: %+v", detail)
	}
	stranger.do("GET", path, nil, 403, nil)
	request := map[string]any{"reason": "My circumstances changed"}
	s.owner.do("POST", path+"/withdrawal", request, 403, nil)
	s.adopter.do("POST", path+"/withdrawal", map[string]any{"reason": " "}, 422, nil)
	s.adopter.do("POST", path+"/withdrawal", request, 200, nil)
	s.adopter.do("POST", path+"/withdrawal", request, 409, nil)
	s.adopter.do("PUT", path+"/screening", map[string]any{"housing": "House", "household": "Two adults", "experience": "Previous pets", "care_plan": "Regular veterinary care and daily walks"}, 409, nil)
	s.owner.do("POST", path+"/decision", map[string]any{"decision": "approved", "reason": "Suitable adopter"}, 409, nil)
	s.adopter.do("GET", path, nil, 200, &detail)
	if detail.History[len(detail.History)-1].ToStatus != "withdrawn" {
		t.Fatal("withdrawal history missing")
	}
	s.adopter.do("POST", "/v1/conversations/"+s.conversationID+"/messages", map[string]any{"body": "message"}, 403, nil)
	approved := newAdoptionSetup(t)
	approved.owner.do("POST", "/v1/applications/"+approved.applicationID+"/decision", map[string]any{"decision": "approved", "reason": "Suitable adopter"}, 200, nil)
	approved.adopter.do("POST", "/v1/applications/"+approved.applicationID+"/withdrawal", request, 409, nil)
}

func TestAppointmentLookup(t *testing.T) {
	s := newAdoptionSetup(t)
	stranger := registerUser(t)
	path := "/v1/applications/" + s.applicationID
	s.owner.do("POST", path+"/decision", map[string]any{"decision": "approved", "reason": "Suitable adopter"}, 200, nil)
	start := time.Now().Add(48 * time.Hour).UTC()
	var appointment struct{ ID string }
	s.owner.do("POST", path+"/appointments", map[string]any{"purpose": "delivery", "starts_at": start, "ends_at": start.Add(time.Hour), "timezone": "America/Recife", "location_name": "Shelter", "address": "Main Street 123"}, 201, &appointment)
	var list struct {
		Items []struct{ ID string } `json:"items"`
	}
	s.adopter.do("GET", path+"/appointments", nil, 200, &list)
	if len(list.Items) != 1 || list.Items[0].ID != appointment.ID {
		t.Fatal("appointment listing missing appointment")
	}
	var detail struct {
		Appointment   struct{ ID string }
		Confirmations []any
	}
	s.adopter.do("GET", "/v1/appointments/"+appointment.ID, nil, 200, &detail)
	if detail.Appointment.ID != appointment.ID || detail.Confirmations == nil {
		t.Fatal("invalid appointment detail")
	}
	stranger.do("GET", path+"/appointments", nil, 403, nil)
	stranger.do("GET", "/v1/appointments/"+appointment.ID, nil, 403, nil)
}

func TestMessageCursorWithEqualTimestamps(t *testing.T) {
	s := newAdoptionSetup(t)
	path := "/v1/conversations/" + s.conversationID + "/messages"
	for _, body := range []string{"hello one", "hello two", "hello three"} {
		s.adopter.do("POST", path, map[string]any{"body": body}, 201, nil)
	}
	// Deliberately force ties to exercise the UUID portion of the cursor.
	if _, err := pool.Exec(context.Background(), "UPDATE messages SET created_at = now() WHERE conversation_id = $1", s.conversationID); err != nil {
		t.Fatal(err)
	}
	var page struct {
		Items []struct{ ID string } `json:"items"`
	}
	s.adopter.do("GET", path+"?page_size=2", nil, 200, &page)
	if len(page.Items) != 2 {
		t.Fatal("invalid first page")
	}
	first, second := page.Items[0].ID, page.Items[1].ID
	s.adopter.do("GET", path+"?page_size=2&before="+second, nil, 200, &page)
	if len(page.Items) != 1 || page.Items[0].ID == first || page.Items[0].ID == second {
		t.Fatalf("invalid cursor page: %+v", page)
	}
	s.adopter.do("GET", path+"?before=bad", nil, http.StatusBadRequest, nil)
	registerUser(t).do("GET", path, nil, 403, nil)
	other := newAdoptionSetup(t)
	var message struct {
		Message struct{ ID string } `json:"message"`
	}
	other.adopter.do("POST", "/v1/conversations/"+other.conversationID+"/messages", map[string]any{"body": "hello"}, 201, &message)
	s.adopter.do("GET", path+"?before="+message.Message.ID, nil, 422, nil)
}

func TestAnimalImageModeration(t *testing.T) {
	s := newAdoptionSetup(t)
	stranger := registerUser(t)
	path := "/v1/animals/" + s.animal.ID
	var image struct{ ID string }
	s.owner.do("POST", path+"/images", map[string]any{"url": "https://example.com/" + unique() + ".jpg"}, 201, &image)
	var detail struct {
		Images []struct{ ID string } `json:"images"`
	}
	anonymous := client{t: t}
	anonymous.do("GET", path, nil, 200, &detail)
	if len(detail.Images) != 0 {
		t.Fatal("pending image publicly visible")
	}
	moderation := path + "/images/" + image.ID + "/moderation"
	stranger.do("POST", moderation, map[string]any{"decision": "approved"}, 403, nil)
	s.owner.do("POST", moderation, map[string]any{"decision": "invalid"}, 422, nil)
	s.owner.do("POST", moderation, map[string]any{"decision": "approved"}, 200, nil)
	anonymous.do("GET", path, nil, 200, &detail)
	if len(detail.Images) != 1 {
		t.Fatal("approved image missing")
	}
	s.owner.do("POST", moderation, map[string]any{"decision": "rejected"}, 200, nil)
	anonymous.do("GET", path, nil, 200, &detail)
	if len(detail.Images) != 0 {
		t.Fatal("rejected image publicly visible")
	}
}
