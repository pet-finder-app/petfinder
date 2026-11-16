//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

type signedDocument struct {
	ID         string
	SHA256     string
	Body       string
	Signatures []struct {
		SignerID string `json:"signer_id"`
	}
}

func emailCode(t *testing.T, to string) string {
	t.Helper()
	for _, part := range strings.Fields(testMail.message(to)) {
		if len(part) == 6 {
			digits := true
			for _, c := range part {
				if c < '0' || c > '9' {
					digits = false
				}
			}
			if digits {
				return part
			}
		}
	}
	t.Fatalf("email code missing for %s", to)
	return ""
}

func signDocument(t *testing.T, u user, doc signedDocument) {
	t.Helper()
	path := "/v1/agreements/" + doc.ID
	u.do("POST", path+"/signing-code", nil, 202, nil)
	code := emailCode(t, u.User.Email)
	u.do("POST", path+"/signatures", map[string]any{"code": code, "sha256": doc.SHA256, "accept": true}, 200, nil)
}

func completeAdoption(t *testing.T, s adoptionSetup) {
	t.Helper()
	application := "/v1/applications/" + s.applicationID
	s.owner.do("POST", application+"/decision", map[string]any{"decision": "approved", "reason": "Applicant approved after interview"}, 200, nil)
	start := time.Now().Add(48 * time.Hour)
	var appointment struct{ ID string }
	s.owner.do("POST", application+"/appointments", map[string]any{"purpose": "delivery", "starts_at": start, "ends_at": start.Add(time.Hour), "timezone": "America/Recife", "location_name": "Shelter", "address": "Main Street"}, 201, &appointment)
	path := "/v1/appointments/" + appointment.ID
	s.owner.do("PATCH", path, map[string]any{"status": "scheduled"}, 200, nil)
	var otp struct{ Code string }
	s.owner.do("POST", path+"/otp", nil, 201, &otp)
	s.owner.do("POST", path+"/confirmations", map[string]any{"type": "delivery"}, 200, nil)
	s.adopter.do("POST", path+"/confirmations", map[string]any{"type": "receipt", "otp_code": otp.Code}, 200, nil)
}

func executeSurrender(t *testing.T, owner, requester user, org organization, id string) {
	t.Helper()
	path := "/v1/surrenders/" + id
	registerUser(t).do("GET", path, nil, 403, nil)
	owner.do("POST", path+"/decision", map[string]any{"decision": "needs_information", "reason": "Please clarify the request"}, 200, nil)
	requester.do("PUT", path, map[string]any{"reason": "Changed circumstances prevent me from continuing to provide care for this animal"}, 200, nil)
	owner.do("POST", path+"/decision", map[string]any{"decision": "accepted", "reason": "Organization agrees to take responsibility"}, 200, nil)
	owner.do("POST", path+"/handover", map[string]any{"type": "receipt", "code": "000000"}, 409, nil)
	var template struct{ ID string }
	owner.do("POST", "/v1/organizations/"+org.ID+"/agreement-templates", map[string]any{"kind": "surrender", "body": "{{user_name}} transfers care of {{animal_name}} to {{organization_name}} on {{date}}."}, 201, &template)
	var doc signedDocument
	owner.do("POST", path+"/agreement", map[string]any{"template_id": template.ID, "representative_id": owner.User.ID}, 201, &doc)
	signDocument(t, requester, doc)
	signDocument(t, owner, doc)
	requester.do("POST", path+"/handover-code", nil, 202, nil)
	code := emailCode(t, requester.User.Email)
	requester.do("POST", path+"/handover", map[string]any{"type": "delivery"}, 200, nil)
	owner.do("POST", path+"/handover", map[string]any{"type": "receipt", "code": code}, 200, nil)
	owner.do("POST", path+"/handover", map[string]any{"type": "receipt", "code": code}, 409, nil)
}

