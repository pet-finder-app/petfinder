//go:build e2e

package e2e

import (
	"bytes"
	"net/http"
	"testing"
)

type uploaded struct {
	ID        string
	UploadURL string `json:"upload_url"`
	URL       string
}

func uploadPNG(t *testing.T, u user, purpose string) uploaded {
	t.Helper()
	data := []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 0}
	var media uploaded
	u.do(http.MethodPost, "/v1/media", map[string]any{"filename": "image.png", "content_type": "image/png", "byte_size": len(data), "purpose": purpose}, http.StatusCreated, &media)
	req, err := http.NewRequest(http.MethodPut, baseURL+media.UploadURL, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+u.AccessToken)
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("upload status %d", res.StatusCode)
	}
	return media
}

// Saving an unchanged profile keeps verification; only real changes reset it.
func TestUnchangedProfilesKeepVerification(t *testing.T) {
	owner, admin := registerUser(t), registerAdmin(t)
	org := createOrganization(t, owner, "Cidade "+unique())
	private := map[string]any{"representative_name": "Pessoa Responsável"}
	owner.do(http.MethodPatch, "/v1/organizations/"+org.ID, map[string]any{"private_profile": private}, http.StatusOK, nil)
	verifyOrganization(t, admin, org.ID)
	owner.do(http.MethodPatch, "/v1/organizations/"+org.ID, map[string]any{"private_profile": private}, http.StatusOK, nil)
	var current organization
	owner.do(http.MethodGet, "/v1/organizations/"+org.ID, nil, http.StatusOK, &current)
	if current.VerificationStatus != "verified" {
		t.Fatalf("unchanged private profile reset status to %q", current.VerificationStatus)
	}
	private["representative_name"] = "Outra Pessoa"
	owner.do(http.MethodPatch, "/v1/organizations/"+org.ID, map[string]any{"private_profile": private}, http.StatusOK, nil)
	owner.do(http.MethodGet, "/v1/organizations/"+org.ID, nil, http.StatusOK, &current)
	if current.VerificationStatus != "pending" {
		t.Fatalf("changed private profile left status %q", current.VerificationStatus)
	}

	u := registerUser(t)
	profile := map[string]any{"occupation": "Professora"}
	u.do(http.MethodPatch, "/v1/me", map[string]any{"profile": profile}, http.StatusOK, nil)
	var v struct{ ID string }
	u.do(http.MethodPost, "/v1/me/verification", map[string]any{"documents": []string{"https://example.com/rg.pdf"}}, http.StatusCreated, &v)
	admin.do(http.MethodPost, "/v1/verifications/"+v.ID+"/decision", map[string]any{"decision": "approved", "reason": "Identidade conferida"}, http.StatusOK, nil)
	var me struct {
		VerificationStatus string `json:"verification_status"`
	}
	u.do(http.MethodPatch, "/v1/me", map[string]any{"profile": profile}, http.StatusOK, &me)
	if me.VerificationStatus != "verified" {
		t.Fatalf("unchanged personal profile reset status to %q", me.VerificationStatus)
	}
}
