package resourcemanager

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// projectIDPattern is Google's rule: 6–30 characters, lowercase letters,
// digits and hyphens, starting with a letter and not ending with a hyphen.
var errInvalidParent = errors.New("parent must be organizations/{id} or folders/{id}")

var projectIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`)

func (api *API) routeProjects(w http.ResponseWriter, r *http.Request, path string) {
	version := apiVersion(path)
	id := resourceIDFrom(path, "projects")

	switch {
	case id == "" && r.Method == http.MethodPost:
		api.createProject(w, r, version)
	case id == "" && r.Method == http.MethodGet:
		api.listProjects(w, r, version)
	case id != "" && r.Method == http.MethodGet:
		api.getProject(w, id, version)
	case id != "" && (r.Method == http.MethodPut || r.Method == http.MethodPatch):
		api.updateProject(w, r, id, version)
	case id != "" && r.Method == http.MethodDelete:
		api.deleteProject(w, id, version)
	default:
		writeError(w, http.StatusMethodNotAllowed, "FAILED_PRECONDITION",
			r.Method+" is not supported on "+path)
	}
}

// createProject implements projects.create for both generations, which disagree
// about the request shape: v1 sends projectId with parent as
// {"type":"folder","id":"123"}, v3 sends projectId with parent as
// "folders/123" and calls the display name displayName rather than name.
func (api *API) createProject(w http.ResponseWriter, r *http.Request, version string) {
	var body struct {
		ProjectID   string            `json:"projectId"`
		Name        string            `json:"name"`        // v1 display name
		DisplayName string            `json:"displayName"` // v3 display name
		Parent      json.RawMessage   `json:"parent"`
		Labels      map[string]string `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: "+err.Error())
		return
	}

	if !projectIDPattern.MatchString(body.ProjectID) {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT",
			"projectId must be 6-30 characters of lowercase letters, digits and hyphens, "+
				"starting with a letter")
		return
	}

	parent, err := parseParent(body.Parent)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
		return
	}

	displayName := body.DisplayName
	if displayName == "" {
		displayName = body.Name
	}
	if displayName == "" {
		displayName = body.ProjectID
	}

	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	if _, exists := api.store.Projects[body.ProjectID]; exists {
		writeError(w, http.StatusConflict, "ALREADY_EXISTS",
			"project "+body.ProjectID+" already exists")
		return
	}
	if parent != "" && !api.store.resourceExists(parent) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "parent "+parent+" not found")
		return
	}

	now := time.Now().UTC()
	p := &project{
		Number:      api.store.newProjectNumber(),
		ProjectID:   body.ProjectID,
		DisplayName: displayName,
		Parent:      parent,
		State:       StateActive,
		Labels:      body.Labels,
		CreateTime:  now,
		UpdateTime:  now,
		Etag:        newEtag(),
	}
	api.store.Projects[p.ProjectID] = p
	api.store.save()

	// v1 answers with an Operation; v3 answers with one too, and both are
	// complete by the time the response is written.
	writeJSON(w, api.completedOperation(renderProject(p, version)))
}

func (api *API) getProject(w http.ResponseWriter, id, version string) {
	api.store.mu.RLock()
	p := api.findProject(id)
	api.store.mu.RUnlock()

	if p == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "project "+id+" not found")
		return
	}
	writeJSON(w, renderProject(p, version))
}

// listProjects implements projects.list. v3 requires a parent; v1 lists
// everything the caller can see, which here is everything.
func (api *API) listProjects(w http.ResponseWriter, r *http.Request, version string) {
	parent := r.URL.Query().Get("parent")
	showDeleted := r.URL.Query().Get("showDeleted") == "true"

	api.store.mu.RLock()
	defer api.store.mu.RUnlock()

	items := []interface{}{}
	for _, p := range activeProjectsIncluding(api.store.Projects, showDeleted) {
		if parent != "" && p.Parent != parent {
			continue
		}
		items = append(items, renderProject(p, version))
	}
	writeJSON(w, map[string]interface{}{"projects": items})
}

