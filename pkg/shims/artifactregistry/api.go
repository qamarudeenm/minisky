package artifactregistry

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
)

func init() {
	registry.Register("artifactregistry.googleapis.com", func(ctx *registry.Context) http.Handler {
		return &API{
			svcMgr: ctx.SvcMgr,
			repos:  make(map[string]*Repository),
		}
	})
}

type Repository struct {
	Name        string            `json:"name"`
	Format      string            `json:"format"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	CreateTime  string            `json:"createTime,omitempty"`
	UpdateTime  string            `json:"updateTime,omitempty"`
}

type Package struct {
	Name        string `json:"name"`
	DisplayName string `displayName,omitempty"`
	CreateTime  string `json:"createTime,omitempty"`
	UpdateTime  string `json:"updateTime,omitempty"`
}

type Version struct {
	Name       string   `json:"name"`
	Description string  `json:"description,omitempty"`
	CreateTime string   `json:"createTime,omitempty"`
	RelatedTags []string `json:"relatedTags,omitempty"`
}

type API struct {
	svcMgr *orchestrator.ServiceManager
	repos  map[string]*Repository
}

func NewAPI(sm *orchestrator.ServiceManager) *API {
	return &API{
		svcMgr: sm,
		repos:  make(map[string]*Repository),
	}
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	// v1/projects/{project}/locations/{location}/repositories
	if strings.Contains(path, "/repositories") {
		if r.Method == "GET" && !strings.Contains(path, "/packages") {
			api.handleListRepositories(w, r)
			return
		}
		if r.Method == "POST" {
			api.handleCreateRepository(w, r)
			return
		}
		if strings.Contains(path, "/packages") {
			if strings.Contains(path, "/versions") {
				api.handleListVersions(w, r)
				return
			}
			api.handleListPackages(w, r)
			return
		}
	}

	w.WriteHeader(http.StatusNotFound)
}

func (api *API) handleListRepositories(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	var project string
	for i, p := range parts {
		if p == "projects" && i+1 < len(parts) {
			project = parts[i+1]
			break
		}
	}

	var list []Repository
	for _, repo := range api.repos {
		if strings.Contains(repo.Name, fmt.Sprintf("projects/%s", project)) {
			list = append(list, *repo)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"repositories": list,
	})
}

func (api *API) handleCreateRepository(w http.ResponseWriter, r *http.Request) {
	var repo Repository
	if err := json.NewDecoder(r.Body).Decode(&repo); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Path: v1/projects/{project}/locations/{location}/repositories?repositoryId=...
	parts := strings.Split(r.URL.Path, "/")
	project := "default"
	location := "us-central1"
	
	for i, p := range parts {
		if p == "projects" && i+1 < len(parts) {
			project = parts[i+1]
		}
		if p == "locations" && i+1 < len(parts) {
			location = parts[i+1]
		}
	}

	repoId := r.URL.Query().Get("repositoryId")
	repo.Name = fmt.Sprintf("projects/%s/locations/%s/repositories/%s", project, location, repoId)
	repo.CreateTime = time.Now().Format(time.RFC3339)
	repo.UpdateTime = repo.CreateTime
	
	api.repos[repo.Name] = &repo

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(repo)
}

func (api *API) handleListPackages(w http.ResponseWriter, r *http.Request) {
	// Dummy implementation returning mocked packages for now
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"packages": []Package{
			{Name: "my-app", DisplayName: "My Application"},
		},
	})
}

func (api *API) handleListVersions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"versions": []Version{
			{Name: "v1.0.0", CreateTime: time.Now().Format(time.RFC3339), RelatedTags: []string{"latest"}},
		},
	})
}

func (api *API) Proxy() *httputil.ReverseProxy {
	return nil
}
