package resourcemanager

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func newTestAPI(t *testing.T) *API {
	t.Helper()
	// The store is built directly on a temp path rather than constructed and
	// then redirected: NewAPI loads from ~/.minisky immediately, so a test that
	// reassigned the path afterwards would already have read whatever hierarchy
	// the developer's own emulator holds, and pass or fail by accident.
	return &API{
		store:      newStoreAt(filepath.Join(t.TempDir(), "resource_hierarchy.json")),
		operations: map[string]map[string]interface{}{},
	}
}

func call(t *testing.T, api *API, method, path, body string) (int, map[string]interface{}) {
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

// operationResponse unwraps the resource from a completed long-running
// operation, which is what folder and project mutations return.
func operationResponse(t *testing.T, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	if body["done"] != true {
		t.Fatalf("expected a completed operation, got %v", body)
	}
	response, ok := body["response"].(map[string]interface{})
	if !ok {
		t.Fatalf("operation carried no response: %v", body)
	}
	return response
}

func seededOrg(t *testing.T, api *API) string {
	t.Helper()
	api.store.mu.RLock()
	defer api.store.mu.RUnlock()
	for name := range api.store.Organizations {
		return name
	}
	t.Fatal("no organization was seeded")
	return ""
}

// A hierarchy needs a root, and organizations cannot be created through this
// API in Google Cloud either.
func TestOrganizationIsSeeded(t *testing.T) {
	api := newTestAPI(t)
	org := seededOrg(t, api)

	code, body := call(t, api, http.MethodGet, "/v3/"+org, "")
	if code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", org, code)
	}
	if body["state"] != StateActive {
		t.Errorf("v3 renders state, got %v", body)
	}

	// v1 spells the same field lifecycleState.
	_, v1 := call(t, api, http.MethodGet, "/v1/"+org, "")
	if v1["lifecycleState"] != StateActive {
		t.Errorf("v1 renders lifecycleState, got %v", v1)
	}
	if _, leaked := v1["state"]; leaked {
		t.Error("v1 must not carry the v3 spelling")
	}
}

func TestOrganizationCannotBeCreated(t *testing.T) {
	api := newTestAPI(t)
	if code, _ := call(t, api, http.MethodPost, "/v3/organizations", `{"displayName":"x"}`); code == http.StatusOK {
		t.Error("organizations must not be creatable through this API")
	}
}

// Google assigns the folder id; the caller supplies only a display name.
func TestCreateFolderAssignsAnID(t *testing.T) {
	api := newTestAPI(t)
	org := seededOrg(t, api)

	code, body := call(t, api, http.MethodPost, "/v3/folders",
		`{"displayName":"Innovation","parent":"`+org+`"}`)
	if code != http.StatusOK {
		t.Fatalf("create = %d, want 200: %v", code, body)
	}

	folder := operationResponse(t, body)
	name, _ := folder["name"].(string)
	if !strings.HasPrefix(name, "folders/") || len(name) <= len("folders/") {
		t.Errorf("name = %q, want folders/{id}", name)
	}
	if folder["displayName"] != "Innovation" || folder["parent"] != org || folder["state"] != StateActive {
		t.Errorf("folder = %v", folder)
	}
}

func TestFolderValidation(t *testing.T) {
	api := newTestAPI(t)
	org := seededOrg(t, api)

	cases := []struct {
		name, body string
		wantCode   int
	}{
		{"missing parent", `{"displayName":"Orphan"}`, http.StatusBadRequest},
		{"unknown parent", `{"displayName":"Lost","parent":"folders/999999999999"}`, http.StatusNotFound},
		{"display name too short", `{"displayName":"a","parent":"` + org + `"}`, http.StatusBadRequest},
		{"illegal characters", `{"displayName":"bad/name","parent":"` + org + `"}`, http.StatusBadRequest},
	}
	for _, tc := range cases {
		if code, _ := call(t, api, http.MethodPost, "/v3/folders", tc.body); code != tc.wantCode {
			t.Errorf("%s: got %d, want %d", tc.name, code, tc.wantCode)
		}
	}
}

// Google rejects two folders with the same display name under one parent.
func TestSiblingFolderNamesMustBeUnique(t *testing.T) {
	api := newTestAPI(t)
	org := seededOrg(t, api)

	body := `{"displayName":"Shared-Services","parent":"` + org + `"}`
	if code, _ := call(t, api, http.MethodPost, "/v3/folders", body); code != http.StatusOK {
		t.Fatal("first create should succeed")
	}
	if code, _ := call(t, api, http.MethodPost, "/v3/folders", body); code != http.StatusConflict {
		t.Errorf("duplicate sibling = %d, want 409", code)
	}
}

