package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/pet-finder-app/petfinder-api/internal/database"
)

func TestOpenAPIContract(t *testing.T) {
	spec := OpenAPI()
	if spec.OpenAPI != "3.1.0" {
		t.Fatalf("version = %q", spec.OpenAPI)
	}
	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	operations := 0
	ids := map[string]bool{}
	for _, path := range document["paths"].(map[string]any) {
		for _, raw := range path.(map[string]any) {
			op := raw.(map[string]any)
			id := op["operationId"].(string)
			if ids[id] {
				t.Fatalf("duplicate operation ID %s", id)
			}
			ids[id] = true
			operations++
		}
	}
	if operations != 40 {
		t.Fatalf("documented %d operations, want 40", operations)
	}
	// Every generated reference must resolve within the document.
	var checkRefs func(any)
	checkRefs = func(value any) {
		switch v := value.(type) {
		case map[string]any:
			if ref, ok := v["$ref"].(string); ok {
				var target any = document
				for _, part := range strings.Split(strings.TrimPrefix(ref, "#/"), "/") {
					object, ok := target.(map[string]any)
					if !ok || object[part] == nil {
						t.Fatalf("unresolved reference %s", ref)
					}
					target = object[part]
				}
			}
			for _, nested := range v {
				checkRefs(nested)
			}
		case []any:
			for _, nested := range v {
				checkRefs(nested)
			}
		}
	}
	checkRefs(document)
	schemas := document["components"].(map[string]any)["schemas"].(map[string]any)
	animal := schemas["Animal"].(map[string]any)["properties"].(map[string]any)
	for field, format := range map[string]string{"id": "uuid", "created_at": "date-time", "birth_date": "date"} {
		property := animal[field].(map[string]any)
		if property["format"] != format || property["properties"] != nil {
			t.Errorf("%s does not describe its serialized value: %v", field, property)
		}
	}
	logout := spec.Paths["/v1/auth/logout"].Post
	if logout.Responses["204"].Content != nil {
		t.Fatal("204 response must not declare a body")
	}
	if len(logout.Security) != 1 || len(spec.Paths["/v1/animals"].Get.Security) != 2 {
		t.Fatal("required and optional authentication not documented correctly")
	}
}

func TestOpenAPIEndpoints(t *testing.T) {
	handler := (&Server{}).Handler()
	spec := OpenAPI()
	jsonSpec, _ := json.Marshal(spec)
	yamlSpec, _ := spec.YAML()
	for _, test := range []struct {
		path, contentType string
		body              []byte
	}{
		{"/openapi.json", "application/json", jsonSpec},
		{"/openapi.yaml", "application/yaml", yamlSpec},
	} {
		t.Run(test.path, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != http.StatusOK || !strings.HasPrefix(response.Header().Get("Content-Type"), test.contentType) {
				t.Fatalf("status %d, content type %q", response.Code, response.Header().Get("Content-Type"))
			}
			if !bytes.Equal(response.Body.Bytes(), test.body) {
				t.Fatal("served contract differs from generated contract")
			}
		})
	}
}

func TestRouteContractValidation(t *testing.T) {
	const valid = `{"name":"Ana","email":"ana@example.com","password":"securepassword12","accepted_terms":true,"accepted_privacy_policy":true}`
	for _, test := range []struct {
		name, body string
		status     int
	}{
		{"valid", valid, 204},
		{"missing required field", strings.Replace(valid, `"accepted_terms":true,`, "", 1), 422},
		{"false consent", strings.Replace(valid, `"accepted_terms":true`, `"accepted_terms":false`, 1), 422},
		{"invalid email", strings.Replace(valid, "ana@example.com", "invalid", 1), 422},
		{"short name", strings.Replace(valid, "Ana", "A", 1), 422},
		{"short password", strings.Replace(valid, "securepassword12", "short", 1), 422},
		{"unknown field", strings.Replace(valid, `"name":`, `"extra":1,"name":`, 1), 422},
		{"trailing JSON", valid + "{}", 400},
		{"null", "null", 422},
	} {
		t.Run(test.name, func(t *testing.T) {
			mux := http.NewServeMux()
			reg := newRouteRegistry(mux)
			registerRoute[registerRequest, noBody](reg, "POST", "/test", "test", "Test", "Test", 204, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request registerRequest
				if decodeJSON(w, r, &request) {
					w.WriteHeader(204)
				}
			}))
			w := httptest.NewRecorder()
			mux.ServeHTTP(w, httptest.NewRequest("POST", "/test", strings.NewReader(test.body)))
			if w.Code != test.status {
				t.Fatalf("status %d, want %d: %s", w.Code, test.status, w.Body.String())
			}
		})
	}
}

func TestPatchContractAllowsOmittedFields(t *testing.T) {
	reg := newRouteRegistry(nil)
	schema := reg.spec.Components.Schemas.Schema(reflect.TypeFor[updateAnimalRequest](), true, "")
	for _, body := range []string{`{}`, `{"name":"Luna"}`} {
		var value any
		if err := json.Unmarshal([]byte(body), &value); err != nil {
			t.Fatal(err)
		}
		result := &huma.ValidateResult{}
		huma.Validate(reg.spec.Components.Schemas, schema, huma.NewPathBuffer(nil, 0), huma.ModeWriteToServer, value, result)
		if len(result.Errors) > 0 {
			t.Fatalf("partial update rejected: %v", result.Errors)
		}
	}
}

func TestOrganizationResponsePreservesVisibilityAndNulls(t *testing.T) {
	for _, privileged := range []bool{false, true} {
		data, err := json.Marshal(organizationView(database.Organization{}, privileged))
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]any
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		for _, field := range []string{"legal_name", "registration_number", "verification_notes"} {
			if _, present := value[field]; present != privileged {
				t.Errorf("%s presence = %v, privileged = %v", field, present, privileged)
			}
		}
		channels := value["official_channels"].(map[string]any)
		for _, field := range []string{"website", "phone"} {
			if raw, present := channels[field]; !present || raw != nil {
				t.Errorf("%s must remain present and null", field)
			}
		}
	}
}