func TestDonationsAndLegalVersions(t *testing.T) {
	s := newAdoptionSetup(t)
	stranger := registerUser(t)
	base := "/v1/organizations/" + s.org.ID
	payment := map[string]any{"method": "pix", "instructions": "Transfer directly to the organization", "pix_key": "donations@example.com", "account_holder": "Shelter Association", "published": true}
	stranger.do("PUT", base+"/payment-details", payment, 403, nil)
	s.owner.do("PUT", base+"/payment-details", payment, 200, nil)
	client{t: t}.do("GET", base+"/payment-details", nil, 200, nil)
	var donation struct {
		ID, Status string
		DonorID    *string `json:"donor_id"`
	}
	s.adopter.do("POST", base+"/donations", map[string]any{"amount_cents": 12500, "transferred_at": time.Now().Add(-time.Hour), "anonymous": true, "note": "Direct transfer"}, 201, &donation)
	if donation.Status != "reported" {
		t.Fatal("transfer was prematurely confirmed")
	}
	path := "/v1/donations/" + donation.ID
	stranger.do("GET", path, nil, 403, nil)
	s.adopter.do("POST", path+"/decision", map[string]any{"status": "confirmed", "reason": "Transfer found in bank statement"}, 403, nil)
	var detail struct {
		Donation struct {
			DonorID *string `json:"donor_id"`
			Status  string
		}
		History []any
	}
	s.owner.do("GET", path, nil, 200, &detail)
	if detail.Donation.DonorID != nil {
		t.Fatal("anonymous donor identity leaked")
	}
	s.owner.do("POST", path+"/decision", map[string]any{"status": "confirmed", "reason": "Transfer found in bank statement"}, 200, &detail)
	if detail.Donation.Status != "confirmed" || len(detail.History) != 1 {
		t.Fatal("donation decision not persisted")
	}
	s.owner.do("POST", path+"/decision", map[string]any{"status": "not_found", "reason": "Cannot decide twice"}, 409, nil)
	var totals struct {
		ConfirmedCents int64 `json:"confirmed_cents"`
	}
	s.owner.do("GET", base+"/donation-totals", nil, 200, &totals)
	if totals.ConfirmedCents != 12500 {
		t.Fatal("incorrect donation total")
	}
	stranger.do("GET", base+"/donation-totals", nil, 403, nil)
	var legal struct {
		ID, Content string
		Placeholder bool
	}
	client{t: t}.do("GET", "/v1/legal/terms", nil, 200, &legal)
	if !strings.Contains(legal.Content, "Lorem ipsum") || !legal.Placeholder {
		t.Fatal("requested placeholder legal text missing")
	}
	client{t: t}.do("GET", "/v1/legal/privacy/versions", nil, 200, nil)
	var acceptances struct{ Items []any }
	s.adopter.do("GET", "/v1/me/legal-acceptances", nil, 200, &acceptances)
	if len(acceptances.Items) != 2 {
		t.Fatal("registration legal versions missing")
	}
	s.adopter.do("POST", "/v1/me/legal-acceptances", map[string]any{"document_id": legal.ID}, 204, nil)
	s.owner.do("POST", "/v1/legal/documents", map[string]any{"kind": "terms", "version": unique(), "title": "Terms", "content": "Lorem ipsum dolor sit amet", "reviewed_by": "Test reviewer", "effective_at": time.Now().Add(time.Hour)}, 403, nil)
}

