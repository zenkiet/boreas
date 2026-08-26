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
	credential, ok := schemas["Credential"]
	if !ok {
		t.Fatal("Credential schema is missing")
	}
	if _, leaked := credential.Schema.Properties["token"]; leaked {
		t.Fatal("the credential response schema must not expose token")
	}
	request, ok := schemas["CreateCredentialRequest"]
	if !ok {
		t.Fatal("CreateCredentialRequest schema is missing")
	}
	if _, present := request.Schema.Properties["token"]; !present {
		t.Fatal("the credential request schema must accept token")
	}
	apiToken, ok := schemas["APIToken"]
	if !ok {
		t.Fatal("APIToken schema is missing")
	}
	for _, forbidden := range []string{"token", "token_hash"} {
		if _, leaked := apiToken.Schema.Properties[forbidden]; leaked {
			t.Fatalf("the API token metadata schema must not expose %s", forbidden)
		}
	}
	created, ok := schemas["CreateAPITokenResponse"]
	if !ok {
		t.Fatal("CreateAPITokenResponse schema is missing")
	}
	if token, present := created.Schema.Properties["token"]; !present || token.Schema == nil || token.Schema.Format == nil || *token.Schema.Format != "password" {
		t.Fatal("the one-time API token response must document token as a password")
	}
}

func TestSpecRoundTrips(t *testing.T) {
	generated, err := MarshalOpenAPIYAML()
	if err != nil {
		t.Fatal(err)
	}
	var parsed openapi3.Spec
	if err := parsed.UnmarshalYAML(generated); err != nil {
		t.Fatalf("generated specification does not parse: %v", err)
	}
	if parsed.Info.Title != "Boreas API" || len(parsed.Paths.MapOfPathItemValues) == 0 {
		t.Fatalf("unexpected round-tripped specification: %+v", parsed.Info)
	}
}

func TestDocsEndpoints(t *testing.T) {
	h := APIHandler(stubTasks{}, &stubAuth{user: testAdmin}, &stubProjects{}, &stubPush{}, slog.New(slog.DiscardHandler))

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

	rr = do(h, httptest.NewRequest(http.MethodGet, "/api/v1/docs", nil))
	body := rr.Body.String()
	if rr.Code != http.StatusOK || !strings.Contains(body, "createApiReference") {
		t.Fatalf("docs status=%d body=%q", rr.Code, body)
	}
	if !strings.Contains(body, "/api/v1/openapi.json") {
		t.Fatal("the documentation page must load the served specification")
	}
}

func TestOperationIDsAreUnique(t *testing.T) {
	seen := map[string]string{}
	for _, r := range routeTable {
		id := operationID(r)
		if previous, duplicate := seen[id]; duplicate {
			t.Fatalf("operation id %q is used by both %s and %s %s", id, previous, r.method, r.path)
		}
		seen[id] = r.method + " " + r.path
	}
}