// updateProject changes the mutable fields: the display name and labels. A
// project's parent moves with projects.move in v3, which Terraform does not
// use, and its id never changes.
func (api *API) updateProject(w http.ResponseWriter, r *http.Request, id, version string) {
	var body struct {
		Name        string            `json:"name"`
		DisplayName string            `json:"displayName"`
		Labels      map[string]string `json:"labels"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: "+err.Error())
		return
	}

	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	p := api.findProject(id)
	if p == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "project "+id+" not found")
		return
	}

	if name := firstNonEmpty(body.DisplayName, body.Name); name != "" {
		p.DisplayName = name
	}
	if body.Labels != nil {
		p.Labels = body.Labels
	}
	p.UpdateTime = time.Now().UTC()
	p.Etag = newEtag()
	api.store.save()

	writeJSON(w, renderProject(p, version))
}

// deleteProject marks the project DELETE_REQUESTED. Google keeps it for thirty
// days so it can be undeleted, and reports it in that state meanwhile.
func (api *API) deleteProject(w http.ResponseWriter, id, version string) {
	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	p := api.findProject(id)
	if p == nil {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "project "+id+" not found")
		return
	}

	now := time.Now().UTC()
	p.State = StateDeleteRequested
	p.DeleteTime = &now
	p.UpdateTime = now
	p.Etag = newEtag()
	api.store.save()

	writeJSON(w, renderProject(p, version))
}

// findProject resolves a project by the id its creator chose or by the number
// Google assigned, since v3 addresses it as projects/{project_number}. Callers
// hold the lock.
func (api *API) findProject(id string) *project {
	if p, ok := api.store.Projects[id]; ok {
		return p
	}
	for _, p := range api.store.Projects {
		if p.Number == id {
			return p
		}
	}
	return nil
}

// renderProject emits the shape the requested generation expects.
func renderProject(p *project, version string) interface{} {
	if version == "v1" {
		out := &ProjectV1{
			ProjectNumber:  p.Number,
			ProjectID:      p.ProjectID,
			Name:           p.DisplayName,
			LifecycleState: p.State,
			Labels:         p.Labels,
			CreateTime:     p.CreateTime.Format(time.RFC3339),
		}
		if p.Parent != "" {
			kind, id, ok := strings.Cut(p.Parent, "/")
			if ok {
				out.Parent = &ResourceID{Type: strings.TrimSuffix(kind, "s"), ID: id}
			}
		}
		return out
	}

	out := &ProjectV3{
		Name:        "projects/" + p.Number,
		Parent:      p.Parent,
		ProjectID:   p.ProjectID,
		State:       p.State,
		DisplayName: p.DisplayName,
		Labels:      p.Labels,
		CreateTime:  p.CreateTime.Format(time.RFC3339),
		UpdateTime:  p.UpdateTime.Format(time.RFC3339),
		Etag:        p.Etag,
	}
	if p.DeleteTime != nil {
		out.DeleteTime = p.DeleteTime.Format(time.RFC3339)
	}
	return out
}

// parseParent accepts both spellings: v3's "folders/123" and v1's
// {"type":"folder","id":"123"}.
func parseParent(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}

	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if asString == "" {
			return "", nil
		}
		if !strings.HasPrefix(asString, "organizations/") && !strings.HasPrefix(asString, "folders/") {
			return "", errInvalidParent
		}
		return asString, nil
	}

	var asObject ResourceID
	if err := json.Unmarshal(raw, &asObject); err != nil {
		return "", errInvalidParent
	}
	if asObject.ID == "" {
		return "", nil
	}
	switch asObject.Type {
	case "organization":
		return "organizations/" + asObject.ID, nil
	case "folder":
		return "folders/" + asObject.ID, nil
	}
	return "", errInvalidParent
}

func activeProjectsIncluding(all map[string]*project, includeDeleted bool) []*project {
	if !includeDeleted {
		return activeProjects(all)
	}
	out := []*project{}
	for _, p := range all {
		out = append(out, p)
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
