//go:build e2e

package e2e

import (
	"net/http"
	"net/url"
	"testing"
	"time"
)

type animalPage struct {
	Items []animal `json:"items"`
}

func discover(c client, city string) []animal {
	c.t.Helper()
	var page animalPage
	c.do(http.MethodGet, "/v1/animals?city="+url.QueryEscape(city), nil, http.StatusOK, &page)
	return page.Items
}

func TestPrivateSubmissionReview(t *testing.T) {
	owner, advertiser, adopter, anonymous := registerUser(t), registerUser(t), registerUser(t), client{t: t}
	city := "Cidade " + unique()
	org := createOrganization(t, owner, city)
	verifyOrganization(t, registerAdmin(t), org.ID)

	var submitted animal
	advertiser.do(http.MethodPost, "/v1/animals", animalRequest(org.ID, city), http.StatusCreated, &submitted)
	if submitted.Status != "pending_organization" || submitted.Source != "private_submission" {
		t.Fatalf("advertiser submission %+v should await organization review", submitted)
	}
	if items := discover(anonymous, city); len(items) != 0 {
		t.Fatalf("pending animal is discoverable: %+v", items)
	}
	anonymous.do(http.MethodGet, "/v1/animals/"+submitted.ID, nil, http.StatusNotFound, nil)
	advertiser.do(http.MethodGet, "/v1/animals/"+submitted.ID, nil, http.StatusOK, nil)

	statusPath := "/v1/animals/" + submitted.ID + "/status"
	advertiser.do(http.MethodPatch, statusPath, map[string]any{"status": "available", "reason": "autopublicação"}, http.StatusForbidden, nil)
	var published animal
	owner.do(http.MethodPatch, statusPath, map[string]any{"status": "available", "reason": "cadastro revisado"}, http.StatusOK, &published)
	if published.Status != "available" {
		t.Fatalf("status = %q after review", published.Status)
	}
	if items := discover(anonymous, city); len(items) != 1 || items[0].ID != submitted.ID {
		t.Fatalf("published animal not discoverable: %+v", items)
	}

	adopter.do(http.MethodPut, "/v1/animals/"+submitted.ID+"/preference", map[string]any{"value": "not_interested"}, http.StatusOK, nil)
	if items := discover(adopter.client, city); len(items) != 0 {
		t.Fatalf("dismissed animal still shown to the adopter: %+v", items)
	}
	if items := discover(anonymous, city); len(items) != 1 {
		t.Fatalf("dismissal by one adopter hid the animal from others: %+v", items)
	}
}

func TestApplicationRequiresLike(t *testing.T) {
	owner, adopter := registerUser(t), registerUser(t)
	city := "Cidade " + unique()
	org := createOrganization(t, owner, city)
	verifyOrganization(t, registerAdmin(t), org.ID)
	var a animal
	owner.do(http.MethodPost, "/v1/animals", animalRequest(org.ID, city), http.StatusCreated, &a)

	application := map[string]any{"animal_id": a.ID, "introduction": "Gostaria muito de adotar este animal."}
	adopter.do(http.MethodPost, "/v1/applications", application, http.StatusNotFound, nil)
	adopter.do(http.MethodPut, "/v1/animals/"+a.ID+"/preference", map[string]any{"value": "liked"}, http.StatusOK, nil)
	adopter.do(http.MethodPost, "/v1/applications", application, http.StatusCreated, nil)
	adopter.do(http.MethodPost, "/v1/applications", application, http.StatusConflict, nil)
}

type applicationDetail struct {
	Application struct {
		Status string `json:"status"`
	} `json:"application"`
	Screening *struct {
		Status string `json:"status"`
	} `json:"screening"`
}