func TestAgreementSigning(t *testing.T) {
	s := newAdoptionSetup(t)
	s.owner.do("POST", "/v1/applications/"+s.applicationID+"/decision", map[string]any{"decision": "approved", "reason": "Approved after interview"}, 200, nil)
	var template struct {
		ID      string
		Version int
	}
	path := "/v1/organizations/" + s.org.ID + "/agreement-templates"
	s.owner.do("POST", path, map[string]any{"kind": "adoption", "body": "{{user_name}} adopts {{animal_name}} from {{organization_name}} on {{date}}."}, 201, &template)
	if template.Version != 1 {
		t.Fatal("wrong template version")
	}
	s.owner.do("GET", path, nil, 200, nil)
	var doc signedDocument
	s.owner.do("POST", "/v1/applications/"+s.applicationID+"/agreement", map[string]any{"template_id": template.ID, "representative_id": s.owner.User.ID}, 201, &doc)
	if strings.Contains(doc.Body, "{{") || len(doc.SHA256) != 64 {
		t.Fatal("agreement not populated")
	}
	registerUser(t).do("GET", "/v1/agreements/"+doc.ID, nil, 403, nil)
	signDocument(t, s.adopter, doc)
	signDocument(t, s.owner, doc)
	s.adopter.do("GET", "/v1/agreements/"+doc.ID, nil, 200, &doc)
	if len(doc.Signatures) != 2 {
		t.Fatal("signatures missing")
	}
	s.adopter.do("GET", "/v1/applications/"+s.applicationID+"/agreement", nil, 200, nil)
	s.adopter.do("POST", "/v1/agreements/"+doc.ID+"/signing-code", nil, 409, nil)
	s.owner.do("GET", "/v1/agreements/"+doc.ID+"/print", nil, 200, nil)
}

func TestFollowupReportsAndReturn(t *testing.T) {
	s := newAdoptionSetup(t)
	completeAdoption(t, s)
	app := "/v1/applications/" + s.applicationID
	var task struct {
		ID, Status string
		Reports    []any
		Reviews    []any
	}
	s.owner.do("POST", app+"/follow-ups", map[string]any{"due_at": time.Now().Add(72 * time.Hour), "instructions": "Report animal welfare and attach photos"}, 201, &task)
	path := "/v1/follow-ups/" + task.ID
	registerUser(t).do("GET", path, nil, 403, nil)
	report := map[string]any{"notes": "The animal is healthy and adapting to the new household", "welfare_status": "good"}
	s.adopter.do("POST", path+"/reports", report, 200, &task)
	if task.Status != "submitted" || len(task.Reports) != 1 {
		t.Fatal("follow-up report missing")
	}
	s.adopter.do("POST", path+"/review", map[string]any{"decision": "closed", "reason": "All information reviewed"}, 403, nil)
	s.owner.do("POST", path+"/review", map[string]any{"decision": "changes_requested", "reason": "Please include more detail"}, 200, nil)
	s.adopter.do("POST", path+"/reports", report, 200, nil)
	s.owner.do("POST", path+"/review", map[string]any{"decision": "closed", "reason": "All information reviewed"}, 200, &task)
	if task.Status != "closed" || len(task.Reports) != 2 || len(task.Reviews) != 2 {
		t.Fatal("follow-up history missing")
	}
	s.adopter.do("POST", path+"/reports", report, 409, nil)
}

func TestSigningCodeAttemptLimit(t *testing.T) {
	s := newAdoptionSetup(t)
	s.owner.do("POST", "/v1/applications/"+s.applicationID+"/decision", map[string]any{"decision": "approved", "reason": "Applicant approved after review"}, 200, nil)
	var template struct{ ID string }
	s.owner.do("POST", "/v1/organizations/"+s.org.ID+"/agreement-templates", map[string]any{"kind": "adoption", "body": "{{user_name}} adopts {{animal_name}} from {{organization_name}}."}, 201, &template)
	var doc signedDocument
	s.owner.do("POST", "/v1/applications/"+s.applicationID+"/agreement", map[string]any{"template_id": template.ID, "representative_id": s.owner.User.ID}, 201, &doc)
	path := "/v1/agreements/" + doc.ID
	s.adopter.do("POST", path+"/signing-code", nil, 202, nil)
	code := emailCode(t, s.adopter.User.Email)
	s.adopter.do("POST", path+"/signing-code", nil, 429, nil)
	wrong := "000000"
	if wrong == code {
		wrong = "111111"
	}
	for i := 0; i < 5; i++ {
		s.adopter.do("POST", path+"/signatures", map[string]any{"code": wrong, "sha256": doc.SHA256, "accept": true}, 422, nil)
	}
	s.adopter.do("POST", path+"/signatures", map[string]any{"code": code, "sha256": doc.SHA256, "accept": true}, 409, nil)
	s.adopter.do("GET", "/v1/me", nil, 200, nil)
}
