package appengine

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"minisky/pkg/orchestrator"
)

func newTestAPI() *API {
	return NewAPI(orchestrator.NewOperationManager(), nil, nil, nil)
}

func do(t *testing.T, api *API, method, path, body string) (int, map[string]interface{}) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)

	var decoded map[string]interface{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	}
	return rec.Code, decoded
}

// The Admin API addresses an application as apps/{appsId}; the dashboard calls
// the same handlers with projects/{projectId}. Both have to resolve, or one of
// the two callers breaks.
func TestApplicationIDAcceptsBothPathShapes(t *testing.T) {
	cases := map[string]string{
		"/v1/apps/my-project":                                   "my-project",
		"/v1/apps/my-project/services":                          "my-project",
		"/api/manage/appengine/projects/my-project/services":    "my-project",
		"/api/manage/appengine/projects/my-project/operations/": "my-project",
		"/v1/apps": "",
	}
	for path, want := range cases {
		if got := applicationID(path); got != want {
			t.Errorf("applicationID(%q) = %q, want %q", path, got, want)
		}
	}
}

// The application path must not capture the resources beneath it, which have
// their own handlers.
func TestIsApplicationPath(t *testing.T) {
	cases := map[string]bool{
		"/v1/apps":                           true,
		"/v1/apps/my-project":                true,
		"/v1/apps/my-project/services":       false,
		"/v1/apps/my-project/services/v1":    false,
		"/v1/apps/my-project/operations/op1": false,
	}
	for path, want := range cases {
		if got := isApplicationPath(path); got != want {
			t.Errorf("isApplicationPath(%q) = %v, want %v", path, got, want)
		}
	}
}

// Getting an application by id used to fall through to a blanket 404, because
// the dispatch only matched a path ending in "/apps".
func TestGetApplicationByID(t *testing.T) {
	api := newTestAPI()

	code, body := do(t, api, http.MethodGet, "/v1/apps/my-project", "")
	if code != http.StatusOK {
		t.Fatalf("GET /v1/apps/my-project = %d, want 200", code)
	}
	if body["id"] != "my-project" {
		t.Errorf("id = %v, want my-project", body["id"])
	}
	if body["defaultHostname"] != "my-project.appspot.com" {
		t.Errorf("defaultHostname = %v", body["defaultHostname"])
	}
}

// There is no list method, and auto-creating for an empty id would leave a
// nameless application in state.
func TestGetApplicationWithoutIDIsRejected(t *testing.T) {
	api := newTestAPI()

	code, body := do(t, api, http.MethodGet, "/v1/apps", "")
	if code != http.StatusBadRequest {
		t.Fatalf("GET /v1/apps = %d, want 400", code)
	}
	if errObj, ok := body["error"].(map[string]interface{}); !ok || errObj["status"] != "INVALID_ARGUMENT" {
		t.Errorf("expected an INVALID_ARGUMENT envelope, got %v", body)
	}

	api.mu.RLock()
	_, nameless := api.apps[""]
	api.mu.RUnlock()
	if nameless {
		t.Error("a nameless application was created")
	}
}

// apps.create previously answered 200 with an empty body and created nothing.
func TestCreateApplicationReturnsAnOperation(t *testing.T) {
	api := newTestAPI()

	code, body := do(t, api, http.MethodPost, "/v1/apps",
		`{"id":"new-project","locationId":"europe-west1"}`)
	if code != http.StatusOK {
		t.Fatalf("POST /v1/apps = %d, want 200", code)
	}
	if body["name"] == nil || body["kind"] != "appengine#operation" {
		t.Fatalf("expected a long-running operation, got %v", body)
	}
	if body["done"] != true {
		t.Errorf("done = %v; creation is a state write and completes immediately", body["done"])
	}

	// The application must actually exist afterwards, with the location asked for.
	code, app := do(t, api, http.MethodGet, "/v1/apps/new-project", "")
	if code != http.StatusOK {
		t.Fatalf("GET after create = %d, want 200", code)
	}
	if app["locationId"] != "europe-west1" {
		t.Errorf("locationId = %v, want europe-west1", app["locationId"])
	}
}

func TestCreateApplicationRejectsDuplicateAndMissingID(t *testing.T) {
	api := newTestAPI()

	if code, _ := do(t, api, http.MethodPost, "/v1/apps", `{"id":"dup"}`); code != http.StatusOK {
		t.Fatalf("first create = %d, want 200", code)
	}
	code, body := do(t, api, http.MethodPost, "/v1/apps", `{"id":"dup"}`)
	if code != http.StatusConflict {
		t.Errorf("second create = %d, want 409", code)
	}
	if errObj, ok := body["error"].(map[string]interface{}); !ok || errObj["status"] != "ALREADY_EXISTS" {
		t.Errorf("expected ALREADY_EXISTS, got %v", body)
	}

	if code, _ := do(t, api, http.MethodPost, "/v1/apps", `{}`); code != http.StatusBadRequest {
		t.Errorf("create without an id = %d, want 400", code)
	}
}

// Resources beneath the application must keep reaching their own handlers.
func TestNestedResourcesStillRoute(t *testing.T) {
	api := newTestAPI()

	if code, _ := do(t, api, http.MethodGet, "/v1/apps/my-project/services", ""); code == http.StatusBadRequest {
		t.Error("services was handled as an application path")
	}
	if code, _ := do(t, api, http.MethodGet, "/v1/apps/my-project/services/default/versions", ""); code == http.StatusBadRequest {
		t.Error("versions was handled as an application path")
	}
}