func TestAdoptionFromApplicationToDelivery(t *testing.T) {
	s := newAdoptionSetup(t)
	applicationPath := "/v1/applications/" + s.applicationID
	var detail applicationDetail

	s.adopter.do(http.MethodPut, applicationPath+"/screening", map[string]any{
		"housing": "Casa com quintal", "household": "Dois adultos", "experience": "Já tive cães",
		"care_plan": "Passeios diários e consultas veterinárias regulares.", "accepts_follow_up": true,
	}, http.StatusOK, nil)
	s.owner.do(http.MethodGet, applicationPath, nil, http.StatusOK, &detail)
	if detail.Application.Status != "screening" || detail.Screening == nil || detail.Screening.Status != "submitted" {
		t.Fatalf("after screening: %+v", detail)
	}

	decision := map[string]any{"decision": "approved", "reason": "perfil adequado"}
	s.adopter.do(http.MethodPost, applicationPath+"/decision", decision, http.StatusForbidden, nil)
	s.owner.do(http.MethodPost, applicationPath+"/decision", decision, http.StatusOK, nil)
	var current struct {
		Animal animal `json:"animal"`
	}
	s.owner.do(http.MethodGet, "/v1/animals/"+s.animal.ID, nil, http.StatusOK, &current)
	if current.Animal.Status != "adoption_in_progress" {
		t.Fatalf("animal status = %q after approval", current.Animal.Status)
	}
	if items := discover(client{t: t}, s.city); len(items) != 0 {
		t.Fatalf("animal in adoption is still discoverable: %+v", items)
	}

	start := time.Now().Add(48 * time.Hour).UTC().Truncate(time.Second)
	var appointment struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	s.owner.do(http.MethodPost, applicationPath+"/appointments", map[string]any{
		"purpose": "delivery", "starts_at": start, "ends_at": start.Add(time.Hour), "timezone": "America/Recife",
		"location_name": "Sede do abrigo", "address": "Rua das Flores, 100",
	}, http.StatusCreated, &appointment)
	appointmentPath := "/v1/appointments/" + appointment.ID
	if appointment.Status != "proposed" {
		t.Fatalf("new appointment status = %q", appointment.Status)
	}
	s.owner.do(http.MethodPost, appointmentPath+"/confirmations", map[string]any{"type": "delivery"}, http.StatusConflict, nil)
	s.owner.do(http.MethodPatch, appointmentPath, map[string]any{"status": "scheduled"}, http.StatusOK, &appointment)
	if appointment.Status != "scheduled" {
		t.Fatalf("appointment status = %q after scheduling", appointment.Status)
	}

	s.adopter.do(http.MethodPost, appointmentPath+"/otp", nil, http.StatusForbidden, nil)
	var otp struct {
		Code string `json:"code"`
	}
	s.owner.do(http.MethodPost, appointmentPath+"/otp", nil, http.StatusCreated, &otp)
	wrong := "000000"
	if otp.Code == wrong {
		wrong = "111111"
	}
	confirmations := appointmentPath + "/confirmations"
	s.adopter.do(http.MethodPost, confirmations, map[string]any{"type": "receipt", "otp_code": wrong}, http.StatusUnauthorized, nil)
	s.adopter.do(http.MethodPost, confirmations, map[string]any{"type": "delivery"}, http.StatusForbidden, nil)
	s.owner.do(http.MethodPost, confirmations, map[string]any{"type": "delivery"}, http.StatusOK, nil)
	s.adopter.do(http.MethodPost, confirmations, map[string]any{"type": "receipt", "otp_code": otp.Code}, http.StatusOK, nil)

	s.adopter.do(http.MethodGet, applicationPath, nil, http.StatusOK, &detail)
	if detail.Application.Status != "completed" {
		t.Fatalf("application status = %q after both confirmations", detail.Application.Status)
	}
	s.owner.do(http.MethodGet, "/v1/animals/"+s.animal.ID, nil, http.StatusOK, &current)
	if current.Animal.Status != "adopted" {
		t.Fatalf("animal status = %q after both confirmations", current.Animal.Status)
	}
	// The completed appointment accepts no further confirmations.
	s.adopter.do(http.MethodPost, confirmations, map[string]any{"type": "receipt", "otp_code": otp.Code}, http.StatusConflict, nil)
}

func TestApplicationIsPrivateToItsParties(t *testing.T) {
	s := newAdoptionSetup(t)
	registerUser(t).do(http.MethodGet, "/v1/applications/"+s.applicationID, nil, http.StatusForbidden, nil)

	type applicationList struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	var adopterList, orgList applicationList
	s.adopter.do(http.MethodGet, "/v1/applications", nil, http.StatusOK, &adopterList)
	s.owner.do(http.MethodGet, "/v1/applications?organization_id="+s.org.ID, nil, http.StatusOK, &orgList)
	for _, list := range []applicationList{adopterList, orgList} {
		if len(list.Items) != 1 || list.Items[0].ID != s.applicationID {
			t.Errorf("applications = %+v, want only %s", list.Items, s.applicationID)
		}
	}
	s.adopter.do(http.MethodGet, "/v1/applications?organization_id="+s.org.ID, nil, http.StatusForbidden, nil)
}
