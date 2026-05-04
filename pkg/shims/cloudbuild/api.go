package cloudbuild

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"strings"
	"sync"
	"time"

	"minisky/pkg/orchestrator"
	"minisky/pkg/registry"
)

func init() {
	registry.Register("cloudbuild.googleapis.com", func(ctx *registry.Context) http.Handler {
		return &API{
			svcMgr: ctx.SvcMgr,
			opMgr:  ctx.OpMgr,
		}
	})
}

type API struct {
	mu     sync.Mutex
	svcMgr *orchestrator.ServiceManager
	opMgr  *orchestrator.OperationManager
}

type Build struct {
	Id         string `json:"id,omitempty"`
	ProjectId  string `json:"projectId,omitempty"`
	Status     string `json:"status,omitempty"`
	Steps      []Step `json:"steps,omitempty"`
	CreateTime string `json:"createTime,omitempty"`
	StartTime  string `json:"startTime,omitempty"`
	FinishTime string `json:"finishTime,omitempty"`
	Source     *Source `json:"source,omitempty"`
}

type Source struct {
	RepoSource *RepoSource `json:"repoSource,omitempty"`
}

type RepoSource struct {
	RepoName  string `json:"repoName"` // e.g. "github.com/user/repo"
	BranchName string `json:"branchName,omitempty"`
}

type BuildTrigger struct {
	Id          string `json:"id,omitempty"`
	Description string `json:"description,omitempty"`
	Github      *GithubConfig `json:"github,omitempty"`
	Build       *Build `json:"build,omitempty"`
}

type GithubConfig struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
	Push  *PushFilter `json:"push,omitempty"`
}

type PushFilter struct {
	Branch string `json:"branch,omitempty"`
}

type Step struct {
	Name string   `json:"name"`
	Args []string `json:"args,omitempty"`
	Env  []string `json:"env,omitempty"`
	Dir  string   `json:"dir,omitempty"`
}

func (api *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	log.Printf("[Shim: Cloud Build] %s %s", r.Method, path)

	if r.Method == "POST" && strings.HasSuffix(path, "/builds") {
		parts := strings.Split(path, "/")
		var project string
		for i, p := range parts {
			if p == "projects" && i+1 < len(parts) {
				project = parts[i+1]
				break
			}
		}
		if project == "" { project = "local-dev-project" }
		api.handleCreateBuild(w, r, project)
		return
	}

	if r.Method == "GET" && strings.HasSuffix(path, "/builds") {
		parts := strings.Split(path, "/")
		var project string
		for i, p := range parts {
			if p == "projects" && i+1 < len(parts) {
				project = parts[i+1]
				break
			}
		}
		if project == "" { project = "local-dev-project" }
		api.handleListBuilds(w, r, project)
		return
	}

	if r.Method == "POST" && strings.HasSuffix(path, "/triggers") {
		parts := strings.Split(path, "/")
		var project string
		for i, p := range parts {
			if p == "projects" && i+1 < len(parts) {
				project = parts[i+1]
				break
			}
		}
		if project == "" { project = "local-dev-project" }
		api.handleCreateTrigger(w, r, project)
		return
	}

	if r.Method == "POST" && strings.Contains(path, "/triggers/") && strings.HasSuffix(path, ":run") {
		parts := strings.Split(path, "/")
		var project, triggerId string
		for i, p := range parts {
			if p == "projects" && i+1 < len(parts) {
				project = parts[i+1]
			}
			if p == "triggers" && i+1 < len(parts) {
				triggerId = parts[i+1]
			}
		}
		triggerId = strings.Split(triggerId, ":")[0]
		api.handleRunTrigger(w, r, project, triggerId)
		return
	}

	w.WriteHeader(http.StatusNotFound)
}

