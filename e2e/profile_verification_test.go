//go:build e2e

package e2e

import "testing"

func TestProfileAndVerification(t *testing.T) {
	u, stranger, admin := registerUser(t), registerUser(t), registerAdmin(t)
	profile := map[string]any{"cpf": "52998224725", "birth_date": "1990-05-12", "occupation": "Teacher", "biography": "I care for animals", "housing": "House", "social_links": []string{"https://example.com/me"}, "documents": []string{"https://example.com/id.pdf"}, "address": map[string]any{"postal_code": "50000-000", "street": "Main Street"}}
	var result struct {
		Profile            struct{ CPF, Occupation string }
		VerificationStatus string `json:"verification_status"`
	}
	u.do("PATCH", "/v1/me", map[string]any{"profile": profile}, 200, &result)
	if result.Profile.CPF != "52998224725" || result.Profile.Occupation != "Teacher" {
		t.Fatal("profile fields were not persisted")
	}
	u.do("PATCH", "/v1/me", map[string]any{"profile": map[string]any{"cpf": "11111111111"}}, 422, nil)
	evidence := map[string]any{"documents": []string{"https://example.com/id.pdf"}, "notes": "Evidence for review"}
	var v struct{ ID, Status string }
	u.do("POST", "/v1/me/verification", evidence, 201, &v)
	u.do("POST", "/v1/me/verification", evidence, 409, nil)
	u.do("GET", "/v1/me/verification/requirements", nil, 200, nil)
	stranger.do("GET", "/v1/verifications", nil, 403, nil)
	admin.do("GET", "/v1/verifications", nil, 200, nil)
	admin.do("GET", "/v1/verifications/"+v.ID, nil, 200, nil)
	stranger.do("POST", "/v1/verifications/"+v.ID+"/decision", map[string]any{"decision": "approved", "reason": "Identity checked"}, 403, nil)
	admin.do("POST", "/v1/verifications/"+v.ID+"/decision", map[string]any{"decision": "under_review", "reason": "Checking documents"}, 200, nil)
	admin.do("POST", "/v1/verifications/"+v.ID+"/decision", map[string]any{"decision": "approved", "reason": "Identity checked"}, 200, nil)
	u.do("GET", "/v1/me", nil, 200, &result)
	if result.VerificationStatus != "verified" {
		t.Fatal("verification did not update profile")
	}
	org := createOrganization(t, u, "City "+unique())
	u.do("PATCH", "/v1/organizations/"+org.ID, map[string]any{"slug": "shelter-" + unique(), "profile": map[string]any{"type": "shelter", "capacity": 50}, "private_profile": map[string]any{"representative_name": "Test Person", "documents": []string{"https://example.com/registration.pdf"}}}, 200, nil)
	u.do("POST", "/v1/organizations/"+org.ID+"/verification-submissions", evidence, 201, &v)
	stranger.do("GET", "/v1/organizations/"+org.ID+"/verification-submissions", nil, 403, nil)
	admin.do("POST", "/v1/verifications/"+v.ID+"/decision", map[string]any{"decision": "approved", "reason": "Registration and representative checked"}, 200, nil)
	var public map[string]any
	client{t: t}.do("GET", "/v1/organizations/"+org.ID, nil, 200, &public)
	if _, exists := public["private_profile"]; exists {
		t.Fatal("private organization profile leaked")
	}
}

func TestVerificationCannotApproveChangedIdentity(t *testing.T) {
	u, admin := registerUser(t), registerAdmin(t)
	var v struct{ ID string }
	u.do("POST", "/v1/me/verification", map[string]any{"documents": []string{"https://example.com/id.pdf"}}, 201, &v)
	u.do("PATCH", "/v1/me", map[string]any{"profile": map[string]any{"occupation": "Changed profile"}}, 200, nil)
	admin.do("POST", "/v1/verifications/"+v.ID+"/decision", map[string]any{"decision": "approved", "reason": "Attempt to approve stale evidence"}, 409, nil)
	admin.do("POST", "/v1/verifications/"+v.ID+"/decision", map[string]any{"decision": "rejected", "reason": "Please submit current identity evidence"}, 200, nil)
}
