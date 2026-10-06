package api

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/jackc/pgx/v5/pgtype"
)

type routeRegistry struct {
	mux  *http.ServeMux
	spec *huma.OpenAPI
}

type requestContract struct {
	registry huma.Registry
	schema   *huma.Schema
	typeOf   reflect.Type
}

type requestContractKey struct{}

// OpenAPI builds the contract from the same registrations used by Handler.
// Generating documentation does not need configuration, credentials, or a database.
func OpenAPI() *huma.OpenAPI {
	return registerRoutes(&Server{}, nil)
}

func newRouteRegistry(mux *http.ServeMux) *routeRegistry {
	registry := huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)
	// pgtype values have custom JSON serialization; document their wire values,
	// rather than the implementation fields of the pgx wrapper structs.
	registry.RegisterTypeAlias(reflect.TypeFor[pgtype.UUID](), reflect.TypeFor[wireUUID]())
	registry.RegisterTypeAlias(reflect.TypeFor[pgtype.Timestamptz](), reflect.TypeFor[wireTimestamp]())
	registry.RegisterTypeAlias(reflect.TypeFor[pgtype.Date](), reflect.TypeFor[wireDate]())
	registry.RegisterTypeAlias(reflect.TypeFor[pgtype.Numeric](), reflect.TypeFor[wireNumeric]())
	return &routeRegistry{mux: mux, spec: &huma.OpenAPI{
		OpenAPI: "3.1.0",
		Info: &huma.Info{Title: "PetFinder API", Version: "1.0.0",
			Description: "Safe animal adoption mediated by verified organizations. Adopters and advertisers communicate with organizations in separate conversations.",
			License:     &huma.License{Name: "MIT", URL: "https://opensource.org/license/mit"},
		},
		Servers:  []*huma.Server{{URL: "/", Description: "Current API server"}},
		Security: []map[string][]string{{}}, // Public unless the operation requires bearerAuth.
		Components: &huma.Components{Schemas: registry, SecuritySchemes: map[string]*huma.SecurityScheme{
			"bearerAuth": {Type: "http", Scheme: "bearer", BearerFormat: "JWT"},
		}},
	}}
}

type wireUUID string

func (wireUUID) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{Type: "string", Format: "uuid", Nullable: true}
}

type wireTimestamp string

func (wireTimestamp) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{Type: "string", Format: "date-time", Nullable: true}
}

type wireDate string

func (wireDate) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{Type: "string", Format: "date", Nullable: true}
}

type wireNumeric float64

func (wireNumeric) Schema(huma.Registry) *huma.Schema {
	return &huma.Schema{Type: "number", Nullable: true}
}

type routeOption func(*routeRegistry, *huma.Operation)

func bearerAuth(optional bool) routeOption {
	return func(_ *routeRegistry, op *huma.Operation) {
		op.Security = []map[string][]string{{"bearerAuth": {}}}
		if optional {
			op.Security = append(op.Security, map[string][]string{})
		}
	}
}

func adminOnly(reg *routeRegistry, op *huma.Operation) {
	bearerAuth(false)(reg, op)
	op.Description = "Requires the platform_admin role."
}

func responseAlternative[T any](reg *routeRegistry, op *huma.Operation) {
	response := op.Responses[strconv.Itoa(op.DefaultStatus)].Content["application/json"]
	response.Schema = &huma.Schema{AnyOf: []*huma.Schema{response.Schema, reg.spec.Components.Schemas.Schema(reflect.TypeFor[T](), true, "")}}
}

func serviceUnavailable(reg *routeRegistry, op *huma.Operation) {
	op.Responses["503"] = &huma.Response{Description: "Database unavailable", Content: map[string]*huma.MediaType{
		"application/json": {Schema: reg.spec.Components.Schemas.Schema(reflect.TypeFor[Problem](), true, "")},
	}}
}

func queryParameters[T any](reg *routeRegistry, op *huma.Operation) {
	schema := huma.SchemaFromType(reg.spec.Components.Schemas, reflect.TypeFor[T]())
	for i := 0; i < reflect.TypeFor[T]().NumField(); i++ {
		field := reflect.TypeFor[T]().Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		op.Parameters = append(op.Parameters, &huma.Param{Name: name, In: "query", Schema: schema.Properties[name], Description: field.Tag.Get("doc")})
	}
}

var pathParameters = regexp.MustCompile(`\{([^}]+)\}`)