// The whole point of the hierarchy: folders nest, and list is scoped to a parent.
func TestFoldersNestAndList(t *testing.T) {
	api := newTestAPI(t)
	org := seededOrg(t, api)

	_, created := call(t, api, http.MethodPost, "/v3/folders",
		`{"displayName":"Innovation","parent":"`+org+`"}`)
	innovation := operationResponse(t, created)["name"].(string)

	for _, child := range []string{"Initiative-A", "Initiative-B"} {
		if code, _ := call(t, api, http.MethodPost, "/v3/folders",
			`{"displayName":"`+child+`","parent":"`+innovation+`"}`); code != http.StatusOK {
			t.Fatalf("creating %s failed", child)
		}
	}

	_, listed := call(t, api, http.MethodGet, "/v3/folders?parent="+innovation, "")
	folders, _ := listed["folders"].([]interface{})
	if len(folders) != 2 {
		t.Fatalf("listed %d folders under Innovation, want 2", len(folders))
	}

	// The organization has one direct child, not three.
	_, top := call(t, api, http.MethodGet, "/v3/folders?parent="+org, "")
	if roots, _ := top["folders"].([]interface{}); len(roots) != 1 {
		t.Errorf("listed %d folders under the org, want 1 — list is scoped to a parent", len(roots))
	}
}

func TestFolderListRequiresAParent(t *testing.T) {
	api := newTestAPI(t)
	if code, _ := call(t, api, http.MethodGet, "/v3/folders", ""); code != http.StatusBadRequest {
		t.Errorf("list without a parent = %d, want 400", code)
	}
}

// A folder changes parent through folders.move, and may not be moved beneath
// itself.
func TestMoveFolder(t *testing.T) {
	api := newTestAPI(t)
	org := seededOrg(t, api)

	_, a := call(t, api, http.MethodPost, "/v3/folders", `{"displayName":"Platform","parent":"`+org+`"}`)
	platform := operationResponse(t, a)["name"].(string)
	_, b := call(t, api, http.MethodPost, "/v3/folders", `{"displayName":"Shared","parent":"`+org+`"}`)
	shared := operationResponse(t, b)["name"].(string)

	code, moved := call(t, api, http.MethodPost, "/v3/"+shared+":move",
		`{"destinationParent":"`+platform+`"}`)
	if code != http.StatusOK {
		t.Fatalf("move = %d: %v", code, moved)
	}
	if operationResponse(t, moved)["parent"] != platform {
		t.Errorf("parent was not updated: %v", moved)
	}

	// Moving a folder under its own descendant would make a cycle.
	if code, _ := call(t, api, http.MethodPost, "/v3/"+platform+":move",
		`{"destinationParent":"`+shared+`"}`); code != http.StatusBadRequest {
		t.Errorf("cyclic move = %d, want 400", code)
	}
}

// Deleting marks DELETE_REQUESTED rather than removing, and is refused while
// the folder still holds anything.
func TestDeleteFolderRequiresItToBeEmpty(t *testing.T) {
	api := newTestAPI(t)
	org := seededOrg(t, api)

	_, parent := call(t, api, http.MethodPost, "/v3/folders", `{"displayName":"Parent","parent":"`+org+`"}`)
	parentName := operationResponse(t, parent)["name"].(string)
	_, child := call(t, api, http.MethodPost, "/v3/folders", `{"displayName":"Child","parent":"`+parentName+`"}`)
	childName := operationResponse(t, child)["name"].(string)

	if code, _ := call(t, api, http.MethodDelete, "/v3/"+parentName, ""); code != http.StatusBadRequest {
		t.Error("deleting a folder with children should be refused")
	}

	code, deleted := call(t, api, http.MethodDelete, "/v3/"+childName, "")
	if code != http.StatusOK {
		t.Fatalf("delete = %d", code)
	}
	if operationResponse(t, deleted)["state"] != StateDeleteRequested {
		t.Error("delete should mark DELETE_REQUESTED, not remove")
	}

	// It disappears from a normal list but can be undeleted.
	_, listed := call(t, api, http.MethodGet, "/v3/folders?parent="+parentName, "")
	if folders, _ := listed["folders"].([]interface{}); len(folders) != 0 {
		t.Error("a deleted folder should not be listed by default")
	}
	if code, _ := call(t, api, http.MethodPost, "/v3/"+childName+":undelete", ""); code != http.StatusOK {
		t.Error("undelete should restore the folder")
	}
}

