package httptransport

import (
	"fmt"
	"net/http"
	"reflect"
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
	spec := reflector.SpecEns()
	spec.Info.
		WithTitle("Boreas API").
		WithVersion("1.0.0").
		WithDescription("Per-task staging environments. Tasks belong to projects and are " +
			"served at /{project}/{task}/. Proxied task traffic is public; this API is not.")
	spec.SetHTTPBearerTokenSecurity(bearerScheme, "",
		"Token from POST /api/v1/auth/login, sent as: Authorization: Bearer <token>")

	configureSchemas(reflector.JSONSchemaReflector())

	for _, r := range routeTable {
		if err := addOperation(reflector, r); err != nil {
			return nil, fmt.Errorf("%s %s: %w", r.method, r.path, err)
		}
	}
	return spec, nil
}

func configureSchemas(js *jsonschema.Reflector) {
	uuidSchema := jsonschema.Schema{}
	uuidSchema.AddType(jsonschema.String)
	uuidSchema.WithFormat("uuid")
	// uuid.UUID is [16]byte, which would otherwise reflect as an array.
	js.AddTypeMapping(uuid.UUID{}, uuidSchema)

	js.DefaultOptions = append(js.DefaultOptions,
		jsonschema.InterceptDefName(schemaName),
		jsonschema.InterceptProp(requiredFromOmitEmpty),
	)
}

// requiredFromOmitEmpty derives required fields from JSON tags so schema and serialization stay aligned.
func requiredFromOmitEmpty(params jsonschema.InterceptPropParams) error {
	if !params.Processed || params.Name == "" || params.ParentSchema == nil {
		return nil
	}
	tag, ok := params.Field.Tag.Lookup("json")
	if !ok || strings.Contains(tag, ",omitempty") {
		return nil
	}
	for _, existing := range params.ParentSchema.Required {
		if existing == params.Name {
			return nil
		}
	}
	params.ParentSchema.Required = append(params.ParentSchema.Required, params.Name)
	return nil
}

func schemaName(_ reflect.Type, defaultName string) string {
	name := defaultName
	for _, prefix := range []string{"Httptransport", "Http", "Core", "Uuid"} {
		name = strings.TrimPrefix(name, prefix)
	}
	if trimmed := strings.TrimSuffix(name, "DTO"); trimmed != "" {
		name = trimmed
	}
	return name
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
		oc.AddReqStructure(r.req)
	}

	respOptions := []openapi.ContentOption{openapi.WithHTTPStatus(r.status)}
	if r.contentType != "" {
		respOptions = append(respOptions, openapi.WithContentType(r.contentType))
	}
	oc.AddRespStructure(r.resp, respOptions...)

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

func (h *Handler) openapiJSON(w http.ResponseWriter, _ *http.Request) {
	spec, err := OpenAPISpec()
	if err != nil {
		writeServiceError(w, h.logger, err)
		return
	}
	writeJSON(w, http.StatusOK, spec)
}

// docsPage uses pinned CDN assets to keep the binary small at the cost of network access.
const docsPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Boreas API</title>
</head>
<body>
<div id="docs"></div>
<script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1"></script>
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
