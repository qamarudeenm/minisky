package resourcemanager

import (
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"
)

// displayNamePattern is Google's rule for a folder name: 3–30 characters of
// letters, digits, spaces, hyphens and underscores, starting with a letter or
// digit.
var displayNamePattern = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} '_-]{1,28}[\p{L}\p{N}]$`)

func (api *API) routeFolders(w http.ResponseWriter, r *http.Request, path string) {
	id := resourceIDFrom(path, "folders")

	switch {
	case id == "" && r.Method == http.MethodPost:
		api.createFolder(w, r)
	case id == "" && r.Method == http.MethodGet:
		api.listFolders(w, r)
	case id != "" && r.Method == http.MethodGet:
		api.getFolder(w, id)
	case id != "" && (r.Method == http.MethodPatch || r.Method == http.MethodPut):
		api.patchFolder(w, r, id)
	case id != "" && r.Method == http.MethodDelete:
		api.deleteFolder(w, id)
	default:
		writeError(w, http.StatusMethodNotAllowed, "FAILED_PRECONDITION",
			r.Method+" is not supported on "+path)
	}
}

// createFolder implements folders.create. The caller supplies a display name
// and a parent; Google assigns the id, which is why a folder is addressed as
// folders/{id} and never by its name.
func (api *API) createFolder(w http.ResponseWriter, r *http.Request) {
	var body Folder
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: "+err.Error())
		return
	}

	if !displayNamePattern.MatchString(body.DisplayName) {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT",
			"displayName must be 3-30 characters of letters, digits, spaces, hyphens or underscores")
		return
	}
	if body.Parent == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT",
			"parent is required, as organizations/{id} or folders/{id}")
		return
	}

	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	if !api.store.resourceExists(body.Parent) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "parent "+body.Parent+" not found")
		return
	}
	if api.store.depthOf(body.Parent) >= maxHierarchyDepth {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT",
			"a folder cannot be nested more than 10 levels below its organization")
		return
	}
	if api.store.siblingNameTaken(body.Parent, body.DisplayName, "") {
		writeError(w, http.StatusConflict, "ALREADY_EXISTS",
			"a folder named "+body.DisplayName+" already exists under "+body.Parent)
		return
	}

	now := nowRFC3339()
	folder := &Folder{
		Name:        "folders/" + api.store.newFolderID(),
		Parent:      body.Parent,
		DisplayName: body.DisplayName,
		State:       StateActive,
		CreateTime:  now,
		UpdateTime:  now,
		Etag:        newEtag(),
	}
	api.store.Folders[folder.Name] = folder
	api.store.save()

	writeJSON(w, api.completedOperation(folder))
}

func (api *API) getFolder(w http.ResponseWriter, id string) {
	api.store.mu.RLock()
	folder, ok := api.store.Folders["folders/"+id]
	api.store.mu.RUnlock()

	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "folders/"+id+" not found")
		return
	}
	writeJSON(w, folder)
}

// listFolders implements folders.list, which is scoped to one parent. Google
// requires the parent: there is no way to list every folder in a hierarchy.
func (api *API) listFolders(w http.ResponseWriter, r *http.Request) {
	parent := r.URL.Query().Get("parent")
	if parent == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT",
			"parent is required, as ?parent=organizations/{id} or ?parent=folders/{id}")
		return
	}
	showDeleted := r.URL.Query().Get("showDeleted") == "true"

	api.store.mu.RLock()
	defer api.store.mu.RUnlock()

	folders := []*Folder{}
	for _, f := range api.store.Folders {
		if f.Parent != parent {
			continue
		}
		if f.State != StateActive && !showDeleted {
			continue
		}
		folders = append(folders, f)
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].DisplayName < folders[j].DisplayName })

	writeJSON(w, map[string]interface{}{"folders": folders})
}

// patchFolder implements folders.patch. Only displayName is mutable — a parent
// is changed with folders.move, not by patching.
func (api *API) patchFolder(w http.ResponseWriter, r *http.Request, id string) {
	var body Folder
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid request body: "+err.Error())
		return
	}

	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	folder, ok := api.store.Folders["folders/"+id]
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "folders/"+id+" not found")
		return
	}

	if body.DisplayName != "" && body.DisplayName != folder.DisplayName {
		if !displayNamePattern.MatchString(body.DisplayName) {
			writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "displayName is not valid")
			return
		}
		if api.store.siblingNameTaken(folder.Parent, body.DisplayName, folder.Name) {
			writeError(w, http.StatusConflict, "ALREADY_EXISTS",
				"a folder named "+body.DisplayName+" already exists under "+folder.Parent)
			return
		}
		folder.DisplayName = body.DisplayName
	}

	folder.UpdateTime = nowRFC3339()
	folder.Etag = newEtag()
	api.store.save()

	writeJSON(w, api.completedOperation(folder))
}

