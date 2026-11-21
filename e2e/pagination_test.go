//go:build e2e

package e2e

import (
	"net/http"
	"net/url"
	"testing"
)

type pageInfo struct {
	Items []map[string]any `json:"items"`
	Page  struct {
		Page     int `json:"page"`
		PageSize int `json:"page_size"`
		Total    int `json:"total"`
	} `json:"page"`
}

// Collections report the number of matching items across all pages.
func TestCollectionsReportRealTotals(t *testing.T) {
	owner, advertiser, admin := registerUser(t), registerUser(t), registerAdmin(t)
	city := "Cidade " + unique()
	org := createOrganization(t, owner, city)
	verifyOrganization(t, admin, org.ID)
	for range 3 {
		advertiser.do(http.MethodPost, "/v1/animals", animalRequest(org.ID, city), http.StatusCreated, nil)
		owner.do(http.MethodPost, "/v1/organizations/"+org.ID+"/posts", map[string]any{"body": "Novidades da ONG"}, http.StatusCreated, nil)
	}
	check := func(who user, path string, wantItems, wantTotal int) {
		t.Helper()
		var got pageInfo
		who.do(http.MethodGet, path, nil, http.StatusOK, &got)
		if len(got.Items) != wantItems || got.Page.Total != wantTotal {
			t.Fatalf("%s: %d items, total %d; want %d items, total %d", path, len(got.Items), got.Page.Total, wantItems, wantTotal)
		}
	}
	check(advertiser, "/v1/me/animals?page_size=2", 2, 3)
	check(advertiser, "/v1/me/animals?page_size=2&page=2", 1, 3)
	check(owner, "/v1/organizations/"+org.ID+"/animals?page_size=1", 1, 3)
	check(owner, "/v1/organizations/"+org.ID+"/animals?page_size=1&status=available", 0, 0)
	check(owner, "/v1/posts?organization_id="+org.ID+"&page_size=2", 2, 3)
	check(advertiser, "/v1/organizations?city="+url.QueryEscape(city)+"&page_size=1", 1, 1)
}

// Application lists filter by animal (organization) and status (adopter), and
// their totals follow the filters.
func TestApplicationListFilters(t *testing.T) {
	s := newAdoptionSetup(t)
	var other animal
	s.owner.do(http.MethodPost, "/v1/animals", animalRequest(s.org.ID, s.city), http.StatusCreated, &other)
	check := func(who user, path string, wantItems, wantTotal int) {
		t.Helper()
		var got pageInfo
		who.do(http.MethodGet, path, nil, http.StatusOK, &got)
		if len(got.Items) != wantItems || got.Page.Total != wantTotal {
			t.Fatalf("%s: %d items, total %d; want %d/%d", path, len(got.Items), got.Page.Total, wantItems, wantTotal)
		}
	}
	base := "/v1/applications?organization_id=" + s.org.ID
	check(s.owner, base+"&animal_id="+s.animal.ID, 1, 1)
	check(s.owner, base+"&animal_id="+other.ID, 0, 0)
	check(s.adopter, "/v1/applications?status=submitted", 1, 1)
	check(s.adopter, "/v1/applications?status=completed", 0, 0)
	s.owner.do(http.MethodGet, base+"&animal_id=nope", nil, http.StatusBadRequest, nil)
}
