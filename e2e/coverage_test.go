//go:build e2e

package e2e

import (
	"testing"
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
