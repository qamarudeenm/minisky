package resourcemanager

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
// Policy analysis
//
// MiniSky does not enforce IAM: it authenticates nobody, so it cannot answer
// "is this caller allowed". What it can answer exactly is what the policies
// say — which is the question a landing zone is actually built to check.
// Access is inherited down the hierarchy, so a binding on the organization
// reaches every project beneath it, and that inheritance is where a policy
// usually turns out to grant more than its author expected.
//
// A role answer here is exact: it comes from the stored bindings. A permission
// answer is drawn from a curated catalogue and says so, because the absence of
// a permission may mean the catalogue is short rather than that access is
// denied.
// ─────────────────────────────────────────────────────────────────────────────

// GrantedRole is one role a principal holds, and the resource it came from.
type GrantedRole struct {
	Role        string `json:"role"`
	GrantedOn   string `json:"grantedOn"`
	Inherited   bool   `json:"inherited"`
	RoleKnown   bool   `json:"roleKnown"`
	Permissions int    `json:"permissionCount"`
}

// Explanation answers "what does this principal have on this resource".
type Explanation struct {
	Principal string        `json:"principal"`
	Resource  string        `json:"resource"`
	Ancestry  []string      `json:"ancestry"`
	Roles     []GrantedRole `json:"roles"`

	// Permission is only present when one was asked about.
	Permission  string   `json:"permission,omitempty"`
	Granted     *bool    `json:"granted,omitempty"`
	GrantedBy   []string `json:"grantedBy,omitempty"`
	UnknownRole []string `json:"rolesNotInCatalogue,omitempty"`
	Caveat      string   `json:"caveat,omitempty"`
}

// ancestryOf returns the resource and each ancestor above it, nearest first.
// This is the order access is inherited in.
func (api *API) ancestryOf(resource string) []string {
	api.store.mu.RLock()
	defer api.store.mu.RUnlock()
	return api.ancestryLocked(resource)
}

func (api *API) ancestryLocked(resource string) []string {
	chain := []string{resource}
	cursor := resource

	for range make([]struct{}, maxHierarchyDepth+2) {
		var parent string
		switch {
		case strings.HasPrefix(cursor, "projects/"):
			if p := api.findProject(strings.TrimPrefix(cursor, "projects/")); p != nil {
				parent = p.Parent
			}
		case strings.HasPrefix(cursor, "folders/"):
			if f, ok := api.store.Folders[cursor]; ok {
				parent = f.Parent
			}
		}
		if parent == "" {
			break
		}
		chain = append(chain, parent)
		cursor = parent
	}
	return chain
}

// Explain reports what a principal holds on a resource, and optionally whether
// that amounts to a given permission.
func (api *API) Explain(principal, resource, permission string) Explanation {
	explanation := Explanation{
		Principal:  principal,
		Resource:   resource,
		Permission: permission,
	}

	api.store.mu.RLock()
	defer api.store.mu.RUnlock()

	explanation.Ancestry = api.ancestryLocked(resource)

	for i, ancestor := range explanation.Ancestry {
		policy, ok := api.store.Policies[ancestor]
		if !ok {
			continue
		}
		for _, binding := range policy.Bindings {
			if !bindingCovers(binding, principal) {
				continue
			}
			permissions, known := permissionsFor(binding.Role)
			explanation.Roles = append(explanation.Roles, GrantedRole{
				Role:        binding.Role,
				GrantedOn:   ancestor,
				Inherited:   i > 0,
				RoleKnown:   known,
				Permissions: len(permissions),
			})
		}
	}

	sort.Slice(explanation.Roles, func(i, j int) bool {
		if explanation.Roles[i].Inherited != explanation.Roles[j].Inherited {
			return !explanation.Roles[i].Inherited
		}
		return explanation.Roles[i].Role < explanation.Roles[j].Role
	})

	if permission == "" {
		return explanation
	}

	granted := false
	for _, held := range explanation.Roles {
		grants, known := roleGrants(held.Role, permission)
		if !known {
			explanation.UnknownRole = append(explanation.UnknownRole, held.Role)
			continue
		}
		if grants {
			granted = true
			explanation.GrantedBy = append(explanation.GrantedBy,
				held.Role+" on "+held.GrantedOn)
		}
	}
	explanation.Granted = &granted

	switch {
	case len(explanation.UnknownRole) > 0:
		explanation.Caveat = "some roles held are not in the curated catalogue, so this answer is incomplete; " +
			"extend it with ~/.minisky/roles.json"
	case !granted:
		explanation.Caveat = "the permission is absent from the curated catalogue's view of these roles, " +
			"which is not the same as being denied by Google"
	}
	return explanation
}

// PrincipalsWith answers the reverse question: who can do this here. It is the
// one a review actually starts from.
func (api *API) PrincipalsWith(resource, permission string) []string {
	api.store.mu.RLock()
	defer api.store.mu.RUnlock()

	found := map[string]bool{}
	for _, ancestor := range api.ancestryLocked(resource) {
		policy, ok := api.store.Policies[ancestor]
		if !ok {
			continue
		}
		for _, binding := range policy.Bindings {
			if grants, known := roleGrants(binding.Role, permission); !known || !grants {
				continue
			}
			for _, member := range binding.Members {
				found[member] = true
			}
		}
	}

	principals := []string{}
	for m := range found {
		principals = append(principals, m)
	}
	sort.Strings(principals)
	return principals
}

// bindingCovers reports whether a binding applies to a principal, honouring the
// wildcard members Google defines.
func bindingCovers(binding IamBinding, principal string) bool {
	for _, member := range binding.Members {
		switch member {
		case principal, "allUsers":
			return true
		case "allAuthenticatedUsers":
			// Everything except an anonymous caller, and MiniSky has no
			// anonymous principal.
			return true
		}
		// domain:example.com covers every principal at that domain.
		if domain, ok := strings.CutPrefix(member, "domain:"); ok {
			if strings.HasSuffix(principal, "@"+domain) {
				return true
			}
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// HTTP surface
//
// This is a MiniSky endpoint rather than a Google one: the closest real
// equivalent is the Policy Troubleshooter API, which answers for an
// authenticated caller MiniSky does not have.
// ─────────────────────────────────────────────────────────────────────────────

func (api *API) handleAnalyze(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Principal  string `json:"principal"`
		Resource   string `json:"resource"`
		Permission string `json:"permission"`
	}
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: "+err.Error())
			return
		}
	}
	if body.Resource == "" {
		body.Resource = r.URL.Query().Get("resource")
	}
	if body.Principal == "" {
		body.Principal = r.URL.Query().Get("principal")
	}
	if body.Permission == "" {
		body.Permission = r.URL.Query().Get("permission")
	}

	if body.Resource == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT",
			"resource is required, as organizations/{id}, folders/{id} or projects/{id}")
		return
	}

	if body.Principal == "" {
		if body.Permission == "" {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT",
				"give a principal, a permission, or both")
			return
		}
		writeJSON(w, map[string]interface{}{
			"resource":   body.Resource,
			"permission": body.Permission,
			"principals": api.PrincipalsWith(body.Resource, body.Permission),
		})
		return
	}

	writeJSON(w, api.Explain(body.Principal, body.Resource, body.Permission))
}