// Terraform's google_project speaks v1, which sends the parent as an object and
// calls the display name "name". v3 sends a string parent and "displayName".
// One store serves both, so a project created through either is visible and
// correctly shaped through the other.
func TestProjectCreateAcceptsBothParentShapes(t *testing.T) {
	api := newTestAPI(t)
	org := seededOrg(t, api)
	orgID := strings.TrimPrefix(org, "organizations/")

	_, folderOp := call(t, api, http.MethodPost, "/v3/folders",
		`{"displayName":"Initiative-A","parent":"`+org+`"}`)
	folder := operationResponse(t, folderOp)["name"].(string)
	folderID := strings.TrimPrefix(folder, "folders/")

	// v1: parent is {"type":"folder","id":"..."}
	code, body := call(t, api, http.MethodPost, "/v1/projects",
		`{"projectId":"initiative-a-dev","name":"Dev","parent":{"type":"folder","id":"`+folderID+`"}}`)
	if code != http.StatusOK {
		t.Fatalf("v1 create = %d: %v", code, body)
	}
	created := operationResponse(t, body)
	if created["lifecycleState"] != StateActive {
		t.Errorf("v1 renders lifecycleState, got %v", created)
	}
	if created["projectNumber"] == nil || created["projectNumber"] == "" {
		t.Error("a project must carry the number Google assigns")
	}

	// v3: parent is "folders/{id}"
	if code, body := call(t, api, http.MethodPost, "/v3/projects",
		`{"projectId":"initiative-a-prod","displayName":"Prod","parent":"`+folder+`"}`); code != http.StatusOK {
		t.Fatalf("v3 create = %d: %v", code, body)
	}

	// The v1 project reads back through v3 with v3's field names.
	_, v3 := call(t, api, http.MethodGet, "/v3/projects/initiative-a-dev", "")
	if v3["state"] != StateActive || v3["parent"] != folder {
		t.Errorf("v3 view = %v", v3)
	}
	if !strings.HasPrefix(v3["name"].(string), "projects/") {
		t.Errorf("v3 names a project projects/{number}, got %v", v3["name"])
	}

	// And an organization parent works in v1's object form too.
	if code, _ := call(t, api, http.MethodPost, "/v1/projects",
		`{"projectId":"platform-shared","parent":{"type":"organization","id":"`+orgID+`"}}`); code != http.StatusOK {
		t.Errorf("organization parent in v1 form = %d", code)
	}
}

func TestProjectValidation(t *testing.T) {
	api := newTestAPI(t)

	cases := []struct {
		name, body string
		wantCode   int
	}{
		{"too short", `{"projectId":"ab"}`, http.StatusBadRequest},
		{"uppercase", `{"projectId":"MyProject"}`, http.StatusBadRequest},
		{"trailing hyphen", `{"projectId":"my-project-"}`, http.StatusBadRequest},
		{"unknown parent", `{"projectId":"valid-project","parent":"folders/999999999999"}`, http.StatusNotFound},
	}
	for _, tc := range cases {
		if code, _ := call(t, api, http.MethodPost, "/v1/projects", tc.body); code != tc.wantCode {
			t.Errorf("%s: got %d, want %d", tc.name, code, tc.wantCode)
		}
	}

	// A duplicate project id is a conflict, as it is globally unique in GCP.
	call(t, api, http.MethodPost, "/v1/projects", `{"projectId":"unique-project"}`)
	if code, _ := call(t, api, http.MethodPost, "/v1/projects", `{"projectId":"unique-project"}`); code != http.StatusConflict {
		t.Error("a duplicate project id should be 409")
	}
}

// A project is addressable by the id its creator chose and by the number Google
// assigned, which is how v3 names it.
func TestProjectResolvesByIDAndNumber(t *testing.T) {
	api := newTestAPI(t)

	_, body := call(t, api, http.MethodPost, "/v1/projects", `{"projectId":"lookup-test"}`)
	number := operationResponse(t, body)["projectNumber"].(string)

	if code, _ := call(t, api, http.MethodGet, "/v1/projects/lookup-test", ""); code != http.StatusOK {
		t.Error("lookup by project id failed")
	}
	if code, _ := call(t, api, http.MethodGet, "/v3/projects/"+number, ""); code != http.StatusOK {
		t.Error("lookup by project number failed")
	}
}

