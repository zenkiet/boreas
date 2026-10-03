package httptransport

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/swaggest/openapi-go/openapi3"
)

func TestCommittedSpecIsCurrent(t *testing.T) {
	generated, err := MarshalOpenAPIYAML()
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if string(generated) != string(committed) {
		t.Fatal("api/openapi.yaml is stale; run 'make openapi'")
	}
}

func TestSpecCoversEveryRoute(t *testing.T) {
	spec, err := OpenAPISpec()
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]bool{}
	for _, r := range routeTable {
		item, ok := spec.Paths.MapOfPathItemValues[r.path]
		if !ok {
			t.Fatalf("%s is missing from the specification", r.path)
		}
		operation, ok := item.MapOfOperationValues[strings.ToLower(r.method)]
		if !ok {
			t.Fatalf("%s %s is missing from the specification", r.method, r.path)
		}
		if secured := len(operation.Security) > 0; secured != (r.access != accessPublic) {
			t.Fatalf("%s %s: security = %v, want %v", r.method, r.path, secured, r.access != accessPublic)
		}
		if _, ok := operation.Responses.MapOfResponseOrRefValues["401"]; !ok && r.access != accessPublic {
			t.Fatalf("%s %s: missing 401 documentation", r.method, r.path)
		}
		if _, ok := operation.Responses.MapOfResponseOrRefValues["500"]; !ok {
			t.Fatalf("%s %s: missing 500 documentation", r.method, r.path)
		}
		id := operationID(r)
		if ids[id] {
			t.Fatalf("%s %s reuses operation id %q", r.method, r.path, id)
		}
		ids[id] = true
	}
}

func TestSpecNeverExposesSecrets(t *testing.T) {
	spec, err := OpenAPISpec()
	if err != nil {
		t.Fatal(err)
	}
	document, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"password_hash", "PasswordHash", "TokenHash"} {
		if strings.Contains(string(document), forbidden) {
			t.Fatalf("%q appears in the specification", forbidden)
		}
	}

	schemas := spec.Components.Schemas.MapOfSchemaOrRefValues
	props := func(name string) map[string]openapi3.SchemaOrRef {
		if schemas[name].Schema == nil {
			t.Fatalf("%s schema is missing", name)
		}
		return schemas[name].Schema.Properties
	}
	if _, leaked := props("Credential")["token"]; leaked {
		t.Fatal("the credential response schema must not expose token")
	}
	if _, present := props("CreateCredentialRequest")["token"]; !present {
		t.Fatal("the credential request schema must accept token")
	}
	for _, forbidden := range []string{"token", "token_hash"} {
		if _, leaked := props("APIToken")[forbidden]; leaked {
			t.Fatalf("the API token metadata schema must not expose %s", forbidden)
		}
	}
	if token, present := props("CreateAPITokenResponse")["token"]; !present || token.Schema == nil || token.Schema.Format == nil || *token.Schema.Format != "password" {
		t.Fatal("the one-time API token response must document token as a password")
	}
}

func TestDocsEndpoints(t *testing.T) {
	h := APIHandler(stubTasks{}, &stubAuth{user: testAdmin}, &stubProjects{}, &stubPush{}, nil, "test", slog.New(slog.DiscardHandler))

	rr := do(h, httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("openapi.json status = %d", rr.Code)
	}
	var spec map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
		t.Fatalf("openapi.json is not valid JSON: %v", err)
	}
	if spec["openapi"] != "3.2.0" {
		t.Fatalf("openapi field = %v", spec["openapi"])
	}

	// The documentation page must load the served specification.
	rr = do(h, httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil))
	if body := rr.Body.String(); rr.Code != http.StatusOK || !strings.Contains(body, "createApiReference") || !strings.Contains(body, "/api/v1/openapi.json") {
		t.Fatalf("docs status=%d body=%q", rr.Code, body)
	}
}
