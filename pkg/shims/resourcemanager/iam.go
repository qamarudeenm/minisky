package resourcemanager

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// IAM on the hierarchy
//
// A landing zone is folders plus the access granted on them, so a hierarchy
// without IAM does not test the thing it exists to test. Cloud Resource Manager
// serves the policies for its own resources — organizations, folders and
// projects — which is why these live here rather than in the IAM shim, whose
// policies belong to service accounts.
//
// Policies are stored but not enforced: MiniSky accepts any caller, so a
// binding records intent for the configuration under test rather than gating
// requests.
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) getIamPolicy(w http.ResponseWriter, r *http.Request, resource string) {
	name := trimVersion(resource)
	if !api.policyResourceExists(name) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", name+" not found")
		return
	}

	api.store.mu.RLock()
	policy, ok := api.store.Policies[name]
	api.store.mu.RUnlock()

	if !ok {
		// Google returns an empty policy rather than a 404 for a resource that
		// has never had one set.
		policy = &IamPolicy{Version: 1, Bindings: []IamBinding{}, Etag: newEtag()}
	}
	writeJSON(w, policy)
}

func (api *API) setIamPolicy(w http.ResponseWriter, r *http.Request, resource string) {
	name := trimVersion(resource)
	if !api.policyResourceExists(name) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", name+" not found")
		return
	}

	var body struct {
		Policy IamPolicy `json:"policy"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: "+err.Error())
		return
	}

	policy := body.Policy
	if policy.Version == 0 {
		policy.Version = 1
	}
	if policy.Bindings == nil {
		policy.Bindings = []IamBinding{}
	}
	// Google returns the policy with a fresh etag, which callers send back on
	// the next write to detect a concurrent change.
	policy.Etag = newEtag()

	sort.Slice(policy.Bindings, func(i, j int) bool {
		return policy.Bindings[i].Role < policy.Bindings[j].Role
	})

	api.store.mu.Lock()
	api.store.Policies[name] = &policy
	api.store.save()
	api.store.mu.Unlock()

	writeJSON(w, policy)
}

// testIamPermissions reports which of the requested permissions the caller
// holds. MiniSky authenticates nobody, so every permission asked for is
// granted — stated plainly rather than implied, because a configuration that
// depends on this answer will behave differently against real IAM.
func (api *API) testIamPermissions(w http.ResponseWriter, r *http.Request, resource string) {
	var body struct {
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: "+err.Error())
		return
	}
	writeJSON(w, map[string]interface{}{"permissions": body.Permissions})
}

// policyResourceExists reports whether a policy may be attached to name.
// Callers must not hold the lock.
func (api *API) policyResourceExists(name string) bool {
	api.store.mu.RLock()
	defer api.store.mu.RUnlock()

	switch {
	case strings.HasPrefix(name, "organizations/"):
		_, ok := api.store.Organizations[name]
		return ok
	case strings.HasPrefix(name, "folders/"):
		_, ok := api.store.Folders[name]
		return ok
	case strings.HasPrefix(name, "projects/"):
		return api.findProject(strings.TrimPrefix(name, "projects/")) != nil
	}
	return false
}
