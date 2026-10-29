//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pet-finder-app/petfinder-api/internal/api"
	"github.com/pet-finder-app/petfinder-api/internal/auth"
)

const (
	testPassword = "e2e-password-123"
	accessTTL    = 15 * time.Minute
)

var (
	baseURL string
	// pool seeds what the API cannot do on its own, such as granting the
	// platform administrator role.
	pool *pgxpool.Pool
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	databaseURL := os.Getenv("E2E_DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "E2E_DATABASE_URL is required; use make test-e2e")
		return 1
	}
	// Request logs would bury test failures; failures already carry the response.
	slog.SetDefault(slog.New(slog.DiscardHandler))
	ctx := context.Background()
	var err error
	pool, err = pgxpool.New(ctx, databaseURL)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "database unavailable:", err)
		return 1
	}
	tokens, err := auth.NewTokenManager(auth.TokenConfig{
		Secret: []byte("e2e-secret-that-is-at-least-32-bytes"), Issuer: "petfinder-api", Audience: "petfinder", AccessTTL: accessTTL,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	apiServer := api.NewServer(pool, tokens, accessTTL, time.Hour)
	server := httptest.NewServer(apiServer.Handler())
	defer server.Close()
	defer apiServer.CloseChat()
	baseURL = server.URL
	return m.Run()
}

// client calls the API as one user; an empty token makes anonymous requests.
type client struct {
	t     *testing.T
	token string
}

// do sends body as JSON, fails the test unless the response has status want,
// and decodes the response into out when it is not nil.
func (c client) do(method, path string, body any, want int, out any) {
	c.t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	data, err := io.ReadAll(res.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	if res.StatusCode != want {
		c.t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.StatusCode, want, data)
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: decode %s: %v", method, path, data, err)
		}
	}
}

type session struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	User         struct {
		ID    string   `json:"id"`
		Email string   `json:"email"`
		Roles []string `json:"roles"`
	} `json:"user"`
}

type user struct {
	client
	session
}

// unique returns a short random suffix so tests sharing a database never
// collide on unique columns or see each other's data in listings.
func unique() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
}

func registerUser(t *testing.T) user {
	t.Helper()
	var s session
	client{t: t}.do(http.MethodPost, "/v1/auth/register", map[string]any{
		"name": "E2E " + unique(), "email": "e2e-" + unique() + "@example.com", "password": testPassword,
		"accepted_terms": true, "accepted_privacy_policy": true,
	}, http.StatusCreated, &s)
	return user{client: client{t: t, token: s.AccessToken}, session: s}
}

func login(t *testing.T, email string) user {
	t.Helper()
	var s session
	client{t: t}.do(http.MethodPost, "/v1/auth/login", map[string]any{"email": email, "password": testPassword}, http.StatusOK, &s)
	return user{client: client{t: t, token: s.AccessToken}, session: s}
}

// registerAdmin grants the platform administrator role directly in the
// database, since no endpoint grants it, and signs in again for a token
// carrying the role.
func registerAdmin(t *testing.T) user {
	t.Helper()
	u := registerUser(t)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE key = 'platform_admin'`, u.User.ID)
	if err != nil {
		t.Fatal(err)
	}
	return login(t, u.User.Email)
}

type organization struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name"`
	VerificationStatus string  `json:"verification_status"`
	LegalName          *string `json:"legal_name"`
}

func location(city string) map[string]any {
	return map[string]any{"city": city, "state": "PE", "country_code": "BR"}
}

func createOrganization(t *testing.T, owner user, city string) organization {
	t.Helper()
	var org organization
	owner.do(http.MethodPost, "/v1/organizations", map[string]any{
		"name": "Abrigo E2E " + unique(), "legal_name": "Associação Abrigo E2E", "registration_number": "12345678000190",
		"description":       "Organização criada pelos testes ponta a ponta.",
		"official_channels": map[string]any{"email": "contato-" + unique() + "@example.com"},
		"location":          location(city),
	}, http.StatusCreated, &org)
	return org
}

func verifyOrganization(t *testing.T, admin user, orgID string) {
	t.Helper()
	admin.do(http.MethodPost, "/v1/organizations/"+orgID+"/verification", map[string]any{
		"decision": "verified", "reason": "documentação conferida pelo e2e",
	}, http.StatusOK, nil)
}

type animal struct {
	ID                        string `json:"id"`
	Status                    string `json:"status"`
	Source                    string `json:"source"`
	ResponsibleOrganizationID string `json:"responsible_organization_id"`
}

func animalRequest(orgID, city string) map[string]any {
	return map[string]any{
		"organization_id": orgID, "name": "Bolinha", "species": "dog", "size": "medium", "sex": "female",
		"description": "Cadela dócil, vacinada e acostumada com crianças.", "location": location(city),
	}
}

// adoptionSetup is a verified organization in its own city with a published
// animal and an adopter who has applied for it.
type adoptionSetup struct {
	owner, adopter user
	org            organization
	animal         animal
	city           string
	applicationID  string
	conversationID string
}

func newAdoptionSetup(t *testing.T) adoptionSetup {
	t.Helper()
	s := adoptionSetup{owner: registerUser(t), adopter: registerUser(t), city: "Cidade " + unique()}
	s.org = createOrganization(t, s.owner, s.city)
	verifyOrganization(t, registerAdmin(t), s.org.ID)
	s.owner.do(http.MethodPost, "/v1/animals", animalRequest(s.org.ID, s.city), http.StatusCreated, &s.animal)
	s.adopter.do(http.MethodPut, "/v1/animals/"+s.animal.ID+"/preference", map[string]any{"value": "liked"}, http.StatusOK, nil)
	var created struct {
		Application    struct{ ID string } `json:"application"`
		ConversationID string              `json:"conversation_id"`
	}
	s.adopter.do(http.MethodPost, "/v1/applications", map[string]any{
		"animal_id": s.animal.ID, "introduction": "Tenho quintal amplo e tempo para cuidar dela.",
	}, http.StatusCreated, &created)
	s.applicationID, s.conversationID = created.Application.ID, created.ConversationID
	return s
}
