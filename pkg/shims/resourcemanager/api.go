package resourcemanager

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"minisky/pkg/registry"
)

func init() {
	registry.Register("cloudresourcemanager.googleapis.com", func(ctx *registry.Context) http.Handler {
		return NewAPI()
	})

	// Both generations are live at once because Terraform uses both:
	// google_folder speaks v3, google_project speaks v1.
	registry.RegisterRoutes("cloudresourcemanager.googleapis.com",
		"/v3/folders",
		"/v3/organizations",
		// A collection-level custom method carries the verb on the collection
		// segment itself, so "organizations:search" is its own literal rather
		// than something the "/v3/organizations" pattern reaches.
		"/v3/folders:search",
		"/v3/projects:search",
		"/v3/organizations:search",
		"/v1/organizations:search",
		// MiniSky's own policy analysis, used by the CLI and the dashboard.
		"/v1/internal/iam/analyze",
		// A folder operation is polled at {base}/v3/operations/{id} and a project
		// operation at {base}/v1/operations/{id}, so both generations serve them.
		"/v3/operations",
		"/v1/operations",
		"/v1/organizations",
		// A project path is claimed exactly. Resource Manager owns
		// /v1/projects and /v1/projects/{id}, but everything below that —
		// /v1/projects/{id}/secrets, /topics, /serviceAccounts — belongs to
		// another service, so the subtree must not come with it.
		"/v1/projects$",
		"/v1/projects/*$",
		"/v3/projects$",
		"/v3/projects/*$",
		// v1 addresses a project with custom methods on the project segment,
		// as /v1/projects/{id}:setIamPolicy.
		"/v1/projects/*:getIamPolicy",
		"/v1/projects/*:setIamPolicy",
		"/v1/projects/*:testIamPermissions",
		"/v1/projects/*:undelete",
	)
}

// API serves Cloud Resource Manager: the organization, folder and project
// hierarchy, and the IAM policies attached to it.
type API struct {
	store *store

	// operations records completed long-running operations so a client that
	// polls one finds it, even though every operation here finishes before the
	// response is written.
	opMu       sync.RWMutex
	operations map[string]map[string]interface{}
}

func NewAPI() *API {
	return &API{
		store:      newStore(),
		operations: map[string]map[string]interface{}{},
	}
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimSuffix(r.URL.Path, "/")
	log.Printf("[Shim: ResourceManager] %s %s", r.Method, path)
	w.Header().Set("Content-Type", "application/json")

	// A custom method is addressed as resource:verb, so it is split off before
	// the resource is resolved.
	resource, verb := splitCustomMethod(path)

	switch {
	case verb != "":
		api.routeCustomMethod(w, r, resource, verb)
	case strings.HasPrefix(path, "/v3/folders"):
		api.routeFolders(w, r, path)
	case strings.HasPrefix(path, "/v3/projects"), strings.HasPrefix(path, "/v1/projects"):
		api.routeProjects(w, r, path)
	case strings.HasPrefix(path, "/v3/organizations"), strings.HasPrefix(path, "/v1/organizations"):
		api.routeOrganizations(w, r, path)
	case strings.HasPrefix(path, "/v3/operations"), strings.HasPrefix(path, "/v1/operations"):
		api.getOperation(w, path)
	case path == "/v1/internal/iam/analyze":
		api.handleAnalyze(w, r)
	default:
		writeError(w, http.StatusNotFound, "NOT_FOUND", "unknown Resource Manager path "+path)
	}
}

// routeCustomMethod dispatches resource:verb calls.
func (api *API) routeCustomMethod(w http.ResponseWriter, r *http.Request, resource, verb string) {
	switch verb {
	case "getIamPolicy":
		api.getIamPolicy(w, r, resource)
	case "setIamPolicy":
		api.setIamPolicy(w, r, resource)
	case "testIamPermissions":
		api.testIamPermissions(w, r, resource)
	case "move":
		api.moveFolder(w, r, resource)
	case "undelete":
		api.undelete(w, r, resource)
	case "search":
		api.search(w, r, resource)
	default:
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "unsupported method :"+verb)
	}
}

