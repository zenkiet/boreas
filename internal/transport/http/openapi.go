package httptransport

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/swaggest/jsonschema-go"
	"github.com/swaggest/openapi-go"
	"github.com/swaggest/openapi-go/openapi3"
)

const bearerScheme = "bearerAuth"

// OpenAPISpec rejects unmatched path parameters so the generated contract cannot drift from routing.
func OpenAPISpec() (*openapi3.Spec, error) {
	reflector := openapi3.NewReflector()
	// The reflector writes OpenAPI 3.0 (`nullable`), so the document says so.
	reflector.Spec = &openapi3.Spec{Openapi: "3.0.3"}
	spec := reflector.SpecEns()
	spec.Info.
		WithTitle("Boreas API").
		WithVersion("1.0.0").
		WithDescription("Per-task staging environments. Tasks belong to projects and are " +
			"served at /{project}/{task}/. Proxied task traffic is public; this API is not.")
	spec.SetHTTPBearerTokenSecurity(bearerScheme, "",
		"Login session or API token, sent as: Authorization: Bearer <token>")

	js := reflector.JSONSchemaReflector()
	uuidSchema := jsonschema.Schema{}
	uuidSchema.AddType(jsonschema.String)
	uuidSchema.WithFormat("uuid")
	// uuid.UUID is [16]byte, which would otherwise reflect as an array.
	js.AddTypeMapping(uuid.UUID{}, uuidSchema)
	js.DefaultOptions = append(js.DefaultOptions,
		jsonschema.InterceptDefName(schemaName),
		jsonschema.InterceptProp(requiredFromOmitEmpty),
	)

	for _, r := range routeTable {
		if err := addOperation(reflector, r); err != nil {
			return nil, fmt.Errorf("%s %s: %w", r.method, r.path, err)
		}
	}
	// A pointer in a PATCH body makes its shared enum nullable; the values themselves never are.
	for _, s := range spec.Components.Schemas.MapOfSchemaOrRefValues {
		if s.Schema != nil && len(s.Schema.Enum) > 0 {
			s.Schema.Nullable = nil
		}
	}
	return spec, nil
}

func requiredFromOmitEmpty(params jsonschema.InterceptPropParams) error {
	if !params.Processed || params.Name == "" || params.ParentSchema == nil {
		return nil
	}
	tag, ok := params.Field.Tag.Lookup("json")
	if !ok || strings.Contains(tag, ",omitempty") || strings.Contains(tag, ",omitzero") {
		return nil
	}
	if !slices.Contains(params.ParentSchema.Required, params.Name) {
		params.ParentSchema.Required = append(params.ParentSchema.Required, params.Name)
	}
	return nil
}

func schemaName(_ reflect.Type, defaultName string) string {
	name := defaultName
	for _, prefix := range []string{"Httptransport", "Http", "Core", "Uuid"} {
		name = strings.TrimPrefix(name, prefix)
	}
	name = strings.ReplaceAll(name, "ApiToken", "APIToken")
	return strings.TrimSuffix(name, "DTO")
}

func addOperation(reflector *openapi3.Reflector, r route) error {
	oc, err := reflector.NewOperationContext(r.method, r.path)
	if err != nil {
		return err
	}
	oc.SetTags(r.tag)
	oc.SetSummary(r.summary)
	if r.description != "" {
		oc.SetDescription(r.description)
	}
	oc.SetID(operationID(r))
	if r.access != accessPublic {
		oc.AddSecurity(bearerScheme)
	}
	if r.req != nil {
		oc.AddReqStructure(r.req, openapi.WithContentType(r.reqType))
	}

	oc.AddRespStructure(r.resp, openapi.WithHTTPStatus(r.status), openapi.WithContentType(r.contentType))

	for _, status := range r.errorStatuses() {
		oc.AddRespStructure(new(errorResponse), openapi.WithHTTPStatus(status))
	}
	return reflector.AddOperation(oc)
}

func operationID(r route) string {
	path := strings.TrimPrefix(r.path, "/api/v1/")
	replacer := strings.NewReplacer("/", "-", "{", "", "}", "")
	return strings.ToLower(r.method) + "-" + replacer.Replace(path)
}

func MarshalOpenAPIYAML() ([]byte, error) {
	spec, err := OpenAPISpec()
	if err != nil {
		return nil, err
	}
	return spec.MarshalYAML()
}

const ardPath = "/.well-known/ard.json"

type ardEntry struct {
	Identifier            string   `json:"identifier"`
	DisplayName           string   `json:"displayName"`
	Type                  string   `json:"type"`
	URL                   string   `json:"url"`
	Description           string   `json:"description"`
	RepresentativeQueries []string `json:"representativeQueries"`
}

// ardManifest lists the API and the public directory for AI agents (agenticresourcediscovery.org), at the origin asked.
func ardManifest(w http.ResponseWriter, r *http.Request) {
	origin := "http://" + r.Host
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		origin = "https://" + r.Host
	}
	urn := "urn:air:" + (&url.URL{Host: r.Host}).Hostname() + ":boreas:"
	writeJSON(w, http.StatusOK, map[string][]ardEntry{"entries": {
		{
			Identifier: urn + "api", DisplayName: "Boreas API", Type: "application/vnd.oai.openapi+json",
			URL:         origin + "/api/v1/openapi.json",
			Description: "Run one staging environment per task: create and deploy tasks, follow their builds and assign people.",
			RepresentativeQueries: []string{
				"deploy a staging environment for a task", "list the tasks in a project", "report a CI build's progress for a task",
			},
		},
		{
			Identifier: urn + "directory", DisplayName: "Boreas public directory", Type: "application/json",
			URL:         origin + "/api/v1/public/projects",
			Description: "Every task on this server with its project, people, build and deploy, readable without a token.",
			RepresentativeQueries: []string{
				"which staging environments are running", "who is working on a task", "is a task's build passing",
			},
		},
	}})
}

func (h *Handler) openapiJSON(w http.ResponseWriter, _ *http.Request) {
	spec, err := OpenAPISpec()
	h.reply(w, http.StatusOK, spec, err)
}

const docsPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Boreas API</title>
</head>
<body>
<div id="docs"></div>
<script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
<script>
Scalar.createApiReference("#docs", {
  url: "/api/v1/openapi.json",
  persistAuth: true,
  preferredSecurityScheme: "` + bearerScheme + `",
});
</script>
</body>
</html>
`

func (h *Handler) docsPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(docsPage))
}
