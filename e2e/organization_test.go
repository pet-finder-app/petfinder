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

// Invitations grant nothing until the invited person accepts them, and only
// they can respond. Removed members can be invited again.
func TestOrganizationInvitationLifecycle(t *testing.T) {
	owner, invitee, outsider := registerUser(t), registerUser(t), registerUser(t)
	org := createOrganization(t, owner, "Cidade "+unique())
	membersPath := "/v1/organizations/" + org.ID + "/members"
	respondPath := "/v1/me/organizations/" + org.ID + "/membership"

	owner.do(http.MethodPost, membersPath, map[string]any{"email": invitee.User.Email, "role": "coordinator"}, http.StatusCreated, nil)
	invitee.do(http.MethodGet, membersPath, nil, http.StatusForbidden, nil)
	outsider.do(http.MethodPut, respondPath, map[string]any{"decision": "accepted"}, http.StatusNotFound, nil)
	invitee.do(http.MethodPut, respondPath, map[string]any{"decision": "maybe"}, http.StatusUnprocessableEntity, nil)

	var member struct{ Status string }
	invitee.do(http.MethodPut, respondPath, map[string]any{"decision": "accepted"}, http.StatusOK, &member)
	if member.Status != "active" {
		t.Fatalf("membership status = %q, want active", member.Status)
	}
	invitee.do(http.MethodGet, membersPath, nil, http.StatusOK, nil)
	invitee.do(http.MethodPut, respondPath, map[string]any{"decision": "declined"}, http.StatusNotFound, nil)

	owner.do(http.MethodDelete, membersPath+"/"+invitee.User.ID, nil, http.StatusNoContent, nil)
	invitee.do(http.MethodGet, membersPath, nil, http.StatusForbidden, nil)
	owner.do(http.MethodPost, membersPath, map[string]any{"email": invitee.User.Email, "role": "moderator"}, http.StatusCreated, nil)
	invitee.do(http.MethodPut, respondPath, map[string]any{"decision": "declined"}, http.StatusOK, &member)
	if member.Status != "declined" {
		t.Fatalf("membership status = %q, want declined", member.Status)
	}
	invitee.do(http.MethodGet, membersPath, nil, http.StatusForbidden, nil)
}

// A suspended organization keeps read access to its records but cannot move
// adoptions forward or expose its animals until it is verified again.
func TestSuspendedOrganizationCannotOperate(t *testing.T) {
	s := newAdoptionSetup(t)
	admin, latecomer := registerAdmin(t), registerUser(t)
	latecomer.do(http.MethodPut, "/v1/animals/"+s.animal.ID+"/preference", map[string]any{"value": "liked"}, http.StatusOK, nil)
	admin.do(http.MethodPost, "/v1/organizations/"+s.org.ID+"/verification", map[string]any{"decision": "suspended", "reason": "Investigação de denúncia de fraude"}, http.StatusOK, nil)

	client{t: t}.do(http.MethodGet, "/v1/animals/"+s.animal.ID, nil, http.StatusNotFound, nil)
	s.owner.do(http.MethodGet, "/v1/animals/"+s.animal.ID, nil, http.StatusOK, nil)
	s.owner.do(http.MethodPost, "/v1/applications/"+s.applicationID+"/decision", map[string]any{"decision": "approved", "reason": "Aprovação durante a suspensão"}, http.StatusConflict, nil)
	latecomer.do(http.MethodPost, "/v1/applications", map[string]any{"animal_id": s.animal.ID, "introduction": "Gostaria muito de adotar este animal."}, http.StatusNotFound, nil)
	s.owner.do(http.MethodPatch, "/v1/animals/"+s.animal.ID+"/status", map[string]any{"status": "under_review", "reason": "Pausa durante a suspensão"}, http.StatusOK, nil)
	s.owner.do(http.MethodPatch, "/v1/animals/"+s.animal.ID+"/status", map[string]any{"status": "available", "reason": "Republicação indevida"}, http.StatusConflict, nil)

	verifyOrganization(t, admin, s.org.ID)
	s.owner.do(http.MethodPatch, "/v1/animals/"+s.animal.ID+"/status", map[string]any{"status": "available", "reason": "Republicação após verificação"}, http.StatusOK, nil)
	s.owner.do(http.MethodPost, "/v1/applications/"+s.applicationID+"/decision", map[string]any{"decision": "approved", "reason": "Aprovação após verificação"}, http.StatusOK, nil)
}

// A verified organization that renames itself or changes its PIX key must be
// verified again; first-time donation setup and other edits keep the status.
func TestIdentityChangesRequireNewVerification(t *testing.T) {
	owner, admin := registerUser(t), registerAdmin(t)
	org := createOrganization(t, owner, "Cidade "+unique())
	base := "/v1/organizations/" + org.ID
	status := func() string {
		var current organization
		owner.do(http.MethodGet, base, nil, http.StatusOK, &current)
		return current.VerificationStatus
	}
	verifyOrganization(t, admin, org.ID)

	owner.do(http.MethodPatch, base, map[string]any{"description": "Nova descrição da organização, sem trocar o nome."}, http.StatusOK, nil)
	owner.do(http.MethodPatch, base, map[string]any{"name": org.Name}, http.StatusOK, nil)
	pix := map[string]any{"method": "pix", "instructions": "Doe pelo PIX da ONG", "pix_key": "doacoes@example.com", "account_holder": "Associação Protetora", "published": true}
	owner.do(http.MethodPut, base+"/payment-details", pix, http.StatusOK, nil)
	pix["instructions"] = "Instruções revisadas"
	pix["pix_key"] = "  doacoes@example.com  "
	owner.do(http.MethodPut, base+"/payment-details", pix, http.StatusOK, nil)
	if got := status(); got != "verified" {
		t.Fatalf("status after unrelated edits = %q, want verified", got)
	}

	owner.do(http.MethodPatch, base, map[string]any{"name": "Outro Nome " + unique()}, http.StatusOK, nil)
	if got := status(); got != "pending" {
		t.Fatalf("status after rename = %q, want pending", got)
	}

	verifyOrganization(t, admin, org.ID)
	pix["pix_key"] = "outra-chave@example.com"
	owner.do(http.MethodPut, base+"/payment-details", pix, http.StatusOK, nil)
	if got := status(); got != "pending" {
		t.Fatalf("status after PIX change = %q, want pending", got)
	}

	// Evidence reviewed before a rename cannot approve the new name.
	var v struct{ ID string }
	owner.do(http.MethodPost, base+"/verification-submissions", map[string]any{"documents": []string{"https://example.com/registro.pdf"}}, http.StatusCreated, &v)
	owner.do(http.MethodPatch, base, map[string]any{"name": "Nome Durante Análise " + unique()}, http.StatusOK, nil)
	admin.do(http.MethodPost, "/v1/verifications/"+v.ID+"/decision", map[string]any{"decision": "approved", "reason": "Documentos de antes da troca"}, http.StatusConflict, nil)
}