// search backs folders:search, projects:search and organizations:search. The
// emulator holds few enough resources that returning everything active is both
// correct and simpler than implementing the query language.
func (api *API) search(w http.ResponseWriter, r *http.Request, resource string) {
	api.store.mu.RLock()
	defer api.store.mu.RUnlock()

	switch {
	case strings.HasSuffix(resource, "/folders"):
		folders := []*Folder{}
		for _, f := range api.store.Folders {
			if f.State == StateActive {
				folders = append(folders, f)
			}
		}
		sort.Slice(folders, func(i, j int) bool { return folders[i].Name < folders[j].Name })
		writeJSON(w, map[string]interface{}{"folders": folders})

	case strings.HasSuffix(resource, "/organizations"):
		orgs := []*Organization{}
		for _, o := range api.store.Organizations {
			orgs = append(orgs, api.renderOrganization(o, apiVersion(resource)))
		}
		writeJSON(w, map[string]interface{}{"organizations": orgs})

	case strings.HasSuffix(resource, "/projects"):
		version := apiVersion(resource)
		items := []interface{}{}
		for _, p := range activeProjects(api.store.Projects) {
			items = append(items, renderProject(p, version))
		}
		writeJSON(w, map[string]interface{}{"projects": items})

	default:
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "search is not supported on "+resource)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Organizations
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) routeOrganizations(w http.ResponseWriter, r *http.Request, path string) {
	version := apiVersion(path)
	id := resourceIDFrom(path, "organizations")

	if id == "" {
		// The collection itself is read-only. Listing is organizations:search,
		// which arrives as a custom method; a bare GET is accepted as the same
		// thing, and nothing else is allowed — an organization comes from Cloud
		// Identity, not from this API.
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "FAILED_PRECONDITION",
				"an organization cannot be created or changed through this API")
			return
		}
		api.search(w, r, path)
		return
	}

	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "FAILED_PRECONDITION",
			"an organization cannot be created or changed through this API")
		return
	}

	api.store.mu.RLock()
	org, ok := api.store.Organizations["organizations/"+id]
	api.store.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "organizations/"+id+" not found")
		return
	}
	writeJSON(w, api.renderOrganization(org, version))
}

// renderOrganization emits the field spellings each version uses: v1 says
// lifecycleState and creationTime, v3 says state and createTime.
func (api *API) renderOrganization(org *Organization, version string) *Organization {
	out := *org
	if version == "v1" {
		out.State = ""
		out.CreateTime = ""
	} else {
		out.LifecycleState = ""
		out.CreationTime = ""
	}
	return &out
}

// ─────────────────────────────────────────────────────────────────────────────
// Operations
// ─────────────────────────────────────────────────────────────────────────────

// completedOperation records and returns a finished long-running operation.
//
// Every mutation here is a state write that completes before the response is
// serialised, so the operation is returned already done with its result
// embedded — which the API permits and clients handle. It is still recorded, so
// a client that polls anyway finds it rather than a 404.
func (api *API) completedOperation(response interface{}) map[string]interface{} {
	name := "operations/cp." + randomDigits(16)
	op := map[string]interface{}{
		"name":     name,
		"done":     true,
		"response": response,
	}

	api.opMu.Lock()
	api.operations[name] = op
	api.opMu.Unlock()
	return op
}

func (api *API) getOperation(w http.ResponseWriter, path string) {
	name := trimVersion(path)

	api.opMu.RLock()
	op, ok := api.operations[name]
	api.opMu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", name+" not found")
		return
	}
	writeJSON(w, op)
}

// ─────────────────────────────────────────────────────────────────────────────
// Project discovery
// ─────────────────────────────────────────────────────────────────────────────

// ListProjects lets the dashboard show declared projects alongside the ones it
// infers from resources other shims hold.
func (api *API) ListProjects() []string {
	api.store.mu.RLock()
	defer api.store.mu.RUnlock()

	ids := []string{}
	for _, p := range api.store.Projects {
		if p.State == StateActive {
			ids = append(ids, p.ProjectID)
		}
	}
	sort.Strings(ids)
	return ids
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// splitCustomMethod separates "resource:verb" into its parts. Only a colon in
// the final segment is a method; one earlier in the path is part of a name.
func splitCustomMethod(path string) (resource, verb string) {
	slash := strings.LastIndex(path, "/")
	colon := strings.LastIndex(path, ":")
	if colon < 0 || colon < slash {
		return path, ""
	}
	return path[:colon], path[colon+1:]
}

// apiVersion reports which generation a path belongs to, since the two render
// the same resource differently.
func apiVersion(path string) string {
	if strings.HasPrefix(path, "/v1/") {
		return "v1"
	}
	return "v3"
}

// resourceIDFrom returns the id following a collection segment, or "" when the
// path addresses the collection itself.
func resourceIDFrom(path, collection string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	for i, part := range parts {
		if part == collection && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func activeProjects(all map[string]*project) []*project {
	out := []*project{}
	for _, p := range all {
		if p.State == StateActive {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ProjectID < out[j].ProjectID })
	return out
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }

func writeJSON(w http.ResponseWriter, body interface{}) {
	_ = json.NewEncoder(w).Encode(body)
}

// writeError emits the envelope every Google API uses for failures.
func writeError(w http.ResponseWriter, code int, status, message string) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"code":    code,
			"message": message,
			"status":  status,
		},
	})
}
