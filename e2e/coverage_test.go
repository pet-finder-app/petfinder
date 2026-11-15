//go:build e2e

package e2e

import (
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