func (api *API) handleCreateBuild(w http.ResponseWriter, r *http.Request, project string) {
	var build Build
	if err := json.NewDecoder(r.Body).Decode(&build); err != nil {
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	build.Id = fmt.Sprintf("build-%d", time.Now().UnixNano())
	build.ProjectId = project
	build.Status = "QUEUED"
	build.CreateTime = time.Now().UTC().Format(time.RFC3339)

	op := api.opMgr.Register("cloudbuild#operation", "CREATE", fmt.Sprintf("/v1/projects/%s/builds/%s", project, build.Id), "", "")
	api.opMgr.UpdateMetadata(op.Name, build)
	api.pushLog(project, "INFO", build.Id, fmt.Sprintf("Build %s queued with %d steps", build.Id, len(build.Steps)))

	api.opMgr.RunAsync(op.Name, func() error {
		api.executeBuild(project, build, op.Name)
		return nil
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(op)
}

func (api *API) handleListBuilds(w http.ResponseWriter, r *http.Request, project string) {
	ops := api.opMgr.List()
	var builds []Build
	for _, op := range ops {
		if op.Kind == "cloudbuild#operation" {
			if b, ok := op.Metadata.(Build); ok && b.ProjectId == project {
				builds = append(builds, b)
			} else {
				// Try map decoding if it's from JSON unmarshal
				bBytes, _ := json.Marshal(op.Metadata)
				var b2 Build
				if err := json.Unmarshal(bBytes, &b2); err == nil && b2.ProjectId == project {
					builds = append(builds, b2)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"builds": builds})
}

func (api *API) executeBuild(project string, build Build, opName string) {
	build.Status = "WORKING"
	build.StartTime = time.Now().UTC().Format(time.RFC3339)
	api.opMgr.UpdateMetadata(opName, build)
	
	// Workspace volume for sharing code between steps
	workspaceVol := fmt.Sprintf("minisky-build-workspace-%s", build.Id)

	failed := false

	// Implicit step: Clone source if provided
	if build.Source != nil && build.Source.RepoSource != nil {
		repo := build.Source.RepoSource.RepoName
		if !strings.HasPrefix(repo, "http") {
			repo = "https://" + repo
		}
		branch := build.Source.RepoSource.BranchName
		if branch == "" { branch = "main" }

		api.pushLog(project, "INFO", build.Id, fmt.Sprintf("Cloning %s (branch: %s)...", repo, branch))
		
		cloneContainer := fmt.Sprintf("minisky-build-clone-%s", build.Id)
		// We use a helper container to clone into a volume
		err := api.svcMgr.ProvisionBuildStep(cloneContainer, "alpine/git:latest", []string{workspaceVol + ":/workspace"}, []string{}, []string{"clone", "-b", branch, repo, "/workspace"})
		if err != nil {
			api.pushLog(project, "ERROR", build.Id, fmt.Sprintf("Source clone failed: %v", err))
			failed = true
		} else {
			time.Sleep(3 * time.Second)
			api.svcMgr.StopAndRemoveContainer(cloneContainer)
		}
	}

	if !failed {
		for i, step := range build.Steps {
			api.pushLog(project, "INFO", build.Id, fmt.Sprintf("Step #%d: %s %s", i, step.Name, strings.Join(step.Args, " ")))
			
			img := step.Name
			if !strings.Contains(img, "/") && !strings.Contains(img, ":") {
				img = img + ":latest"
			}
			
			if strings.HasPrefix(img, "gcr.io/cloud-builders/") {
				tool := strings.TrimPrefix(img, "gcr.io/cloud-builders/")
				if tool == "docker" { img = "docker:latest" }
			}

			containerName := fmt.Sprintf("minisky-build-step-%s-%d", build.Id, i)
			// Mount the workspace volume to all steps
			err := api.svcMgr.ProvisionBuildStep(containerName, img, []string{workspaceVol + ":/workspace"}, step.Env, step.Args)
			if err != nil {
				api.pushLog(project, "ERROR", build.Id, fmt.Sprintf("Step #%d failed: %v", i, err))
				failed = true
				break
			}
			
			time.Sleep(3 * time.Second) // Simulate build time
			api.pushLog(project, "INFO", build.Id, fmt.Sprintf("Step #%d finished successfully", i))
			api.svcMgr.StopAndRemoveContainer(containerName)
		}
	}

	build.FinishTime = time.Now().UTC().Format(time.RFC3339)
	if failed {
		build.Status = "FAILURE"
		api.opMgr.Fail(opName, 500, "Build failed")
	} else {
		build.Status = "SUCCESS"
		api.pushLog(project, "INFO", build.Id, "Build SUCCESS")
	}
	api.opMgr.UpdateMetadata(opName, build)
}

func (api *API) handleCreateTrigger(w http.ResponseWriter, r *http.Request, project string) {
	var trigger BuildTrigger
	json.NewDecoder(r.Body).Decode(&trigger)
	trigger.Id = fmt.Sprintf("trigger-%d", time.Now().Unix())
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(trigger)
}

func (api *API) handleRunTrigger(w http.ResponseWriter, r *http.Request, project, triggerId string) {
	// In a real implementation, we'd look up the trigger by ID.
	// For the emulator, we just simulate starting a build from "GitHub"
	build := Build{
		Id: fmt.Sprintf("build-trigger-%d", time.Now().Unix()),
		ProjectId: project,
		Status: "QUEUED",
		CreateTime: time.Now().UTC().Format(time.RFC3339),
		Source: &Source{
			RepoSource: &RepoSource{
				RepoName: "github.com/GoogleCloudPlatform/cloud-builders",
				BranchName: "master",
			},
		},
		Steps: []Step{
			{Name: "ubuntu", Args: []string{"echo", "Triggered from GitHub!"}},
		},
	}

	op := api.opMgr.Register("cloudbuild#operation", "RUN_TRIGGER", fmt.Sprintf("/v1/projects/%s/builds/%s", project, build.Id), "", "")
	api.opMgr.UpdateMetadata(op.Name, build)
	go api.executeBuild(project, build, op.Name)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(op)
}

func (api *API) Proxy() *httputil.ReverseProxy {
	return nil // Not used in this implementation style
}

func (api *API) pushLog(project, severity, id, msg string) {
	log.Printf("[%s] BUILD %s: %s", severity, id, msg)
}