// Deleting a project marks it DELETE_REQUESTED — Google keeps it recoverable
// for thirty days rather than removing it.
func TestProjectDeleteIsRecoverable(t *testing.T) {
	api := newTestAPI(t)
	call(t, api, http.MethodPost, "/v1/projects", `{"projectId":"doomed-project"}`)

	_, deleted := call(t, api, http.MethodDelete, "/v1/projects/doomed-project", "")
	if deleted["lifecycleState"] != StateDeleteRequested {
		t.Fatalf("delete should mark DELETE_REQUESTED, got %v", deleted)
	}

	_, listed := call(t, api, http.MethodGet, "/v1/projects", "")
	if projects, _ := listed["projects"].([]interface{}); len(projects) != 0 {
		t.Error("a deleted project should not be listed by default")
	}

	if code, _ := call(t, api, http.MethodPost, "/v1/projects/doomed-project:undelete", ""); code != http.StatusOK {
		t.Error("undelete should restore the project")
	}
	_, restored := call(t, api, http.MethodGet, "/v1/projects/doomed-project", "")
	if restored["lifecycleState"] != StateActive {
		t.Errorf("restored project = %v", restored)
	}
}

// A landing zone is folders plus the access granted on them, so policies have
// to attach to the hierarchy.
func TestIamPolicyOnFoldersAndProjects(t *testing.T) {
	api := newTestAPI(t)
	org := seededOrg(t, api)

	_, folderOp := call(t, api, http.MethodPost, "/v3/folders",
		`{"displayName":"Regulated","parent":"`+org+`"}`)
	folder := operationResponse(t, folderOp)["name"].(string)
	call(t, api, http.MethodPost, "/v1/projects", `{"projectId":"policy-target"}`)

	policy := `{"policy":{"bindings":[{"role":"roles/viewer","members":["user:ada@example.com"]}]}}`

	for _, target := range []string{"/v3/" + folder, "/v1/projects/policy-target"} {
		code, set := call(t, api, http.MethodPost, target+":setIamPolicy", policy)
		if code != http.StatusOK {
			t.Fatalf("setIamPolicy on %s = %d: %v", target, code, set)
		}
		if set["etag"] == nil || set["etag"] == "" {
			t.Errorf("a policy must come back with an etag: %v", set)
		}

		_, got := call(t, api, http.MethodPost, target+":getIamPolicy", "{}")
		bindings, _ := got["bindings"].([]interface{})
		if len(bindings) != 1 {
			t.Fatalf("getIamPolicy on %s returned %v", target, got)
		}
		binding := bindings[0].(map[string]interface{})
		if binding["role"] != "roles/viewer" {
			t.Errorf("binding = %v", binding)
		}
	}

	// A resource that does not exist has no policy to read or write.
	if code, _ := call(t, api, http.MethodPost, "/v3/folders/999999999999:getIamPolicy", "{}"); code != http.StatusNotFound {
		t.Error("a policy on a missing folder should be 404")
	}

	// An unset policy reads as empty rather than missing, as Google does.
	_, empty := call(t, api, http.MethodPost, "/v3/"+org+":getIamPolicy", "{}")
	if empty["bindings"] == nil {
		t.Errorf("an unset policy should read as an empty policy, got %v", empty)
	}
}

// The hierarchy has to survive a restart, or it cannot be used to test one.
func TestHierarchyPersists(t *testing.T) {
	api := newTestAPI(t)
	org := seededOrg(t, api)

	_, folderOp := call(t, api, http.MethodPost, "/v3/folders",
		`{"displayName":"Persistent","parent":"`+org+`"}`)
	folder := operationResponse(t, folderOp)["name"].(string)
	call(t, api, http.MethodPost, "/v1/projects", `{"projectId":"persistent-proj","parent":"`+folder+`"}`)

	// A second API over the same file is what a restart looks like.
	restarted := &API{store: newStoreAt(api.store.path), operations: map[string]map[string]interface{}{}}

	code, reread := call(t, restarted, http.MethodGet, "/v3/"+folder, "")
	if code != http.StatusOK {
		t.Fatalf("folder did not survive the restart: %d", code)
	}
	if reread["displayName"] != "Persistent" {
		t.Errorf("folder = %v", reread)
	}
	if code, _ := call(t, restarted, http.MethodGet, "/v1/projects/persistent-proj", ""); code != http.StatusOK {
		t.Error("project did not survive the restart")
	}
	if got := restarted.ListProjects(); len(got) != 1 || got[0] != "persistent-proj" {
		t.Errorf("ListProjects after restart = %v", got)
	}
}
