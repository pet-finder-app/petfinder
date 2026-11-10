//go:build e2e

package e2e

import (
	"net/http"
	"net/url"
	"testing"
)

type organizationPage struct {
	Items []organization `json:"items"`
}

func TestOrganizationVerification(t *testing.T) {
	owner, outsider, anonymous := registerUser(t), registerUser(t), client{t: t}
	city := "Cidade " + unique()
	listPath := "/v1/organizations?city=" + url.QueryEscape(city)

	org := createOrganization(t, owner, city)
	if org.VerificationStatus != "pending" || org.LegalName == nil {
		t.Fatalf("new organization %+v should be pending and show private fields to its owner", org)
	}
	anonymous.do(http.MethodGet, "/v1/organizations/"+org.ID, nil, http.StatusNotFound, nil)
	owner.do(http.MethodGet, "/v1/organizations/"+org.ID, nil, http.StatusOK, nil)
	var listed organizationPage
	anonymous.do(http.MethodGet, listPath, nil, http.StatusOK, &listed)
	if len(listed.Items) != 0 {
		t.Fatalf("pending organization listed: %+v", listed.Items)
	}

	outsider.do(http.MethodPost, "/v1/organizations/"+org.ID+"/verification", map[string]any{
		"decision": "verified", "reason": "tentativa sem ser administrador",
	}, http.StatusForbidden, nil)
	verifyOrganization(t, registerAdmin(t), org.ID)

	var public organization
	anonymous.do(http.MethodGet, "/v1/organizations/"+org.ID, nil, http.StatusOK, &public)
	if public.VerificationStatus != "verified" || public.LegalName != nil {
		t.Fatalf("public view %+v should be verified without private fields", public)
	}
	anonymous.do(http.MethodGet, listPath, nil, http.StatusOK, &listed)
	if len(listed.Items) != 1 || listed.Items[0].ID != org.ID {
		t.Fatalf("verified organization not listed: %+v", listed.Items)
	}
}

func TestOrganizationMembers(t *testing.T) {
	owner, invitee, outsider := registerUser(t), registerUser(t), registerUser(t)
	org := createOrganization(t, owner, "Cidade "+unique())
	membersPath := "/v1/organizations/" + org.ID + "/members"

	outsider.do(http.MethodPost, membersPath, map[string]any{"email": invitee.User.Email, "role": "representative"}, http.StatusForbidden, nil)
	owner.do(http.MethodPost, membersPath, map[string]any{"email": invitee.User.Email, "role": "representative"}, http.StatusCreated, nil)
	owner.do(http.MethodPost, membersPath, map[string]any{"email": invitee.User.Email, "role": "representative"}, http.StatusConflict, nil)

	var members struct {
		Items []struct {
			UserID     string `json:"user_id"`
			MemberRole string `json:"member_role"`
			Status     string `json:"status"`
		} `json:"items"`
	}
	owner.do(http.MethodGet, membersPath, nil, http.StatusOK, &members)
	if len(members.Items) != 2 {
		t.Fatalf("members = %+v, want the owner and the invitee", members.Items)
	}
	outsider.do(http.MethodGet, membersPath, nil, http.StatusForbidden, nil)

	owner.do(http.MethodDelete, membersPath+"/"+owner.User.ID, nil, http.StatusUnprocessableEntity, nil)
	owner.do(http.MethodDelete, membersPath+"/"+invitee.User.ID, nil, http.StatusNoContent, nil)
}