// deleteFolder implements folders.delete, which marks the folder
// DELETE_REQUESTED rather than removing it, and refuses while anything still
// lives underneath.
func (api *API) deleteFolder(w http.ResponseWriter, id string) {
	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	name := "folders/" + id
	folder, ok := api.store.Folders[name]
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", name+" not found")
		return
	}
	if api.store.hasChildren(name) {
		writeError(w, http.StatusBadRequest, "FAILED_PRECONDITION",
			"cannot delete "+name+" while it still contains folders or projects")
		return
	}

	folder.State = StateDeleteRequested
	folder.DeleteTime = nowRFC3339()
	folder.UpdateTime = folder.DeleteTime
	folder.Etag = newEtag()
	api.store.save()

	writeJSON(w, api.completedOperation(folder))
}

// moveFolder implements folders.move, the only way a folder changes parent.
func (api *API) moveFolder(w http.ResponseWriter, r *http.Request, resource string) {
	id := resourceIDFrom(resource, "folders")
	if id == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "move applies to a folder")
		return
	}

	var body struct {
		DestinationParent string `json:"destinationParent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.DestinationParent == "" {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "destinationParent is required")
		return
	}

	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	name := "folders/" + id
	folder, ok := api.store.Folders[name]
	if !ok {
		writeError(w, http.StatusNotFound, "NOT_FOUND", name+" not found")
		return
	}
	if !api.store.resourceExists(body.DestinationParent) {
		writeError(w, http.StatusNotFound, "NOT_FOUND", "destination "+body.DestinationParent+" not found")
		return
	}
	if api.store.wouldCycle(name, body.DestinationParent) {
		writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT",
			"cannot move "+name+" beneath itself")
		return
	}
	if api.store.siblingNameTaken(body.DestinationParent, folder.DisplayName, name) {
		writeError(w, http.StatusConflict, "ALREADY_EXISTS",
			"a folder named "+folder.DisplayName+" already exists under "+body.DestinationParent)
		return
	}

	folder.Parent = body.DestinationParent
	folder.UpdateTime = nowRFC3339()
	folder.Etag = newEtag()
	api.store.save()

	writeJSON(w, api.completedOperation(folder))
}

// undelete restores a folder or project that was marked for deletion.
func (api *API) undelete(w http.ResponseWriter, r *http.Request, resource string) {
	api.store.mu.Lock()
	defer api.store.mu.Unlock()

	if id := resourceIDFrom(resource, "folders"); id != "" {
		folder, ok := api.store.Folders["folders/"+id]
		if !ok {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "folders/"+id+" not found")
			return
		}
		if folder.State == StateActive {
			writeError(w, http.StatusBadRequest, "FAILED_PRECONDITION", "folders/"+id+" is not deleted")
			return
		}
		folder.State = StateActive
		folder.DeleteTime = ""
		folder.UpdateTime = nowRFC3339()
		folder.Etag = newEtag()
		api.store.save()
		writeJSON(w, api.completedOperation(folder))
		return
	}

	if id := resourceIDFrom(resource, "projects"); id != "" {
		p, ok := api.store.Projects[id]
		if !ok {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "project "+id+" not found")
			return
		}
		if p.State == StateActive {
			writeError(w, http.StatusBadRequest, "FAILED_PRECONDITION", "project "+id+" is not deleted")
			return
		}
		p.State = StateActive
		p.DeleteTime = nil
		p.UpdateTime = time.Now().UTC()
		p.Etag = newEtag()
		api.store.save()
		writeJSON(w, renderProject(p, apiVersion(resource)))
		return
	}

	writeError(w, http.StatusBadRequest, "INVALID_ARGUMENT", "undelete applies to a folder or project")
}

// trimVersion drops the leading /v1 or /v3 so a resource reads as Google names
// it: folders/123, projects/my-app.
func trimVersion(path string) string {
	for _, prefix := range []string{"/v1/", "/v3/"} {
		if strings.HasPrefix(path, prefix) {
			return strings.TrimPrefix(path, prefix)
		}
	}
	return strings.TrimPrefix(path, "/")
}
