package resourcemanager

import (
	"net/http"
	"strings"
	"testing"
)

// buildHierarchy makes org → Innovation → Initiative-A → a project, which is the
// shape a landing zone actually has and the shape inheritance matters in.
func buildHierarchy(t *testing.T, api *API) (org, parent, child, projectID string) {
	t.Helper()
	org = seededOrg(t, api)

	_, a := call(t, api, http.MethodPost, "/v3/folders",
		`{"displayName":"Innovation","parent":"`+org+`"}`)
	parent = operationResponse(t, a)["name"].(string)

	_, b := call(t, api, http.MethodPost, "/v3/folders",
		`{"displayName":"Initiative-A","parent":"`+parent+`"}`)
	child = operationResponse(t, b)["name"].(string)

	projectID = "initiative-a-dev"
	call(t, api, http.MethodPost, "/v1/projects",
		`{"projectId":"`+projectID+`","parent":"`+child+`"}`)
	return org, parent, child, projectID
}

// Ancestry is the spine of the whole thing: a project inherits from every
// folder above it and from the organization.
func TestAncestryWalksToTheOrganization(t *testing.T) {
	api := newTestAPI(t)
	org, parent, child, projectID := buildHierarchy(t, api)

	got := api.ancestryOf("projects/" + projectID)
	want := []string{"projects/" + projectID, child, parent, org}

	if len(got) != len(want) {
		t.Fatalf("ancestry = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ancestry[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// The reason this feature exists: a binding on the organization silently
// reaches every project beneath it, which is how a landing zone grants more
// than its author intended.
func TestAccessIsInheritedDownTheHierarchy(t *testing.T) {
	api := newTestAPI(t)
	org, parent, _, projectID := buildHierarchy(t, api)

	call(t, api, http.MethodPost, "/v3/"+org+":setIamPolicy",
		`{"policy":{"bindings":[{"role":"roles/viewer","members":["user:auditor@example.com"]}]}}`)
	call(t, api, http.MethodPost, "/v3/"+parent+":setIamPolicy",
		`{"policy":{"bindings":[{"role":"roles/compute.admin","members":["user:ada@example.com"]}]}}`)

	// The auditor holds nothing directly on the project, yet has viewer there.
	auditor := api.Explain("user:auditor@example.com", "projects/"+projectID, "")
	if len(auditor.Roles) != 1 {
		t.Fatalf("auditor roles = %v", auditor.Roles)
	}
	if auditor.Roles[0].Role != "roles/viewer" || !auditor.Roles[0].Inherited {
		t.Errorf("expected roles/viewer inherited, got %+v", auditor.Roles[0])
	}
	if auditor.Roles[0].GrantedOn != org {
		t.Errorf("granted on %q, want the organization", auditor.Roles[0].GrantedOn)
	}

	// And Ada's folder binding reaches the project two levels down.
	ada := api.Explain("user:ada@example.com", "projects/"+projectID, "compute.instances.create")
	if ada.Granted == nil || !*ada.Granted {
		t.Fatalf("compute.admin on the folder should reach the project: %+v", ada)
	}
	if len(ada.GrantedBy) == 0 || !strings.Contains(ada.GrantedBy[0], parent) {
		t.Errorf("grantedBy = %v, want the folder named", ada.GrantedBy)
	}

	// Someone with no binding anywhere holds nothing.
	nobody := api.Explain("user:nobody@example.com", "projects/"+projectID, "compute.instances.create")
	if nobody.Granted == nil || *nobody.Granted {
		t.Errorf("an unbound principal must hold nothing: %+v", nobody)
	}
}

// A binding lower down does not leak upward.
func TestAccessIsNotInheritedUpward(t *testing.T) {
	api := newTestAPI(t)
	org, parent, child, _ := buildHierarchy(t, api)

	call(t, api, http.MethodPost, "/v3/"+child+":setIamPolicy",
		`{"policy":{"bindings":[{"role":"roles/owner","members":["user:dev@example.com"]}]}}`)

	for _, above := range []string{parent, org} {
		if held := api.Explain("user:dev@example.com", above, ""); len(held.Roles) != 0 {
			t.Errorf("a binding on %s leaked up to %s: %v", child, above, held.Roles)
		}
	}
}

// The basic roles nest, so owner has to carry what viewer does.
func TestBasicRolesNest(t *testing.T) {
	api := newTestAPI(t)
	_, _, _, projectID := buildHierarchy(t, api)

	call(t, api, http.MethodPost, "/v1/projects/"+projectID+":setIamPolicy",
		`{"policy":{"bindings":[{"role":"roles/owner","members":["user:boss@example.com"]}]}}`)

	// A permission that lives on viewer, held through owner → editor → viewer.
	held := api.Explain("user:boss@example.com", "projects/"+projectID, "resourcemanager.projects.get")
	if held.Granted == nil || !*held.Granted {
		t.Errorf("owner should include viewer's permissions: %+v", held)
	}
}

// A wildcard member covers everyone, which is exactly the binding a review
// needs to surface.
func TestWildcardAndDomainMembers(t *testing.T) {
	api := newTestAPI(t)
	_, _, _, projectID := buildHierarchy(t, api)

	call(t, api, http.MethodPost, "/v1/projects/"+projectID+":setIamPolicy",
		`{"policy":{"bindings":[
			{"role":"roles/storage.objectViewer","members":["allUsers"]},
			{"role":"roles/bigquery.jobUser","members":["domain:example.com"]}
		]}}`)

	anyone := api.Explain("user:stranger@elsewhere.com", "projects/"+projectID, "storage.objects.get")
	if anyone.Granted == nil || !*anyone.Granted {
		t.Error("allUsers must cover any principal")
	}

	insider := api.Explain("user:ada@example.com", "projects/"+projectID, "bigquery.jobs.create")
	if insider.Granted == nil || !*insider.Granted {
		t.Error("domain: must cover a principal at that domain")
	}
	outsider := api.Explain("user:ada@other.com", "projects/"+projectID, "bigquery.jobs.create")
	if outsider.Granted != nil && *outsider.Granted {
		// allUsers above still grants storage, but not bigquery.
		t.Error("domain: must not cover a principal at another domain")
	}
}

// An answer drawn from a partial catalogue must say so, rather than reading as
// a denial.
func TestUnknownRoleIsReportedNotAssumedEmpty(t *testing.T) {
	api := newTestAPI(t)
	_, _, _, projectID := buildHierarchy(t, api)

	call(t, api, http.MethodPost, "/v1/projects/"+projectID+":setIamPolicy",
		`{"policy":{"bindings":[{"role":"roles/dataproc.hubAgent","members":["user:ada@example.com"]}]}}`)

	held := api.Explain("user:ada@example.com", "projects/"+projectID, "dataproc.clusters.create")
	if len(held.Roles) != 1 || held.Roles[0].RoleKnown {
		t.Fatalf("the role should be held but flagged unknown: %+v", held.Roles)
	}
	if len(held.UnknownRole) == 0 || held.Caveat == "" {
		t.Errorf("an incomplete answer must carry a caveat: %+v", held)
	}
}

// The question a review starts from: who can do this here.
func TestPrincipalsWithAPermission(t *testing.T) {
	api := newTestAPI(t)
	org, parent, _, projectID := buildHierarchy(t, api)

	call(t, api, http.MethodPost, "/v3/"+org+":setIamPolicy",
		`{"policy":{"bindings":[{"role":"roles/owner","members":["user:boss@example.com"]}]}}`)
	call(t, api, http.MethodPost, "/v3/"+parent+":setIamPolicy",
		`{"policy":{"bindings":[{"role":"roles/storage.admin","members":["serviceAccount:ci@example.com"]}]}}`)

	who := api.PrincipalsWith("projects/"+projectID, "storage.objects.delete")
	if len(who) != 2 {
		t.Fatalf("principals = %v, want the owner and the service account", who)
	}
}

func TestAnalyzeEndpoint(t *testing.T) {
	api := newTestAPI(t)
	_, _, _, projectID := buildHierarchy(t, api)
	call(t, api, http.MethodPost, "/v1/projects/"+projectID+":setIamPolicy",
		`{"policy":{"bindings":[{"role":"roles/viewer","members":["user:ada@example.com"]}]}}`)

	code, body := call(t, api, http.MethodPost, "/v1/internal/iam/analyze",
		`{"principal":"user:ada@example.com","resource":"projects/`+projectID+`","permission":"resourcemanager.projects.get"}`)
	if code != http.StatusOK {
		t.Fatalf("analyze = %d: %v", code, body)
	}
	if body["granted"] != true {
		t.Errorf("granted = %v, want true", body["granted"])
	}

	// A resource is always required.
	if code, _ := call(t, api, http.MethodPost, "/v1/internal/iam/analyze", `{"principal":"user:x"}`); code != http.StatusBadRequest {
		t.Error("a request without a resource should be rejected")
	}
}