// registerRoute registers HTTP handling and generates its schemas together.
// Req is the type decoded by the handler; Resp is the type it serializes.
func registerRoute[Req, Resp any](reg *routeRegistry, method, path, id, summary, tag string, status int, handler http.Handler, options ...routeOption) {
	registry := reg.spec.Components.Schemas
	op := &huma.Operation{Method: method, Path: path, OperationID: id, Summary: summary, Tags: []string{tag}, DefaultStatus: status,
		Responses: map[string]*huma.Response{},
	}
	for _, match := range pathParameters.FindAllStringSubmatch(path, -1) {
		op.Parameters = append(op.Parameters, &huma.Param{Name: match[1], In: "path", Required: true, Schema: &huma.Schema{Type: "string", Format: "uuid"}})
	}
	response := &huma.Response{Description: http.StatusText(status)}
	if reflect.TypeFor[Resp]() != reflect.TypeFor[noBody]() {
		response.Content = map[string]*huma.MediaType{"application/json": {Schema: registry.Schema(reflect.TypeFor[Resp](), true, "")}}
	}
	op.Responses[strconv.Itoa(status)] = response
	problem := registry.Schema(reflect.TypeFor[Problem](), true, "")
	for _, code := range []int{400, 401, 403, 404, 409, 422, 500} {
		schema := problem
		if code == 401 || code == 403 {
			schema = &huma.Schema{AnyOf: []*huma.Schema{problem, registry.Schema(reflect.TypeFor[authErrorResponse](), true, "")}}
		}
		op.Responses[strconv.Itoa(code)] = &huma.Response{Description: http.StatusText(code), Content: map[string]*huma.MediaType{"application/json": {Schema: schema}}}
	}
	var contract *requestContract
	if reflect.TypeFor[Req]() != reflect.TypeFor[noBody]() {
		schema := registry.Schema(reflect.TypeFor[Req](), true, "")
		op.RequestBody = &huma.RequestBody{Required: true, Content: map[string]*huma.MediaType{"application/json": {Schema: schema}}}
		contract = &requestContract{registry: registry, schema: schema, typeOf: reflect.TypeFor[Req]()}
	}
	for _, option := range options {
		option(reg, op)
	}
	reg.spec.AddOperation(op)
	if reg.mux != nil {
		reg.mux.Handle(method+" "+path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if contract != nil {
				r = r.WithContext(context.WithValue(r.Context(), requestContractKey{}, contract))
			}
			handler.ServeHTTP(w, r)
		}))
	}
}

func serveOpenAPI(mux *http.ServeMux, spec *huma.OpenAPI) {
	encodedJSON, err := json.Marshal(spec)
	if err != nil {
		panic(err)
	}
	encodedYAML, err := spec.YAML()
	if err != nil {
		panic(err)
	}
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(encodedJSON)
	})
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		_, _ = w.Write(encodedYAML)
	})
}

type paginationQuery struct {
	Page     int `json:"page,omitempty" minimum:"1" default:"1"`
	PageSize int `json:"page_size,omitempty" minimum:"1" maximum:"100" default:"20"`
}

type organizationQuery struct {
	State string `json:"state,omitempty"`
	City  string `json:"city,omitempty"`
}

type animalQuery struct {
	Species   string  `json:"species,omitempty" enum:"dog,cat,other"`
	Size      string  `json:"size,omitempty" enum:"small,medium,large,extra_large"`
	State     string  `json:"state,omitempty"`
	City      string  `json:"city,omitempty"`
	Latitude  float64 `json:"latitude,omitempty" minimum:"-90" maximum:"90"`
	Longitude float64 `json:"longitude,omitempty" minimum:"-180" maximum:"180"`
}

type preferenceQuery struct {
	Value string `json:"value,omitempty" enum:"liked,not_interested" default:"liked"`
}

type organizationIDQuery struct {
	OrganizationID string `json:"organization_id,omitempty" format:"uuid" doc:"Requires active membership in the organization, or administrator access."`
}

type applicationQuery struct {
	Status string `json:"status,omitempty" enum:"submitted,screening,more_information_requested,approved,rejected,withdrawn,completed,cancelled"`
}

type messagePaginationQuery struct {
	PageSize int `json:"page_size,omitempty" minimum:"1" maximum:"100" default:"20" doc:"Returns the most recent messages, newest first."`
}
